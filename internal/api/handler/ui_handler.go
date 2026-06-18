package handler

import (
	"fmt"
	"net/http"
)

// ServeAppUI renders the general operator UI for the queue system.
func (h *JobHandler) ServeAppUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, appHTML)
}

const appHTML = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Task Queue System</title>
  <style>
    :root { --bg:#0b1020; --panel:#121a33; --panel2:#182241; --text:#e8ecff; --muted:#9aa7d6; --line:#2a355f; --accent:#7c9cff; --good:#3ddc97; --bad:#ff6b81; --warn:#ffc857; }
    * { box-sizing:border-box; }
    body { margin:0; font-family: Inter, Arial, sans-serif; background: radial-gradient(circle at top, #15203f, var(--bg)); color:var(--text); }
    header { padding:20px 24px; border-bottom:1px solid var(--line); display:flex; justify-content:space-between; align-items:center; backdrop-filter: blur(8px); position:sticky; top:0; background:rgba(11,16,32,.85); z-index:10; }
    h1 { margin:0; font-size:20px; }
    main { padding:24px; display:grid; gap:20px; max-width:1400px; margin:0 auto; }
    .grid { display:grid; gap:16px; }
    .cards { grid-template-columns: repeat(auto-fit,minmax(180px,1fr)); }
    .card { background: linear-gradient(180deg, rgba(255,255,255,.03), rgba(255,255,255,.015)); border:1px solid var(--line); border-radius:16px; padding:16px; box-shadow:0 18px 40px rgba(0,0,0,.22); }
    .label { font-size:12px; color:var(--muted); text-transform:uppercase; letter-spacing:.08em; }
    .value { font-size:28px; margin-top:8px; font-weight:700; }
    .muted { color:var(--muted); font-size:13px; }
    input, select, textarea, button { width:100%; border-radius:12px; border:1px solid var(--line); background:var(--panel); color:var(--text); padding:12px 14px; font-size:14px; }
    textarea { min-height:100px; resize:vertical; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
    button { cursor:pointer; background:linear-gradient(180deg, #89a5ff, #5f7cff); color:#07101f; font-weight:700; border:none; }
    button.secondary { background:var(--panel2); color:var(--text); border:1px solid var(--line); }
    button.danger { background:linear-gradient(180deg, #ff8fa1, #ff5f7d); color:#1b0710; }
    .row { display:grid; gap:10px; margin-top:12px; }
    .row.cols2 { grid-template-columns:1fr 1fr; }
    .row.cols3 { grid-template-columns:1fr 1fr 1fr; }
    .row.cols4 { grid-template-columns:1fr 1fr 1fr 1fr; }
    pre { margin:0; white-space:pre-wrap; word-break:break-word; background:#081022; border:1px solid var(--line); border-radius:12px; padding:12px; max-height:320px; overflow:auto; font-size:13px; }
    table { width:100%; border-collapse:collapse; font-size:14px; }
    th, td { text-align:left; padding:10px 8px; border-bottom:1px solid rgba(42,53,95,.7); vertical-align:top; }
    th { color:var(--muted); font-size:12px; text-transform:uppercase; letter-spacing:.08em; }
    .pill { display:inline-block; padding:4px 8px; border-radius:999px; background:rgba(124,156,255,.14); color:#bfd0ff; font-size:12px; }
    .pill.good { background:rgba(61,220,151,.14); color:#9ef0c8; }
    .pill.bad { background:rgba(255,107,129,.14); color:#ffb1be; }
    .pill.warn { background:rgba(255,200,87,.14); color:#ffe499; }
    .toolbar { display:flex; gap:10px; flex-wrap:wrap; }
    .toolbar > * { flex:1 1 220px; }
    .split { display:grid; gap:16px; grid-template-columns: repeat(auto-fit,minmax(340px,1fr)); }
    .tabs { display:flex; gap:10px; flex-wrap:wrap; }
    .tab { flex:0 0 auto; padding:10px 14px; border-radius:999px; border:1px solid var(--line); background:var(--panel2); color:var(--text); cursor:pointer; font-size:13px; }
    .tab.active { background:linear-gradient(180deg, #89a5ff, #5f7cff); color:#07101f; border-color:transparent; }
    .panel { display:none; }
    .panel.active { display:block; }
    a { color:var(--accent); }
    .pagination { display:flex; gap:8px; align-items:center; margin-top:10px; }
    .pagination button { flex:0; width:auto; padding:6px 14px; }
    .badge { display:inline-block; width:10px; height:10px; border-radius:50%; margin-right:6px; }
    .badge.green { background:var(--good); }
    .badge.red { background:var(--bad); }
    .badge.yellow { background:var(--warn); }
    .toast { position:fixed; bottom:24px; right:24px; background:var(--panel2); border:1px solid var(--line); border-radius:12px; padding:12px 18px; font-size:14px; opacity:0; transition:opacity .3s; pointer-events:none; z-index:99; }
    .toast.show { opacity:1; }
  </style>
</head>
<body>
  <div id="toast" class="toast"></div>
  <header>
    <div>
      <h1>Task Queue System</h1>
      <div class="muted">Operator dashboard — jobs, workers, stats, DAG, circuit breaker, webhooks</div>
    </div>
    <div class="toolbar" style="max-width:720px;">
      <select id="auth-mode" onchange="saveAuthMode()">
        <option value="api-key">API Key</option>
        <option value="bearer">Bearer Token</option>
      </select>
      <input id="token" placeholder="Paste API key or JWT" />
      <button class="secondary" onclick="saveToken()">Save token</button>
      <button class="secondary" onclick="loadEverything()">Refresh all</button>
    </div>
  </header>

  <main>
    <!-- ── Stats Cards ── -->
    <section class="grid cards">
      <div class="card"><div class="label">Status</div><div id="stat-status" class="value">-</div><div class="muted">API reachable</div></div>
      <div class="card"><div class="label">Pending Jobs</div><div id="stat-pending" class="value">-</div><div class="muted">Total queued work</div></div>
      <div class="card"><div class="label">Workers</div><div id="stat-workers" class="value">-</div><div class="muted">Active heartbeats</div></div>
      <div class="card"><div class="label">DLQ</div><div id="stat-dlq" class="value">-</div><div class="muted">Failed jobs</div></div>
      <div class="card"><div class="label">Circuit Breakers</div><div id="stat-cb" class="value">0</div><div class="muted">Open / monitored</div></div>
    </section>

    <!-- ── Action Bar ── -->
    <section class="card">
      <div class="toolbar">
        <button onclick="fillExample()">Example email job</button>
        <button class="secondary" onclick="loadStats()">Stats</button>
        <button class="secondary" onclick="loadWorkers()">Workers</button>
        <button class="secondary" onclick="loadMetrics()">Metrics</button>
        <button class="secondary" onclick="loadCircuitBreakers()">Circuit Breakers</button>
        <button class="secondary" onclick="loadWebhooks()">Webhooks</button>
      </div>
    </section>

    <!-- ── Tabs ── -->
    <section class="card">
      <div class="tabs">
        <button class="tab active" data-tab="jobs" onclick="showTab('jobs')">Jobs</button>
        <button class="tab" data-tab="stats" onclick="showTab('stats');loadStats()">Stats</button>
        <button class="tab" data-tab="dag" onclick="showTab('dag')">DAG</button>
        <button class="tab" data-tab="ops" onclick="showTab('ops')">Workers / Health</button>
        <button class="tab" data-tab="cb" onclick="showTab('cb');loadCircuitBreakers()">Circuit Breaker</button>
        <button class="tab" data-tab="dlq" onclick="showTab('dlq')">DLQ</button>
        <button class="tab" data-tab="webhooks" onclick="showTab('webhooks');loadWebhooks()">Webhooks</button>
      </div>
    </section>

    <!-- ── Panel: Jobs ── -->
    <section id="panel-jobs" class="panel active">
      <div class="split">
        <div class="card">
          <h2>Create Job</h2>
          <div class="row cols2">
            <input id="create-type" placeholder="type e.g. email" value="email" />
            <input id="create-priority" placeholder="priority" value="medium" />
          </div>
          <div class="row cols2">
            <input id="create-tenant" placeholder="tenant_id" />
            <input id="create-dedup" placeholder="dedup_key (optional)" />
          </div>
          <div class="row cols2">
            <input id="create-corr" placeholder="correlation_id" />
            <input id="create-shard" placeholder="shard_key (optional)" />
          </div>
          <div class="row cols2">
            <input id="create-retries" placeholder="max_retries" value="3" />
            <input id="create-timeout" placeholder="timeout seconds" value="60" />
          </div>
          <div class="row cols2">
            <input id="create-runat" placeholder="run_at RFC3339" />
            <input id="create-cron" placeholder="cron expr (e.g. */5 * * * *)" />
          </div>
          <textarea id="create-payload">{ "to": "user@example.com", "subject": "hello" }</textarea>
          <div class="row cols2">
            <button onclick="createJob()">Create job</button>
            <button class="secondary" onclick="fillExample()">Fill email example</button>
          </div>
        </div>

        <div class="card">
          <h2>Search / List Jobs</h2>
          <div class="row cols2">
            <input id="job-search-status" placeholder="status filter" />
            <input id="job-search-type" placeholder="type filter" />
          </div>
          <div class="row cols2">
            <input id="job-search-tenant" placeholder="tenant_id" />
            <input id="job-search-limit" placeholder="limit" value="10" />
          </div>
          <div class="row cols2">
            <button onclick="searchJobs(1)">Search</button>
            <button onclick="getJobByID()">Get by ID</button>
          </div>
          <input id="job-search-id" placeholder="or enter job ID directly" />
          <div class="pagination" id="job-pagination"></div>
          <pre id="job-output">No results yet.</pre>
        </div>
      </div>
    </section>

    <!-- ── Panel: Stats ── -->
    <section id="panel-stats" class="panel">
      <div class="split">
        <div class="card">
          <h2>Queue Breakdown</h2>
          <pre id="stats-queue">Loading…</pre>
        </div>
        <div class="card">
          <h2>Overview</h2>
          <pre id="stats-overview">Loading…</pre>
        </div>
      </div>
    </section>

    <!-- ── Panel: DAG ── -->
    <section id="panel-dag" class="panel">
      <div class="card">
        <h2>DAG Dependency Inspector</h2>
        <div class="row cols2">
          <input id="dag-job-id" placeholder="Job ID" />
          <button onclick="loadDAG()">Inspect</button>
        </div>
        <div class="split">
          <div>
            <div class="label">Depends On (upstream)</div>
            <pre id="dag-upstream">Enter a job ID above.</pre>
          </div>
          <div>
            <div class="label">Dependents (downstream)</div>
            <pre id="dag-downstream">Jobs that list this ID as a dependency appear here.</pre>
          </div>
        </div>
      </div>
    </section>

    <!-- ── Panel: Ops ── -->
    <section id="panel-ops" class="panel">
      <div class="split">
        <div class="card">
          <h2>Workers</h2>
          <button onclick="loadWorkers()">Refresh workers</button>
          <table><thead><tr><th>ID</th><th>Last Heartbeat</th></tr></thead><tbody id="workers-body"><tr><td colspan="2" class="muted">No workers loaded.</td></tr></tbody></table>
        </div>
        <div class="card">
          <h2>Health & Metrics</h2>
          <div class="row cols2">
            <button onclick="checkHealth('/healthz')">/healthz</button>
            <button onclick="checkHealth('/readyz')">/readyz</button>
          </div>
          <button onclick="loadMetrics()">Load Prometheus metrics</button>
          <div class="row cols2">
            <div><div class="label">Health</div><pre id="health-output">Click a health check.</pre></div>
            <div><div class="label">Metrics</div><pre id="metrics-output">Prometheus metrics appear here.</pre></div>
          </div>
        </div>
      </div>
    </section>

    <!-- ── Panel: Circuit Breaker ── -->
    <section id="panel-cb" class="panel">
      <div class="card">
        <h2>Circuit Breaker Status</h2>
        <p class="muted">Monitored by the worker process (port 8081). Open breakers block execution.</p>
        <button onclick="loadCircuitBreakers()">Refresh</button>
        <table><thead><tr><th>Plugin Type</th><th>Status</th><th>Actions</th></tr></thead><tbody id="cb-body"><tr><td colspan="3" class="muted">Loading…</td></tr></tbody></table>
      </div>
    </section>

    <!-- ── Panel: DLQ ── -->
    <section id="panel-dlq" class="panel">
      <div class="card">
        <h2>Dead Letter Queue</h2>
        <div class="toolbar">
          <input id="dlq-queue" placeholder="queue filter" />
          <input id="dlq-tenant" placeholder="tenant filter" />
          <button onclick="loadDLQ()">Load DLQ</button>
        </div>
        <div class="row cols2">
          <input id="dlq-search" placeholder="search text" />
          <button class="secondary" onclick="searchDLQ()">Search</button>
        </div>
        <div class="row cols2">
          <input id="dlq-replay-id" placeholder="job id to replay" />
          <button class="secondary" onclick="replayDLQ()">Replay</button>
        </div>
        <div class="row cols2">
          <input id="dlq-purge-id" placeholder="job id to delete" />
          <button class="danger" onclick="deleteDLQ()">Delete</button>
        </div>
        <div class="row cols2">
          <input id="dlq-older" placeholder="older than RFC3339" />
          <button class="danger" onclick="bulkPurgeDLQ()">Bulk purge</button>
        </div>
        <div class="row cols2">
          <button class="secondary" onclick="exportDLQ()">Export JSON</button>
          <button class="secondary" onclick="loadDLQ()">Reload</button>
        </div>
        <pre id="dlq-output">No DLQ items loaded yet.</pre>
      </div>
    </section>

    <!-- ── Panel: Webhooks ── -->
    <section id="panel-webhooks" class="panel">
      <div class="split">
        <div class="card">
          <h2>Register Webhook</h2>
          <input id="wh-url" placeholder="https://example.com/hook" />
          <input id="wh-secret" placeholder="signing secret" />
          <input id="wh-events" placeholder="events (comma-sep: completed,failed)" />
          <button onclick="registerWebhook()">Register</button>
        </div>
        <div class="card">
          <h2>Your Webhooks</h2>
          <button class="secondary" onclick="loadWebhooks()">Refresh</button>
          <pre id="wh-output">No webhooks loaded.</pre>
        </div>
      </div>
    </section>
  </main>

  <script>
    const API_BASE = '/api/v1';
    const TOKEN_KEY = 'task_queue_token';
    const WORKER_PORT = '8081';

    function toast(msg) { const t=document.getElementById('toast'); t.textContent=msg; t.classList.add('show'); setTimeout(()=>t.classList.remove('show'),2500); }

    function saveToken() {
      localStorage.setItem(TOKEN_KEY, document.getElementById('token').value.trim());
      toast('Token saved');
    }
    function saveAuthMode() {
      localStorage.setItem('task_queue_auth_mode', document.getElementById('auth-mode').value);
      toast('Auth mode saved');
    }
    function getToken() {
      const t = localStorage.getItem(TOKEN_KEY) || '';
      document.getElementById('token').value = t;
      document.getElementById('auth-mode').value = localStorage.getItem('task_queue_auth_mode') || 'api-key';
      return t;
    }
    function headers() {
      const token = (localStorage.getItem(TOKEN_KEY) || '').trim();
      const mode = localStorage.getItem('task_queue_auth_mode') || 'api-key';
      const hdr = { 'Content-Type': 'application/json' };
      if (token) {
        if (mode === 'bearer') hdr['Authorization'] = token.startsWith('Bearer ') ? token : 'Bearer ' + token;
        else if (token.startsWith('Bearer ')) hdr['Authorization'] = token;
        else if (token.includes('.') && token.split('.').length === 3) hdr['Authorization'] = 'Bearer ' + token;
        else hdr['X-API-Key'] = token;
      }
      return hdr;
    }
    function flash(msg) { document.getElementById('stat-status').innerText = msg; }
    function showTab(name) {
      document.querySelectorAll('.tab').forEach(btn => btn.classList.toggle('active', btn.dataset.tab === name));
      document.querySelectorAll('.panel').forEach(panel => panel.classList.remove('active'));
      const p = document.getElementById('panel-'+name);
      if (p) p.classList.add('active');
      localStorage.setItem('task_queue_tab', name);
    }
    async function api(path, options = {}) {
      const res = await fetch(path, { ...options, headers: { ...headers(), ...(options.headers || {}) } });
      if (res.status === 204) return null;
      const text = await res.text();
      try { return JSON.parse(text); } catch (_) { return text; }
    }

    // ── Create Job ──
    function fillExample() {
      document.getElementById('create-type').value = 'email';
      document.getElementById('create-payload').value = JSON.stringify({to:'user@example.com', subject:'hello'}, null, 2);
    }
    async function createJob() {
      const body = {
        type: document.getElementById('create-type').value.trim() || 'email',
        priority: document.getElementById('create-priority').value.trim() || 'medium',
        tenant_id: document.getElementById('create-tenant').value.trim(),
        correlation_id: document.getElementById('create-corr').value.trim(),
        dedup_key: document.getElementById('create-dedup').value.trim(),
        shard_key: document.getElementById('create-shard').value.trim(),
        run_at: document.getElementById('create-runat').value.trim(),
        cron_expr: document.getElementById('create-cron').value.trim(),
        timeout: parseInt(document.getElementById('create-timeout').value || '60', 10),
        max_retries: parseInt(document.getElementById('create-retries').value || '3', 10),
        payload: JSON.parse(document.getElementById('create-payload').value || '{}')
      };
      const out = await api('/jobs', { method: 'POST', body: JSON.stringify(body) });
      document.getElementById('job-output').innerText = JSON.stringify(out, null, 2);
      if (out && out.id) document.getElementById('job-search-id').value = out.id;
      flash('Job created');
    }

    // ── Search / List Jobs ──
    let currentPage = 1;
    async function searchJobs(page) {
      currentPage = page || 1;
      const status = document.getElementById('job-search-status').value.trim();
      const type = document.getElementById('job-search-type').value.trim();
      const tenant = document.getElementById('job-search-tenant').value.trim();
      const limit = parseInt(document.getElementById('job-search-limit').value || '10', 10);
      const params = new URLSearchParams();
      if (status) params.set('status', status);
      if (type) params.set('type', type);
      if (tenant) params.set('tenant_id', tenant);
      params.set('limit', String(limit));
      params.set('page', String(currentPage));
      const out = await api('/jobs?' + params.toString());
      document.getElementById('job-output').innerText = JSON.stringify(out, null, 2);
      // Pagination controls
      const total = out && out.total ? out.total : 0;
      const pages = Math.ceil(total / limit) || 1;
      const pg = document.getElementById('job-pagination');
      let html = '<span class="muted">Page ' + currentPage + ' of ' + pages + ' (' + total + ' total)</span>';
      if (currentPage > 1) html += '<button class="secondary" onclick="searchJobs(' + (currentPage-1) + ')">Prev</button>';
      if (currentPage < pages) html += '<button class="secondary" onclick="searchJobs(' + (currentPage+1) + ')">Next</button>';
      pg.innerHTML = html;
    }
    async function getJobByID() {
      const id = document.getElementById('job-search-id').value.trim();
      if (!id) return;
      const out = await api('/jobs/' + encodeURIComponent(id));
      document.getElementById('job-output').innerText = JSON.stringify(out, null, 2);
    }

    // ── Metrics & Health ──
    async function loadMetrics() {
      const out = await api('/metrics');
      document.getElementById('metrics-output').innerText = typeof out === 'string' ? out : JSON.stringify(out, null, 2);
    }
    async function checkHealth(path) {
      const out = await api(path);
      document.getElementById('health-output').innerText = JSON.stringify(out, null, 2);
      document.getElementById('stat-status').innerText = out && out.status ? out.status : 'ok';
    }

    // ── Workers ──
    async function loadWorkers() {
      const out = await api('/workers');
      const rows = (out || []).map(w => '<tr><td>' + w.id + '</td><td>' + new Date(w.last_heartbeat).toLocaleString() + '</td></tr>').join('');
      document.getElementById('workers-body').innerHTML = rows || '<tr><td colspan="2" class="muted">No workers found.</td></tr>';
      document.getElementById('stat-workers').innerText = String((out || []).length);
    }

    // ── Stats ──
    async function loadStats() {
      try {
        const stats = await api('/api/v1/stats');
        document.getElementById('stats-queue').innerText = JSON.stringify(stats.queue_breakdown || {}, null, 2);
        document.getElementById('stats-overview').innerText = JSON.stringify({
          total_pending: stats.total_pending,
          worker_count: stats.worker_count,
          workers: stats.workers,
          approx_completed: stats.approx_completed,
          approx_failed: stats.approx_failed
        }, null, 2);
        if (stats.total_pending !== undefined) document.getElementById('stat-pending').innerText = String(stats.total_pending);
      } catch(e) {
        document.getElementById('stats-queue').innerText = 'Failed to load stats: ' + e;
      }
    }

    // ── DAG ──
    async function loadDAG() {
      const id = document.getElementById('dag-job-id').value.trim();
      if (!id) { toast('Enter a job ID'); return; }
      const out = await api('/api/v1/jobs/' + encodeURIComponent(id) + '/deps');
      if (!out) { document.getElementById('dag-upstream').innerText = 'No response'; return; }
      document.getElementById('dag-upstream').innerText = JSON.stringify(out.depends_on || [], null, 2);
      document.getElementById('dag-downstream').innerText = JSON.stringify(out.dependents || [], null, 2);
    }

    // ── Circuit Breaker ──
    async function loadCircuitBreakers() {
      const out = await api('//' + location.hostname + ':' + WORKER_PORT + '/circuit-breaker');
      document.getElementById('stat-cb').innerText = String(Object.keys(out || {}).length);
      const rows = Object.entries(out || {}).map(([k, v]) => {
        const cls = v.startsWith('open') ? 'bad' : v === 'closed' ? 'good' : 'warn';
        return '<tr><td>' + k + '</td><td><span class="pill ' + cls + '">' + v + '</span></td><td><button class="secondary" onclick="resetBreaker(\''+k+'\')">Reset</button></td></tr>';
      }).join('');
      document.getElementById('cb-body').innerHTML = rows || '<tr><td colspan="3" class="muted">No circuit breakers active.</td></tr>';
    }
    async function resetBreaker(type) {
      await api('//' + location.hostname + ':' + WORKER_PORT + '/circuit-breaker/reset/' + encodeURIComponent(type), { method: 'POST' });
      toast('Reset circuit breaker for ' + type);
      loadCircuitBreakers();
    }

    // ── DLQ ──
    async function loadDLQ() {
      const queue = document.getElementById('dlq-queue').value.trim();
      const tenant = document.getElementById('dlq-tenant').value.trim();
      const path = API_BASE + '/dlq' + (queue ? '?queue=' + encodeURIComponent(queue) : '');
      const out = await api(path);
      const filtered = (out || []).filter(j => !tenant || (j.tenant_id || '').includes(tenant));
      document.getElementById('dlq-output').innerText = JSON.stringify(filtered, null, 2);
      document.getElementById('stat-dlq').innerText = String(filtered.length);
      return filtered;
    }
    function searchDLQ() {
      const q = document.getElementById('dlq-search').value.trim().toLowerCase();
      const raw = document.getElementById('dlq-output').innerText;
      try {
        const items = JSON.parse(raw || '[]');
        const filtered = items.filter(j => JSON.stringify(j).toLowerCase().includes(q));
        document.getElementById('dlq-output').innerText = JSON.stringify(filtered, null, 2);
        document.getElementById('stat-dlq').innerText = String(filtered.length);
      } catch(_) { flash('Nothing to search'); }
    }
    function exportDLQ() {
      const raw = document.getElementById('dlq-output').innerText;
      const blob = new Blob([raw], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a'); a.href = url; a.download = 'dlq-export.json'; a.click();
      URL.revokeObjectURL(url);
    }
    async function replayDLQ() {
      const id = document.getElementById('dlq-replay-id').value.trim();
      if (!id) return;
      await api(API_BASE + '/dlq/' + encodeURIComponent(id) + '/replay', { method: 'POST' });
      loadDLQ();
    }
    async function deleteDLQ() {
      const id = document.getElementById('dlq-purge-id').value.trim();
      if (!id) return;
      await api(API_BASE + '/dlq/' + encodeURIComponent(id), { method: 'DELETE' });
      loadDLQ();
    }
    async function bulkPurgeDLQ() {
      const older = document.getElementById('dlq-older').value.trim();
      if (!older) return;
      const queue = document.getElementById('dlq-queue').value.trim();
      await api(API_BASE + '/dlq?older_than=' + encodeURIComponent(older) + (queue ? '&queue=' + encodeURIComponent(queue) : ''), { method: 'DELETE' });
      loadDLQ();
    }

    // ── Webhooks ──
    async function loadWebhooks() {
      try {
        const out = await api(API_BASE + '/webhooks');
        document.getElementById('wh-output').innerText = JSON.stringify(out || [], null, 2);
      } catch(e) {
        document.getElementById('wh-output').innerText = 'Failed to load webhooks. Auth required.';
      }
    }
    async function registerWebhook() {
      const body = {
        url: document.getElementById('wh-url').value.trim(),
        secret: document.getElementById('wh-secret').value.trim(),
        events: (document.getElementById('wh-events').value.trim() || 'completed,failed').split(',').map(s => s.trim())
      };
      if (!body.url) { toast('URL is required'); return; }
      const out = await api(API_BASE + '/webhooks', { method: 'POST', body: JSON.stringify(body) });
      document.getElementById('wh-output').innerText = JSON.stringify(out, null, 2);
      loadWebhooks();
    }

    // ── Init ──
    async function loadEverything() {
      getToken();
      const tab = localStorage.getItem('task_queue_tab') || 'jobs';
      showTab(tab);
      await Promise.all([
        loadDLQ(), loadWorkers(), loadMetrics(),
        checkHealth('/healthz'), checkHealth('/readyz'),
        loadStats(), loadCircuitBreakers()
      ]);
    }

    getToken();
    loadEverything();
  </script>
</body>
</html>
`
