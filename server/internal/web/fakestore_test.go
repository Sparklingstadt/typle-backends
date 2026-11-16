package web

import (
	"context"
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

func (f fakeStore) Modify(context.Context, string, func(*workspace.Workspace) (bool, error)) (*workspace.Workspace, error) {
	return nil, f.err
}

func get(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(method, path, nil))
	return response
}

func TestNotFound(t *testing.T) {
	response := get(t, New(Config{Store: fakeStore{}}), http.MethodGet, "/api/nothing")
	if response.Code != http.StatusNotFound || strings.TrimSpace(response.Body.String()) != `{"error":"notFound"}` {
		t.Fatalf("response = %d %s", response.Code, response.Body)
	}
}

func (f fakeStore) AdminGallery(context.Context) ([]store.AdminGalleryPack, error) { return nil, f.err }

func (f fakeStore) SetHidden(context.Context, string, bool) (bool, error) { return false, f.err }

func (f fakeStore) Admin(context.Context, string, string) (bool, error) { return false, f.err }

func (f fakeStore) Overview(context.Context) (*store.AdminOverview, error) { return nil, f.err }

func (f fakeStore) Users(context.Context, int) (*store.AdminUsers, error) { return nil, f.err }

// No account has signed out of every device, whatever err is, so that a test of another failure fails where it means
// to; revocations_test.go has its own store for them.
func (f fakeStore) SessionsRevokedAt(context.Context, string) (time.Time, error) {
	return time.Time{}, nil
}

func (f fakeStore) RevokeSessions(context.Context, string) error { return f.err }
