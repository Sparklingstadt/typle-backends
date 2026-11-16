package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sparklingstadt/typle-r/server/internal/session"
)

// maxBodyBytes is MAX_WORKSPACE_BYTES (src/core/workspaceRequest.ts).
const maxBodyBytes = 1 << 20

var (
	errTooLarge  = errors.New("request body is too large")
	errInvalid   = errors.New("request is not a valid workspace")
	errNoBody    = errors.New("request has no body")
	errNotObject = errors.New("request body is not a JSON object")
)

// readLimited reads the body, giving up as soon as it is past limit rather than after reading all of it: one byte more
// than the limit is enough to know.
func readLimited(body io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err == nil && int64(len(data)) > limit {
		return nil, errTooLarge
	}
	return data, err
}

// parseObject reads the body as one JSON object. Its members stay unread, so that each is judged where the route
// judges it; a name is matched exactly, as in JavaScript, and the last of a repeated name wins.
func parseObject(body []byte) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, errInvalid
	}
	return fields, nil
}

// readObject reads the request's body, of at most limit bytes, as one JSON object (parseObject). It fails with errNoBody
// when there is no body, errTooLarge past the limit and errNotObject when the body is not an object; any other error is
// the body's stream breaking. Each caller answers these as its route always has.
func readObject(r *http.Request, limit int64) (map[string]json.RawMessage, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, errNoBody
	}
	body, err := readLimited(r.Body, limit)
	if err != nil {
		return nil, err
	}
	fields, err := parseObject(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errNotObject, err)
	}
	return fields, nil
}

// accountID is the request's accountId when it is a string.
func accountID(fields map[string]json.RawMessage) (string, bool) {
	var id string
	return id, json.Unmarshal(fields["accountId"], &id) == nil
}

// saveRequest checks a request that saves, to /api/workspace or its records: the Origin of a cookie's request, a JSON
// body, then the login. It reads the body as a JSON object, answering one that is not with invalid; the object's
// account is the handler's to check. A body that cannot be read is reported as reported. Otherwise it answers the
// request.
func (s *server) saveRequest(w http.ResponseWriter, r *http.Request, reported, invalid string) (session.Session, map[string]json.RawMessage, bool) {
	if !s.fromAppOrOrigin(r) || !jsonContent(r) {
		refuse(w, http.StatusForbidden, "forbiddenRequest")
		return session.Session{}, nil, false
	}
	user, ok := s.signedIn(w, r)
	if !ok {
		return session.Session{}, nil, false
	}
	fields, err := readObject(r, maxBodyBytes)
	switch {
	case err == nil:
		return user, fields, true
	case errors.Is(err, errNoBody):
		refuse(w, http.StatusBadRequest, "emptyRequest")
	case errors.Is(err, errTooLarge):
		refuse(w, http.StatusRequestEntityTooLarge, "workspaceTooLarge")
	case errors.Is(err, errNotObject):
		refuse(w, http.StatusBadRequest, invalid)
	default:
		s.report(r, err, reported)
		refuse(w, http.StatusServiceUnavailable, "workspaceSaveFailed")
	}
	return session.Session{}, nil, false
}

// fromAppOrOrigin reports whether a request may change the account's data: the Origin keeps other sites from changing it
// with the browser's cookie. An app's (iPad, Mac, Android or Windows) token is not sent by a browser on its own, so its
// requests, which have no Origin, need none.
func (s *server) fromAppOrOrigin(r *http.Request) bool {
	return fromApp(r) || s.sameOrigin(r)
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

// jsonContent reports whether the request's body is declared as JSON. A form cannot send that type to another site
// without asking first (CORS), so with the Origin it keeps other sites' pages out.
//
// The media type is compared as written, so Application/JSON is refused (TestPutForbidden, "other case"),
// though media types are case-insensitive. This is on purpose: the app always sends application/json, and the check
// is kept as it was decided. Accepting other cases would not weaken the CORS check, as no case of application/json is a type a form may send without asking.
func jsonContent(r *http.Request) bool {
	return strings.TrimSpace(strings.SplitN(r.Header.Get("Content-Type"), ";", 2)[0]) == "application/json"
}
