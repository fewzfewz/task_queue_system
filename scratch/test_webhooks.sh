go build -o bin/api cmd/api/main.go
go build -o bin/worker cmd/worker/main.go
bin/api &
API_PID=$!
bin/worker &
WORKER_PID=$!
sleep 2

# Register tenant and webhook
curl -s -X POST http://localhost:8080/api/v1/clients -d '{"tenant_id": "wh-tester"}' > res.json
TOKEN=$(jq -r '.api_key' res.json)
curl -s -X POST http://localhost:8080/api/v1/webhooks \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"url": "http://localhost:8081/webhook", "events": ["completed", "failed"]}'

# Start dummy webhook receiver
nc -l -p 8081 > webhook_output.txt &
NC_PID=$!

# Submit job
curl -s -X POST http://localhost:8080/api/v1/jobs \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"type": "echo", "payload": {"msg": "hello webhooks!"}}'

sleep 3

kill $API_PID $WORKER_PID $NC_PID
cat webhook_output.txt
