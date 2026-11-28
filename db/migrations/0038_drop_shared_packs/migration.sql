-- 0038_drop_shared_packs: the last table of sharing a pack, emptied by 0037_drop_sharing and kept for the app before
-- v2.49.0, whose every save of lists or packs deleted rows of it. From v2.49.0 nothing reads or writes it, so it is
-- dropped. The app before v2.49.0 cannot save on the database after this migration: roll back no further than v2.49.0.
DROP TABLE IF EXISTS "typle_shared_packs";
