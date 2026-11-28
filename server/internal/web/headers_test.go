package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The headers next.config.mjs gives the Next.js answers of /api paths.
var wantSecurityHeaders = map[string]string{
	"Content-Security-Policy": "frame-ancestors 'none'; base-uri 'self'; object-src 'none'",
	"X-Frame-Options":         "DENY",
	"X-Content-Type-Options":  "nosniff",
	"Referrer-Policy":         "strict-origin-when-cross-origin",
	"Permissions-Policy":      "camera=(), microphone=(), geolocation=()",
}

func checkSecurityHeaders(t *testing.T, name string, header http.Header) {
	t.Helper()
	for key, want := range wantSecurityHeaders {
		if got := header.Values(key); len(got) != 1 || got[0] != want {
			t.Errorf("%s: %s = %q; want %q", name, key, got, want)
		}
	}
}

// Every answer carries the security headers and is written as reply in src/server/response.ts writes it: not kept by
// any cache, and varying with the cookie.
func TestHeadersUniform(t *testing.T) {
	h := newHarness(t, &fakeWorkspaces{}, testBase)
	responses := map[string]*httptest.ResponseRecorder{
		"workspace":            h.get(testOwner),
		"workspace signed out": h.get(""),
		"not found":            get(t, h.handler, http.MethodGet, "/nowhere"),
		"method not allowed":   get(t, h.handler, http.MethodDelete, "/api/workspace"),
		"records refused":      get(t, h.handler, http.MethodPost, "/api/workspace/records"),
		"auth unavailable":     get(t, New(Config{Store: fakeStore{}}), http.MethodGet, "/api/workspace"),
		"database missing":     get(t, New(Config{Secret: testSecret, BaseURL: mustURL(t, testBase)}), http.MethodGet, "/api/workspace"),
	}
	for name, response := range responses {
		checkSecurityHeaders(t, name, response.Header())
		header := response.Header()
		if header.Get("Content-Type") != "application/json; charset=utf-8" || header.Get("Cache-Control") != "private, no-store" ||
			header.Get("Vary") != "Cookie" {
			t.Errorf("%s: %d %v", name, response.Code, header)
		}
	}
}

// Every other method is a 405 with the methods of the route file (src/app/api), whatever else is wrong with the request.
func TestMethodNotAllowed(t *testing.T) {
	routes := map[string]struct {
		allow   string
		refused []string
	}{
		"/api/health":            {"GET, HEAD", []string{"POST", "PUT", "PATCH", "DELETE", "OPTIONS"}},
		"/api/workspace":         {"GET, HEAD, PUT", []string{"POST", "PATCH", "DELETE", "OPTIONS"}},
		"/api/workspace/records": {"POST, DELETE", []string{"GET", "HEAD", "PUT", "PATCH", "OPTIONS"}},
	}
	// No login settings and no database: the 405 comes first all the same.
	handler := New(Config{})
	for path, route := range routes {
		for _, method := range route.refused {
			response := get(t, handler, method, path)
			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != route.allow {
				t.Errorf("%s %s = %d %v", method, path, response.Code, response.Header())
			}
			if method != http.MethodHead && response.Body.String() != "{\"error\":\"methodNotAllowed\"}\n" {
				t.Errorf("%s %s = %q", method, path, response.Body)
			}
		}
	}
}

// HEAD /api/workspace is answered as GET, and a server leaves the body out.
func TestWorkspaceHead(t *testing.T) {
	h := newHarness(t, &fakeWorkspaces{}, testBase)
	server := httptest.NewServer(h.handler)
	defer server.Close()
	request, err := http.NewRequest(http.MethodHead, server.URL+"/api/workspace", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(h.cookie(testOwner, false))
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || len(body) != 0 || response.Header.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("response = %d %q %v", response.StatusCode, body, response.Header)
	}
	if signedOut := h.request(http.MethodHead, "", ""); signedOut.Code != http.StatusUnauthorized {
		t.Fatalf("signed out: %d", signedOut.Code)
	}
}
