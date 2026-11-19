// Package handler is the Vercel Go function of the backend comparison (docs/superpowers/specs/2026-11-19-backend-compare-design.md),
// served at /api/go. With TYPLE_BACKEND_COMPARE=1, next.config.mjs rewrites a request for /api/health, /api/workspace
// or /api/workspace/records that carries x-typle-backend: go to it, and server/vercel answers it as the Next.js Route
// Handlers would. With TYPLE_BACKEND=go it gets every request for those paths, in production too. Without either
// variable every request is a 404.
package handler

import (
	"net/http"

	"github.com/sparklingstadt/typle-r/server/vercel"
)

// Handler is the function Vercel's Go runtime calls for each request.
func Handler(w http.ResponseWriter, r *http.Request) {
	vercel.Handler().ServeHTTP(w, r)
}
