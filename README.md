# 🚀 Go Distributed Task Queue

A production-ready, horizontally scalable, and pluggable distributed task queue system built in Go. Designed for high availability, transactional consistency, and architectural flexibility.

---

## 🏗️ Architecture

```mermaid
graph LR
    Client([Client])
    
    subgraph API_Layer ["API Layer"]
        API[HTTP API Server]
    end
    
    subgraph Broker ["Broker & Store"]
        Redis[(Redis)]
    end

    subgraph Scheduler_Service ["Maintenance Engine"]
        SCH[Scheduler]
    end

    subgraph Worker_Pool ["Worker Pool (Scalable)"]
        W1[Worker 1]
        W2[Worker 2]
        W3[Worker N]
    end
    
    subgraph DB_Layer ["Database Layer"]
        DB[(Persistence)]
    end

    Client -->|POST /jobs| API
    API -->|Enqueue| Redis
    SCH -->|Promote/Reclaim| Redis
    Redis --- W1
    Redis --- W2
    Redis --- W3
    W1 --> DB
    W2 --> DB
    W3 --> DB
    
    %% Style notes
    linkStyle 2,3,4 stroke:#f96,stroke-width:2px;
    linkStyle 5,6,7 stroke-dasharray: 5 5;
```

---

## ✨ Key Features

- **🛡️ Atomic Visibility Timeouts**: Uses Redis Sorted Sets (`ZSET`) to ensure no job is ever processed by two workers simultaneously, with automatic reclamation for stalled workers.
- **🔌 Pluggable Architecture**: Add new job types (Email, Image Processing, etc.) without touching the core engine via a dynamic `init()` based plugin registration system.
- **📈 Horizontal Scaling**: Workers are fully stateless. Spin up 1 or 100 instances flawlessly using standard orchestration.
- **🔄 Smart Retries**: Built-in exponential backoff (2ⁿ seconds) to handle transient failures gracefully.
- **📬 Dead Letter Queue (DLQ)**: Failed tasks are preserved with full stack traces/error messages for manual inspection.
- **📊 Real-time Observability**: JSON structured logging, `/metrics` endpoint for Prometheus, and a `/workers` health registry.
- **🛡️ API Security**: Simple but effective X-API-Key middleware protection.

---

## 🛠️ Tech Stack

- **Core**: Go (Golang) 1.22+
- **Broker/Storage**: Redis (Optimised for atomic ops)
- **Database**: PostgreSQL (Persisted Audit Log)
- **Containerisation**: Docker & Docker Compose
- **Observability**: Structured `slog` (JSON), Swagger/OpenAPI

---

## 🚀 Getting Started

### 0. Quick Start (Interactive Demo)
The fastest way to see the system in action:
```bash
./demo.sh
```
This script boots the entire stack, submits immediate and scheduled jobs, and monitors the processing in real-time.

### 1. Run with Docker Compose
One command to start the API, Redis, and a pool of 3 workers:
```bash
docker-compose up --build --scale worker=3
```

### 2. Submit a Job (CLI)
Use our pre-built CLI tool for easy testing:
```bash
./tq submit --type email --payload '{"to":"recruiter@top-tier.com"}'
```

### 3. Submit a Job (cURL)
```bash
curl -X POST http://localhost:8080/jobs \
     -H "X-API-Key: secret-api-key" \
     -H "Content-Type: application/json" \
     -d '{"type":"email","payload":{"to":"user@example.com"},"priority":"high"}'
```

---

## ⚖️ Scaling & Performance

### Horizontal Scalability
This system is architected to be **Shared-Nothing**. Workers do not communicate with each other. They interact solely with the centralized Redis broker using atomic operations (`ZADD`, `LPOP`, `TXPipeline`). 

**To scale processing power:**
1. Simply increase the `replicas` in Kubernetes or the `--scale` flag in Docker Compose.
2. New workers will automatically register their heartbeats and begin competing for available tasks in the queue.
3. No configuration changes are required on the API or existing workers.

### Performance Benchmarking
A high-concurrency stress test script is included:
```bash
# Fire 1000 jobs with 50 concurrent connections
./scripts/load_test.sh 1000 50
```

---

## 📁 Project Structure

- `/cmd`: Entry points for API, Worker, and CLI.
- `/internal/queue`: Core Redis broker implementation with atomic visibility logic.
- `/internal/worker`: Lifecycle management, plugin registry, and processing loops.
- `/internal/jobs`: Extensible job plugin implementations.
- `/internal/api`: HTTP routes, handlers, and middlewares.
- `/docs`: Swagger 2.0 API documentation.

---

## 🛠️ Development

### Generate Swagger Docs
```bash
swag init -g cmd/api/main.go
```

### Run Tests
```bash
go test ./...
```
