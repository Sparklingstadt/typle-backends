package web

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sparklingstadt/typle-r/server/internal/session"
)

// The signed-in session: who is signed in (/api/auth/session), the app token (/api/auth/token), signing out and the
// local development login.

// tokenAnswer is the app's token and who it signs in.
type tokenAnswer struct {
	Token string       `json:"token"`
	User  *sessionUser `json:"user"`
}

func userOf(s session.Session) *sessionUser {
	body := &sessionUser{ID: s.Sub, Name: s.Name}
	if s.Image != "" {
		body.Image = &s.Image
	}
	return body
}

// token trades the code a login of an app (iPad, Mac, Android or Windows) ended with, and the verifier of its
// challenge, for the app's token. It takes no cookie and sets none, so it needs no Origin: only the app that started
// the login knows the verifier. Any mistake is loginFailed, and the app starts the login again.
func (s *server) token(w http.ResponseWriter, r *http.Request) {
	if !methods(w, r, http.MethodPost) {
		return
	}
	var body struct {
		Code     string `json:"code"`
		Verifier string `json:"verifier"`
	}
	data, err := readLimited(r.Body, maxTokenRequest)
	if err != nil || json.Unmarshal(data, &body) != nil || !pkceValue.MatchString(body.Verifier) {
		refuse(w, http.StatusBadRequest, "loginFailed")
		return
	}
	code, ok := s.config.Keys.OpenCode(body.Code, s.now())
	if !ok || subtle.ConstantTimeCompare([]byte(challengeOf(body.Verifier)), []byte(code.Challenge)) != 1 {
		refuse(w, http.StatusBadRequest, "loginFailed")
		return
	}
	signedIn := session.New(code.Sub, code.Name, code.Image, s.now())
	reply(w, http.StatusOK, tokenAnswer{Token: s.config.Keys.SealApp(signedIn), User: userOf(signedIn)})
}

// sessionUser is the signed-in user as the screen sees them. The image is null when there is none.
type sessionUser struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Image *string `json:"image"`
}

// sessionAnswer is who is signed in, and the providers the screen may offer to sign in with (in the order the screen
// lists them).
type sessionAnswer struct {
	User      *sessionUser `json:"user"`
	Providers []string     `json:"providers"`
}

// providers are the logins that are configured, "github" before "ffpf". It is never nil, so the answer has a list.
func (s *server) providers() []string {
	list := []string{}
	for _, provider := range []string{providerGitHub, providerFFPF} {
		if s.ready(provider) {
			list = append(list, provider)
		}
	}
	return list
}

// ErrSessionsUnchecked is what a SessionSource answers when it cannot tell yet, without waiting, whether a session was
// revoked: the /api/auth part does not open the database for the session's answer (server/vercel).
var ErrSessionsUnchecked = errors.New("the revocations were not read")

// session says who is signed in. It is not sealed again however active the user is: a session ends MaxAge after its
// login. A revoked one (signed out of every device) is no one, and its cookie is removed, when the revocations can be
// read at once; otherwise the answer goes on without them, as the session never waits for the database, and the
// account's data, which is read with the revocations (signedIn), stays out of reach all the same.
func (s *server) session(w http.ResponseWriter, r *http.Request) {
	if !methods(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	answer := sessionAnswer{Providers: s.providers()}
	user, ok := s.owner(r)
	if !ok {
		reply(w, http.StatusOK, answer)
		return
	}
	revoked, err := s.revoked(r, user)
	if err != nil && !errors.Is(err, ErrSessionsUnchecked) && !errors.Is(err, errNoSessions) {
		logOnly(r, err, "session.check")
	}
	if revoked {
		s.dropSession(w, r)
		reply(w, http.StatusOK, answer)
		return
	}
	answer.User = userOf(user)
	reply(w, http.StatusOK, answer)
}

// signout forgets the session in the browser. With ?everywhere=1 it also signs the account out of every device: every
// session cookie and app token of a login until now is revoked (Store.RevokeSessions), which is the one way to stop a
// stolen one before it expires. An app may ask for that too, with its token and without an Origin. Signing out
// everywhere needs a session that is itself not revoked, so a stolen cookie that was revoked cannot sign out the logins
// made since; it answers loginRequired, and databaseUnavailable when the revocations cannot be read or written, and
// then the cookie is kept so that the user can try again. A browser that signs out is also marked (reloginCookie), so
// that its next login with FFPF ID asks for the password instead of passing straight through the portal's session.
func (s *server) signout(w http.ResponseWriter, r *http.Request) {
	if !methods(w, r, http.MethodPost) {
		return
	}
	everywhere := r.URL.Query().Get("everywhere") == "1"
	if !(s.sameOrigin(r) || everywhere && fromApp(r)) {
		refuse(w, http.StatusForbidden, "forbiddenRequest")
		return
	}
	if everywhere {
		user, ok := s.owner(r)
		if !ok {
			refuse(w, http.StatusUnauthorized, "loginRequired")
			return
		}
		revoked, err := s.revoked(r, user)
		if err == nil && !revoked {
			err = s.sessions().RevokeSessions(r.Context(), user.Sub)
		}
		switch {
		case err != nil:
			s.report(r, err, "session.revoke")
			refuse(w, http.StatusServiceUnavailable, "databaseUnavailable")
			return
		case revoked:
			s.dropSession(w, r)
			refuse(w, http.StatusUnauthorized, "loginRequired")
			return
		}
	}
	if !fromApp(r) {
		http.SetCookie(w, s.sessionCookie("", -1))
		http.SetCookie(w, s.relogin(int(session.MaxAge.Seconds())))
	}
	noContent(w)
}

// localLogin signs in as a made-up account, for practice on a development machine. It is off unless Config.LocalLogin.
func (s *server) localLogin(w http.ResponseWriter, r *http.Request) {
	if !s.config.LocalLogin {
		refuse(w, http.StatusNotFound, "notFound")
		return
	}
	if !methods(w, r, http.MethodPost) {
		return
	}
	if !s.sameOrigin(r) {
		refuse(w, http.StatusForbidden, "forbiddenRequest")
		return
	}
	s.issue(w, localSub, localName, "")
	noContent(w)
}

func noContent(w http.ResponseWriter) {
	header := w.Header()
	header.Set("Cache-Control", "private, no-store")
	header.Set("Vary", "Cookie")
	w.WriteHeader(http.StatusNoContent)
}
