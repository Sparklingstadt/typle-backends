package web

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/sparklingstadt/typle-r/server/internal/store"
)

// SaveMeter keeps a measured save (store.Store.RecordSave). It is not part of Store: a Store that is no SaveMeter is
// simply not measured.
type SaveMeter interface {
	RecordSave(ctx context.Context, m store.SaveMetric) error
}

// measureTimeout is the longest a measurement may hold the answer of the save it measured.
const measureTimeout = 2 * time.Second

// counted is a ResponseWriter that remembers the status and the bytes of the body written through it.
type counted struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (c *counted) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
	c.ResponseWriter.WriteHeader(status)
}

func (c *counted) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	n, err := c.ResponseWriter.Write(p)
	c.bytes += n
	return n, err
}

// saveOperation names the save a request to /api/workspace or /api/workspace/records asks for, as the failures are
// reported; "" for a request that saves nothing (GET, HEAD, a method that is not allowed).
func saveOperation(r *http.Request) string {
	switch {
	case r.URL.Path == "/api/workspace" && r.Method == http.MethodPut:
		return "workspace.write"
	case r.URL.Path == "/api/workspace/records" && r.Method == http.MethodPost:
		return "record.add"
	case r.URL.Path == "/api/workspace/records" && r.Method == http.MethodDelete:
		return "record.delete"
	}
	return ""
}

// measured is next with every save measured (Config.MeasureSaves, Issue #174): how long the handler took, the status it
// answered with, and the bytes of the request's body and of the answer. The measurement is kept after the handler, on
// the same request, because a serverless instance may stop once the request is answered; so it holds the answer for
// one INSERT, and never for longer than measureTimeout. One that fails is only logged. Without Config.MeasureSaves,
// or with a Store that is no SaveMeter, it is next itself.
func (s *server) measured(next http.HandlerFunc) http.HandlerFunc {
	meter, ok := s.config.Store.(SaveMeter)
	if !s.config.MeasureSaves || !ok {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		operation := saveOperation(r)
		if operation == "" {
			next(w, r)
			return
		}
		c := &counted{ResponseWriter: w}
		start := s.now()
		next(c, r)
		elapsed := s.now().Sub(start)
		if c.status == 0 {
			c.status = http.StatusOK
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), measureTimeout)
		defer cancel()
		metric := store.SaveMetric{Operation: operation, Status: c.status, DurationMS: int(elapsed / time.Millisecond),
			RequestBytes: int(max(r.ContentLength, 0)), ResponseBytes: c.bytes}
		if err := meter.RecordSave(ctx, metric); err != nil {
			log.Printf("measure %s: %v", operation, err)
		}
	}
}
