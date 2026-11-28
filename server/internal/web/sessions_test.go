package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/sparklingstadt/typle-backends/server/internal/session"
	"github.com/sparklingstadt/typle-backends/server/internal/store"
)

// sessionsDown is a database whose sessions cannot be read.
type sessionsDown struct{ Store }

func (sessionsDown) SessionUser(context.Context, string, time.Time) (session.Session, bool, error) {
	return session.Session{}, false, errors.New("down")
}

func savedEmpty() *fakeWorkspaces {
	return &fakeWorkspaces{saved: &store.Saved{Lists: []byte("[]"), Devices: []byte("[]"), Revision: 1}}
}

// A Better Auth session opens the account's data wherever it is read or changed, and no answer sets a cookie (typle-r
// reads the session through auth.api.getSession, whose cookies never reach the answer).
func TestSessionOpens(t *testing.T) {
	h := newHarness(t, savedEmpty(), testBase)
	for name, response := range map[string]int{
		"get": h.get(testOwner).Code,
		"put": h.put(testOwner, payload(testOwner, `"revision":1,"devices":[]`)).Code,
	} {
		if response != http.StatusOK {
			t.Errorf("%s = %d", name, response)
		}
	}
	refused := h.get("")
	expectError(t, refused, http.StatusUnauthorized, "loginRequired")
	if cookie := refused.Header().Get("Set-Cookie"); cookie != "" {
		t.Fatalf("Set-Cookie = %q", cookie)
	}
	// Over HTTPS the cookie is the __Secure- one, and the plain one is no one.
	hs := newHarness(t, savedEmpty(), "https://typle.example")
	if response := hs.get("", func(r *http.Request) { r.AddCookie(hs.cookie(testOwner, true)) }); response.Code != http.StatusOK {
		t.Fatalf("https = %d %s", response.Code, response.Body)
	}
	if response := hs.get("", func(r *http.Request) { r.AddCookie(hs.cookie(testOwner, false)) }); response.Code != http.StatusUnauthorized {
		t.Fatalf("https with the plain cookie = %d", response.Code)
	}
}

// The iPad and Mac apps' bearer tokens are gone from typle-r: an Authorization header neither opens the data nor stands
// in for the cookie's Origin.
func TestAuthorizationIsIgnored(t *testing.T) {
	h := newHarness(t, savedEmpty(), testBase)
	bearer := func(r *http.Request) { r.Header.Set("Authorization", "Bearer x") }
	expectError(t, h.get("", bearer), http.StatusUnauthorized, "loginRequired")
	if response := h.get(testOwner, bearer); response.Code != http.StatusOK {
		t.Fatalf("cookie with a bearer token = %d", response.Code)
	}
	noOrigin := func(r *http.Request) { r.Header.Del("Origin"); bearer(r) }
	expectError(t, h.put(testOwner, payload(testOwner, `"revision":1,"devices":[]`), noOrigin), http.StatusForbidden, "forbiddenRequest")
}

// When the sessions cannot be read, the data is not served: the database is as good as down.
func TestSessionsUnreadable(t *testing.T) {
	h := newHarness(t, sessionsDown{&fakeWorkspaces{}}, testBase)
	expectError(t, h.get(testOwner), http.StatusServiceUnavailable, "databaseUnavailable")
	if len(h.reports) != 1 || h.reports[0] != "session.check" {
		t.Fatalf("reports = %v", h.reports)
	}
}

// Without BETTER_AUTH_SECRET Better Auth does not start in typle-r, and reading the session throws: every request that
// needs the login is a 503 databaseUnavailable, with a cookie or without.
func TestNoSecret(t *testing.T) {
	h := newHarness(t, savedEmpty(), testBase, func(c *Config) { c.Secret = "" })
	expectError(t, h.get(testOwner), http.StatusServiceUnavailable, "databaseUnavailable")
	expectError(t, h.get(""), http.StatusServiceUnavailable, "databaseUnavailable")
	if len(h.reports) != 2 || h.reports[0] != "session.check" {
		t.Fatalf("reports = %v", h.reports)
	}
}

// The shared demo account's sessions work only while its login is on (DEMO_LOGIN=1).
func TestDemoAccount(t *testing.T) {
	off := newHarness(t, savedEmpty(), testBase)
	expectError(t, off.get(demoAccountID), http.StatusUnauthorized, "loginRequired")
	on := newHarness(t, savedEmpty(), testBase, func(c *Config) { c.DemoLogin = true })
	if response := on.get(demoAccountID); response.Code != http.StatusOK {
		t.Fatalf("demo with DEMO_LOGIN = %d", response.Code)
	}
}

// cookieValue: the first cookie of the name, a leading quote dropped with the last character.
func TestCookieValue(t *testing.T) {
	h := newHarness(t, savedEmpty(), testBase)
	name := session.CookieName(false)
	good := h.cookie(testOwner, false).Value
	for header, want := range map[string]int{
		name + "=" + good:                    http.StatusOK,
		"a=b; " + name + "=\"" + good + "\"": http.StatusOK,
		" " + name + " = " + good + " ;x=y":  http.StatusOK,
		name + "=bad; " + name + "=" + good:  http.StatusUnauthorized,
		"__Secure-" + name + "=" + good:      http.StatusUnauthorized,
		"better-auth.session_data=" + good:   http.StatusUnauthorized,
		"":                                   http.StatusUnauthorized,
	} {
		response := h.get("", func(req *http.Request) { req.Header.Set("Cookie", header) })
		if response.Code != want {
			t.Errorf("Cookie %q = %d; want %d", header, response.Code, want)
		}
	}
}

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
