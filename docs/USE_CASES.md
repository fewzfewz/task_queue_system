# Task Queue Use Cases & Examples

This document outlines the primary real-world use cases for the Task Queue system, explaining *why* you would use it for these scenarios and providing concrete JSON payload examples to trigger them.

---

## 1. Heavy Computational Work (Offloading)
**The Problem:** Your web application needs to perform CPU-intensive or time-consuming tasks (like generating a PDF invoice or resizing high-resolution user avatars). If you do this synchronously, the user's HTTP request will hang and potentially time out.
**The Solution:** Hand the work to the Task Queue. Your API instantly responds with `202 Accepted`, while a background worker crunches the data.

**Example: HTML to PDF Generation**
```json
POST /api/v1/jobs
{
  "type": "pdf",
  "payload": {
    "url": "https://invoice.example.com/12345",
    "paper_size": "A4"
  },
  "priority": 50
}
```

---

## 2. Unreliable 3rd-Party APIs (Resilience)
**The Problem:** You need to interact with external services (SendGrid for emails, Slack for notifications, OpenAI for AI). External APIs experience downtime, rate limits, and network blips.
**The Solution:** The queue handles external failures gracefully. If an API is down, the **Circuit Breaker** trips to prevent hammering the broken API. Once it recovers, the queue uses **Exponential Backoff** to safely retry the job until it succeeds.

**Example: Sending a Slack Alert**
```json
POST /api/v1/jobs
{
  "type": "slack",
  "payload": {
    "text": "Alert: Database CPU utilization exceeds 90%!"
  },
  "max_retries": 5
}
```

---

## 3. Long-Running Batch Jobs (Progress Tracking)
**The Problem:** You need to export 5 million database rows to a CSV file and upload it to S3. This takes 15 minutes. The user needs to know it hasn't frozen.
**The Solution:** The `data_export` plugin processes data in batches and calls the `ReportProgress()` hook. The user's browser listens via Server-Sent Events (SSE) and displays a smooth progress bar from 0% to 100%.

**Example: Database Export**
```json
POST /api/v1/jobs
{
  "type": "data_export",
  "payload": {
    "format": "csv",
    "query": "SELECT * FROM users WHERE active = true"
  }
}
```

---

## 4. Scheduled & Recurring Tasks (Time-Shifting)
**The Problem:** You need to send a "Welcome" email exactly 3 days after a user signs up, or you need to run a billing script at 12:00 AM on the 1st of every month.
**The Solution:** Use delayed jobs (`run_at`) or recurring jobs (`cron_expr`). The Scheduler daemon holds these jobs in a sleeping state and automatically promotes them to the active queue at the precise execution time.

**Example: Nightly Database Cleanup (Cron)**
```json
POST /api/v1/jobs
{
  "type": "database_cleanup",
  "cron_expr": "0 2 * * *", 
  "payload": {
    "target": "expired_sessions"
  }
}
```

---

## 5. Complex Workflows (DAGs)
**The Problem:** You have a multi-step pipeline. Step C (Upload to S3) absolutely cannot start until Step A (Download Video) and Step B (Compress Video) are completely finished.
**The Solution:** Directed Acyclic Graphs (DAGs). You can pass an array of `dependencies` (Job IDs) when creating a job. The queue will permanently block the downstream job until all upstream dependencies resolve successfully.

**Example: Media Processing Pipeline**
```json
// First, create the upload job dependent on the processing jobs:
POST /api/v1/jobs
{
  "type": "s3_upload",
  "dependencies": ["job_id_download", "job_id_compress"],
  "payload": {
    "bucket": "my-video-bucket",
    "object_key": "user_video.mp4"
  }
}
```

---

## 6. Multi-Tenant SaaS (QoS & Fairness)
**The Problem:** You run a SaaS application. "Client A" does a bulk CSV import that spawns 100,000 background jobs. "Client B" clicks "Reset Password". You cannot let Client A's massive import delay Client B's password reset email.
**The Solution:** Tenant QoS and Priorities. Assign a strict concurrency limit to Client A via the API. The queue will only allow Client A to use a specific number of workers at a time, keeping the rest of the cluster free to instantly process Client B's high-priority jobs.

**Example: High Priority Password Reset with Deduplication**
```json
POST /api/v1/jobs
{
  "type": "email",
  "tenant_id": "client_b_tenant",
  "priority": 100,
  "dedup_key": "pwd_reset_user_789",
  "payload": {
    "to": "user789@example.com",
    "template": "password_reset"
  }
}
```

---

## 7. Map-Reduce (Fan-out / Fan-in)
**The Problem:** You have a massive array of items (like 10,000 URLs to scrape or 1,000 images to process) and want to process them fully in parallel, but need to aggregate all the results at the very end to generate a final report.
**The Solution:** The `map_reduce` plugin automatically shards your array into thousands of individual "Map" jobs. It then creates a single "Reduce" job that uses DAG `dependencies` to automatically pause execution until every single Map job is complete!

**Example: Parallel Image Processing**
```json
POST /api/v1/jobs
{
  "type": "map_reduce",
  "payload": {
    "items": ["img1.png", "img2.png", "img3.png"],
    "map_job_type": "image_compress",
    "reduce_job_type": "generate_summary_report"
  }
}
```
