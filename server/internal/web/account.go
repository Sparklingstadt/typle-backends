package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/sparklingstadt/typle-r/server/internal/session"
)

// Who a request is signed in as, and whether that login still counts, as src/server/account.ts tells it.

// fromApp reports whether the request is an app's (iPad, Mac): it carries an Authorization header that is not empty,
// which is then all it is judged by.
func fromApp(r *http.Request) bool {
	return header(r, "Authorization") != ""
}

// cookieValue is cookieValue of src/server/account.ts: the value of the first cookie named name in the Cookie header,
// with quotes around it dropped.
func cookieValue(r *http.Request, name string) (string, bool) {
	for _, part := range strings.Split(header(r, "Cookie"), ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(key) != name {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
			value = value[1 : len(value)-1]
		}
		return value, true
	}
	return "", false
}

// owner is currentUser: from an app's bearer token when the request has an Authorization header, and otherwise from
// the login cookie. A request with both is judged by the token, so a cookie never stands in for a token that is refused.
func (s *server) owner(r *http.Request) (session.Session, bool) {
	if authorization := header(r, "Authorization"); authorization != "" {
		token, _ := strings.CutPrefix(authorization, "Bearer ")
		if token == authorization || token == "" || strings.ContainsAny(token, " \t") {
			return session.Session{}, false
		}
		return s.config.Keys.OpenApp(token, s.now())
	}
	value, ok := cookieValue(r, session.CookieName(s.secure()))
	if !ok {
		return session.Session{}, false
	}
	return s.config.Keys.Open(value, s.now())
}

// errNoSessions is a server set up without a database to keep the revocations in.
var errNoSessions = errors.New("no database to check the session against")

// signedIn is signedIn of src/server/account.ts, for a request that reads or changes the account's data: loginRequired
// without a session or with a revoked one (whose cookie is then removed, unless the request is an app's), and
// databaseUnavailable when the revocations cannot be read, as the data cannot be either. It answers the request unless
// it returns true.
func (s *server) signedIn(w http.ResponseWriter, r *http.Request) (session.Session, bool) {
	user, ok := s.owner(r)
	if !ok {
		refuse(w, http.StatusUnauthorized, "loginRequired")
		return session.Session{}, false
	}
	if s.config.Store == nil {
		s.report(r, errNoSessions, "session.check")
		refuse(w, http.StatusServiceUnavailable, "databaseUnavailable")
		return session.Session{}, false
	}
	revokedAt, err := s.config.Store.SessionsRevokedAt(r.Context(), user.Sub)
	if err != nil {
		s.report(r, err, "session.check")
		refuse(w, http.StatusServiceUnavailable, "databaseUnavailable")
		return session.Session{}, false
	}
	if user.RevokedBy(revokedAt) {
		if !fromApp(r) {
			s.dropCookie(w)
		}
		refuse(w, http.StatusUnauthorized, "loginRequired")
		return session.Session{}, false
	}
	return user, true
}

// dropCookie removes the login cookie as dropCookie of src/server/account.ts writes it.
func (s *server) dropCookie(w http.ResponseWriter) {
	secure := ""
	if s.secure() {
		secure = "; Secure"
	}
	w.Header().Add("Set-Cookie", session.CookieName(s.secure())+"=; Path=/; Max-Age=0; HttpOnly"+secure+"; SameSite=Lax")
}
