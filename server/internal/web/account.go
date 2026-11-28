package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/sparklingstadt/typle-backends/server/internal/session"
)

// Who a request is signed in as, as signedIn of src/server/account.ts tells it: the Better Auth session its cookie names.

// demoAccountID is the shared demo account's ID (src/server/demoLogin.ts). Its sessions stop working when its login is
// turned off (DEMO_LOGIN), not only new logins.
const demoAccountID = "demo:shared"

// cookieValue is the value of the first cookie named name in the Cookie header, as better-call's parseCookies reads it:
// the first pair with that name wins, and a value that starts with a double quote loses its first and last character.
// The value is not decoded here (session.Token does).
func cookieValue(r *http.Request, name string) (string, bool) {
	for _, part := range strings.Split(header(r, "Cookie"), ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(key) != name {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, `"`) {
			value = value[1:max(1, len(value)-1)]
		}
		return value, true
	}
	return "", false
}

var (
	// errNoSecret is a server set up without BETTER_AUTH_SECRET, with which Better Auth cannot start in typle-r either.
	errNoSecret = errors.New("BETTER_AUTH_SECRET is not set")
	// errNoSessions is a server set up without a database to read the sessions from.
	errNoSessions = errors.New("no database to read the session from")
)

// signedIn is signedIn of src/server/account.ts, for a request that reads or changes the account's data: loginRequired
// without a session cookie, with one whose signature does not check out, or with one of no session or an expired one;
// databaseUnavailable when the session cannot be read (also without the secret, as Better Auth then fails to start), as
// the data cannot be either. No cookie is removed: typle-r reads the session through auth.api.getSession, whose cookies
// do not reach the answer. The session row is always read (typle-r's cookie cache may answer for up to 5 minutes
// without it), so a session deleted by signing out of every device is refused at once. It answers the request unless it
// returns true.
func (s *server) signedIn(w http.ResponseWriter, r *http.Request) (session.Session, bool) {
	if s.config.Secret == "" {
		s.report(r, errNoSecret, "session.check")
		refuse(w, http.StatusServiceUnavailable, "databaseUnavailable")
		return session.Session{}, false
	}
	value, ok := cookieValue(r, session.CookieName(s.secure()))
	token, signed := session.Token(value, s.config.Secret)
	if !ok || !signed {
		refuse(w, http.StatusUnauthorized, "loginRequired")
		return session.Session{}, false
	}
	if s.config.Store == nil {
		s.report(r, errNoSessions, "session.check")
		refuse(w, http.StatusServiceUnavailable, "databaseUnavailable")
		return session.Session{}, false
	}
	user, found, err := s.config.Store.SessionUser(r.Context(), token, s.now())
	if err != nil {
		s.report(r, err, "session.check")
		refuse(w, http.StatusServiceUnavailable, "databaseUnavailable")
		return session.Session{}, false
	}
	if !found || (user.Sub == demoAccountID && !s.config.DemoLogin) {
		refuse(w, http.StatusUnauthorized, "loginRequired")
		return session.Session{}, false
	}
	return user, true
}
