// Package store reads and writes accounts' workspaces in the app's database (the typle_workspaces table, and their
// practice records in typle_records: see records.go and db/migrations).
package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sparklingstadt/typle-backends/server/internal/workspace"
)

// ErrConflict means the revision a save was made against is no longer the saved one, or the workspace it expected to
// create is already there. The app reloads and asks again (409).
var ErrConflict = errors.New("workspace revision is stale")

type Store struct{ pool *pgxpool.Pool }

// OpenApp makes the pool as createPool of src/server/database.ts makes node-postgres's, so the backends are compared
// on the same terms: at most 5 connections, closed after 30 seconds idle, connecting given up after 5 seconds. Nothing
// is connected until the first query, as with node-postgres: a database that is down fails the request that needs it.
// Queries are sent as node-postgres sends them, unnamed and in one round trip (QueryExecModeExec), rather than
// prepared first and cached per connection.
func OpenApp(ctx context.Context, connection string) (*Store, error) {
	strict, err := ConnString(connection)
	if err != nil {
		return nil, err
	}
	config, err := pgxpool.ParseConfig(strict)
	if err != nil {
		return nil, fmt.Errorf("database URL: %w", err)
	}
	config.MaxConns = 5
	config.MaxConnIdleTime = 30 * time.Second
	config.ConnConfig.ConnectTimeout = 5 * time.Second
	config.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

// lockOwner holds the owner's advisory lock until the end of tx, as writeWorkspace of src/server/workspace.ts takes it
// for a first save: it stands in for the row lock where there is no row yet, so two first saves wait for each other.
func lockOwner(ctx context.Context, tx pgx.Tx, owner string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('typle_workspaces:' || $1))`, owner)
	return err
}

// Ping checks that the database answers.
func (s *Store) Ping(ctx context.Context) error {
	var one int
	return s.pool.QueryRow(ctx, `SELECT 1`).Scan(&one)
}

func (s *Store) Close() { s.pool.Close() }

// decodeSaved reads a workspace as it is stored, naming the owner whose saved data cannot be read.
func decodeSaved(owner string, lists, devices, packs []byte) (*workspace.Workspace, error) {
	w, err := workspace.Decode(lists, devices, packs)
	if err != nil {
		return nil, fmt.Errorf("saved workspace of %s: %w", owner, err)
	}
	return w, nil
}

// Modify changes the owner's workspace and saves it, in one transaction that holds the row locked. While it is held,
// a save from the app waits; afterwards the app finds the revision changed and reloads (409), as it does when another
// browser saved first. An account that has saved nothing starts from the initial workspace. When change reports nothing
// changed, nothing is written.
//
// change edits the lists as Decode shows them, with their packs' category and tags; ApplyListChange then puts those
// edits back into the packs, so the workspace returned has its lists in their packs (a new list in a pack of its own)
// and both are written. Anything change keeps of the workspace, such as a pointer into its lists, is from before that:
// find the list again in the workspace returned.
func (s *Store) Modify(ctx context.Context, owner string, change func(*workspace.Workspace) (bool, error)) (*workspace.Workspace, error) {
	var result *workspace.Workspace
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		w, before, err := lock(ctx, tx, owner)
		if err != nil {
			return err
		}
		lists, packs, err := encodeLibrary(w)
		if err != nil {
			return err
		}
		// ApplyListChange reads the lists and packs as they were; change may edit the lists in place.
		previous := &workspace.Workspace{Lists: slices.Clone(w.Lists), Packs: slices.Clone(w.Packs)}
		changed, err := change(w)
		if err != nil {
			return err
		}
		w.ApplyListChange(previous)
		result = w
		if !changed {
			return nil
		}
		return saveModified(ctx, tx, owner, w, before, len(lists)+len(packs))
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// saveModified writes w, changed by Modify, over the owner's locked row: before is its records as they were read
// (see lock), and size the bytes of its lists and packs, which CheckSize compares the changed ones with.
func saveModified(ctx context.Context, tx pgx.Tx, owner string, w *workspace.Workspace, before map[recordKey]recordRow, size int) error {
	// Records are given ids before anything is encoded, so the workspace returned and the rows agree.
	rows, err := recordRows(w.Lists)
	if err != nil {
		return err
	}
	after, afterPacks, err := encodeLibrary(w)
	if err != nil {
		return err
	}
	if err := workspace.CheckSize(size, len(after)+len(afterPacks)); err != nil {
		return err
	}
	devices, err := workspace.EncodeDevices(w.Devices)
	if err != nil {
		return err
	}
	// A workspace that would not be read back is never saved: the account would no longer load, here or in the app.
	if _, err := workspace.Decode(after, devices, afterPacks); err != nil {
		return fmt.Errorf("changed workspace of %s: %w", owner, err)
	}
	stored, err := workspace.Encode(withoutRecords(workspace.WithoutFollowedWords(w.Packs, w.Lists)))
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE typle_workspaces SET lists = $2::jsonb, packs = $3::jsonb, devices = $4::jsonb,
		records_split = true, revision = revision + 1, updated_at = now() WHERE owner_id = $1`,
		owner, string(stored), string(afterPacks), string(devices)); err != nil {
		return err
	}
	return syncRecords(ctx, tx, owner, before, rows)
}

// encodeLibrary writes the lists and packs of w as the app saves them: the lists with their records, and without the
// words of followed packs (workspace.WithoutFollowedWords).
func encodeLibrary(w *workspace.Workspace) (lists, packs []byte, err error) {
	if lists, err = workspace.Encode(workspace.WithoutFollowedWords(w.Packs, w.Lists)); err != nil {
		return nil, nil, err
	}
	packs, err = workspace.EncodePacks(w.Packs)
	return lists, packs, err
}

// lock reads the owner's row FOR UPDATE, first inserting one with the initial workspace when there is
// none. Another request may insert it at the same moment; the row is then read and locked once that request is done
// with it. The records are read after the row is locked, so that no write can come between: before is them as rows
// (beforeRows), or nil when the workspace's records have not moved to typle_records yet.
func lock(ctx context.Context, tx pgx.Tx, owner string) (w *workspace.Workspace, before map[recordKey]recordRow, err error) {
	for inserted := false; ; inserted = true {
		var lists, packs, devices []byte
		var split bool
		err := tx.QueryRow(ctx, `SELECT lists::text, packs::text, devices::text, records_split FROM typle_workspaces
			WHERE owner_id = $1 FOR UPDATE`, owner).Scan(&lists, &packs, &devices, &split)
		if err == nil {
			if split {
				records, stored, err := readRecords(ctx, tx, owner)
				if err != nil {
					return nil, nil, err
				}
				if lists, err = withRecords(lists, records); err != nil {
					return nil, nil, fmt.Errorf("saved workspace of %s: %w", owner, err)
				}
				if before, err = beforeRows(stored); err != nil {
					return nil, nil, fmt.Errorf("saved workspace of %s: %w", owner, err)
				}
			}
			w, err := decodeSaved(owner, lists, devices, packs)
			if err != nil {
				return nil, nil, err
			}
			return w, before, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) || inserted {
			return nil, nil, err
		}
		// As the first save does (see lockOwner).
		if err := lockOwner(ctx, tx, owner); err != nil {
			return nil, nil, err
		}
		initial, err := workspace.Initial()
		if err != nil {
			return nil, nil, err
		}
		encoded, encodedPacks, err := encodeLibrary(initial)
		if err != nil {
			return nil, nil, err
		}
		// The initial lists have no records, so the row starts with them in typle_records.
		created, err := tx.Exec(ctx, `INSERT INTO typle_workspaces (owner_id, lists, packs, devices, records_split)
			VALUES ($1, $2::jsonb, $3::jsonb, '[]'::jsonb, true) ON CONFLICT (owner_id) DO NOTHING`,
			owner, string(encoded), string(encodedPacks))
		if err == nil && created.RowsAffected() == 1 {
			err = forgetRecords(ctx, tx, owner)
		}
		if err != nil {
			return nil, nil, err
		}
	}
}
