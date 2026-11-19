package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sparklingstadt/typle-r/server/internal/store"
	"github.com/sparklingstadt/typle-r/server/internal/workspace"
)

// fakeStore is a database whose every call returns err (nil: it answers, with nothing in it).
type fakeStore struct{ err error }

func (f fakeStore) Ping(context.Context) error { return f.err }

func (f fakeStore) ReadRevision(context.Context, string) (*store.Saved, error) { return nil, f.err }

func (f fakeStore) WriteRevision(context.Context, string, int, store.Change) (*store.Saved, error) {
	return nil, f.err
}

func (f fakeStore) Modify(context.Context, string, func(*workspace.Workspace) (bool, error)) (*workspace.Workspace, error) {
	return nil, f.err
}

// No account has signed out of every device, whatever err is, so that a test of another failure fails where it means
// to; revocations_test.go has its own store for them.
func (f fakeStore) SessionsRevokedAt(context.Context, string) (time.Time, error) {
	return time.Time{}, nil
}

func get(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(method, path, nil))
	return response
}

// /api/health needs no login settings, as checkHealth does not.
func TestHealthOK(t *testing.T) {
	response := get(t, New(Config{Store: fakeStore{}}), http.MethodGet, "/api/health")
	if response.Code != http.StatusOK || response.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("response = %d %q", response.Code, response.Body)
	}
	if response.Header().Get("Cache-Control") != "private, no-store" || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("headers = %v", response.Header())
	}
	checkSecurityHeaders(t, "health", response.Header())
}

// A recorder keeps the body of a HEAD response that a server would drop, so this goes through a real one.
func TestHealthHead(t *testing.T) {
	server := httptest.NewServer(New(Config{Store: fakeStore{}}))
	defer server.Close()
	response, err := server.Client().Head(server.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || len(body) != 0 || response.Header.Get("Cache-Control") != "private, no-store" ||
		response.Header.Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("response = %d %q %v", response.StatusCode, body, response.Header)
	}
}

func TestHealthDatabaseDown(t *testing.T) {
	for name, handler := range map[string]http.Handler{
		"refused": New(Config{Store: fakeStore{err: errors.New("connection refused")}}),
		"no URL":  New(Config{}),
	} {
		response := get(t, handler, http.MethodGet, "/api/health")
		if response.Code != http.StatusServiceUnavailable || strings.TrimSpace(response.Body.String()) != `{"error":"databaseUnavailable"}` {
			t.Fatalf("%s: response = %d %s", name, response.Code, response.Body)
		}
	}
}

func TestNotFound(t *testing.T) {
	for _, path := range []string{"/api/nothing", "/api/workspace/", "/api/auth/session", "/api/shares", "/"} {
		response := get(t, New(Config{Store: fakeStore{}}), http.MethodGet, path)
		if response.Code != http.StatusNotFound || response.Body.String() != "{\"error\":\"notFound\"}\n" {
			t.Fatalf("%s: response = %d %s", path, response.Code, response.Body)
		}
	}
	response := get(t, NotFound(), http.MethodPut, "/api/workspace")
	if response.Code != http.StatusNotFound || response.Body.String() != "{\"error\":\"notFound\"}\n" {
		t.Fatalf("NotFound: response = %d %s", response.Code, response.Body)
	}
	checkSecurityHeaders(t, "NotFound", response.Header())
}
