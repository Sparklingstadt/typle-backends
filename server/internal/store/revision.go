package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/sparklingstadt/typle-backends/server/internal/workspace"
)

// Saving by revision (/api/workspace): a save names the revision it was made from, and is refused when the workspace
// has moved on since.

// Saved is a workspace as stored: its lists, packs, devices and interests columns as JSON, with the revision a later
// save must name. The JSON is the database's own text of the jsonb, so object keys may be in another order than they
// were written in. Packs is nil, which is JSON's null, when the column is NULL: the lists were saved before packs
// existed. Interests are nil when their column is NULL: none have been saved.
type Saved struct {
	Lists, Packs, Devices, Interests json.RawMessage
	Revision                         int
}

// Change is what a save sets. A part that is not set (HasLists, HasPacks, HasDevices, HasInterests) stays as it is.
// Interests are only the app's: Modify leaves them as they are. Packs are set
// only with their lists, which are written without labels (workspace.Encode); lists set without packs are each put in
// a pack of their own.
type Change struct {
	Lists        []workspace.List
	Packs        []workspace.Pack
	Devices      []workspace.Device
	Interests    []string
	HasLists     bool
	HasPacks     bool
	HasDevices   bool
	HasInterests bool
}

// ReadRevision returns the owner's saved workspace with its revision, or nil when nothing has been saved. Unlike Read
// it does not make up the initial workspace: the app's /api/workspace answers revision 0 for an account without a row. The row
// is checked with Decode, so saved data that Go cannot read is an error, but it is answered as stored (the jsonb text,
// with the records of typle_records put back in their lists), not as Go decoded it. Data that Go reads and the app's decodeLists or decodeDevices refuses therefore reaches the
// browser, which says savedDataCorrupted, rather than being quietly changed here.
func (s *Store) ReadRevision(ctx context.Context, owner string) (*Saved, error) {
	saved, err := readSaved(ctx, s.pool, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := decodeSaved(owner, saved.Lists, saved.Devices, saved.Packs); err != nil {
		return nil, err
	}
	if _, err := workspace.DecodeInterests(saved.Interests); err != nil {
		return nil, fmt.Errorf("saved interests of %s: %w", owner, err)
	}
	return saved, nil
}

// WriteRevision saves change over the workspace at revision: revision 0 creates the row, any other revision replaces
// what is saved only if it is still that revision (else ErrConflict). Lists may be more than MaxLists in saved data
// merged from removed profiles (and followed packs more than MaxFollowedPacks); a save may keep or lower each number,
// never raise it. Lists of followed packs are not counted in MaxLists (workspace.OwnListCount). The size of the lists
// is not checked here: the /api/workspace handler (server/internal/web) only limits the request.
//
// It returns ErrConflict for a stale revision, a *workspace.Error for lists past the limits of validateLists or packs
// without lists (invalidWorkspace) or more lists or followed packs than allowed (listLimit, followLimit), and any other
// error when the change cannot be written. Whether packs hold their lists (workspace.CheckLibrary) is the handler's to
// check, on what was sent. A change that Decode would refuse is never written: the account would no longer load, here
// or in the app. All of this, apart from the conflict and the saved numbers of lists and followed packs, is checked
// before the database is touched.
//
// What is saved is not read back through Decode: only the saved revision and numbers are read, a part left out
// is kept by the UPDATE itself, and the result is the row's JSON as stored, with its records put back in its lists.
// Lists saved bring their records: those of typle_records become exactly theirs (see records.go), and the row's
// records_split is set.
func (s *Store) WriteRevision(ctx context.Context, owner string, revision int, change Change) (*Saved, error) {
	parts, err := encodeChange(owner, &change)
	if err != nil {
		return nil, err
	}
	var saved *Saved
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := writeRevision(ctx, tx, owner, revision, change, parts); err != nil {
			return err
		}
		// Lists saved bring their records: the owner's rows become exactly theirs.
		if change.HasLists {
			if err := syncRecords(ctx, tx, owner, nil, parts.rows); err != nil {
				return err
			}
		}
		var err error
		saved, err = readSaved(ctx, tx, owner)
		return err
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}

// writeRevision writes the parts of WriteRevision, as encodeChange encoded them, in tx. Lists written have their
// records in typle_records (records_split), which the caller then writes.
func writeRevision(ctx context.Context, tx pgx.Tx, owner string, revision int, change Change, parts *encodedChange) error {
	lists, packs, devices, interests := parts.lists, parts.packs, parts.devices, parts.interests
	own, followed := workspace.OwnListCount(change.Packs, change.Lists), workspace.FollowedPackCount(change.Packs)
	if revision == 0 {
		if change.HasLists && own > workspace.MaxLists {
			return &workspace.Error{Key: "listLimit"}
		}
		if change.HasLists && followed > workspace.MaxFollowedPacks {
			return &workspace.Error{Key: "followLimit"}
		}
		return insertRevision(ctx, tx, owner, parts)
	}
	// The saved lists and followed packs are counted as OwnListCount and FollowedPackCount count them.
	var savedRevision, savedOwn, savedFollowed int
	err := tx.QueryRow(ctx, `WITH followed AS (
			SELECT p->>'id' AS id FROM typle_workspaces, jsonb_array_elements(COALESCE(packs, '[]'::jsonb)) AS p
			WHERE owner_id = $1 AND p->'origin'->>'followed' = 'true')
		SELECT revision,
			(SELECT count(*) FROM jsonb_array_elements(lists) AS l
				WHERE l->>'packId' IS NULL OR l->>'packId' NOT IN (SELECT id FROM followed)),
			(SELECT count(*) FROM followed)
		FROM typle_workspaces WHERE owner_id = $1`, owner).
		Scan(&savedRevision, &savedOwn, &savedFollowed)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && savedRevision != revision) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if change.HasLists && own > workspace.MaxLists && own > savedOwn {
		return &workspace.Error{Key: "listLimit"}
	}
	if change.HasLists && followed > workspace.MaxFollowedPacks && followed > savedFollowed {
		return &workspace.Error{Key: "followLimit"}
	}
	// Matching the revision in the UPDATE itself makes the comparison and the write one atomic operation.
	err = tx.QueryRow(ctx, `UPDATE typle_workspaces SET lists = COALESCE($3::jsonb, lists),
		packs = COALESCE($4::jsonb, packs), devices = COALESCE($5::jsonb, devices), interests = COALESCE($6::jsonb, interests),
		records_split = records_split OR $3::jsonb IS NOT NULL, revision = revision + 1, updated_at = now()
		WHERE owner_id = $1 AND revision = $2 RETURNING revision`,
		owner, revision, nullable(lists), nullable(packs), nullable(devices), nullable(interests)).Scan(new(int))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	return err
}

// insertRevision writes the first save of the owner, revision 0: ErrConflict when another save made the row first.
func insertRevision(ctx context.Context, tx pgx.Tx, owner string, parts *encodedChange) error {
	// Two first saves of the same owner wait here for each other.
	if err := lockOwner(ctx, tx, owner); err != nil {
		return err
	}
	// Unique owner makes the existence check and the insert one atomic operation.
	err := tx.QueryRow(ctx, `INSERT INTO typle_workspaces (owner_id, lists, packs, devices, interests, records_split)
		VALUES ($1, $2::jsonb, $3::jsonb, $4::jsonb, $5::jsonb, true) ON CONFLICT (owner_id) DO NOTHING RETURNING revision`,
		owner, string(orEmpty(parts.lists)), nullable(parts.packs), string(orEmpty(parts.devices)), nullable(parts.interests)).Scan(new(int))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	return forgetRecords(ctx, tx, owner)
}

// readSaved reads the owner's row as stored, with its records put back in its lists (see withRecords), in one
// statement. A no-rows error is passed on as it is.
func readSaved(ctx context.Context, q querier, owner string) (*Saved, error) {
	var lists, packs, devices, interests, records []byte
	var revision int
	err := q.QueryRow(ctx, `SELECT lists::text, packs::text, devices::text, interests::text, revision, `+recordsColumn+`
		FROM typle_workspaces WHERE owner_id = $1`, owner).Scan(&lists, &packs, &devices, &interests, &revision, &records)
	if err != nil {
		return nil, err
	}
	if records != nil {
		stored, err := decodeColumn(records)
		if err != nil {
			return nil, fmt.Errorf("records of %s: %w", owner, err)
		}
		if lists, err = withRecords(lists, stored); err != nil {
			return nil, fmt.Errorf("saved workspace of %s: %w", owner, err)
		}
	}
	// Interests not saved (NULL) are read as nil.
	return &Saved{Lists: lists, Packs: packs, Devices: devices, Interests: interests, Revision: revision}, nil
}
