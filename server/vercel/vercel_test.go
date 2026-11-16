package vercel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sparklingstadt/typle-r/server/internal/web"
	"github.com/sparklingstadt/typle-r/server/internal/workspace"
)

// backend is the Backend under test; reset gives each test a fresh one, built from its own environment.
var backend *Backend

func reset(t *testing.T) {
	t.Helper()
	backend = New(false)
}

// fakeStore is a database that answers.
type fakeStore struct{}

func (fakeStore) Ping(context.Context) error { return nil }
func (fakeStore) Modify(context.Context, string, func(*workspace.Workspace) (bool, error)) (*workspace.Workspace, error) {
	return nil, errors.New("not used")
}

func (fakeStore) Admin(context.Context, string, string) (bool, error) { return false, nil }

func settings(t *testing.T) {
	t.Helper()
	t.Setenv("AUTH_SECRET", "secret")
	t.Setenv("AUTH_URL", "https://typle.example")
	t.Setenv("AUTH_GITHUB_ID", "id")
	t.Setenv("AUTH_GITHUB_SECRET", "client-secret")
	t.Setenv("FFPF_ID_ISSUER", "")
	t.Setenv("FFPF_ID_CLIENT_ID", "")
	t.Setenv("FFPF_ID_CLIENT_SECRET", "")
	t.Setenv("DATABASE_URL", "postgresql://typle@db.invalid/typle")
	t.Setenv("POSTGRES_URL", "")
	t.Setenv("SENTRY_DSN", "")
	t.Setenv("NEXT_PUBLIC_SENTRY_DSN", "")
}

func get(path string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	backend.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response
}

// probe is a path of the database part, which serves no path of its own now that /api/admin/* is Next.js's too (until
// the Go backend is removed): with the login settings and the database open, it answers notFound.
const probe = "/api/nothing"

func serve(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	return get(probe)
}

// expectGitHub fails unless response sends the browser to GitHub's authorize URL: the auth part is built and signin
// starts a GitHub login.
func expectGitHub(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if location := response.Header().Get("Location"); response.Code != http.StatusFound ||
		!strings.HasPrefix(location, "https://github.com/login/oauth/authorize?") {
		t.Fatalf("signin = %d %q, want a redirect to GitHub", response.Code, location)
	}
}

func expect(t *testing.T, response *httptest.ResponseRecorder, status int, body string) {
	t.Helper()
	if response.Code != status || strings.TrimSpace(response.Body.String()) != body {
		t.Fatalf("response = %d %s, want %d %s", response.Code, response.Body, status, body)
	}
}

// Without the login settings the auth paths and the workspace say authUnavailable, without opening the database, and
// nothing panics (a zero web.Config would panic on the first login). Nothing of the failure is kept: once the
// settings are there, the next request is served.
func TestHandlerWithoutSecret(t *testing.T) {
	reset(t)
	settings(t)
	var opened atomic.Int32
	backend.open = func(context.Context, string) (web.Store, error) { opened.Add(1); return fakeStore{}, nil }
	t.Setenv("AUTH_SECRET", "")

	expect(t, get("/api/auth/signin"), http.StatusServiceUnavailable, `{"error":"authUnavailable"}`)
	expect(t, get("/api/auth/callback/github?code=x&state=y"), http.StatusServiceUnavailable, `{"error":"authUnavailable"}`)
	expect(t, get("/api/auth/callback/ffpf?code=x&state=y"), http.StatusServiceUnavailable, `{"error":"authUnavailable"}`)
	expect(t, get(probe), http.StatusServiceUnavailable, `{"error":"authUnavailable"}`)
	if opened.Load() != 0 {
		t.Fatal("the database was opened for a path that cannot be served without the login settings")
	}

	t.Setenv("AUTH_SECRET", "secret")
	expectGitHub(t, get("/api/auth/signin"))
	expect(t, get(probe), http.StatusNotFound, `{"error":"notFound"}`)
	if opened.Load() != 1 {
		t.Fatalf("the database was opened %d times", opened.Load())
	}
}

func TestHandlerInvalidAuthURL(t *testing.T) {
	reset(t)
	settings(t)
	backend.open = func(context.Context, string) (web.Store, error) { return fakeStore{}, nil }
	t.Setenv("AUTH_URL", "ftp://x")
	expect(t, get("/api/auth/signin"), http.StatusServiceUnavailable, `{"error":"authUnavailable"}`)
}

// While the database does not open, the probe and /api/workspace say databaseUnavailable, and the auth paths are
// answered all the same: they need no database, so a login still starts.
func TestHandlerDatabaseDown(t *testing.T) {
	reset(t)
	settings(t)
	backend.retryAfter = 0
	backend.open = func(context.Context, string) (web.Store, error) { return nil, errors.New("connection refused") }
	for _, path := range []string{probe} {
		expect(t, get(path), http.StatusServiceUnavailable, `{"error":"databaseUnavailable"}`)
	}
	expectGitHub(t, get("/api/auth/signin"))
	backend.open = func(context.Context, string) (web.Store, error) { return fakeStore{}, nil }
	expect(t, get(probe), http.StatusNotFound, `{"error":"notFound"}`)
}

// The auth paths never open the database: not when they are the first request, and not while it is down.
func TestAuthDoesNotOpenTheDatabase(t *testing.T) {
	reset(t)
	settings(t)
	var opened atomic.Int32
	backend.open = func(context.Context, string) (web.Store, error) { opened.Add(1); return nil, errors.New("down") }
	// The paths that moved to Next.js are answered too (notFound), without the database.
	for _, path := range []string{"/api/auth/signin", "/api/auth/callback/github?code=x&state=y", "/api/auth/session", "/api/auth/token"} {
		if code := get(path).Code; code == http.StatusServiceUnavailable {
			t.Errorf("%s = %d", path, code)
		}
	}
	if opened.Load() != 0 {
		t.Fatalf("the database was opened %d times for the auth paths", opened.Load())
	}
	get(probe)
	if opened.Load() != 1 {
		t.Fatalf("the database was opened %d times for the workspace", opened.Load())
	}
}

// The end of a login to the administrator's dashboard is the one auth request that asks the database, and it opens it
// only then: when it does not open, the question fails, and the handler says adminUnavailable.
func TestLazyAdmins(t *testing.T) {
	reset(t)
	settings(t)
	backend.retryAfter = 0
	var opened atomic.Int32
	backend.open = func(context.Context, string) (web.Store, error) { opened.Add(1); return nil, errors.New("down") }
	admins := lazyAdmins{backend}
	if ok, err := admins.Admin(context.Background(), "admin@example.com", "cadmin"); ok || err == nil {
		t.Fatalf("Admin with the database down = %v, %v", ok, err)
	}
	backend.open = func(context.Context, string) (web.Store, error) { opened.Add(1); return fakeStore{}, nil }
	if ok, err := admins.Admin(context.Background(), "admin@example.com", "cadmin"); ok || err != nil {
		t.Fatalf("Admin = %v, %v", ok, err)
	}
	if opened.Load() != 2 {
		t.Fatalf("opened %d times", opened.Load())
	}
}

// With a database that cannot be reached at all: a login still starts, while the database part says
// databaseUnavailable. The auth part never waits for the database.
func TestHandlerSigninWithUnreachableDatabase(t *testing.T) {
	reset(t)
	settings(t)
	t.Setenv("DATABASE_URL", "postgresql://typle@127.0.0.1:1/typle?connect_timeout=1")
	expectGitHub(t, get("/api/auth/signin"))
	expect(t, get(probe), http.StatusServiceUnavailable, `{"error":"databaseUnavailable"}`)
	expectGitHub(t, get("/api/auth/signin"))
}

// Without the GitHub OAuth App the functions are served: only signin says authUnavailable.
func TestHandlerWithoutGitHub(t *testing.T) {
	reset(t)
	settings(t)
	t.Setenv("AUTH_GITHUB_ID", "")
	t.Setenv("AUTH_GITHUB_SECRET", "")
	backend.open = func(context.Context, string) (web.Store, error) { return fakeStore{}, nil }
	expect(t, get("/api/auth/signin"), http.StatusServiceUnavailable, `{"error":"authUnavailable"}`)
	expect(t, get(probe), http.StatusNotFound, `{"error":"notFound"}`)
}

// Once built, the auth handler is found without the lock, and without reading the environment again.
func TestAuthHandlerBuiltIsKept(t *testing.T) {
	reset(t)
	settings(t)
	expectGitHub(t, get("/api/auth/signin"))
	first := backend.authHandler()
	t.Setenv("AUTH_SECRET", "")
	backend.mu.Lock()
	defer backend.mu.Unlock()
	done := make(chan http.Handler)
	go func() { done <- backend.authHandler() }()
	select {
	case again := <-done:
		if again != first {
			t.Fatal("the auth handler was built again")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("authHandler waits for the lock once the handler is built")
	}
}

func TestHandlerWithoutDatabaseURL(t *testing.T) {
	reset(t)
	settings(t)
	t.Setenv("DATABASE_URL", "")
	backend.open = func(context.Context, string) (web.Store, error) { return fakeStore{}, nil }
	expect(t, get(probe), http.StatusServiceUnavailable, `{"error":"databaseUnavailable"}`)
	t.Setenv("POSTGRES_URL", "postgresql://typle@db.invalid/typle")
	expect(t, get(probe), http.StatusNotFound, `{"error":"notFound"}`)
}

// Many requests that arrive while the database is down share one connection attempt instead of queueing behind one
// each, and nobody holds the lock while it is made.
func TestHandlerConcurrentWhileDown(t *testing.T) {
	reset(t)
	settings(t)
	var attempts atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	backend.open = func(context.Context, string) (web.Store, error) {
		if attempts.Add(1) == 1 {
			close(started)
		}
		<-release
		return nil, errors.New("connection refused")
	}

	const callers = 8
	var wg sync.WaitGroup
	codes := make(chan int, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- get(probe).Code
		}()
	}
	<-started
	// While the connection is being made the lock is free, so that a request that does not need it is not held up. The
	// other callers take it for a moment to find the attempt, so it is tried for a while rather than once.
	deadline := time.Now().Add(time.Second)
	for !backend.mu.TryLock() {
		if time.Now().After(deadline) {
			t.Fatal("the lock is held while the database is being opened")
		}
		time.Sleep(time.Millisecond)
	}
	backend.mu.Unlock()
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != http.StatusServiceUnavailable {
			t.Errorf("status = %d", code)
		}
	}
	if attempts.Load() != 1 {
		t.Fatalf("%d connection attempts for %d concurrent requests", attempts.Load(), callers)
	}
}

// A failed attempt is not repeated by every request that follows it: for retryAfter they get the 503 at once, and then
// the next one tries again.
func TestHandlerBackOff(t *testing.T) {
	reset(t)
	settings(t)
	backend.retryAfter = time.Second
	// A clock that moves only when the test says so.
	moment := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	backend.now = func() time.Time { return moment }
	var attempts atomic.Int32
	backend.open = func(context.Context, string) (web.Store, error) {
		attempts.Add(1)
		return nil, errors.New("connection refused")
	}
	for i := 0; i < 5; i++ {
		expect(t, get(probe), http.StatusServiceUnavailable, `{"error":"databaseUnavailable"}`)
		moment = moment.Add(100 * time.Millisecond)
	}
	if attempts.Load() != 1 {
		t.Fatalf("%d attempts within the back-off", attempts.Load())
	}
	moment = moment.Add(time.Second)
	get(probe)
	if attempts.Load() != 2 {
		t.Fatalf("%d attempts after the back-off", attempts.Load())
	}
	// The back-off does not hide a recovered database for longer than it lasts.
	backend.open = func(context.Context, string) (web.Store, error) { return fakeStore{}, nil }
	expect(t, get(probe), http.StatusServiceUnavailable, `{"error":"databaseUnavailable"}`)
	moment = moment.Add(time.Second)
	expect(t, get(probe), http.StatusNotFound, `{"error":"notFound"}`)
}

// Once built, the handler is kept and found without the lock or another connection.
func TestHandlerBuiltIsKept(t *testing.T) {
	reset(t)
	settings(t)
	var attempts atomic.Int32
	backend.open = func(context.Context, string) (web.Store, error) { attempts.Add(1); return fakeStore{}, nil }
	expect(t, serve(t), http.StatusNotFound, `{"error":"notFound"}`)
	first := backend.databaseHandler()

	backend.mu.Lock()
	defer backend.mu.Unlock()
	done := make(chan http.Handler)
	go func() { done <- backend.databaseHandler() }()
	select {
	case again := <-done:
		if again != first {
			t.Fatal("the handler was built again")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Handler waits for the lock once the handler is built")
	}
	if attempts.Load() != 1 {
		t.Fatalf("%d connections", attempts.Load())
	}
}

// With a real database: the default opener connects, and POSTGRES_URL is used when DATABASE_URL is not set.
func TestHandlerRealDatabase(t *testing.T) {
	connection := os.Getenv("TYPLE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("set TYPLE_TEST_DATABASE_URL to a migrated test database")
	}
	reset(t)
	settings(t)
	t.Setenv("DATABASE_URL", "")
	t.Setenv("POSTGRES_URL", connection)
	expect(t, serve(t), http.StatusNotFound, `{"error":"notFound"}`)
	expectGitHub(t, get("/api/auth/signin"))
}

func TestHandlerUnreachableDatabase(t *testing.T) {
	reset(t)
	settings(t)
	t.Setenv("DATABASE_URL", "postgresql://typle@127.0.0.1:1/typle?connect_timeout=1")
	expect(t, serve(t), http.StatusServiceUnavailable, `{"error":"databaseUnavailable"}`)
}
