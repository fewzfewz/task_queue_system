APP_NAME=task-queue-system

.PHONY: help deps test build build-api build-worker build-scheduler build-cli run-api run-worker run-scheduler swagger docker-build docker-up docker-down migrate migrate-schema chaos load-test benchmark

help:
	@echo "Targets:"
	@echo "  deps            Download Go module dependencies"
	@echo "  test            Run the test suite"
	@echo "  build           Build all binaries"
	@echo "  run-api         Run the API locally"
	@echo "  run-worker      Run the worker locally"
	@echo "  run-scheduler   Run the scheduler locally"
	@echo "  swagger         Regenerate Swagger docs"
	@echo "  docker-build    Build the Docker image"
	@echo "  docker-up       Start the local Docker Compose stack"
	@echo "  docker-down     Stop the local Docker Compose stack"
	@echo "  migrate         Run the Redis -> Postgres migration CLI"
	@echo "  migrate-schema  Apply versioned SQL migrations to Postgres"
	@echo "  chaos           Run chaos tests and export JSON results"
	@echo "  load-test       Run the load test script"
	@echo "  benchmark       Run the benchmark script"

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

swagger:
	swag init -g cmd/api/main.go

docker-build:
	docker build -t $(APP_NAME) .

docker-up:
	docker compose -f deploy/local/docker-compose.yml up --build --scale worker=3

docker-down:
	docker compose -f deploy/local/docker-compose.yml down

migrate:
	./scripts/migrate.sh

migrate-schema:
	go run ./cmd/cli migrate-schema --dir db/migrations

chaos:
	./scripts/chaos.sh chaos-report.json

load-test:
	./scripts/load_test.sh

benchmark:
	./scripts/benchmark.sh
