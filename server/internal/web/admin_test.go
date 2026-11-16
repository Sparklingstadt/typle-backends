package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sparklingstadt/typle-r/server/internal/auth"
	"github.com/sparklingstadt/typle-r/server/internal/session"
)

// adminStore is a database with typle_admins: allowed answers Admin, and it remembers what it was asked.
type adminStore struct {
	fakeStore
	allowed  bool
	checkErr error
	checks   int
	email    string
	sub      string
}

func (a *adminStore) Admin(_ context.Context, email, sub string) (bool, error) {
	a.checks++
	a.email, a.sub = email, sub
	return a.allowed, a.checkErr
}

// newAdminHarness is newLoginHarness with the administrator's FFPF ID account and the store a.
func newAdminHarness(t *testing.T, base string, a *adminStore) *loginHarness {
	t.Helper()
	h := newLoginHarness(t, base)
	h.ffpf.user = auth.FFPFUser{Sub: "cadmin", Email: "Admin@Example.com", Name: "Admin"}
	parsed, _ := url.Parse(base)
	h.handler = New(Config{
		Store: a, Keys: h.keys, BaseURL: parsed, OAuth: h.oauth, GitHub: h.users, FFPF: h.ffpf,
		Now:    func() time.Time { return h.now },
		Report: func(_ *http.Request, err error, operation string) { h.reports = append(h.reports, operation) },
	})
	return h
}

// adminSignin goes through /api/auth/signin?admin=1 and returns the OAuth cookie it set.
func (h *loginHarness) adminSignin(t *testing.T, query string) *http.Cookie {
	t.Helper()
	response := h.do(http.MethodGet, "/api/auth/signin?admin=1"+query)
	cookie := findCookie(response, session.OAuthCookie)
	if response.Code != http.StatusFound || cookie == nil {
		t.Fatalf("signin = %d %v", response.Code, response.Header())
	}
	return &http.Cookie{Name: cookie.Name, Value: cookie.Value}
}

// onlyOAuthCookie fails unless the response set no cookie but the spent OAuth cookie.
func onlyOAuthCookie(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	for _, c := range response.Result().Cookies() {
		if c.Name != session.OAuthCookie {
			t.Fatalf("cookie set: %+v", c)
		}
	}
}

// A login to the dashboard always goes through FFPF ID, whatever provider says, and always returns to the dashboard;
// an app's client is not taken.
func TestAdminSignin(t *testing.T) {
	h := newAdminHarness(t, testBase, &adminStore{})
	for _, query := range []string{"", "&provider=github", "&provider=ffpf"} {
		h.ffpf.state = ""
		response := h.do(http.MethodGet, "/api/auth/signin?admin=1&returnTo=%2Fplay&client=ipad&code_challenge=x"+query)
		if location := response.Header().Get("Location"); response.Code != http.StatusFound ||
			location != "https://id.test/oauth/authorize?state="+url.QueryEscape(h.ffpf.state) {
			t.Fatalf("%q: signin = %d %q", query, response.Code, location)
		}
		opened, ok := h.keys.OpenOAuth(findCookie(response, session.OAuthCookie).Value, h.now)
		if !ok || !opened.Admin || opened.Provider != "ffpf" || opened.Nonce == "" || opened.ReturnTo != "/admin" || opened.Challenge != "" {
			t.Fatalf("%q: OAuth cookie = %+v, %v", query, opened, ok)
		}
	}
	if h.oauth.state != "" {
		t.Fatal("GitHub was asked")
	}
	plain, _ := h.keys.OpenOAuth(h.signin(t, "/play").Value, h.now)
	if plain.Admin {
		t.Fatal("a login without admin=1 is a login to the dashboard")
	}
}

// An administrator's account gets the administrator's cookie, and no session cookie.
func TestAdminCallback(t *testing.T) {
	for _, base := range []string{testBase, "https://typle.test"} {
		a := &adminStore{allowed: true}
		h := newAdminHarness(t, base, a)
		cookie := h.adminSignin(t, "")
		response := h.ffpfCallback("state="+url.QueryEscape(h.ffpf.state)+"&code=c", cookie)
		if response.Code != http.StatusFound || response.Header().Get("Location") != "/admin" {
			t.Fatalf("%s: callback = %d %v", base, response.Code, response.Header())
		}
		if findCookie(response, h.sessionCookieName()) != nil {
			t.Fatalf("%s: the dashboard's login set the session cookie", base)
		}
		admin := findCookie(response, session.AdminCookieName(h.secure))
		if admin == nil || !admin.HttpOnly || admin.SameSite != http.SameSiteStrictMode || admin.Path != "/" ||
			admin.Secure != h.secure || admin.MaxAge != int(session.AdminMaxAge.Seconds()) {
			t.Fatalf("%s: admin cookie = %+v", base, admin)
		}
		opened, ok := h.keys.OpenAdmin(admin.Value, h.now)
		if !ok || opened.Email != "admin@example.com" || opened.Sub != "cadmin" {
			t.Fatalf("%s: admin session = %+v, %v", base, opened, ok)
		}
		if a.checks != 1 || a.email != "admin@example.com" || a.sub != "cadmin" {
			t.Fatalf("%s: checked %d times, %q %q", base, a.checks, a.email, a.sub)
		}
	}
}

// An account typle_admins does not let in goes back to the dashboard refused, signed in to nothing; when typle_admins
// cannot be asked it goes back with adminUnavailable, and that is reported.
func TestAdminCallbackRefuses(t *testing.T) {
	a := &adminStore{}
	h := newAdminHarness(t, testBase, a)
	h.ffpf.user = auth.FFPFUser{Sub: "cother", Email: "someone@example.com", Name: "Admin"}
	cookie := h.adminSignin(t, "")
	response := h.ffpfCallback("state="+url.QueryEscape(h.ffpf.state)+"&code=c", cookie)
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/admin?error=adminForbidden" {
		t.Fatalf("callback = %d %v", response.Code, response.Header())
	}
	onlyOAuthCookie(t, response)
	if a.email != "someone@example.com" || a.sub != "cother" || len(h.reports) != 0 {
		t.Fatalf("checked %q %q, reports %v", a.email, a.sub, h.reports)
	}

	a.checkErr = errors.New("down")
	cookie = h.adminSignin(t, "")
	response = h.ffpfCallback("state="+url.QueryEscape(h.ffpf.state)+"&code=c", cookie)
	if response.Header().Get("Location") != "/admin?error=adminUnavailable" {
		t.Fatalf("callback with the database down = %d %v", response.Code, response.Header())
	}
	onlyOAuthCookie(t, response)
	if strings.Join(h.reports, ",") != "admin.check" {
		t.Fatalf("reports = %v", h.reports)
	}

	// A failed login returns to the dashboard too, without asking typle_admins.
	a.checks = 0
	cookie = h.adminSignin(t, "")
	response = h.ffpfCallback("state=wrong&code=c", cookie)
	if response.Header().Get("Location") != "/admin" || a.checks != 0 {
		t.Fatalf("failed callback = %d %v", response.Code, response.Header())
	}
	onlyOAuthCookie(t, response)
	h.ffpf.err = auth.ErrFFPFRejected
	cookie = h.adminSignin(t, "")
	response = h.ffpfCallback("state="+url.QueryEscape(h.ffpf.state)+"&code=c", cookie)
	if response.Header().Get("Location") != "/admin" || a.checks != 0 {
		t.Fatalf("refused callback = %d %v", response.Code, response.Header())
	}
	onlyOAuthCookie(t, response)
}

// Without a database to ask, nobody becomes the administrator: the login ends with adminUnavailable.
func TestAdminCallbackWithoutDatabase(t *testing.T) {
	h := newLoginHarness(t, testBase)
	h.ffpf.user = auth.FFPFUser{Sub: "cadmin", Email: "admin@example.com", Name: "Admin"}
	h.handler = New(Config{Keys: h.keys, BaseURL: mustURL(t, testBase), FFPF: h.ffpf, Now: func() time.Time { return h.now },
		Report: func(_ *http.Request, err error, operation string) { h.reports = append(h.reports, operation) }})
	cookie := h.adminSignin(t, "")
	response := h.ffpfCallback("state="+url.QueryEscape(h.ffpf.state)+"&code=c", cookie)
	if response.Header().Get("Location") != "/admin?error=adminUnavailable" {
		t.Fatalf("callback = %d %v", response.Code, response.Header())
	}
	onlyOAuthCookie(t, response)

	// Config.Admins is asked before Store.
	admins := &adminStore{allowed: true}
	h.handler = New(Config{Store: &adminStore{}, Admins: admins, Keys: h.keys, BaseURL: mustURL(t, testBase), FFPF: h.ffpf,
		Now: func() time.Time { return h.now }})
	cookie = h.adminSignin(t, "")
	response = h.ffpfCallback("state="+url.QueryEscape(h.ffpf.state)+"&code=c", cookie)
	if response.Header().Get("Location") != "/admin" || admins.checks != 1 || findCookie(response, session.AdminCookieName(false)) == nil {
		t.Fatalf("callback with Admins = %d %v", response.Code, response.Header())
	}
}

// GitHub's callback never makes an administrator: a login to the dashboard that comes back there is refused, without
// asking GitHub or typle_admins.
func TestAdminCallbackNotThroughGitHub(t *testing.T) {
	a := &adminStore{allowed: true}
	h := newAdminHarness(t, testBase, a)
	cookie := h.adminSignin(t, "")
	opened, _ := h.keys.OpenOAuth(cookie.Value, h.now)
	// Also one started before the dashboard's login went to FFPF ID: an admin cookie of a GitHub login.
	old := &http.Cookie{Name: session.OAuthCookie, Value: h.keys.SealOAuth(session.OAuth{
		State: "s", Verifier: opened.Verifier, ReturnTo: "/admin", Admin: true, Expires: h.now.Add(time.Minute)})}
	for _, c := range []struct {
		cookie *http.Cookie
		state  string
	}{{cookie, opened.State}, {old, "s"}} {
		response := h.callback("state="+url.QueryEscape(c.state)+"&code=c", c.cookie)
		if response.Code != http.StatusFound || response.Header().Get("Location") != "/admin?error=adminForbidden" {
			t.Fatalf("callback = %d %v", response.Code, response.Header())
		}
		onlyOAuthCookie(t, response)
	}
	if h.oauth.exchanges != 0 || h.users.calls != 0 || a.checks != 0 {
		t.Fatalf("exchanges = %d, users = %d, checks = %d", h.oauth.exchanges, h.users.calls, a.checks)
	}
}
