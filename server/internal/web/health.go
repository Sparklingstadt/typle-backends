package web

import (
	"context"
	"log"
	"net/http"
	"time"
)

// health answers as checkHealth in src/server/health.ts: 200 once the database answers within 5 seconds, and 503
// databaseUnavailable otherwise, also when no database is set. It needs no login settings. HEAD is served as GET;
// net/http leaves the body out.
func (s *server) health(w http.ResponseWriter, r *http.Request) {
	if s.config.Store == nil {
		log.Printf("%s /api/health: DATABASE_URL is not set", r.Method)
		refuse(w, http.StatusServiceUnavailable, "databaseUnavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.config.Store.Ping(ctx); err != nil {
		log.Printf("%s /api/health: %v", r.Method, err)
		refuse(w, http.StatusServiceUnavailable, "databaseUnavailable")
		return
	}
	reply(w, http.StatusOK, map[string]string{"status": "ok"})
}
