package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sparklingstadt/typle-r/server/internal/session"
	"github.com/sparklingstadt/typle-r/server/internal/store"
	"github.com/sparklingstadt/typle-r/server/internal/workspace"
)

// fakeShares stands in for the links of the database: what reaches the store and how the handlers answer what comes
// back.
type fakeShares struct {
	fakeStore
	shares []store.Share
	put    *store.Share
	putErr error
	calls  []string
	owner  string
	packID string
	listed *bool
}

func (f *fakeShares) PutShare(_ context.Context, owner, packID string, listed *bool) (*store.Share, error) {
	f.calls = append(f.calls, "put")
	f.owner, f.packID, f.listed = owner, packID, listed
	return f.put, f.putErr
}

func (f *fakeShares) DeleteShare(_ context.Context, owner, packID string) error {
	f.calls = append(f.calls, "delete")
	f.owner, f.packID = owner, packID
	return f.err
}

func (f *fakeShares) ListShares(_ context.Context, owner string) ([]store.Share, error) {
	f.calls = append(f.calls, "list")
	f.owner = owner
	return f.shares, f.err
}

// shareRequest is a signed-in request to /api/shares from the app's own origin, with a JSON body when there is one,
// unless the test changes it.
func (h *harness) shareRequest(method, target, body, owner string, change ...func(*http.Request)) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, target, reader)
	if method != http.MethodGet && method != http.MethodHead {
		r.Header.Set("Origin", testBase)
	}
	if body != "" {
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

var sharedAt = time.Date(2026, 10, 26, 9, 30, 15, 123456789, time.UTC)

func TestSharesLoginRequired(t *testing.T) {
	fake := &fakeShares{}
	h := newHarness(t, fake, testBase)
	expectError(t, h.shareRequest(http.MethodGet, "/api/shares", "", ""), http.StatusUnauthorized, "loginRequired")
	expectError(t, h.shareRequest(http.MethodPut, "/api/shares", `{"packId":"p"}`, ""), http.StatusUnauthorized, "loginRequired")
	expectError(t, h.shareRequest(http.MethodDelete, "/api/shares?pack=p", "", ""), http.StatusUnauthorized, "loginRequired")
	if len(fake.calls) != 0 {
		t.Fatalf("calls = %v", fake.calls)
	}
}

// A cookie's request must come from the app's own origin, with a JSON body, as PUT /api/workspace; an app's token
// needs no Origin.
func TestSharesForbidden(t *testing.T) {
	fake := &fakeShares{put: &store.Share{ID: "i", PackID: "p", UpdatedAt: sharedAt}}
	h := newHarness(t, fake, testBase)
	noOrigin := func(r *http.Request) { r.Header.Del("Origin") }
	otherOrigin := func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }
	textPlain := func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }
	expectError(t, h.shareRequest(http.MethodPut, "/api/shares", `{"packId":"p"}`, testOwner, noOrigin), http.StatusForbidden, "forbiddenRequest")
	expectError(t, h.shareRequest(http.MethodPut, "/api/shares", `{"packId":"p"}`, testOwner, otherOrigin), http.StatusForbidden, "forbiddenRequest")
	expectError(t, h.shareRequest(http.MethodPut, "/api/shares", `{"packId":"p"}`, testOwner, textPlain), http.StatusForbidden, "forbiddenRequest")
	expectError(t, h.shareRequest(http.MethodDelete, "/api/shares?pack=p", "", testOwner, noOrigin), http.StatusForbidden, "forbiddenRequest")
	if len(fake.calls) != 0 {
		t.Fatalf("calls = %v", fake.calls)
	}

	token := func(r *http.Request) {
		r.Header.Del("Origin")
		r.Header.Set("Authorization", "Bearer "+h.keys.SealApp(session.New(testOwner, "", "", time.Now())))
	}
	if response := h.shareRequest(http.MethodPut, "/api/shares", `{"packId":"p"}`, "", token); response.Code != http.StatusOK {
		t.Fatalf("app token: %d %s", response.Code, response.Body)
	}
	if response := h.shareRequest(http.MethodDelete, "/api/shares?pack=p", "", "", token); response.Code != http.StatusNoContent {
		t.Fatalf("app token delete: %d %s", response.Code, response.Body)
	}
}

func TestSharesPutInvalid(t *testing.T) {
	fake := &fakeShares{put: &store.Share{ID: "i", PackID: "p", UpdatedAt: sharedAt}}
	h := newHarness(t, fake, testBase)
	for _, body := range []string{
		`[]`, `null`, `"p"`, `{}`, `{"packId":""}`, `{"packId":1}`, `{"packId":null}`, `{"packId":"p","more":1}`, `{`,
		`{"packId":"p","listed":"yes"}`, `{"packId":"p","listed":null}`, `{"packId":"p","listed":1}`,
		`{"packId":"p","listed":true,"more":1}`, `{"listed":true}`,
		`{"packId":"` + strings.Repeat("p", 5000) + `"}`,
	} {
		expectError(t, h.shareRequest(http.MethodPut, "/api/shares", body, testOwner), http.StatusBadRequest, "invalidWorkspace")
	}
	empty := func(r *http.Request) { r.Header.Set("Content-Type", "application/json") }
	expectError(t, h.shareRequest(http.MethodPut, "/api/shares", "", testOwner, empty), http.StatusBadRequest, "invalidWorkspace")
	expectError(t, h.shareRequest(http.MethodDelete, "/api/shares", "", testOwner), http.StatusBadRequest, "invalidWorkspace")
	expectError(t, h.shareRequest(http.MethodDelete, "/api/shares?pack=", "", testOwner), http.StatusBadRequest, "invalidWorkspace")
	if len(fake.calls) != 0 {
		t.Fatalf("calls = %v", fake.calls)
	}
}

func TestSharesPut(t *testing.T) {
	fake := &fakeShares{put: &store.Share{ID: "0b6f2c1e-3d4a-4f5b-8c9d-0e1f2a3b4c5d", PackID: "pk", Name: "Pack K", UpdatedAt: sharedAt}}
	h := newHarness(t, fake, testBase)
	response := h.shareRequest(http.MethodPut, "/api/shares", `{"packId":"pk"}`, testOwner)
	want := `{"id":"0b6f2c1e-3d4a-4f5b-8c9d-0e1f2a3b4c5d","packId":"pk","name":"Pack K","updatedAt":"2026-10-26T09:30:15.123Z","listed":false,"hidden":false}`
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != want {
		t.Fatalf("response = %d %s; want %s", response.Code, response.Body, want)
	}
	expectPrivate(t, response)
	if fake.owner != testOwner || fake.packID != "pk" || fake.listed != nil {
		t.Fatalf("store got %s %s %v", fake.owner, fake.packID, fake.listed)
	}
}

func TestSharesPutListed(t *testing.T) {
	fake := &fakeShares{put: &store.Share{ID: "i", PackID: "p", Name: "P", UpdatedAt: sharedAt, Listed: true}}
	h := newHarness(t, fake, testBase)
	for _, want := range []bool{true, false} {
		body := `{"packId":"p","listed":` + strconv.FormatBool(want) + `}`
		response := h.shareRequest(http.MethodPut, "/api/shares", body, testOwner)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"listed":true,"hidden":false`) {
			t.Fatalf("%s: %d %s", body, response.Code, response.Body)
		}
		if fake.listed == nil || *fake.listed != want {
			t.Fatalf("%s: store got %v", body, fake.listed)
		}
	}
	h.shareRequest(http.MethodPut, "/api/shares", `{"packId":"p"}`, testOwner)
	if fake.listed != nil {
		t.Fatalf("without listed: store got %v", *fake.listed)
	}
}

func TestSharesPutHidden(t *testing.T) {
	h := newHarness(t, &fakeShares{putErr: &workspace.Error{Key: "sharedPackHidden"}}, testBase)
	expectError(t, h.shareRequest(http.MethodPut, "/api/shares", `{"packId":"p","listed":true}`, testOwner), http.StatusConflict, "sharedPackHidden")
	if len(h.reports) != 0 {
		t.Fatalf("reports = %v", h.reports)
	}
}

func TestDeleteShareWrongOrigin(t *testing.T) {
	fake := &fakeShares{}
	h := newHarness(t, fake, testBase)
	other := func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }
	expectError(t, h.shareRequest(http.MethodDelete, "/api/shares?pack=p", "", testOwner, other), http.StatusForbidden, "forbiddenRequest")
	if len(fake.calls) != 0 {
		t.Fatalf("calls = %v", fake.calls)
	}
}

func TestSharesPutRefused(t *testing.T) {
	cases := []struct {
		err    error
		status int
		key    string
	}{
		{&workspace.Error{Key: "packNotFound"}, http.StatusNotFound, "packNotFound"},
		{&workspace.Error{Key: "invalidWorkspace"}, http.StatusBadRequest, "invalidWorkspace"},
		{errors.New("down"), http.StatusServiceUnavailable, "workspaceSaveFailed"},
	}
	for _, tc := range cases {
		h := newHarness(t, &fakeShares{putErr: tc.err}, testBase)
		expectError(t, h.shareRequest(http.MethodPut, "/api/shares", `{"packId":"pk"}`, testOwner), tc.status, tc.key)
		if want := tc.status == http.StatusServiceUnavailable; want != (len(h.reports) == 1 && h.reports[0] == "shares.write") {
			t.Fatalf("%v: reports = %v", tc.err, h.reports)
		}
	}
}

func TestSharesDelete(t *testing.T) {
	fake := &fakeShares{}
	h := newHarness(t, fake, testBase)
	response := h.shareRequest(http.MethodDelete, "/api/shares?pack=p%20k", "", testOwner)
	if response.Code != http.StatusNoContent || response.Body.Len() != 0 {
		t.Fatalf("response = %d %s", response.Code, response.Body)
	}
	if fake.owner != testOwner || fake.packID != "p k" {
		t.Fatalf("store got %s %q", fake.owner, fake.packID)
	}

	h = newHarness(t, &fakeShares{fakeStore: fakeStore{err: errors.New("down")}}, testBase)
	expectError(t, h.shareRequest(http.MethodDelete, "/api/shares?pack=p", "", testOwner), http.StatusServiceUnavailable, "workspaceSaveFailed")
	if len(h.reports) != 1 || h.reports[0] != "shares.write" {
		t.Fatalf("reports = %v", h.reports)
	}
}

func TestSharesList(t *testing.T) {
	fake := &fakeShares{}
	h := newHarness(t, fake, testBase)
	response := h.shareRequest(http.MethodGet, "/api/shares", "", testOwner)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != `{"shares":[]}` {
		t.Fatalf("empty: %d %s", response.Code, response.Body)
	}
	expectPrivate(t, response)

	fake.shares = []store.Share{
		{ID: "b", PackID: "two", Name: "Two", UpdatedAt: sharedAt, Listed: true, Hidden: true},
		{ID: "a", PackID: "one", Name: "One", UpdatedAt: sharedAt.Add(-time.Hour)},
	}
	response = h.shareRequest(http.MethodGet, "/api/shares", "", testOwner)
	want := `{"shares":[{"id":"b","packId":"two","name":"Two","updatedAt":"2026-10-26T09:30:15.123Z","listed":true,"hidden":true},` +
		`{"id":"a","packId":"one","name":"One","updatedAt":"2026-10-26T08:30:15.123Z","listed":false,"hidden":false}]}`
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != want || fake.owner != testOwner {
		t.Fatalf("response = %d %s (owner %s)", response.Code, response.Body, fake.owner)
	}

	h = newHarness(t, &fakeShares{fakeStore: fakeStore{err: errors.New("down")}}, testBase)
	expectError(t, h.shareRequest(http.MethodGet, "/api/shares", "", testOwner), http.StatusServiceUnavailable, "databaseUnavailable")
	if len(h.reports) != 1 || h.reports[0] != "shares.read" {
		t.Fatalf("reports = %v", h.reports)
	}
}

func TestSharesMethod(t *testing.T) {
	h := newHarness(t, &fakeShares{}, testBase)
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		response := h.shareRequest(method, "/api/shares", "", testOwner)
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET, HEAD, PUT, DELETE" {
			t.Fatalf("%s: response = %d %v", method, response.Code, response.Header())
		}
	}
}

const aSharedID = "0b6f2c1e-3d4a-4f5b-8c9d-0e1f2a3b4c5d"

// With a real database: a pack of the owner's is shared, listed and unshared (Next.js reads it: tests/apiShared.test.cjs). The
// owner starts empty, so the pack is made first.
func TestSharesRealDatabase(t *testing.T) {
	s, owner := realStore(t)
	h := newHarness(t, s, testBase)
	name := "動物"
	saved, err := s.Modify(context.Background(), owner, func(w *workspace.Workspace) (bool, error) {
		id := workspace.NewID()
		w.Packs = []workspace.Pack{{ID: "pack-" + id, Name: name, CreatedAt: "2026-10-21T00:00:00.000Z"}}
		w.Lists = []workspace.List{{ID: id, Name: name, PackID: "pack-" + id, Words: []workspace.Word{{Display: "cat", Input: "cat"}}}}
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	packID := saved.Packs[0].ID
	t.Cleanup(func() { _ = s.DeleteShare(context.Background(), owner, packID) })

	expectError(t, h.shareRequest(http.MethodPut, "/api/shares", `{"packId":"nope"}`, owner), http.StatusNotFound, "packNotFound")
	response := h.shareRequest(http.MethodPut, "/api/shares", `{"packId":"`+packID+`"}`, owner)
	if response.Code != http.StatusOK {
		t.Fatalf("put = %d %s", response.Code, response.Body)
	}
	var share struct{ ID, PackID, UpdatedAt string }
	if err := json.Unmarshal(response.Body.Bytes(), &share); err != nil || share.PackID != packID || share.ID == "" {
		t.Fatalf("put body %s: %v", response.Body, err)
	}
	if list := h.shareRequest(http.MethodGet, "/api/shares", "", owner); !strings.Contains(list.Body.String(), share.ID) {
		t.Fatalf("list = %s", list.Body)
	}
	if deleted := h.shareRequest(http.MethodDelete, "/api/shares?pack="+packID, "", owner); deleted.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", deleted.Code)
	}
	if list := h.shareRequest(http.MethodGet, "/api/shares", "", owner); strings.Contains(list.Body.String(), share.ID) {
		t.Fatalf("list after delete = %s", list.Body)
	}
}
