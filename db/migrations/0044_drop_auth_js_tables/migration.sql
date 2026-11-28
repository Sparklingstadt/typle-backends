BEGIN;
-- 0044_drop_auth_js_tables: the tables of the Auth.js login, which v2.62.1 replaced with Better Auth (0042). Nothing reads
-- them since then: signing out of every device deletes the user's Better Auth sessions instead of writing
-- typle_session_revocations, and 0043_import_auth_accounts copied every row of typle_sid_links into typle_auth_accounts,
-- where npm run sid:link now adds links. Both are dropped with their rows.
DROP TABLE IF EXISTS "typle_session_revocations";
DROP TABLE IF EXISTS "typle_sid_links";
COMMIT;
