package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func TestRecordSave(t *testing.T) {
	ctx := context.Background()
	connection, conn := database(t)
	s := open(t, connection)
	// An operation no handler names, so that this test's rows are told from any other's.
	random := make([]byte, 8)
	_, _ = rand.Read(random)
	operation := "test." + hex.EncodeToString(random)
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `DELETE FROM typle_save_metrics WHERE operation = $1`, operation)
	})
	// A save measured too long ago is forgotten by the next one.
	if _, err := conn.Exec(ctx, `INSERT INTO typle_save_metrics (at, operation, status, duration_ms, request_bytes, response_bytes)
		VALUES (now() - interval '91 days', $1, 200, 1, 1, 1)`, operation); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordSave(ctx, SaveMetric{Operation: operation, Status: 409, DurationMS: 12, RequestBytes: 345, ResponseBytes: 6789}); err != nil {
		t.Fatal(err)
	}
	var rows, status, duration, request, response int
	if err := conn.QueryRow(ctx, `SELECT count(*)::int, max(status)::int, max(duration_ms)::int, max(request_bytes)::int, max(response_bytes)::int
		FROM typle_save_metrics WHERE operation = $1`, operation).Scan(&rows, &status, &duration, &request, &response); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || status != 409 || duration != 12 || request != 345 || response != 6789 {
		t.Fatalf("kept %d rows: status %d, %dms, %d and %d bytes", rows, status, duration, request, response)
	}
}
