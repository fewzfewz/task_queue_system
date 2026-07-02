# Architecture

## System Overview

The task queue system is composed of four independent binaries that coordinate through Redis:

```
                   ┌──────────┐
                   │  Client  │
                   └────┬─────┘
                        │ POST /jobs
                        ▼
              ┌─────────────────┐
              │  cmd/api        │───► HTTP 8080
              │  (HTTP Server)  │
              └────────┬────────┘
                       │
              ┌────────▼────────┐
              │     Redis       │
              │  ┌──────────┐   │
              │  │ Queue    │   │
              │  │ Store    │   │
              │  │ Metrics  │   │
              │  │ Webhooks │   │
              │  │ Heartbeat│   │
              │  └──────────┘   │
              └──┬────┬────┬───┘
                 │    │    │
    ┌────────────┘    │    └────────────┐
    ▼                 ▼                 ▼
┌──────────┐  ┌──────────────┐  ┌──────────────┐
│ cmd/api  │  │ cmd/worker   │  │ cmd/scheduler │
│ (Store)  │  │ (Executor)   │  │ (Maintenance) │
└──────────┘  └──────┬───────┘  └──────────────┘
                     │
         ┌───────────┴───────────┐
         ▼                       ▼
   ┌──────────┐          ┌──────────────┐
   │ Plugins  │          │ PostgreSQL   │
   │ (email,  │          │ (Optional)   │
   │  image)  │          └──────────────┘
   └──────────┘
```

## Components

### cmd/api — HTTP API Server (port 8080)
- Accepts job submissions (`POST /jobs`)
- Serves browser UI (`GET /`, `/ui`, `/admin/dlq`)
- Exposes metrics and worker health (`GET /metrics`, `/workers`)
- DLQ management endpoints (`/api/v1/dlq/*`)
- Swagger UI at `/swagger/`
- Auth: JWT (RS256) or X-API-Key with Vault/env fallback

### cmd/worker — Job Executor (port 8081)
- Runs N concurrent worker goroutines (configurable via `WORKER_POOL_SIZE`)
- Each worker dequeues jobs from Redis and executes the matching plugin
- Implements retry with exponential backoff (2^retry seconds)
- Dead-letter queue for permanently failed jobs
- Webhook dispatcher for job status callbacks
- Graceful drain on shutdown (finishes in-flight jobs, then exits)
- Heartbeat every 10s, metrics refreshed every 15s

### cmd/scheduler — Maintenance Loop (port 8082)
- Every 1.5s: promotes delayed jobs from ZSET to active queues
- Every 1.5s: reclaims timed-out jobs back to active queues

### cmd/cli — Migration Tool
- `migrate-jobs`: Redis → PostgreSQL job migration
- `migrate-schema`: Applies versioned SQL migrations

## Data Flow

### Job Submission
```
Client ──POST /jobs──► Auth Middleware ──► DTO Validation
    ──► JobService.CreateJob()
        ├── IsAllowed() — tenant rate limit
        ├── Size() — queue capacity check
        ├── store.Save() — persist job record
        └── queue.Enqueue() — push to Redis priority list
```

### Job Execution
```
Worker ──Dequeue()──► RedisQueue (BRPOP, weighted fair dequeue)
    ──► Idempotency Check (Redis SISMEMBER + DB status)
    ──► store.UpdateStatus(processing)
    ──► JobExecutor.Execute()
        ├── Success: MarkProcessed + UpdateResult(completed) + Ack
        └── Failure:
            ├── retries < maxRetries → UpdateStatus(pending) + Enqueue + backoff
            └── retries >= maxRetries → UpdateResult(failed) + Fail(DLQ)
```

## Storage Architecture

### Queue Layer (Redis)
| Structure | Key Pattern | Purpose |
|-----------|-------------|---------|
| List (x3 partitions x3 priorities) | `task_queue:jobs:{high,medium,low}:N` | Pending jobs |
| Sorted Set | `task_queue:in_flight` | In-flight jobs with visibility timeout |
| Hash | `task_queue:payloads` | Full job payloads indexed by ID |
| Sorted Set | `delayed_jobs` | Scheduled jobs keyed by run time |
| List | `task_queue:jobs:dead_letter` | Permanently failed jobs |
| Set | `task_queue:processed` | Completed job IDs (idempotency) |
| Stream | `task_queue:webhooks:stream` | Webhook event stream |
| String | `task_queue:workers:heartbeat:*` | Worker heartbeats with 30s TTL |
| String | `task_queue:tenant:*:rate` | Per-tenant rate limit counters |

### Store Layer (Redis / PostgreSQL / Dual)

The `Store` interface abstracts persistence:

| Method | Redis | PostgreSQL | Dual |
|--------|-------|------------|------|
| Save | HSet JSON | INSERT ON CONFLICT | Both |
| GetByID | HGet | SELECT | Read from primary |
| UpdateStatus | Read-modify-write | UPDATE | Both |
| UpdateResult | Read-modify-write | UPDATE + jsonb_set | Both |
| Dequeue | Stub (use Queue) | SKIP LOCKED | Primary only |
| ListJobs | HVals + filter | SELECT with WHERE | Primary only |

## Priority Queuing

Three priority levels with weighted round-robin dequeue:
- **High** (70%): jobs.priority = "high"
- **Medium** (20%): jobs.priority = "medium"
- **Low** (10%): jobs.priority = "low"

Each priority level has 3 hash-based partitions for concurrency.

## Security Model

- `/healthz`, `/readyz`: No auth (health checks)
- `/`, `/ui`, `/admin/dlq`: No auth (UI pages, API key injected from server)
- `/metrics`: No auth (Prometheus scraping)
- `POST /jobs`: Auth required (X-API-Key header)
- `/api/v1/dlq/*`: Auth required
- `/api/v1/webhooks/*`: Auth required

## Observability

### Metrics (Prometheus)
- `task_queue_job_total` — job count by type/tenant/status
- `task_queue_job_latency_seconds` — execution time histogram
- `task_queue_job_sla_compliance` — SLA tracking (5s target)
- `task_queue_worker_utilization` — active worker gauge
- `task_queue_worker_busy_ratio` — busy/total ratio
- `task_queue_queue_length` — pending count by type/tenant
- `task_queue_webhook_delivery_failures` — webhook failure counter

### Health Endpoints
- `GET /healthz` — liveness (always 200)
- `GET /readyz` — readiness (checks Redis ping)
- `POST /healthz/shutdown` — worker graceful drain trigger (worker only)

## Graceful Shutdown

```
SIGINT/SIGTERM
    │
    ├── API: drain HTTP (10s timeout) → close Redis
    │
    ├── Worker: initiate drain → stop accepting new jobs
    │   └── wait for in-flight jobs (DRAIN_TIMEOUT, default 60s)
    │       └── force exit on timeout
    │
    └── Scheduler: cancel context → stop promotion loop → close Redis
```

## Key Interfaces

```go
// Queue — job lifecycle in Redis
type Queue interface {
    Enqueue, Dequeue, Ack, Fail
    Size, GetFailedJobs, GetMetrics
    RegisterHeartbeat, GetActiveWorkers
    IsProcessed, MarkProcessed
    PromoteScheduledJobs, ReclaimTimedOutJobs
    IsAllowed, PublishWebhookEvent
}

// Store — job record persistence
type Store interface {
    Save, GetByID, UpdateStatus, UpdateResult
    GetByWorkerAndStatus, Enqueue, Dequeue
    Heartbeat, Complete, Fail
    ListJobs, RecoverOrphans
    DeleteJob, DeleteJobsBefore, GetQueueLengths
}

// JobPlugin — worker extension point
type JobPlugin interface {
    Type() string
    Execute(ctx, job) (result, error)
}
```
