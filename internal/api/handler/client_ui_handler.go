package handler

import (
	"fmt"
	"net/http"
)

// ServeClientPortal renders the client registration page.
func (h *JobHandler) ServeClientRegisterPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, clientRegisterHTML)
}

// ServeClientLoginPage renders the client API-key login page.
func (h *JobHandler) ServeClientLoginPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, clientLoginHTML)
}

// ServeClientDashboard renders the full client dashboard (SPA, key stored in localStorage).
func (h *JobHandler) ServeClientDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, clientDashboardHTML)
}

// ServeLandingPage renders the public-facing landing page with inline client registration.
func (h *JobHandler) ServeLandingPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, landingHTML)
}

/* ─────────────────────────────────────────────────────────────────────────────
   PUBLIC LANDING PAGE
───────────────────────────────────────────────────────────────────────────── */
const landingHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>TaskQueue — Reliable Background Job Processing</title>
  <meta name="description" content="TaskQueue gives your application a resilient, priority-aware background job engine. Register today and start processing jobs in minutes.">
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

    /* ── Hero ── */
    .hero{
      padding:148px clamp(20px,5vw,80px) 80px;
      max-width:1200px;margin:0 auto;
      display:grid;grid-template-columns:1fr 1fr;gap:60px;align-items:center;
    }
    @media(max-width:900px){.hero{grid-template-columns:1fr;padding-top:110px}.hero-visual{display:none}}
    .hero-badge{
      display:inline-flex;align-items:center;gap:8px;
      background:rgba(99,102,241,.1);border:1px solid rgba(99,102,241,.3);
      color:var(--accent2);font-size:12px;font-weight:600;letter-spacing:.06em;text-transform:uppercase;
      padding:6px 14px;border-radius:999px;margin-bottom:24px;
    }
    .hero-badge .dot{width:6px;height:6px;border-radius:50%;background:var(--good);box-shadow:0 0 8px var(--good);animation:pulse 2s infinite}
    .hero h2{font-size:clamp(36px,5vw,58px);font-weight:900;line-height:1.1;margin-bottom:20px}
    .hero h2 .grad{background:linear-gradient(135deg,var(--accent2) 0%,var(--accent3) 50%,var(--accent4) 100%);-webkit-background-clip:text;-webkit-text-fill-color:transparent;background-clip:text}
    .hero p{font-size:17px;color:var(--muted);line-height:1.7;margin-bottom:36px;max-width:480px}
    .hero-actions{display:flex;align-items:center;gap:14px;flex-wrap:wrap}
    .btn-hero{
      padding:14px 28px;border-radius:12px;font-size:15px;font-weight:600;
      cursor:pointer;transition:all .25s;border:none;text-decoration:none;display:inline-flex;align-items:center;gap:8px;
    }
    .btn-hero.primary{background:linear-gradient(135deg,var(--accent),var(--accent3));color:#fff;box-shadow:0 8px 28px rgba(99,102,241,.4)}
    .btn-hero.primary:hover{transform:translateY(-2px);box-shadow:0 12px 36px rgba(99,102,241,.55)}
    .btn-hero.ghost{background:rgba(255,255,255,.06);color:var(--text);border:1px solid var(--border)}
    .btn-hero.ghost:hover{background:rgba(255,255,255,.1);border-color:var(--border-h)}
    .hero-note{font-size:13px;color:var(--muted2);margin-top:16px;display:flex;align-items:center;gap:6px}
    .hero-note i{color:var(--good)}

    /* ── Visual panel (right side) ── */
    .hero-visual{position:relative}
    .terminal{
      background:rgba(10,15,30,0.9);border:1px solid var(--border);border-radius:16px;
      padding:20px;font-size:13px;font-family:'SF Mono','Fira Code',monospace;
      box-shadow:0 30px 80px rgba(0,0,0,.6);backdrop-filter:blur(16px);
      animation:floatUp 3s ease-in-out infinite alternate;
    }
    @keyframes floatUp{from{transform:translateY(0)} to{transform:translateY(-8px)}}
    .term-bar{display:flex;align-items:center;gap:6px;margin-bottom:16px}
    .term-dot{width:12px;height:12px;border-radius:50%}
    .term-dot.r{background:#f87171}.term-dot.y{background:#fbbf24}.term-dot.g{background:#34d399}
    .term-title{font-size:11px;color:var(--muted2);margin-left:8px}
    .term-line{margin-bottom:6px;line-height:1.6}
    .tc-prompt{color:var(--accent2)}.tc-cmd{color:var(--text)}.tc-comment{color:var(--muted2)}
    .tc-ok{color:var(--good)}.tc-key{color:var(--accent3)}.tc-val{color:var(--warn)}
    .cursor{display:inline-block;width:8px;height:15px;background:var(--accent2);border-radius:2px;animation:blink .9s step-end infinite;vertical-align:middle}
    @keyframes blink{0%,100%{opacity:1}50%{opacity:0}}
    .stat-chips{display:flex;gap:10px;margin-top:14px;flex-wrap:wrap}
    .stat-chip{
      flex:1;min-width:90px;background:rgba(99,102,241,.08);border:1px solid rgba(99,102,241,.2);
      border-radius:12px;padding:12px;text-align:center;
    }
    .stat-chip .sc-val{font-size:22px;font-weight:800;color:var(--accent2)}
    .stat-chip .sc-lbl{font-size:11px;color:var(--muted2);margin-top:2px}

    /* ── Features ── */
    .features{max-width:1200px;margin:0 auto;padding:80px clamp(20px,5vw,80px)}
    .section-header{text-align:center;margin-bottom:56px}
    .section-tag{display:inline-block;font-size:11px;font-weight:700;letter-spacing:.1em;text-transform:uppercase;color:var(--accent2);margin-bottom:12px}
    .section-header h3{font-size:clamp(26px,4vw,40px);font-weight:800;margin-bottom:12px}
    .section-header p{font-size:16px;color:var(--muted);max-width:520px;margin:0 auto;line-height:1.7}
    .feat-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:20px}
    .feat-card{
      background:var(--surface);border:1px solid var(--border);border-radius:20px;
      padding:28px;transition:all .3s;backdrop-filter:blur(16px);
    }
    .feat-card:hover{transform:translateY(-4px);border-color:var(--border-h);box-shadow:0 20px 60px rgba(0,0,0,.3)}
    .feat-icon{
      width:48px;height:48px;border-radius:14px;display:flex;align-items:center;justify-content:center;
      font-size:20px;margin-bottom:18px;
    }
    .feat-card h4{font-size:16px;font-weight:700;margin-bottom:8px}
    .feat-card p{font-size:13.5px;color:var(--muted);line-height:1.7}

    /* ── Registration section ── */
    .register-section{
      max-width:680px;margin:0 auto;padding:80px clamp(20px,5vw,40px);
    }
    .register-card{
      background:rgba(26,34,54,0.55);border:1px solid var(--border);border-radius:28px;
      padding:48px;backdrop-filter:blur(24px);
      box-shadow:0 30px 100px rgba(0,0,0,.4),inset 0 1px 0 rgba(255,255,255,.06);
      animation:fadeIn .6s ease;
    }
    @keyframes fadeIn{from{opacity:0;transform:translateY(20px)} to{opacity:1;transform:translateY(0)}}
    .register-card .rc-head{text-align:center;margin-bottom:36px}
    .register-card .rc-icon{
      width:64px;height:64px;border-radius:20px;
      background:linear-gradient(135deg,var(--accent),var(--accent3));
      display:flex;align-items:center;justify-content:center;margin:0 auto 18px;
      box-shadow:0 12px 40px rgba(99,102,241,.4),inset 0 2px 4px rgba(255,255,255,.2);
      font-size:26px;color:#fff;
    }
    .register-card h3{font-size:24px;font-weight:800;margin-bottom:6px}
    .register-card .rc-head p{font-size:15px;color:var(--muted)}
    .form-group{margin-bottom:20px}
    .form-group label{display:block;font-size:13px;font-weight:600;color:var(--muted);margin-bottom:8px;letter-spacing:.03em}
    .form-group input{
      width:100%;background:rgba(10,15,30,0.6);border:1px solid var(--border);
      border-radius:12px;padding:13px 16px;color:var(--text);font-size:15px;font-family:inherit;
      outline:none;transition:all .25s;backdrop-filter:blur(10px);
    }
    .form-group input:focus{border-color:var(--accent2);box-shadow:0 0 0 3px rgba(99,102,241,.15)}
    .form-group input::placeholder{color:var(--muted2)}
    .alert-box{
      display:none;padding:12px 16px;border-radius:12px;font-size:13.5px;font-weight:500;
      margin-bottom:20px;
    }
    .alert-box.error{background:rgba(248,113,113,.1);border:1px solid rgba(248,113,113,.3);color:#f87171}
    .alert-box.success{background:rgba(52,211,153,.1);border:1px solid rgba(52,211,153,.3);color:#34d399}
    .btn-register{
      width:100%;padding:15px;border:none;border-radius:14px;font-size:16px;font-weight:700;
      cursor:pointer;transition:all .25s;font-family:inherit;
      background:linear-gradient(135deg,var(--accent),var(--accent3));color:#fff;
      box-shadow:0 8px 28px rgba(99,102,241,.4);
    }
    .btn-register:hover{transform:translateY(-2px);box-shadow:0 14px 40px rgba(99,102,241,.55)}
    .btn-register:disabled{opacity:.5;cursor:not-allowed;transform:none}
    .spinner-inline{display:inline-block;width:16px;height:16px;border:2px solid rgba(255,255,255,.3);border-top-color:#fff;border-radius:50%;animation:spin .6s linear infinite;vertical-align:middle;margin-right:8px}
    @keyframes spin{to{transform:rotate(360deg)}}
    .divider{border:none;border-top:1px solid var(--border);margin:24px 0}
    .login-row{text-align:center;font-size:14px;color:var(--muted)}
    .login-row a{color:var(--accent2);text-decoration:none;font-weight:600}
    .login-row a:hover{color:var(--accent3)}

    /* ── Key reveal box ── */
    .key-reveal{display:none;text-align:center;animation:fadeIn .4s ease}
    .key-reveal .kr-icon{font-size:40px;margin-bottom:12px}
    .key-reveal h4{font-size:18px;font-weight:700;margin-bottom:6px;color:var(--good)}
    .key-reveal p{font-size:13px;color:var(--muted);margin-bottom:16px}
    .key-box{
      background:rgba(10,15,30,0.8);border:1px solid rgba(52,211,153,.3);border-radius:12px;
      padding:14px 16px;display:flex;align-items:center;gap:10px;
    }
    .key-box code{flex:1;font-size:13px;font-family:'SF Mono','Fira Code',monospace;word-break:break-all;color:var(--good)}
    .key-box button{
      background:rgba(52,211,153,.1);border:1px solid rgba(52,211,153,.3);color:var(--good);
      border-radius:8px;padding:6px 12px;cursor:pointer;font-size:12px;font-weight:600;font-family:inherit;
      transition:all .2s;white-space:nowrap;
    }
    .key-box button:hover{background:rgba(52,211,153,.2)}
    .key-warning{font-size:12px;color:var(--warn);margin-top:10px;display:flex;align-items:center;gap:6px}
    .key-actions{display:flex;gap:10px;margin-top:16px}
    .key-actions a{
      flex:1;text-align:center;padding:12px;border-radius:12px;font-size:14px;font-weight:600;
      text-decoration:none;transition:all .2s;
    }
    .key-actions .ka-dash{background:linear-gradient(135deg,var(--accent),var(--accent3));color:#fff}
    .key-actions .ka-docs{background:rgba(255,255,255,.06);color:var(--muted);border:1px solid var(--border)}
    .key-actions .ka-docs:hover{background:rgba(255,255,255,.1)}

    /* ── Footer ── */
    footer{
      border-top:1px solid var(--border);padding:40px clamp(20px,5vw,80px);
      display:flex;align-items:center;justify-content:space-between;flex-wrap:wrap;gap:16px;
      max-width:100%;
    }
    footer p{font-size:13px;color:var(--muted2)}
    footer a{color:var(--muted2);text-decoration:none;font-size:13px}
    footer a:hover{color:var(--muted)}
    .footer-links{display:flex;gap:20px}

    @keyframes pulse{0%,100%{opacity:1;box-shadow:0 0 8px var(--good)} 50%{opacity:.6;box-shadow:none}}
  </style>
</head>
<body>

<!-- Navbar -->
<nav>
  <a class="nav-brand" href="/">
    <div class="nav-logo"><i class="fas fa-bolt"></i></div>
    <h1>TaskQueue</h1>
  </a>
  <div class="nav-links">
    <a href="#features">Features</a>
    <a href="#register">Docs</a>
    <a href="/client/login">Sign In</a>
    <a href="#register" class="nav-cta">Get Started</a>
  </div>
</nav>

<!-- Hero -->
<section class="hero">
  <div class="hero-copy">
    <div class="hero-badge"><span class="dot"></span>Production Ready</div>
    <h2>Background jobs that<br><span class="grad">never drop the ball</span></h2>
    <p>A resilient, priority-aware job queue with retries, circuit breakers, dead-letter queues, and real-time monitoring — built for services that can't afford to miss a beat.</p>
    <div class="hero-actions">
      <a href="#register" class="btn-hero primary"><i class="fas fa-rocket"></i> Get Your API Key</a>
      <a href="/swagger/" class="btn-hero ghost"><i class="fas fa-book"></i> API Docs</a>
    </div>
    <div class="hero-note"><i class="fas fa-check-circle"></i> Free to register &mdash; no credit card required</div>
  </div>
  <div class="hero-visual">
    <div class="terminal">
      <div class="term-bar">
        <div class="term-dot r"></div><div class="term-dot y"></div><div class="term-dot g"></div>
        <span class="term-title">terminal</span>
      </div>
      <div class="term-line"><span class="tc-prompt">$</span> <span class="tc-cmd">curl -X POST /api/v1/register \</span></div>
      <div class="term-line">&nbsp;&nbsp;&nbsp;&nbsp;<span class="tc-cmd">-d '{"tenant_id":"acme-corp"}'</span></div>
      <div class="term-line tc-comment"># Response:</div>
      <div class="term-line"><span class="tc-ok">{</span></div>
      <div class="term-line">&nbsp;&nbsp;<span class="tc-key">"api_key"</span><span class="tc-ok">:</span> <span class="tc-val">"tq_live_a8f2..."</span>,</div>
      <div class="term-line">&nbsp;&nbsp;<span class="tc-key">"tenant_id"</span><span class="tc-ok">:</span> <span class="tc-val">"acme-corp"</span></div>
      <div class="term-line"><span class="tc-ok">}</span></div>
      <div class="term-line" style="margin-top:12px"><span class="tc-prompt">$</span> <span class="tc-cmd">curl /jobs -H "X-API-Key: tq_live_a8f2..."</span> <span class="cursor"></span></div>
      <div class="stat-chips">
        <div class="stat-chip"><div class="sc-val">99.9%</div><div class="sc-lbl">Uptime</div></div>
        <div class="stat-chip"><div class="sc-val">&lt;5ms</div><div class="sc-lbl">Enqueue</div></div>
        <div class="stat-chip"><div class="sc-val">∞</div><div class="sc-lbl">Scale</div></div>
      </div>
    </div>
  </div>
</section>

<!-- Features -->
<section class="features" id="features">
  <div class="section-header">
    <div class="section-tag">Why TaskQueue</div>
    <h3>Everything you need, nothing you don't</h3>
    <p>Ship background jobs with confidence. TaskQueue handles the hard parts so you can focus on your product.</p>
  </div>
  <div class="feat-grid">
    <div class="feat-card">
      <div class="feat-icon" style="background:rgba(99,102,241,.15);color:var(--accent2)"><i class="fas fa-layer-group"></i></div>
      <h4>Priority Queues</h4>
      <p>Three-tier priority system (high / medium / low) ensures critical jobs are always processed first, even under heavy load.</p>
    </div>
    <div class="feat-card">
      <div class="feat-icon" style="background:rgba(52,211,153,.12);color:var(--good)"><i class="fas fa-redo"></i></div>
      <h4>Automatic Retries</h4>
      <p>Configurable retry counts with exponential back-off. Jobs that keep failing land in the Dead Letter Queue for manual review.</p>
    </div>
    <div class="feat-card">
      <div class="feat-icon" style="background:rgba(192,132,252,.12);color:var(--accent3)"><i class="fas fa-shield-alt"></i></div>
      <h4>Circuit Breakers</h4>
      <p>Per-plugin circuit breakers automatically trip when a downstream service degrades, protecting your system from cascading failures.</p>
    </div>
    <div class="feat-card">
      <div class="feat-icon" style="background:rgba(56,189,248,.1);color:var(--accent4)"><i class="fas fa-broadcast-tower"></i></div>
      <h4>Real-time SSE</h4>
      <p>Live Server-Sent Events stream job lifecycle events directly to your dashboard — no polling, no delay.</p>
    </div>
    <div class="feat-card">
      <div class="feat-icon" style="background:rgba(251,191,36,.1);color:var(--warn)"><i class="fas fa-project-diagram"></i></div>
      <h4>DAG Dependencies</h4>
      <p>Define job dependency graphs. TaskQueue resolves the execution order and won't start a job until all its dependencies succeed.</p>
    </div>
    <div class="feat-card">
      <div class="feat-icon" style="background:rgba(248,113,113,.1);color:var(--bad)"><i class="fas fa-globe"></i></div>
      <h4>Webhook Callbacks</h4>
      <p>Register HTTP callbacks for any job event. Your service gets notified the instant a job completes, fails, or is cancelled.</p>
    </div>
  </div>
</section>

<!-- Registration -->
<section class="register-section" id="register">
  <div class="register-card" id="register-form-wrap">
    <div class="rc-head">
      <div class="rc-icon"><i class="fas fa-key"></i></div>
      <h3>Create your free account</h3>
      <p>Pick a unique tenant ID and you'll receive an API key instantly.</p>
    </div>

    <div id="reg-alert" class="alert-box"></div>

    <div class="form-group">
      <label for="reg-tenant">Tenant ID</label>
      <input id="reg-tenant" type="text" placeholder="e.g. acme-corp" autocomplete="off" maxlength="64" spellcheck="false">
    </div>
    <div class="form-group">
      <label for="reg-service">Service Name <span style="color:var(--muted2);font-weight:400">(optional)</span></label>
      <input id="reg-service" type="text" placeholder="e.g. payment-worker" autocomplete="off">
    </div>

    <button class="btn-register" id="reg-btn" onclick="doRegister()">
      <span id="reg-label"><i class="fas fa-rocket"></i> &nbsp;Create Account & Get API Key</span>
    </button>

    <hr class="divider">
    <div class="login-row">Already have a key? <a href="/client/login">Sign in to your dashboard</a></div>
  </div>

  <!-- Key reveal (shown after success) -->
  <div class="register-card key-reveal" id="key-reveal">
    <div class="kr-icon">🎉</div>
    <h4>You're all set!</h4>
    <p>Here is your API key. <strong>Copy it now</strong> — it won't be shown again.</p>
    <div class="key-box">
      <code id="key-display"></code>
      <button onclick="copyKey()"><i class="fas fa-copy"></i> Copy</button>
    </div>
    <div class="key-warning"><i class="fas fa-exclamation-triangle"></i> Store this key securely. It cannot be recovered.</div>
    <div class="key-actions">
      <a href="/client/dashboard" class="ka-dash"><i class="fas fa-gauge-high"></i> Open Dashboard</a>
      <a href="/swagger/" class="ka-docs"><i class="fas fa-book"></i> API Reference</a>
    </div>
  </div>
</section>

<!-- Footer -->
<footer>
  <p>© 2025 TaskQueue. Built for developers who ship.</p>
  <div class="footer-links">
    <a href="/swagger/">API Reference</a>
    <a href="/client/login">Sign In</a>
    <a href="/ui">Operator Panel</a>
  </div>
</footer>

<script>
  let generatedKey = '';

  async function doRegister() {
    const tenant = document.getElementById('reg-tenant').value.trim();
    const alert  = document.getElementById('reg-alert');
    const btn    = document.getElementById('reg-btn');
    const label  = document.getElementById('reg-label');

    // Validate
    if (!tenant) { showAlert(alert, 'error', 'Please enter a Tenant ID.'); return; }
    if (!/^[a-z0-9][a-z0-9\-]{1,62}[a-z0-9]$/i.test(tenant)) {
      showAlert(alert, 'error', 'Tenant ID must be 3–64 chars: letters, numbers, hyphens only.'); return;
    }

    btn.disabled = true;
    label.innerHTML = '<span class="spinner-inline"></span> Creating your account…';
    alert.style.display = 'none';

    try {
      const body = { tenant_id: tenant };
      const svc = document.getElementById('reg-service').value.trim();
      if (svc) body.service_name = svc;

      const res = await fetch('/api/v1/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      });

      const data = await res.json().catch(() => ({}));

      if (!res.ok) {
        const msg = data.error || data.message || ('Registration failed (HTTP ' + res.status + ')');
        showAlert(alert, 'error', msg);
        btn.disabled = false;
        label.innerHTML = '<i class="fas fa-rocket"></i> &nbsp;Create Account & Get API Key';
        return;
      }

      // Success
      generatedKey = data.api_key || '';
      document.getElementById('key-display').textContent = generatedKey;
      // Store key so dashboard auto-logs in
      if (generatedKey) localStorage.setItem('tq_api_key', generatedKey);

      document.getElementById('register-form-wrap').style.display = 'none';
      const reveal = document.getElementById('key-reveal');
      reveal.style.display = 'block';

    } catch(e) {
      showAlert(alert, 'error', 'Network error — check your connection and try again.');
      btn.disabled = false;
      label.innerHTML = '<i class="fas fa-rocket"></i> &nbsp;Create Account & Get API Key';
    }
  }

  function copyKey() {
    if (!generatedKey) return;
    navigator.clipboard.writeText(generatedKey).then(() => {
      const btn = document.querySelector('.key-box button');
      btn.innerHTML = '<i class="fas fa-check"></i> Copied!';
      setTimeout(() => { btn.innerHTML = '<i class="fas fa-copy"></i> Copy'; }, 2000);
    });
  }

  function showAlert(el, type, msg) {
    el.className = 'alert-box ' + type;
    el.innerHTML = '<i class="fas fa-' + (type === 'error' ? 'circle-exclamation' : 'circle-check') + '"></i> ' + msg;
    el.style.display = 'block';
  }

  // Smooth scroll for anchor links
  document.querySelectorAll('a[href^="#"]').forEach(a => {
    a.addEventListener('click', e => {
      const target = document.querySelector(a.getAttribute('href'));
      if (target) { e.preventDefault(); target.scrollIntoView({ behavior: 'smooth' }); }
    });
  });

  // Enter key submits form
  document.addEventListener('keydown', e => {
    if (e.key === 'Enter' && document.getElementById('register-form-wrap').style.display !== 'none') {
      doRegister();
    }
  });
</script>
</body>
</html>`

/* ─────────────────────────────────────────────────────────────────────────────
   Shared CSS variables — identical to admin UI palette for consistency
───────────────────────────────────────────────────────────────────────────── */
const clientSharedCSS = `
  *{margin:0;padding:0;box-sizing:border-box}
  :root{
    --bg:#070a14;--bg2:#0f1629;--surface:rgba(26,34,54,0.65);--surface2:rgba(31,42,64,0.8);
    --border:rgba(255,255,255,0.08);--border-hover:rgba(255,255,255,0.15);
    --text:#f8fafc;--muted:#94a3b8;--muted2:#64748b;
    --accent:#6366f1;--accent2:#818cf8;--accent3:#c084fc;
    --good:#34d399;--bad:#f87171;--warn:#fbbf24;--info:#93c5fd;
    --good-bg:rgba(52,211,153,.12);--bad-bg:rgba(248,113,113,.12);
    --warn-bg:rgba(251,191,36,.12);--info-bg:rgba(147,197,253,.1);
    --good-border:rgba(52,211,153,.35);--bad-border:rgba(248,113,113,.4);
    --warn-border:rgba(251,191,36,.35);--info-border:rgba(147,197,253,.3);
  }
  body{font-family:'Outfit','Inter',-apple-system,sans-serif;background:var(--bg);color:var(--text);min-height:100vh}
  ::-webkit-scrollbar{width:6px}
  ::-webkit-scrollbar-thumb{background:var(--border);border-radius:3px}
  input,select,textarea{
    width:100%;background:rgba(15,22,41,0.6);border:1px solid var(--border);
    border-radius:12px;padding:12px 16px;color:var(--text);font-size:14px;
    outline:none;transition:all .3s cubic-bezier(0.4, 0, 0.2, 1);font-family:inherit;
    backdrop-filter:blur(10px);
  }
  input:hover,select:hover,textarea:hover{border-color:var(--border-hover)}
  input:focus,select:focus,textarea:focus{border-color:var(--accent);box-shadow:0 0 0 4px rgba(99,102,241,.15);background:rgba(15,22,41,0.8)}
  textarea{min-height:80px;resize:vertical}
  button,.btn{
    padding:12px 24px;border-radius:12px;font-size:14px;font-weight:600;
    cursor:pointer;transition:all .2s cubic-bezier(0.4, 0, 0.2, 1);border:none;font-family:inherit;
    position:relative;overflow:hidden;
  }
  .btn-primary{background:linear-gradient(135deg,var(--accent),var(--accent3));color:#fff;box-shadow:0 4px 14px rgba(99,102,241,.3), inset 0 1px 0 rgba(255,255,255,0.2)}
  .btn-primary:hover{box-shadow:0 6px 20px rgba(99,102,241,.4), inset 0 1px 0 rgba(255,255,255,0.3);transform:translateY(-1px)}
  .btn-primary:active{transform:translateY(1px)}
  .btn-secondary{background:var(--surface2);border:1px solid var(--border);color:var(--text);backdrop-filter:blur(10px)}
  .btn-secondary:hover{background:rgba(255,255,255,0.05);border-color:var(--border-hover);transform:translateY(-1px)}
  .btn-danger{background:linear-gradient(135deg,#dc2626,#ef4444);color:#fff;box-shadow:0 4px 14px rgba(220,38,38,.3)}
  .btn-danger:hover{transform:translateY(-1px);box-shadow:0 6px 20px rgba(220,38,38,.4)}
  .btn-sm{padding:8px 16px;font-size:12px;border-radius:8px}
  .btn-full{width:100%}
  .pill{display:inline-block;padding:4px 12px;border-radius:999px;font-size:11px;font-weight:600;backdrop-filter:blur(4px)}
  .pill.good{background:var(--good-bg);color:var(--good);border:1px solid var(--good-border)}
  .pill.bad{background:var(--bad-bg);color:var(--bad);border:1px solid var(--bad-border)}
  .pill.warn{background:var(--warn-bg);color:var(--warn);border:1px solid var(--warn-border)}
  .pill.info{background:var(--info-bg);color:var(--info);border:1px solid var(--info-border)}
  .section-card{
    background:var(--surface);border:1px solid var(--border);border-radius:20px;
    padding:24px;margin-bottom:20px;backdrop-filter:blur(16px);
    box-shadow:0 4px 24px rgba(0,0,0,0.1);transition:transform 0.3s ease, box-shadow 0.3s ease;
  }
  .section-card:hover{transform:translateY(-2px);box-shadow:0 8px 32px rgba(0,0,0,0.2);border-color:var(--border-hover)}
  .section-title{font-size:12px;font-weight:700;color:var(--accent2);text-transform:uppercase;letter-spacing:.12em;margin-bottom:20px;display:flex;align-items:center;gap:8px}
  .form-group{margin-bottom:16px}
  .form-group label{display:block;font-size:13px;color:var(--muted);margin-bottom:8px;font-weight:500}
  .form-row{display:grid;grid-template-columns:1fr 1fr;gap:16px}
  table{width:100%;border-collapse:separate;border-spacing:0;font-size:14px}
  th,td{text-align:left;padding:12px 16px;border-bottom:1px solid var(--border)}
  th{color:var(--muted2);font-size:12px;text-transform:uppercase;letter-spacing:.08em;font-weight:600;background:rgba(0,0,0,0.2)}
  th:first-child{border-top-left-radius:12px}th:last-child{border-top-right-radius:12px}
  tr:last-child td:first-child{border-bottom-left-radius:12px}tr:last-child td:last-child{border-bottom-right-radius:12px}
  tr:last-child td{border-bottom:none}
  tr{transition:background-color 0.2s ease}
  tr:hover td{background:rgba(255,255,255,.03)}
  pre,.code-block{
    background:rgba(10,15,30,0.8);border:1px solid var(--border);border-radius:12px;
    padding:16px;font-size:13px;line-height:1.6;overflow:auto;
    font-family:'Fira Code',monospace;white-space:pre-wrap;word-break:break-all;
    box-shadow:inset 0 2px 8px rgba(0,0,0,0.2);
  }
  .toast{
    position:fixed;bottom:24px;right:24px;padding:16px 24px;border-radius:16px;
    font-size:14px;font-weight:500;z-index:9999;max-width:400px;
    animation:slideUp .4s cubic-bezier(0.175, 0.885, 0.32, 1.275);display:flex;align-items:center;gap:12px;
    box-shadow:0 12px 40px rgba(0,0,0,.4);backdrop-filter:blur(12px);
  }
  .toast.success{background:rgba(6,78,59,0.85);border:1px solid var(--good-border);color:var(--good)}
  .toast.error{background:rgba(69,10,10,0.85);border:1px solid var(--bad-border);color:var(--bad)}
  .toast.info{background:rgba(12,26,58,0.85);border:1px solid var(--info-border);color:var(--info)}
  @keyframes slideUp{from{opacity:0;transform:translateY(30px) scale(0.95)}to{opacity:1;transform:translateY(0) scale(1)}}
  .spinner{display:inline-block;width:20px;height:20px;border:2.5px solid rgba(255,255,255,.2);border-top-color:#fff;border-radius:50%;animation:spin .8s cubic-bezier(0.4, 0, 0.2, 1) infinite}
  @keyframes spin{to{transform:rotate(360deg)}}
  .hidden{display:none!important}
  .loading-overlay{
    position:absolute;inset:0;background:rgba(7,10,20,.6);display:flex;
    align-items:center;justify-content:center;border-radius:20px;z-index:10;
    backdrop-filter:blur(4px)
  }
  .alert{padding:14px 18px;border-radius:12px;font-size:14px;margin-bottom:16px;display:flex;align-items:flex-start;gap:12px;backdrop-filter:blur(8px)}
  .alert.success{background:var(--good-bg);border:1px solid var(--good-border);color:var(--good)}
  .alert.error{background:var(--bad-bg);border:1px solid var(--bad-border);color:var(--bad)}
  .alert.warning{background:var(--warn-bg);border:1px solid var(--warn-border);color:var(--warn)}
  .alert.info{background:var(--info-bg);border:1px solid var(--info-border);color:var(--info)}
`

/* ─────────────────────────────────────────────────────────────────────────────
   REGISTER PAGE
───────────────────────────────────────────────────────────────────────────── */
const clientRegisterHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Get API Key — Task Queue</title>
  <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@400;500;600;700;800&family=Inter:wght@400;500;600&display=swap" rel="stylesheet">
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css">
  <style>
    ` + clientSharedCSS + `
    body{display:flex;align-items:center;justify-content:center;min-height:100vh;padding:20px;overflow:hidden;}
    .auth-bg{
      position:fixed;inset:0;z-index:0;
      background:radial-gradient(ellipse 80% 80% at 50% -20%,rgba(99,102,241,.25),transparent),
                 radial-gradient(ellipse 60% 60% at 80% 90%,rgba(192,132,252,.15),transparent),
                 radial-gradient(ellipse 50% 50% at 20% 80%,rgba(52,211,153,.1),transparent);
      filter:blur(40px);
    }
    .card{
      position:relative;z-index:1;width:100%;max-width:500px;
      background:var(--surface);border:1px solid var(--border);border-radius:28px;
      padding:48px;box-shadow:0 32px 80px rgba(0,0,0,.6);backdrop-filter:blur(24px);
      animation:slideUp .6s cubic-bezier(0.16, 1, 0.3, 1);
    }
    .brand{display:flex;align-items:center;gap:16px;margin-bottom:36px}
    .brand-icon{
      width:52px;height:52px;background:linear-gradient(135deg,var(--accent),var(--accent3));
      border-radius:16px;display:flex;align-items:center;justify-content:center;
      box-shadow:0 12px 28px rgba(99,102,241,.4), inset 0 2px 4px rgba(255,255,255,.3)
    }
    .brand-icon i{color:#fff;font-size:24px}
    .brand h1{font-size:24px;font-weight:800;letter-spacing:-0.5px}
    .brand p{font-size:13px;color:var(--accent2);margin-top:2px;font-weight:500}
    h2{font-size:26px;font-weight:800;margin-bottom:8px;letter-spacing:-0.5px}
    .subtitle{font-size:14px;color:var(--muted);margin-bottom:28px;line-height:1.5}
    .steps{display:flex;gap:8px;margin-bottom:28px}
    .step{flex:1;height:3px;border-radius:2px;background:var(--border)}
    .step.active{background:var(--accent)}
    .step.done{background:var(--good)}
    .divider{height:1px;background:var(--border);margin:24px 0}
    .link-row{text-align:center;font-size:13px;color:var(--muted)}
    .link-row a{color:var(--accent2);text-decoration:none;font-weight:500}
    .link-row a:hover{text-decoration:underline}
    .key-reveal{
      background:var(--bg2);border:1px solid var(--good-border);border-radius:12px;
      padding:20px;margin-bottom:20px;position:relative
    }
    .key-reveal .key-label{font-size:11px;color:var(--muted2);text-transform:uppercase;letter-spacing:.1em;font-weight:600;margin-bottom:8px}
    .key-reveal .key-value{
      font-family:'SF Mono','Fira Code',monospace;font-size:13px;color:var(--good);
      word-break:break-all;line-height:1.6
    }
    .key-reveal .copy-btn{
      position:absolute;top:12px;right:12px;background:var(--surface);
      border:1px solid var(--border);border-radius:8px;padding:5px 10px;
      font-size:11px;color:var(--muted);cursor:pointer;transition:all .15s
    }
    .key-reveal .copy-btn:hover{border-color:var(--accent);color:var(--accent2)}
    .warning-box{
      background:var(--warn-bg);border:1px solid var(--warn-border);
      border-radius:10px;padding:12px 16px;font-size:12px;color:var(--warn);
      display:flex;gap:10px;align-items:flex-start;margin-bottom:20px
    }
    #step1,#step2{transition:opacity .3s}
  </style>
</head>
<body>
<div class="auth-bg"></div>
<div class="card">
  <div class="brand">
    <div class="brand-icon"><i class="fas fa-layer-group"></i></div>
    <div>
      <h1>Task Queue</h1>
      <p>Client Portal</p>
    </div>
  </div>
  <div class="steps">
    <div class="step active" id="s1"></div>
    <div class="step" id="s2"></div>
  </div>

  <!-- Step 1: Register -->
  <div id="step1">
    <h2>Create your account</h2>
    <p class="subtitle">Register your service to receive a unique API key. Each service name gets its own isolated tenant.</p>
    <div id="reg-alert" class="alert error hidden"></div>
    <div class="form-group">
      <label for="tenant-id">Service / Tenant Name</label>
      <input id="tenant-id" type="text" placeholder="e.g. my-backend-service" autocomplete="off">
    </div>
    <button class="btn btn-primary btn-full" id="reg-btn" onclick="doRegister()">
      <span id="reg-label">Generate API Key</span>
    </button>
  </div>

  <!-- Step 2: Show Key -->
  <div id="step2" class="hidden">
    <h2>Your API Key</h2>
    <p class="subtitle">Copy and store your key securely. It will <strong>never be shown again</strong>.</p>
    <div class="key-reveal">
      <div class="key-label">API Key</div>
      <div class="key-value" id="key-display"></div>
      <button class="copy-btn" onclick="copyKey()"><i class="fas fa-copy"></i> Copy</button>
    </div>
    <div class="warning-box">
      <i class="fas fa-triangle-exclamation" style="margin-top:1px;flex-shrink:0"></i>
      <span>This key is shown only once. Save it in your environment variables or a secrets manager before proceeding.</span>
    </div>
    <div class="alert success">
      <i class="fas fa-circle-check"></i>
      <span>Tenant <strong id="tenant-display"></strong> registered successfully.</span>
    </div>
    <button class="btn btn-primary btn-full" onclick="goToDashboard()">
      <i class="fas fa-gauge"></i> Go to Dashboard
    </button>
  </div>

  <div class="divider"></div>
  <div class="link-row">Already have a key? <a href="/client/login">Sign in</a></div>
</div>

<script>
  let generatedKey = '';

  async function doRegister() {
    const tid = document.getElementById('tenant-id').value.trim();
    const alert = document.getElementById('reg-alert');
    if (!tid) { showAlert(alert, 'Service name is required.'); return; }
    if (!/^[a-z0-9_-]+$/i.test(tid)) { showAlert(alert, 'Use only letters, numbers, hyphens, and underscores.'); return; }

    const btn = document.getElementById('reg-btn');
    const label = document.getElementById('reg-label');
    btn.disabled = true;
    label.innerHTML = '<span class="spinner"></span> Registering...';
    alert.classList.add('hidden');

    try {
      const res = await fetch('/api/v1/register', {
        method: 'POST',
        headers: {'Content-Type':'application/json'},
        body: JSON.stringify({tenant_id: tid})
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Registration failed');
      generatedKey = data.api_key;
      document.getElementById('key-display').textContent = data.api_key;
      document.getElementById('tenant-display').textContent = data.tenant_id;
      document.getElementById('s1').classList.add('done');
      document.getElementById('s2').classList.add('active');
      document.getElementById('step1').classList.add('hidden');
      document.getElementById('step2').classList.remove('hidden');
    } catch(e) {
      showAlert(alert, e.message);
      btn.disabled = false;
      label.textContent = 'Generate API Key';
    }
  }

  function copyKey() {
    navigator.clipboard.writeText(generatedKey).then(() => showToast('API key copied!', 'success'));
  }

  function goToDashboard() {
    localStorage.setItem('tq_api_key', generatedKey);
    window.location.href = '/client/dashboard';
  }

  function showAlert(el, msg) {
    el.innerHTML = '<i class="fas fa-circle-exclamation"></i> ' + msg;
    el.classList.remove('hidden');
  }

  function showToast(msg, type='info') {
    const t = document.createElement('div');
    t.className = 'toast ' + type;
    t.innerHTML = '<i class="fas fa-' + (type==='success'?'check-circle':'info-circle') + '"></i>' + msg;
    document.body.appendChild(t);
    setTimeout(() => t.remove(), 3000);
  }

  document.getElementById('tenant-id').addEventListener('keydown', e => { if(e.key==='Enter') doRegister(); });
</script>
</body>
</html>`

/* ─────────────────────────────────────────────────────────────────────────────
   LOGIN PAGE
───────────────────────────────────────────────────────────────────────────── */
const clientLoginHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Client Login — Task Queue</title>
  <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@400;500;600;700;800&family=Inter:wght@400;500;600&display=swap" rel="stylesheet">
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css">
  <style>
    ` + clientSharedCSS + `
    body{display:flex;align-items:center;justify-content:center;min-height:100vh;padding:20px;overflow:hidden;}
    .auth-bg{
      position:fixed;inset:0;z-index:0;
      background:radial-gradient(ellipse 80% 80% at 50% -20%,rgba(99,102,241,.25),transparent),
                 radial-gradient(ellipse 60% 60% at 80% 90%,rgba(192,132,252,.15),transparent),
                 radial-gradient(ellipse 50% 50% at 20% 80%,rgba(52,211,153,.1),transparent);
      filter:blur(40px);
    }
    .card{
      position:relative;z-index:1;width:100%;max-width:500px;
      background:var(--surface);border:1px solid var(--border);border-radius:28px;
      padding:48px;box-shadow:0 32px 80px rgba(0,0,0,.6);backdrop-filter:blur(24px);
      animation:slideUp .6s cubic-bezier(0.16, 1, 0.3, 1);
    }
    .brand{display:flex;align-items:center;gap:16px;margin-bottom:36px}
    .brand-icon{
      width:52px;height:52px;background:linear-gradient(135deg,var(--accent),var(--accent3));
      border-radius:16px;display:flex;align-items:center;justify-content:center;
      box-shadow:0 12px 28px rgba(99,102,241,.4), inset 0 2px 4px rgba(255,255,255,.3)
    }
    .brand-icon i{color:#fff;font-size:24px}
    .brand h1{font-size:24px;font-weight:800;letter-spacing:-0.5px}
    .brand p{font-size:13px;color:var(--accent2);margin-top:2px;font-weight:500}
    h2{font-size:26px;font-weight:800;margin-bottom:8px;letter-spacing:-0.5px}
    .subtitle{font-size:14px;color:var(--muted);margin-bottom:28px}
    .divider{height:1px;background:var(--border);margin:24px 0}
    .link-row{text-align:center;font-size:14px;color:var(--muted)}
    .link-row a{color:var(--accent2);text-decoration:none;font-weight:600;transition:color 0.2s;}
    .link-row a:hover{text-decoration:underline;color:var(--accent3)}
    .input-wrap{position:relative}
    .input-wrap input{padding-right:44px}
    .input-wrap button{
      position:absolute;right:12px;top:50%;transform:translateY(-50%);
      background:none;border:none;color:var(--muted);cursor:pointer;padding:4px;font-size:14px
    }
    .help-text{font-size:11px;color:var(--muted2);margin-top:5px;line-height:1.4}
  </style>
</head>
<body>
<div class="auth-bg"></div>
<div class="card">
  <div class="brand">
    <div class="brand-icon"><i class="fas fa-layer-group"></i></div>
    <div>
      <h1>Task Queue</h1>
      <p>Client Portal</p>
    </div>
  </div>

  <h2>Sign in</h2>
  <p class="subtitle">Enter your API key to access your dashboard.</p>

  <div id="login-alert" class="alert error hidden"></div>

  <div class="form-group">
    <label for="api-key">API Key</label>
    <div class="input-wrap">
      <input id="api-key" type="password" placeholder="tq_live_..." autocomplete="off">
      <button type="button" onclick="toggleVis()" id="vis-btn"><i id="vis-icon" class="fas fa-eye"></i></button>
    </div>
    <p class="help-text">Your key starts with <code style="color:var(--accent2)">tq_live_</code>. Keep it secret.</p>
  </div>

  <button class="btn btn-primary btn-full" id="login-btn" onclick="doLogin()">
    <span id="login-label"><i class="fas fa-arrow-right-to-bracket"></i> Access Dashboard</span>
  </button>

  <div class="divider"></div>
  <div class="link-row">No key yet? <a href="/client/register">Register your service</a></div>
</div>

<script>
  function toggleVis() {
    const inp = document.getElementById('api-key');
    const icon = document.getElementById('vis-icon');
    if (inp.type === 'password') { inp.type = 'text'; icon.className = 'fas fa-eye-slash'; }
    else { inp.type = 'password'; icon.className = 'fas fa-eye'; }
  }

  async function doLogin() {
    const key = document.getElementById('api-key').value.trim();
    const alert = document.getElementById('login-alert');
    if (!key) { showAlert(alert, 'Please enter your API key.'); return; }

    const btn = document.getElementById('login-btn');
    const label = document.getElementById('login-label');
    btn.disabled = true;
    label.innerHTML = '<span class="spinner"></span> Verifying...';
    alert.classList.add('hidden');

    try {
      // Verify key is valid by calling a protected endpoint
      const res = await fetch('/api/v1/stats', {
        headers: {'X-API-Key': key}
      });
      if (!res.ok) throw new Error('Invalid API key. Please check and try again.');
      // Key is valid — store and go
      localStorage.setItem('tq_api_key', key);
      window.location.href = '/client/dashboard';
    } catch(e) {
      showAlert(alert, e.message);
      btn.disabled = false;
      label.innerHTML = '<i class="fas fa-arrow-right-to-bracket"></i> Access Dashboard';
    }
  }

  function showAlert(el, msg) {
    el.innerHTML = '<i class="fas fa-circle-exclamation"></i> ' + msg;
    el.classList.remove('hidden');
  }

  // Auto-redirect if key already stored
  const stored = localStorage.getItem('tq_api_key');
  if (stored) window.location.href = '/client/dashboard';

  document.getElementById('api-key').addEventListener('keydown', e => { if(e.key==='Enter') doLogin(); });
</script>
</body>
</html>`

/* ─────────────────────────────────────────────────────────────────────────────
   DASHBOARD (SPA)
───────────────────────────────────────────────────────────────────────────── */
const clientDashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Client Dashboard — Task Queue</title>
  <meta name="description" content="Monitor and manage your background jobs in the Task Queue system.">
  <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@400;500;600;700;800&family=Inter:wght@400;500;600&display=swap" rel="stylesheet">
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.5.1/css/all.min.css">
  <style>
    ` + clientSharedCSS + `
    :root { --sidebar-w:260px; --header-h:70px; }

    /* ── Layout ── */
    .layout{display:flex;min-height:100vh;overflow-x:hidden;}
    #sidebar{
      width:var(--sidebar-w);background:rgba(21,27,43,0.85);border-right:1px solid var(--border);
      position:fixed;top:0;left:0;height:100vh;z-index:100;
      display:flex;flex-direction:column;transition:transform .3s cubic-bezier(0.4, 0, 0.2, 1);
      backdrop-filter:blur(20px);
    }
    #sidebar.closed{transform:translateX(-100%)}
    .sidebar-brand{
      padding:20px 20px 16px;border-bottom:1px solid var(--border);
      display:flex;align-items:center;gap:12px
    }
    .sidebar-brand .brand-icon{
      width:40px;height:40px;background:linear-gradient(135deg,var(--accent),var(--accent3));
      border-radius:12px;display:flex;align-items:center;justify-content:center;
      box-shadow:0 4px 12px rgba(99,102,241,.3)
    }
    .sidebar-brand .brand-icon i{color:#fff;font-size:18px}
    .sidebar-brand h2{font-size:15px;font-weight:700}
    .sidebar-brand span{font-size:11px;color:var(--accent2);font-weight:500}
    .sidebar-nav{flex:1;overflow-y:auto;padding:12px 10px}
    .nav-label{font-size:10px;color:var(--muted2);text-transform:uppercase;letter-spacing:.1em;padding:16px 12px 6px;font-weight:600}
    .nav-item{
      display:flex;align-items:center;gap:12px;padding:10px 14px;
      border-radius:10px;cursor:pointer;transition:all .15s;
      font-size:13px;font-weight:500;color:var(--muted);margin-bottom:2px;
      text-decoration:none;border:none;background:none;width:100%;text-align:left
    }
    .nav-item i{width:18px;text-align:center;font-size:14px}
    .nav-item:hover{background:var(--surface2);color:var(--text)}
    .nav-item.active{background:rgba(99,102,241,.12);color:var(--accent2)}
    .sidebar-footer{
      padding:14px 16px;border-top:1px solid var(--border);
      display:flex;align-items:center;gap:10px
    }
    .sidebar-footer .avatar{
      width:32px;height:32px;border-radius:10px;background:linear-gradient(135deg,var(--accent),var(--accent3));
      display:flex;align-items:center;justify-content:center;font-size:13px;font-weight:700;color:#fff;flex-shrink:0
    }
    .sidebar-footer .user-info{flex:1;min-width:0}
    .sidebar-footer .user-name{font-size:12px;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
    .sidebar-footer .user-role{font-size:10px;color:var(--muted2)}
    .sidebar-footer .logout-btn{
      background:none;border:1px solid var(--border);border-radius:8px;
      padding:5px 10px;color:var(--muted);font-size:11px;cursor:pointer;transition:all .15s;white-space:nowrap
    }
    .sidebar-footer .logout-btn:hover{border-color:var(--bad);color:var(--bad)}

    /* ── Main ── */
    #main{margin-left:var(--sidebar-w);flex:1;transition:margin-left .3s;min-height:100vh}
    #main.expanded{margin-left:0}
    #topbar{
      height:var(--header-h);background:rgba(26,34,54,.9);border-bottom:1px solid var(--border);
      display:flex;align-items:center;justify-content:space-between;
      padding:0 24px;position:sticky;top:0;z-index:50;backdrop-filter:blur(12px)
    }
    #topbar .left{display:flex;align-items:center;gap:16px}
    #topbar .menu-btn{background:none;border:none;color:var(--muted);font-size:18px;cursor:pointer;padding:6px;border-radius:8px;display:none}
    #topbar .menu-btn:hover{background:var(--surface2)}
    .page-title h3{font-size:16px;font-weight:600}
    .page-title p{font-size:12px;color:var(--muted)}
    #topbar .right{display:flex;align-items:center;gap:10px}
    .live-badge{
      display:flex;align-items:center;gap:6px;padding:6px 12px;
      background:var(--good-bg);border:1px solid var(--good-border);
      border-radius:8px;font-size:11px;font-weight:600;color:var(--good)
    }
    .live-dot{width:7px;height:7px;border-radius:50%;background:var(--good);animation:pulse 1.8s infinite}
    @keyframes pulse{0%,100%{opacity:1}50%{opacity:.4}}
    .dead-badge{
      display:flex;align-items:center;gap:6px;padding:6px 12px;
      background:var(--bad-bg);border:1px solid var(--bad-border);
      border-radius:8px;font-size:11px;font-weight:600;color:var(--bad)
    }

    #content{padding:24px;max-width:1360px;margin:0 auto}
    .page{display:none;animation:fadeIn .25s ease}
    .page.active{display:block}
    @keyframes fadeIn{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:translateY(0)}}

    /* ── Stats ── */
    .stats-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:14px;margin-bottom:24px}
    .stat-card{
      background:var(--surface);border:1px solid var(--border);border-radius:16px;
      padding:18px 20px;transition:border-color .2s,transform .2s;cursor:default
    }
    .stat-card:hover{border-color:var(--accent);transform:translateY(-2px)}
    .stat-card .stat-icon{
      width:36px;height:36px;border-radius:10px;display:flex;align-items:center;justify-content:center;
      font-size:15px;margin-bottom:12px
    }
    .stat-card .stat-label{font-size:11px;color:var(--muted2);text-transform:uppercase;letter-spacing:.08em;font-weight:600}
    .stat-card .stat-value{font-size:28px;font-weight:700;margin-top:4px;line-height:1}
    .stat-card .stat-desc{font-size:11px;color:var(--muted);margin-top:4px}

    /* ── Job Submit Form ── */
    .submit-grid{display:grid;grid-template-columns:1fr 1fr;gap:20px}
    @media(max-width:800px){.submit-grid,.form-row{grid-template-columns:1fr}}
    .priority-group{display:flex;gap:8px}
    .priority-btn{
      flex:1;padding:8px;border-radius:8px;font-size:12px;font-weight:600;
      border:1px solid var(--border);background:var(--bg2);color:var(--muted);cursor:pointer;transition:all .15s;text-align:center
    }
    .priority-btn.selected{border-color:var(--accent);background:rgba(99,102,241,.12);color:var(--accent2)}

    /* ── Job Table ── */
    .table-header{display:flex;align-items:center;justify-content:space-between;margin-bottom:14px;gap:12px;flex-wrap:wrap}
    .table-header h4{font-size:14px;font-weight:600}
    .table-controls{display:flex;gap:8px}
    .table-controls input,.table-controls select{width:auto;min-width:140px}
    .job-id{font-family:'SF Mono','Fira Code',monospace;font-size:11px;color:var(--muted2)}
    .progress-bar-outer{height:4px;background:var(--border);border-radius:2px;min-width:80px}
    .progress-bar-inner{height:4px;background:linear-gradient(90deg,var(--accent),var(--accent3));border-radius:2px;transition:width .5s}
    .action-btns{display:flex;gap:6px}

    /* ── Modal ── */
    .modal-overlay{
      position:fixed;inset:0;background:rgba(0,0,0,.7);z-index:200;
      display:flex;align-items:center;justify-content:center;padding:20px;
      backdrop-filter:blur(4px);animation:fadeIn .2s ease
    }
    .modal{
      background:var(--surface);border:1px solid var(--border);border-radius:20px;
      width:100%;max-width:580px;max-height:85vh;overflow-y:auto;
      box-shadow:0 24px 64px rgba(0,0,0,.6);padding:28px
    }
    .modal-header{display:flex;align-items:center;justify-content:space-between;margin-bottom:24px}
    .modal-header h3{font-size:16px;font-weight:700}
    .modal-close{background:none;border:none;color:var(--muted);font-size:20px;cursor:pointer;padding:4px;border-radius:6px}
    .modal-close:hover{color:var(--text)}

    /* ── Webhook ── */
    .webhook-list{display:flex;flex-direction:column;gap:10px}
    .webhook-card{
      background:var(--bg2);border:1px solid var(--border);border-radius:12px;padding:14px 16px;
      display:flex;align-items:center;justify-content:space-between;gap:12px
    }
    .webhook-card .wh-info{flex:1;min-width:0}
    .webhook-card .wh-url{font-size:13px;font-weight:500;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
    .webhook-card .wh-events{font-size:11px;color:var(--muted);margin-top:3px}

    /* ── Keys panel ── */
    .key-display-box{
      background:var(--bg2);border:1px solid var(--border);border-radius:12px;
      padding:16px;font-family:'SF Mono','Fira Code',monospace;font-size:12px;
      color:var(--accent2);word-break:break-all;position:relative
    }
    .masked{color:var(--muted2);letter-spacing:2px}

    @media(max-width:768px){
      #sidebar{transform:translateX(-100%)}
      #sidebar.open{transform:translateX(0)}
      #main{margin-left:0}
      #topbar .menu-btn{display:block}
    }
  </style>
</head>
<body>
<div class="layout">

<!-- ── Sidebar ── -->
<nav id="sidebar">
  <div class="sidebar-brand">
    <div class="brand-icon"><i class="fas fa-layer-group"></i></div>
    <div>
      <h2>Task Queue</h2>
      <span>Client Portal</span>
    </div>
  </div>
  <div class="sidebar-nav">
    <div class="nav-label">Overview</div>
    <button class="nav-item active" id="nav-overview" onclick="showPage('overview','Overview','Monitor your queue health')">
      <i class="fas fa-gauge-high"></i> Dashboard
    </button>

    <div class="nav-label">Jobs</div>
    <button class="nav-item" id="nav-submit" onclick="showPage('submit','Submit Job','Create and dispatch a new background job')">
      <i class="fas fa-paper-plane"></i> Submit Job
    </button>
    <button class="nav-item" id="nav-jobs" onclick="showPage('jobs','My Jobs','All jobs for your tenant'); loadJobs()">
      <i class="fas fa-list-check"></i> My Jobs
    </button>
    <button class="nav-item" id="nav-dlq" onclick="showPage('dlq','Dead Letter Queue','Jobs that failed permanently'); loadDLQ()">
      <i class="fas fa-skull-crossbones"></i> Dead Letter Queue
    </button>

    <div class="nav-label">Integrations</div>
    <button class="nav-item" id="nav-webhooks" onclick="showPage('webhooks','Webhooks','Receive real-time job notifications'); loadWebhooks()">
      <i class="fas fa-webhook"></i> Webhooks
    </button>

    <div class="nav-label">Account</div>
    <button class="nav-item" id="nav-apikey" onclick="showPage('apikey','API Key','Manage your access credentials')">
      <i class="fas fa-key"></i> API Key
    </button>
    <button class="nav-item" id="nav-docs" onclick="showPage('docs','How To Use','Quick reference for all API actions')">
      <i class="fas fa-book-open"></i> Documentation
    </button>
  </div>
  <div class="sidebar-footer">
    <div class="avatar" id="avatar-initials">?</div>
    <div class="user-info">
      <div class="user-name" id="sidebar-tenant">Loading...</div>
      <div class="user-role">Client Tenant</div>
    </div>
    <button class="logout-btn" onclick="logout()"><i class="fas fa-sign-out-alt"></i></button>
  </div>
</nav>

<!-- ── Main ── -->
<div id="main">
  <div id="topbar">
    <div class="left">
      <button class="menu-btn" onclick="toggleSidebar()"><i class="fas fa-bars"></i></button>
      <div class="page-title">
        <h3 id="page-title-h3">Dashboard</h3>
        <p id="page-title-p">Monitor your queue health</p>
      </div>
    </div>
    <div class="right">
      <div id="conn-badge" class="live-badge"><div class="live-dot"></div>Live</div>
      <button class="btn btn-sm btn-secondary" onclick="refreshAll()"><i class="fas fa-rotate-right"></i> Refresh</button>
    </div>
  </div>

  <div id="content">

    <!-- ════ OVERVIEW PAGE ════ -->
    <div class="page active" id="page-overview">
      <div class="stats-grid" id="stats-grid">
        <div class="stat-card"><div class="stat-icon" style="background:var(--info-bg);color:var(--info)"><i class="fas fa-circle-dot"></i></div><div class="stat-label">Pending</div><div class="stat-value" id="s-pending">—</div><div class="stat-desc">Queued for processing</div></div>
        <div class="stat-card"><div class="stat-icon" style="background:rgba(99,102,241,.12);color:var(--accent2)"><i class="fas fa-gear fa-spin"></i></div><div class="stat-label">Processing</div><div class="stat-value" id="s-processing">—</div><div class="stat-desc">Currently running</div></div>
        <div class="stat-card"><div class="stat-icon" style="background:var(--good-bg);color:var(--good)"><i class="fas fa-circle-check"></i></div><div class="stat-label">Completed</div><div class="stat-value" id="s-completed">—</div><div class="stat-desc">Successfully finished</div></div>
        <div class="stat-card"><div class="stat-icon" style="background:var(--bad-bg);color:var(--bad)"><i class="fas fa-circle-xmark"></i></div><div class="stat-label">Failed</div><div class="stat-value" id="s-failed">—</div><div class="stat-desc">Needs attention</div></div>
        <div class="stat-card"><div class="stat-icon" style="background:var(--warn-bg);color:var(--warn)"><i class="fas fa-skull"></i></div><div class="stat-label">Dead Letter</div><div class="stat-value" id="s-dlq">—</div><div class="stat-desc">Permanently failed</div></div>
        <div class="stat-card"><div class="stat-icon" style="background:rgba(52,211,153,.08);color:#6ee7b7"><i class="fas fa-chart-line"></i></div><div class="stat-label">Total Jobs</div><div class="stat-value" id="s-total">—</div><div class="stat-desc">All time</div></div>
      </div>

      <div class="section-card">
        <div class="section-title"><i class="fas fa-bolt" style="color:var(--accent2);margin-right:6px"></i>Live Job Stream</div>
        <div id="live-events" style="max-height:300px;overflow-y:auto;font-family:'SF Mono','Fira Code',monospace;font-size:12px;line-height:1.8">
          <span style="color:var(--muted2)">Connecting to event stream...</span>
        </div>
      </div>

      <div class="section-card">
        <div class="table-header">
          <h4><i class="fas fa-clock-rotate-left" style="color:var(--muted2);margin-right:8px"></i>Recent Jobs</h4>
          <button class="btn btn-sm btn-secondary" onclick="loadRecentJobs()"><i class="fas fa-rotate"></i></button>
        </div>
        <div id="recent-jobs-table">
          <p style="color:var(--muted);font-size:13px;text-align:center;padding:20px">Loading...</p>
        </div>
      </div>
    </div>

    <!-- ════ SUBMIT JOB PAGE ════ -->
    <div class="page" id="page-submit">
      <div class="submit-grid">
        <div>
          <div class="section-card">
            <div class="section-title"><i class="fas fa-sliders" style="color:var(--accent2);margin-right:6px"></i>Job Configuration</div>
            <div id="submit-alert" class="hidden"></div>
            <div class="form-group">
              <label>Job Type</label>
              <select id="job-type">
                <option value="email">email — Send an email</option>
                <option value="image">image — Process an image</option>
                <option value="test">test — Test/no-op job</option>
              </select>
            </div>
            <div class="form-row">
              <div class="form-group">
                <label>Priority</label>
                <div class="priority-group" id="priority-group">
                  <div class="priority-btn selected" onclick="setPriority('low',this)">Low</div>
                  <div class="priority-btn" onclick="setPriority('normal',this)">Normal</div>
                  <div class="priority-btn" onclick="setPriority('high',this)">High</div>
                </div>
              </div>
              <div class="form-group">
                <label>Max Retries</label>
                <input id="max-retries" type="number" value="3" min="0" max="10">
              </div>
            </div>
            <div class="form-row">
              <div class="form-group">
                <label>Correlation ID <span style="color:var(--muted2)">(optional)</span></label>
                <input id="correlation-id" type="text" placeholder="trace-abc-123">
              </div>
              <div class="form-group">
                <label>Dedup Key <span style="color:var(--muted2)">(optional)</span></label>
                <input id="dedup-key" type="text" placeholder="unique-idempotency-key">
              </div>
            </div>
            <div class="form-group">
              <label>Run At <span style="color:var(--muted2)">(leave empty = immediate)</span></label>
              <input id="run-at" type="datetime-local">
            </div>
          </div>
        </div>

        <div>
          <div class="section-card" style="height:calc(100% - 16px)">
            <div class="section-title"><i class="fas fa-code" style="color:var(--accent2);margin-right:6px"></i>Payload (JSON)</div>
            <textarea id="job-payload" style="min-height:180px;font-family:'SF Mono','Fira Code',monospace;font-size:12px" placeholder='{"to": "user@example.com", "subject": "Hello"}'></textarea>
            <div style="height:14px"></div>
            <div class="section-title"><i class="fas fa-bell" style="color:var(--accent2);margin-right:6px"></i>Webhook (optional)</div>
            <div class="form-group">
              <label>Callback URL</label>
              <input id="webhook-url" type="url" placeholder="https://my-service.com/hooks/tq">
            </div>
          </div>
        </div>
      </div>
      <div style="display:flex;gap:10px;justify-content:flex-end">
        <button class="btn btn-secondary" onclick="resetSubmitForm()"><i class="fas fa-xmark"></i> Reset</button>
        <button class="btn btn-primary" id="submit-btn" onclick="submitJob()">
          <i class="fas fa-paper-plane"></i> Submit Job
        </button>
      </div>

      <!-- Success banner -->
      <div id="submit-success" class="section-card hidden" style="margin-top:16px;border-color:var(--good-border);background:var(--good-bg)">
        <div style="font-size:13px;color:var(--good);font-weight:600;margin-bottom:8px"><i class="fas fa-circle-check"></i> Job submitted successfully</div>
        <div style="font-size:12px;color:var(--muted)">Job ID: <code id="new-job-id" style="color:var(--accent2)"></code></div>
        <div style="margin-top:10px;display:flex;gap:8px">
          <button class="btn btn-sm btn-secondary" onclick="showPage('jobs','My Jobs','All jobs for your tenant');loadJobs()">View All Jobs</button>
          <button class="btn btn-sm btn-secondary" onclick="resetSubmitForm()">Submit Another</button>
        </div>
      </div>
    </div>

    <!-- ════ MY JOBS PAGE ════ -->
    <div class="page" id="page-jobs">
      <div class="section-card">
        <div class="table-header">
          <h4>All Jobs</h4>
          <div class="table-controls">
            <input type="text" id="jobs-search" placeholder="Search by ID or type..." oninput="filterJobs()" style="max-width:200px">
            <select id="jobs-filter-status" onchange="loadJobs()">
              <option value="">All Status</option>
              <option value="pending">Pending</option>
              <option value="processing">Processing</option>
              <option value="completed">Completed</option>
              <option value="failed">Failed</option>
              <option value="paused">Paused</option>
            </select>
            <button class="btn btn-sm btn-secondary" onclick="loadJobs()"><i class="fas fa-rotate"></i></button>
          </div>
        </div>
        <div style="overflow-x:auto" id="jobs-table-wrap">
          <p style="color:var(--muted);font-size:13px;text-align:center;padding:30px">Loading jobs...</p>
        </div>
        <div style="display:flex;align-items:center;justify-content:space-between;margin-top:14px">
          <button class="btn btn-sm btn-secondary" id="jobs-prev" onclick="jobsPage--;loadJobs()" disabled>← Prev</button>
          <span id="jobs-page-info" style="font-size:12px;color:var(--muted)"></span>
          <button class="btn btn-sm btn-secondary" id="jobs-next" onclick="jobsPage++;loadJobs()">Next →</button>
        </div>
      </div>
    </div>

    <!-- ════ DLQ PAGE ════ -->
    <div class="page" id="page-dlq">
      <div class="section-card">
        <div class="table-header">
          <h4><i class="fas fa-skull-crossbones" style="color:var(--bad);margin-right:6px"></i>Dead Letter Queue</h4>
          <button class="btn btn-sm btn-secondary" onclick="loadDLQ()"><i class="fas fa-rotate"></i></button>
        </div>
        <div id="dlq-alert" class="hidden" style="margin-bottom:12px"></div>
        <div id="dlq-table-wrap">
          <p style="color:var(--muted);font-size:13px;text-align:center;padding:30px">Loading...</p>
        </div>
      </div>
    </div>

    <!-- ════ WEBHOOKS PAGE ════ -->
    <div class="page" id="page-webhooks">
      <div class="section-card">
        <div class="table-header">
          <h4><i class="fas fa-webhook" style="color:var(--accent2);margin-right:6px"></i>Your Webhooks</h4>
          <button class="btn btn-sm btn-primary" onclick="openWebhookModal()"><i class="fas fa-plus"></i> Add Webhook</button>
        </div>
        <div id="webhooks-list"><p style="color:var(--muted);font-size:13px;text-align:center;padding:30px">Loading...</p></div>
      </div>

      <div class="section-card">
        <div class="section-title"><i class="fas fa-circle-info" style="color:var(--info);margin-right:6px"></i>How Webhooks Work</div>
        <p style="font-size:13px;color:var(--muted);line-height:1.7">
          When a job in your tenant completes or fails, the Task Queue will send an HTTP <code style="color:var(--accent2)">POST</code> to your registered URL.
          The request body is a JSON object with the full job record. Verify the HMAC-SHA256 signature in the
          <code style="color:var(--accent2)">X-TQ-Signature</code> header using your webhook secret to ensure authenticity.
        </p>
      </div>
    </div>

    <!-- ════ API KEY PAGE ════ -->
    <div class="page" id="page-apikey">
      <div class="section-card">
        <div class="section-title"><i class="fas fa-key" style="color:var(--accent2);margin-right:6px"></i>Your API Key</div>
        <p style="font-size:13px;color:var(--muted);margin-bottom:16px;line-height:1.6">
          This is the key currently stored in your browser session. Include it in the <code style="color:var(--accent2)">X-API-Key</code> header on every API request.
        </p>
        <div class="key-display-box" id="key-display-box">
          <span class="masked">Loading...</span>
          <button class="btn btn-sm btn-secondary" style="position:absolute;top:10px;right:10px" onclick="toggleKeyVis()"><i class="fas fa-eye" id="key-vis-icon"></i></button>
        </div>
        <div class="alert warning" style="margin-top:16px">
          <i class="fas fa-triangle-exclamation"></i>
          <span>Never share your API key. Anyone with this key can submit and manage jobs on behalf of your tenant.</span>
        </div>
        <div style="display:flex;gap:10px;margin-top:16px">
          <button class="btn btn-secondary" onclick="copyApiKey()"><i class="fas fa-copy"></i> Copy Key</button>
          <button class="btn btn-danger" onclick="logout()"><i class="fas fa-sign-out-alt"></i> Sign Out & Clear</button>
        </div>
      </div>

      <div class="section-card">
        <div class="section-title"><i class="fas fa-terminal" style="color:var(--accent2);margin-right:6px"></i>Quick Usage</div>
        <pre id="usage-snippet">Loading...</pre>
      </div>
    </div>

    <!-- ════ DOCS PAGE ════ -->
    <div class="page" id="page-docs">
      <div class="section-card">
        <div class="section-title">Quick Reference</div>
        <p style="font-size:13px;color:var(--muted);margin-bottom:20px">Everything you can do from the terminal using your API key.</p>

        <div style="display:flex;flex-direction:column;gap:14px">
          <div>
            <div style="font-size:12px;font-weight:600;color:var(--accent2);margin-bottom:6px"><i class="fas fa-paper-plane" style="margin-right:6px"></i>Submit a Job</div>
            <pre>curl -X POST http://your-host/jobs \
  -H "X-API-Key: YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{"type":"email","payload":{"to":"user@example.com"},"priority":"high","tenant_id":"your-tenant"}'</pre>
          </div>
          <div>
            <div style="font-size:12px;font-weight:600;color:var(--accent2);margin-bottom:6px"><i class="fas fa-eye" style="margin-right:6px"></i>Check Job Status</div>
            <pre>curl http://your-host/api/v1/jobs/{job_id} \
  -H "X-API-Key: YOUR_KEY"</pre>
          </div>
          <div>
            <div style="font-size:12px;font-weight:600;color:var(--accent2);margin-bottom:6px"><i class="fas fa-list" style="margin-right:6px"></i>List Your Jobs</div>
            <pre>curl "http://your-host/jobs?status=pending&limit=20" \
  -H "X-API-Key: YOUR_KEY"</pre>
          </div>
          <div>
            <div style="font-size:12px;font-weight:600;color:var(--accent2);margin-bottom:6px"><i class="fas fa-stop" style="margin-right:6px"></i>Cancel a Job</div>
            <pre>curl -X POST http://your-host/jobs/{job_id}/cancel \
  -H "X-API-Key: YOUR_KEY"</pre>
          </div>
          <div>
            <div style="font-size:12px;font-weight:600;color:var(--accent2);margin-bottom:6px"><i class="fas fa-rotate" style="margin-right:6px"></i>Replay a Failed Job (DLQ)</div>
            <pre>curl -X POST http://your-host/api/v1/dlq/{job_id}/replay \
  -H "X-API-Key: YOUR_KEY"</pre>
          </div>
          <div>
            <div style="font-size:12px;font-weight:600;color:var(--accent2);margin-bottom:6px"><i class="fas fa-webhook" style="margin-right:6px"></i>Register a Webhook</div>
            <pre>curl -X POST http://your-host/api/v1/webhooks \
  -H "X-API-Key: YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://my-app.com/hooks/tq","events":["job.completed","job.failed"],"tenant_id":"your-tenant"}'</pre>
          </div>
          <div>
            <div style="font-size:12px;font-weight:600;color:var(--accent2);margin-bottom:6px"><i class="fas fa-wave-square" style="margin-right:6px"></i>Subscribe to Live Events (SSE)</div>
            <pre>curl -N http://your-host/api/v1/events \
  -H "X-API-Key: YOUR_KEY"</pre>
          </div>
        </div>
      </div>
    </div>

  </div><!-- /content -->
</div><!-- /main -->
</div><!-- /layout -->

<!-- ── Webhook Modal ── -->
<div id="webhook-modal" class="modal-overlay hidden" onclick="closeWebhookModalIfOutside(event)">
  <div class="modal">
    <div class="modal-header">
      <h3><i class="fas fa-webhook" style="color:var(--accent2);margin-right:8px"></i>Add Webhook</h3>
      <button class="modal-close" onclick="closeWebhookModal()"><i class="fas fa-xmark"></i></button>
    </div>
    <div id="wh-modal-alert" class="hidden" style="margin-bottom:14px"></div>
    <div class="form-group">
      <label>Callback URL</label>
      <input id="wh-url" type="url" placeholder="https://your-service.com/hooks/tq">
    </div>
    <div class="form-group">
      <label>Events</label>
      <div style="display:flex;flex-wrap:wrap;gap:8px;margin-top:4px">
        <label style="display:flex;align-items:center;gap:6px;font-size:13px;color:var(--text);cursor:pointer">
          <input type="checkbox" id="ev-completed" checked style="width:auto;accent-color:var(--accent)"> job.completed
        </label>
        <label style="display:flex;align-items:center;gap:6px;font-size:13px;color:var(--text);cursor:pointer">
          <input type="checkbox" id="ev-failed" checked style="width:auto;accent-color:var(--accent)"> job.failed
        </label>
        <label style="display:flex;align-items:center;gap:6px;font-size:13px;color:var(--text);cursor:pointer">
          <input type="checkbox" id="ev-created" style="width:auto;accent-color:var(--accent)"> job.created
        </label>
      </div>
    </div>
    <div class="form-group">
      <label>Secret <span style="color:var(--muted2)">(optional — used for HMAC verification)</span></label>
      <input id="wh-secret" type="text" placeholder="my-webhook-secret">
    </div>
    <div style="display:flex;gap:10px;justify-content:flex-end;margin-top:6px">
      <button class="btn btn-secondary" onclick="closeWebhookModal()">Cancel</button>
      <button class="btn btn-primary" id="wh-save-btn" onclick="saveWebhook()"><i class="fas fa-plus"></i> Register</button>
    </div>
  </div>
</div>

<!-- ── Job Detail Modal ── -->
<div id="job-modal" class="modal-overlay hidden" onclick="closeJobModalIfOutside(event)">
  <div class="modal">
    <div class="modal-header">
      <h3><i class="fas fa-circle-info" style="color:var(--accent2);margin-right:8px"></i>Job Detail</h3>
      <button class="modal-close" onclick="closeJobModal()"><i class="fas fa-xmark"></i></button>
    </div>
    <div id="job-modal-body" style="font-size:13px">Loading...</div>
    <div style="display:flex;gap:8px;margin-top:20px;flex-wrap:wrap" id="job-modal-actions"></div>
  </div>
</div>

<script>
// ── Auth ──────────────────────────────────────────────────────────────
const KEY = localStorage.getItem('tq_api_key');
if (!KEY) { window.location.href = '/client/login'; }

const H = {'X-API-Key': KEY, 'Content-Type': 'application/json'};
let currentTenant = '';
let jobsPage = 0;
const jobsPageSize = 20;
let selectedPriority = 'low';
let allJobs = [];
let keyVisible = false;

// ── Bootstrap ────────────────────────────────────────────────────────
async function bootstrap() {
  try {
    const res = await fetch('/api/v1/stats', {headers: H});
    if (res.status === 401) { logout(); return; }
    const data = await res.json();
    // Derive tenant from stats or use placeholder
    currentTenant = KEY.substring(8, 18) + '...';
    document.getElementById('sidebar-tenant').textContent = 'Tenant: ' + currentTenant;
    document.getElementById('avatar-initials').textContent = currentTenant[0].toUpperCase();
    updateStats(data);
  } catch(e) { showToast('Could not connect to server', 'error'); }
  loadRecentJobs();
  connectSSE();
  updateKeyPage();
}

// ── Stats ─────────────────────────────────────────────────────────────
function updateStats(d) {
  const set = (id, val) => { const el = document.getElementById(id); if(el) el.textContent = val !== undefined ? val : '—'; };
  set('s-pending',    d.total_pending    ?? d.pending    ?? '—');
  set('s-processing', d.total_processing ?? d.processing ?? '—');
  set('s-completed',  d.total_completed  ?? d.completed  ?? '—');
  set('s-failed',     d.total_failed     ?? d.failed     ?? '—');
  set('s-dlq',        d.total_dlq        ?? d.dlq_count  ?? '—');
  set('s-total',      d.total_jobs       ?? '—');
}

async function refreshAll() {
  try {
    const res = await fetch('/api/v1/stats', {headers: H});
    if (!res.ok) throw new Error();
    updateStats(await res.json());
    showToast('Refreshed', 'success');
  } catch { showToast('Refresh failed', 'error'); }
}

// ── SSE Live Events ────────────────────────────────────────────────────
function connectSSE() {
  const box = document.getElementById('live-events');
  const badge = document.getElementById('conn-badge');
  const url = '/api/v1/events';
  const es = new EventSource(url, {headers: {'X-API-Key': KEY}});
  // Note: EventSource doesn't support custom headers; use URL param fallback
  const es2 = new EventSource(url + '?api_key=' + encodeURIComponent(KEY));
  es2.onopen = () => { badge.className = 'live-badge'; badge.innerHTML = '<div class="live-dot"></div>Live'; };
  es2.onerror = () => { badge.className = 'dead-badge'; badge.innerHTML = '<i class="fas fa-wifi-slash" style="font-size:10px"></i> Disconnected'; };
  es2.onmessage = (e) => {
    const line = document.createElement('div');
    try {
      const d = JSON.parse(e.data);
      const color = d.status === 'completed' ? 'var(--good)' : d.status === 'failed' ? 'var(--bad)' : 'var(--accent2)';
      line.innerHTML = '<span style="color:var(--muted2)">' + new Date().toLocaleTimeString() + '</span> '
        + '<span style="color:' + color + ';font-weight:600">' + (d.status || 'event') + '</span>'
        + ' <span style="color:var(--muted)">' + (d.id || '') + '</span>'
        + (d.type ? ' <span style="color:var(--info)">[' + d.type + ']</span>' : '');
    } catch { line.textContent = e.data; line.style.color = 'var(--muted)'; }
    box.appendChild(line);
    box.scrollTop = box.scrollHeight;
    if (box.children.length > 100) box.removeChild(box.firstChild);
  };
}

// ── Recent Jobs (Overview) ─────────────────────────────────────────────
async function loadRecentJobs() {
  const wrap = document.getElementById('recent-jobs-table');
  try {
    const res = await fetch('/jobs?limit=8', {headers: H});
    const data = await res.json();
    const jobs = data.jobs || data || [];
    wrap.innerHTML = buildJobTable(jobs.slice(0, 8));
  } catch { wrap.innerHTML = '<p style="color:var(--muted);font-size:13px;text-align:center;padding:20px">Could not load jobs.</p>'; }
}

// ── Jobs Page ─────────────────────────────────────────────────────────
async function loadJobs() {
  const wrap = document.getElementById('jobs-table-wrap');
  const status = document.getElementById('jobs-filter-status').value;
  const offset = jobsPage * jobsPageSize;
  wrap.innerHTML = '<p style="color:var(--muted);font-size:13px;text-align:center;padding:30px">Loading...</p>';
  try {
    let url = '/jobs?limit=' + jobsPageSize + '&offset=' + offset;
    if (status) url += '&status=' + status;
    const res = await fetch(url, {headers: H});
    const data = await res.json();
    allJobs = data.jobs || data || [];
    wrap.innerHTML = buildJobTable(allJobs);
    document.getElementById('jobs-prev').disabled = jobsPage === 0;
    document.getElementById('jobs-next').disabled = allJobs.length < jobsPageSize;
    document.getElementById('jobs-page-info').textContent = 'Page ' + (jobsPage + 1);
  } catch { wrap.innerHTML = '<p style="color:var(--muted);font-size:13px;text-align:center;padding:30px">Error loading jobs.</p>'; }
}

function filterJobs() {
  const q = document.getElementById('jobs-search').value.toLowerCase();
  const filtered = allJobs.filter(j => j.id.toLowerCase().includes(q) || (j.type||'').toLowerCase().includes(q));
  document.getElementById('jobs-table-wrap').innerHTML = buildJobTable(filtered);
}

function buildJobTable(jobs) {
  if (!jobs.length) return '<p style="color:var(--muted);font-size:13px;text-align:center;padding:30px">No jobs found.</p>';
  const statusClass = s => ({completed:'good',failed:'bad',processing:'info',pending:'',paused:'warn'}[s] || '');
  return '<table><thead><tr><th>ID</th><th>Type</th><th>Status</th><th>Priority</th><th>Progress</th><th>Created</th><th></th></tr></thead><tbody>'
    + jobs.map(j => '<tr>'
      + '<td class="job-id">' + (j.id||'').substring(0,12) + '…</td>'
      + '<td>' + (j.type||'—') + '</td>'
      + '<td><span class="pill ' + statusClass(j.status) + '">' + (j.status||'—') + '</span></td>'
      + '<td style="text-transform:capitalize">' + (j.priority||'—') + '</td>'
      + '<td><div class="progress-bar-outer"><div class="progress-bar-inner" style="width:' + (j.progress||0) + '%"></div></div><span style="font-size:10px;color:var(--muted)">' + Math.round(j.progress||0) + '%</span></td>'
      + '<td style="color:var(--muted);font-size:11px">' + fmtTime(j.created_at) + '</td>'
      + '<td><div class="action-btns">'
      + '<button class="btn btn-sm btn-secondary" title="Details" onclick="viewJob(\'' + j.id + '\')"><i class="fas fa-eye"></i></button>'
      + (j.status==='processing'||j.status==='pending' ? '<button class="btn btn-sm btn-danger" title="Cancel" onclick="cancelJob(\'' + j.id + '\')"><i class="fas fa-stop"></i></button>' : '')
      + '</div></td>'
      + '</tr>').join('') + '</tbody></table>';
}

// ── DLQ ───────────────────────────────────────────────────────────────
async function loadDLQ() {
  const wrap = document.getElementById('dlq-table-wrap');
  wrap.innerHTML = '<p style="color:var(--muted);font-size:13px;text-align:center;padding:30px">Loading...</p>';
  try {
    const res = await fetch('/api/v1/dlq', {headers: H});
    const data = await res.json();
    const jobs = data.jobs || data || [];
    if (!jobs.length) { wrap.innerHTML = '<p style="color:var(--good);font-size:13px;text-align:center;padding:30px"><i class="fas fa-circle-check"></i> Dead Letter Queue is empty.</p>'; return; }
    wrap.innerHTML = '<table><thead><tr><th>ID</th><th>Type</th><th>Retries</th><th>Failed At</th><th>Error</th><th></th></tr></thead><tbody>'
      + jobs.map(j => '<tr>'
        + '<td class="job-id">' + (j.id||'').substring(0,12) + '…</td>'
        + '<td>' + (j.type||'—') + '</td>'
        + '<td style="color:var(--bad)">' + (j.attempts||j.retries||0) + '</td>'
        + '<td style="font-size:11px;color:var(--muted)">' + fmtTime(j.updated_at) + '</td>'
        + '<td style="font-size:11px;color:var(--muted);max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + (extractError(j)) + '</td>'
        + '<td><div class="action-btns">'
        + '<button class="btn btn-sm btn-secondary" title="Replay" onclick="replayJob(\'' + j.id + '\')"><i class="fas fa-rotate"></i> Replay</button>'
        + '<button class="btn btn-sm btn-danger" title="Delete" onclick="deleteFromDLQ(\'' + j.id + '\')"><i class="fas fa-trash"></i></button>'
        + '</div></td>'
        + '</tr>').join('') + '</tbody></table>';
  } catch { wrap.innerHTML = '<p style="color:var(--bad);font-size:13px;text-align:center;padding:30px">Error loading DLQ.</p>'; }
}

function extractError(j) {
  if (j.error) return j.error;
  if (j.result && typeof j.result === 'string') return j.result;
  if (j.error_history && j.error_history.length) return j.error_history[j.error_history.length-1].error || '';
  return '—';
}

async function replayJob(id) {
  try {
    const res = await fetch('/api/v1/dlq/' + id + '/replay', {method:'POST', headers: H});
    if (!res.ok) throw new Error('Replay failed');
    showToast('Job replayed successfully', 'success');
    loadDLQ();
  } catch(e) { showToast(e.message, 'error'); }
}

async function deleteFromDLQ(id) {
  if (!confirm('Delete this job permanently?')) return;
  try {
    const res = await fetch('/api/v1/dlq/' + id, {method:'DELETE', headers: H});
    if (!res.ok) throw new Error();
    showToast('Job deleted', 'success');
    loadDLQ();
  } catch { showToast('Delete failed', 'error'); }
}

// ── Job Detail Modal ──────────────────────────────────────────────────
async function viewJob(id) {
  document.getElementById('job-modal').classList.remove('hidden');
  document.getElementById('job-modal-body').innerHTML = '<p style="color:var(--muted)">Loading...</p>';
  document.getElementById('job-modal-actions').innerHTML = '';
  try {
    const res = await fetch('/api/v1/jobs/' + id, {headers: H});
    const j = await res.json();
    const statusClass = s => ({completed:'good',failed:'bad',processing:'info',pending:'',paused:'warn'}[s] || '');
    document.getElementById('job-modal-body').innerHTML =
      '<div style="display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-bottom:16px">'
      + kv('ID', '<code style="color:var(--accent2);font-size:11px">' + j.id + '</code>')
      + kv('Type', j.type)
      + kv('Status', '<span class="pill ' + statusClass(j.status) + '">' + j.status + '</span>')
      + kv('Priority', j.priority)
      + kv('Attempts', (j.attempts||j.retries||0) + ' / ' + (j.max_attempts||j.max_retries||3))
      + kv('Progress', Math.round(j.progress||0) + '%')
      + kv('Created', fmtTime(j.created_at))
      + kv('Updated', fmtTime(j.updated_at))
      + '</div>'
      + '<div style="margin-bottom:12px"><div class="section-title">Payload</div><pre>' + JSON.stringify(j.payload||{}, null, 2) + '</pre></div>'
      + (j.result ? '<div><div class="section-title">Result</div><pre>' + JSON.stringify(j.result, null, 2) + '</pre></div>' : '');

    const acts = document.getElementById('job-modal-actions');
    if (j.status === 'pending' || j.status === 'processing') {
      acts.innerHTML += '<button class="btn btn-sm btn-secondary" onclick="pauseJob(\'' + id + '\')"><i class="fas fa-pause"></i> Pause</button>';
      acts.innerHTML += '<button class="btn btn-sm btn-danger" onclick="cancelJob(\'' + id + '\')"><i class="fas fa-stop"></i> Cancel</button>';
    }
    if (j.status === 'paused') {
      acts.innerHTML += '<button class="btn btn-sm btn-primary" onclick="resumeJob(\'' + id + '\')"><i class="fas fa-play"></i> Resume</button>';
    }
  } catch { document.getElementById('job-modal-body').innerHTML = '<p style="color:var(--bad)">Could not load job details.</p>'; }
}

function kv(k, v) {
  return '<div><div style="font-size:11px;color:var(--muted2);font-weight:600;text-transform:uppercase;letter-spacing:.08em;margin-bottom:3px">' + k + '</div><div style="font-size:13px">' + v + '</div></div>';
}

function closeJobModal() { document.getElementById('job-modal').classList.add('hidden'); }
function closeJobModalIfOutside(e) { if (e.target.id === 'job-modal') closeJobModal(); }

// ── Job Actions ───────────────────────────────────────────────────────
async function cancelJob(id) {
  if (!confirm('Cancel this job?')) return;
  try {
    await fetch('/jobs/' + id + '/cancel', {method:'POST', headers: H});
    showToast('Job cancelled', 'success'); closeJobModal(); loadJobs();
  } catch { showToast('Cancel failed', 'error'); }
}

async function pauseJob(id) {
  try {
    await fetch('/jobs/' + id + '/pause', {method:'POST', headers: H});
    showToast('Job paused', 'success'); closeJobModal(); loadJobs();
  } catch { showToast('Pause failed', 'error'); }
}

async function resumeJob(id) {
  try {
    await fetch('/jobs/' + id + '/resume', {method:'POST', headers: H});
    showToast('Job resumed', 'success'); closeJobModal(); loadJobs();
  } catch { showToast('Resume failed', 'error'); }
}

// ── Submit Job ────────────────────────────────────────────────────────
function setPriority(val, el) {
  selectedPriority = val;
  document.querySelectorAll('.priority-btn').forEach(b => b.classList.remove('selected'));
  el.classList.add('selected');
}

async function submitJob() {
  const btn = document.getElementById('submit-btn');
  const alertEl = document.getElementById('submit-alert');
  alertEl.className = 'hidden';
  document.getElementById('submit-success').classList.add('hidden');

  let payload = {};
  const raw = document.getElementById('job-payload').value.trim();
  if (raw) {
    try { payload = JSON.parse(raw); }
    catch { showSubmitAlert('Payload must be valid JSON.', 'error'); return; }
  }

  const body = {
    type: document.getElementById('job-type').value,
    payload,
    priority: selectedPriority,
    max_retries: parseInt(document.getElementById('max-retries').value) || 3,
    tenant_id: 'client-portal',
  };
  const cid = document.getElementById('correlation-id').value.trim();
  if (cid) body.correlation_id = cid;
  const dk = document.getElementById('dedup-key').value.trim();
  if (dk) body.dedup_key = dk;
  const ra = document.getElementById('run-at').value;
  if (ra) body.run_at = new Date(ra).toISOString();
  const wurl = document.getElementById('webhook-url').value.trim();
  if (wurl) body.webhook = {url: wurl, events: ['job.completed','job.failed']};

  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span> Submitting...';

  try {
    const res = await fetch('/jobs', {method:'POST', headers: H, body: JSON.stringify(body)});
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Submit failed');
    document.getElementById('new-job-id').textContent = data.id || data.job_id;
    document.getElementById('submit-success').classList.remove('hidden');
    showToast('Job submitted!', 'success');
  } catch(e) {
    showSubmitAlert(e.message, 'error');
  } finally {
    btn.disabled = false;
    btn.innerHTML = '<i class="fas fa-paper-plane"></i> Submit Job';
  }
}

function showSubmitAlert(msg, type) {
  const el = document.getElementById('submit-alert');
  el.className = 'alert ' + type;
  el.innerHTML = '<i class="fas fa-circle-exclamation"></i> ' + msg;
}

function resetSubmitForm() {
  document.getElementById('job-payload').value = '';
  document.getElementById('correlation-id').value = '';
  document.getElementById('dedup-key').value = '';
  document.getElementById('run-at').value = '';
  document.getElementById('webhook-url').value = '';
  document.getElementById('max-retries').value = '3';
  document.getElementById('submit-alert').className = 'hidden';
  document.getElementById('submit-success').classList.add('hidden');
  setPriority('low', document.querySelector('.priority-btn'));
}

// ── Webhooks ──────────────────────────────────────────────────────────
async function loadWebhooks() {
  const list = document.getElementById('webhooks-list');
  try {
    const res = await fetch('/api/v1/webhooks', {headers: H});
    const data = await res.json();
    const hooks = data.webhooks || data || [];
    if (!hooks.length) { list.innerHTML = '<p style="color:var(--muted);font-size:13px;text-align:center;padding:30px">No webhooks registered yet.</p>'; return; }
    list.innerHTML = '<div class="webhook-list">' + hooks.map(h =>
      '<div class="webhook-card">'
      + '<div class="wh-info"><div class="wh-url">' + h.url + '</div><div class="wh-events">' + (h.events||[]).join(', ') + '</div></div>'
      + '<button class="btn btn-sm btn-danger" onclick="deleteWebhook(\'' + h.id + '\')"><i class="fas fa-trash"></i></button>'
      + '</div>'
    ).join('') + '</div>';
  } catch { list.innerHTML = '<p style="color:var(--bad);font-size:13px;text-align:center;padding:30px">Could not load webhooks.</p>'; }
}

function openWebhookModal() { document.getElementById('webhook-modal').classList.remove('hidden'); }
function closeWebhookModal() { document.getElementById('webhook-modal').classList.add('hidden'); }
function closeWebhookModalIfOutside(e) { if (e.target.id === 'webhook-modal') closeWebhookModal(); }

async function saveWebhook() {
  const url = document.getElementById('wh-url').value.trim();
  const alert = document.getElementById('wh-modal-alert');
  if (!url) { showModalAlert(alert, 'URL is required'); return; }
  const events = [];
  if (document.getElementById('ev-completed').checked) events.push('job.completed');
  if (document.getElementById('ev-failed').checked) events.push('job.failed');
  if (document.getElementById('ev-created').checked) events.push('job.created');
  const body = {url, events, tenant_id: 'client-portal'};
  const sec = document.getElementById('wh-secret').value.trim();
  if (sec) body.secret = sec;

  const btn = document.getElementById('wh-save-btn');
  btn.disabled = true; btn.innerHTML = '<span class="spinner"></span>';
  try {
    const res = await fetch('/api/v1/webhooks', {method:'POST', headers: H, body: JSON.stringify(body)});
    if (!res.ok) throw new Error('Failed to register webhook');
    showToast('Webhook registered', 'success');
    closeWebhookModal();
    loadWebhooks();
  } catch(e) { showModalAlert(alert, e.message); }
  finally { btn.disabled = false; btn.innerHTML = '<i class="fas fa-plus"></i> Register'; }
}

async function deleteWebhook(id) {
  if (!confirm('Delete this webhook?')) return;
  try {
    await fetch('/api/v1/webhooks/' + id, {method:'DELETE', headers: H});
    showToast('Webhook deleted', 'success'); loadWebhooks();
  } catch { showToast('Delete failed', 'error'); }
}

function showModalAlert(el, msg) {
  el.className = 'alert error';
  el.innerHTML = '<i class="fas fa-circle-exclamation"></i> ' + msg;
}

// ── API Key Page ───────────────────────────────────────────────────────
function updateKeyPage() {
  const box = document.getElementById('key-display-box');
  const snippet = document.getElementById('usage-snippet');
  if (box) box.innerHTML = '<span class="masked">' + '•'.repeat(40) + '</span><button class="btn btn-sm btn-secondary" style="position:absolute;top:10px;right:10px" onclick="toggleKeyVis()"><i class="fas fa-eye" id="key-vis-icon"></i></button>';
  if (snippet) snippet.textContent = 'curl -X POST http://your-host/jobs \\\n  -H "X-API-Key: ' + KEY + '" \\\n  -H "Content-Type: application/json" \\\n  -d \'{"type":"email","payload":{"to":"user@example.com"},"priority":"high","tenant_id":"my-service"}\'';
}

function toggleKeyVis() {
  keyVisible = !keyVisible;
  const box = document.getElementById('key-display-box');
  const icon = document.getElementById('key-vis-icon');
  if (keyVisible) {
    box.innerHTML = '<span style="word-break:break-all">' + KEY + '</span><button class="btn btn-sm btn-secondary" style="position:absolute;top:10px;right:10px" onclick="toggleKeyVis()"><i class="fas fa-eye-slash" id="key-vis-icon"></i></button>';
  } else {
    box.innerHTML = '<span class="masked">' + '•'.repeat(40) + '</span><button class="btn btn-sm btn-secondary" style="position:absolute;top:10px;right:10px" onclick="toggleKeyVis()"><i class="fas fa-eye" id="key-vis-icon"></i></button>';
  }
}

function copyApiKey() {
  navigator.clipboard.writeText(KEY).then(() => showToast('API key copied', 'success'));
}

// ── Navigation ────────────────────────────────────────────────────────
function showPage(id, title, subtitle) {
  document.querySelectorAll('.page').forEach(p => p.classList.remove('active'));
  document.querySelectorAll('.nav-item').forEach(n => n.classList.remove('active'));
  const page = document.getElementById('page-' + id);
  if (page) page.classList.add('active');
  const nav = document.getElementById('nav-' + id);
  if (nav) nav.classList.add('active');
  document.getElementById('page-title-h3').textContent = title;
  document.getElementById('page-title-p').textContent = subtitle;
}

function toggleSidebar() {
  document.getElementById('sidebar').classList.toggle('open');
}

function logout() {
  localStorage.removeItem('tq_api_key');
  window.location.href = '/client/login';
}

// ── Helpers ───────────────────────────────────────────────────────────
function fmtTime(ts) {
  if (!ts) return '—';
  try { return new Date(ts).toLocaleString(); } catch { return ts; }
}

function showToast(msg, type='info') {
  const icon = {success:'circle-check', error:'circle-exclamation', info:'circle-info'}[type] || 'circle-info';
  const t = document.createElement('div');
  t.className = 'toast ' + type;
  t.innerHTML = '<i class="fas fa-' + icon + '"></i> ' + msg;
  document.body.appendChild(t);
  setTimeout(() => { t.style.opacity='0'; t.style.transition='opacity .3s'; setTimeout(() => t.remove(), 300); }, 3000);
}

// ── Start ─────────────────────────────────────────────────────────────
bootstrap();
</script>
</body>
</html>`
