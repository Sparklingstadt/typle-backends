// Package vercel builds the handler that the Vercel Go function api/go.go serves, and that server/cmd/typle-web serves
// locally. It is outside server/internal because api/ at the repository root may not import internal packages.
package vercel

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sparklingstadt/typle-r/server/internal/store"
	"github.com/sparklingstadt/typle-r/server/internal/web"
)

// BackendHeader is the header that picks the backend of a compared request, and that every answer of this one carries
// with the value "go", so that the benchmark can tell which backend answered.
const BackendHeader = "x-typle-backend"

// Backend is the Go backend of the comparison, built lazily from the environment: the login settings (AUTH_SECRET and
// AUTH_URL) and the database (DATABASE_URL, or else POSTGRES_URL). Once both are there the handler is kept and found
// with one atomic load; until then each request builds one from what is there, so a request answers 503
// authUnavailable or databaseUnavailable where the Next.js routes do, and a later one looks at the environment again.
type Backend struct {
	// gated is the Vercel function's: every request is a 404 unless, at the time, TYPLE_BACKEND_COMPARE is 1 or
	// TYPLE_BACKEND is go (answers reports which).
	gated  bool
	getenv func(string) string
	// open makes the pool; the tests replace it.
	open func(ctx context.Context, connection string) (web.Store, error)

	ready atomic.Pointer[http.Handler]

	mu     sync.Mutex
	source web.Store // the pool, once made
}

// New returns a Backend. gated is true for the Vercel function and false for the typle-web command, which is only run
// on a development machine.
func New(gated bool) *Backend {
	return &Backend{gated: gated, getenv: os.Getenv, open: func(ctx context.Context, connection string) (web.Store, error) {
		source, err := store.OpenApp(ctx, connection)
		if err != nil {
			return nil, err
		}
		return source, nil
	}}
}

var shared = New(true)

// Handler returns the handler of the Vercel function: the Backend of this process.
func Handler() http.Handler { return shared }

var notFound = web.NotFound()

// ServeHTTP answers r as the Next.js route of its path would, on the path routedPath finds, and marks the answer as
// this backend's.
func (b *Backend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(BackendHeader, "go")
	if b.gated && !b.answers() {
		notFound.ServeHTTP(w, r)
		return
	}
	path := routedPath(r.URL)
	if path != r.URL.Path {
		routed := *r.URL
		routed.Path, routed.RawPath = path, ""
		r = r.WithContext(r.Context())
		r.URL = &routed
	}
	b.handler().ServeHTTP(w, r)
}

// answers reports whether the gated function answers: in the comparison (TYPLE_BACKEND_COMPARE=1), and when Go is the
// backend of the compared paths (TYPLE_BACKEND=go, which next.config.mjs reads too).
func (b *Backend) answers() bool {
	return b.getenv("TYPLE_BACKEND_COMPARE") == "1" || b.getenv("TYPLE_BACKEND") == "go"
}

// routedPath is the path of the Next.js route a request is for. A Vercel rewrite may hand the function the request's
// own path (/api/workspace), the rewritten one with the prefix of the function (/api/go/workspace), or the function's
// path alone (/api/go) with the route in the path query parameter (/api/go?path=workspace); each is read here.
func routedPath(u *url.URL) string {
	if rest, ok := strings.CutPrefix(u.Path, "/api/go/"); ok {
		return "/api/" + rest
	}
	if u.Path == "/api/go" {
		if route := u.Query().Get("path"); route != "" {
			return "/api/" + strings.TrimPrefix(route, "/")
		}
	}
	return u.Path
}

// Close releases the pool, if it was made.
func (b *Backend) Close() {
	b.mu.Lock()
	source := b.source
	b.mu.Unlock()
	if open, ok := source.(interface{ Close() }); ok {
		open.Close()
	}
}

// handler is the kept handler, or one built from the environment as it is now.
func (b *Backend) handler() http.Handler {
	if built := b.ready.Load(); built != nil {
		return *built
	}
	keys, base, loginReady := web.LoginFromEnv(b.getenv)
	source := b.database()
	handler := web.New(web.Config{Store: source, Keys: keys, BaseURL: base})
	if loginReady && source != nil {
		b.ready.CompareAndSwap(nil, &handler)
		return *b.ready.Load()
	}
	return handler
}

// database is the pool, made on first use from DATABASE_URL (or POSTGRES_URL) as pool() of src/server/database.ts
// makes it, or nil when no URL is set or it is not a database URL. Making it connects to nothing.
func (b *Backend) database() web.Store {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.source != nil {
		return b.source
	}
	connection := b.getenv("DATABASE_URL")
	if connection == "" {
		connection = b.getenv("POSTGRES_URL")
	}
	if connection == "" {
		return nil
	}
	source, err := b.open(context.Background(), connection)
	if err != nil {
		log.Printf("open the database: %v", err)
		return nil
	}
	b.source = source
	return source
}
