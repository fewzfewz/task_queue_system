# Task Queue SDKs (Polyglot Workers)

While the core Task Queue worker engine is written in Go for maximum performance, you can write actual worker logic in **any language** using our HTTP Push SDKs.

This allows you to easily execute tasks in environments better suited for them, such as running Machine Learning models in Python, or rendering React PDFs in Node.js.

## How it works

Instead of external workers repeatedly polling the queue, our Go worker engine acts as a high-performance orchestrator. It pulls jobs from the Redis queues and **HTTP Pushes** the payload to your external worker. 

The Go engine handles all the complexity:
- Exponential Backoff
- Circuit Breaking (if your Python server goes down)
- Tenant Rate Limiting (QoS)
- Dead Letter Queue routing

Your external worker just receives a POST request, does the work, and returns a JSON response.

## Available SDKs

- [Python Worker SDK](./python/taskqueue.py) - See `example_worker.py`
- [TypeScript Worker SDK](./typescript/TaskQueueWorker.ts) - See `example.ts`
- [Go Client SDK](../pkg/tq/README.md) (For submitting jobs)

## Registering Remote Jobs

To send a job to a Python worker listening on port `5050` for the `ml_inference` route, you submit a job of type `http`:

```json
POST /api/v1/jobs
{
  "type": "http",
  "payload": {
    "url": "http://python-worker:5050/ml_inference",
    "method": "POST",
    "body": {
      "text": "I love this queue!"
    }
  },
  "max_retries": 3
}
```

The Go engine will robustly deliver this payload to your Python worker and capture the result!
