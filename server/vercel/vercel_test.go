package vercel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sparklingstadt/typle-r/server/internal/store"
	"github.com/sparklingstadt/typle-r/server/internal/web"
	"github.com/sparklingstadt/typle-r/server/internal/workspace"
)

// fakeStore answers every call with nothing saved; Ping fails with err.
type fakeStore struct{ err error }

func (f fakeStore) Ping(context.Context) error                               { return f.err }
func (fakeStore) ReadRevision(context.Context, string) (*store.Saved, error) { return nil, nil }
func (fakeStore) SessionsRevokedAt(context.Context, string) (time.Time, error) {
	return time.Time{}, nil
}
func (fakeStore) WriteRevision(context.Context, string, int, store.Change) (*store.Saved, error) {
	return nil, errors.New("not here")
}
func (fakeStore) Modify(context.Context, string, func(*workspace.Workspace) (bool, error)) (*workspace.Workspace, error) {
	return nil, errors.New("not here")
}

// backend is a Backend over env, whose database is a fakeStore; opened counts the pools made.
func backend(gated bool, env map[string]string, opened *int) *Backend {
	b := New(gated)
	b.getenv = func(name string) string { return env[name] }
	b.open = func(context.Context, string) (web.Store, error) {
		*opened++
		return fakeStore{}, nil
	}
	return b
}

func serve(b http.Handler, method, target string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	b.ServeHTTP(response, httptest.NewRequest(method, target, nil))
	return response
}

var compareEnv = map[string]string{"TYPLE_BACKEND_COMPARE": "1", "DATABASE_URL": "postgres://db", "AUTH_SECRET": "s",
	"AUTH_URL": "http://127.0.0.1:3101"}

// Without TYPLE_BACKEND_COMPARE=1 every request is a 404, as in production, and nothing is opened.
func TestGate(t *testing.T) {
	for _, value := range []string{"", "0", "true", " 1"} {
		env := map[string]string{"DATABASE_URL": "postgres://db", "AUTH_SECRET": "s", "AUTH_URL": "http://127.0.0.1:3101",
			"TYPLE_BACKEND_COMPARE": value}
		opened := 0
		b := backend(true, env, &opened)
		for _, target := range []string{"/api/health", "/api/go/health", "/api/go?path=workspace", "/api/workspace/records"} {
			response := serve(b, http.MethodGet, target)
			if response.Code != http.StatusNotFound || response.Body.String() != "{\"error\":\"notFound\"}\n" ||
				response.Header().Get(BackendHeader) != "go" {
				t.Errorf("%q %s = %d %q %v", value, target, response.Code, response.Body, response.Header())
			}
		}
		if opened != 0 {
			t.Errorf("%q: the database was opened", value)
		}
	}
	// TYPLE_BACKEND=go opens the gate without the comparison; other values do not.
	for value, want := range map[string]int{"go": http.StatusOK, "next": http.StatusNotFound, "Go": http.StatusNotFound, "": http.StatusNotFound} {
		opened := 0
		env := map[string]string{"DATABASE_URL": "postgres://db", "TYPLE_BACKEND": value}
		if response := serve(backend(true, env, &opened), http.MethodGet, "/api/go?path=health"); response.Code != want {
			t.Errorf("TYPLE_BACKEND=%q health = %d; want %d", value, response.Code, want)
		}
	}
	// The typle-web command is not gated.
	opened := 0
	if response := serve(backend(false, map[string]string{"DATABASE_URL": "postgres://db"}, &opened), http.MethodGet, "/api/health"); response.Code != http.StatusOK {
		t.Fatalf("ungated health = %d", response.Code)
	}
}

// Each way a rewrite may hand over the path reaches the same route, and every answer says it is Go's.
func TestRoutedPath(t *testing.T) {
	opened := 0
	b := backend(true, compareEnv, &opened)
	for target, want := range map[string]int{
		"/api/health":                        http.StatusOK,
		"/api/go/health":                     http.StatusOK,
		"/api/go?path=health":                http.StatusOK,
		"/api/workspace":                     http.StatusUnauthorized,
		"/api/go/workspace":                  http.StatusUnauthorized,
		"/api/go?path=workspace":             http.StatusUnauthorized,
		"/api/go/workspace/records":          http.StatusMethodNotAllowed,
		"/api/go?path=workspace%2Frecords":   http.StatusMethodNotAllowed,
		"/api/go?path=workspace/records&x=1": http.StatusMethodNotAllowed,
		"/api/go":                            http.StatusNotFound,
		"/api/go/nothing":                    http.StatusNotFound,
		"/api/index":                         http.StatusNotFound,
	} {
		response := serve(b, http.MethodGet, target)
		if response.Code != want || response.Header().Get(BackendHeader) != "go" {
			t.Errorf("%s = %d %v; want %d", target, response.Code, response.Header(), want)
		}
	}
	if opened != 1 {
		t.Fatalf("the pool was made %d times", opened)
	}
}

// Missing settings are answered as the Next.js routes answer them, and looked at again on the next request.
func TestUnavailable(t *testing.T) {
	env := map[string]string{"TYPLE_BACKEND_COMPARE": "1"}
	opened := 0
	b := backend(true, env, &opened)
	expect := func(target, key string) {
		t.Helper()
		response := serve(b, http.MethodGet, target)
		if response.Code != http.StatusServiceUnavailable || strings.TrimSpace(response.Body.String()) != `{"error":"`+key+`"}` {
			t.Fatalf("%s = %d %s; want %s", target, response.Code, response.Body, key)
		}
	}
	expect("/api/workspace", "authUnavailable")
	expect("/api/health", "databaseUnavailable")
	env["AUTH_SECRET"], env["AUTH_URL"] = "s", "ftp://host"
	expect("/api/workspace", "authUnavailable")
	env["AUTH_URL"] = "http://127.0.0.1:3101"
	expect("/api/workspace", "databaseUnavailable")
	env["DATABASE_URL"] = "postgres://db"
	if response := serve(b, http.MethodGet, "/api/workspace"); response.Code != http.StatusUnauthorized {
		t.Fatalf("ready = %d %s", response.Code, response.Body)
	}
	if b.ready.Load() == nil || opened != 1 {
		t.Fatalf("kept %v, opened %d", b.ready.Load() != nil, opened)
	}
}
