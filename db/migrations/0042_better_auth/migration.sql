BEGIN;
-- 0042_better_auth: the tables of Better Auth (the login that replaces Auth.js). The user's id is the account ID
-- (github:<n>, sid:<sub>), the owner_id of the account's data, so it is text, not a generated id. The address is made up
-- from the ID (<id>@users.invalid). Nothing reads these tables until the handler is mounted; the old tables are untouched.
CREATE TABLE "typle_auth_users" (
  "id" text PRIMARY KEY,
  "name" text NOT NULL,
  "email" text NOT NULL UNIQUE,
  "email_verified" boolean NOT NULL DEFAULT false,
  "image" text,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE "typle_auth_sessions" (
  "id" text PRIMARY KEY,
  "expires_at" timestamptz NOT NULL,
  "token" text NOT NULL UNIQUE,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now(),
  "ip_address" text,
  "user_agent" text,
  "user_id" text NOT NULL REFERENCES "typle_auth_users" ("id") ON DELETE CASCADE ON UPDATE CASCADE
);

CREATE INDEX "typle_auth_sessions_user_id_idx" ON "typle_auth_sessions" ("user_id");

CREATE TABLE "typle_auth_accounts" (
  "id" text PRIMARY KEY,
  "account_id" text NOT NULL,
  "provider_id" text NOT NULL,
  "user_id" text NOT NULL REFERENCES "typle_auth_users" ("id") ON DELETE CASCADE ON UPDATE CASCADE,
  "access_token" text,
  "refresh_token" text,
  "id_token" text,
  "access_token_expires_at" timestamptz,
  "refresh_token_expires_at" timestamptz,
  "scope" text,
  "password" text,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX "typle_auth_accounts_user_id_idx" ON "typle_auth_accounts" ("user_id");

-- One row per login method and its ID there: the same GitHub or Sparklingstadt ID cannot belong to two users.
CREATE UNIQUE INDEX "typle_auth_accounts_provider_id_account_id_key" ON "typle_auth_accounts" ("provider_id", "account_id");

CREATE TABLE "typle_auth_verifications" (
  "id" text PRIMARY KEY,
  "identifier" text NOT NULL,
  "value" text NOT NULL,
  "expires_at" timestamptz NOT NULL,
  "created_at" timestamptz NOT NULL DEFAULT now(),
  "updated_at" timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX "typle_auth_verifications_identifier_idx" ON "typle_auth_verifications" ("identifier");
COMMIT;
