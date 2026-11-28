BEGIN;
-- 0043_import_auth_accounts: the accounts and Sparklingstadt ID links that exist today, copied into the Better Auth tables
-- of 0042 so that a login finds the account (and its data) it opened before. Every ID that owns data, or is the target of a
-- link, becomes a user whose id is the account ID and whose address is <id>@users.invalid (accountEmail in
-- src/server/authUser.ts). Only IDs of the shapes the app uses (ACCOUNT_SUB in src/server/session.ts) are imported.
-- password: and ffpf: IDs get a user but no account: no login method signs in as them any more, and a Sparklingstadt ID
-- linked to one reaches it through its sid account. Every insert skips what exists, so applying this again changes nothing.
INSERT INTO "typle_auth_users" ("id", "name", "email", "email_verified", "created_at", "updated_at")
SELECT ids.id, '', ids.id || '@users.invalid', true, now(), now()
FROM (
  SELECT owner_id AS id FROM typle_workspaces
  UNION SELECT owner_id FROM typle_records
  UNION SELECT account_id FROM typle_sid_links
) ids
WHERE ids.id ~ '^(github:[0-9]+|password:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|ffpf:[a-z0-9]{1,64}|sid:[a-z0-9]{1,64})$'
ON CONFLICT DO NOTHING;

-- GitHub: github:<n> signs in with the GitHub user id n.
INSERT INTO "typle_auth_accounts" ("id", "account_id", "provider_id", "user_id", "created_at", "updated_at")
SELECT gen_random_uuid()::text, substr(u.id, 8), 'github', u.id, now(), now()
FROM "typle_auth_users" u
WHERE u.id ~ '^github:[0-9]+$'
ON CONFLICT ("provider_id", "account_id") DO NOTHING;

-- Sparklingstadt ID linked to an account (npm run sid:link).
INSERT INTO "typle_auth_accounts" ("id", "account_id", "provider_id", "user_id", "created_at", "updated_at")
SELECT gen_random_uuid()::text, l.sub, 'sid', l.account_id, now(), now()
FROM typle_sid_links l
JOIN "typle_auth_users" u ON u.id = l.account_id
ON CONFLICT ("provider_id", "account_id") DO NOTHING;

-- Sparklingstadt ID without a link: the account of its own, sid:<sub>.
INSERT INTO "typle_auth_accounts" ("id", "account_id", "provider_id", "user_id", "created_at", "updated_at")
SELECT gen_random_uuid()::text, substr(u.id, 5), 'sid', u.id, now(), now()
FROM "typle_auth_users" u
WHERE u.id ~ '^sid:[a-z0-9]{1,64}$'
ON CONFLICT ("provider_id", "account_id") DO NOTHING;
COMMIT;
