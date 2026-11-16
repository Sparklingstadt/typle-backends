package web

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"

	"github.com/sparklingstadt/typle-r/server/internal/store"
)

// The gallery is the shares their owners list and the operator has not hidden. Its reads (/api/gallery and
// /api/gallery/categories) are answered by Next.js (src/server/gallery.ts); its writes, here, are not for search
// engines either, and nothing keeps them.

// noStore makes the answer carry Cache-Control: no-store, over the private, no-store that reply writes.
type noStore struct{ http.ResponseWriter }

func (w noStore) WriteHeader(status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(status)
}

// galleryHeaders marks every answer of the gallery, errors and 405s included, as not for search engines or caches.
func galleryHeaders(w http.ResponseWriter) http.ResponseWriter {
	w.Header().Set("X-Robots-Tag", "noindex")
	return noStore{w}
}

// queryPage reads the query's page, which is fallback when it is left out. ok is false when it is not a number.
func queryPage(r *http.Request, fallback int) (page int, ok bool) {
	value := r.URL.Query().Get("page")
	if value == "" {
		return fallback, true
	}
	page, err := strconv.Atoi(value)
	return page, err == nil
}

// galleryAdded counts one more "added to my packs" for the pack ?id=. Its Origin is checked as DELETE /api/shares's is.
// It is limited to addedPerMinute a minute per client (overLimit), after the checks. A pack that is not in the gallery is answered the same 204, so that the answer does not tell which shares are listed.
func (s *server) galleryAdded(w http.ResponseWriter, r *http.Request) {
	w = galleryHeaders(w)
	if !methods(w, r, http.MethodPost) {
		return
	}
	if !s.fromAppOrOrigin(r) {
		refuse(w, http.StatusForbidden, "forbiddenRequest")
		return
	}
	if f := s.countAdded(w, r, r.URL.Query().Get("id")); f != nil {
		s.fail(w, r, f)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// countAdded counts one more add of the pack id, within addedPerMinute a minute for the request's client (overLimit).
func (s *server) countAdded(w http.ResponseWriter, r *http.Request, id string) *failure {
	if f := s.overLimit(w, r, "added", addedPerMinute); f != nil {
		return f
	}
	if err := s.config.Store.CountAdded(r.Context(), id); err != nil {
		return broken(err, "gallery.added", "databaseUnavailable")
	}
	return nil
}

// maxIDBodyBytes is the most a report or a hide needs: a pack id and a reason, and room to spare.
const maxIDBodyBytes = 4 << 10

// readStringFields reads a body that is a JSON object with exactly the string members names, none of them empty or
// null, and no other member.
func readStringFields(r *http.Request, limit int64, names ...string) (map[string]string, bool) {
	fields, err := readObject(r, limit)
	if err != nil || len(fields) != len(names) {
		return nil, false
	}
	values := make(map[string]string, len(names))
	for _, name := range names {
		raw, ok := fields[name]
		var value string
		// null would unmarshal into value without an error and leave it empty, which is refused with it.
		if !ok || json.Unmarshal(raw, &value) != nil || value == "" {
			return nil, false
		}
		values[name] = value
	}
	return values, true
}

// galleryReport records a report of the pack "id" for "reason". It is checked as POST /api/gallery/added is, and as
// PUT /api/shares is for its JSON body. A share that is not in the gallery is notInGallery, so that /shared/<id> does
// not say a link-only share has ended. It is limited to reportsPerMinute a minute per client (overLimit), after the checks.
func (s *server) galleryReport(w http.ResponseWriter, r *http.Request) {
	w = galleryHeaders(w)
	if !methods(w, r, http.MethodPost) {
		return
	}
	if !s.fromAppOrOrigin(r) || !jsonContent(r) {
		refuse(w, http.StatusForbidden, "forbiddenRequest")
		return
	}
	fields, ok := readStringFields(r, maxIDBodyBytes, "id", "reason")
	if !ok {
		refuse(w, http.StatusBadRequest, "invalidReport")
		return
	}
	if f := s.reportPack(w, r, fields["id"], fields["reason"]); f != nil {
		s.fail(w, r, f)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// reportPack records a report of the pack id for reason: invalidReport without an id or with a reason that is not one
// of store.ReportReasons, then within reportsPerMinute a minute for the request's client (overLimit), and notInGallery
// for a share that is not in the gallery.
func (s *server) reportPack(w http.ResponseWriter, r *http.Request, id, reason string) *failure {
	if id == "" || !slices.Contains(store.ReportReasons, reason) {
		return refused(http.StatusBadRequest, "invalidReport")
	}
	if f := s.overLimit(w, r, "reports", reportsPerMinute); f != nil {
		return f
	}
	found, err := s.config.Store.Report(r.Context(), id, reason)
	if err != nil {
		return broken(err, "gallery.report", "databaseUnavailable")
	}
	if !found {
		return refused(http.StatusNotFound, "notInGallery")
	}
	return nil
}
