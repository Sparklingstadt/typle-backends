-- 0039_password_accounts: accounts that sign in with an ID and a password. typle_users holds the account (the
-- login_id is stored lower-cased, so the unique index is case-insensitive), typle_email_tokens the one-time links sent
-- by mail (only the SHA-256 of a token is kept), typle_auth_limits the counters that throttle sign-in, sign-up and mail.
CREATE TABLE IF NOT EXISTS "typle_users" (
  "id" uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  "login_id" text NOT NULL UNIQUE,
  "email" text NOT NULL,
  "email_verified_at" timestamptz,
  "password_hash" text NOT NULL,
  "locale" text NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS "typle_users_email_idx" ON "typle_users" ("email");

CREATE TABLE IF NOT EXISTS "typle_email_tokens" (
  "token_hash" text PRIMARY KEY,
  "user_id" uuid NOT NULL REFERENCES "typle_users" ("id") ON DELETE CASCADE,
  "kind" text NOT NULL CHECK ("kind" IN ('verify', 'reset')),
  "email" text NOT NULL,
  "expires_at" timestamptz NOT NULL,
  "used_at" timestamptz
);

CREATE INDEX IF NOT EXISTS "typle_email_tokens_user_idx" ON "typle_email_tokens" ("user_id");

CREATE TABLE IF NOT EXISTS "typle_auth_limits" (
  "key" text PRIMARY KEY,
  "count" integer NOT NULL,
  "window_start" timestamptz NOT NULL,
  "blocked_until" timestamptz
);
