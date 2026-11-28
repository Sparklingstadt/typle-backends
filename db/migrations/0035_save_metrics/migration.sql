-- 0035_save_metrics: how long each save took and how large it was, for the measurement of Issue #174. The Go backend
-- adds a row per PUT /api/workspace and per POST or DELETE /api/workspace/records while TYPLE_MEASURE_SAVES=1, with no
-- account and no content: the operation, the HTTP status (409 is a conflict), the handler's time and the bytes of the
-- request and the answer. Rows older than 90 days are deleted by the next row. Only a table is added, so the app
-- before this migration keeps working on it.
CREATE TABLE IF NOT EXISTS "typle_save_metrics" (
  "id" BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  "at" TIMESTAMPTZ(6) NOT NULL DEFAULT now(),
  "operation" TEXT NOT NULL,
  "status" INTEGER NOT NULL,
  "duration_ms" INTEGER NOT NULL,
  "request_bytes" INTEGER NOT NULL,
  "response_bytes" INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS "typle_save_metrics_at" ON "typle_save_metrics" ("at");
