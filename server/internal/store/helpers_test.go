package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sparklingstadt/typle-r/server/internal/workspace"
)

// Helpers shared by the tests of this package: the database they run against, accounts made for them, and the
// workspace as saved.

// read returns the owner's saved workspace, such as github:12345's. An account that has saved nothing yet has the
// initial workspace, as in the app: empty since v0.98.0. Only the tests read a workspace whole like this; the server
// reads it as it changes it (Modify) or as the app asks for it (ReadRevision).
func (s *Store) read(ctx context.Context, owner string) (*workspace.Workspace, error) {
	saved, err := readSaved(ctx, s.pool, owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return workspace.Initial()
	}
	if err != nil {
		return nil, err
	}
	return decodeSaved(owner, saved.Lists, saved.Devices, saved.Packs)
}

// The database tests need the app's tables (npm run db:migrate). They add rows under random owners and delete them.
func database(t *testing.T) (string, *pgx.Conn) {
	t.Helper()
	connection := os.Getenv("TYPLE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("set TYPLE_TEST_DATABASE_URL to a migrated test database")
	}
	conn, err := pgx.Connect(context.Background(), connection)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return connection, conn
}

func owner(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	random := make([]byte, 8)
	_, _ = rand.Read(random)
	id := "go-test:" + hex.EncodeToString(random)
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `DELETE FROM typle_workspaces WHERE owner_id = $1`, id)
		_, _ = conn.Exec(context.Background(), `DELETE FROM typle_records WHERE owner_id = $1`, id)
	})
	return id
}

func open(t *testing.T, connection string) *Store {
	t.Helper()
	s, err := OpenApp(context.Background(), connection)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func row(t *testing.T, conn *pgx.Conn, id string) (lists string, revision int, updated time.Time) {
	t.Helper()
	if err := conn.QueryRow(context.Background(), `SELECT lists::text, revision, updated_at FROM typle_workspaces WHERE owner_id = $1`, id).
		Scan(&lists, &revision, &updated); err != nil {
		t.Fatal(err)
	}
	return lists, revision, updated
}

func sameJSON(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("got %s, want %s", got, want)
	}
}
