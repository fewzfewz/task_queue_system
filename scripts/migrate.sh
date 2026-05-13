#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

exec go run ./cmd/cli migrate-jobs --from redis --to postgres --batch "${BATCH:-500}"
