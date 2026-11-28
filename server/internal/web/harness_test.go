package web

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sparklingstadt/typle-backends/server/internal/session"
	"github.com/sparklingstadt/typle-backends/server/internal/store"
	"github.com/sparklingstadt/typle-backends/server/internal/workspace"
)

// The harness shared by the tests of this package: a handler for a fake or real store, signed-in requests to it, and
// the bodies and answers they send and check.

const testBase = "http://localhost:3000"

var testOwner = "github:4242"

// fakeWorkspaces stands in for the database where a test is about the handler: what reaches the store and how the
// handler answers what comes back.
type fakeWorkspaces struct {
	fakeStore
	saved    *store.Saved
	reads    int
	writes   int
	revision int
	change   store.Change
}

func (f *fakeWorkspaces) ReadRevision(context.Context, string) (*store.Saved, error) {
	f.reads++
	return f.saved, f.err
}

func (f *fakeWorkspaces) WriteRevision(_ context.Context, _ string, revision int, change store.Change) (*store.Saved, error) {
	f.writes++
	f.revision, f.change = revision, change
	if f.err != nil {
		return nil, f.err
	}
	// Packs left out are not saved (null), as for a row saved without lists.
	var packs []byte
	if change.HasPacks {
		var err error
		if packs, err = workspace.EncodePacks(change.Packs); err != nil {
			return nil, err
		}
	}
	lists, err := workspace.Encode(change.Lists)
	if err != nil {
		return nil, err
	}
	devices, err := workspace.EncodeDevices(change.Devices)
	if err != nil {
		return nil, err
	}
	interests, err := workspace.EncodeInterests(change.Interests)
	if err != nil {
		return nil, err
	}
	return &store.Saved{Lists: lists, Packs: packs, Devices: devices, Interests: interests, Revision: revision + 1}, nil
}

// The database tests need the app's tables (npm run db:migrate in typle-r). They use a random account, signed in with a
// Better Auth session, and delete its rows.
func realStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	connection := testDatabase(t)
	s, err := store.OpenApp(context.Background(), connection)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, realAccount(t)
}

func testDatabase(t *testing.T) string {
	t.Helper()
	connection := os.Getenv("TYPLE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("set TYPLE_TEST_DATABASE_URL to a migrated test database")
	}
	return connection
}

// realAccount is a new random account with its Better Auth user and a session of a day, whose token is the one the
// harness's cookie names. Its rows are deleted when the test ends.
func realAccount(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	random := make([]byte, 8)
	_, _ = rand.Read(random)
	owner := fmt.Sprintf("github:%d", binary.BigEndian.Uint64(random)>>1)
	t.Cleanup(func() {
		_, _ = conn.Exec(ctx, `DELETE FROM typle_workspaces WHERE owner_id = $1`, owner)
		_, _ = conn.Exec(ctx, `DELETE FROM typle_records WHERE owner_id = $1`, owner)
		// The session row goes with its user (ON DELETE CASCADE).
		_, _ = conn.Exec(ctx, `DELETE FROM typle_auth_users WHERE id = $1`, owner)
		conn.Close(ctx)
	})
	if _, err := conn.Exec(ctx, `INSERT INTO typle_auth_users (id, name, email, email_verified) VALUES ($1, 'Test', $2, true)`,
		owner, owner+"@users.invalid"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO typle_auth_sessions (id, token, user_id, expires_at) VALUES ($1, $2, $1, $3)`,
		owner, tokenOf(owner), time.Now().Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	return owner
}

// testSecret is the BETTER_AUTH_SECRET of the tests' handlers.
const testSecret = "test secret"

// tokenOf is the session token the fake stores know as a session of owner (fakeStore.SessionUser).
func tokenOf(owner string) string { return "session-of:" + owner }

// sessionCookie is Better Auth's session cookie for a session of owner, signed with secret.
func sessionCookie(owner, secret string, secure bool) *http.Cookie {
	return &http.Cookie{Name: session.CookieName(secure), Value: session.Sign(tokenOf(owner), secret)}
}

type harness struct {
	handler http.Handler
	reports []string
}

func newHarness(t *testing.T, s Store, base string, change ...func(*Config)) *harness {
	t.Helper()
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{}
	config := Config{Store: s, Secret: testSecret, BaseURL: parsed, Report: func(r *http.Request, err error, operation string) {
		h.reports = append(h.reports, operation)
	}}
	for _, f := range change {
		f(&config)
	}
	h.handler = New(config)
	return h
}

func (h *harness) cookie(sub string, secure bool) *http.Cookie {
	return sessionCookie(sub, testSecret, secure)
}

// request is a signed-in PUT from the app's own origin unless the test changes it.
func (h *harness) request(method, body string, owner string, change ...func(*http.Request)) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, "/api/workspace", reader)
	if method == http.MethodPut {
		r.Header.Set("Origin", testBase)
		r.Header.Set("Content-Type", "application/json")
	}
	if owner != "" {
		r.AddCookie(h.cookie(owner, false))
	}
	for _, f := range change {
		f(r)
	}
	response := httptest.NewRecorder()
	h.handler.ServeHTTP(response, r)
	return response
}

func (h *harness) get(owner string, change ...func(*http.Request)) *httptest.ResponseRecorder {
	return h.request(http.MethodGet, "", owner, change...)
}

func (h *harness) put(owner, body string, change ...func(*http.Request)) *httptest.ResponseRecorder {
	return h.request(http.MethodPut, body, owner, change...)
}

// payload is a request body of the account's, with the fields the test is about.
func payload(owner, fields string) string {
	return `{"accountId":"` + owner + `",` + fields + `}`
}

func expectError(t *testing.T, response *httptest.ResponseRecorder, status int, key string) {
	t.Helper()
	if response.Code != status || strings.TrimSpace(response.Body.String()) != `{"error":"`+key+`"}` {
		t.Fatalf("response = %d %s; want %d %s", response.Code, response.Body, status, key)
	}
	expectPrivate(t, response)
}

func expectPrivate(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("Vary") != "Cookie" {
		t.Fatalf("headers = %v", response.Header())
	}
}

func decodeBody(t *testing.T, response *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("body %q: %v", response.Body, err)
	}
	return fields
}

var oneList = aList("one", "One")

// aList is a list that passes validateList, with this id and name.
func aList(id, name string) string {
	return `{"id":"` + id + `","name":"` + name + `","createdAt":"2026-09-06","words":[{"display":"a","input":"a","annotation":""}],"records":[]}`
}
