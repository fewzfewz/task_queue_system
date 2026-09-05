package handler

import (
	"net/http"
)

func (h *JobHandler) ServeFeaturesPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(featuresHTML))
}

func (h *JobHandler) ServeDocsPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(docsHTML))
}

func (h *JobHandler) ServeSDKPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(sdkHTML))
}

const sharedHeader = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>TaskQueue</title>
  <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@300;400;500;600;700;800;900&family=Inter:wght@400;500&display=swap" rel="stylesheet">
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css">
  <style>
    *{margin:0;padding:0;box-sizing:border-box}
    :root{
      --bg:#05080f;--bg2:#0a0f1e;--surface:rgba(26,34,54,0.5);--surface2:rgba(31,42,64,0.8);
      --border:rgba(255,255,255,0.07);--border-h:rgba(255,255,255,0.14);
      --text:#f8fafc;--muted:#94a3b8;--muted2:#64748b;
      --accent:#6366f1;--accent2:#818cf8;--accent3:#c084fc;--accent4:#38bdf8;
      --good:#34d399;--bad:#f87171;--warn:#fbbf24;
    }
    body{font-family:'Outfit','Inter',sans-serif;background:var(--bg);color:var(--text);min-height:100vh;overflow-x:hidden}
    ::-webkit-scrollbar{width:6px}
    ::-webkit-scrollbar-thumb{background:rgba(99,102,241,.4);border-radius:3px}

    /* ── Ambient background ── */
    body::before{
      content:'';position:fixed;inset:0;z-index:-1;
      background:
        radial-gradient(ellipse 80% 60% at 20% -10%,rgba(99,102,241,.18) 0%,transparent 60%),
        radial-gradient(ellipse 60% 50% at 85% 10%,rgba(192,132,252,.12) 0%,transparent 55%),
        radial-gradient(ellipse 50% 40% at 50% 90%,rgba(56,189,248,.08) 0%,transparent 50%);
    }

    /* ── Navbar ── */
    nav{
      position:fixed;top:0;left:0;right:0;z-index:100;
      display:flex;align-items:center;justify-content:space-between;
      padding:0 clamp(20px,5vw,80px);height:68px;
      background:rgba(5,8,15,0.7);backdrop-filter:blur(20px);
      border-bottom:1px solid var(--border);
    }
    .nav-brand{display:flex;align-items:center;gap:12px;text-decoration:none}
    .nav-logo{
      width:38px;height:38px;border-radius:10px;
      background:linear-gradient(135deg,var(--accent),var(--accent3));
      display:flex;align-items:center;justify-content:center;
      box-shadow:0 6px 20px rgba(99,102,241,.4);
    }
    .nav-logo i{color:#fff;font-size:17px}
    .nav-brand h1{font-size:18px;font-weight:700;color:var(--text)}
    .nav-links{display:flex;align-items:center;gap:8px}
    .nav-links a{
      color:var(--muted);text-decoration:none;font-size:14px;font-weight:500;
      padding:8px 14px;border-radius:8px;transition:all .2s;
    }
    .nav-links a:hover{color:var(--text);background:rgba(255,255,255,.06)}
    .nav-cta{
      background:linear-gradient(135deg,var(--accent),var(--accent3));
      color:#fff !important;border-radius:10px !important;
      box-shadow:0 4px 16px rgba(99,102,241,.35);
    }
    .nav-cta:hover{opacity:.9;transform:translateY(-1px)}
    
    .page-container {
        padding: 120px clamp(20px, 5vw, 80px) 80px;
        max-width: 1000px;
        margin: 0 auto;
    }
    .page-container h1 { font-size: 42px; font-weight: 800; margin-bottom: 24px; color: var(--text); }
    .page-container h2 { font-size: 28px; font-weight: 700; margin-top: 48px; margin-bottom: 16px; color: var(--accent2); }
    .page-container p { font-size: 16px; color: var(--muted); margin-bottom: 24px; line-height: 1.7; }
    .page-container ul { margin-left: 24px; margin-bottom: 24px; color: var(--muted); line-height: 1.7; }
    .page-container li { margin-bottom: 8px; }
    .page-container pre { background: var(--surface2); padding: 16px; border-radius: 8px; margin-bottom: 24px; overflow-x: auto; color: var(--text); border: 1px solid var(--border); }
    .page-container code { font-family: 'SF Mono', monospace; font-size: 14px; }
    .btn { display: inline-block; background: var(--surface2); color: var(--text); padding: 10px 20px; text-decoration: none; border-radius: 8px; border: 1px solid var(--border); transition: all .2s; }
    .btn:hover { background: rgba(255,255,255,0.1); border-color: var(--border-h); }
  </style>
</head>
<body>
<nav>
  <a class="nav-brand" href="/">
    <div class="nav-logo"><i class="fas fa-bolt"></i></div>
    <h1>TaskQueue</h1>
  </a>
  <div class="nav-links">
    <a href="/features">Features</a>
    <a href="/docs">Docs</a>
    <a href="/docs/sdk">Dev SDK</a>
    <a href="/client/login">Sign In</a>
    <a href="/#register" class="nav-cta">Get Started</a>
  </div>
</nav>
`

const featuresHTML = sharedHeader + `
<div class="page-container">
    <h1>Features</h1>
    <p>TaskQueue is packed with enterprise-grade features designed to keep your background jobs running smoothly.</p>

    <h2>Priority Queues</h2>
    <p>Not all jobs are created equal. TaskQueue uses a three-tier priority system (high, medium, low) to ensure that critical jobs like password resets are always processed before bulk operations like daily reports.</p>

    <h2>Automatic Retries & Exponential Backoff</h2>
    <p>Network glitches happen. If a job fails, TaskQueue automatically retries it using an exponential backoff algorithm. This prevents overwhelming downstream services while ensuring transient errors are handled gracefully.</p>

    <h2>Circuit Breakers</h2>
    <p>Protect your system from cascading failures. If a specific plugin or webhook endpoint starts failing consistently, TaskQueue's circuit breakers trip, temporarily halting jobs of that type until the service recovers.</p>

    <h2>Dead Letter Queues (DLQ)</h2>
    <p>Jobs that repeatedly fail and exhaust their retry limits are safely moved to a Dead Letter Queue. Operators can inspect these jobs, fix the underlying issues, and replay them without losing the original payload.</p>

    <h2>Concurrency Limits</h2>
    <p>Throttle your outbound requests. Configure maximum concurrency limits per job type to ensure you never exceed API rate limits or overwhelm your database connections.</p>
    
    <div style="margin-top: 40px;">
        <a href="/#register" class="btn">Get Started Today</a>
    </div>
</div>
</body>
</html>`

const docsHTML = sharedHeader + `
<div class="page-container">
    <h1>Documentation</h1>
    <p>Welcome to the official TaskQueue documentation. This guide explains how the system operates and how to integrate it into your workflow.</p>

    <h2>Core Concepts</h2>
    <ul>
        <li><strong>Tenant ID:</strong> A unique identifier for your organization. All jobs and metrics are isolated per tenant.</li>
        <li><strong>Job Type:</strong> Categorizes your tasks (e.g., <code>email</code>, <code>webhook</code>). You can configure specific concurrency limits and retry policies per Job Type.</li>
        <li><strong>Payload:</strong> The JSON data provided when enqueuing a job. This is passed directly to the worker processing the job.</li>
    </ul>

    <h2>The Job Lifecycle</h2>
    <p>When you enqueue a job, it enters the <code>pending</code> state. The Scheduler promotes it to the active queue when it's due. A Worker picks it up, transitioning it to <code>processing</code>. If successful, it becomes <code>completed</code>. If it fails, it is marked as <code>failed</code> and retried based on your policy.</p>

    <h2>Using the API</h2>
    <p>You can manage jobs using our REST API. You must include your API Key in the <code>X-API-Key</code> header for all requests.</p>
    
    <pre><code>curl -X POST https://your-taskqueue.com/api/v1/jobs \\
  -H "X-API-Key: YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "type": "email",
    "payload": {"to": "user@example.com"}
  }'</code></pre>

    <h2>More Resources</h2>
    <ul>
        <li><a href="/swagger/" style="color:var(--accent2)">Swagger API Reference</a> - Interactive endpoint testing.</li>
        <li><a href="/docs/sdk" style="color:var(--accent2)">Developer SDK Guide</a> - Native integration for Go developers.</li>
    </ul>
</div>
</body>
</html>`

const sdkHTML = sharedHeader + `
<div class="page-container">
    <h1>Developer SDK</h1>
    <p>For Go developers, we provide a native SDK (<code>pkg/tq</code>) that makes interacting with TaskQueue completely seamless.</p>

    <h2>Installation</h2>
    <pre><code>import "github.com/your-org/task-queue-system/pkg/tq"</code></pre>

    <h2>Initialization</h2>
    <p>Initialize the client using your server URL and API Key:</p>
    <pre><code>client := tq.NewClient("https://your-taskqueue.com", "YOUR_API_KEY")</code></pre>

    <h2>Enqueuing Jobs</h2>
    <p>Enqueue a job by providing the type and a payload. You can optionally specify a priority and timeout.</p>
    <pre><code>job, err := client.Enqueue(context.Background(), tq.EnqueueRequest{
    Type:     "webhook",
    Payload:  map[string]interface{}{"url": "https://api.example.com"},
    Priority: 50,
    Timeout:  30,
})</code></pre>

    <h2>Checking Job Status</h2>
    <p>Retrieve the current status and progress of any job:</p>
    <pre><code>status, err := client.GetJob(context.Background(), job.ID)
fmt.Printf("Status: %s, Progress: %d%%\\n", status.Status, status.Progress)</code></pre>

    <h2>Canceling Jobs</h2>
    <p>Cancel a long-running job mid-flight. TaskQueue uses realtime Pub/Sub to kill the execution instantly.</p>
    <pre><code>err := client.CancelJob(context.Background(), job.ID)</code></pre>
</div>
</body>
</html>`
