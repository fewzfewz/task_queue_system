package handler

import (
	"fmt"
	"net/http"
)

// ServeAdminDLQ renders the single-page HTML management console for the Dead Letter Queue.
func (h *JobHandler) ServeAdminDLQ(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, adminHTML)
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
                    <input type="text" id="filter-queue" placeholder="Filter by queue..." 
                        class="bg-gray-800 border-white/5 text-sm rounded-lg focus:ring-indigo-500 focus:border-indigo-500 block w-56 pl-10 pr-3 py-2 placeholder-gray-500 transition-all">
                    <svg class="w-4 h-4 text-gray-500 absolute left-3 top-2.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path></svg>
                </div>
                <div class="relative group">
                    <input type="text" id="filter-tenant" placeholder="Filter by tenant..." 
                        class="bg-gray-800 border-white/5 text-sm rounded-lg focus:ring-indigo-500 focus:border-indigo-500 block w-56 pl-10 pr-3 py-2 placeholder-gray-500 transition-all">
                    <svg class="w-4 h-4 text-gray-500 absolute left-3 top-2.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a4 4 0 00-5-3.87M12 20H7v-2a4 4 0 015-3.87m0 0A4 4 0 1012 4a4 4 0 000 8.13z"></path></svg>
                </div>
                <button onclick="loadDashboard()" class="px-4 py-2 bg-indigo-600 hover:bg-indigo-500 rounded-lg text-sm font-semibold transition-all shadow-lg shadow-indigo-600/20 active:scale-95">Refresh</button>
            </div>
            
            <button onclick="bulkPurge()" class="px-4 py-2 bg-red-600/10 hover:bg-red-600 text-red-500 hover:text-white border border-red-500/20 rounded-lg text-sm font-semibold transition-all flex items-center space-x-2">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path></svg>
                <span>Purge > 7 Days</span>
            </button>
        </div>

        <!-- Table -->
        <div class="glass rounded-xl overflow-hidden shadow-2xl relative">
            <div id="loading" class="absolute inset-0 bg-gray-900/50 flex items-center justify-center z-10 hidden">
                <div class="w-8 h-8 border-4 border-indigo-500 border-t-transparent rounded-full animate-spin"></div>
            </div>
            <table class="w-full text-left border-collapse">
                <thead class="bg-indigo-600/10 border-b border-white/5">
                    <tr>
                        <th class="px-6 py-4 text-xs font-bold uppercase tracking-wider text-indigo-400">Job ID</th>
                        <th class="px-6 py-4 text-xs font-bold uppercase tracking-wider text-indigo-400">Queue</th>
                        <th class="px-6 py-4 text-xs font-bold uppercase tracking-wider text-indigo-400">Tenant</th>
                        <th class="px-6 py-4 text-xs font-bold uppercase tracking-wider text-indigo-400">Last Error</th>
                        <th class="px-6 py-4 text-xs font-bold uppercase tracking-wider text-indigo-400">Failed At</th>
                        <th class="px-6 py-4 text-xs font-bold uppercase tracking-wider text-indigo-400 text-right">Actions</th>
                    </tr>
                </thead>
                <tbody id="dlq-body" class="divide-y divide-white/5">
                    <!-- Data injected here -->
                </tbody>
            </table>
        </div>

        <section class="glass rounded-xl p-5">
            <div class="flex items-center justify-between mb-4">
                <div>
                    <h2 class="text-lg font-bold text-white">Active Workers</h2>
                    <p class="text-sm text-gray-400">Heartbeats from live worker processes</p>
                </div>
                <button onclick="loadWorkers()" class="px-4 py-2 bg-gray-800 hover:bg-gray-700 rounded-lg text-sm font-semibold transition-all">Refresh workers</button>
            </div>
            <div id="workers-list" class="grid gap-3 md:grid-cols-2 xl:grid-cols-3"></div>
        </section>
    </main>

    <!-- Modal -->
    <div id="modal" class="fixed inset-0 z-[60] flex items-center justify-center p-6 modal-bg opacity-0 pointer-events-none transition-all duration-300">
        <div class="glass w-full max-w-4xl max-h-[85vh] rounded-2xl shadow-3xl overflow-hidden flex flex-col scale-95 transition-transform duration-300">
            <div class="p-6 border-b border-white/5 flex items-center justify-between">
                <h3 class="text-xl font-bold">Job Inspection</h3>
                <button onclick="closeModal()" class="p-2 hover:bg-white/10 rounded-full">
                    <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path></svg>
                </button>
            </div>
            <div class="p-6 overflow-auto flex-1 space-y-6">
                <div>
                    <h4 class="text-xs uppercase tracking-widest text-gray-500 font-bold mb-2">Payload</h4>
                    <pre id="modal-payload" class="bg-gray-950 p-4 rounded-xl border border-white/5 text-sm font-mono text-indigo-300 overflow-x-auto"></pre>
                </div>
                <div>
                    <h4 class="text-xs uppercase tracking-widest text-gray-500 font-bold mb-2">Error Trace</h4>
                    <div id="modal-errors" class="space-y-3"></div>
                </div>
            </div>
        </div>
    </div>

    <script>
        const API_BASE = '/api/v1';
        let refreshTimer = null;

        // Auth
        if (!sessionStorage.getItem('task_queue_token')) {
            const token = prompt('Enter your API/Bearer Token:');
            if (token) sessionStorage.setItem('task_queue_token', token);
            else document.body.innerHTML = '<div class="h-full flex items-center justify-center text-red-400 font-bold">Authentication Required</div>';
        }

        async function api(path, options = {}) {
            const token = sessionStorage.getItem('task_queue_token');
            const res = await fetch(path, {
                ...options,
                headers: {
                    ...options.headers,
                    'Authorization': token.startsWith('Bearer ') ? token : ` + "`" + `Bearer ${token}` + "`" + `,
                    'Content-Type': 'application/json'
                }
            });
            if (res.status === 401) {
                sessionStorage.removeItem('task_queue_token');
                location.reload();
            }
            if (res.status === 204) return null;
            return res.json();
        }

        async function loadDLQ() {
            const loading = document.getElementById('loading');
            loading.classList.remove('hidden');
            try {
                const queue = document.getElementById('filter-queue').value;
                const tenant = document.getElementById('filter-tenant').value;
                const data = await api(` + "`" + `${API_BASE}/dlq?queue=${queue}` + "`" + `);
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
                const workers = await api(` + "`" + `${API_BASE}/workers` + "`" + `);
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

            list.innerHTML = workers.map(w => ` + "`" + `
                <div class="bg-gray-950/60 border border-white/5 rounded-xl p-4">
                    <div class="flex items-start justify-between gap-3">
                        <div>
                            <div class="font-mono text-sm text-white">${w.id}</div>
                            <div class="text-xs text-gray-500 mt-1">Last heartbeat</div>
                        </div>
                        <span class="px-2 py-1 rounded-full text-[10px] uppercase tracking-widest bg-green-500/10 text-green-400 border border-green-500/20">Live</span>
                    </div>
                    <div class="mt-3 text-sm text-gray-300">${new Date(w.last_heartbeat).toLocaleString()}</div>
                </div>
            ` + "`" + `).join('');
        }

        function renderTable(jobs) {
            const body = document.getElementById('dlq-body');
            if (!jobs || jobs.length === 0) {
                body.innerHTML = '<tr><td colspan="6" class="px-6 py-12 text-center text-gray-500 italic">No failed jobs found</td></tr>';
                return;
            }

            body.innerHTML = jobs.map(j => {
                const lastError = j.error_history ? j.error_history[j.error_history.length - 1] : {error: 'Unknown'};
                return ` + "`" + `
                <tr class="hover:bg-white/[0.02] transition-colors group">
                    <td class="px-6 py-4 font-mono text-xs text-gray-400">${j.id.split('-')[0]}...</td>
                    <td class="px-6 py-4"><span class="px-2 py-1 bg-indigo-500/10 text-indigo-400 rounded text-xs font-bold uppercase">${j.type}</span></td>
                    <td class="px-6 py-4 text-sm font-medium text-gray-300">${j.tenant_id}</td>
                    <td class="px-6 py-4 text-sm max-w-xs truncate text-gray-500 group-hover:text-gray-300 transition-colors">${lastError.error}</td>
                    <td class="px-6 py-4 text-xs text-gray-500">${new Date(j.updated_at).toLocaleString()}</td>
                    <td class="px-6 py-4 text-right space-x-2">
                        <button onclick='inspect(${JSON.stringify(j)})' class="p-2 hover:bg-white/10 rounded-lg transition-all" title="Inspect">
                            <svg class="w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/><path d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"/></svg>
                        </button>
                        <button onclick="replay('${j.id}')" class="p-2 hover:bg-green-500/10 rounded-lg transition-all" title="Replay">
                            <svg class="w-4 h-4 text-green-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/></svg>
                        </button>
                        <button onclick="purge('${j.id}')" class="p-2 hover:bg-red-500/10 rounded-lg transition-all" title="Purge">
                            <svg class="w-4 h-4 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>
                        </button>
                    </td>
                </tr>
                ` + "`" + `;
            }).join('');
        }

        window.inspect = function(job) {
            document.getElementById('modal-payload').innerText = JSON.stringify(job.payload, null, 2);
            const errList = job.error_history.map(e => ` + "`" + `
                <div class="bg-red-500/5 border border-red-500/10 p-3 rounded-lg">
                    <div class="flex justify-between text-[10px] text-red-400 font-bold uppercase mb-1">
                        <span>Attempt #${e.attempt}</span>
                        <span>${new Date(e.timestamp).toLocaleString()}</span>
                    </div>
                    <p class="text-sm text-gray-300 font-mono">${e.error}</p>
                </div>
            ` + "`" + `).reverse().join('');
            document.getElementById('modal-errors').innerHTML = errList || '<p class="text-gray-500 italic">No error history recorded</p>';
            
            const modal = document.getElementById('modal');
            modal.classList.remove('opacity-0', 'pointer-events-none');
            modal.children[0].classList.remove('scale-95');
        }

        window.closeModal = function() {
            const modal = document.getElementById('modal');
            modal.classList.add('opacity-0', 'pointer-events-none');
            modal.children[0].classList.add('scale-95');
        }

        window.replay = async function(id) {
            if (!confirm('Re-enqueue this job? It will be reset and added back to the pool.')) return;
            await api(` + "`" + `${API_BASE}/dlq/${id}/replay` + "`" + `, { method: 'POST' });
            loadDLQ();
        }

        window.purge = async function(id) {
            if (!confirm('Permanently delete this failed job?')) return;
            await api(` + "`" + `${API_BASE}/dlq/${id}` + "`" + `, { method: 'DELETE' });
            loadDLQ();
        }

        window.bulkPurge = async function() {
            const date = new Date();
            date.setDate(date.getDate() - 7);
            const iso = date.toISOString();
            if (!confirm(` + "`" + `Delete ALL failed jobs older than ${date.toLocaleString()}?` + "`" + `)) return;
            
            const queue = document.getElementById('filter-queue').value;
            await api(` + "`" + `${API_BASE}/dlq?older_than=${iso}&queue=${queue}` + "`" + `, { method: 'DELETE' });
            loadDashboard();
        }

        window.logout = function() {
            sessionStorage.removeItem('task_queue_token');
            location.reload();
        }

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

        // Initial Load
        async function loadDashboard() {
            await Promise.all([loadDLQ(), loadWorkers()]);
        }

        loadDashboard();
    </script>
</body>
</html>
`
