#!/bin/bash

# Load Testing Script for Task Queue System
# Usage: ./load_test.sh [number_of_jobs] [concurrency]

JOBS=${1:-100}
CONCURRENCY=${2:-10}
API_URL=${API_URL:-"http://localhost:8080/jobs"}
API_KEY=${API_KEY:-"secret-api-key"}

echo "🚀 Starting load test: $JOBS jobs, concurrency $CONCURRENCY"
echo "──────────────────────────────────────────────────────────"

# Create a temporary file for the job payload
PAYLOAD='{"type":"email","payload":{"to":"loadtest@example.com","subject":"stress test"},"priority":"high"}'

start_time=$(date +%s.%N)

# Use xargs to handle concurrency
# We pipe the payload N times into curl
seq $JOBS | xargs -I {} -P $CONCURRENCY curl -s -X POST "$API_URL" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: $API_KEY" \
    -d "$PAYLOAD" > /dev/null

end_time=$(date +%s.%N)

# Calculate results
duration=$(echo "$end_time - $start_time" | bc)
jps=$(echo "scale=2; $JOBS / $duration" | bc)

echo "──────────────────────────────────────────────────────────"
echo "✔ Load test finished!"
echo "  Total Jobs   : $JOBS"
echo "  Concurrency  : $CONCURRENCY"
echo "  Total Time   : ${duration}s"
echo "  Throughput   : ${jps} jobs/sec"
echo "──────────────────────────────────────────────────────────"
