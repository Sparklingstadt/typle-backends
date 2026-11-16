package store

import "context"

// SaveMetric is one save as measured by the handler (web.measured): which operation it was ("workspace.write",
// "record.add" or "record.delete"), the HTTP status it was answered with, how long the handler took, and the bytes of
// the request's body and of the answer. It names no account and holds nothing of the workspace.
type SaveMetric struct {
	Operation     string
	Status        int
	DurationMS    int
	RequestBytes  int
	ResponseBytes int
}

// saveMetricsKept is how long a measured save stays. The next one deletes the older ones.
const saveMetricsKept = "90 days"

// RecordSave keeps one measured save in typle_save_metrics, and forgets the ones older than saveMetricsKept.
func (s *Store) RecordSave(ctx context.Context, m SaveMetric) error {
	_, err := s.pool.Exec(ctx, `WITH old AS (DELETE FROM typle_save_metrics WHERE at < now() - interval '`+saveMetricsKept+`')
		INSERT INTO typle_save_metrics (operation, status, duration_ms, request_bytes, response_bytes)
		VALUES ($1, $2, $3, $4, $5)`, m.Operation, m.Status, m.DurationMS, m.RequestBytes, m.ResponseBytes)
	return err
}
