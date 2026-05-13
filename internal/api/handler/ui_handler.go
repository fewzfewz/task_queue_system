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
    :root { --bg:#0b1020; --panel:#121a33; --panel2:#182241; --text:#e8ecff; --muted:#9aa7d6; --line:#2a355f; --accent:#7c9cff; --good:#3ddc97; --bad:#ff6b81; }
    * { box-sizing:border-box; }
    body { margin:0; font-family: Inter, Arial, sans-serif; background: radial-gradient(circle at top, #15203f, var(--bg)); color:var(--text); }
    header { padding:20px 24px; border-bottom:1px solid var(--line); display:flex; justify-content:space-between; align-items:center; backdrop-filter: blur(8px); position:sticky; top:0; background:rgba(11,16,32,.85); }
    h1 { margin:0; font-size:20px; }
    main { padding:24px; display:grid; gap:20px; max-width:1400px; margin:0 auto; }
    .grid { display:grid; gap:16px; }
    .cards { grid-template-columns: repeat(auto-fit,minmax(180px,1fr)); }
    .two { grid-template-columns: repeat(auto-fit,minmax(320px,1fr)); }
    .three { grid-template-columns: repeat(auto-fit,minmax(280px,1fr)); }
    .card { background: linear-gradient(180deg, rgba(255,255,255,.03), rgba(255,255,255,.015)); border:1px solid var(--line); border-radius:16px; padding:16px; box-shadow:0 18px 40px rgba(0,0,0,.22); }
    .label { font-size:12px; color:var(--muted); text-transform:uppercase; letter-spacing:.08em; }
    .value { font-size:28px; margin-top:8px; font-weight:700; }
    .muted { color:var(--muted); font-size:13px; }
    input, select, textarea, button { width:100%; border-radius:12px; border:1px solid var(--line); background:var(--panel); color:var(--text); padding:12px 14px; font-size:14px; }
    textarea { min-height:120px; resize:vertical; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
    button { cursor:pointer; background:linear-gradient(180deg, #89a5ff, #5f7cff); color:#07101f; font-weight:700; border:none; }
    button.secondary { background:var(--panel2); color:var(--text); border:1px solid var(--line); }
    button.danger { background:linear-gradient(180deg, #ff8fa1, #ff5f7d); color:#1b0710; }
    .row { display:grid; gap:10px; margin-top:12px; }
    .row.cols2 { grid-template-columns:1fr 1fr; }
    .row.cols3 { grid-template-columns:1fr 1fr 1fr; }
    pre { margin:0; white-space:pre-wrap; word-break:break-word; background:#081022; border:1px solid var(--line); border-radius:12px; padding:12px; max-height:320px; overflow:auto; }
    table { width:100%; border-collapse:collapse; font-size:14px; }
    th, td { text-align:left; padding:10px 8px; border-bottom:1px solid rgba(42,53,95,.7); vertical-align:top; }
    th { color:var(--muted); font-size:12px; text-transform:uppercase; letter-spacing:.08em; }
    .pill { display:inline-block; padding:4px 8px; border-radius:999px; background:rgba(124,156,255,.14); color:#bfd0ff; font-size:12px; }
    .pill.good { background:rgba(61,220,151,.14); color:#9ef0c8; }
    .pill.bad { background:rgba(255,107,129,.14); color:#ffb1be; }
    .toolbar { display:flex; gap:10px; flex-wrap:wrap; }
    .toolbar > * { flex:1 1 220px; }
    .split { display:grid; gap:16px; grid-template-columns: repeat(auto-fit,minmax(340px,1fr)); }
    .tabs { display:flex; gap:10px; flex-wrap:wrap; }
    .tab { flex:0 0 auto; padding:10px 14px; border-radius:999px; border:1px solid var(--line); background:var(--panel2); color:var(--text); cursor:pointer; }
    .tab.active { background:linear-gradient(180deg, #89a5ff, #5f7cff); color:#07101f; border-color:transparent; }
    .panel { display:none; }
    .panel.active { display:block; }
    a { color:var(--accent); }
  </style>
</head>
<body>
  <header>
    <div>
      <h1>Task Queue System</h1>
      <div class="muted">Simple browser UI for jobs, workers, health, metrics, and DLQ actions</div>
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
    <section class="grid cards">
      <div class="card"><div class="label">Status</div><div id="stat-status" class="value">-</div><div class="muted">Ready / live checks</div></div>
      <div class="card"><div class="label">Jobs</div><div id="stat-jobs" class="value">-</div><div class="muted">Newest jobs returned by the API</div></div>
      <div class="card"><div class="label">Workers</div><div id="stat-workers" class="value">-</div><div class="muted">Active worker heartbeats</div></div>
      <div class="card"><div class="label">DLQ</div><div id="stat-dlq" class="value">-</div><div class="muted">Failed jobs currently listed</div></div>
    </section>

    <section class="card">
      <div class="toolbar">
        <button onclick="fillExample()">Example email job</button>
        <button class="secondary" onclick="checkHealth('/healthz')">Health</button>
        <button class="secondary" onclick="checkHealth('/readyz')">Ready</button>
        <button class="secondary" onclick="loadWorkers()">Workers</button>
        <button class="secondary" onclick="loadMetrics()">Metrics</button>
        <button class="secondary" onclick="loadDLQ()">DLQ</button>
      </div>
    </section>

    <section class="card">
      <div class="tabs">
        <button class="tab active" data-tab="jobs" onclick="showTab('jobs')">Create / Search Jobs</button>
        <button class="tab" data-tab="ops" onclick="showTab('ops')">Workers / Health</button>
        <button class="tab" data-tab="dlq" onclick="showTab('dlq')">DLQ / Admin</button>
      </div>
    </section>

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
            <input id="create-runat" placeholder="run_at RFC3339 optional" />
          </div>
          <div class="row cols2">
            <input id="create-corr" placeholder="correlation_id optional" />
            <input id="create-timeout" placeholder="timeout seconds" value="60" />
          </div>
          <div class="row cols2">
            <input id="create-retries" placeholder="max_retries" value="3" />
            <input id="create-version" placeholder="version" value="1" />
          </div>
          <textarea id="create-payload">{ "to": "user@example.com", "subject": "hello" }</textarea>
          <div class="row cols2">
            <button onclick="createJob()">Create job</button>
            <button class="secondary" onclick="fillExample()">Fill email example</button>
          </div>
        </div>

        <div class="card">
          <h2>Search Job</h2>
          <div class="row cols2">
            <input id="job-id" placeholder="job id" />
            <input id="job-tenant-filter" placeholder="tenant filter optional" />
          </div>
          <div class="row cols3">
            <button onclick="getJob()">Get job</button>
            <button class="secondary" onclick="loadMetrics()">Metrics</button>
            <button class="secondary" onclick="checkHealth('/readyz')">Readiness</button>
          </div>
          <pre id="job-output">No job loaded yet.</pre>
        </div>
      </div>
    </section>

    <section id="panel-ops" class="panel">
      <div class="split">
        <div class="card">
          <h2>Workers</h2>
          <div class="row cols2">
            <button onclick="loadWorkers()">Refresh workers</button>
            <button class="secondary" onclick="checkHealth('/healthz')">Check liveness</button>
          </div>
          <table>
            <thead>
              <tr><th>ID</th><th>Last Heartbeat</th></tr>
            </thead>
            <tbody id="workers-body">
              <tr><td colspan="2" class="muted">No workers loaded.</td></tr>
            </tbody>
          </table>
        </div>

        <div class="card">
          <h2>Health & Metrics</h2>
          <div class="row cols2">
            <button onclick="checkHealth('/healthz')">/healthz</button>
            <button onclick="checkHealth('/readyz')">/readyz</button>
          </div>
          <div class="row">
            <button onclick="loadMetrics()">Load metrics</button>
          </div>
          <div class="row cols2">
            <div>
              <div class="label">Health</div>
              <pre id="health-output">Click a health check.</pre>
            </div>
            <div>
              <div class="label">Metrics</div>
              <pre id="metrics-output">Prometheus metrics appear here.</pre>
            </div>
          </div>
        </div>
      </div>
    </section>

    <section id="panel-dlq" class="panel">
      <div class="card">
        <h2>Dead Letter Queue</h2>
        <div class="toolbar">
          <input id="dlq-queue" placeholder="queue filter optional" />
          <input id="dlq-tenant" placeholder="tenant filter optional" />
          <button onclick="loadDLQ()">Load DLQ</button>
        </div>
        <div class="row cols2">
          <input id="dlq-search" placeholder="search error / job id / tenant / type" />
          <button class="secondary" onclick="searchDLQ()">Search</button>
        </div>
        <div class="row cols2">
          <input id="dlq-replay-id" placeholder="job id to replay" />
          <button class="secondary" onclick="replayDLQ()">Replay job</button>
        </div>
        <div class="row cols2">
          <input id="dlq-purge-id" placeholder="job id to delete" />
          <button class="danger" onclick="deleteDLQ()">Delete job</button>
        </div>
        <div class="row cols2">
          <input id="dlq-older" placeholder="older than RFC3339 for bulk purge" />
          <button class="danger" onclick="bulkPurgeDLQ()">Bulk purge</button>
        </div>
        <div class="row cols2">
          <button class="secondary" onclick="exportDLQ()">Export JSON</button>
          <button class="secondary" onclick="loadDLQ()">Clear search / reload</button>
        </div>
        <pre id="dlq-output">No DLQ items loaded yet.</pre>
      </div>
    </section>
  </main>

  <script>
    const API_BASE = '/api/v1';
    const TOKEN_KEY = 'task_queue_token';

    function saveToken() {
      localStorage.setItem(TOKEN_KEY, document.getElementById('token').value.trim());
      flash('Token saved');
    }

    function saveAuthMode() {
      localStorage.setItem('task_queue_auth_mode', document.getElementById('auth-mode').value);
      flash('Auth mode saved');
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
        if (mode === 'bearer') {
          hdr['Authorization'] = token.startsWith('Bearer ') ? token : 'Bearer ' + token;
        } else if (token.startsWith('Bearer ')) {
          hdr['Authorization'] = token;
        } else if (token.includes('.') && token.split('.').length === 3) {
          hdr['Authorization'] = 'Bearer ' + token;
        } else {
          hdr['X-API-Key'] = token;
        }
      }
      return hdr;
    }

    function flash(msg) { document.getElementById('stat-status').innerText = msg; }

    function showTab(name) {
      document.querySelectorAll('.tab').forEach(btn => btn.classList.toggle('active', btn.dataset.tab === name));
      document.querySelectorAll('.panel').forEach(panel => panel.classList.remove('active'));
      document.getElementById('panel-' + name).classList.add('active');
      localStorage.setItem('task_queue_tab', name);
    }

    async function api(path, options = {}) {
      const res = await fetch(path, { ...options, headers: { ...headers(), ...(options.headers || {}) } });
      if (res.status === 204) return null;
      const text = await res.text();
      try { return JSON.parse(text); } catch (_) { return text; }
    }

    function fillExample() {
      document.getElementById('create-type').value = 'email';
      document.getElementById('create-payload').value = JSON.stringify({to:'user@example.com', subject:'hello'}, null, 2);
    }

    async function createJob() {
      const body = {
        type: document.getElementById('create-type').value.trim(),
        priority: document.getElementById('create-priority').value.trim(),
        tenant_id: document.getElementById('create-tenant').value.trim(),
        correlation_id: document.getElementById('create-corr').value.trim(),
        run_at: document.getElementById('create-runat').value.trim(),
        timeout: parseInt(document.getElementById('create-timeout').value || '60', 10),
        max_retries: parseInt(document.getElementById('create-retries').value || '3', 10),
        version: parseInt(document.getElementById('create-version').value || '1', 10),
        payload: JSON.parse(document.getElementById('create-payload').value)
      };
      const out = await api('/jobs', { method: 'POST', body: JSON.stringify(body) });
      document.getElementById('job-output').innerText = JSON.stringify(out, null, 2);
      document.getElementById('job-id').value = out.id || '';
      flash('Job created');
    }

    async function getJob() {
      const id = document.getElementById('job-id').value.trim();
      if (!id) return;
      const tenant = document.getElementById('job-tenant-filter').value.trim();
      const path = '/jobs/' + encodeURIComponent(id) + (tenant ? '?tenant_id=' + encodeURIComponent(tenant) : '');
      const out = await api(path);
      document.getElementById('job-output').innerText = JSON.stringify(out, null, 2);
    }

    async function loadMetrics() {
      const out = await api('/metrics');
      document.getElementById('metrics-output').innerText = typeof out === 'string' ? out : JSON.stringify(out, null, 2);
    }

    async function loadWorkers() {
      const out = await api('/workers');
      const rows = (out || []).map(w => '<tr><td>' + w.id + '</td><td>' + new Date(w.last_heartbeat).toLocaleString() + '</td></tr>').join('');
      document.getElementById('workers-body').innerHTML = rows || '<tr><td colspan="2" class="muted">No workers found.</td></tr>';
      document.getElementById('stat-workers').innerText = String((out || []).length);
    }

    async function checkHealth(path) {
      const out = await api(path);
      document.getElementById('health-output').innerText = JSON.stringify(out, null, 2);
      document.getElementById('stat-status').innerText = out.status || 'ok';
    }

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
      } catch (_) {
        flash('Nothing to search');
      }
    }

    function exportDLQ() {
      const raw = document.getElementById('dlq-output').innerText;
      const blob = new Blob([raw], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'dlq-export.json';
      a.click();
      URL.revokeObjectURL(url);
    }

    async function replayDLQ() {
      const id = document.getElementById('dlq-replay-id').value.trim();
      if (!id) return;
      const out = await api(API_BASE + '/dlq/' + encodeURIComponent(id) + '/replay', { method: 'POST' });
      document.getElementById('dlq-output').innerText = JSON.stringify(out, null, 2);
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
      const queue = document.getElementById('dlq-queue').value.trim();
      if (!older) return;
      const path = API_BASE + '/dlq?older_than=' + encodeURIComponent(older) + (queue ? '&queue=' + encodeURIComponent(queue) : '');
      const out = await api(path, { method: 'DELETE' });
      document.getElementById('dlq-output').innerText = JSON.stringify(out, null, 2);
      loadDLQ();
    }

    async function loadEverything() {
      getToken();
      const tab = localStorage.getItem('task_queue_tab') || 'jobs';
      showTab(tab);
      if (tab === 'jobs') {
        document.getElementById('stat-jobs').innerText = 'Ready';
      }
      await Promise.all([loadDLQ(), loadWorkers(), loadMetrics(), checkHealth('/healthz'), checkHealth('/readyz')]);
    }

    getToken();
    loadEverything();
  </script>
</body>
</html>
`
