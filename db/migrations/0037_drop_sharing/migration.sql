-- 0037_drop_sharing: sharing a pack (/api/shares, /api/shared) and the gallery (/api/gallery and its reports and counts)
-- were removed in v2.49.0; a pack is handed to someone as a CSV or .typle-pack.json file instead. The reports and the
-- rate limit's counts are dropped, and the snapshots of shared packs are deleted (the owners keep the packs themselves
-- in their workspaces). typle_shared_packs itself stays, empty, for the app before v2.49.0, which runs on this database
-- while a deploy builds and after an Instant Rollback: every save of its lists or packs deletes rows of that table, and
-- would fail without it. Its sharing and gallery answer databaseUnavailable or not found. A later release drops it.
DROP TABLE IF EXISTS "typle_gallery_reports";
DROP TABLE IF EXISTS "typle_gallery_hits";
DELETE FROM "typle_shared_packs";
