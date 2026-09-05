package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"task-queue-system/internal/api/middleware"
)

// ServeAdminDLQ renders the single-page HTML management console for the Dead
// Letter Queue. The page contains no secrets; it authenticates against the
// session API on load.
func (h *JobHandler) ServeAdminDLQ(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, adminHTML)
}

// ListClients returns a JSON list of all registered clients (tenants).
func (h *JobHandler) ListClients(w http.ResponseWriter, r *http.Request) {
	clients, err := h.service.Store().ListClients(r.Context())
	if err != nil {
		h.logger.Error("failed to list clients", "error", err)
		middleware.SendJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch clients")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"clients": clients,
	})
}

const adminHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>TaskQueue | DLQ Console</title>
  <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@400;500;600;700;800&family=Inter:wght@400;500;600&display=swap" rel="stylesheet">
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css">
  <style>
    * { margin:0; padding:0; box-sizing:border-box; }
    :root {
      --bg:#070a14; --bg2:#0f1629; --surface:rgba(26,34,54,0.65); --surface2:rgba(31,42,64,0.8);
      --border:rgba(255,255,255,0.08); --border-hover:rgba(255,255,255,0.15); --text:#f8fafc; --muted:#94a3b8; --muted2:#64748b;
      --accent:#6366f1; --accent2:#818cf8; --accent3:#c084fc;
      --good:#34d399; --bad:#f87171; --warn:#fbbf24;
      --sidebar-w:260px; --header-h:70px;
    }
    body {
      font-family:'Outfit','Inter',-apple-system,sans-serif; background:var(--bg); color:var(--text);
      min-height:100vh; overflow-x:hidden;
    }
    ::-webkit-scrollbar { width:6px; }
    ::-webkit-scrollbar-track { background:transparent; }
    ::-webkit-scrollbar-thumb { background:var(--border); border-radius:3px; }

    @keyframes fadeIn { from{opacity:0;transform:scale(.96)} to{opacity:1;transform:scale(1)} }

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
      width:44px; height:44px; background:linear-gradient(135deg,#ef4444,#f87171);
      border-radius:12px; display:flex; align-items:center; justify-content:center;
      box-shadow:0 8px 24px rgba(239,68,68,.3), inset 0 2px 4px rgba(255,255,255,.2);
    }
    .sidebar-brand .brand-icon i { color:#fff; font-size:18px; }
    .sidebar-brand h2 { font-size:16px; font-weight:700; }
    .sidebar-brand span { font-size:11px; color:#f87171; font-weight:500; }
    .sidebar-nav { flex:1; overflow-y:auto; padding:12px 10px; }
    .nav-item {
      display:flex; align-items:center; gap:12px; padding:10px 14px;
      border-radius:10px; cursor:pointer; transition:all .15s;
      font-size:13px; font-weight:500; color:var(--muted);
      margin-bottom:2px;
    }
    .nav-item i { width:18px; text-align:center; font-size:14px; }
    .nav-item:hover { background:var(--surface2); color:var(--text); }
    .nav-item.active { background:rgba(239,68,68,.12); color:#f87171; }
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

    .stats-grid { display:grid; grid-template-columns:repeat(auto-fit,minmax(180px,1fr)); gap:16px; margin-bottom:24px; }
    .stat-card {
      background:var(--surface); border:1px solid var(--border); border-radius:20px;
      padding:20px 24px; transition:transform 0.3s ease, box-shadow 0.3s ease, border-color 0.2s;
      backdrop-filter:blur(16px);
      box-shadow:0 4px 24px rgba(0,0,0,0.1);
    }
    .stat-card:hover { transform:translateY(-2px); box-shadow:0 8px 32px rgba(0,0,0,0.2); border-color:#ef4444; }
    .stat-card .stat-label { font-size:12px; color:var(--muted2); text-transform:uppercase; letter-spacing:.1em; font-weight:700; }
    .stat-card .stat-value { font-size:32px; font-weight:800; margin-top:8px; }
    .stat-card .stat-desc { font-size:13px; color:var(--muted); margin-top:4px; }

    .section-card {
      background:var(--surface); border:1px solid var(--border); border-radius:20px;
      padding:24px; margin-bottom:20px; backdrop-filter:blur(16px);
      box-shadow:0 4px 24px rgba(0,0,0,0.1); transition:transform 0.3s ease, box-shadow 0.3s ease;
    }
    .section-card:hover { transform:translateY(-1px); box-shadow:0 8px 32px rgba(0,0,0,0.15); border-color:var(--border-hover); }
    .section-card .section-title {
      font-size:13px; font-weight:600; color:var(--muted2); text-transform:uppercase;
      letter-spacing:.08em; margin-bottom:14px;
    }

    input, select, textarea, .input {
      width:100%; background:var(--bg2); border:1px solid var(--border);
      border-radius:10px; padding:11px 14px; color:var(--text); font-size:13px;
      outline:none; transition:border-color .2s;
    }
    input:focus { border-color:#ef4444; }
    .grid-3 { display:grid; grid-template-columns:1fr 1fr 1fr; gap:12px; }
    .grid-2 { display:grid; grid-template-columns:1fr 1fr; gap:12px; }
    .toolbar { display:flex; gap:8px; flex-wrap:wrap; align-items:center; }

    button, .btn {
      padding:10px 18px; border-radius:10px; font-size:13px; font-weight:600;
      cursor:pointer; transition:all .15s; border:none;
    }
    .btn-primary { background:linear-gradient(135deg,#dc2626,#ef4444); color:#fff; }
    .btn-primary:hover { opacity:.9; }
    .btn-secondary { background:var(--surface2); border:1px solid var(--border); color:var(--text); }
    .btn-secondary:hover { background:var(--border); }
    .btn-danger { background:linear-gradient(135deg,#dc2626,#ef4444); color:#fff; }
    .btn-sm { padding:6px 12px; font-size:11px; border-radius:8px; }

    table { width:100%; border-collapse:collapse; font-size:13px; }
    th, td { text-align:left; padding:10px 12px; border-bottom:1px solid var(--border); }
    th { color:var(--muted2); font-size:11px; text-transform:uppercase; letter-spacing:.08em; font-weight:600; }
    tr:hover td { background:rgba(255,255,255,.02); }
    .pill { display:inline-block; padding:3px 10px; border-radius:999px;
      font-size:11px; font-weight:500; }
    .pill.red { background:rgba(239,68,68,.12); color:#f87171; }
    .pill.green { background:rgba(34,197,94,.12); color:#4ade80; }
    .pill.blue { background:rgba(99,102,241,.12); color:#818cf8; }
    .pill.gray { background:rgba(148,163,184,.12); color:#94a3b8; }

    .worker-card {
      background:var(--bg2); border:1px solid var(--border); border-radius:12px;
      padding:14px; display:flex; align-items:center; justify-content:space-between;
      transition:border-color .2s;
    }
    .worker-card:hover { border-color:#ef4444; }
    .worker-card .w-info { display:flex; align-items:center; gap:10px; }
    .worker-card .w-info .w-id { font-size:13px; font-weight:500; }
    .worker-card .w-info .w-time { font-size:11px; color:var(--muted); }

    #toast {
      position:fixed; bottom:24px; right:24px;
      background:var(--surface2); border:1px solid var(--border);
      border-radius:12px; padding:14px 20px; font-size:13px;
      opacity:0; transform:translateY(10px); transition:all .3s ease;
      pointer-events:none; z-index:999; max-width:360px;
      box-shadow:0 12px 40px rgba(0,0,0,.4);
    }
    #toast.show { opacity:1; transform:translateY(0); }

    @media(max-width:768px) {
      #sidebar { transform:translateX(-100%); }
      #sidebar.open { transform:translateX(0); box-shadow:0 0 40px rgba(0,0,0,.5); }
      #main { margin-left:0; }
      #topbar .left button { display:block; }
      .grid-3, .grid-2 { grid-template-columns:1fr; }
      .stats-grid { grid-template-columns:repeat(2,1fr); }
      #content { padding:16px; }
      #sidebar { width:280px; }
    }
    @media(min-width:769px) {
      #sidebar { transform:translateX(0) !important; }
    }
    @keyframes slideUp { from{opacity:0;transform:translateY(12px)} to{opacity:1;transform:translateY(0)} }
    .slide-up { animation:slideUp .3s ease; }

    /* ── Modal ── */
    #modal-overlay {
      position:fixed; inset:0; background:rgba(4,7,18,.72); z-index:500;
      display:none; align-items:flex-start; justify-content:center; overflow-y:auto;
      padding:48px 20px;
    }
    #modal-overlay.show { display:flex; }
    .modal {
      background:var(--surface); border:1px solid var(--border); border-radius:16px;
      width:760px; max-width:100%; animation:slideUp .2s ease;
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
    .modal .modal-foot { display:flex; gap:10px; padding:14px 22px; border-top:1px solid var(--border); justify-content:flex-end; }
    pre {
      background:var(--bg2); border:1px solid var(--border); border-radius:10px;
      padding:14px; font-size:12px; line-height:1.5; overflow:auto;
      max-height:320px; font-family:'SF Mono','Fira Code',monospace;
      white-space:pre-wrap; word-break:break-all;
    }
  </style>
</head>
<body>
  <div id="toast"></div>
  <div id="modal-overlay"><div class="modal">
    <div class="modal-head"><h3 id="modal-title">Failed Job</h3><button onclick="closeModal()"><i class="fas fa-times"></i></button></div>
    <div class="modal-body" id="modal-body"></div>
    <div class="modal-foot" id="modal-foot"></div>
  </div></div>

  <!-- Dashboard -->
  <div id="app">
  <div class="layout">

    <!-- Sidebar -->
    <nav id="sidebar">
      <div class="sidebar-brand">
        <div class="brand-icon"><i class="fas fa-trash-alt"></i></div>
        <div><h2>TaskQueue</h2><span>DLQ Console</span></div>
      </div>
      <div class="sidebar-nav">
        <a class="nav-item" href="/ui" style="color:var(--accent2);margin-bottom:6px;border:1px solid rgba(99,102,241,.2);background:rgba(99,102,241,.07);">
          <i class="fas fa-arrow-left"></i> Operator Panel
        </a>
        <div style="height:1px;background:var(--border);margin:6px 0 10px;"></div>
        <a class="nav-item active" data-page="overview" onclick="showPage('overview')">
          <i class="fas fa-chart-pie"></i> Overview
        </a>
        <a class="nav-item" data-page="workers" onclick="showPage('workers');loadWorkers()">
          <i class="fas fa-server"></i> Workers
        </a>
        <a class="nav-item" data-page="dlq" onclick="showPage('dlq');loadDLQ()">
          <i class="fas fa-list"></i> DLQ Table
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
            <h3 id="page-title">DLQ Overview</h3>
            <p id="page-desc">Dead letter queue management console</p>
          </div>
        </div>
        <div class="right">
          <span id="refresh-status" style="font-size:12px;color:var(--muted);">
            <span class="status-dot yellow"></span> Auto-refresh Off
          </span>
          <button id="toggle-refresh" class="btn-secondary btn-sm">
            <i class="fas fa-sync"></i> Toggle Refresh
          </button>
          <button class="btn-secondary btn-sm" onclick="logout()"><i class="fas fa-sign-out-alt"></i></button>
        </div>
      </div>
      <div id="content">

        <div id="ro-banner" style="display:none;background:rgba(245,158,11,.1);border:1px solid var(--warn);color:var(--warn);border-radius:10px;padding:10px 16px;font-size:13px;margin-bottom:16px;">
          <i class="fas fa-eye"></i> Read-only session. Changes require an admin account.
        </div>

        <!-- Page: Overview -->
        <div id="page-overview" class="page active">
          <div class="stats-grid">
            <div class="stat-card"><div class="stat-label">Failed Jobs</div><div id="stat-failed" class="stat-value">0</div><div class="stat-desc">Current DLQ depth</div></div>
            <div class="stat-card"><div class="stat-label">Queues</div><div id="stat-queues" class="stat-value">0</div><div class="stat-desc">Distinct job types</div></div>
            <div class="stat-card"><div class="stat-label">Tenants</div><div id="stat-tenants" class="stat-value">0</div><div class="stat-desc">Affected tenants</div></div>
            <div class="stat-card"><div class="stat-label">Workers</div><div id="stat-workers" class="stat-value">0</div><div class="stat-desc">Active heartbeats</div></div>
          </div>
          <div class="section-card slide-up">
            <div class="section-title"><i class="fas fa-tools"></i> Quick Actions</div>
            <div class="toolbar">
              <button class="btn-primary btn-sm" onclick="loadDLQ()"><i class="fas fa-sync"></i> Refresh DLQ</button>
              <button class="btn-secondary btn-sm" onclick="exportDLQ()"><i class="fas fa-download"></i> Export</button>
              <button class="btn-secondary btn-sm" onclick="showPage('workers');loadWorkers()"><i class="fas fa-server"></i> Workers</button>
              <button class="btn-secondary btn-sm" onclick="showPage('dlq');loadDLQ()"><i class="fas fa-list"></i> DLQ Table</button>
            </div>
          </div>
        </div>

        <!-- Page: Workers -->
        <div id="page-workers" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-server"></i> Active Workers</div>
            <div id="workers-list" class="grid-2" style="margin-top:10px;">
              <div class="worker-card"><div class="w-info"><i class="fas fa-circle" style="font-size:6px;color:var(--muted);"></i><span style="color:var(--muted);">No active workers</span></div></div>
            </div>
          </div>
        </div>

        <!-- Page: DLQ Table -->
        <div id="page-dlq" class="page">
          <div class="section-card">
            <div class="section-title"><i class="fas fa-filter"></i> Filters</div>
            <div class="grid-3" style="margin-bottom:12px;">
              <div><label style="display:block;font-size:12px;color:var(--muted);margin-bottom:4px;">Queue</label><input id="filter-queue" value="email" placeholder="Filter by queue..." /></div>
              <div><label style="display:block;font-size:12px;color:var(--muted);margin-bottom:4px;">Tenant</label><input id="filter-tenant" value="tenant-a" placeholder="Filter by tenant..." /></div>
              <div><label style="display:block;font-size:12px;color:var(--muted);margin-bottom:4px;">Search</label><input id="search-dlq" placeholder="Search in results..." /></div>
            </div>
            <div class="grid-3" style="margin-bottom:12px;">
              <div><label style="display:block;font-size:12px;color:var(--muted);margin-bottom:4px;">Page</label><input id="dlq-page" type="number" value="1" min="1" /></div>
              <div><label style="display:block;font-size:12px;color:var(--muted);margin-bottom:4px;">Limit</label><input id="dlq-limit" type="number" value="20" min="1" max="50" /></div>
              <div style="display:flex;align-items:flex-end;gap:8px;">
                <button class="btn-secondary btn-sm" onclick="dlqPrevPage()"><i class="fas fa-chevron-left"></i> Prev</button>
                <button class="btn-secondary btn-sm" onclick="dlqNextPage()"><i class="fas fa-chevron-right"></i> Next</button>
              </div>
            </div>
            <div class="toolbar">
              <button class="btn-primary btn-sm" onclick="loadDLQ()"><i class="fas fa-sync"></i> Refresh</button>
              <button class="btn-secondary btn-sm" onclick="exportDLQ()"><i class="fas fa-download"></i> Export</button>
              <span id="dlq-page-info" style="font-size:12px;color:var(--muted);">Page 1</span>
            </div>
          </div>
          <div class="section-card">
            <div class="section-title"><i class="fas fa-list"></i> Failed Jobs</div>
            <div id="loading" style="display:none;padding:20px;text-align:center;color:var(--muted);">Loading...</div>
            <div style="overflow-x:auto;">
              <table>
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Type</th>
                    <th>Status</th>
                    <th>Tenant</th>
                    <th>Error</th>
                    <th>Retries</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody id="dlq-table-body">
                  <tr><td colspan="7" style="text-align:center;color:var(--muted);padding:24px;">No failed jobs found. Load the DLQ to see results.</td></tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>

      </div>
    </div>
  </div>
  </div>

  <script>
    const API_BASE='/api/v1';
    const IDLE_TIMEOUT=15*60*1000;
    let SESSION={authenticated:false,username:'',role:'',csrf_token:''};
    let refreshTimer=null;
    let idleTimer=null;
    let dlqState={page:1,limit:20};

    function toast(msg){
      const t=document.getElementById('toast');t.textContent=msg;
      t.classList.add('show');setTimeout(()=>t.classList.remove('show'),2500);
    }
    function esc(s){
      return String(s==null?'':s).replace(/[&<>"']/g,function(c){
        return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];
      });
    }
    function isAdmin(){return SESSION.role==='admin';}
    function confirmAction(message){return window.confirm(message);}

    async function api(path,options={}){
      const method=options.method||'GET';
      const headers=Object.assign({},options.headers||{},{'Content-Type':'application/json'});
      if(method!=='GET'&&method!=='HEAD'&&SESSION.csrf_token){headers['X-CSRF-Token']=SESSION.csrf_token;}
      try{
        const res=await fetch(path,Object.assign({},options,{headers:headers,credentials:'same-origin'}));
        if(res.status===204)return null;
        return res.json();
      }catch(_){return null;}
    }

    async function loadSession(){
      try{
        const res=await fetch(API_BASE+'/session',{credentials:'same-origin'});
        if(res.ok){SESSION=await res.json();}
      }catch(_){SESSION={authenticated:false};}
      if(!SESSION.authenticated){window.location.href='/login';return false;}
      applyRoleGating();
      startIdleTimer();
      return true;
    }
    function applyRoleGating(){
      const admin=isAdmin();
      document.querySelectorAll('[data-admin]').forEach(function(el){el.style.display=admin?'':'none';});
      const banner=document.getElementById('ro-banner');
      if(banner){banner.style.display=admin?'none':'';}
    }
    function startIdleTimer(){
      const reset=function(){
        clearTimeout(idleTimer);
        idleTimer=setTimeout(logout,IDLE_TIMEOUT);
      };
      ['click','keydown','mousemove','scroll','touchstart'].forEach(function(ev){
        window.addEventListener(ev,reset,{passive:true});
      });
      reset();
    }
    async function logout(){
      try{await fetch(API_BASE+'/logout',{method:'POST',credentials:'same-origin'});}catch(_){}
      window.location.href='/login';
    }

    function toggleSidebar(){
      document.getElementById('sidebar').classList.toggle('open');
    }
    document.getElementById('sidebar').addEventListener('click',function(){
      if(window.innerWidth<=768)this.classList.remove('open');
    });

    function showPage(name){
      const titles={overview:'DLQ Overview',workers:'Workers',dlq:'DLQ Table'};
      const descs={overview:'Dead letter queue management console',workers:'Active worker instances',dlq:'Failed jobs detail table'};
      document.querySelectorAll('.page').forEach(p=>p.classList.remove('active'));
      document.querySelectorAll('.nav-item').forEach(n=>n.classList.remove('active'));
      const p=document.getElementById('page-'+name);if(p)p.classList.add('active');
      const nav=document.querySelector('.nav-item[data-page="'+name+'"]');if(nav)nav.classList.add('active');
      document.getElementById('page-title').textContent=titles[name]||name;
      document.getElementById('page-desc').textContent=descs[name]||'';
    }

    async function loadDLQ(){
      const loading=document.getElementById('loading');loading.style.display='block';
      try{
        const queue=document.getElementById('filter-queue').value;
        const tenant=document.getElementById('filter-tenant').value;
        const page=parseInt(document.getElementById('dlq-page').value)||1;
        const limit=parseInt(document.getElementById('dlq-limit').value)||20;
        const p=new URLSearchParams({page:page,limit:limit});
        if(queue)p.set('queue',queue);
        const data=await api(API_BASE+'/dlq?'+p.toString());
        const jobs=Array.isArray(data)?data:((data&&data.jobs)||[]);
        const total=(data&&data.total!=null)?data.total:jobs.length;
        dlqState.page=page;
        document.getElementById('dlq-page-info').innerText='Page '+page+' · '+total+' total';
        const filtered=jobs.filter(j=>!tenant||(j.tenant_id||'').includes(tenant));
        renderSummary(filtered,total);
        renderTable(filtered);
      }catch(e){console.error(e);}finally{loading.style.display='none';}
    }
    function dlqPrevPage(){
      if(dlqState.page>1){dlqState.page--;document.getElementById('dlq-page').value=dlqState.page;loadDLQ();}
    }
    function dlqNextPage(){
      dlqState.page++;document.getElementById('dlq-page').value=dlqState.page;loadDLQ();
    }
    async function loadWorkers(){
      try{
        const workers=await api('/workers');
        renderWorkers(workers||[]);
      }catch(e){console.error(e);}
    }
    function renderSummary(jobs,total){
      const tenants=new Set(),queues=new Set();
      (jobs||[]).forEach(j=>{if(j.tenant_id)tenants.add(j.tenant_id);if(j.type)queues.add(j.type);});
      document.getElementById('stat-failed').innerText=String(total!=null?total:(jobs||[]).length);
      document.getElementById('stat-queues').innerText=String(queues.size);
      document.getElementById('stat-tenants').innerText=String(tenants.size);
    }
    function renderWorkers(workers){
      document.getElementById('stat-workers').innerText=String((workers||[]).length);
      const list=document.getElementById('workers-list');
      if(!workers||workers.length===0){
        list.innerHTML='<div class="worker-card"><div class="w-info"><i class="fas fa-circle" style="font-size:6px;color:var(--muted);"></i><span style="color:var(--muted);">No active workers</span></div></div>';
        return;
      }
      list.innerHTML=workers.map(function(w){
        return '<div class="worker-card"><div class="w-info"><i class="fas fa-circle" style="font-size:8px;color:var(--good);"></i>'+
          '<div><div class="w-id">'+esc(w.id)+'</div><div class="w-time">'+(w.last_heartbeat?new Date(w.last_heartbeat).toLocaleString():'Just now')+'</div></div></div>'+
          '<span class="pill green">Live</span></div>';
      }).join('');
    }
    function lastError(j){
      const hist=j.error_history||[];
      return hist.length?hist[hist.length-1].error:(j.error||'');
    }
    function renderTable(jobs){
      const tbody=document.getElementById('dlq-table-body');
      if(!jobs||jobs.length===0){
        tbody.innerHTML='<tr><td colspan="7" style="text-align:center;color:var(--muted);padding:24px;">No failed jobs found.</td></tr>';
        return;
      }
      const isAdminSession=isAdmin();
      tbody.innerHTML=jobs.map(function(j){
        const actions=
          '<button class="btn-secondary btn-sm" data-detail="'+esc(j.id)+'" style="margin-right:4px;">Details</button>'+
          (isAdminSession?
            '<button class="btn-primary btn-sm" data-replay="'+esc(j.id)+'" style="margin-right:4px;">Replay</button>'+
            '<button class="btn-danger btn-sm" data-purge="'+esc(j.id)+'">Purge</button>':
            '<span style="color:var(--muted);font-size:11px;">read-only</span>');
        return '<tr><td style="font-size:12px;font-family:monospace;color:var(--muted);">'+esc(j.id.substring(0,12))+'...</td>'+
          '<td><span class="pill blue">'+esc(j.type)+'</span></td>'+
          '<td><span class="pill red">'+esc(j.status)+'</span></td>'+
          '<td style="font-size:12px;color:var(--muted);">'+esc(j.tenant_id||'-')+'</td>'+
          '<td style="font-size:12px;color:#f87171;max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="'+esc(lastError(j))+'">'+esc(String(lastError(j)).substring(0,50))+'</td>'+
          '<td style="font-size:12px;color:var(--muted);">'+esc(j.retries||0)+'</td>'+
          '<td style="white-space:nowrap;">'+actions+'</td></tr>';
      }).join('');
    }
    document.getElementById('dlq-table-body').addEventListener('click',function(e){
      const detail=e.target.closest('[data-detail]');
      if(detail){openDLQDetail(detail.getAttribute('data-detail'));return;}
      const replay=e.target.closest('[data-replay]');
      if(replay){replayDLQ(replay.getAttribute('data-replay'));return;}
      const purge=e.target.closest('[data-purge]');
      if(purge){purgeDLQ(purge.getAttribute('data-purge'));}
    });

    // ── Detail modal ──
    function openModal(title,body,foot){
      document.getElementById('modal-title').innerHTML=title;
      document.getElementById('modal-body').innerHTML=body||'';
      document.getElementById('modal-foot').innerHTML=foot||'';
      document.getElementById('modal-overlay').classList.add('show');
    }
    function closeModal(){
      document.getElementById('modal-overlay').classList.remove('show');
    }
    document.getElementById('modal-overlay').addEventListener('click',function(e){
      if(e.target===this)closeModal();
    });
    document.addEventListener('keydown',function(e){if(e.key==='Escape')closeModal();});
    async function openDLQDetail(id){
      const j=await api(API_BASE+'/dlq/'+encodeURIComponent(id));
      if(!j){toast('Job not found');return;}
      const history=(j.error_history||[]).map(function(h){
        return '<tr><td>'+(h.attempt!=null?'#'+h.attempt:'')+'</td><td>'+esc(h.timestamp||'')+'</td><td>'+esc(h.error)+'</td></tr>';
      }).join('')||'<tr><td colspan="3" style="text-align:center;color:var(--muted);">No recorded attempts.</td></tr>';
      const body='<table style="margin-bottom:12px;"><tbody>'+
        '<tr><th style="width:150px;">ID</th><td>'+esc(j.id)+'</td></tr>'+
        '<tr><th>Type</th><td>'+esc(j.type)+'</td></tr>'+
        '<tr><th>Tenant</th><td>'+esc(j.tenant_id||'')+'</td></tr>'+
        '<tr><th>Retries</th><td>'+esc(j.retries)+' / '+esc(j.max_retries)+'</td></tr>'+
        '<tr><th>Updated</th><td>'+esc(j.updated_at)+'</td></tr>'+
        '</tbody></table>'+
        '<div class="section-title" style="font-size:11px;">Error History</div>'+
        '<table><thead><tr><th>Attempt</th><th>Timestamp</th><th>Error</th></tr></thead><tbody>'+history+'</tbody></table>'+
        '<div class="section-title" style="font-size:11px;margin-top:14px;">Payload</div>'+
        '<pre>'+esc(JSON.stringify((j.payload)||{},null,2))+'</pre>';
      const foot='<button class="btn-secondary btn-sm" onclick="closeModal()">Close</button>'+
        (isAdmin()?'<button class="btn-primary btn-sm" onclick="replayDLQ(\''+esc(j.id)+'\')"><i class="fas fa-redo"></i> Replay</button>'+
        '<button class="btn-danger btn-sm" onclick="purgeDLQ(\''+esc(j.id)+'\')"><i class="fas fa-times"></i> Purge</button>':'');
      openModal('Failed Job '+esc(j.id),body,foot);
    }

    async function replayDLQ(id){
      if(!confirmAction('Replay job '+id+' from the dead letter queue?'))return;
      await api(API_BASE+'/dlq/'+encodeURIComponent(id)+'/replay',{method:'POST'});
      loadDLQ();toast('Replayed job: '+id);
    }
    async function purgeDLQ(id){
      if(!confirmAction('Permanently purge job '+id+'? This cannot be undone.'))return;
      await api(API_BASE+'/dlq/'+encodeURIComponent(id),{method:'DELETE'});
      loadDLQ();toast('Purged job: '+id);
    }
    async function exportDLQ(){
      const raw=document.getElementById('dlq-table-body').innerText;
      const queue=document.getElementById('filter-queue').value;
      const data=await api(API_BASE+'/dlq?queue='+encodeURIComponent(queue));
      const jobs=Array.isArray(data)?data:((data&&data.jobs)||[]);
      const blob=new Blob([JSON.stringify(jobs,null,2)],{type:'application/json'});
      const url=URL.createObjectURL(blob);const a=document.createElement('a');
      a.href=url;a.download='dlq-export.json';a.click();URL.revokeObjectURL(url);
    }

    async function loadDashboard(){
      await Promise.all([loadDLQ().catch(function(){}),loadWorkers().catch(function(){})]);
    }

    // ── Live refresh via SSE ──
    let eventSource=null;
    function connectEvents(){
      try{eventSource=new EventSource(API_BASE+'/events');}catch(e){return;}
      eventSource.onopen=function(){
        const s=document.getElementById('refresh-status');
        if(!refreshTimer)s.innerHTML='<span class="status-dot green"></span> Live';
      };
      eventSource.onmessage=function(e){
        let data={};
        try{data=JSON.parse(e.data);}catch(_){}
        if(data.kind==='dlq'||data.kind==='job'){loadDLQ().catch(function(){});}
      };
    }
    loadSession().then(function(ok){if(ok){loadDashboard();connectEvents();}});

    document.getElementById('toggle-refresh').addEventListener('click',()=>{
      const status=document.getElementById('refresh-status');
      if(refreshTimer){
        clearInterval(refreshTimer);refreshTimer=null;
        status.innerHTML='<span class="status-dot yellow"></span> Auto-refresh Off';
      }else{
        refreshTimer=setInterval(loadDLQ,10000);
        status.innerHTML='<span class="status-dot green"></span> Auto-refresh On (10s)';
        loadDashboard();
      }
    });
    document.getElementById('search-dlq').addEventListener('input',function(){
      const q=this.value.toLowerCase();
      document.querySelectorAll('#dlq-table-body tr').forEach(tr=>{
        tr.style.display=tr.innerText.toLowerCase().includes(q)?'':'none';
      });
    });
  </script>
</body>
</html>`
