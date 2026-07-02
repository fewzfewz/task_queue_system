package handler

import (
	"fmt"
	"net/http"
	"strings"
)

// ServeAdminDLQ renders the single-page HTML management console for the Dead Letter Queue.
func (h *JobHandler) ServeAdminDLQ(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, strings.ReplaceAll(adminHTML, "__API_KEY__", h.apiKey))
}

const adminHTML = `
<!DOCTYPE html>
<html lang="en" class="h-full bg-gray-900">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>TaskQueue | DLQ Control</title>
    <script src="https://cdn.tailwindcss.com"></script>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;600;700&display=swap" rel="stylesheet">
    <style>
        body { font-family: 'Inter', sans-serif; }
        .glass { background: rgba(17, 24, 39, 0.7); backdrop-filter: blur(12px); border: 1px solid rgba(255, 255, 255, 0.1); }
        .modal-bg { background: rgba(0, 0, 0, 0.8); backdrop-filter: blur(4px); }
    </style>
</head>
<body class="h-full text-gray-100 antialiased overflow-hidden flex flex-col">

    <!-- Login overlay -->
    <div id="login-overlay" class="fixed inset-0 modal-bg flex items-center justify-center z-50">
        <div class="glass rounded-2xl p-8 w-96 max-w-[90vw]">
            <h2 class="text-2xl font-bold mb-1">TaskQueue</h2>
            <p class="text-sm text-gray-400 mb-6">Sign in to manage the dead letter queue</p>
            <div class="space-y-4">
                <input id="login-user" type="text" placeholder="Username" value="admin" class="w-full bg-gray-800 border border-white/10 rounded-lg px-4 py-3 text-sm focus:ring-indigo-500 focus:border-indigo-500" />
                <input id="login-pass" type="password" placeholder="Password" value="admin123" class="w-full bg-gray-800 border border-white/10 rounded-lg px-4 py-3 text-sm focus:ring-indigo-500 focus:border-indigo-500" />
                <button onclick="login()" class="w-full px-4 py-3 bg-indigo-600 hover:bg-indigo-500 rounded-lg text-sm font-semibold transition-colors">Sign in</button>
                <p id="login-error" class="text-red-400 text-sm hidden">Invalid credentials</p>
            </div>
        </div>
    </div>

    <!-- Dashboard -->
    <div id="app" class="flex flex-col h-full" style="display:none;">

    <!-- Header -->
    <header class="glass sticky top-0 z-50 px-6 py-4 flex items-center justify-between shadow-2xl">
        <div class="flex items-center space-x-3">
            <div class="w-10 h-10 bg-indigo-600 rounded-lg flex items-center justify-center shadow-lg shadow-indigo-500/20">
                <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path></svg>
            </div>
            <div>
                <h1 class="text-xl font-bold tracking-tight">TaskQueue <span class="text-indigo-400">DLQ</span></h1>
                <p class="text-xs text-gray-400 font-medium uppercase tracking-widest">Management Console</p>
            </div>
        </div>
        <div class="flex items-center space-x-4">
            <div class="flex items-center space-x-2 bg-gray-800/50 rounded-full px-4 py-1.5 border border-white/5">
                <span class="w-2 h-2 bg-green-500 rounded-full animate-pulse"></span>
                <span id="refresh-status" class="text-xs font-semibold text-gray-300">Auto-refresh Off</span>
                <button id="toggle-refresh" class="ml-2 p-1 hover:bg-gray-700 rounded transition-colors">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"></path></svg>
                </button>
            </div>
            <button onclick="logout()" class="p-2 hover:bg-red-500/10 hover:text-red-400 text-gray-400 rounded-full transition-all">
                <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"></path></svg>
            </button>
        </div>
    </header>

    <main class="flex-1 overflow-auto p-6 space-y-6">
        <!-- Overview -->
        <section class="grid gap-4 md:grid-cols-4">
            <div class="glass rounded-xl p-4">
                <p class="text-xs uppercase tracking-widest text-gray-500 font-bold">Failed jobs</p>
                <div id="stat-failed" class="mt-2 text-3xl font-bold text-white">0</div>
                <p class="text-sm text-gray-400 mt-1">Current DLQ depth</p>
            </div>
            <div class="glass rounded-xl p-4">
                <p class="text-xs uppercase tracking-widest text-gray-500 font-bold">Queues</p>
                <div id="stat-queues" class="mt-2 text-3xl font-bold text-white">0</div>
                <p class="text-sm text-gray-400 mt-1">Distinct failed job types</p>
            </div>
            <div class="glass rounded-xl p-4">
                <p class="text-xs uppercase tracking-widest text-gray-500 font-bold">Tenants</p>
                <div id="stat-tenants" class="mt-2 text-3xl font-bold text-white">0</div>
                <p class="text-sm text-gray-400 mt-1">Tenants represented in DLQ</p>
            </div>
            <div class="glass rounded-xl p-4">
                <p class="text-xs uppercase tracking-widest text-gray-500 font-bold">Workers</p>
                <div id="stat-workers" class="mt-2 text-3xl font-bold text-white">0</div>
                <p class="text-sm text-gray-400 mt-1">Active worker heartbeats</p>
            </div>
        </section>

        <!-- Toolbar -->
        <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 glass p-4 rounded-xl">
            <div class="flex flex-col lg:flex-row lg:items-center gap-3">
                <div class="relative group">
                    <input type="text" id="filter-queue" placeholder="Filter by queue..." value="email"
                        class="bg-gray-800 border-white/5 text-sm rounded-lg focus:ring-indigo-500 focus:border-indigo-500 block w-56 pl-10 pr-3 py-2 placeholder-gray-500 transition-all">
                    <svg class="w-4 h-4 text-gray-500 absolute left-3 top-2.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path></svg>
                </div>
                <div class="relative group">
                    <input type="text" id="filter-tenant" placeholder="Filter by tenant..." value="tenant-a"
                        class="bg-gray-800 border-white/5 text-sm rounded-lg focus:ring-indigo-500 focus:border-indigo-500 block w-56 pl-10 pr-3 py-2 placeholder-gray-500 transition-all">
                    <svg class="w-4 h-4 text-gray-500 absolute left-3 top-2.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a4 4 0 00-5-3.87M12 20H7v-2a4 4 0 015-3.87m0 0A4 4 0 1012 4a4 4 0 000 8.13z"></path></svg>
                </div>
            </div>
            <div class="flex items-center gap-3">
                <input type="text" id="search-dlq" placeholder="Search in results..."
                    class="bg-gray-800 border-white/5 text-sm rounded-lg focus:ring-indigo-500 focus:border-indigo-500 block w-48 px-3 py-2 placeholder-gray-500">
                <button onclick="loadDLQ()" class="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 rounded-lg text-xs font-semibold transition-colors">Refresh</button>
            </div>
        </div>

        <!-- Workers -->
        <div class="glass rounded-xl p-4">
            <h2 class="text-sm font-bold uppercase tracking-widest text-gray-400 mb-4">Active Workers</h2>
            <div id="workers-list" class="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
                <div class="text-sm text-gray-500 italic">No active workers reported</div>
            </div>
        </div>

        <!-- DLQ Table -->
        <div class="glass rounded-xl overflow-hidden">
            <div id="loading" class="hidden p-8 text-center text-gray-400 text-sm">Loading failed jobs...</div>
            <div class="overflow-x-auto">
                <table class="w-full text-sm">
                    <thead>
                        <tr class="border-b border-white/5 text-gray-400 uppercase tracking-wider text-[10px]">
                            <th class="text-left p-4 font-semibold">ID</th>
                            <th class="text-left p-4 font-semibold">Type</th>
                            <th class="text-left p-4 font-semibold">Status</th>
                            <th class="text-left p-4 font-semibold">Tenant</th>
                            <th class="text-left p-4 font-semibold">Error</th>
                            <th class="text-left p-4 font-semibold">Attempts</th>
                            <th class="text-left p-4 font-semibold">Actions</th>
                        </tr>
                    </thead>
                    <tbody id="dlq-table-body">
                        <tr><td colspan="7" class="p-8 text-center text-gray-500">No failed jobs found. Load the DLQ to see results.</td></tr>
                    </tbody>
                </table>
            </div>
        </div>
    </main>
    </div>

    <script>
        const API_BASE = '/api/v1';
        const TOKEN_KEY = 'task_queue_api_key';
        let refreshTimer = null;

        function getToken() { return localStorage.getItem(TOKEN_KEY) || ''; }

        async function api(path, options = {}) {
            try {
                const t = getToken();
                const res = await fetch(path, {
                    ...options,
                    headers: { ...options.headers, 'Content-Type': 'application/json', ...(t ? {'X-API-Key': t} : {}) }
                });
                if (res.status === 204) return null;
                return res.json();
            } catch (_) { return null; }
        }

        async function login() {
            const user = document.getElementById('login-user').value.trim();
            const pass = document.getElementById('login-pass').value.trim();
            if (!user || !pass) { document.getElementById('login-error').classList.remove('hidden'); return; }
            const res = await api('/api/v1/login', {
                method: 'POST',
                body: JSON.stringify({username: user, password: pass}),
                headers: { 'Content-Type': 'application/json' }
            });
            if (res && res.api_key) {
                localStorage.setItem(TOKEN_KEY, res.api_key);
                document.getElementById('login-overlay').style.display = 'none';
                document.getElementById('app').style.display = 'flex';
                loadDashboard();
            } else {
                document.getElementById('login-error').classList.remove('hidden');
            }
        }

        function logout() {
            localStorage.removeItem(TOKEN_KEY);
            document.getElementById('app').style.display = 'none';
            document.getElementById('login-overlay').style.display = 'flex';
        }

        document.getElementById('login-pass').addEventListener('keydown', function(e) {
            if (e.key === 'Enter') login();
        });

        if (getToken()) {
            document.getElementById('login-overlay').style.display = 'none';
            document.getElementById('app').style.display = 'flex';
        }

        async function loadDLQ() {
            const loading = document.getElementById('loading');
            loading.classList.remove('hidden');
            try {
                const queue = document.getElementById('filter-queue').value;
                const tenant = document.getElementById('filter-tenant').value;
                const data = await api(API_BASE + '/dlq?queue=' + queue);
                const filtered = (data || []).filter(j => {
                    if (!tenant) return true;
                    return (j.tenant_id || '').includes(tenant);
                });
                renderSummary(filtered);
                renderTable(filtered);
            } catch (e) {
                console.error(e);
            } finally {
                loading.classList.add('hidden');
            }
        }

        async function loadWorkers() {
            try {
                const workers = await api(API_BASE + '/workers');
                renderWorkers(workers || []);
            } catch (e) {
                console.error(e);
            }
        }

        function renderSummary(jobs) {
            const tenants = new Set();
            const queues = new Set();
            (jobs || []).forEach(j => {
                if (j.tenant_id) tenants.add(j.tenant_id);
                if (j.type) queues.add(j.type);
            });
            document.getElementById('stat-failed').innerText = String((jobs || []).length);
            document.getElementById('stat-queues').innerText = String(queues.size);
            document.getElementById('stat-tenants').innerText = String(tenants.size);
        }

        function renderWorkers(workers) {
            document.getElementById('stat-workers').innerText = String((workers || []).length);
            const list = document.getElementById('workers-list');
            if (!workers || workers.length === 0) {
                list.innerHTML = '<div class="text-sm text-gray-500 italic">No active workers reported</div>';
                return;
            }
            list.innerHTML = workers.map(function(w) { return '<div class="bg-gray-950/60 border border-white/5 rounded-xl p-4">' +
                '<div class="flex items-start justify-between gap-3">' +
                '<div>' +
                '<div class="font-mono text-sm text-white">' + w.id + '</div>' +
                '<div class="text-xs text-gray-500 mt-1">Last heartbeat</div>' +
                '</div>' +
                '<span class="px-2 py-1 rounded-full text-[10px] uppercase tracking-widest bg-green-500/10 text-green-400 border border-green-500/20">Live</span>' +
                '</div>' +
                '<div class="mt-3 text-sm text-gray-300">' + new Date(w.last_heartbeat).toLocaleString() + '</div>' +
                '</div>'; }).join('');
        }

        function renderTable(jobs) {
            const tbody = document.getElementById('dlq-table-body');
            if (!jobs || jobs.length === 0) {
                tbody.innerHTML = '<tr><td colspan="7" class="p-8 text-center text-gray-500">No failed jobs found.</td></tr>';
                return;
            }
            tbody.innerHTML = jobs.map(function(j) { return '<tr class="border-b border-white/5 hover:bg-white/5 transition-colors">' +
                '<td class="p-4 font-mono text-xs text-gray-300">' + j.id + '</td>' +
                '<td class="p-4"><span class="px-2 py-0.5 rounded-full text-[10px] uppercase tracking-wider bg-indigo-500/10 text-indigo-300 border border-indigo-500/20">' + j.type + '</span></td>' +
                '<td class="p-4"><span class="px-2 py-0.5 rounded-full text-[10px] uppercase tracking-wider bg-red-500/10 text-red-300 border border-red-500/20">' + j.status + '</span></td>' +
                '<td class="p-4 text-xs text-gray-400">' + (j.tenant_id || '-') + '</td>' +
                '<td class="p-4 text-xs text-red-300 max-w-[200px] truncate">' + (j.last_error || j.result || '').substring(0, 60) + '</td>' +
                '<td class="p-4 text-xs text-gray-400">' + (j.attempts || 0) + '</td>' +
                '<td class="p-4">' +
                '<button onclick="replayDLQ(\'' + j.id + '\')" class="px-3 py-1 bg-indigo-600 hover:bg-indigo-500 rounded text-[10px] font-semibold transition-colors">Replay</button>' +
                '<button onclick="purgeDLQ(\'' + j.id + '\')" class="ml-1 px-3 py-1 bg-red-600/50 hover:bg-red-500 rounded text-[10px] font-semibold transition-colors">Purge</button>' +
                '</td>' +
                '</tr>'; }).join('');
        }

        window.replayDLQ = async function(id) {
            await api(API_BASE + '/dlq/' + id + '/replay', { method: 'POST' });
            loadDLQ();
        };

        window.purgeDLQ = async function(id) {
            await api(API_BASE + '/dlq/' + id, { method: 'DELETE' });
            loadDLQ();
        };

        async function loadDashboard() {
            await Promise.all([
                loadDLQ().catch(function(){}),
                loadWorkers().catch(function(){})
            ]);
        }

        if (getToken()) loadDashboard();

        document.getElementById('toggle-refresh').addEventListener('click', () => {
            const status = document.getElementById('refresh-status');
            if (refreshTimer) {
                clearInterval(refreshTimer);
                refreshTimer = null;
                status.innerText = 'Auto-refresh Off';
                status.classList.replace('text-indigo-400', 'text-gray-300');
            } else {
                refreshTimer = setInterval(loadDLQ, 10000);
                status.innerText = 'Auto-refresh On (10s)';
                status.classList.replace('text-gray-300', 'text-indigo-400');
                loadDashboard();
            }
        });

        document.getElementById('search-dlq').addEventListener('input', function() {
            const q = this.value.toLowerCase();
            document.querySelectorAll('#dlq-table-body tr').forEach(tr => {
                tr.style.display = tr.innerText.toLowerCase().includes(q) ? '' : 'none';
            });
        });
    </script>
</body>
</html>
`
