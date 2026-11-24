#!/bin/sh
# Copies, from a typle-r checkout, the files the backends' tests and builds read in place (README.md, "typle-r からの写し"):
# the shared fixtures, the official word lists, the base language's device name, the records API's definition and the
# rollback script. They keep typle-r's relative paths, so nothing here needs to know that they are copies.
set -eu
src=${1:?usage: scripts/sync-from-typle-r.sh <typle-r checkout>}
root=$(cd "$(dirname "$0")/.." && pwd)

rm -rf "$root/tests/fixtures" "$root/src/lib/packs"
mkdir -p "$root/tests" "$root/src/lib/packs/commands" "$root/src/messages/ja" "$root/docs/api"
cp -R "$src/tests/fixtures" "$root/tests/"
cp "$src/src/lib/packs/samples.json" "$src/src/lib/packs/countries.json" "$root/src/lib/packs/"
cp "$src"/src/lib/packs/commands/*.json "$root/src/lib/packs/commands/"
cp "$src/src/messages/ja/format.json" "$root/src/messages/ja/"
cp "$src/docs/api/workspace-records.openapi.json" "$root/docs/api/"
cp "$src/scripts/records-rollback.sql" "$root/scripts/"

echo "Copied from typle-r $(git -C "$src" rev-parse --short HEAD). Run the tests, and name this commit in README.md."
