#!/bin/bash
# scripts/bench/capture.sh
#
# Baseline performance capture for the task-queue-system.
#
# Runs a per-priority enqueue/completion benchmark against a running stack,
# records p50/p95/p99 enqueue latency, p50/p95/p99 end-to-end completion
# latency, sustained completed jobs/sec, error rate, DLQ growth, and
# steady-state container CPU/mem utilisation, and writes:
#
#   docs/benchmarks/<prefix>-<date>.json
#   docs/benchmarks/<prefix>-<date>.md
#
# It also invokes scripts/load_test.sh for its headline throughput figure as a
# cross-check of the enqueue path.
#
# Reference environment (see docs/benchmarks/baseline-*.md):
#   docker-compose -p tq-bench -f docker-compose.yml \
#     -f deploy/test/docker-compose.bench.yml up -d --build --scale worker=3
#   (stop any other stack on ports 8080/6379 first)
#
# Environment overrides:
#   API_URL            (default http://localhost:8080)
#   API_KEY            (default secret-api-key)
#   JOBS_PER_TIER      jobs enqueued per priority tier (default 5000)
#   CONCURRENCY        parallel enqueue clients (default 50)
#   REDIS_ALIAS        bench Redis container name (default task_queue_redis)
#   DOCKER_PROJECT     compose project name (default tq-bench)
#   OUT_DIR            report directory (default docs/benchmarks)
#   PREFIX             report filename prefix (default baseline)
#   POLL_INTERVAL      completion polling interval ms (default 200)
#   SETTLE_TIMEOUT     max seconds to wait for completion (default 300)

set -uo pipefail

API_URL=${API_URL:-http://localhost:8080}
API_KEY=${API_KEY:-secret-api-key}
JOBS_PER_TIER=${JOBS_PER_TIER:-5000}
CONCURRENCY=${CONCURRENCY:-50}
REDIS_ALIAS=${REDIS_ALIAS:-task_queue_redis}
DOCKER_PROJECT=${DOCKER_PROJECT:-tq-bench}
OUT_DIR=${OUT_DIR:-docs/benchmarks}
PREFIX=${PREFIX:-baseline}
POLL_INTERVAL=${POLL_INTERVAL:-200}
SETTLE_TIMEOUT=${SETTLE_TIMEOUT:-300}

STAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)
REPORT_BASE="${OUT_DIR}/${PREFIX}-$(date +%Y-%m-%d)"
JSON_OUT="${REPORT_BASE}.json"
MD_OUT="${REPORT_BASE}.md"

TMPDIR_="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_"' EXIT

ENDPOINT="$API_URL/jobs"
DLQ_URL="$API_URL/api/v1/dlq"
DLQ_KEY="task_queue:jobs:dead_letter"
METRICS_KEY="task_queue:jobs:metrics:completed"
TOTAL_KEY="task_queue:jobs:metrics:total"

log() { printf '[capture] %s\n' "$*" >&2; }
warn() { printf '[capture] WARNING: %s\n' "$*" >&2; }

for tool in curl jq; do
    if ! command -v "$tool" >/dev/null 2>&1; then
        echo "Error: $tool is required but not installed." >&2
        exit 1
    fi
done

mkdir -p "$OUT_DIR"

# ── helpers ──────────────────────────────────────────────────────────────────

percentile_col() {
    # usage: percentile_col <file> <column> <p>   -> value at percentile p
    local file=$1 col=$2 p=$3
    if [ ! -s "$file" ]; then echo "NaN"; return; fi
    sort -n -k"$col" "$file" | awk -v col="$col" -v p="$p" '
        { a[NR] = $col }
        END {
            n = NR
            idx = (p / 100) * (n - 1)
            lo = int(idx)
            frac = idx - lo
            if (lo + 1 < n) v = a[lo] + frac * (a[lo + 1] - a[lo]); else v = a[lo]
            printf "%.1f", v
        }'
}

mean_col() {
    local file=$1 col=$2
    if [ ! -s "$file" ]; then echo "NaN"; return; fi
    awk -v col="$col" '{ s += $col; n++ } END { if (n > 0) printf "%.1f", s / n; else print "NaN" }' "$file"
}

total_rows() { wc -l <"$1" 2>/dev/null | tr -d ' '; }

redis_info() {
    docker exec "$REDIS_ALIAS" redis-cli INFO 2>/dev/null || true
}

redis_field() {
    # usage: redis_field <name> -> value of INFO field
    redis_info | awk -F: -v name="$1" '$1 == name { gsub("\r", "", $2); print $2; exit }'
}

dlq_len() {
    docker exec "$REDIS_ALIAS" redis-cli LLEN "$DLQ_KEY" 2>/dev/null || {
        curl -sf --max-time 10 "$DLQ_URL?limit=10000" | jq 'length' 2>/dev/null || echo "0"
    }
}

completed_counter() {
    docker exec "$REDIS_ALIAS" redis-cli GET "$METRICS_KEY" 2>/dev/null | tr -d '\r'
}

total_counter() {
    docker exec "$REDIS_ALIAS" redis-cli GET "$TOTAL_KEY" 2>/dev/null | tr -d '\r'
}

# sample_counters <outfile> <total-base> <completed-base> — background sampler
# of the enqueue-arrival (total) and completion counters, one MGET per iteration
# (both counters read atomically in a single exec), stopping once the completed
# counter has been stable for 3 consecutive samples (drain done).
# Writes "<epoch_ms>\t<total>\t<completed>" lines.
sample_counters() {
    local out=$1 tbase=$2 cbase=$3
    local end now tc t c tprev=-1 cprev=-1 cstable=0
    end=$(( $(date +%s) + SETTLE_TIMEOUT ))
    while [ "$(date +%s)" -lt "$end" ]; do
        now=$(date +%s%N)
        tc=$(docker exec "$REDIS_ALIAS" redis-cli MGET "$TOTAL_KEY" "$METRICS_KEY" 2>/dev/null | tr '\r\n' '  ')
        t=$(echo "$tc" | awk '{ print $1 + 0 }')
        c=$(echo "$tc" | awk '{ print $2 + 0 }')
        printf "%d\t%s\t%s\n" "$((now / 1000000))" "${t:-0}" "${c:-0}" >> "$out"
        if [ "$c" = "$cprev" ]; then
            [ "$c" -gt "$cbase" ] && cstable=$((cstable + 1))
        else
            cstable=0
        fi
        cprev=$c
        tprev=$t
        [ "$cstable" -ge 3 ] && break
        sleep 0.02
    done
}

# curve_latencies <counters-file> <total-base> <completed-base> <count> — prints
# one completion latency (ms) per job: time-in-system from server-side arrival
# (total counter crossing) to completion (completed counter crossing), assuming
# FIFO completion. Linear interpolation between counter samples.
curve_latencies() {
    local curve=$1 tbase=$2 cbase=$3 count=$4
    awk -F'\t' -v tbase="$tbase" -v cbase="$cbase" -v ccount="$count" '
        {
            e[++ne] = $1; tv[ne] = $2; cv[ne] = $3
        }
        END {
            ts = 1; cs = 1
            for (k = 1; k <= ccount; k++) {
                tt = tbase + k
                ct = cbase + k
                while (ts <= ne && tv[ts] < tt) ts++
                while (cs <= ne && cv[cs] < ct) cs++
                if (ts > ne || cs > ne) break
                if (ts > 1 && tv[ts-1] < tt && tv[ts] > tv[ts-1]) {
                    f = (tt - tv[ts-1]) / (tv[ts] - tv[ts-1])
                    ta = e[ts-1] + f * (e[ts] - e[ts-1])
                } else {
                    ta = e[ts]
                }
                if (cs > 1 && cv[cs-1] < ct && cv[cs] > cv[cs-1]) {
                    f = (ct - cv[cs-1]) / (cv[cs] - cv[cs-1])
                    tc = e[cs-1] + f * (e[cs] - e[cs-1])
                } else {
                    tc = e[cs]
                }
                print tc - ta
            }
        }' "$curve"
}

# ── environment spec ──────────────────────────────────────────────────────────

envspec() {
    # Returns a single JSON object describing host + containers.
    local host_json container_json
    host_json=$(jq -n \
        --arg kernel "$(uname -r)" \
        --arg compose "$(docker-compose version --short 2>/dev/null || echo unknown)" \
        --argjson cpu "$(nproc)" \
        --argjson mem "$(free -m | awk '/^Mem:/ { print $2 }')" \
        '{cpu_count: $cpu, memory_total_mb: $mem, kernel: $kernel, docker_compose_version: $compose}')

    container_json=$(docker ps --filter "label=com.docker.compose.project=$DOCKER_PROJECT" \
        --format '{{.Names}}' | sort | while read -r name; do
            docker inspect "$name" --format '{{json .}}' 2>/dev/null | jq -c --arg n "$name" '
                {
                    name: $n,
                    image: .Config.Image,
                    status: .State.Status,
                    cpus_nano: .HostConfig.NanoCpus,
                    mem_bytes: .HostConfig.Memory,
                    worker_pool_size: ([.Config.Env[]? | capture("^WORKER_POOL_SIZE=(?<v>.*)$")][0].v // null)
                }'
        done | jq -s 'if length == 0 then [] else . end')

    jq -n --argjson host "$host_json" --argjson containers "$container_json" \
        '{host: $host, containers: $containers}'
}

# ── per-tier capture ──────────────────────────────────────────────────────────

enqueue_tier() {
    # usage: enqueue_tier <tier|mixed> <outfile>
    local tier=$1 outfile=$2
    rm -f "$outfile"
    export TQ_ENDPOINT="$ENDPOINT" TQ_API_KEY="$API_KEY" TQ_OUT="$outfile" TQ_TIER="$tier"

    seq 1 "$JOBS_PER_TIER" | xargs -P "$CONCURRENCY" -I{} bash -c '
        i={}
        case "$TQ_TIER" in
            mixed) case $(( i % 3 )) in 0) prio=high;; 1) prio=medium;; 2) prio=low;; esac ;;
            *)     prio=$TQ_TIER ;;
        esac
        payload="{\"type\":\"email\",\"payload\":{\"to\":\"bench@example.com\",\"subject\":\"baseline\"},\"priority\":\"$prio\"}"
        resp_file=$(mktemp)
        t0=$(date +%s%N)
        code=$(curl -s -o "$resp_file" -w "%{http_code}" -X POST "$TQ_ENDPOINT" \
            -H "Content-Type: application/json" \
            -H "X-API-Key: $TQ_API_KEY" \
            -d "$payload")
        id=$(jq -r ".id // empty" < "$resp_file" 2>/dev/null)
        rm -f "$resp_file"
        t1=$(date +%s%N)
        printf "%s\t%d\t%d\t%s\n" "${id:-MISSING}" "$(( (t1 - t0) / 1000000 ))" "$((t1 / 1000000))" "$code" >> "$TQ_OUT"
    '
    unset TQ_ENDPOINT TQ_API_KEY TQ_OUT TQ_TIER
}

poll_tier() {
    # usage: poll_tier <enqueue-file> -> prints: id submit_ms enq_ms code complete_ms status
    # Polls pending jobs in parallel (bounded skew); completion latency here is
    # only used as a fallback — the counter-curve reconstruction is authoritative.
    local file=$1
    local out="$TMPDIR_/completion.txt"
    local donefile="$TMPDIR_/done.txt"
    local deadline poll_s
    deadline=$(( $(date +%s) + SETTLE_TIMEOUT ))
    poll_s=$(awk -v p="$POLL_INTERVAL" 'BEGIN{ printf "%.3f", p / 1000 }')
    : > "$out"
    : > "$donefile"
    export TQ_POLL_URL="$API_URL"

    while [ "$(date +%s)" -lt "$deadline" ]; do
        local pending
        pending=$(( $(total_rows "$file") - $(total_rows "$out") ))
        [ "$pending" -le 0 ] && break

        cut -f1 "$out" 2>/dev/null | sort -u > "$donefile"
        awk -F'\t' -v df="$donefile" 'FILENAME==df { skip[$1]=1; next } !($1 in skip) && $1 != "MISSING"' "$donefile" "$file" \
        | xargs -P 64 -r -n1 -d '\n' bash -c '
            IFS=$'"'"'\t'"'"' read -r r_id r_elapsed r_submit r_code <<< "$1"
            status=$(curl -sf --max-time 3 "$TQ_POLL_URL/jobs/$r_id" 2>/dev/null | jq -r ".status // \"unknown\"" 2>/dev/null || echo unknown)
            case "$status" in
                completed|failed|cancelled)
                    now=$(date +%s%N)
                    printf "%s\t%d\t%d\t%s\t%d\t%s\n" "$r_id" "$r_submit" "$r_elapsed" "$r_code" "$(( (now / 1000000) - r_submit ))" "$status"
                    ;;
            esac
        ' _ >> "$out"

        sleep "$poll_s"
    done
    cat "$out"
}

run_tier() {
    local tier=$1
    local enq_file="$TMPDIR_/enqueue-$tier.txt"
    local res_file="$TMPDIR_/results-$tier.txt"
    local elat_file="$TMPDIR_/elat-$tier.txt"
    local clat_file="$TMPDIR_/clat-$tier.txt"
    local curve_file="$TMPDIR_/curve-$tier.txt"
    local samples="$TMPDIR_/stats-$tier.txt"
    local dlq_before dlq_after dlq_growth
    local enq_start enq_end enq_ms total errors ok completed failed cancelled unfinished
    local counter_base counter_final counter_delta curve_pid
    local first_submit last_epoch span_ms completed_per_sec
    local container_stats

    dlq_before=$(dlq_len)
    counter_base=$(completed_counter); counter_base=${counter_base:-0}
    total_base=$(total_counter); total_base=${total_base:-0}
    log "tier=$tier: enqueueing $JOBS_PER_TIER jobs (concurrency $CONCURRENCY)"

    : > "$curve_file"
    sample_counters "$curve_file" "$total_base" "$counter_base" &
    curve_pid=$!

    enq_start=$(date +%s%N)
    enqueue_tier "$tier" "$enq_file"
    enq_end=$(date +%s%N)
    enq_ms=$(( (enq_end - enq_start) / 1000000 ))

    total=$(total_rows "$enq_file")
    ok=$(awk -F'\t' '$4 == 201 { n++ } END { print n+0 }' "$enq_file")
    errors=$(awk -F'\t' '$4 != 201 { n++ } END { print n+0 }' "$enq_file")
    log "tier=$tier: enqueued $ok/$total ok in ${enq_ms}ms (errors=$errors)"

    (
        local ids end
        ids=$(docker ps -q --filter "label=com.docker.compose.project=$DOCKER_PROJECT" | tr '\n' ' ')
        end=$(( $(date +%s) + SETTLE_TIMEOUT ))
        while [ "$(date +%s)" -lt "$end" ]; do
            [ -e "$samples.stop" ] && break
            docker stats --no-stream --format '{{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}' $ids >> "$samples" 2>/dev/null
            sleep 1
        done
    ) &
    local sampler_pid=$!

    log "tier=$tier: waiting for completion (SLO ${SETTLE_TIMEOUT}s)"
    poll_tier "$enq_file" > "$res_file"
    touch "$samples.stop"
    wait "$sampler_pid" 2>/dev/null || true
    wait "$curve_pid" 2>/dev/null || true

    completed=$(awk -F'\t' '$6 == "completed" { n++ } END { print n+0 }' "$res_file")
    failed=$(awk -F'\t' '$6 == "failed" { n++ } END { print n+0 }' "$res_file")
    cancelled=$(awk -F'\t' '$6 == "cancelled" { n++ } END { print n+0 }' "$res_file")
    unfinished=$(( total - completed - failed - cancelled ))

    counter_final=$(completed_counter); counter_final=${counter_final:-0}
    counter_delta=$(( counter_final - counter_base ))
    if [ "$counter_delta" -ne "$completed" ]; then
        warn "tier=$tier: completed counter delta ($counter_delta) != polled completed ($completed)"
    fi

    dlq_after=$(dlq_len)
    dlq_growth=$(( dlq_after - dlq_before ))

    awk -F'\t' '{ print $2 }' "$enq_file" > "$elat_file"

    # Completion latency: time-in-system reconstructed from the arrival (total)
    # and completion counter curves (FIFO). Falls back to observation-based
    # latency from the poll if the reconstruction is unavailable.
    : > "$clat_file"
    last_epoch=""
    first_arrival=""
    if [ "$completed" -gt 0 ] && [ -s "$curve_file" ]; then
        curve_latencies "$curve_file" "$total_base" "$counter_base" "$completed" > "$clat_file"
        first_arrival=$(awk -F'\t' -v t="$((total_base + 1))" '$2 >= t { print $1; exit }' "$curve_file")
        last_epoch=$(awk -F'\t' -v t="$((counter_base + completed))" '$3 >= t { print $1; exit }' "$curve_file")
        [ -z "$first_arrival" ] && warn "tier=$tier: arrival curve never reached base+1"
        [ -z "$last_epoch" ] && warn "tier=$tier: completion curve never reached base+$completed"
    fi
    if [ ! -s "$clat_file" ] && [ "$completed" -gt 0 ]; then
        warn "tier=$tier: counter-curve reconstruction unavailable; using observation-based completion latency"
        awk -F'\t' '$6 != "" { print $5 }' "$res_file" | grep -E '^[0-9]+$' > "$clat_file"
        last_epoch=$(awk -F'\t' '{ s = $2 + $5; if (s > max) max = s } END { print max+0 }' "$res_file")
        first_arrival=$(sort -n -k3 "$enq_file" | head -1 | cut -f3)
    fi

    if [ -n "$last_epoch" ] && [ -n "$first_arrival" ] && [ "$completed" -gt 0 ]; then
        span_ms=$(( last_epoch - first_arrival ))
        [ "$span_ms" -gt 0 ] || span_ms=1
        completed_per_sec=$(awk -v c="$completed" -v s="$span_ms" 'BEGIN{ printf "%.1f", c / (s / 1000) }')
    else
        completed_per_sec=0
    fi

    if [ -s "$samples" ]; then
        container_stats=$(awk -F'\t' '
            {
                gsub("^/", "", $1); name = $1
                cpu = $2; gsub("%", "", cpu)
                split($3, a, " / "); used = a[1]
                if (!(name in count)) { count[name] = 0; sum_cpu[name] = 0; max_cpu[name] = 0; max_mem[name] = 0 }
                count[name]++
                sum_cpu[name] += cpu
                if (cpu > max_cpu[name]) max_cpu[name] = cpu
                if (used > max_mem[name]) max_mem[name] = used
            }
            END {
                out = "["
                for (n in count) {
                    if (out != "[") out = out ","
                    out = out sprintf("{\"container\":\"%s\",\"samples\":%d,\"cpu_avg_pct\":%.1f,\"cpu_max_pct\":%.1f,\"mem_used_max\":\"%s\"}", n, count[n], sum_cpu[n]/count[n], max_cpu[n], max_mem[n])
                }
                print out "]"
            }' "$samples")
    else
        container_stats="[]"
    fi

    jq -n \
        --argjson enqueue_ms "$enq_ms" \
        --argjson total "$total" \
        --argjson ok "$ok" \
        --argjson errors "$errors" \
        --argjson ep50 "$(percentile_col "$elat_file" 1 50)" \
        --argjson ep95 "$(percentile_col "$elat_file" 1 95)" \
        --argjson ep99 "$(percentile_col "$elat_file" 1 99)" \
        --argjson emean "$(mean_col "$elat_file" 1)" \
        --argjson completed "$completed" \
        --argjson failed "$failed" \
        --argjson cancelled "$cancelled" \
        --argjson unfinished "$unfinished" \
        --argjson cp50 "$(percentile_col "$clat_file" 1 50)" \
        --argjson cp95 "$(percentile_col "$clat_file" 1 95)" \
        --argjson cp99 "$(percentile_col "$clat_file" 1 99)" \
        --argjson cmean "$(mean_col "$clat_file" 1)" \
        --argjson cps "$completed_per_sec" \
        --argjson dlq "$dlq_growth" \
        --arg container_stats "$container_stats" \
        '{
            enqueue_ms: $enqueue_ms,
            enqueued_total: $total,
            enqueue_ok_201: $ok,
            enqueue_errors: $errors,
            enqueue_error_rate: (if $total > 0 then ($errors / $total) else 0 end),
            enqueue_latency_ms: {p50: $ep50, p95: $ep95, p99: $ep99, mean: $emean},
            completed: $completed,
            failed: $failed,
            cancelled: $cancelled,
            unfinished: $unfinished,
            completion_latency_ms: {p50: $cp50, p95: $cp95, p99: $cp99, mean: $cmean},
            sustained_completed_per_sec: $cps,
            dlq_growth: $dlq,
            container_stats: ($container_stats | fromjson)
        }'
}

# ── main ──────────────────────────────────────────────────────────────────────

log "capturing benchmark baseline (date=$STAMP, jobs/tier=$JOBS_PER_TIER, concurrency=$CONCURRENCY)"
curl -sf --max-time 5 "$API_URL/readyz" >/dev/null || {
    echo "Error: API not reachable at $API_URL (is the stack up?)" >&2
    exit 1
}

ENV_JSON=$(envspec)

T_JSON="{}"
for tier in high medium low mixed; do
    log "tier=$tier"
    T_JSON=$(jq -n --argjson acc "$T_JSON" --arg tier "$tier" --argjson v "$(run_tier "$tier")" '$acc + {($tier): $v}')
done

log "cross-check: running scripts/load_test.sh (1000 jobs, concurrency 50)"
LT_OUT=$(API_URL="$ENDPOINT" API_KEY="$API_KEY" ./scripts/load_test.sh 1000 50 2>&1)
LT_JPS=$(printf '%s\n' "$LT_OUT" | grep -oE 'Throughput[[:space:]]*:[[:space:]]*[0-9.]+ jobs/sec' | grep -oE '[0-9.]+' | head -1)

jq -n \
    --arg stamp "$STAMP" \
    --argjson env "$ENV_JSON" \
    --argjson tiers "$T_JSON" \
    --argjson lt_jps "${LT_JPS:-NaN}" \
    --arg tool "scripts/bench/capture.sh" \
    --argjson jobs_per_tier "$JOBS_PER_TIER" \
    --argjson concurrency "$CONCURRENCY" \
    '{
        tool: $tool,
        captured_at_utc: $stamp,
        jobs_per_tier: $jobs_per_tier,
        concurrency: $concurrency,
        environment: $env,
        tiers: $tiers,
        cross_check: { tool: "scripts/load_test.sh", jobs: 1000, concurrency: 50, throughput_jobs_per_sec: $lt_jps }
    }' > "$JSON_OUT"

REDIS_OBJ=$(jq -n \
    --arg ver "$(redis_field redis_version)" \
    --arg mm "$(redis_field maxmemory)" \
    --arg mp "$(redis_field maxmemory_policy)" \
    '{"version": $ver, "maxmemory_bytes": $mm, "maxmemory_policy": $mp}')
jq --argjson redis "$REDIS_OBJ" '. + {redis: $redis}' "$JSON_OUT" > "$TMPDIR_/final.json"
mv "$TMPDIR_/final.json" "$JSON_OUT"

log "wrote $JSON_OUT"

# ── markdown report ──────────────────────────────────────────────────────────

{
    echo "# Benchmark Baseline — $(date -u +%Y-%m-%d)"
    echo
    echo "Captured with \`scripts/bench/capture.sh\` at $STAMP UTC."
    echo
    echo "## Reference environment"
    echo
    echo '```json'
    jq '{environment, redis}' "$JSON_OUT"
    echo '```'
    echo
    echo "## Results"
    echo
    echo "| tier | enq p50 | enq p95 | enq p99 | comp p50 | comp p95 | comp p99 | completed | failed | unfinished | err rate | DLQ growth | sustained jobs/s |"
    echo "|------|---------|---------|---------|----------|----------|----------|-----------|--------|------------|----------|-------------|------------------|"
    jq -r '.tiers | to_entries[] | [.key,
        .value.enqueue_latency_ms.p50, .value.enqueue_latency_ms.p95, .value.enqueue_latency_ms.p99,
        .value.completion_latency_ms.p50, .value.completion_latency_ms.p95, .value.completion_latency_ms.p99,
        .value.completed, .value.failed, .value.unfinished,
        .value.enqueue_error_rate, .value.dlq_growth, .value.sustained_completed_per_sec] | @tsv' "$JSON_OUT" \
        | awk -F'\t' '{ printf "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13 }'
    echo
    echo "Cross-check (scripts/load_test.sh, 1000 jobs @50): throughput $(jq -r '.cross_check.throughput_jobs_per_sec' "$JSON_OUT") jobs/sec."
    echo
    echo "## Verdict"
    echo
    VERDICT="PASS"
    if ! jq -r '.tiers | to_entries[] | [.key, (.value.enqueue_error_rate|tostring), (.value.dlq_growth|tostring), (.value.unfinished|tostring)] | @tsv' "$JSON_OUT" \
        | awk -F'\t' '$2 != 0 || $3 != 0 || $4 != 0 { bad = 1 } END { exit bad }'; then
        VERDICT="FAIL"
    fi
    echo "$VERDICT"
    echo
    echo "## Methodology"
    echo
    echo "- Enqueue latency: client-side POST round-trip time (curl timing) at concurrency ${CONCURRENCY}."
    echo "- Completion latency: end-to-end time-in-system reconstructed from the Redis \`metrics:total\`"
    echo "  (arrival) and \`metrics:completed\` (completion) counter curves sampled at ~50ms, assuming"
    echo "  FIFO completion per priority queue. Resolution is limited to the sampling grid; for very fast"
    echo "  jobs (<100ms) treat sub-sample precision as approximate."
    echo "- Sustained jobs/s: completed count / (last completion crossing - first arrival crossing)."
    echo "- Error rate, DLQ growth, and unfinished count are exact (polled per job)."
    echo
    echo "Full data: $JSON_OUT"
} > "$MD_OUT"

log "wrote $MD_OUT"
log "done"
