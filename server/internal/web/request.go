package web

import (
	"errors"
	"io"
	"net/http"
	"strings"
)

var errTooLarge = errors.New("request body is too large")

// readLimited reads the body, giving up as soon as it is past limit rather than after reading all of it: one byte more
// than the limit is enough to know.
func readLimited(body io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err == nil && int64(len(data)) > limit {
		return nil, errTooLarge
	}
	return data, err
}

// sameOrigin reports whether the request comes from the app's own pages: its Origin is that of BaseURL, written as a
// browser writes it (lower case, without the default port). State-changing requests must pass this.
func (s *server) sameOrigin(r *http.Request) bool {
	base := s.config.BaseURL
	if base == nil || base.Host == "" {
		return false
	}
	scheme, host := strings.ToLower(base.Scheme), strings.ToLower(base.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port := base.Port(); port != "" && !(scheme == "http" && port == "80") && !(scheme == "https" && port == "443") {
		host += ":" + port
	}
	origin := r.Header.Get("Origin")
	return origin != "" && origin == scheme+"://"+host
}
