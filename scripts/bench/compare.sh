#!/bin/bash
# scripts/bench/compare.sh
#
# Diffs a benchmark report against a committed baseline and reports drift.
#
# Usage:
#   scripts/bench/compare.sh <new-report.json> [baseline.json]
#
# If the baseline is omitted, the newest docs/benchmarks/baseline-*.json is
# used. Metrics compared per tier: enqueue p50/p95/p99, completion p50/p95/p99,
# sustained completed/sec, error rate, DLQ growth, unfinished count.
#
# Exit code:
#   0  no drift beyond tolerance
#   1  drift detected (metrics beyond tolerance), unless --warn
#
# Flags:
#   -t <pct>     tolerance percent (default 20)
#   --warn       exit 0 even when drift is detected (non-blocking mode)
#   -v           verbose per-metric drift table

set -uo pipefail

TOLERANCE=${TOLERANCE:-20}
WARN=0
VERBOSE=0

usage() {
    echo "usage: $0 [-t pct] [--warn] [-v] <new-report.json> [baseline.json]" >&2
    exit 2
}

while [ $# -gt 0 ]; do
    case "$1" in
        -t) TOLERANCE=$2; shift 2 ;;
        --warn) WARN=1; shift ;;
        -v) VERBOSE=1; shift ;;
        -*) usage ;;
        *) break ;;
    esac
done

NEW=${1:-}
BASE=${2:-}
if [ -z "$NEW" ]; then usage; fi
if [ -z "$BASE" ]; then
    BASE=$(ls docs/benchmarks/baseline-*.json 2>/dev/null | sort | tail -1)
    if [ -z "$BASE" ]; then
        echo "Error: no committed baseline found in docs/benchmarks/" >&2
        exit 2
    fi
fi

if ! jq -e . "$NEW" >/dev/null 2>&1; then echo "Error: invalid JSON in $NEW" >&2; exit 2; fi
if ! jq -e . "$BASE" >/dev/null 2>&1; then echo "Error: invalid JSON in $BASE" >&2; exit 2; fi

TOL=$(awk -v t="$TOLERANCE" 'BEGIN{ print t / 100 }')

DRIFT=0
if [ "$VERBOSE" -eq 1 ]; then
    printf '%-14s %-16s %12s %12s %9s\n' metric tier baseline new "drift%"
fi

check() {
    # usage: check <label> <tier> <baseline-value> <new-value>
    local label=$1 tier=$2 base=$3 new=$4
    local drift_pct
    if [ "$base" = "NaN" ] || [ "$new" = "NaN" ]; then
        printf '%-14s %-16s %12s %12s %8s\n' "$label" "$tier" "$base" "$new" "SKIP"
        return
    fi
    if [ "$(awk -v b="$base" 'BEGIN{ print (b==0?1:0) }')" -eq 1 ]; then
        # zero baseline: flag any non-zero new value
        if [ "$(awk -v n="$new" 'BEGIN{ print (n==0?0:1) }')" -eq 1 ]; then
            printf '%-14s %-16s %12s %12s %8s\n' "$label" "$tier" "$base" "$new" "INF"
            DRIFT=1
        fi
        return
    fi
    drift_pct=$(awk -v b="$base" -v n="$new" 'BEGIN{ printf "%.1f", ((n - b) / b) * 100 }')
    if [ "$VERBOSE" -eq 1 ]; then
        printf '%-14s %-16s %12s %12s %9s\n' "$label" "$tier" "$base" "$new" "${drift_pct}%"
    fi
    if [ "$(awk -v d="$drift_pct" -v t="$TOL" 'BEGIN{ print (d < -t ? 1 : (d > t ? 1 : 0)) }')" -eq 1 ]; then
        DRIFT=1
        printf 'DRIFT  %-12s %-16s %s -> %s (%.1f%% > %.0f%%)\n' "$label" "$tier" "$base" "$new" "$drift_pct" "$TOLERANCE" >&2
    fi
}

for tier in high medium low mixed; do
    for pair in \
        "enqueue_latency_ms.p50" \
        "enqueue_latency_ms.p95" \
        "enqueue_latency_ms.p99" \
        "completion_latency_ms.p50" \
        "completion_latency_ms.p95" \
        "completion_latency_ms.p99" \
        "sustained_completed_per_sec" \
        "enqueue_error_rate" \
        "dlq_growth" \
        "unfinished"; do
        b=$(jq -r ".tiers[\"$tier\"].$pair" "$BASE")
        n=$(jq -r ".tiers[\"$tier\"].$pair" "$NEW")
        check "$pair" "$tier" "$b" "$n"
    done
done

# cross-check throughput
b=$(jq -r '.cross_check.throughput_jobs_per_sec' "$BASE")
n=$(jq -r '.cross_check.throughput_jobs_per_sec' "$NEW")
check "throughput_jps" "cross" "$b" "$n"

if [ "$DRIFT" -eq 1 ]; then
    if [ "$WARN" -eq 1 ]; then
        echo "Drift detected beyond ±${TOLERANCE}% (non-blocking mode, exit 0)" >&2
        exit 0
    fi
    echo "Drift detected beyond ±${TOLERANCE}% vs baseline $BASE" >&2
    exit 1
fi

echo "No drift beyond ±${TOLERANCE}% vs baseline $BASE"
exit 0
