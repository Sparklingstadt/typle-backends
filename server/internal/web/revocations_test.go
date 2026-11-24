package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/sparklingstadt/typle-backends/server/internal/session"
	"github.com/sparklingstadt/typle-backends/server/internal/store"
)

// revocations is a database that keeps when each account signed out of every device.
type revocations struct {
	fakeWorkspaces
	revokedAt map[string]time.Time
}

func (r *revocations) SessionsRevokedAt(_ context.Context, owner string) (time.Time, error) {
	return r.revokedAt[owner], nil
}

// revocationsDown is a database whose revocations cannot be read.
type revocationsDown struct{ Store }

func (revocationsDown) SessionsRevokedAt(context.Context, string) (time.Time, error) {
	return time.Time{}, errors.New("down")
}

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// cleared is the Set-Cookie of dropCookie (src/server/account.ts), or "" when the answer removes no cookie.
func cleared(response *httptest.ResponseRecorder) string {
	return response.Header().Get("Set-Cookie")
}

// A session of a login no later than the account's revocation is refused wherever the account's data is read or
// changed, and a browser's cookie is removed; a login since is let in.
func TestRevokedSessionIsRefused(t *testing.T) {
	revokedAt := time.Unix(1_800_000_000, 0)
	r := &revocations{revokedAt: map[string]time.Time{testOwner: revokedAt}}
	r.saved = &store.Saved{Lists: []byte("[]"), Devices: []byte("[]"), Revision: 1}
	h := newHarness(t, r, testBase)
	at := func(issued time.Time) func(*http.Request) {
		return func(req *http.Request) {
			req.Header.Del("Cookie")
			req.AddCookie(&http.Cookie{Name: "typle-session", Value: h.keys.Seal(session.New(testOwner, "A", "", issued))})
		}
	}
	for name, issued := range map[string]time.Time{"before": revokedAt.Add(-time.Hour), "the same second": revokedAt} {
		for _, response := range []*httptest.ResponseRecorder{
			h.get(testOwner, at(issued)),
			h.put(testOwner, payload(testOwner, `"revision":1,"devices":[]`), at(issued)),
			h.postRecord(testOwner, recordBody(testOwner, "one", aRun), at(issued)),
		} {
			expectError(t, response, http.StatusUnauthorized, "loginRequired")
			if got := cleared(response); got != "typle-session=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax" {
				t.Errorf("%s: Set-Cookie = %q", name, got)
			}
		}
	}
	if response := h.get(testOwner, at(revokedAt.Add(time.Second))); response.Code != http.StatusOK || cleared(response) != "" {
		t.Fatalf("a later login = %d %q", response.Code, cleared(response))
	}
	// Another account's revocation is not this one's.
	if response := h.get("github:7"); response.Code != http.StatusOK {
		t.Fatalf("another account = %d", response.Code)
	}
	// Over HTTPS the cookie removed is the __Host- one, and Secure.
	hs := newHarness(t, r, "https://typle.example")
	response := hs.get("", func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: "__Host-typle-session", Value: hs.keys.Seal(session.New(testOwner, "A", "", revokedAt))})
	})
	if response.Code != http.StatusUnauthorized || cleared(response) != "__Host-typle-session=; Path=/; Max-Age=0; HttpOnly; Secure; SameSite=Lax" {
		t.Fatalf("https = %d %q", response.Code, cleared(response))
	}
}

// An app's revoked token is refused too, without a cookie to remove.
func TestRevokedAppTokenIsRefused(t *testing.T) {
	revokedAt := time.Unix(1_800_000_000, 0)
	r := &revocations{revokedAt: map[string]time.Time{testOwner: revokedAt}}
	h := newHarness(t, r, testBase)
	token := h.keys.SealApp(session.New(testOwner, "A", "", revokedAt.Add(-time.Minute)))
	response := h.get("", func(req *http.Request) { req.Header.Set("Authorization", "Bearer "+token) })
	expectError(t, response, http.StatusUnauthorized, "loginRequired")
	if cleared(response) != "" {
		t.Fatalf("Set-Cookie = %q", cleared(response))
	}
}

// When the revocations cannot be read, the data is not served: the database is as good as down.
func TestRevocationsUnreadable(t *testing.T) {
	h := newHarness(t, revocationsDown{&fakeWorkspaces{}}, testBase)
	expectError(t, h.get(testOwner), http.StatusServiceUnavailable, "databaseUnavailable")
	if len(h.reports) != 1 || h.reports[0] != "session.check" {
		t.Fatalf("reports = %v", h.reports)
	}
}

// currentUser: a request with an Authorization header is judged by it alone, and only "Bearer <token>" is a token.
func TestBearerToken(t *testing.T) {
	h := newHarness(t, &fakeWorkspaces{}, testBase)
	token := h.keys.SealApp(session.New(testOwner, "A", "", time.Now()))
	for value, want := range map[string]int{
		"Bearer " + token:                http.StatusOK,
		"bearer " + token:                http.StatusUnauthorized,
		"Bearer  " + token:               http.StatusUnauthorized,
		"Bearer " + token + " x":         http.StatusUnauthorized,
		"Bearer ":                        http.StatusUnauthorized,
		h.cookie(testOwner, false).Value: http.StatusUnauthorized,
	} {
		// The cookie is valid: a token that is refused is never made up for by it.
		response := h.get(testOwner, func(req *http.Request) { req.Header.Set("Authorization", value) })
		if response.Code != want {
			t.Errorf("Authorization %q = %d; want %d", value, response.Code, want)
		}
	}
	// A cookie opened with the app's key, or a token with the cookie's, is no one.
	cookieAsToken := h.keys.Seal(session.New(testOwner, "A", "", time.Now()))
	if response := h.get("", func(req *http.Request) { req.Header.Set("Authorization", "Bearer "+cookieAsToken) }); response.Code != http.StatusUnauthorized {
		t.Fatalf("a cookie as a token = %d", response.Code)
	}
}

// cookieValue: the first cookie of the name, its quotes dropped.
func TestCookieValue(t *testing.T) {
	h := newHarness(t, &fakeWorkspaces{}, testBase)
	good := h.cookie(testOwner, false).Value
	for header, want := range map[string]int{
		"typle-session=" + good:                    http.StatusOK,
		"a=b; typle-session=\"" + good + "\"":      http.StatusOK,
		" typle-session = " + good + " ;x=y":       http.StatusOK,
		"typle-session=bad; typle-session=" + good: http.StatusUnauthorized,
		"__Host-typle-session=" + good:             http.StatusUnauthorized,
		"":                                         http.StatusUnauthorized,
	} {
		response := h.get("", func(req *http.Request) { req.Header.Set("Cookie", header) })
		if response.Code != want {
			t.Errorf("Cookie %q = %d; want %d", header, response.Code, want)
		}
	}
}
