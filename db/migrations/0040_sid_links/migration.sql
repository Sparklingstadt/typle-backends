-- 0040_sid_links: a Sparklingstadt ID account linked to the account of another login (GitHub, an ID and a password),
-- so that signing in with that ID opens the account and its data. A Sparklingstadt ID without a row here is an account
-- of its own, sid:<sub>. A row is added by hand with npm run sid:link (scripts/sid-link.cjs): one ID to one account.
CREATE TABLE IF NOT EXISTS "typle_sid_links" (
  "sub" text PRIMARY KEY,
  "account_id" text NOT NULL UNIQUE,
  "created_at" timestamptz NOT NULL DEFAULT now()
);
