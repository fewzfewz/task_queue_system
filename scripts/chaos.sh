#!/usr/bin/env sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT_DIR"

OUT_FILE="${1:-chaos-report.json}"
go test -tags chaos -json ./chaos > "$OUT_FILE"
echo "chaos results written to $OUT_FILE"
