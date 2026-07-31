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

// ServeLoginPage renders the standalone login page.
func (h *JobHandler) ServeLoginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, loginPageHTML)
}

const appHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Task Queue</title>
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&display=swap" rel="stylesheet">
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css">
  <style>
    * { margin:0; padding:0; box-sizing:border-box; }
    :root {
      --bg:#0a0f1e; --bg2:#111827; --surface:#1a2236; --surface2:#1f2a40;
      --border:#2a3555; --text:#e2e8f0; --muted:#94a3b8; --muted2:#64748b;
      --accent:#6366f1; --accent2:#818cf8; --good:#22c55e; --bad:#ef4444; --warn:#f59e0b;
      --sidebar-w:250px; --header-h:64px;
    }
    body {
      font-family:'Inter',-apple-system,sans-serif; background:var(--bg); color:var(--text);
      min-height:100vh; overflow-x:hidden;
    }
    ::-webkit-scrollbar { width:6px; }
    ::-webkit-scrollbar-track { background:transparent; }
    ::-webkit-scrollbar-thumb { background:var(--border); border-radius:3px; }

    /* ── Layout ── */
    .layout { display:flex; min-height:100vh; }
    #sidebar {
      width:var(--sidebar-w); background:var(--surface); border-right:1px solid var(--border);
      position:fixed; top:0; left:0; height:100vh; z-index:100;
      display:flex; flex-direction:column; transition:transform .3s ease;
    }
    #sidebar.closed { transform:translateX(-100%); }
    .sidebar-brand {
      padding:20px 20px 16px; border-bottom:1px solid var(--border);
      display:flex; align-items:center; gap:12px;
    }
    .sidebar-brand .brand-icon {
      width:40px; height:40px; background:linear-gradient(135deg,var(--accent),#a78bfa);
      border-radius:12px; display:flex; align-items:center; justify-content:center;
      box-shadow:0 4px 12px rgba(99,102,241,.3);
    }
    .sidebar-brand .brand-icon i { color:#fff; font-size:18px; }
    .sidebar-brand h2 { font-size:16px; font-weight:700; }
    .sidebar-brand span { font-size:11px; color:var(--accent2); font-weight:500; }
    .sidebar-nav { flex:1; overflow-y:auto; padding:12px 10px; }
    .sidebar-nav .nav-label { font-size:10px; color:var(--muted2); text-transform:uppercase;
      letter-spacing:.1em; padding:16px 12px 6px; font-weight:600; }
    .nav-item {
      display:flex; align-items:center; gap:12px; padding:10px 14px;
      border-radius:10px; cursor:pointer; transition:all .15s;
      font-size:13px; font-weight:500; color:var(--muted);
      text-decoration:none; margin-bottom:2px;
    }
    .nav-item i { width:18px; text-align:center; font-size:14px; }
    .nav-item:hover { background:var(--surface2); color:var(--text); }
    .nav-item.active { background:rgba(99,102,241,.12); color:var(--accent2); }
    .sidebar-footer {
      padding:14px 16px; border-top:1px solid var(--border);
      display:flex; align-items:center; justify-content:space-between;
    }
    .sidebar-footer .user { font-size:12px; color:var(--muted); }
    .sidebar-footer button {
      background:none; border:1px solid var(--border); border-radius:8px;
      padding:6px 12px; color:var(--muted); font-size:12px; cursor:pointer;
      transition:all .15s;
    }
    .sidebar-footer button:hover { border-color:var(--bad); color:var(--bad); }

    /* ── Main Content ── */
    #main {
      margin-left:var(--sidebar-w); flex:1; transition:margin-left .3s ease;
      min-height:100vh;
    }
    #main.expanded { margin-left:0; }
    #topbar {
      height:var(--header-h); background:var(--surface); border-bottom:1px solid var(--border);
      display:flex; align-items:center; justify-content:space-between;
      padding:0 24px; position:sticky; top:0; z-index:50;
      backdrop-filter:blur(12px); background:rgba(26,34,54,.85);
    }
    #topbar .left { display:flex; align-items:center; gap:16px; }
    #topbar .left button {
      background:none; border:none; color:var(--muted); font-size:18px;
      cursor:pointer; padding:6px; border-radius:8px; display:none;
    }
    #topbar .left button:hover { background:var(--surface2); }
    #topbar .title-section h3 { font-size:16px; font-weight:600; }
    #topbar .title-section p { font-size:12px; color:var(--muted); }
    #topbar .right { display:flex; align-items:center; gap:12px; }
    #topbar .right button {
      background:var(--surface2); border:1px solid var(--border); border-radius:8px;
      padding:8px 14px; color:var(--text); font-size:12px; font-weight:500;
      cursor:pointer; transition:all .15s;
    }
    #topbar .right button:hover { background:var(--border); }
    .status-dot { width:8px; height:8px; border-radius:50%; display:inline-block; }
    .status-dot.green { background:var(--good); box-shadow:0 0 8px rgba(34,197,94,.4); }
    .status-dot.red { background:var(--bad); box-shadow:0 0 8px rgba(239,68,68,.4); }

    #content { padding:24px; max-width:1360px; margin:0 auto; }
    .page { display:none; }
    .page.active { display:block; animation:fadeIn .25s ease; }
    @keyframes fadeIn { from{opacity:0;transform:scale(.96)} to{opacity:1;transform:scale(1)} }

    /* ── Stats Cards ── */
    .stats-grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(180px,1fr)); gap:14px; margin-bottom:24px; }
    .stat-card {
      background:var(--surface); border:1px solid var(--border); border-radius:16px;
      padding:18px 20px; transition:border-color .2s;
    }
    .stat-card:hover { border-color:var(--accent); }
    .stat-card .stat-label { font-size:11px; color:var(--muted2); text-transform:uppercase;
      letter-spacing:.08em; font-weight:600; }
    .stat-card .stat-value { font-size:28px; font-weight:700; margin-top:6px; }
    .stat-card .stat-desc { font-size:12px; color:var(--muted); margin-top:2px; }

    /* ── Cards & Sections ── */
    .section-card {
      background:var(--surface); border:1px solid var(--border); border-radius:16px;
      padding:20px; margin-bottom:16px;
    }
    .section-card .section-title {
      font-size:13px; font-weight:600; color:var(--muted2); text-transform:uppercase;
      letter-spacing:.08em; margin-bottom:14px;
    }
    .grid-2 { display:grid; grid-template-columns:1fr 1fr; gap:14px; }
    .grid-3 { display:grid; grid-template-columns:1fr 1fr 1fr; gap:14px; }
    .grid-4 { display:grid; grid-template-columns:repeat(auto-fit,minmax(160px,1fr)); gap:14px; }

    input, select, textarea, .input {
      width:100%; background:var(--bg2); border:1px solid var(--border);
      border-radius:10px; padding:11px 14px; color:var(--text); font-size:13px;
      outline:none; transition:border-color .2s;
    }
    input:focus, select:focus, textarea:focus { border-color:var(--accent); }
    textarea { min-height:90px; resize:vertical; font-family:'Inter',monospace; }
    button, .btn {
      padding:10px 18px; border-radius:10px; font-size:13px; font-weight:600;
      cursor:pointer; transition:all .15s; border:none;
    }
    .btn-primary { background:linear-gradient(135deg,var(--accent),#a78bfa); color:#fff; }
    .btn-primary:hover { opacity:.9; }
    .btn-secondary { background:var(--surface2); border:1px solid var(--border); color:var(--text); }
    .btn-secondary:hover { background:var(--border); }
    .btn-danger { background:linear-gradient(135deg,#dc2626,#ef4444); color:#fff; }
    .btn-danger:hover { opacity:.9; }
    .btn-sm { padding:6px 12px; font-size:11px; border-radius:8px; }
    .form-group { margin-bottom:12px; }
    .form-group label { display:block; font-size:12px; color:var(--muted); margin-bottom:4px; font-weight:500; }
    .form-row { display:grid; grid-template-columns:1fr 1fr; gap:10px; }

    pre, .code-block {
      background:var(--bg2); border:1px solid var(--border); border-radius:10px;
      padding:14px; font-size:12px; line-height:1.5; overflow:auto;
      max-height:400px; font-family:'SF Mono','Fira Code',monospace;
      white-space:pre-wrap; word-break:break-all;
    }
    table { width:100%; border-collapse:collapse; font-size:13px; }
    th, td { text-align:left; padding:10px 12px; border-bottom:1px solid var(--border); }
    th { color:var(--muted2); font-size:11px; text-transform:uppercase; letter-spacing:.08em; font-weight:600; }
    tr:hover td { background:rgba(255,255,255,.02); }

    .pill { display:inline-block; padding:3px 10px; border-radius:999px;
      font-size:11px; font-weight:500; background:rgba(99,102,241,.12); color:var(--accent2); }
    .pill.good { background:rgba(34,197,94,.12); color:#4ade80; }
    .pill.bad { background:rgba(239,68,68,.12); color:#f87171; }
    .pill.warn { background:rgba(245,158,11,.12); color:#fbbf24; }
    .badge { display:inline-block; width:8px; height:8px; border-radius:50%; margin-right:6px; }
    .badge.green { background:var(--good); }
    .badge.red { background:var(--bad); }
    .badge.yellow { background:var(--warn); }
    .pagination { display:flex; gap:8px; align-items:center; margin-top:12px; }
    .toolbar { display:flex; gap:8px; flex-wrap:wrap; align-items:center; }

    /* ── Toast ── */
    #toast {
      position:fixed; bottom:24px; right:24px;
      background:var(--surface2); border:1px solid var(--border);
      border-radius:12px; padding:14px 20px; font-size:13px;
      opacity:0; transform:translateY(10px); transition:all .3s ease;
      pointer-events:none; z-index:999; max-width:360px;
      box-shadow:0 12px 40px rgba(0,0,0,.4);
    }
    #toast.show { opacity:1; transform:translateY(0); }

    /* ── Responsive ── */
    @media(max-width:768px) {
      #sidebar { transform:translateX(-100%); }
      #sidebar.open { transform:translateX(0); box-shadow:0 0 40px rgba(0,0,0,.5); }
      #main { margin-left:0; }
      #topbar .left button { display:block; }
      .grid-2, .grid-3 { grid-template-columns:1fr; }
      .stats-grid { grid-template-columns:repeat(2,1fr); }
      #content { padding:16px; }
      #sidebar { width:280px; }
    }
    @media(min-width:769px) {
      #sidebar { transform:translateX(0) !important; }
    }

    /* ── Animations ── */
    @keyframes slideUp { from{opacity:0;transform:translateY(12px)} to{opacity:1;transform:translateY(0)} }
    .slide-up { animation:slideUp .3s ease; }
    @keyframes pulse { 0%,100%{opacity:1} 50%{opacity:.5} }
    .pulse { animation:pulse 2s ease-in-out infinite; }
  </style>
</head>
<body>
  <div id="toast"></div>

  <!-- Dashboard -->
  <div id="app">
  <div class="layout">

    <!-- Sidebar -->
    <nav id="sidebar">
      <div class="sidebar-brand">
        <div class="brand-icon"><i class="fas fa-tasks"></i></div>
        <div><h2>Task Queue</h2><span>Operator Panel</span></div>
      </div>
      <div class="sidebar-nav">
        <div class="nav-label">Overview</div>
        <a class="nav-item active" data-page="dashboard" onclick="showPage('dashboard')">
          <i class="fas fa-chart-pie"></i> Dashboard
        </a>
        <div class="nav-label">Jobs</div>
        <a class="nav-item" data-page="jobs" onclick="showPage('jobs')">
          <i class="fas fa-plus-circle"></i> Create Jobs
        </a>
        <a class="nav-item" data-page="search" onclick="showPage('search')">
          <i class="fas fa-search"></i> Search Jobs
        </a>
        <a class="nav-item" data-page="stats" onclick="showPage('stats');loadStats()">
          <i class="fas fa-chart-bar"></i> Stats
        </a>
        <a class="nav-item" data-page="dag" onclick="showPage('dag')">
          <i class="fas fa-project-diagram"></i> DAG Dependencies
        </a>
        <div class="nav-label">Monitoring</div>
        <a class="nav-item" data-page="workers" onclick="showPage('workers');loadWorkers();loadMetrics();checkHealth()">
          <i class="fas fa-server"></i> Workers &amp; Health
        </a>
        <a class="nav-item" data-page="cb" onclick="showPage('cb');loadCircuitBreakers()">
          <i class="fas fa-shield-alt"></i> Circuit Breaker
        </a>
        <a class="nav-item" data-page="dlq" onclick="showPage('dlq');loadDLQ()">
          <i class="fas fa-trash-alt"></i> Dead Letter Queue
        </a>
        <div class="nav-label">Settings</div>
        <a class="nav-item" data-page="webhooks" onclick="showPage('webhooks');loadWebhooks()">
          <i class="fas fa-globe"></i> Webhooks
        </a>
      </div>
      <div class="sidebar-footer">
        <span class="user"><i class="fas fa-user-circle"></i> Admin</span>
        <button onclick="logout()"><i class="fas fa-sign-out-alt"></i> Logout</button>
      </div>
    </nav>

    <!-- Main -->
    <div id="main">
      <div id="topbar">
        <div class="left">
          <button onclick="toggleSidebar()"><i class="fas fa-bars"></i></button>
          <div class="title-section">
            <h3 id="page-title">Dashboard</h3>
            <p id="page-desc">System overview and statistics</p>
          </div>
        </div>
        <div class="right">
          <span id="status-indicator"><span class="status-dot green"></span> Online</span>
          <button onclick="loadEverything()"><i class="fas fa-sync-alt"></i> Refresh</button>
        </div>
      </div>
      <div id="content">

        <!-- Page: Dashboard -->
        <div id="page-dashboard" class="page active">
          <div class="stats-grid">
            <div class="stat-card"><div class="stat-label">Status</div><div id="stat-status" class="stat-value" style="font-size:18px;">Online</div><div class="stat-desc">API reachable</div></div>
            <div class="stat-card"><div class="stat-label">Pending Jobs</div><div id="stat-pending" class="stat-value">-</div><div class="stat-desc">Queued work</div></div>
            <div class="stat-card"><div class="stat-label">Active Workers</div><div id="stat-workers" class="stat-value">-</div><div class="stat-desc">Heartbeat received</div></div>
            <div class="stat-card"><div class="stat-label">Failed Jobs</div><div id="stat-dlq" class="stat-value">-</div><div class="stat-desc">Dead letter queue</div></div>
            <div class="stat-card"><div class="stat-label">Circuit Breakers</div><div id="stat-cb" class="stat-value">0</div><div class="stat-desc">Open circuits</div></div>
          </div>
          <div class="section-card slide-up">
            <div class="section-title"><i class="fas fa-bolt"></i> Quick Actions</div>
            <div class="toolbar">
              <button class="btn-primary btn-sm" onclick="showPage('jobs')"><i class="fas fa-plus"></i> Create Job</button>
              <button class="btn-secondary btn-sm" onclick="showPage('search')"><i class="fas fa-search"></i> Search Jobs</button>
              <button class="btn-secondary btn-sm" onclick="showPage('dlq');loadDLQ()"><i class="fas fa-trash"></i> DLQ</button>
              <button class="btn-secondary btn-sm" onclick="showPage('stats');loadStats()"><i class="fas fa-chart-bar"></i> Stats</button>
              <button class="btn-secondary btn-sm" onclick="showPage('workers');loadWorkers()"><i class="fas fa-server"></i> Workers</button>
              <button class="btn-secondary btn-sm" onclick="showPage('webhooks');loadWebhooks()"><i class="fas fa-globe"></i> Webhooks</button>
            </div>
          </div>
        </div>

        <!-- Page: Create Jobs -->
        <div id="page-jobs" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-plus-circle"></i> Create Job</div>
            <div class="grid-2">
              <div>
                <div class="form-group"><label>Job Type</label><input id="create-type" value="email" /></div>
                <div class="form-row">
                  <div class="form-group"><label>Priority</label><input id="create-priority" value="medium" /></div>
                  <div class="form-group"><label>Tenant ID</label><input id="create-tenant" value="tenant-a" /></div>
                </div>
                <div class="form-row">
                  <div class="form-group"><label>Correlation ID</label><input id="create-corr" value="corr-001" /></div>
                  <div class="form-group"><label>Dedup Key</label><input id="create-dedup" value="dedup-001" /></div>
                </div>
                <div class="form-group"><label>Shard Key</label><input id="create-shard" value="shard-1" /></div>
                <div class="form-group"><label>Schedule (ISO8601)</label><input id="create-runat" placeholder="Optional" /></div>
              </div>
              <div>
                <div class="form-group"><label>Payload (JSON)</label><textarea id="create-payload">{"to":"user@example.com","body":"Welcome!"}</textarea></div>
              </div>
            </div>
            <div class="toolbar" style="margin-top:10px;">
              <button class="btn-primary" onclick="createJob()"><i class="fas fa-paper-plane"></i> Create</button>
              <button class="btn-secondary" onclick="fillExample()"><i class="fas fa-magic"></i> Fill Example</button>
            </div>
            <pre id="create-output" style="margin-top:12px;">Result will appear here</pre>
          </div>
        </div>

        <!-- Page: Search Jobs -->
        <div id="page-search" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-search"></i> Search Jobs</div>
            <div class="grid-3">
              <div class="form-group"><label>Filter by Type</label><input id="search-type" value="email" /></div>
              <div class="form-group"><label>Filter by Status</label><input id="search-status" value="pending" placeholder="pending/running/completed/failed" /></div>
              <div class="form-group"><label>Tenant ID</label><input id="search-tenant" placeholder="Optional" /></div>
            </div>
            <div class="form-row" style="grid-template-columns:1fr 1fr 2fr;margin-bottom:12px;">
              <div class="form-group"><label>Page</label><input id="search-page" type="number" value="1" /></div>
              <div class="form-group"><label>Limit</label><input id="search-limit" type="number" value="20" /></div>
              <div style="display:flex;align-items:flex-end;gap:8px;">
                <button class="btn-primary" onclick="searchJobs()" style="flex:1;"><i class="fas fa-search"></i> Search</button>
                <button class="btn-secondary" onclick="prevPage()"><i class="fas fa-chevron-left"></i></button>
                <button class="btn-secondary" onclick="nextPage()"><i class="fas fa-chevron-right"></i></button>
              </div>
            </div>
            <div class="pagination" style="justify-content:space-between;">
              <span id="page-info" style="font-size:12px;color:var(--muted);">Page 1</span>
            </div>
            <pre id="search-output" style="margin-top:8px;">Results will appear here</pre>
          </div>
        </div>

        <!-- Page: Stats -->
        <div id="page-stats" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-chart-bar"></i> Queue Statistics</div>
            <pre id="stats-output">Click "Stats" to load</pre>
          </div>
        </div>

        <!-- Page: DAG -->
        <div id="page-dag" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-project-diagram"></i> Dependency Graph</div>
            <div class="form-row">
              <div class="form-group"><label>Job ID</label><input id="dag-job-id" value="job-123" /></div>
              <div style="display:flex;align-items:flex-end;"><button class="btn-primary" onclick="loadDAG()"><i class="fas fa-search"></i> Lookup</button></div>
            </div>
            <div class="grid-2" style="margin-top:14px;">
              <div>
                <div class="section-title" style="font-size:11px;">Upstream (depends on)</div>
                <pre id="dag-upstream">Enter a job ID and click Lookup</pre>
              </div>
              <div>
                <div class="section-title" style="font-size:11px;">Downstream (depended by)</div>
                <pre id="dag-downstream"></pre>
              </div>
            </div>
          </div>
        </div>

        <!-- Page: Workers & Health -->
        <div id="page-workers" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-server"></i> Workers</div>
            <pre id="workers-output">Click "Workers" to load</pre>
          </div>
          <div class="section-card">
            <div class="section-title"><i class="fas fa-chart-line"></i> Prometheus Metrics</div>
            <pre id="metrics-output">Click "Metrics" to load</pre>
          </div>
          <div class="section-card">
            <div class="section-title"><i class="fas fa-heartbeat"></i> Health Checks</div>
            <div id="health-status"></div>
          </div>
        </div>

        <!-- Page: Circuit Breaker -->
        <div id="page-cb" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-shield-alt"></i> Circuit Breaker Status</div>
            <table><thead><tr><th>Plugin Type</th><th>State</th><th>Action</th></tr></thead><tbody id="cb-body"><tr><td colspan="3" style="text-align:center;color:var(--muted);">No data</td></tr></tbody></table>
          </div>
        </div>

        <!-- Page: DLQ -->
        <div id="page-dlq" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-trash-alt"></i> Dead Letter Queue</div>
            <div class="grid-3" style="margin-bottom:12px;">
              <div class="form-group"><label>Search</label><input id="dlq-search" placeholder="Filter results..." /></div>
              <div class="form-group"><label>Queue Filter</label><input id="dlq-queue" value="email" /></div>
              <div class="form-group"><label>Tenant Filter</label><input id="dlq-tenant" value="tenant-a" /></div>
            </div>
            <div class="toolbar" style="margin-bottom:12px;">
              <button class="btn-primary btn-sm" onclick="loadDLQ()"><i class="fas fa-sync"></i> Load DLQ</button>
              <button class="btn-secondary btn-sm" onclick="searchDLQ()"><i class="fas fa-filter"></i> Filter</button>
              <button class="btn-secondary btn-sm" onclick="exportDLQ()"><i class="fas fa-download"></i> Export JSON</button>
            </div>
            <pre id="dlq-output">Load DLQ to see failed jobs</pre>
            <div class="grid-2" style="margin-top:12px;">
              <div class="form-row">
                <div class="form-group"><label>Job ID to Replay</label><input id="dlq-replay-id" value="job-123" /></div>
                <div style="display:flex;align-items:flex-end;"><button class="btn-primary btn-sm" onclick="replayDLQ()"><i class="fas fa-redo"></i> Replay</button></div>
              </div>
              <div class="form-row">
                <div class="form-group"><label>Job ID to Purge</label><input id="dlq-purge-id" value="job-123" /></div>
                <div style="display:flex;align-items:flex-end;"><button class="btn-danger btn-sm" onclick="deleteDLQ()"><i class="fas fa-times"></i> Purge</button></div>
              </div>
            </div>
            <div class="form-row" style="margin-top:12px;grid-template-columns:1fr auto;">
              <div class="form-group"><label>Bulk Purge (older than)</label><input id="dlq-older" value="2025-01-01T00:00:00Z" /></div>
              <div style="display:flex;align-items:flex-end;"><button class="btn-danger btn-sm" onclick="bulkPurgeDLQ()"><i class="fas fa-trash"></i> Bulk Purge</button></div>
            </div>
          </div>
        </div>

        <!-- Page: Webhooks -->
        <div id="page-webhooks" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-globe"></i> Register Webhook</div>
            <div class="grid-3">
              <div class="form-group"><label>URL</label><input id="wh-url" value="http://localhost:9090/hook" /></div>
              <div class="form-group"><label>Secret</label><input id="wh-secret" value="wh-secret-123" /></div>
              <div class="form-group"><label>Events (comma)</label><input id="wh-events" value="completed,failed" /></div>
            </div>
            <button class="btn-primary" onclick="registerWebhook()"><i class="fas fa-plus"></i> Register</button>
          </div>
          <div class="section-card">
            <div class="section-title"><i class="fas fa-list"></i> Registered Webhooks</div>
            <pre id="wh-output">Click "Webhooks" to load</pre>
          </div>
        </div>

      </div>
    </div>
  </div>
  </div>

  <script>
    const API_BASE = '/api/v1';
    const WORKER_PORT = '8081';
    const TOKEN_KEY = 'task_queue_api_key';
    const PAGE_NAMES = {
      dashboard:'Dashboard','jobs':'Create Jobs','search':'Search Jobs','stats':'Queue Statistics',
      dag:'DAG Dependencies','workers':'Workers & Health','cb':'Circuit Breaker',
      dlq:'Dead Letter Queue','webhooks':'Webhooks'
    };
    const PAGE_DESCS = {
      dashboard:'System overview and real-time statistics','jobs':'Submit new tasks to the queue',
      search:'Find and inspect queued jobs','stats':'Detailed queue performance metrics',
      dag:'Visualize job dependency chains','workers':'Monitor worker processes and health endpoints',
      cb':'Track circuit breaker states per plugin','dlq':'Manage failed jobs and retries',
      webhooks:'Configure event-driven HTTP callbacks'
    };

    function toast(msg) {
      const t=document.getElementById('toast'); t.textContent=msg;
      t.classList.add('show'); setTimeout(()=>t.classList.remove('show'),2500);
    }
    function getToken() { return localStorage.getItem(TOKEN_KEY) || ''; }
    function headers() {
      const t=getToken();
      return t?{'Content-Type':'application/json','X-API-Key':t}:{'Content-Type':'application/json'};
    }
    async function api(path,options={}) {
      try {
        const res=await fetch(path,{...options,headers:{...headers(),...(options.headers||{})}});
        if(res.status===204)return null;
        const text=await res.text();
        try{return JSON.parse(text)}catch(_){return text}
      }catch(_){return null}
    }

    function toggleSidebar() {
      document.getElementById('sidebar').classList.toggle('open');
    }
    document.getElementById('sidebar').addEventListener('click',function(e){
      if(window.innerWidth<=768)this.classList.remove('open');
    });

    function logout() {
      localStorage.removeItem(TOKEN_KEY);
      window.location.href='/login';
    }
    if(!getToken()){
      window.location.href='/login';
    }

    function showPage(name) {
      document.querySelectorAll('.page').forEach(p=>p.classList.remove('active'));
      document.querySelectorAll('.nav-item').forEach(n=>n.classList.remove('active'));
      const page=document.getElementById('page-'+name);
      if(page)page.classList.add('active');
      const nav=document.querySelector('.nav-item[data-page="'+name+'"]');
      if(nav)nav.classList.add('active');
      document.getElementById('page-title').textContent=PAGE_NAMES[name]||name;
      document.getElementById('page-desc').textContent=PAGE_DESCS[name]||'';
      localStorage.setItem('task_queue_page',name);
    }

    // ── Create Job ──
    function fillExample() {
      document.getElementById('create-type').value='email';
      document.getElementById('create-payload').value=JSON.stringify({to:'user@example.com',subject:'hello'},null,2);
      document.getElementById('create-priority').value='medium';
      document.getElementById('create-tenant').value='tenant-a';
      document.getElementById('create-corr').value='corr-'+Date.now();
      document.getElementById('create-dedup').value='dedup-'+Date.now();
      document.getElementById('create-shard').value='shard-1';
      toast('Example job filled');
    }
    async function createJob() {
      const body={
        type:document.getElementById('create-type').value.trim()||'email',
        priority:document.getElementById('create-priority').value.trim()||'medium',
        tenant_id:document.getElementById('create-tenant').value.trim(),
        correlation_id:document.getElementById('create-corr').value.trim(),
        dedup_key:document.getElementById('create-dedup').value.trim(),
        shard_key:document.getElementById('create-shard').value.trim(),
        run_at:document.getElementById('create-runat').value.trim()||undefined,
        payload:{}
      };
      try{body.payload=JSON.parse(document.getElementById('create-payload').value.trim()||'{}');}catch(_){}
      const out=await api('/jobs',{method:'POST',body:JSON.stringify(body)});
      document.getElementById('create-output').innerText=JSON.stringify(out,null,2);
      if(out&&out.id){toast('Job created: '+out.id);}else{toast('Create failed');}
    }

    // ── Search Jobs ──
    let searchState={page:1,limit:20};
    async function searchJobs() {
      const type=document.getElementById('search-type').value.trim();
      const status=document.getElementById('search-status').value.trim();
      const tenant=document.getElementById('search-tenant').value.trim();
      searchState.page=parseInt(document.getElementById('search-page').value)||1;
      searchState.limit=parseInt(document.getElementById('search-limit').value)||20;
      const p=new URLSearchParams({page:searchState.page,limit:searchState.limit});
      if(type)p.set('type',type);
      if(status)p.set('status',status);
      if(tenant)p.set('tenant_id',tenant);
      const out=await api('/jobs?'+p.toString());
      document.getElementById('search-output').innerText=JSON.stringify(out,null,2);
      document.getElementById('page-info').innerText='Page '+searchState.page;
    }
    function prevPage(){if(searchState.page>1){searchState.page--;document.getElementById('search-page').value=searchState.page;searchJobs();}}
    function nextPage(){searchState.page++;document.getElementById('search-page').value=searchState.page;searchJobs();}

    // ── Stats ──
    async function loadStats() {
      const out=await api(API_BASE+'/stats');
      document.getElementById('stats-output').innerText=JSON.stringify(out,null,2);
      if(out){
        document.getElementById('stat-pending').innerText=String(out.total_pending||0);
        document.getElementById('stat-workers').innerText=String(out.worker_count||0);
      }
    }

    // ── Workers ──
    async function loadWorkers() {
      const out=await api('/workers');
      document.getElementById('workers-output').innerText=JSON.stringify(out,null,2);
    }
    async function loadMetrics() {
      const out=await api('/metrics');
      document.getElementById('metrics-output').innerText=typeof out==='string'?out.substring(0,2000):JSON.stringify(out,null,2);
    }
    async function checkHealth() {
      const healthz=await api('/healthz');
      const readyz=await api('/readyz');
      const el=document.getElementById('health-status');
      el.innerHTML='<p><span class="badge '+(healthz?'green':'red')+'"></span> /healthz '+(healthz?'OK':'FAIL')+'</p>'+
        '<p><span class="badge '+(readyz?'green':'red')+'"></span> /readyz '+(readyz?'OK':'FAIL')+'</p>';
    }

    // ── DAG ──
    async function loadDAG() {
      const id=document.getElementById('dag-job-id').value.trim();
      if(!id){toast('Enter a job ID');return;}
      const out=await api(API_BASE+'/jobs/'+encodeURIComponent(id)+'/deps');
      if(!out){document.getElementById('dag-upstream').innerText='No response';return;}
      document.getElementById('dag-upstream').innerText=JSON.stringify(out.depends_on||[],null,2);
      document.getElementById('dag-downstream').innerText=JSON.stringify(out.dependents||[],null,2);
    }

    // ── Circuit Breaker ──
    async function loadCircuitBreakers() {
      const out=await api('//'+location.hostname+':'+WORKER_PORT+'/circuit-breaker');
      document.getElementById('stat-cb').innerText=String(Object.keys(out||{}).length);
      const rows=Object.entries(out||{}).map(([k,v])=>{
        const cls=v.startsWith('open')?'bad':v==='closed'?'good':'warn';
        return '<tr><td>'+k+'</td><td><span class="pill '+cls+'">'+v+'</span></td><td><button class="btn-secondary btn-sm" onclick="resetBreaker(\''+k+'\')">Reset</button></td></tr>';
      }).join('');
      document.getElementById('cb-body').innerHTML=rows||'<tr><td colspan="3" style="text-align:center;color:var(--muted);">No circuit breakers active.</td></tr>';
    }
    async function resetBreaker(type) {
      await api('//'+location.hostname+':'+WORKER_PORT+'/circuit-breaker/reset/'+encodeURIComponent(type),{method:'POST'});
      toast('Reset breaker for '+type); loadCircuitBreakers();
    }

    // ── DLQ ──
    async function loadDLQ() {
      const queue=document.getElementById('dlq-queue').value.trim();
      const tenant=document.getElementById('dlq-tenant').value.trim();
      const path=API_BASE+'/dlq'+(queue?'?queue='+encodeURIComponent(queue):'');
      const out=await api(path);
      const filtered=(out||[]).filter(j=>!tenant||(j.tenant_id||'').includes(tenant));
      document.getElementById('dlq-output').innerText=JSON.stringify(filtered,null,2);
      document.getElementById('stat-dlq').innerText=String(filtered.length);
      return filtered;
    }
    function searchDLQ() {
      const q=document.getElementById('dlq-search').value.trim().toLowerCase();
      const raw=document.getElementById('dlq-output').innerText;
      try{
        const items=JSON.parse(raw||'[]');
        const filtered=items.filter(j=>JSON.stringify(j).toLowerCase().includes(q));
        document.getElementById('dlq-output').innerText=JSON.stringify(filtered,null,2);
        document.getElementById('stat-dlq').innerText=String(filtered.length);
      }catch(_){toast('Nothing to search');}
    }
    function exportDLQ() {
      const raw=document.getElementById('dlq-output').innerText;
      const blob=new Blob([raw],{type:'application/json'});
      const url=URL.createObjectURL(blob);
      const a=document.createElement('a');a.href=url;a.download='dlq-export.json';a.click();
      URL.revokeObjectURL(url);
    }
    async function replayDLQ() {
      const id=document.getElementById('dlq-replay-id').value.trim();
      if(!id)return;
      await api(API_BASE+'/dlq/'+encodeURIComponent(id)+'/replay',{method:'POST'});
      loadDLQ(); toast('Replayed job: '+id);
    }
    async function deleteDLQ() {
      const id=document.getElementById('dlq-purge-id').value.trim();
      if(!id)return;
      await api(API_BASE+'/dlq/'+encodeURIComponent(id),{method:'DELETE'});
      loadDLQ(); toast('Purged job: '+id);
    }
    async function bulkPurgeDLQ() {
      const older=document.getElementById('dlq-older').value.trim();
      if(!older)return;
      const queue=document.getElementById('dlq-queue').value.trim();
      await api(API_BASE+'/dlq?older_than='+encodeURIComponent(older)+(queue?'&queue='+encodeURIComponent(queue):''),{method:'DELETE'});
      loadDLQ(); toast('Bulk purge completed');
    }

    // ── Webhooks ──
    async function loadWebhooks() {
      try{
        const out=await api(API_BASE+'/webhooks');
        document.getElementById('wh-output').innerText=JSON.stringify(out||[],null,2);
      }catch(e){document.getElementById('wh-output').innerText='Failed to load webhooks.';}
    }
    async function registerWebhook() {
      const body={
        url:document.getElementById('wh-url').value.trim(),
        secret:document.getElementById('wh-secret').value.trim(),
        events:(document.getElementById('wh-events').value.trim()||'completed,failed').split(',').map(s=>s.trim())
      };
      if(!body.url){toast('URL is required');return;}
      const out=await api(API_BASE+'/webhooks',{method:'POST',body:JSON.stringify(body)});
      document.getElementById('wh-output').innerText=JSON.stringify(out,null,2);
      loadWebhooks(); toast('Webhook registered');
    }

    // ── Init ──
    async function loadEverything() {
      const page=localStorage.getItem('task_queue_page')||'dashboard';
      showPage(page);
      await Promise.all([
        loadDLQ().catch(function(){}),
        loadWorkers().catch(function(){}),
        loadMetrics().catch(function(){}),
        checkHealth().catch(function(){}),
        loadStats().catch(function(){}),
        loadCircuitBreakers().catch(function(){})
      ]);
    }
    if(getToken())loadEverything();
  </script>
</body>
</html>`

const loginPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Task Queue — Sign In</title>
  <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&display=swap" rel="stylesheet">
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css">
  <style>
    * { margin:0; padding:0; box-sizing:border-box; }
    :root {
      --bg:#0a0f1e; --bg2:#111827; --surface:#1a2236; --surface2:#1f2a40;
      --border:#2a3555; --text:#e2e8f0; --muted:#94a3b8; --muted2:#64748b;
      --accent:#6366f1; --accent2:#818cf8; --good:#22c55e; --bad:#ef4444; --warn:#f59e0b;
    }
    body {
      font-family:'Inter',-apple-system,sans-serif;
      background:radial-gradient(ellipse at center, #1a2236 0%, #0a0f1e 70%);
      color:var(--text); min-height:100vh; display:flex; align-items:center; justify-content:center;
    }
    ::-webkit-scrollbar { width:6px; }
    ::-webkit-scrollbar-track { background:transparent; }
    ::-webkit-scrollbar-thumb { background:var(--border); border-radius:3px; }
    @keyframes fadeIn { from{opacity:0;transform:scale(.96)} to{opacity:1;transform:scale(1)} }
    @keyframes slideUp { from{opacity:0;transform:translateY(20px)} to{opacity:1;transform:translateY(0)} }
    .login-card {
      background:var(--surface); border:1px solid var(--border);
      border-radius:24px; padding:48px 40px; width:400px; max-width:90vw;
      box-shadow:0 25px 80px rgba(0,0,0,.5); animation:fadeIn .5s ease;
    }
    .login-card .logo {
      width:56px; height:56px; background:linear-gradient(135deg,var(--accent),#a78bfa);
      border-radius:16px; display:flex; align-items:center; justify-content:center;
      margin-bottom:20px; box-shadow:0 8px 24px rgba(99,102,241,.3);
    }
    .login-card .logo i { font-size:24px; color:#fff; }
    .login-card h1 { font-size:22px; font-weight:700; margin-bottom:4px; animation:slideUp .4s ease .1s both; }
    .login-card p { color:var(--muted); font-size:14px; margin-bottom:28px; animation:slideUp .4s ease .15s both; }
    .login-card input {
      width:100%; background:var(--bg2); border:1px solid var(--border);
      border-radius:12px; padding:14px 16px; color:var(--text); font-size:14px;
      margin-bottom:12px; outline:none; transition:border-color .2s;
    }
    .login-card input:focus { border-color:var(--accent); }
    .login-card button {
      width:100%; padding:14px; border:none; border-radius:12px;
      background:linear-gradient(135deg,var(--accent),#a78bfa); color:#fff;
      font-size:15px; font-weight:600; cursor:pointer; transition:opacity .2s;
    }
    .login-card button:hover { opacity:.9; }
    .login-card button:disabled { opacity:.5; cursor:not-allowed; }
    .login-card .error { color:var(--bad); font-size:13px; margin-top:8px; display:none; }
    .login-card .info { text-align:center; margin-top:12px; font-size:12px; color:var(--muted2); }
    .login-card .spinner { display:none; width:18px; height:18px; border:2px solid rgba(255,255,255,.3);
      border-top-color:#fff; border-radius:50%; animation:spin .6s linear infinite;
      margin:0 auto; }
    @keyframes spin { to{transform:rotate(360deg)} }
  </style>
</head>
<body>
  <div class="login-card" id="card">
    <div class="logo"><i class="fas fa-tasks"></i></div>
    <h1>Task Queue</h1>
    <p>Sign in to manage jobs, workers, and monitor the system</p>
    <input id="login-user" type="text" placeholder="Username" value="admin" autocomplete="username" />
    <input id="login-pass" type="password" placeholder="Password" value="admin123" autocomplete="current-password" />
    <button id="login-btn" onclick="login()">Sign In</button>
    <div id="login-error" class="error">Invalid credentials</div>
    <div class="info"><i class="fas fa-info-circle"></i> Default: admin / admin123</div>
  </div>
  <script>
    const TOKEN_KEY='task_queue_api_key';
    if(localStorage.getItem(TOKEN_KEY)){window.location.href='/';}
    async function login(){
      const btn=document.getElementById('login-btn');
      const err=document.getElementById('login-error');
      const user=document.getElementById('login-user').value.trim();
      const pass=document.getElementById('login-pass').value.trim();
      if(!user||!pass){err.style.display='block';err.textContent='Username and password required';return;}
      btn.disabled=true;btn.innerHTML='<div class="spinner" style="display:inline-block"></div>';
      err.style.display='none';
      try{
        const res=await fetch('/api/v1/login',{
          method:'POST',
          body:JSON.stringify({username:user,password:pass}),
          headers:{'Content-Type':'application/json'}
        });
        const data=await res.json();
        if(res.ok&&data.api_key){
          localStorage.setItem(TOKEN_KEY,data.api_key);
          window.location.href='/';
        }else{
          err.style.display='block';err.textContent='Invalid credentials';
        }
      }catch(e){
        err.style.display='block';err.textContent='Connection error';
      }
      btn.disabled=false;btn.innerHTML='Sign In';
    }
    document.getElementById('login-pass').addEventListener('keydown',function(e){
      if(e.key==='Enter')login();
    });
  </script>
</body>
</html>`
