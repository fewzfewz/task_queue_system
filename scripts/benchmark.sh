#!/bin/bash
set -e

URL=${URL:-"http://localhost:8080/jobs"}
METRICS_URL=${METRICS_URL:-"http://localhost:8080/metrics"}
PAYLOAD_FILE=${PAYLOAD_FILE:-"scripts/payload.json"}
CONCURRENCY=${CONCURRENCY:-10}
REQUESTS=${REQUESTS:-1000}
API_KEY=${API_KEY:-"secret-api-key"}

if ! command -v curl &>/dev/null; then
    echo "Error: curl is required but not installed."
    exit 1
fi

if [ ! -f "$PAYLOAD_FILE" ]; then
    echo "Error: Payload file $PAYLOAD_FILE not found!"
    exit 1
fi

echo "Starting benchmark against $URL"
echo "Concurrency: $CONCURRENCY, Requests: $REQUESTS"

echo "Initial metrics (Prometheus text, first 8000 bytes):"
curl -s "$METRICS_URL" | head -c 8000
echo

if command -v ab &>/dev/null; then
    ab -p "$PAYLOAD_FILE" -T "application/json" -H "X-API-Key: $API_KEY" -c "$CONCURRENCY" -n "$REQUESTS" "$URL"
else
    echo "ab (ApacheBench) not found. Using curl for a simpler benchmark..."
    start=$(date +%s%N)
    for i in $(seq 1 "$REQUESTS"); do
        curl -s -X POST "$URL" -H "Content-Type: application/json" -H "X-API-Key: $API_KEY" -d @"$PAYLOAD_FILE" > /dev/null &
        if [ $((i % CONCURRENCY)) -eq 0 ]; then
            wait
        fi
    done
    wait
    end=$(date +%s%N)
    duration=$(( (end - start) / 1000000 ))
    echo "Completed $REQUESTS requests in ${duration}ms"
fi

sleep 2

echo "Final metrics (Prometheus text, first 8000 bytes):"
curl -s "$METRICS_URL" | head -c 8000
echo
