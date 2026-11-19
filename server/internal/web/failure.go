package web

import (
	"log"
	"net/http"
)

// failure is a request refused: the status and the error key it is answered with. A failure of the server (a 503) also
// has the error to report, with the operation that failed. The handlers answer it with refuse (fail).
type failure struct {
	status    int
	key       string
	err       error
	operation string
}

func refused(status int, key string) *failure { return &failure{status: status, key: key} }

// broken is a failure of the server that is answered with a 503 and key, and reported.
func broken(err error, operation, key string) *failure {
	return &failure{status: http.StatusServiceUnavailable, key: key, err: err, operation: operation}
}

// fail reports f if it is the server's failure, and answers the request with it.
func (s *server) fail(w http.ResponseWriter, r *http.Request, f *failure) {
	if f.err != nil {
		s.report(r, f.err, f.operation)
	}
	refuse(w, f.status, f.key)
}

// report tells Config.Report, or the log, of a failure that is answered with a 503, as reportFailure of
// src/server/report.ts logs it (Sentry is left out here).
func (s *server) report(r *http.Request, err error, operation string) {
	if s.config.Report != nil {
		s.config.Report(r, err, operation)
		return
	}
	log.Printf("%s %s: %s: %v", r.Method, r.URL.Path, operation, err)
}
