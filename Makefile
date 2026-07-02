APP_NAME=task-queue-system

.PHONY: help deps test build build-api build-worker build-scheduler build-cli run-api run-worker run-scheduler dev dev-stop swagger docker-build docker-up docker-down migrate migrate-schema migrate-down migrate-down-schema chaos load-test benchmark

help:
	@echo "Targets:"
	@echo "  deps               Download Go module dependencies"
	@echo "  test               Run the test suite"
	@echo "  build              Build all binaries"
	@echo "  run-api            Run the API locally"
	@echo "  run-worker         Run the worker locally"
	@echo "  run-scheduler      Run the scheduler locally"
	@echo "  dev                Start Redis + all services (in background)"
	@echo "  dev-stop           Stop all dev background processes"
	@echo "  swagger            Regenerate Swagger docs"
	@echo "  docker-build       Build the Docker image"
	@echo "  docker-up          Start the local Docker Compose stack"
	@echo "  docker-down        Stop the local Docker Compose stack"
	@echo "  migrate            Run the Redis -> Postgres migration CLI"
	@echo "  migrate-schema     Apply versioned SQL migrations to Postgres"
	@echo "  migrate-down       Rollback the last data migration (one-way, safe no-op)"
	@echo "  migrate-down-schema  Rollback the last applied SQL migration"
	@echo "  chaos              Run chaos tests and export JSON results"
	@echo "  load-test          Run the load test script"
	@echo "  benchmark          Run the benchmark script"

deps:
	go mod download

test:
	go test ./...

build: build-api build-worker build-scheduler build-cli

build-api:
	go build -o bin/api ./cmd/api

build-worker:
	go build -o bin/worker ./cmd/worker

build-scheduler:
	go build -o bin/scheduler ./cmd/scheduler

build-cli:
	go build -o bin/cli ./cmd/cli

run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

run-scheduler:
	go run ./cmd/scheduler

dev: deps
	@echo "[dev] Starting Redis..."
	@-docker run -d -p 6379:6379 --name task_queue_redis redis:7-alpine > /dev/null 2>&1; \
		case $$? in \
			0) echo "Redis started" ;; \
			1) echo "Redis already running on :6379" ;; \
			*) echo "Warning: Redis may not be available" ;; \
		esac
	@echo "[dev] Starting API (logs/api.log)..."
	@mkdir -p logs
	@go run ./cmd/api > logs/api.log 2>&1 & echo $$! > /tmp/tq-api.pid
	@sleep 2
	@echo "[dev] Starting Worker (logs/worker.log)..."
	@go run ./cmd/worker > logs/worker.log 2>&1 & echo $$! > /tmp/tq-worker.pid
	@sleep 1
	@echo "[dev] Starting Scheduler (logs/scheduler.log)..."
	@go run ./cmd/scheduler > logs/scheduler.log 2>&1 & echo $$! > /tmp/tq-scheduler.pid
	@echo "[dev] All services started."
	@echo "       API:       http://localhost:8080"
	@echo "       Worker:    :8081 (health/metrics/CB)"
	@echo "       Scheduler: :8082 (health/metrics)"
	@echo "       Logs:      logs/*.log"
	@echo ""
	@echo "  Run 'make dev-stop' to stop all services."

dev-stop:
	@echo "[dev] Stopping all services..."
	@-kill -TERM $$(cat /tmp/tq-api.pid 2>/dev/null) 2>/dev/null; rm -f /tmp/tq-api.pid
	@-kill -TERM $$(cat /tmp/tq-worker.pid 2>/dev/null) 2>/dev/null; rm -f /tmp/tq-worker.pid
	@-kill -TERM $$(cat /tmp/tq-scheduler.pid 2>/dev/null) 2>/dev/null; rm -f /tmp/tq-scheduler.pid
	@echo "[dev] Stopping Redis..."
	@-docker rm -f task_queue_redis 2>/dev/null \
		&& echo "Redis container removed" \
		|| echo "No Redis container to remove (leaving external Redis running)"
	@sleep 1
	@echo "[dev] All services stopped."

swagger:
	swag init -g cmd/api/main.go

docker-build:
	docker build -t $(APP_NAME) .

docker-up:
	docker-compose -f deploy/local/docker-compose.yml up --build --scale worker=3

docker-down:
	docker-compose -f deploy/local/docker-compose.yml down

migrate:
	./scripts/migrate.sh

migrate-schema:
	go run ./cmd/cli migrate-schema --dir db/migrations

migrate-down:
	go run ./cmd/cli migrate-down

migrate-down-schema:
	go run ./cmd/cli migrate-down-schema --dir db/migrations

chaos:
	./scripts/chaos.sh chaos-report.json

load-test:
	./scripts/load_test.sh

benchmark:
	./scripts/benchmark.sh
