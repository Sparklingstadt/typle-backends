BEGIN;
-- 0041_drop_save_metrics: measuring saves (TYPLE_MEASURE_SAVES=1, 0035_save_metrics) was removed in v2.60.9, when
-- Issue #174 was closed without the measurement. typle_save_metrics, which nothing else read, is dropped with its rows.
-- The app before v2.60.9 still runs on the database without it: with TYPLE_MEASURE_SAVES=1 the row of a save fails to
-- be written, which is only logged, and the save is answered as before.
DROP TABLE IF EXISTS "typle_save_metrics";
COMMIT;
