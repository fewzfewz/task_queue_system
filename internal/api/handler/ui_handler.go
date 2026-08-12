package handler

import (
	"fmt"
	"net/http"
)

// ServeAppUI renders the general operator UI for the queue system. The page
// contains no secrets; it authenticates against the session API on load.
func (h *JobHandler) ServeAppUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, appHTML)
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
  <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@400;500;600;700;800&family=Inter:wght@400;500;600&display=swap" rel="stylesheet">
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css">
  <style>
    * { margin:0; padding:0; box-sizing:border-box; }
    :root {
      --bg:#070a14; --bg2:#0f1629; --surface:rgba(26,34,54,0.65); --surface2:rgba(31,42,64,0.8);
      --border:rgba(255,255,255,0.08); --border-hover:rgba(255,255,255,0.15); --text:#f8fafc; --muted:#94a3b8; --muted2:#64748b;
      --accent:#6366f1; --accent2:#818cf8; --accent3:#c084fc;
      /* severity palette — consistent everywhere */
      --sev-ok:#34d399; --sev-ok-bg:rgba(52,211,153,.12); --sev-ok-border:rgba(52,211,153,.35);
      --sev-warn:#fbbf24; --sev-warn-bg:rgba(251,191,36,.12); --sev-warn-border:rgba(251,191,36,.35);
      --sev-crit:#f87171; --sev-crit-bg:rgba(248,113,113,.12); --sev-crit-border:rgba(248,113,113,.4);
      --sev-info:#93c5fd; --sev-info-bg:rgba(147,197,253,.1); --sev-info-border:rgba(147,197,253,.3);
      --good:var(--sev-ok); --bad:var(--sev-crit); --warn:var(--sev-warn);
      --sidebar-w:260px; --header-h:70px;
    }
    body {
      font-family:'Outfit','Inter',-apple-system,sans-serif; background:var(--bg); color:var(--text);
      min-height:100vh; overflow-x:hidden;
    }
    ::-webkit-scrollbar { width:6px; }
    ::-webkit-scrollbar-track { background:transparent; }
    ::-webkit-scrollbar-thumb { background:var(--border); border-radius:3px; }

    /* ── Layout ── */
    .layout { display:flex; min-height:100vh; }
    #sidebar {
      width:var(--sidebar-w); background:rgba(21,27,43,0.85); border-right:1px solid var(--border);
      position:fixed; top:0; left:0; height:100vh; z-index:100;
      display:flex; flex-direction:column; transition:transform .3s cubic-bezier(0.4, 0, 0.2, 1);
      backdrop-filter:blur(20px);
    }
    #sidebar.closed { transform:translateX(-100%); }
    .sidebar-brand {
      padding:20px 20px 16px; border-bottom:1px solid var(--border);
      display:flex; align-items:center; gap:12px;
    }
    .sidebar-brand .brand-icon {
      width:44px; height:44px; background:linear-gradient(135deg,var(--accent),var(--accent3));
      border-radius:12px; display:flex; align-items:center; justify-content:center;
      box-shadow:0 8px 24px rgba(99,102,241,.3), inset 0 2px 4px rgba(255,255,255,.2);
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
    .status-dot.yellow { background:var(--warn); box-shadow:0 0 8px rgba(245,158,11,.4); }

    #content { padding:24px; max-width:1360px; margin:0 auto; }
    .page { display:none; }
    .page.active { display:block; animation:fadeIn .25s ease; }
    .hidden { display:none !important; }
    @keyframes fadeIn { from{opacity:0;transform:scale(.96)} to{opacity:1;transform:scale(1)} }

    /* ── Stats Cards ── */
    .stats-grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(180px,1fr)); gap:16px; margin-bottom:24px; }
    .stat-card {
      background:var(--surface); border:1px solid var(--border); border-radius:20px;
      padding:20px 24px; transition:transform 0.3s ease, box-shadow 0.3s ease, border-color 0.2s;
      backdrop-filter:blur(16px);
      box-shadow:0 4px 24px rgba(0,0,0,0.1);
    }
    .stat-card:hover { transform:translateY(-2px); box-shadow:0 8px 32px rgba(0,0,0,0.2); border-color:var(--border-hover); }
    .stat-card .stat-label { font-size:12px; color:var(--muted2); text-transform:uppercase;
      letter-spacing:.1em; font-weight:700; }
    .stat-card .stat-value { font-size:32px; font-weight:800; margin-top:8px; }
    .stat-card .stat-desc { font-size:13px; color:var(--muted); margin-top:4px; }

    /* ── Cards & Sections ── */
    .section-card {
      background:var(--surface); border:1px solid var(--border); border-radius:20px;
      padding:24px; margin-bottom:20px; backdrop-filter:blur(16px);
      box-shadow:0 4px 24px rgba(0,0,0,0.1); transition:transform 0.3s ease, box-shadow 0.3s ease;
    }
    .section-card:hover { transform:translateY(-1px); box-shadow:0 8px 32px rgba(0,0,0,0.15); border-color:var(--border-hover); }
    .section-card .section-title {
      font-size:14px; font-weight:700; color:var(--accent2); text-transform:uppercase;
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
    input:focus, select:focus, textarea:focus { border-color:var(--accent); box-shadow:0 0 0 3px rgba(99,102,241,.25); }
    button:focus-visible, .btn:focus-visible, .nav-item:focus-visible {
      outline:2px solid var(--accent2); outline-offset:2px;
    }
    input:focus-visible, select:focus-visible, textarea:focus-visible {
      outline:2px solid var(--accent2); outline-offset:0;
    }
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
      font-size:11px; font-weight:600; background:var(--sev-info-bg); color:var(--sev-info); border:1px solid var(--sev-info-border); }
    .pill.good { background:var(--sev-ok-bg); color:var(--sev-ok); border-color:var(--sev-ok-border); }
    .pill.bad { background:var(--sev-crit-bg); color:var(--sev-crit); border-color:var(--sev-crit-border); }
    .pill.warn { background:var(--sev-warn-bg); color:var(--sev-warn); border-color:var(--sev-warn-border); }
    .badge { display:inline-block; width:8px; height:8px; border-radius:50%; margin-right:6px; }
    .badge.green { background:var(--good); }
    .badge.red { background:var(--bad); }
    .badge.yellow { background:var(--warn); }
    .pagination { display:flex; gap:8px; align-items:center; margin-top:12px; }
    .toolbar { display:flex; gap:8px; flex-wrap:wrap; align-items:center; }

    /* ── CB chips ── */
    .cb-chips { display:flex; flex-wrap:wrap; gap:10px; }
    .cb-chip {
      display:flex; align-items:center; gap:10px; padding:10px 14px;
      background:var(--bg2); border:1px solid var(--border); border-radius:12px;
      font-size:12px; font-weight:500;
    }
    .cb-chip .dot { width:10px; height:10px; border-radius:50%; }
    .cb-chip.closed .dot { background:var(--good); box-shadow:0 0 8px rgba(34,197,94,.5); }
    .cb-chip.open .dot { background:var(--bad); box-shadow:0 0 8px rgba(239,68,68,.5); }
    .cb-chip.half .dot { background:var(--warn); box-shadow:0 0 8px rgba(245,158,11,.5); }
    .cb-chip.closed { border-color:rgba(34,197,94,.3); }
    .cb-chip.open { border-color:rgba(239,68,68,.4); }
    .cb-chip.half { border-color:rgba(245,158,11,.4); }
    .cb-chip .actions { display:flex; gap:6px; }

    /* ── Live feed ── */
    #feed-body { max-height:340px; overflow-y:auto; }
    .feed-item {
      display:flex; align-items:center; gap:12px; padding:8px 4px;
      border-bottom:1px solid rgba(42,53,85,.4); font-size:12.5px;
    }
    .feed-item:last-child { border-bottom:none; }
    .feed-item .time { color:var(--muted2); font-size:11px; font-family:'SF Mono',monospace; min-width:66px; }
    .feed-item .kind {
      min-width:82px; text-align:center; font-size:10px; font-weight:700;
      text-transform:uppercase; letter-spacing:.05em; padding:3px 8px; border-radius:999px;
    }
    .feed-item .kind.job { background:rgba(99,102,241,.12); color:var(--accent2); }
    .feed-item .kind.rate_limit { background:rgba(245,158,11,.12); color:#fbbf24; }
    .feed-item .kind.circuit_breaker { background:rgba(239,68,68,.12); color:#f87171; }
    .feed-item .kind.dlq { background:var(--sev-crit-bg); color:var(--sev-crit); }

    /* ── Standard states ── */
    .state-box {
      display:flex; align-items:center; gap:10px; padding:14px 16px;
      border-radius:12px; font-size:13px; border:1px solid var(--border);
      background:var(--bg2); color:var(--muted);
    }
    .state-box.loading { border-color:var(--sev-info-border); color:var(--sev-info); }
    .state-box.empty { border-style:dashed; }
    .state-box.error { border-color:var(--sev-crit-border); background:var(--sev-crit-bg); color:var(--sev-crit); }
    .state-box .spinner-sm {
      width:16px; height:16px; border:2px solid rgba(147,197,253,.3);
      border-top-color:var(--sev-info); border-radius:50%;
      animation:spin .7s linear infinite; flex-shrink:0;
    }
    @keyframes spin { to{transform:rotate(360deg)} }

    /* ── Health banner ── */
    #health-banner {
      border-radius:16px; padding:16px 20px; margin-bottom:16px;
      border:1px solid var(--border); background:var(--surface);
      transition:border-color .3s, background .3s;
    }
    #health-banner.sev-ok { border-color:var(--sev-ok-border); }
    #health-banner.sev-warn { border-color:var(--sev-warn-border); background:rgba(251,191,36,.04); }
    #health-banner.sev-crit { border-color:var(--sev-crit-border); background:rgba(248,113,113,.05); }
    .health-head { display:flex; align-items:center; justify-content:space-between; gap:12px; flex-wrap:wrap; margin-bottom:12px; }
    .health-head .health-title { font-size:15px; font-weight:700; display:flex; align-items:center; gap:10px; }
    .health-head .health-title i { font-size:18px; }
    .health-metrics { display:grid; grid-template-columns:repeat(auto-fit,minmax(130px,1fr)); gap:10px; }
    .health-metric {
      background:var(--bg2); border:1px solid var(--border); border-radius:12px;
      padding:10px 12px; min-width:0;
    }
    .health-metric .hm-label { font-size:10px; color:var(--muted2); text-transform:uppercase; letter-spacing:.06em; font-weight:600; }
    .health-metric .hm-value { font-size:22px; font-weight:700; margin-top:2px; line-height:1.1; }
    .health-metric .hm-sub { font-size:11px; color:var(--muted); margin-top:2px; }
    .health-metric.crit .hm-value { color:var(--sev-crit); }
    .health-metric.warn .hm-value { color:var(--sev-warn); }
    .health-metric.ok .hm-value { color:var(--sev-ok); }
    .tier-bars { display:flex; gap:6px; margin-top:6px; height:6px; border-radius:999px; overflow:hidden; background:var(--border); }
    .tier-bars span { display:block; height:100%; border-radius:999px; min-width:2px; }
    .tier-bars .t-high { background:var(--sev-crit); }
    .tier-bars .t-med { background:var(--sev-warn); }
    .tier-bars .t-low { background:var(--sev-info); }

    /* ── SSE status bar ── */
    #sse-bar {
      display:none; align-items:center; gap:10px; padding:10px 16px;
      border-radius:10px; margin-bottom:16px; font-size:13px; font-weight:500;
    }
    #sse-bar.show { display:flex; }
    #sse-bar.connected { background:var(--sev-ok-bg); border:1px solid var(--sev-ok-border); color:var(--sev-ok); }
    #sse-bar.reconnecting { background:var(--sev-warn-bg); border:1px solid var(--sev-warn-border); color:var(--sev-warn); }
    #sse-bar.disconnected { background:var(--sev-crit-bg); border:1px solid var(--sev-crit-border); color:var(--sev-crit); }
    #sse-bar .sse-dot { width:8px; height:8px; border-radius:50%; flex-shrink:0; }
    #sse-bar.connected .sse-dot { background:var(--sev-ok); box-shadow:0 0 8px var(--sev-ok); }
    #sse-bar.reconnecting .sse-dot { background:var(--sev-warn); animation:pulse 1.5s ease-in-out infinite; }
    #sse-bar.disconnected .sse-dot { background:var(--sev-crit); }

    .stat-card.sev-warn { border-color:var(--sev-warn-border); }
    .stat-card.sev-crit { border-color:var(--sev-crit-border); }
    .stat-card.sev-warn .stat-value { color:var(--sev-warn); }
    .stat-card.sev-crit .stat-value { color:var(--sev-crit); }

    /* ── Modal ── */
    #modal-overlay {
      position:fixed; inset:0; background:rgba(4,7,18,.72); z-index:500;
      display:none; align-items:flex-start; justify-content:center; overflow-y:auto;
      padding:48px 20px;
    }
    #modal-overlay.show { display:flex; }
    .modal {
      background:var(--surface); border:1px solid var(--border); border-radius:16px;
      width:760px; max-width:100%; animation:fadeIn .2s ease;
    }
    .modal .modal-head {
      display:flex; align-items:center; justify-content:space-between;
      padding:18px 22px; border-bottom:1px solid var(--border);
    }
    .modal .modal-head h3 { font-size:15px; font-weight:600; }
    .modal .modal-head button {
      background:none; border:none; color:var(--muted); font-size:16px; cursor:pointer;
      padding:4px 8px; border-radius:8px;
    }
    .modal .modal-head button:hover { background:var(--surface2); color:var(--bad); }
    .modal .modal-body { padding:20px 22px; }
    .modal .modal-foot {
      display:flex; gap:10px; padding:14px 22px; border-top:1px solid var(--border);
      justify-content:flex-end;
    }

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
      .health-metrics { grid-template-columns:repeat(2,1fr); }
      #content { padding:16px; }
      #sidebar { width:280px; }
      table { display:block; overflow-x:auto; white-space:nowrap; }
    }
    @media(max-width:1100px) and (min-width:769px) {
      .health-metrics { grid-template-columns:repeat(3,1fr); }
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
  <div id="modal-overlay"><div class="modal">
    <div class="modal-head"><h3 id="modal-title">Details</h3><button onclick="closeModal()"><i class="fas fa-times"></i></button></div>
    <div class="modal-body" id="modal-body"></div>
    <div class="modal-foot" id="modal-foot"></div>
  </div></div>

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
        <a class="nav-item" data-page="jobs" data-admin onclick="showPage('jobs')">
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
        <a class="nav-item" data-page="clients" data-admin onclick="showPage('clients');loadClients()">
          <i class="fas fa-users"></i> Clients
        </a>
        <a class="nav-item" data-page="webhooks" data-admin onclick="showPage('webhooks');loadWebhooks()">
          <i class="fas fa-globe"></i> Webhooks
        </a>
        <a class="nav-item" data-page="jobtypes" data-admin onclick="showPage('jobtypes');loadJobTypes()">
          <i class="fas fa-tags"></i> Job Types
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
          <span id="status-indicator" title="SSE connection status"><span class="status-dot yellow"></span> <span id="status-label">Connecting</span></span>
          <button onclick="loadEverything()" aria-label="Refresh all data"><i class="fas fa-sync-alt"></i> Refresh</button>
        </div>
      </div>
      <div id="content">

        <div id="ro-banner" style="display:none;background:rgba(245,158,11,.1);border:1px solid var(--warn);color:var(--warn);border-radius:10px;padding:10px 16px;font-size:13px;margin-bottom:16px;">
          <i class="fas fa-eye"></i> Read-only session. Changes require an admin account.
        </div>

        <!-- Page: Dashboard -->
        <div id="page-dashboard" class="page active">
          <div id="sse-bar" class="reconnecting" role="status" aria-live="polite">
            <span class="sse-dot"></span>
            <span id="sse-bar-text">Connecting to live event stream…</span>
          </div>

          <div id="health-banner" class="sev-ok" role="status" aria-live="polite">
            <div class="health-head">
              <div class="health-title"><i class="fas fa-heart-pulse" id="health-icon"></i> <span id="health-summary">Assessing system health…</span></div>
              <span id="health-updated" style="font-size:11px;color:var(--muted2);">—</span>
            </div>
            <div class="health-metrics" id="health-metrics">
              <div class="health-metric" id="hm-pending"><div class="hm-label">Queue Depth</div><div class="hm-value">—</div><div class="hm-sub" id="hm-tier-sub">high / med / low</div><div class="tier-bars" id="hm-tier-bars"><span class="t-high" style="width:33%"></span><span class="t-med" style="width:33%"></span><span class="t-low" style="width:34%"></span></div></div>
              <div class="health-metric" id="hm-dlq"><div class="hm-label">DLQ Failed</div><div class="hm-value">—</div><div class="hm-sub">permanent failures</div></div>
              <div class="health-metric" id="hm-cb"><div class="hm-label">Circuits Open</div><div class="hm-value">—</div><div class="hm-sub">plugin breakers</div></div>
              <div class="health-metric" id="hm-rl"><div class="hm-label">Rate Limited</div><div class="hm-value">—</div><div class="hm-sub">tenants at cap</div></div>
              <div class="health-metric" id="hm-workers"><div class="hm-label">Workers</div><div class="hm-value">—</div><div class="hm-sub">active heartbeats</div></div>
              <div class="health-metric" id="hm-api"><div class="hm-label">API Health</div><div class="hm-value">—</div><div class="hm-sub">/healthz · /readyz</div></div>
            </div>
          </div>

          <div class="stats-grid">
            <div class="stat-card" id="card-pending"><div class="stat-label">Pending Jobs</div><div id="stat-pending" class="stat-value">—</div><div class="stat-desc">Queued work</div></div>
            <div class="stat-card" id="card-dlq"><div class="stat-label">Failed Jobs</div><div id="stat-dlq" class="stat-value">—</div><div class="stat-desc">Dead letter queue</div></div>
            <div class="stat-card" id="card-cb"><div class="stat-label">Open Circuits</div><div id="stat-cb" class="stat-value">—</div><div class="stat-desc">Circuit breakers open</div></div>
            <div class="stat-card" id="card-rl"><div class="stat-label">Rate Limited</div><div id="stat-rl" class="stat-value">—</div><div class="stat-desc">Tenants at limit</div></div>
          </div>

          <div class="grid-2" style="align-items:start;">
            <div>
              <div class="section-card slide-up">
                <div class="section-title"><i class="fas fa-gauge-high"></i> Rate Limits <span style="text-transform:none;font-size:11px;">(fixed window)</span></div>
                <div id="rl-state"><div class="state-box loading"><span class="spinner-sm"></span> Loading rate limits…</div></div>
              </div>
              <div class="section-card slide-up">
                <div class="section-title"><i class="fas fa-shield-halved"></i> Circuit Breakers</div>
                <div id="cb-state"><div class="state-box loading"><span class="spinner-sm"></span> Loading circuit breakers…</div></div>
              </div>
            </div>
            <div>
              <div class="section-card slide-up">
                <div class="section-title"><i class="fas fa-bolt"></i> Live Activity <span id="feed-status" class="pill warn" style="float:right;" aria-live="polite">connecting</span></div>
                <div id="feed-body"><div class="state-box empty"><i class="fas fa-satellite-dish"></i> Waiting for events…</div></div>
              </div>
              <div class="section-card slide-up">
                <div class="section-title"><i class="fas fa-bolt"></i> Quick Actions</div>
                <div class="toolbar">
                  <button class="btn-primary btn-sm" data-admin onclick="showPage('jobs')"><i class="fas fa-plus"></i> Create Job</button>
                  <button class="btn-secondary btn-sm" onclick="showPage('search')"><i class="fas fa-search"></i> Search Jobs</button>
                  <button class="btn-secondary btn-sm" onclick="showPage('dlq');loadDLQ()"><i class="fas fa-trash"></i> DLQ</button>
                  <button class="btn-secondary btn-sm" onclick="showPage('stats');loadStats()"><i class="fas fa-chart-bar"></i> Stats</button>
                  <button class="btn-secondary btn-sm" onclick="showPage('workers');loadWorkers()"><i class="fas fa-server"></i> Workers</button>
                  <button class="btn-secondary btn-sm" onclick="showPage('webhooks');loadWebhooks()"><i class="fas fa-globe"></i> Webhooks</button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Page: Create Jobs -->
        <div id="page-jobs" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-plus-circle"></i> Create Job</div>
            <div class="grid-2">
              <div>
                <div class="form-group"><label>Job Type</label>
                  <select id="create-type" onchange="adminOnJobTypeChange()">
                    <option value="email">email</option>
                    <option value="image">image</option>
                    <option value="http">http</option>
                  </select>
                </div>
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
                <div class="section-title" style="display:flex;justify-content:space-between;align-items:center;margin-bottom:10px">
                  <span id="admin-payload-title">Job Payload</span>
                  <button type="button" class="btn-secondary btn-sm" onclick="adminTogglePayloadMode()" style="font-size:11px"><i class="fas fa-code"></i> JSON</button>
                </div>
                <div id="admin-payload-fields"></div>
                <div id="admin-payload-json-wrap" class="hidden">
                  <textarea id="create-payload">{"to":"user@example.com","body":"Welcome!"}</textarea>
                </div>
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
                <button class="btn-primary" onclick="searchJobs()"><i class="fas fa-search"></i> Search</button>
                <button class="btn-secondary" onclick="prevPage()"><i class="fas fa-chevron-left"></i></button>
                <button class="btn-secondary" onclick="nextPage()"><i class="fas fa-chevron-right"></i></button>
              </div>
            </div>
            <table>
              <thead><tr><th>ID</th><th>Type</th><th>Status</th><th>Priority</th><th>Tenant</th><th>Retries</th><th>Deps</th><th>Created</th><th></th></tr></thead>
              <tbody id="search-body"><tr><td colspan="9"><div class="state-box empty"><i class="fas fa-search"></i> Run a search to see jobs.</div></td></tr></tbody>
            </table>
          </div>
        </div>

        <!-- Page: Stats -->
        <div id="page-stats" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-layer-group"></i> Priority &amp; Partition Depths</div>
            <div id="stats-priority-view"><div class="state-box loading"><span class="spinner-sm"></span> Loading priority breakdown…</div></div>
          </div>
          <div class="section-card">
            <div class="section-title"><i class="fas fa-chart-bar"></i> Queue Statistics (raw)</div>
            <pre id="stats-output"><span style="color:var(--muted);">Click Refresh or navigate here to load.</span></pre>
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
                <div class="section-title" style="font-size:11px;">Upstream (depends on) — blocked until these complete</div>
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
            <pre id="workers-output"><span style="color:var(--muted);">Navigate here to load worker data.</span></pre>
          </div>
          <div class="section-card">
            <div class="section-title"><i class="fas fa-chart-line"></i> Prometheus Metrics</div>
            <pre id="metrics-output"><span style="color:var(--muted);">Navigate here to load metrics.</span></pre>
          </div>
          <div class="section-card">
            <div class="section-title"><i class="fas fa-heartbeat"></i> Health Checks</div>
            <div id="health-status"><div class="state-box empty"><i class="fas fa-stethoscope"></i> Health checks not run yet.</div></div>
          </div>
        </div>

        <!-- Page: Circuit Breaker -->
        <div id="page-cb" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-shield-alt"></i> Circuit Breaker Status</div>
            <table><thead><tr><th>Plugin Type</th><th>State</th><th>Action</th></tr></thead><tbody id="cb-body"><tr><td colspan="3"><div class="state-box empty"><i class="fas fa-shield-alt"></i> No circuit breaker data loaded.</div></td></tr></tbody></table>
          </div>
        </div>

        <!-- Page: DLQ -->
        <div id="page-dlq" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-trash-alt"></i> Dead Letter Queue</div>
            <div class="grid-3" style="margin-bottom:12px;">
              <div class="form-group"><label>Queue Filter</label><input id="dlq-queue" value="email" /></div>
              <div class="form-group"><label>Tenant Filter</label><input id="dlq-tenant" value="tenant-a" /></div>
              <div class="form-group"><label>Page</label><input id="dlq-page" type="number" value="1" /></div>
            </div>
            <div class="toolbar" style="margin-bottom:12px;">
              <button class="btn-primary btn-sm" onclick="loadDLQ()"><i class="fas fa-sync"></i> Load DLQ</button>
              <button class="btn-secondary btn-sm" onclick="dlqPrevPage()"><i class="fas fa-chevron-left"></i> Prev</button>
              <button class="btn-secondary btn-sm" onclick="dlqNextPage()"><i class="fas fa-chevron-right"></i> Next</button>
              <span id="dlq-page-info" style="font-size:12px;color:var(--muted);">Page 1</span>
              <button class="btn-secondary btn-sm" onclick="exportDLQ()"><i class="fas fa-download"></i> Export JSON</button>
            </div>
            <table>
              <thead><tr><th>ID</th><th>Type</th><th>Tenant</th><th>Error</th><th>Retries</th><th>Failed At</th><th></th></tr></thead>
              <tbody id="dlq-body"><tr><td colspan="7"><div class="state-box empty"><i class="fas fa-trash-alt"></i> Load DLQ to see failed jobs.</div></td></tr></tbody>
            </table>
            <div class="grid-2" style="margin-top:12px;">
              <div class="form-row">
                <div class="form-group"><label>Job ID to Replay</label><input id="dlq-replay-id" value="job-123" /></div>
                <div style="display:flex;align-items:flex-end;"><button class="btn-primary btn-sm" data-admin onclick="replayDLQ()"><i class="fas fa-redo"></i> Replay</button></div>
              </div>
              <div class="form-row">
                <div class="form-group"><label>Job ID to Purge</label><input id="dlq-purge-id" value="job-123" /></div>
                <div style="display:flex;align-items:flex-end;"><button class="btn-danger btn-sm" data-admin onclick="deleteDLQ()"><i class="fas fa-times"></i> Purge</button></div>
              </div>
            </div>
            <div class="form-row" style="margin-top:12px;grid-template-columns:1fr auto;">
              <div class="form-group"><label>Bulk Purge (older than)</label><input id="dlq-older" value="2025-01-01T00:00:00Z" /></div>
              <div style="display:flex;align-items:flex-end;"><button class="btn-danger btn-sm" data-admin onclick="bulkPurgeDLQ()"><i class="fas fa-trash"></i> Bulk Purge</button></div>
            </div>
          </div>
        </div>

        <!-- Page: Clients -->
        <div id="page-clients" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-users"></i> Registered Clients (Tenants)</div>
            <p style="font-size:13px;color:var(--muted);margin-bottom:16px;">
              This section lists all client API keys registered in the system along with their tenant IDs.
            </p>
            <div style="overflow-x:auto;">
              <table style="margin:0;width:100%;">
                <thead><tr><th>Tenant ID</th><th>Registration Date</th><th>Actions</th></tr></thead>
                <tbody id="clients-body">
                  <tr><td colspan="3" style="text-align:center;padding:30px;color:var(--muted);">Loading clients...</td></tr>
                </tbody>
              </table>
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
            <div class="form-group"><label>Tenant ID <span style="color:var(--muted2)">(required for operator sessions)</span></label><input id="wh-tenant" placeholder="my-tenant" /></div>
            <button class="btn-primary" data-admin onclick="registerWebhook()"><i class="fas fa-plus"></i> Register</button>
          </div>
          <div class="section-card">
            <div class="section-title"><i class="fas fa-list"></i> Registered Webhooks</div>
            <table>
              <thead><tr><th>ID</th><th>URL</th><th>Events</th><th>Created</th><th></th></tr></thead>
              <tbody id="wh-body"><tr><td colspan="5"><div class="state-box empty"><i class="fas fa-globe"></i> No webhooks loaded yet.</div></td></tr></tbody>
            </table>
          </div>
        </div>

        <!-- Page: Job Types -->
        <div id="page-jobtypes" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-tags"></i> Register Custom Job Type</div>
            <p style="font-size:13px;color:var(--muted);margin-bottom:14px">
              Built-in types: <code>email</code> (simulated SMTP), <code>image</code> (image processing), <code>http</code> (HTTP callback).
              Register custom types that delegate to the <code>http</code> handler for real-world integrations.
            </p>
            <div class="grid-3">
              <div class="form-group"><label>Name</label><input id="jt-name" placeholder="slack-notify" /></div>
              <div class="form-group"><label>Handler</label><select id="jt-handler"><option value="http">http</option><option value="email">email</option><option value="image">image</option></select></div>
              <div class="form-group"><label>Payload Hint</label><input id="jt-hint" placeholder='{"url":"https://..."}' /></div>
            </div>
            <div class="form-group"><label>Description</label><input id="jt-desc" placeholder="Send Slack notification via webhook" /></div>
            <button class="btn-primary" data-admin onclick="createJobType()"><i class="fas fa-plus"></i> Add Job Type</button>
          </div>
          <div class="section-card">
            <div class="section-title"><i class="fas fa-list"></i> Available Job Types</div>
            <table>
              <thead><tr><th>Name</th><th>Handler</th><th>Description</th><th>Built-in</th><th></th></tr></thead>
              <tbody id="jt-body"><tr><td colspan="5"><div class="state-box empty"><i class="fas fa-tags"></i> No job types loaded yet.</div></td></tr></tbody>
            </table>
          </div>
        </div>

      </div>
    </div>
  </div>
  </div>

  <script>
    const API_BASE = '/api/v1';
    const IDLE_TIMEOUT = 15 * 60 * 1000;
    const PAGE_NAMES = {
      dashboard: 'Dashboard',
      jobs: 'Create Jobs',
      search: 'Search Jobs',
      stats: 'Queue Statistics',
      dag: 'DAG Dependencies',
      workers: 'Workers \u0026 Health',
      cb: 'Circuit Breaker',
      dlq: 'Dead Letter Queue',
      clients: 'Clients',
      webhooks: 'Webhooks',
      jobtypes: 'Job Types'
    };
    const PAGE_DESCS = {
      dashboard: 'System overview and real-time statistics',
      jobs: 'Submit new tasks to the queue',
      search: 'Find and inspect queued jobs',
      stats: 'Detailed queue performance metrics',
      dag: 'Visualize job dependency chains',
      workers: 'Monitor worker processes and health endpoints',
      cb: 'Track circuit breaker states per plugin',
      dlq: 'Manage failed jobs and retries',
      clients: 'Manage tenant credentials',
      webhooks: 'Configure event-driven HTTP callbacks',
      jobtypes: 'Register custom job types for workers'
    };
    let SESSION = { authenticated:false, username:'', role:'', csrf_token:'' };
    let idleTimer = null;
    let sseState = 'connecting'; // connecting | connected | reconnecting | disconnected
    let healthCache = { pending:0, dlq:0, cb:0, rl:0, workers:0, apiOk:false, tiers:{high:0,medium:0,low:0} };

    /* ── Standard state renderers ── */
    function stateLoading(msg){ return '<div class="state-box loading"><span class="spinner-sm"></span> '+esc(msg||'Loading…')+'</div>'; }
    function stateEmpty(icon,msg){ return '<div class="state-box empty"><i class="fas '+icon+'"></i> '+esc(msg)+'</div>'; }
    function stateError(msg){ return '<div class="state-box error"><i class="fas fa-exclamation-triangle"></i> '+esc(msg)+'</div>'; }

    function toast(msg, type) {
      const t=document.getElementById('toast');
      t.textContent=msg;
      t.style.borderColor=type==='error'?'var(--sev-crit-border)':type==='warn'?'var(--sev-warn-border)':'var(--border)';
      t.classList.add('show'); setTimeout(()=>t.classList.remove('show'),3000);
    }
    function esc(s) {
      return String(s==null?'':s).replace(/[&<>"']/g,function(c){
        return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];
      });
    }
    function isAdmin() { return SESSION.role === 'admin'; }
    function confirmAction(message) { return window.confirm(message); }

    async function api(path,options={}) {
      const method=options.method||'GET';
      const headers=Object.assign({'Content-Type':'application/json'},options.headers||{});
      if(method!=='GET'&&method!=='HEAD'&&SESSION.csrf_token){headers['X-CSRF-Token']=SESSION.csrf_token;}
      try {
        const res=await fetch(path,Object.assign({},options,{headers:headers,credentials:'same-origin'}));
        if(res.status===204)return {_ok:true,_status:204};
        const text=await res.text();
        let data;
        try{data=JSON.parse(text);}catch(_){data=text}
        if(!res.ok){
          const errMsg=(data&&data.error)?data.error:('HTTP '+res.status);
          return {_ok:false,_status:res.status,_error:errMsg,data:data};
        }
        if(typeof data==='object'&&data!==null){data._ok=true;data._status=res.status;}
        return data;
      }catch(e){return {_ok:false,_status:0,_error:'Network error — check connection'};}
    }

    /* ── SSE status ── */
    function setSSEState(state) {
      sseState=state;
      const bar=document.getElementById('sse-bar');
      const barText=document.getElementById('sse-bar-text');
      const feedPill=document.getElementById('feed-status');
      const dot=document.querySelector('#status-indicator .status-dot');
      const label=document.getElementById('status-label');
      bar.className='show '+state;
      if(state==='connected'){
        barText.textContent='Live — data updates automatically via SSE';
        feedPill.textContent='live'; feedPill.className='pill good';
        dot.className='status-dot green'; label.textContent='Live';
      }else if(state==='reconnecting'){
        barText.textContent='Reconnecting — data may be stale. Click Refresh to update manually.';
        feedPill.textContent='reconnecting'; feedPill.className='pill warn';
        dot.className='status-dot yellow'; label.textContent='Reconnecting';
      }else if(state==='disconnected'){
        barText.textContent='Disconnected — showing last known data. Click Refresh.';
        feedPill.textContent='offline'; feedPill.className='pill bad';
        dot.className='status-dot red'; label.textContent='Offline';
      }else{
        barText.textContent='Connecting to live event stream…';
        feedPill.textContent='connecting'; feedPill.className='pill warn';
        dot.className='status-dot yellow'; label.textContent='Connecting';
      }
    }

    /* ── Health banner ── */
    function setMetricSeverity(id, val, warnAt, critAt) {
      const el=document.getElementById(id);
      if(!el)return;
      el.classList.remove('ok','warn','crit');
      if(val>=critAt)el.classList.add('crit');
      else if(val>=warnAt)el.classList.add('warn');
      else el.classList.add('ok');
    }
    function setStatCardSeverity(cardId, sev) {
      const el=document.getElementById(cardId);
      if(!el)return;
      el.classList.remove('sev-warn','sev-crit');
      if(sev)el.classList.add('sev-'+sev);
    }
    function updateHealthBanner() {
      const h=healthCache;
      const issues=[];
      if(h.dlq>0)issues.push(h.dlq+' failed in DLQ');
      if(h.cb>0)issues.push(h.cb+' circuit'+(h.cb>1?'s':'')+' open');
      if(h.rl>0)issues.push(h.rl+' tenant'+(h.rl>1?'s':'')+' rate-limited');
      if(h.workers===0)issues.push('no active workers');
      if(!h.apiOk)issues.push('API health check failing');
      if(sseState==='disconnected'||sseState==='reconnecting')issues.push('live feed '+sseState);

      const banner=document.getElementById('health-banner');
      const icon=document.getElementById('health-icon');
      const summary=document.getElementById('health-summary');
      if(issues.length===0){
        banner.className='sev-ok';
        icon.className='fas fa-heart-pulse'; icon.style.color='var(--sev-ok)';
        summary.textContent='All systems operational';
      }else if(h.dlq>10||h.cb>0||!h.apiOk||h.workers===0){
        banner.className='sev-crit';
        icon.className='fas fa-triangle-exclamation'; icon.style.color='var(--sev-crit)';
        summary.textContent='Critical: '+issues.join(' · ');
      }else{
        banner.className='sev-warn';
        icon.className='fas fa-circle-exclamation'; icon.style.color='var(--sev-warn)';
        summary.textContent='Attention: '+issues.join(' · ');
      }
      document.getElementById('health-updated').textContent='Updated '+new Date().toLocaleTimeString();

      document.querySelector('#hm-pending .hm-value').textContent=String(h.pending);
      document.querySelector('#hm-dlq .hm-value').textContent=String(h.dlq);
      document.querySelector('#hm-cb .hm-value').textContent=String(h.cb);
      document.querySelector('#hm-rl .hm-value').textContent=String(h.rl);
      document.querySelector('#hm-workers .hm-value').textContent=String(h.workers);
      document.querySelector('#hm-api .hm-value').textContent=h.apiOk?'OK':'FAIL';
      document.querySelector('#hm-api .hm-value').style.color=h.apiOk?'var(--sev-ok)':'var(--sev-crit)';

      const tierSub=document.getElementById('hm-tier-sub');
      tierSub.textContent='H:'+h.tiers.high+' M:'+h.tiers.medium+' L:'+h.tiers.low;
      const total=h.tiers.high+h.tiers.medium+h.tiers.low||1;
      document.getElementById('hm-tier-bars').innerHTML=
        '<span class="t-high" style="width:'+Math.round(h.tiers.high/total*100)+'%"></span>'+
        '<span class="t-med" style="width:'+Math.round(h.tiers.medium/total*100)+'%"></span>'+
        '<span class="t-low" style="width:'+Math.round(h.tiers.low/total*100)+'%"></span>';

      setMetricSeverity('hm-dlq',h.dlq,1,10);
      setMetricSeverity('hm-cb',h.cb,1,1);
      setMetricSeverity('hm-rl',h.rl,1,1);
      setMetricSeverity('hm-workers',h.workers===0?1:0,1,1);
      setStatCardSeverity('card-dlq',h.dlq>=10?'crit':h.dlq>0?'warn':null);
      setStatCardSeverity('card-cb',h.cb>0?'crit':null);
      setStatCardSeverity('card-rl',h.rl>0?'warn':null);
    }

    async function loadSession() {
      try {
        const res=await fetch(API_BASE+'/session',{credentials:'same-origin'});
        if(res.ok){SESSION=await res.json();}
      }catch(_){SESSION={authenticated:false};}
      if(!SESSION.authenticated){window.location.href='/login';return false;}
      applyRoleGating();
      startIdleTimer();
      return true;
    }
    function applyRoleGating() {
      const admin=isAdmin();
      document.querySelectorAll('[data-admin]').forEach(function(el){el.style.display=admin?'':'none';});
      const banner=document.getElementById('ro-banner');
      if(banner){banner.style.display=admin?'none':'';}
    }
    function startIdleTimer() {
      const reset=function(){
        clearTimeout(idleTimer);
        idleTimer=setTimeout(logout,IDLE_TIMEOUT);
      };
      ['click','keydown','mousemove','scroll','touchstart'].forEach(function(ev){
        window.addEventListener(ev,reset,{passive:true});
      });
      reset();
    }
    async function logout() {
      try{await fetch(API_BASE+'/logout',{method:'POST',credentials:'same-origin'});}catch(_){}
      window.location.href='/login';
    }

    function toggleSidebar() {
      document.getElementById('sidebar').classList.toggle('open');
    }
    document.getElementById('sidebar').addEventListener('click',function(e){
      if(window.innerWidth<=768)this.classList.remove('open');
    });

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

    // ── Modal ──
    function openModal(title,bodyHTML,footHTML) {
      document.getElementById('modal-title').innerHTML=title;
      document.getElementById('modal-body').innerHTML=bodyHTML||'';
      document.getElementById('modal-foot').innerHTML=footHTML||'';
      document.getElementById('modal-overlay').classList.add('show');
    }
    function closeModal() {
      document.getElementById('modal-overlay').classList.remove('show');
    }
    document.getElementById('modal-overlay').addEventListener('click',function(e){
      if(e.target===this)closeModal();
    });
    document.addEventListener('keydown',function(e){if(e.key==='Escape')closeModal();});

    // ── Create Job ──
    let adminPayloadMode = 'form';
    let adminJobTypeRegistry = {};
    const ADMIN_PAYLOAD_SCHEMAS = {
      email: { title:'Email', fields:[
        {id:'to',label:'To',type:'email',required:true,placeholder:'user@example.com'},
        {id:'subject',label:'Subject',type:'text',placeholder:'Hello'},
        {id:'body',label:'Body',type:'textarea',placeholder:'Message...'}
      ]},
      image: { title:'Image', fields:[
        {id:'source_url',label:'Source URL',type:'url',required:true,placeholder:'https://example.com/img.jpg'},
        {id:'operation',label:'Operation',type:'select',options:['process','resize','compress','watermark'],default:'process'}
      ]},
      http: { title:'HTTP', fields:[
        {id:'url',label:'URL',type:'url',required:true,placeholder:'https://api.example.com/hook'},
        {id:'method',label:'Method',type:'select',options:['POST','GET','PUT','PATCH','DELETE'],default:'POST'},
        {id:'headers',label:'Headers',type:'text',placeholder:'Authorization: Bearer token'},
        {id:'body',label:'Body (JSON)',type:'textarea',placeholder:'{"key":"value"}'}
      ]}
    };
    function adminResolveHandler(name) {
      if (ADMIN_PAYLOAD_SCHEMAS[name]) return name;
      const m = adminJobTypeRegistry[name];
      return (m && m.handler) || 'http';
    }
    function adminRenderPayloadForm() {
      const type = document.getElementById('create-type').value;
      const handler = adminResolveHandler(type);
      const schema = ADMIN_PAYLOAD_SCHEMAS[handler] || ADMIN_PAYLOAD_SCHEMAS.http;
      document.getElementById('admin-payload-title').textContent = schema.title + ' Payload';
      document.getElementById('admin-payload-fields').innerHTML = schema.fields.map(f => {
        const req = f.required ? ' *' : '';
        let input = f.type === 'textarea' ? '<textarea id="apf-'+f.id+'" rows="3" placeholder="'+(f.placeholder||'')+'"></textarea>'
          : f.type === 'select' ? '<select id="apf-'+f.id+'">'+(f.options||[]).map(o=>'<option'+(o===f.default?' selected':'')+'>'+o+'</option>').join('')+'</select>'
          : '<input id="apf-'+f.id+'" type="'+(f.type||'text')+'" placeholder="'+(f.placeholder||'')+'">';
        return '<div class="form-group"><label>'+f.label+req+'</label>'+input+'</div>';
      }).join('');
    }
    function adminTogglePayloadMode() {
      adminPayloadMode = adminPayloadMode === 'form' ? 'json' : 'form';
      const showJson = adminPayloadMode === 'json';
      document.getElementById('admin-payload-fields').classList.toggle('hidden', showJson);
      document.getElementById('admin-payload-json-wrap').classList.toggle('hidden', !showJson);
      if (showJson) document.getElementById('create-payload').value = JSON.stringify(adminCollectPayload(), null, 2);
      else adminRenderPayloadForm();
    }
    function adminOnJobTypeChange() {
      if (adminPayloadMode === 'form') adminRenderPayloadForm();
    }
    function adminParseHeaders(raw) {
      raw = (raw||'').trim(); if (!raw) return undefined;
      if (raw.startsWith('{')) { try { return JSON.parse(raw); } catch { return null; } }
      const out = {}; raw.split('\n').forEach(l=>{const i=l.indexOf(':'); if(i>0) out[l.slice(0,i).trim()]=l.slice(i+1).trim();});
      return Object.keys(out).length ? out : undefined;
    }
    function adminCollectPayload() {
      const handler = adminResolveHandler(document.getElementById('create-type').value);
      const schema = ADMIN_PAYLOAD_SCHEMAS[handler] || ADMIN_PAYLOAD_SCHEMAS.http;
      const payload = {};
      schema.fields.forEach(f => {
        const el = document.getElementById('apf-'+f.id); if (!el) return;
        const val = (el.value||'').trim(); if (!val) return;
        if (f.id === 'headers') { const h = adminParseHeaders(val); if (h) payload.headers = h; return; }
        if (f.id === 'body' && handler === 'http') { try { payload.body = JSON.parse(val); } catch { payload.body = val; } return; }
        payload[f.id] = f.type === 'number' ? Number(val) : val;
      });
      return payload;
    }
    function fillExample() {
      document.getElementById('create-type').value='email';
      adminRenderPayloadForm();
      const toEl = document.getElementById('apf-to');
      if (toEl) toEl.value = 'user@example.com';
      const subEl = document.getElementById('apf-subject');
      if (subEl) subEl.value = 'hello';
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
      if (adminPayloadMode === 'json') {
        try { body.payload = JSON.parse(document.getElementById('create-payload').value.trim() || '{}'); }
        catch(_) { toast('Invalid JSON payload','error'); return; }
      } else {
        body.payload = adminCollectPayload();
      }
      const out=await api('/jobs',{method:'POST',body:JSON.stringify(body)});
      document.getElementById('create-output').innerText=JSON.stringify(out,null,2);
      if(out&&out._ok&&out.id){toast('Job created: '+out.id);}else{toast((out&&out._error)||'Create failed','error');}
    }

    // ── Search Jobs ──
    let searchState={page:1,limit:20};
    const STATUS_PILL={pending:'warn',running:'',completed:'good',failed:'bad',cancelled:'bad',paused:'warn'};
    function statusPill(s){
      const cls=STATUS_PILL[s]||'';
      return '<span class="pill '+cls+'">'+esc(s)+'</span>';
    }
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
      const body=document.getElementById('search-body');
      body.innerHTML='<tr><td colspan="9">'+stateLoading('Searching jobs…')+'</td></tr>';
      const out=await api('/jobs?'+p.toString());
      if(!out||!out._ok){body.innerHTML='<tr><td colspan="9">'+stateError((out&&out._error)||'Search failed')+'</td></tr>';return;}
      const jobs=(out.jobs||out.items||(Array.isArray(out)?out:[]));
      if(!jobs.length){body.innerHTML='<tr><td colspan="9">'+stateEmpty('fa-inbox','No jobs match these filters.')+'</td></tr>';}
      else{
        body.innerHTML=jobs.map(j=>'<tr>'+
          '<td>'+esc(j.id)+'</td>'+
          '<td>'+esc(j.type)+'</td>'+
          '<td>'+statusPill(j.status)+'</td>'+
          '<td>'+esc(j.priority)+'</td>'+
          '<td>'+esc(j.tenant_id||'')+'</td>'+
          '<td>'+(j.retries||0)+'/'+(j.max_retries!=null?j.max_retries:'-')+'</td>'+
          '<td>'+(j.dependencies&&j.dependencies.length?'<span class="pill warn">'+j.dependencies.length+' dep</span>':'-')+'</td>'+
          '<td style="font-size:11px;color:var(--muted);">'+esc((j.created_at||'').slice(0,19).replace('T',' '))+'</td>'+
          '<td><button class="btn-secondary btn-sm" onclick="openJobDetail(\''+esc(j.id)+'\')">Details</button></td>'+
          '</tr>').join('');
      }
      document.getElementById('page-info').innerText='Page '+searchState.page;
    }
    function prevPage(){if(searchState.page>1){searchState.page--;document.getElementById('search-page').value=searchState.page;searchJobs();}}
    function nextPage(){searchState.page++;document.getElementById('search-page').value=searchState.page;searchJobs();}

    async function openJobDetail(id) {
      const out=await api(API_BASE+'/jobs/'+encodeURIComponent(id));
      if(!out||out._ok===false){toast('Job not found','error');return;}
      let depsHTML='<tr><td colspan="2" style="text-align:center;color:var(--muted);">No dependencies.</td></tr>';
      let blocked=false;
      if(out&&out.dependencies&&out.dependencies.length){
        const dag=await api(API_BASE+'/jobs/'+encodeURIComponent(id)+'/deps');
        const upstream=(dag&&dag.depends_on)||[];
        blocked=upstream.some(d=>d.status!=='completed');
        depsHTML=upstream.map(d=>'<tr>'+
          '<td>'+esc(d.id)+'</td>'+
          '<td>'+statusPill(d.status)+'</td>'+
          '</tr>').join('');
        if(!upstream.length){
          const ids=(out.dependencies||[]).map(esc).join(', ');
          depsHTML='<tr><td colspan="2" style="color:var(--muted);">'+ids+' (not resolvable)</td></tr>';
        }
      }
      const badge=blocked
        ?'<span class="pill bad" title="One or more dependencies have not completed"><i class="fas fa-lock"></i> Blocked by dependencies</span>'
        :(out&&out.dependencies&&out.dependencies.length?'<span class="pill good"><i class="fas fa-unlock"></i> Dependencies satisfied</span>':'');
      const body='<table style="margin-bottom:12px;"><tbody>'+
        '<tr><th style="width:150px;">ID</th><td>'+esc(out&&out.id)+'</td></tr>'+
        '<tr><th>Type</th><td>'+esc(out&&out.type)+'</td></tr>'+
        '<tr><th>Status</th><td>'+statusPill(out&&out.status)+' '+badge+'</td></tr>'+
        '<tr><th>Priority</th><td>'+esc(out&&out.priority)+'</td></tr>'+
        '<tr><th>Tenant</th><td>'+esc(out&&out.tenant_id||'')+'</td></tr>'+
        '<tr><th>Retries</th><td>'+(out&&out.retries||0)+' / '+(out&&out.max_retries!=null?out.max_retries:'-')+'</td></tr>'+
        '<tr><th>Created</th><td>'+esc(out&&out.created_at)+'</td></tr>'+
        '<tr><th>Updated</th><td>'+esc(out&&out.updated_at)+'</td></tr>'+
        '<tr><th>Shard Key</th><td>'+esc(out&&out.shard_key||'')+'</td></tr>'+
        '</tbody></table>'+
        '<div class="section-title" style="font-size:11px;">Payload</div>'+
        '<pre>'+esc(JSON.stringify((out&&out.payload)||{},null,2))+'</pre>'+
        '<div class="section-title" style="font-size:11px;margin-top:14px;">Dependencies</div>'+
        '<table><thead><tr><th>Job</th><th>Status</th></tr></thead><tbody>'+depsHTML+'</tbody></table>';
      openModal('Job '+esc(id),body,'<button class="btn-secondary btn-sm" onclick="closeModal()">Close</button>');
    }

    // ── Stats ──
    function renderPriorityBreakdown(pb) {
      if(!pb||!pb.by_priority){
        return '<span style="color:var(--muted);">No priority breakdown available.</span>';
      }
      const weights=pb.dequeue_weights||{};
      const parts=pb.partitions_per_priority||3;
      let html='<p style="font-size:12px;color:var(--muted2);margin-bottom:10px;">'+parts+' hash partitions per tier · weights high '+((weights.high)||70)+'% / medium '+((weights.medium)||20)+'% / low '+((weights.low)||10)+'%</p>';
      html+='<table><thead><tr><th>Tier</th><th>Total</th>';
      for(let i=1;i<=parts;i++){html+='<th>P'+i+'</th>';}
      html+='</tr></thead><tbody>';
      ['high','medium','low'].forEach(function(tier){
        const td=pb.by_priority[tier]||{total:0,partitions:{}};
        const cls=tier==='high'?'bad':tier==='low'?'':'warn';
        html+='<tr><td><span class="pill '+cls+'">'+tier+'</span></td><td><strong>'+(td.total||0)+'</strong></td>';
        for(let i=1;i<=parts;i++){
          const n=(td.partitions&&td.partitions[String(i)])||0;
          html+='<td style="color:var(--muted);">'+n+'</td>';
        }
        html+='</tr>';
      });
      html+='</tbody></table>';
      return html;
    }
    async function loadStats() {
      const pv=document.getElementById('stats-priority-view');
      pv.innerHTML=stateLoading('Loading priority breakdown…');
      const out=await api(API_BASE+'/stats');
      if(!out||!out._ok){pv.innerHTML=stateError((out&&out._error)||'Failed to load stats');return;}
      document.getElementById('stats-output').innerText=JSON.stringify(out,null,2);
      const pb=out.priority_breakdown;
      pv.innerHTML=renderPriorityBreakdown(pb);
      healthCache.pending=out.total_pending||0;
      healthCache.workers=out.worker_count||0;
      if(pb&&pb.by_priority){
        healthCache.tiers={
          high:(pb.by_priority.high&&pb.by_priority.high.total)||0,
          medium:(pb.by_priority.medium&&pb.by_priority.medium.total)||0,
          low:(pb.by_priority.low&&pb.by_priority.low.total)||0
        };
      }
      document.getElementById('stat-pending').innerText=String(healthCache.pending);
      updateHealthBanner();
    }

    // ── Workers ──
    async function loadWorkers() {
      const el=document.getElementById('workers-output');
      el.innerHTML=stateLoading('Loading workers…');
      const out=await api('/workers');
      if(!out||out._ok===false){el.innerHTML=stateError((out&&out._error)||'Failed to load workers');return;}
      el.innerText=JSON.stringify(out,null,2);
    }
    async function loadMetrics() {
      const el=document.getElementById('metrics-output');
      el.innerHTML=stateLoading('Loading metrics…');
      const out=await api('/metrics');
      if(!out||out._ok===false){el.innerHTML=stateError((out&&out._error)||'Failed to load metrics');return;}
      el.innerText=typeof out==='string'?out.substring(0,2000):JSON.stringify(out,null,2);
    }
    async function checkHealth() {
      const el=document.getElementById('health-status');
      el.innerHTML=stateLoading('Running health checks…');
      const healthz=await api('/healthz');
      const readyz=await api('/readyz');
      const hzOk=healthz&&healthz._ok!==false;
      const rdOk=readyz&&readyz._ok!==false;
      healthCache.apiOk=hzOk&&rdOk;
      el.innerHTML=
        '<p style="margin-bottom:8px;"><span class="badge '+(hzOk?'green':'red')+'"></span> /healthz '+(hzOk?'<span class="pill good">OK</span>':'<span class="pill bad">FAIL</span>')+'</p>'+
        '<p><span class="badge '+(rdOk?'green':'red')+'"></span> /readyz '+(rdOk?'<span class="pill good">OK</span>':'<span class="pill bad">FAIL</span>')+'</p>';
      updateHealthBanner();
    }

    // ── DAG ──
    async function loadDAG() {
      const id=document.getElementById('dag-job-id').value.trim();
      if(!id){toast('Enter a job ID');return;}
      const out=await api(API_BASE+'/jobs/'+encodeURIComponent(id)+'/deps');
      if(!out){document.getElementById('dag-upstream').innerText='No response';return;}
      const up=out.depends_on||[], down=out.dependents||[];
      const fmt=function(list){return list.map(d=>{
        const s=d.status==='completed'?'good':(d.status==='failed'?'bad':'warn');
        return '['+d.status+'] '+d.id+(d.type?' ('+d.type+')':'');
      }).join('\n')||'none';};
      document.getElementById('dag-upstream').innerText=fmt(up);
      document.getElementById('dag-downstream').innerText=fmt(down);
      const blocked=up.some(d=>d.status!=='completed');
      if(blocked){toast('Job is blocked by '+up.filter(d=>d.status!=='completed').length+' uncompleted dependency/dependencies');}
    }

    // ── Rate Limits ──
    async function loadRateLimits() {
      const container=document.getElementById('rl-state');
      container.innerHTML=stateLoading('Loading rate limits…');
      const out=await api(API_BASE+'/rate-limits');
      if(!out||!out._ok){container.innerHTML=stateError((out&&out._error)||'Failed to load rate limits');return;}
      const tenants=out.tenants||[];
      const limited=tenants.filter(t=>t.limited).length;
      healthCache.rl=limited;
      document.getElementById('stat-rl').innerText=String(limited);
      if(out.unlimited){
        container.innerHTML=stateEmpty('fa-infinity','Rate limiting is disabled (unlimited).');
        updateHealthBanner();return;
      }
      if(!tenants.length){
        container.innerHTML=stateEmpty('fa-users','No tenants have submitted requests yet.');
        updateHealthBanner();return;
      }
      container.innerHTML='<table><thead><tr><th>Tenant</th><th>Current</th><th>Limit</th><th>Window</th><th>Status</th></tr></thead><tbody>'+
        tenants.map(t=>'<tr>'+
          '<td>'+esc(t.tenant)+'</td><td>'+esc(t.current)+'</td><td>'+esc(t.limit)+'</td>'+
          '<td>'+esc(t.window_seconds)+'s</td>'+
          '<td>'+(t.limited?'<span class="pill bad">limited</span>':'<span class="pill good">ok</span>')+'</td></tr>').join('')+
        '</tbody></table>';
      updateHealthBanner();
    }

    // ── Circuit Breaker ──
    async function loadCircuitBreakers() {
      const dashContainer=document.getElementById('cb-state');
      const pageBody=document.getElementById('cb-body');
      dashContainer.innerHTML=stateLoading('Loading circuit breakers…');
      const out=await api(API_BASE+'/circuit-breakers');
      if(!out||out._ok===false){
        const err=stateError((out&&out._error)||'Failed to load circuit breakers');
        dashContainer.innerHTML=err;
        pageBody.innerHTML='<tr><td colspan="3">'+err+'</td></tr>';
        return;
      }
      const map={};
      if(out&&typeof out==='object'){
        Object.entries(out).forEach(function(e){if(e[0][0]!=='_')map[e[0]]=e[1];});
      }
      const open=Object.entries(map).filter(([,v])=>/open/.test(v)).length;
      healthCache.cb=open;
      document.getElementById('stat-cb').innerText=String(open);
      const rows=Object.entries(map).map(([k,v])=>{
        const cls=v.startsWith('open')?'bad':v==='closed'?'good':'warn';
        const action=isAdmin()?'<td><button class="btn-secondary btn-sm" data-reset="'+esc(k)+'">Reset</button></td>':'<td></td>';
        return '<tr><td>'+esc(k)+'</td><td><span class="pill '+cls+'">'+esc(v)+'</span></td>'+action+'</tr>';
      }).join('');
      pageBody.innerHTML=rows||'<tr><td colspan="3">'+stateEmpty('fa-shield-alt','No circuit breakers registered.')+'</td></tr>';
      const chips=Object.entries(map).map(([k,v])=>{
        const cls=v.startsWith('open')?'open':v==='closed'?'closed':'half';
        return '<div class="cb-chip '+cls+'"><span class="dot"></span>'+esc(k)+' <span style="color:var(--muted);font-size:11px;">'+esc(v)+'</span>'+
          (isAdmin()?'<span class="actions"><button class="btn-secondary btn-sm" data-reset="'+esc(k)+'">Reset</button></span>':'')+
          '</div>';
      }).join('');
      dashContainer.innerHTML=chips?'<div class="cb-chips">'+chips+'</div>':stateEmpty('fa-shield-alt','No circuit breakers active.');
      updateHealthBanner();
    }
    document.getElementById('cb-body').addEventListener('click',function(e){
      const b=e.target.closest('[data-reset]');
      if(b)resetBreaker(b.getAttribute('data-reset'));
    });
    document.getElementById('cb-state').addEventListener('click',function(e){
      const b=e.target.closest('[data-reset]');
      if(b)resetBreaker(b.getAttribute('data-reset'));
    });
    async function resetBreaker(type) {
      if(!confirmAction('Reset the circuit breaker for plugin "'+type+'"?'))return;
      await api(API_BASE+'/circuit-breakers/reset/'+encodeURIComponent(type),{method:'POST'});
      toast('Reset breaker for '+type); loadCircuitBreakers();
    }

    // ── DLQ ──
    let dlqState={page:1,limit:20,cache:[],total:0};
    async function loadDLQ() {
      const queue=document.getElementById('dlq-queue').value.trim();
      const tenant=document.getElementById('dlq-tenant').value.trim();
      dlqState.page=parseInt(document.getElementById('dlq-page').value)||1;
      const body=document.getElementById('dlq-body');
      body.innerHTML='<tr><td colspan="7">'+stateLoading('Loading dead letter queue…')+'</td></tr>';
      const p=new URLSearchParams({page:dlqState.page,limit:dlqState.limit});
      if(queue)p.set('queue',queue);
      const out=await api(API_BASE+'/dlq?'+p.toString());
      if(!out||out._ok===false){body.innerHTML='<tr><td colspan="7">'+stateError((out&&out._error)||'Failed to load DLQ')+'</td></tr>';return;}
      const list=Array.isArray(out)?out:(out.jobs||[]);
      dlqState.cache=list;
      dlqState.total=(out.total!=null)?out.total:list.length;
      healthCache.dlq=dlqState.total;
      document.getElementById('dlq-page-info').innerText='Page '+dlqState.page+' · '+dlqState.total+' total';
      const filtered=list.filter(j=>!tenant||(j.tenant_id||'').includes(tenant));
      if(!filtered.length){body.innerHTML='<tr><td colspan="7">'+stateEmpty('fa-check-circle','No failed jobs on this page.')+'</td></tr>';}
      else{
        body.innerHTML=filtered.map(j=>{
          const err=((j.error_history&&j.error_history.length)?j.error_history[j.error_history.length-1].error:j.error)||'';
          return '<tr>'+
            '<td>'+esc(j.id)+'</td>'+
            '<td>'+esc(j.type)+'</td>'+
            '<td>'+esc(j.tenant_id||'')+'</td>'+
            '<td style="max-width:260px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--muted);" title="'+esc(err)+'">'+esc(err)+'</td>'+
            '<td>'+(j.retries||0)+'</td>'+
            '<td style="font-size:11px;color:var(--muted);">'+esc((j.updated_at||'').slice(0,19).replace('T',' '))+'</td>'+
            '<td><button class="btn-secondary btn-sm" onclick="openDLQDetail(\''+esc(j.id)+'\')">Details</button>'+
            (isAdmin()?' <button class="btn-primary btn-sm" onclick="replayDLQId(\''+esc(j.id)+'\')" aria-label="Replay job"><i class="fas fa-redo"></i></button>':'')+
            '</td></tr>';
        }).join('');
      }
      document.getElementById('stat-dlq').innerText=String(dlqState.total);
      updateHealthBanner();
      return filtered;
    }
    function dlqPrevPage(){if(dlqState.page>1){dlqState.page--;document.getElementById('dlq-page').value=dlqState.page;loadDLQ();}}
    function dlqNextPage(){dlqState.page++;document.getElementById('dlq-page').value=dlqState.page;loadDLQ();}

    async function openDLQDetail(id) {
      const j=await api(API_BASE+'/dlq/'+encodeURIComponent(id));
      if(!j||j._ok===false){toast('Job not found','error');return;}
      const lastErr=(j.error_history&&j.error_history.length)?j.error_history[j.error_history.length-1].error:(j.error||'Unknown failure');
      const history=(j.error_history||[]).map(h=>
        '<tr><td>'+(h.attempt!=null?'#'+h.attempt:'')+'</td><td>'+esc(h.timestamp||'')+'</td><td>'+esc(h.error)+'</td></tr>'
      ).join('')||'<tr><td colspan="3" style="text-align:center;color:var(--muted);">No recorded attempts.</td></tr>';
      const body=
        '<div style="background:rgba(239,68,68,.08);border:1px solid rgba(239,68,68,.3);border-radius:10px;padding:12px 14px;margin-bottom:14px;">'+
        '<div style="font-size:11px;color:var(--muted2);text-transform:uppercase;letter-spacing:.06em;margin-bottom:4px;">Failure Reason</div>'+
        '<div style="font-size:13px;color:#f87171;word-break:break-word;">'+esc(lastErr)+'</div></div>'+
        '<table style="margin-bottom:12px;"><tbody>'+
        '<tr><th style="width:150px;">ID</th><td>'+esc(j.id)+'</td></tr>'+
        '<tr><th>Type</th><td>'+esc(j.type)+'</td></tr>'+
        '<tr><th>Tenant</th><td>'+esc(j.tenant_id||'')+'</td></tr>'+
        '<tr><th>Retries</th><td>'+esc(j.retries)+' / '+esc(j.max_retries)+'</td></tr>'+
        '<tr><th>Updated</th><td>'+esc(j.updated_at)+'</td></tr>'+
        '</tbody></table>'+
        '<div class="section-title" style="font-size:11px;">Retry History</div>'+
        '<table><thead><tr><th>Attempt</th><th>Timestamp</th><th>Error</th></tr></thead><tbody>'+history+'</tbody></table>'+
        '<div class="section-title" style="font-size:11px;margin-top:14px;">Original Payload</div>'+
        '<pre>'+esc(JSON.stringify((j.payload)||{},null,2))+'</pre>';
      const foot='<button class="btn-secondary btn-sm" onclick="closeModal()">Close</button>'+
        (isAdmin()?'<button class="btn-primary btn-sm" onclick="replayDLQId(\''+esc(j.id)+'\')"><i class="fas fa-redo"></i> Replay</button>'+
        '<button class="btn-danger btn-sm" onclick="deleteDLQId(\''+esc(j.id)+'\')"><i class="fas fa-times"></i> Purge</button>':'');
      openModal('Failed Job '+esc(j.id),body,foot);
    }
    async function replayDLQId(id) {
      if(!confirmAction('Replay job '+id+' from the dead letter queue?'))return;
      await api(API_BASE+'/dlq/'+encodeURIComponent(id)+'/replay',{method:'POST'});
      closeModal(); loadDLQ(); toast('Replayed job: '+id);
    }
    async function deleteDLQId(id) {
      if(!confirmAction('Permanently purge job '+id+'? This cannot be undone.'))return;
      await api(API_BASE+'/dlq/'+encodeURIComponent(id),{method:'DELETE'});
      closeModal(); loadDLQ(); toast('Purged job: '+id);
    }
    function exportDLQ() {
      const blob=new Blob([JSON.stringify(dlqState.cache,null,2)],{type:'application/json'});
      const url=URL.createObjectURL(blob);
      const a=document.createElement('a');a.href=url;a.download='dlq-export.json';a.click();
      URL.revokeObjectURL(url);
    }
    async function replayDLQ() {
      const id=document.getElementById('dlq-replay-id').value.trim();
      if(!id)return;
      if(!confirmAction('Replay job '+id+' from the dead letter queue?'))return;
      await api(API_BASE+'/dlq/'+encodeURIComponent(id)+'/replay',{method:'POST'});
      loadDLQ(); toast('Replayed job: '+id);
    }
    async function deleteDLQ() {
      const id=document.getElementById('dlq-purge-id').value.trim();
      if(!id)return;
      if(!confirmAction('Permanently purge job '+id+'? This cannot be undone.'))return;
      await api(API_BASE+'/dlq/'+encodeURIComponent(id),{method:'DELETE'});
      loadDLQ(); toast('Purged job: '+id);
    }
    async function bulkPurgeDLQ() {
      const older=document.getElementById('dlq-older').value.trim();
      if(!older)return;
      if(!confirmAction('Permanently purge ALL failed jobs older than '+older+'? This cannot be undone.'))return;
      const queue=document.getElementById('dlq-queue').value.trim();
      await api(API_BASE+'/dlq?older_than='+encodeURIComponent(older)+(queue?'&queue='+encodeURIComponent(queue):''),{method:'DELETE'});
      loadDLQ(); toast('Bulk purge completed');
    }

    // ── Clients ──
    async function loadClients() {
      const body=document.getElementById('clients-body');
      body.innerHTML='<tr><td colspan="3">'+stateLoading('Loading clients…')+'</td></tr>';
      const out=await api(API_BASE+'/clients');
      const list=(out&&out.clients)||[];
      if(!list.length){body.innerHTML='<tr><td colspan="3">'+stateEmpty('fa-users','No clients registered yet.')+'</td></tr>';return;}
      body.innerHTML=list.map(c=>
        '<tr><td style="font-family:monospace;">'+esc(c.tenant_id)+'</td>'+
        '<td style="color:var(--muted);font-size:12px;">'+esc((c.created_at||'').slice(0,19).replace('T',' '))+'</td>'+
        '<td style="white-space:nowrap;">'+
        '<button class="btn-secondary btn-sm" onclick="rotateClientKey(\''+esc(c.tenant_id)+'\')" style="margin-right:4px"><i class="fas fa-rotate"></i> Rotate</button>'+
        '<button class="btn-danger btn-sm" onclick="revokeClient(\''+esc(c.tenant_id)+'\')"><i class="fas fa-ban"></i> Revoke</button>'+
        '</td></tr>'
      ).join('');
    }
    async function revokeClient(tenantId) {
      if(!confirmAction('Revoke all API keys for tenant '+tenantId+'?'))return;
      await api(API_BASE+'/clients/'+encodeURIComponent(tenantId),{method:'DELETE'});
      loadClients(); toast('Client revoked: '+tenantId);
    }
    async function rotateClientKey(tenantId) {
      if(!confirmAction('Rotate API key for tenant '+tenantId+'? The old key will stop working.'))return;
      const out=await api(API_BASE+'/clients/'+encodeURIComponent(tenantId)+'/rotate',{method:'POST'});
      if(out&&out.api_key){
        openModal('New API Key for '+esc(tenantId),'<p style="font-size:12px;color:var(--muted);margin-bottom:10px">Copy this key now — it will not be shown again.</p><pre>'+esc(out.api_key)+'</pre>','<button class="btn-secondary btn-sm" onclick="closeModal()">Close</button>');
      }
      loadClients();
    }

    // ── Webhooks ──
    async function loadWebhooks() {
      const body=document.getElementById('wh-body');
      body.innerHTML='<tr><td colspan="5">'+stateLoading('Loading webhooks…')+'</td></tr>';
      const out=await api(API_BASE+'/webhooks');
      if(!out||out._ok===false){body.innerHTML='<tr><td colspan="5">'+stateError((out&&out._error)||'Failed to load webhooks')+'</td></tr>';return;}
      const list=Array.isArray(out)?out:[];
      if(!list.length){body.innerHTML='<tr><td colspan="5">'+stateEmpty('fa-globe','No webhooks registered.')+'</td></tr>';return;}
      body.innerHTML=list.map(w=>
        '<tr>'+
        '<td style="font-family:monospace;font-size:12px;">'+esc(w.id)+'</td>'+
        '<td style="color:var(--muted);">'+esc(w.url)+'</td>'+
        '<td>'+(w.events||[]).map(e=>'<span class="pill">'+esc(e)+'</span>').join(' ')+'</td>'+
        '<td style="font-size:11px;color:var(--muted);">'+esc((w.created_at||'').slice(0,19).replace('T',' '))+'</td>'+
        '<td><button class="btn-secondary btn-sm" onclick="openWhHistory(\''+esc(w.id)+'\',\''+esc(w.url)+'\')"><i class="fas fa-history"></i> History</button></td>'+
        '</tr>').join('');
    }
    async function registerWebhook() {
      const body={
        url:document.getElementById('wh-url').value.trim(),
        secret:document.getElementById('wh-secret').value.trim(),
        events:(document.getElementById('wh-events').value.trim()||'completed,failed').split(',').map(s=>s.trim())
      };
      const tenant=document.getElementById('wh-tenant').value.trim();
      if(tenant)body.tenant_id=tenant;
      if(!body.url){toast('URL is required','warn');return;}
      const out=await api(API_BASE+'/webhooks',{method:'POST',body:JSON.stringify(body)});
      if(!out||out._ok===false){toast((out&&out._error)||'Register failed','error');return;}
      loadWebhooks(); toast('Webhook registered');
    }
    async function openWhHistory(id,url) {
      const out=await api(API_BASE+'/webhooks/'+encodeURIComponent(id)+'/deliveries?limit=20');
      const list=Array.isArray(out)?out:[];
      const rows=list.map(d=>{
        const ok=d.success;
        const backoff=d.backoff_ms>0?esc(d.backoff_ms)+'ms':'—';
        return '<tr>'+
          '<td style="font-size:11px;color:var(--muted);">'+esc((d.timestamp||'').slice(0,19).replace('T',' '))+'</td>'+
          '<td style="font-family:monospace;font-size:12px;">'+esc(d.job_id||'')+'</td>'+
          '<td>#'+(d.attempt||1)+'</td>'+
          '<td>'+backoff+'</td>'+
          '<td>'+(d.status_code?esc(d.status_code):'<span style="color:var(--bad);">transport</span>')+'</td>'+
          '<td>'+(ok?'<span class="pill good">delivered</span>':'<span class="pill bad">failed</span>')+'</td>'+
          '<td style="max-width:220px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;color:var(--muted);" title="'+esc(d.error)+'">'+esc(d.error||'')+'</td>'+
          '</tr>';
      }).join('')||'<tr><td colspan="7" style="text-align:center;color:var(--muted);">No deliveries recorded yet.</td></tr>';
      const body='<p style="font-size:12px;color:var(--muted);margin-bottom:12px;word-break:break-all;">'+esc(url)+'</p>'+
        '<table><thead><tr><th>Time</th><th>Job</th><th>Attempt</th><th>Backoff</th><th>Status</th><th>Result</th><th>Error</th></tr></thead><tbody>'+rows+'</tbody></table>';
      openModal('Delivery History',body,'<button class="btn-secondary btn-sm" onclick="closeModal()">Close</button>');
    }

    // ── Job Types ──
    async function loadJobTypes() {
      const body=document.getElementById('jt-body');
      body.innerHTML='<tr><td colspan="5">'+stateLoading('Loading job types…')+'</td></tr>';
      const out=await api(API_BASE+'/job-types');
      const list=(out&&out.job_types)||[];
      adminJobTypeRegistry = {};
      list.forEach(t => { adminJobTypeRegistry[t.name] = t; });
      const createSel = document.getElementById('create-type');
      if (createSel && list.length) {
        createSel.innerHTML = list.map(t =>
          '<option value="'+esc(t.name)+'">'+esc(t.name)+(t.built_in?'':' (custom)')+'</option>'
        ).join('');
      }
      adminRenderPayloadForm();
      if(!list.length){body.innerHTML='<tr><td colspan="5">'+stateEmpty('fa-tags','No job types found.')+'</td></tr>';return;}
      body.innerHTML=list.map(jt=>
        '<tr>'+
        '<td><span class="pill blue">'+esc(jt.name)+'</span></td>'+
        '<td style="font-family:monospace;font-size:12px;">'+esc(jt.handler||jt.name)+'</td>'+
        '<td style="color:var(--muted);font-size:12px;">'+esc(jt.description||'')+'</td>'+
        '<td>'+(jt.built_in?'<span class="pill good">yes</span>':'<span class="pill">custom</span>')+'</td>'+
        '<td>'+(jt.built_in?'':'<button class="btn-danger btn-sm" onclick="deleteJobType(\''+esc(jt.name)+'\')"><i class="fas fa-trash"></i></button>')+'</td>'+
        '</tr>').join('');
    }
    async function createJobType() {
      const body={
        name:document.getElementById('jt-name').value.trim(),
        description:document.getElementById('jt-desc').value.trim(),
        handler:document.getElementById('jt-handler').value,
        payload_hint:document.getElementById('jt-hint').value.trim()
      };
      if(!body.name){toast('Name is required','warn');return;}
      const out=await api(API_BASE+'/job-types',{method:'POST',body:JSON.stringify(body)});
      if(!out||out._ok===false){toast((out&&out._error)||'Create failed','error');return;}
      document.getElementById('jt-name').value='';
      document.getElementById('jt-desc').value='';
      document.getElementById('jt-hint').value='';
      loadJobTypes(); toast('Job type registered');
    }
    async function deleteJobType(name) {
      if(!confirmAction('Delete job type '+name+'?'))return;
      await api(API_BASE+'/job-types/'+encodeURIComponent(name),{method:'DELETE'});
      loadJobTypes(); toast('Job type deleted');
    }

    // ── Live Activity (SSE) ──
    let eventSource=null;
    let sseDisconnectTimer=null;
    function connectEvents() {
      if(eventSource){eventSource.close();eventSource=null;}
      setSSEState('connecting');
      try{
        eventSource=new EventSource(API_BASE+'/events');
      }catch(e){setSSEState('disconnected');return;}
      eventSource.onopen=function(){
        clearTimeout(sseDisconnectTimer);
        setSSEState('connected');
      };
      eventSource.onerror=function(){
        if(sseState==='connected'||sseState==='connecting'){
          setSSEState('reconnecting');
        }
        clearTimeout(sseDisconnectTimer);
        sseDisconnectTimer=setTimeout(function(){
          if(eventSource&&eventSource.readyState===EventSource.CLOSED){
            setSSEState('disconnected');
          }
        },8000);
      };
      eventSource.onmessage=function(e){
        clearTimeout(sseDisconnectTimer);
        if(sseState!=='connected')setSSEState('connected');
        let data={};
        try{data=JSON.parse(e.data);}catch(_){}
        pushFeed(data);
        if(data.kind==='job'){loadStats().catch(function(){});loadCircuitBreakers().catch(function(){});}
        else if(data.kind==='rate_limit'){loadRateLimits().catch(function(){});}
        else if(data.kind==='circuit_breaker'){loadCircuitBreakers().catch(function(){});}
        else if(data.kind==='dlq'){loadDLQ().catch(function(){});}
      };
      eventSource.addEventListener('session_expired',function(){
        toast('Session expired — redirecting to login','warn');
        setTimeout(function(){window.location.href='/login';},1500);
      });
    }
    function pushFeed(data) {
      const feed=document.getElementById('feed-body');
      const time=new Date().toTimeString().slice(0,8);
      const kind=data.kind||'job';
      let msg;
      if(kind==='job'){msg=(data.status||'')+' '+(data.type||'')+' job '+esc(data.job_id||'')+(data.tenant_id?' <'+esc(data.tenant_id)+'>':'');}
      else if(kind==='rate_limit'){msg='Rate limit rejected for tenant '+esc(data.tenant_id||'');}
      else if(kind==='circuit_breaker'){msg='Circuit breaker '+(data.status||'changed')+' for plugin '+esc(data.type||'');}
      else if(kind==='dlq'){msg=(data.status||'')+' failed job '+esc(data.job_id||'');}
      else{msg=esc(data.job_id||data.type||'');}
      if(data.error){msg+=' — '+esc(data.error);}
      const item=document.createElement('div');
      item.className='feed-item';
      item.innerHTML='<span class="time">'+time+'</span><span class="kind '+esc(kind)+'">'+esc(kind)+'</span><span class="msg">'+msg+'</span>';
      feed.prepend(item);
      while(feed.children.length>50){feed.removeChild(feed.lastChild);}
    }

    // ── Init ──
    async function loadEverything() {
      setSSEState(sseState==='connected'?sseState:'connecting');
      await Promise.all([
        loadDLQ().catch(function(){}),
        loadWorkers().catch(function(){}),
        loadMetrics().catch(function(){}),
        checkHealth().catch(function(){}),
        loadStats().catch(function(){}),
        loadCircuitBreakers().catch(function(){}),
        loadRateLimits().catch(function(){}),
        loadWebhooks().catch(function(){}),
        loadJobTypes().catch(function(){})
      ]);
      updateHealthBanner();
    }
    loadSession().then(function(ok){
      if(ok){
        adminRenderPayloadForm();
        const page=localStorage.getItem('task_queue_page')||'dashboard';
        showPage(page);
        loadEverything();
        connectEvents();
      }
    });
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
    <input id="login-user" type="text" placeholder="Username" autocomplete="username" autofocus />
    <input id="login-pass" type="password" placeholder="Password" autocomplete="current-password" />
    <button id="login-btn" onclick="login()">Sign In</button>
    <div id="login-error" class="error">Invalid credentials</div>
  </div>
  <script>
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
          headers:{'Content-Type':'application/json'},
          credentials:'same-origin'
        });
        if(res.ok){
          window.location.href='/';
        }else{
          const data=await res.json().catch(function(){return {};});
          err.style.display='block';
          err.textContent=(data&&data.error)?data.error:'Invalid credentials';
        }
      }catch(e){
        err.style.display='block';err.textContent='Connection error';
      }
      btn.disabled=false;btn.innerHTML='Sign In';
    }
    document.getElementById('login-pass').addEventListener('keydown',function(e){
      if(e.key==='Enter')login();
    });
    document.getElementById('login-btn').addEventListener('click',function(e){e.preventDefault();login();});
  </script>
</body>
</html>`
