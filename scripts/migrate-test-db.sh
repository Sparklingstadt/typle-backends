#!/bin/sh
# Makes the tables of typle-r in the database at $1 (or TYPLE_TEST_DATABASE_URL) for the Go tests: every migration of
# db/migrations (copied from typle-r, scripts/sync-from-typle-r.sh), in order (a migration that needs one opens its own transaction). It is for a new,
# empty database, as the CI's; typle-r's own database is migrated by typle-r (npm run db:migrate).
set -eu
url=${1:-${TYPLE_TEST_DATABASE_URL:?usage: scripts/migrate-test-db.sh <database URL>}}
root=$(cd "$(dirname "$0")/.." && pwd)
export PGOPTIONS="-c client_min_messages=warning"
for file in "$root"/db/migrations/*/migration.sql; do
  psql "$url" --quiet --no-psqlrc -v ON_ERROR_STOP=1 -f "$file" >/dev/null
done
echo "Applied $(ls "$root"/db/migrations/*/migration.sql | wc -l | tr -d ' ') migrations."
