// Package web is the Go backend of the backend comparison (docs/superpowers/specs/2026-11-19-backend-compare-design.md):
// /api/health, /api/workspace and /api/workspace/records as one http.Handler, answering as the Next.js Route Handlers
// (src/app/api and src/server) answer them. It is served by the Vercel Go function api/go.go (server/vercel) and, for
// local runs, by server/cmd/typle-web.
package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sparklingstadt/typle-r/server/internal/operation"
	"github.com/sparklingstadt/typle-r/server/internal/session"
	"github.com/sparklingstadt/typle-r/server/internal/store"
)

// Store is what the handlers need of the database.
type Store interface {
	Ping(ctx context.Context) error
	ReadRevision(ctx context.Context, owner string) (*store.Saved, error)
	WriteRevision(ctx context.Context, owner string, revision int, change store.Change) (*store.Saved, error)
	operation.Writer
	SessionSource
}

// SessionSource tells when each account last signed out of every device (store.Store.SessionsRevokedAt).
type SessionSource interface {
	SessionsRevokedAt(ctx context.Context, owner string) (time.Time, error)
}

// Config holds what the handlers depend on.
type Config struct {
	// Store is the database. nil means it cannot be opened (no DATABASE_URL), which the routes answer with 503
	// databaseUnavailable where the Next.js routes do.
	Store Store
	// Keys opens the login cookie and the app token. BaseURL is where the app is served (AUTH_URL): its scheme picks the
	// cookie's name and its origin is the only one a write may come from. Without BaseURL the login settings are
	// missing, and /api/workspace and /api/workspace/records answer 503 authUnavailable, as readLoginSettings returning
	// null does in src/server/workspace.ts.
	Keys    session.Keys
	BaseURL *url.URL
	// Now is the clock; time.Now when nil.
	Now func() time.Time
	// Report is told of a failure that is answered with a 503, with the operation that failed (such as
	// "workspace.read"). When nil the failure is only logged.
	Report func(r *http.Request, err error, operation string)
}

type server struct {
	config Config
}

// The paths and the methods each serves, as the route files of src/app/api export them. Any other method is a 405
// with the Allow header, answered before anything else is looked at, as Next.js calls those exports first.
var routes = map[string][]string{
	"/api/health":            {http.MethodGet, http.MethodHead},
	"/api/workspace":         {http.MethodGet, http.MethodHead, http.MethodPut},
	"/api/workspace/records": {http.MethodPost, http.MethodDelete},
}

// New serves the compared paths; any other path is a 404 notFound.
func New(c Config) http.Handler {
	s := &server{config: c}
	return withSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allow, ok := routes[r.URL.Path]
		if !ok {
			refuse(w, http.StatusNotFound, "notFound")
			return
		}
		if !methods(w, r, allow...) {
			return
		}
		switch r.URL.Path {
		case "/api/health":
			s.health(w, r)
		case "/api/workspace":
			s.workspace(w, r)
		default:
			s.records(w, r)
		}
	}))
}

// The headers next.config.mjs sends with the Next.js answers of /api paths. Every answer of this handler carries them
// too, so the two backends answer with the same headers whether or not Vercel adds next.config.mjs's to a rewrite.
var securityHeaders = [...][2]string{
	{"X-Frame-Options", "DENY"},
	{"X-Content-Type-Options", "nosniff"},
	{"Referrer-Policy", "strict-origin-when-cross-origin"},
	{"Permissions-Policy", "camera=(), microphone=(), geolocation=()"},
	{"Content-Security-Policy", "frame-ancestors 'none'; base-uri 'self'; object-src 'none'"},
}

// A pointer, not a HandlerFunc, so that handlers can be compared.
type secured struct{ next http.Handler }

func withSecurityHeaders(next http.Handler) http.Handler { return &secured{next} }

func (s *secured) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := w.Header()
	for _, pair := range securityHeaders {
		header.Set(pair[0], pair[1])
	}
	s.next.ServeHTTP(w, r)
}

// reply writes every JSON answer as reply in src/server/response.ts does: JSON.stringify's bytes (& < > as they are)
// and a newline, not kept by any cache, varying with the cookie.
func reply(w http.ResponseWriter, status int, value any) {
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
	header := w.Header()
	header.Set("Content-Type", "application/json; charset=utf-8")
	header.Set("Cache-Control", "private, no-store")
	header.Set("Vary", "Cookie")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}

// Refuse answers a refused request with the key, one of the app's `errors` messages, as refuse in src/server/response.ts.
func Refuse(w http.ResponseWriter, status int, key string) {
	reply(w, status, map[string]string{"error": key})
}

func refuse(w http.ResponseWriter, status int, key string) { Refuse(w, status, key) }

// methods answers a method the path does not serve with 405 methodNotAllowed and the Allow header (methodNotAllowed in
// src/server/response.ts), and reports whether r's method is served.
func methods(w http.ResponseWriter, r *http.Request, allow ...string) bool {
	for _, method := range allow {
		if r.Method == method {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(allow, ", "))
	refuse(w, http.StatusMethodNotAllowed, "methodNotAllowed")
	return false
}

// header is the request's header name as the Fetch API's Headers.get reads it: every value, joined with ", ".
func header(r *http.Request, name string) string {
	return strings.Join(r.Header.Values(name), ", ")
}

func (s *server) now() time.Time {
	if s.config.Now != nil {
		return s.config.Now()
	}
	return time.Now()
}

// loginReady reports whether the login settings are there (readLoginSettings is not null).
func (s *server) loginReady() bool {
	return s.config.BaseURL != nil
}

func (s *server) secure() bool {
	return s.config.BaseURL != nil && s.config.BaseURL.Scheme == "https"
}

// NotFound answers every request with 404 notFound, as New answers a path it does not serve. The Vercel function
// answers so while the comparison is off.
func NotFound() http.Handler {
	return withSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refuse(w, http.StatusNotFound, "notFound")
	}))
}
