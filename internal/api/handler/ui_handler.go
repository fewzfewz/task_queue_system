package handler

import (
	"fmt"
	"net/http"
	"strings"
)

// ServeAppUI renders the general operator UI for the queue system.
func (h *JobHandler) ServeAppUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, strings.ReplaceAll(appHTML, "__API_KEY__", h.apiKey))
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
    #login-overlay { position:fixed; inset:0; display:flex; align-items:center; justify-content:center; background:var(--bg); z-index:100; }
    #login-overlay.hidden { display:none; }
    .login-card { background:var(--panel); border:1px solid var(--line); border-radius:20px; padding:40px; width:360px; max-width:90vw; }
    .login-card h2 { margin:0 0 8px; }
    .login-card .muted { margin-bottom:24px; }
    .login-card .error { color:var(--bad); font-size:13px; margin-top:8px; display:none; }
  </style>
</head>
<body>
  <div id="toast" class="toast"></div>

  <!-- Login -->
  <div id="login-overlay">
    <div class="login-card">
      <h2>Task Queue</h2>
      <div class="muted">Sign in to manage jobs and workers</div>
      <div class="row">
        <input id="login-user" placeholder="Username" value="admin" />
        <input id="login-pass" type="password" placeholder="Password" value="admin123" />
        <button onclick="login()">Sign in</button>
        <div id="login-error" class="error">Invalid credentials</div>
      </div>
    </div>
  </div>

  <!-- Dashboard -->
  <div id="app" style="display:none;">
  <header>
    <div>
      <h1>Task Queue System</h1>
      <div class="muted">Operator dashboard — jobs, workers, stats, DAG, circuit breaker, webhooks</div>
    </div>
    <div class="toolbar" style="max-width:720px;">
      <button class="secondary" onclick="loadEverything()">Refresh all</button>
      <button class="secondary" onclick="logout()" style="flex:0;padding:12px 18px;">Logout</button>
    </div>
  </header>

  <main>
    <!-- Stats Cards -->
    <section class="grid cards">
      <div class="card"><div class="label">Status</div><div id="stat-status" class="value">-</div><div class="muted">API reachable</div></div>
      <div class="card"><div class="label">Pending Jobs</div><div id="stat-pending" class="value">-</div><div class="muted">Total queued work</div></div>
      <div class="card"><div class="label">Workers</div><div id="stat-workers" class="value">-</div><div class="muted">Active heartbeats</div></div>
      <div class="card"><div class="label">DLQ</div><div id="stat-dlq" class="value">-</div><div class="muted">Failed jobs</div></div>
      <div class="card"><div class="label">Circuit Breakers</div><div id="stat-cb" class="value">0</div><div class="muted">Open / monitored</div></div>
    </section>

    <!-- Action Bar -->
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

    <!-- Tabs -->
    <section class="card">
      <div class="tabs">
        <button class="tab active" data-tab="jobs" onclick="showTab('jobs')">Jobs</button>
        <button class="tab" data-tab="stats" onclick="showTab('stats');loadStats()">Stats</button>
        <button class="tab" data-tab="dag" onclick="showTab('dag')">DAG</button>
        <button class="tab" data-tab="ops" onclick="showTab('ops')">Workers / Health</button>
        <button class="tab" data-tab="cb" onclick="showTab('cb');loadCircuitBreakers()">Circuit Breaker</button>
        <button class="tab" data-tab="dlq" onclick="showTab('dlq');loadDLQ()">DLQ</button>
        <button class="tab" data-tab="wh" onclick="showTab('wh');loadWebhooks()">Webhooks</button>
      </div>

      <!-- Tab: Jobs -->
      <div id="panel-jobs" class="panel active">
        <div class="row cols2">
          <div class="card">
            <div class="label">Create Job</div>
            <div class="row">
              <input id="create-type" placeholder="Job type (e.g. email)" value="email" />
              <input id="create-priority" placeholder="Priority (high/medium/low)" value="medium" />
              <input id="create-tenant" placeholder="Tenant ID (optional)" value="tenant-a" />
              <input id="create-corr" placeholder="Correlation ID (optional)" value="corr-001" />
              <input id="create-dedup" placeholder="Dedup key (optional)" value="dedup-email-001" />
              <input id="create-shard" placeholder="Shard key (optional)" value="shard-1" />
              <textarea id="create-payload" placeholder='Payload JSON'>{"to":"user@example.com","body":"Welcome!"}</textarea>
              <input id="create-runat" placeholder="Run at (ISO8601, optional)" />
              <button onclick="createJob()">Create Job</button>
            </div>
          </div>
          <div class="card">
            <div class="label">Search Jobs</div>
            <div class="row">
              <input id="search-type" placeholder="Filter by type (e.g. email)" value="email" />
              <input id="search-status" placeholder="Filter by status (pending/running/completed/failed)" value="pending" />
              <input id="search-tenant" placeholder="Tenant ID filter" />
              <div class="row cols2" style="margin:0;">
                <input id="search-page" type="number" placeholder="Page" value="1" />
                <input id="search-limit" type="number" placeholder="Limit" value="20" />
              </div>
              <button onclick="searchJobs()">Search</button>
            </div>
            <pre id="search-output">Results will appear here</pre>
            <div class="pagination">
              <button class="secondary" onclick="prevPage()">Prev</button>
              <span id="page-info" class="muted">Page 1</span>
              <button class="secondary" onclick="nextPage()">Next</button>
            </div>
          </div>
        </div>
      </div>

      <!-- Tab: Stats -->
      <div id="panel-stats" class="panel">
        <pre id="stats-output">Click "Stats" tab or button to load</pre>
      </div>

      <!-- Tab: DAG -->
      <div id="panel-dag" class="panel">
        <div class="row cols2">
          <input id="dag-job-id" placeholder="Job ID" value="job-123" />
          <button onclick="loadDAG()">Lookup dependencies</button>
        </div>
        <div class="split">
          <div><div class="label">Upstream (depends on)</div><pre id="dag-upstream">Enter a job ID and click Lookup</pre></div>
          <div><div class="label">Downstream (depended by)</div><pre id="dag-downstream"></pre></div>
        </div>
      </div>

      <!-- Tab: Workers / Health -->
      <div id="panel-ops" class="panel">
        <div class="split">
          <div>
            <div class="label">Workers</div>
            <pre id="workers-output">Click "Workers" to load</pre>
            <div class="label" style="margin-top:12px;">Metrics</div>
            <pre id="metrics-output">Click "Metrics" to load</pre>
          </div>
          <div>
            <div class="label">Health Checks</div>
            <div id="health-status"></div>
          </div>
        </div>
      </div>

      <!-- Tab: Circuit Breaker -->
      <div id="panel-cb" class="panel">
        <table><thead><tr><th>Type</th><th>State</th><th>Action</th></tr></thead><tbody id="cb-body"><tr><td colspan="3" class="muted">No data</td></tr></tbody></table>
      </div>

      <!-- Tab: DLQ -->
      <div id="panel-dlq" class="panel">
        <div class="row cols3">
          <input id="dlq-search" placeholder="Search in results" />
          <input id="dlq-queue" placeholder="Queue filter" value="email" />
          <input id="dlq-tenant" placeholder="Tenant filter" value="tenant-a" />
        </div>
        <div class="toolbar">
          <button onclick="loadDLQ()">Load DLQ</button>
          <button class="secondary" onclick="searchDLQ()">Filter results</button>
          <button class="secondary" onclick="exportDLQ()">Export JSON</button>
        </div>
        <pre id="dlq-output">Load DLQ to see failed jobs</pre>
        <div class="row cols4">
          <input id="dlq-replay-id" placeholder="Job ID to replay" value="job-123" />
          <button onclick="replayDLQ()">Replay</button>
          <input id="dlq-purge-id" placeholder="Job ID to purge" value="job-123" />
          <button onclick="deleteDLQ()">Purge</button>
        </div>
        <div class="row cols2">
          <input id="dlq-older" placeholder="Older than (ISO8601)" value="2025-01-01T00:00:00Z" />
          <button class="danger" onclick="bulkPurgeDLQ()">Bulk purge older</button>
        </div>
      </div>

      <!-- Tab: Webhooks -->
      <div id="panel-wh" class="panel">
        <div class="row cols3">
          <input id="wh-url" placeholder="Webhook URL" value="http://localhost:9090/hook" />
          <input id="wh-secret" placeholder="Secret" value="wh-secret-123" />
          <input id="wh-events" placeholder="Events (comma-separated)" value="completed,failed" />
        </div>
        <button onclick="registerWebhook()">Register Webhook</button>
        <pre id="wh-output">Click "Webhooks" to load</pre>
      </div>
    </section>
  </main>
  </div>

  <script>
    const API_BASE = '/api/v1';
    const WORKER_PORT = '8081';
    const TOKEN_KEY = 'task_queue_api_key';

    function toast(msg) { const t=document.getElementById('toast'); t.textContent=msg; t.classList.add('show'); setTimeout(()=>t.classList.remove('show'),2500); }

    function getToken() { return localStorage.getItem(TOKEN_KEY) || ''; }

    function headers() {
      const t = getToken();
      return t ? { 'Content-Type': 'application/json', 'X-API-Key': t } : { 'Content-Type': 'application/json' };
    }

    async function api(path, options = {}) {
      try {
        const res = await fetch(path, { ...options, headers: { ...headers(), ...(options.headers || {}) } });
        if (res.status === 204) return null;
        const text = await res.text();
        try { return JSON.parse(text); } catch (_) { return text; }
      } catch (_) { return null; }
    }

    async function login() {
      const user = document.getElementById('login-user').value.trim();
      const pass = document.getElementById('login-pass').value.trim();
      if (!user || !pass) { document.getElementById('login-error').style.display='block'; return; }
      const res = await api('/api/v1/login', {
        method: 'POST',
        body: JSON.stringify({username: user, password: pass}),
        headers: { 'Content-Type': 'application/json' }
      });
      if (res && res.api_key) {
        localStorage.setItem(TOKEN_KEY, res.api_key);
        document.getElementById('login-overlay').classList.add('hidden');
        document.getElementById('app').style.display = 'block';
        loadEverything();
        toast('Logged in');
      } else {
        document.getElementById('login-error').style.display = 'block';
      }
    }

    function logout() {
      localStorage.removeItem(TOKEN_KEY);
      document.getElementById('app').style.display = 'none';
      document.getElementById('login-overlay').classList.remove('hidden');
    }

    // Auto-login if token exists
    if (getToken()) {
      document.getElementById('login-overlay').classList.add('hidden');
      document.getElementById('app').style.display = 'block';
    }

    // Enter key logs in
    document.getElementById('login-pass').addEventListener('keydown', function(e) {
      if (e.key === 'Enter') login();
    });

    function flash(msg) { document.getElementById('stat-status').innerText = msg; }
    function showTab(name) {
      document.querySelectorAll('.tab').forEach(btn => btn.classList.toggle('active', btn.dataset.tab === name));
      document.querySelectorAll('.panel').forEach(panel => panel.classList.remove('active'));
      const p = document.getElementById('panel-'+name);
      if (p) p.classList.add('active');
      localStorage.setItem('task_queue_tab', name);
    }

    // ── Create Job ──
    function fillExample() {
      document.getElementById('create-type').value = 'email';
      document.getElementById('create-payload').value = JSON.stringify({to:'user@example.com', subject:'hello'}, null, 2);
      document.getElementById('create-priority').value = 'medium';
      document.getElementById('create-tenant').value = 'tenant-a';
      document.getElementById('create-corr').value = 'corr-' + Date.now();
      document.getElementById('create-dedup').value = 'dedup-' + Date.now();
      document.getElementById('create-shard').value = 'shard-1';
      toast('Example job filled');
    }
    async function createJob() {
      const body = {
        type: document.getElementById('create-type').value.trim() || 'email',
        priority: document.getElementById('create-priority').value.trim() || 'medium',
        tenant_id: document.getElementById('create-tenant').value.trim(),
        correlation_id: document.getElementById('create-corr').value.trim(),
        dedup_key: document.getElementById('create-dedup').value.trim(),
        shard_key: document.getElementById('create-shard').value.trim(),
        run_at: document.getElementById('create-runat').value.trim() || undefined,
        payload: {}
      };
      try { body.payload = JSON.parse(document.getElementById('create-payload').value.trim() || '{}'); } catch(_) { body.payload = {}; }
      const out = await api('/jobs', { method: 'POST', body: JSON.stringify(body) });
      document.getElementById('search-output').innerText = JSON.stringify(out, null, 2);
      flash(out && out.id ? 'Job created: ' + out.id : 'Create failed');
    }

    // ── Search Jobs ──
    let searchState = { page: 1, limit: 20 };
    async function searchJobs() {
      const type = document.getElementById('search-type').value.trim();
      const status = document.getElementById('search-status').value.trim();
      const tenant = document.getElementById('search-tenant').value.trim();
      searchState.page = parseInt(document.getElementById('search-page').value) || 1;
      searchState.limit = parseInt(document.getElementById('search-limit').value) || 20;
      const params = new URLSearchParams({ page: searchState.page, limit: searchState.limit });
      if (type) params.set('type', type);
      if (status) params.set('status', status);
      if (tenant) params.set('tenant_id', tenant);
      const out = await api('/jobs?' + params.toString());
      document.getElementById('search-output').innerText = JSON.stringify(out, null, 2);
      document.getElementById('page-info').innerText = 'Page ' + searchState.page;
    }
    function prevPage() { if (searchState.page > 1) { searchState.page--; document.getElementById('search-page').value = searchState.page; searchJobs(); } }
    function nextPage() { searchState.page++; document.getElementById('search-page').value = searchState.page; searchJobs(); }

    // ── Stats ──
    async function loadStats() {
      const out = await api(API_BASE + '/stats');
      document.getElementById('stats-output').innerText = JSON.stringify(out, null, 2);
      if (out) {
        document.getElementById('stat-pending').innerText = String(out.total_pending || 0);
        document.getElementById('stat-workers').innerText = String(out.worker_count || 0);
      }
    }

    // ── Workers ──
    async function loadWorkers() {
      const out = await api('/workers');
      document.getElementById('workers-output').innerText = JSON.stringify(out, null, 2);
    }

    // ── Metrics ──
    async function loadMetrics() {
      const out = await api('/metrics');
      document.getElementById('metrics-output').innerText = typeof out === 'string' ? out.substring(0, 2000) : JSON.stringify(out, null, 2);
    }

    // ── Health ──
    async function checkHealth(path) {
      const out = await api(path);
      const el = document.getElementById('health-status');
      if (el) {
        const p = document.createElement('p');
        p.innerHTML = '<span class="badge ' + (out ? 'green' : 'red') + '"></span> ' + path + (out ? ' OK' : ' FAIL');
        el.appendChild(p);
      }
      return out;
    }

    // ── DAG ──
    async function loadDAG() {
      const id = document.getElementById('dag-job-id').value.trim();
      if (!id) { toast('Enter a job ID'); return; }
      const out = await api(API_BASE + '/jobs/' + encodeURIComponent(id) + '/deps');
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
        document.getElementById('wh-output').innerText = 'Failed to load webhooks.';
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
      const tab = localStorage.getItem('task_queue_tab') || 'jobs';
      showTab(tab);
      await Promise.all([
        loadDLQ().catch(function(){}),
        loadWorkers().catch(function(){}),
        loadMetrics().catch(function(){}),
        checkHealth('/healthz').catch(function(){}),
        checkHealth('/readyz').catch(function(){}),
        loadStats().catch(function(){}),
        loadCircuitBreakers().catch(function(){})
      ]);
    }

    if (getToken()) loadEverything();
  </script>
</body>
</html>
`
