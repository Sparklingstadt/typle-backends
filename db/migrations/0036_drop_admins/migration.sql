-- 0036_drop_admins: the administrator's dashboard (/admin, /api/admin/*) was removed in v2.48.0, and the data is looked
-- at with a separate tool that connects to the database itself. typle_admins, which only the dashboard's login read, is
-- dropped. The app before v2.48.0 still runs on the database without it: its dashboard login ends at adminUnavailable,
-- and nothing else reads the table. A shared pack is still hidden from the gallery by hand, with
-- UPDATE typle_shared_packs SET hidden_at = now() WHERE id = '<id>'.
DROP TABLE IF EXISTS "typle_admins";
