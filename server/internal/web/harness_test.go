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
	"github.com/sparklingstadt/typle-r/server/internal/session"
	"github.com/sparklingstadt/typle-r/server/internal/store"
	"github.com/sparklingstadt/typle-r/server/internal/workspace"
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

// The database tests need the app's tables (npm run db:migrate). They use a random account and delete its row.
func realStore(t *testing.T) (*store.Store, string) {
	t.Helper()
	connection := os.Getenv("TYPLE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("set TYPLE_TEST_DATABASE_URL to a migrated test database")
	}
	ctx := context.Background()
	s, err := store.OpenApp(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	conn, err := pgx.Connect(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	random := make([]byte, 8)
	_, _ = rand.Read(random)
	owner := fmt.Sprintf("github:%d", binary.BigEndian.Uint64(random)>>1)
	t.Cleanup(func() {
		_, _ = conn.Exec(ctx, `DELETE FROM typle_workspaces WHERE owner_id = $1`, owner)
		_, _ = conn.Exec(ctx, `DELETE FROM typle_records WHERE owner_id = $1`, owner)
		conn.Close(ctx)
	})
	return s, owner
}

type harness struct {
	handler http.Handler
	keys    session.Keys
	reports []string
}

func newHarness(t *testing.T, s Store, base string) *harness {
	t.Helper()
	keys, err := session.NewKeys("test secret")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{keys: keys}
	h.handler = New(Config{Store: s, Keys: keys, BaseURL: parsed, Report: func(r *http.Request, err error, operation string) {
		h.reports = append(h.reports, operation)
	}})
	return h
}

func (h *harness) cookie(sub string, secure bool) *http.Cookie {
	return &http.Cookie{Name: session.CookieName(secure), Value: h.keys.Seal(session.New(sub, "", "", time.Now()))}
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
