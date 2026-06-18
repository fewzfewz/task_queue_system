#!/bin/bash
set -e

JOBS=${1:-100}
CONCURRENCY=${2:-10}
API_URL=${API_URL:-"http://localhost:8080/jobs"}
API_KEY=${API_KEY:-"secret-api-key"}

if ! command -v curl &>/dev/null; then
    echo "Error: curl is required but not installed."
    exit 1
fi

echo "Starting load test: $JOBS jobs, concurrency $CONCURRENCY"

PAYLOAD='{"type":"email","payload":{"to":"loadtest@example.com","subject":"stress test"},"priority":"high"}'

start_time=$(date +%s%N)

seq "$JOBS" | xargs -I {} -P "$CONCURRENCY" curl -s -X POST "$API_URL" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: $API_KEY" \
    -d "$PAYLOAD" > /dev/null

end_time=$(date +%s%N)
duration_ns=$((end_time - start_time))
duration=$(echo "scale=3; $duration_ns / 1000000000" | perl -lne 'print $1 if /([\d.]+)/' 2>/dev/null || echo "unknown")

if command -v bc &>/dev/null; then
    duration_s=$(echo "scale=3; $duration_ns / 1000000000" | bc 2>/dev/null || echo "$duration")
    jps=$(echo "scale=2; $JOBS / $duration_s" | bc 2>/dev/null || echo "N/A")
elif command -v perl &>/dev/null; then
    duration_s=$(perl -e "print $duration_ns / 1000000000")
    jps=$(perl -e "printf '%.2f', $JOBS / $duration_s")
elif command -v python3 &>/dev/null; then
    duration_s=$(python3 -c "print($duration_ns / 1e9)")
    jps=$(python3 -c "print(f'{$JOBS / ($duration_ns / 1e9):.2f}')")
else
    duration_s="$duration"
    jps="N/A"
fi

echo "Load test finished!"
echo "  Total Jobs   : $JOBS"
echo "  Concurrency  : $CONCURRENCY"
echo "  Total Time   : ${duration_s}s"
echo "  Throughput   : ${jps} jobs/sec"
