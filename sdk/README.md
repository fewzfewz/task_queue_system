# Task Queue Polyglot SDKs

The Task Queue System provides native client and worker SDKs in **Go**, **Python**, and **TypeScript**.

---

## 1. Client SDKs (Submitting & Managing Jobs)

Submit and track jobs with just 3 lines of code in your preferred language.

### Python Client

```python
import taskqueue

client = taskqueue.Client(base_url="http://localhost:8080", api_key="your-api-key")
job = client.submit("email", payload={"to": "user@example.com"}, priority="high")
print(f"Submitted job: {job['id']}, Status: {job['status']}")
```

Methods available on `taskqueue.Client`:
- `submit(job_type, payload, priority, max_retries, timeout_seconds, correlation_id, dedup_key, run_at, labels)`
- `submit_batch(jobs)`
- `get_status(job_id)`
- `list_jobs(status, job_type, limit, page, label_key, label_value)`
- `pause(job_id)` / `resume(job_id)` / `cancel(job_id)`

### TypeScript Client

```typescript
import { TaskQueueClient } from './TaskQueueClient';

const client = new TaskQueueClient({ baseURL: 'http://localhost:8080', apiKey: 'your-api-key' });
const job = await client.submit({ type: 'email', payload: { to: 'user@example.com' }, priority: 'high' });
console.log(`Submitted job: ${job.id}, Status: ${job.status}`);
```

Methods available on `TaskQueueClient`:
- `submit(options)`
- `submitBatch(jobs)`
- `getStatus(jobId)`
- `listJobs(filter)`
- `pause(jobId)` / `resume(jobId)` / `cancel(jobId)`

### Go Client

See the [Go Client SDK Documentation](../pkg/tq/README.md).

---

## 2. Worker SDKs (Polyglot External Workers)

Execute heavy compute tasks in environments suited for them (such as running PyTorch models in Python or generating React PDFs in Node.js). The Go engine orchestrates queue pulling, retries, rate limits, and pushes payloads via HTTP.

- [Python Worker SDK](./python/taskqueue.py) (`TaskQueueWorker`) - See `example_worker.py`
- [TypeScript Worker SDK](./typescript/TaskQueueWorker.ts) (`TaskQueueWorker`) - See `example.ts`
