#!/bin/bash

# --- Colors ---
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}====================================================${NC}"
echo -e "${BLUE}       🚀 DISTRIBUTED TASK QUEUE DEMO 🚀             ${NC}"
echo -e "${BLUE}====================================================${NC}"

# Check dependencies
if ! command -v jq &> /dev/null; then
    echo -e "${YELLOW}Warning: 'jq' is not installed. JSON output will be raw.${NC}"
    alias jq='cat'
fi

# 1. Start the system
echo -e "\n${YELLOW}[1/4] Starting System via Docker Compose...${NC}"
docker-compose up -d --build --scale worker=3
echo -e "${GREEN}✔ System is booting up (API, 3 Workers, Scheduler, Redis)${NC}"

# 2. Wait for healthcheck
echo -e "\n${YELLOW}[2/4] Waiting for API to be ready...${NC}"
until curl -s http://localhost:8080/metrics > /dev/null; do
  echo -n "."
  sleep 1
done
echo -e "\n${GREEN}✔ API is LIVE!${NC}"

# 3. Submit Jobs
echo -e "\n${YELLOW}[3/4] Submitting Jobs...${NC}"

# A. Immediate Email Job
echo -e "${BLUE}Submitting an immediate 'email' job...${NC}"
curl -s -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -H "X-API-Key: secret-api-key" \
  -d '{
    "type": "email",
    "priority": "high",
    "payload": {"to": "user@example.com", "body": "Welcome to the queue!"}
  }' | jq .

# B. Scheduled Image Job (10 seconds from now)
SCHEDULE_TIME=$(date -u -d "+10 seconds" +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u -v+10s +"%Y-%m-%dT%H:%M:%SZ")
echo -e "\n${BLUE}Submitting a scheduled 'image' job (RunAt: $SCHEDULE_TIME)...${NC}"
curl -s -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -H "X-API-Key: secret-api-key" \
  -d "{
    \"type\": \"image\",
    \"priority\": \"medium\",
    \"run_at\": \"$SCHEDULE_TIME\",
    \"payload\": {\"url\": \"https://example.com/logo.png\", \"action\": \"resize\"}
  }" | jq .

# 4. Success / Monitoring
echo -e "\n${YELLOW}[4/4] Monitoring system logs... (Ctrl+C to stop)${NC}"
echo -e "${BLUE}Watch for:${NC}"
echo -e " - ${GREEN}Worker processing${NC} immediate jobs"
echo -e " - ${GREEN}Scheduler promoting${NC} the image job in 10 seconds"
echo -e " - ${GREEN}Worker processing${NC} the promoted image job"

docker-compose logs -f --tail=20
