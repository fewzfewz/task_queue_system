# TaskQueue Client Integration Guide

Welcome to TaskQueue! This guide will show you how to integrate our background job processing engine into your applications.

TaskQueue is a resilient, priority-aware background job engine. By using TaskQueue, you can decouple slow, heavy, or failure-prone tasks (like sending emails, processing payments, or generating PDFs) from your main web application.

---

## 1. Getting Started

Before you can enqueue jobs, you need a **Tenant ID** and an **API Key**.

1. Visit the TaskQueue landing page at `/`.
2. Enter a unique Tenant ID (e.g., `acme-corp`) and click **Create Account & Get API Key**.
3. Save your generated API Key securely. You will use this key to authenticate all requests to the TaskQueue API.

---

## 2. Using the Native Go SDK

If your application is written in Go, the easiest way to integrate with TaskQueue is using our native SDK.

### Installation
Import the SDK into your project:
```go
import "github.com/your-org/task-queue-system/pkg/tq"
```

### Initialization
Create a new TaskQueue client using your API Key and the server URL:

```go
client := tq.NewClient("http://localhost:8080", "YOUR_API_KEY")
```

### Enqueuing Jobs
To enqueue a job, you just need to specify the job `Type` and provide a JSON-serializable `Payload`. You can optionally configure priority and timeouts.

```go
package main

import (
    "context"
    "log"
    "github.com/your-org/task-queue-system/pkg/tq"
)

func main() {
    client := tq.NewClient("http://localhost:8080", "YOUR_API_KEY")

    job, err := client.Submit(context.Background(), tq.SubmitJobRequest{
        Type:     "email",                     // The type of job
        Payload:  map[string]interface{}{      // The data your worker needs
            "to": "user@example.com",
            "subject": "Welcome!",
            "body": "Thanks for signing up.",
        },
        DedupKey: "welcome_email_user_123",    // Optional: prevents duplicate enqueues (returns 409 Conflict if already running/pending)
        Priority: 10,                          // 1 (low) to 100 (high)
        Timeout:  30,                          // Maximum execution time in seconds
    })

    if err != nil {
        log.Fatalf("Failed to enqueue job: %v", err)
    }

    log.Printf("Successfully enqueued job! ID: %s", job.ID)
    
    // You can also bulk-insert thousands of jobs in a single request:
    // batch, err := client.SubmitBatch(context.Background(), []tq.SubmitJobRequest{...})
}
```

### Checking Job Status
You can poll the API to see if your job has finished, or check its progress:

```go
status, err := client.GetJob(context.Background(), job.ID)
if err != nil {
    log.Fatal(err)
}

log.Printf("Job Status: %s (Progress: %d%%)", status.Status, status.Progress)
if status.Status == "completed" {
    log.Printf("Result: %v", status.Result)
}
```

### Canceling Jobs
If you enqueue a long-running job by mistake, you can cancel it mid-flight:
```go
err := client.CancelJob(context.Background(), job.ID)
```
TaskQueue uses a realtime Pub/Sub system to instantly send a kill signal to whichever worker is executing your job.

---

## 3. Using the REST API (Any Language)

If you aren't using Go, you can interact with TaskQueue using standard HTTP requests.

**Authentication:** Include your API key in the `X-API-Key` header.

### Enqueue a Job (POST /api/v1/jobs)
```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "X-API-Key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "type": "webhook",
    "payload": {
      "url": "https://api.example.com/callback",
      "method": "POST"
    },
    "priority": 50,
    "timeout": 60
  }'
```

### Check Job Status (GET /api/v1/jobs/{id})
```bash
curl http://localhost:8080/api/v1/jobs/job_123456789 \
  -H "X-API-Key: YOUR_API_KEY"
```

### Cancel a Job (POST /api/v1/jobs/{id}/cancel)
```bash
curl -X POST http://localhost:8080/api/v1/jobs/job_123456789/cancel \
  -H "X-API-Key: YOUR_API_KEY"
```

---

## 4. How Things Work Under the Hood

### Priorities
Jobs are assigned a priority between `1` and `100` (default is `10`). A job with priority `100` will *always* be processed before a job with priority `10`, allowing you to fast-track critical tasks (like password resets) ahead of bulk tasks (like nightly analytics).

### Automatic Retries
If your job fails (e.g., a third-party API is down), TaskQueue automatically retries it using an **exponential backoff** algorithm. It will try again in 5 seconds, then 25 seconds, then 2 minutes, etc.

### Dead Letter Queue (DLQ)
If a job repeatedly fails and exhausts its maximum retry count (default is 5), it is marked as `failed` and moved to the **Dead Letter Queue**. Administrators can review these failed jobs in the UI, fix the underlying bug in the code, and click "Replay" to try processing them again without losing the original payload.

### Concurrency Limits
To prevent your jobs from overwhelming downstream services (like a database or a rate-limited API), you can configure maximum concurrency limits per Job Type in the Admin UI. If you set the limit for the `email` queue to `5`, TaskQueue will never process more than 5 emails concurrently across the entire cluster. Additional jobs are gracefully deferred until a slot opens up.

---

## 5. Client Dashboard

You can monitor your tenant's jobs, queue health, and metrics using the built-in Client Dashboard.
Navigate to `/client/login` and log in using your API Key to access your private dashboard.

Happy Queuing!

### Exactly-Once Deduplication (Unique Jobs)
To prevent creating duplicate jobs, you can provide a `dedup_key`.
```json
{
  "type": "process_payment",
  "payload": {"amount": 100},
  "dedup_key": "tx_abc123"
}
```
If a job with the same `dedup_key` is already pending, processing, or recently completed, the API will reject the duplicate request and return a `409 Conflict` HTTP status.

### Webhooks (Event Callbacks)
Instead of polling the API to check job status, you can configure Webhooks to have TaskQueue HTTP POST to your backend when jobs transition to specific states (`created`, `completed`, `failed`).

```bash
curl -X POST http://localhost:8080/api/v1/webhooks \
  -H "X-API-Key: YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://api.example.com/task-queue-events",
    "events": ["completed", "failed"]
  }'
```

Webhooks include an `X-Webhook-Signature` HMAC header so you can cryptographically verify the payload originated from your TaskQueue tenant.

### Tenant Rate Limiting (QoS)
To prevent your jobs from causing "noisy neighbor" starvation across the cluster, administrators can assign maximum concurrency limits per Tenant. If your tenant submits thousands of jobs instantly, TaskQueue will cap the number of active workers processing your tenant's jobs to your configured limit. Additional jobs are gracefully held in a deferred state and automatically promoted as capacity frees up.
