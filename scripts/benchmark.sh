#!/bin/bash

# Configuration
URL="http://localhost:8080/jobs"
METRICS_URL="http://localhost:8080/metrics"
PAYLOAD_FILE="scripts/payload.json"
CONCURRENCY=10
REQUESTS=1000

echo "🚀 Starting Load Test using 'ab'..."
echo "Target: $URL"
echo "Concurrency: $CONCURRENCY"
echo "Total Requests: $REQUESTS"
echo "--------------------------------------------------"

# Ensure the payload file exists
if [ ! -f "$PAYLOAD_FILE" ]; then
    echo "❌ Error: Payload file $PAYLOAD_FILE not found!"
    exit 1
fi

# Initial metrics
echo "📊 Initial Metrics:"
curl -s "$METRICS_URL" | jq .
echo ""

# Run benchmark
# -p: File containing data to POST
# -T: Content-type header for POSTing
# -c: Concurrency
# -n: Number of requests
ab -p "$PAYLOAD_FILE" -T "application/json" -c "$CONCURRENCY" -n "$REQUESTS" "$URL"

echo "--------------------------------------------------"
echo "🏁 Test complete!"
echo "📊 Final Metrics (after workers processed jobs):"
# Wait a second for workers to drain if needed, though they should be fast
sleep 2
curl -s "$METRICS_URL" | jq .
