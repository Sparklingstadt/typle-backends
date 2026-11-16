package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/sparklingstadt/typle-r/server/internal/auth"
	"github.com/sparklingstadt/typle-r/server/internal/session"
)

// The administrator's dashboard (/admin) has a login of its own. It goes through FFPF ID only
// (/api/auth/signin?admin=1, then /api/auth/callback/ffpf), and ends with the administrator's cookie instead of the
// session cookie, for an account that typle_admins lets in: one registered there by its email, which binds the
// account's sub at its first sign-in. Each /api/admin request (src/server/admin.ts since v2.46.8) checks the account
// against typle_admins again, so an administrator whose row is removed is let in no longer.
const (
	adminPage = "/admin"
	// adminRefused is where a login of an account that is not an administrator ends: the page says so.
	adminRefused = adminPage + "?error=adminForbidden"
	// adminUnavailable is where a login to the dashboard ends when typle_admins could not be asked.
	adminUnavailable = adminPage + "?error=adminUnavailable"
)

// adminCookie is SameSite=Strict: the dashboard is only reached from the app's own pages, and its cookie is never sent
// with a request another site starts.
func (s *server) adminCookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: session.AdminCookieName(s.secure()), Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.secure(),
	}
}

// finishAdmin ends a login to the dashboard through FFPF ID: the administrator's cookie if typle_admins lets the
// account in, and adminRefused otherwise, or adminUnavailable when the database does not answer. It never sets the
// session cookie, so a login to the dashboard does not sign in to the app.
func (s *server) finishAdmin(w http.ResponseWriter, r *http.Request, user auth.FFPFUser) {
	email := strings.ToLower(user.Email)
	allowed, err := s.isAdmin(r.Context(), email, user.Sub)
	if err != nil {
		s.report(r, err, "admin.check")
		goTo(w, adminUnavailable)
		return
	}
	if !allowed {
		goTo(w, adminRefused)
		return
	}
	value := s.config.Keys.SealAdmin(session.NewAdmin(email, user.Sub, s.now()))
	http.SetCookie(w, s.relogin(-1))
	http.SetCookie(w, s.adminCookie(value, int(session.AdminMaxAge.Seconds())))
	goTo(w, adminPage)
}

// errNoAdmins is a server set up without a database to ask about administrators.
var errNoAdmins = errors.New("no database to check the administrator against")

// isAdmin asks typle_admins, through Config.Admins or else Config.Store, whether the FFPF ID account is an
// administrator.
func (s *server) isAdmin(ctx context.Context, email, sub string) (bool, error) {
	if s.config.Admins != nil {
		return s.config.Admins.Admin(ctx, email, sub)
	}
	if s.config.Store != nil {
		return s.config.Store.Admin(ctx, email, sub)
	}
	return false, errNoAdmins
}
