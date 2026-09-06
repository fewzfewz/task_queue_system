import re

with open('internal/api/handler/client_ui_handler.go', 'r') as f:
    code = f.read()

# We need to replace everything inside the landingHTML constant.
match = re.search(r'(const landingHTML = `)(.*?)(`\n)', code, re.DOTALL)
if not match:
    print("Could not find landingHTML")
    exit(1)

html = match.group(2)

# Make CSS modifications
html = html.replace('.hero{', '.hero{padding:120px clamp(20px,5vw,80px) 40px;')
html = html.replace('.features{max-width:1200px;margin:0 auto;padding:80px clamp(20px,5vw,80px)}', '')
html = html.replace('.register-section{', '.register-section{')
html = html.replace('.terminal{', '.terminal{position:relative;')

# We'll use CSS to create the new layout.
new_css = """
    .bottom-layout {
      max-width: 1200px; margin: 0 auto; padding: 40px clamp(20px, 5vw, 80px) 80px;
      display: grid; grid-template-columns: 1.2fr 0.8fr; gap: 60px;
    }
    @media(max-width:900px){ .bottom-layout { grid-template-columns: 1fr; } }
    
    .features h3 { font-size: 28px; font-weight: 800; margin-bottom: 24px; color: var(--text); }
    .feat-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
    .feat-card {
      background: rgba(31,42,64,0.4); border: 1px solid var(--border); border-radius: 16px;
      padding: 20px; transition: all .3s; backdrop-filter: blur(16px);
      text-align: left;
    }
    .feat-card:hover { border-color: rgba(99,102,241,0.5); background: rgba(31,42,64,0.6); }
    .feat-icon {
      width: 44px; height: 44px; border-radius: 12px; display: inline-flex; align-items: center; justify-content: center;
      font-size: 18px; margin-bottom: 12px;
    }
    .feat-card h4 { font-size: 14px; font-weight: 600; margin-bottom: 0; }
    .feat-card p { font-size: 13px; color: var(--muted); margin-top: 8px; line-height: 1.5; display: none; }
    
    .register-section { width: 100%; margin: 0; padding: 0; }
    .register-card {
      background: rgba(255,255,255,0.03); border: 1px solid var(--border); border-radius: 20px;
      padding: 40px; box-shadow: 0 30px 80px rgba(0,0,0,.6); backdrop-filter: blur(20px);
      position: relative;
    }
    .register-card::before {
      content: ''; position: absolute; inset: -1px; border-radius: 21px; z-index: -1;
      background: linear-gradient(135deg, rgba(255,255,255,0.1), transparent);
    }
    .rc-head { text-align: center; margin-bottom: 32px; }
    .rc-icon {
      width: 48px; height: 48px; border-radius: 50%; background: rgba(255,255,255,0.1);
      display: inline-flex; align-items: center; justify-content: center; font-size: 20px; color: #fff;
      margin-bottom: 16px;
    }
    .rc-head h3 { display: none; }
    .rc-head p { display: none; }
    
    .terminal-wrapper {
      position: relative;
      padding: 2px;
      border-radius: 18px;
      background: linear-gradient(135deg, rgba(99,102,241,0.8), rgba(192,132,252,0.8), rgba(56,189,248,0.8));
      box-shadow: 0 0 60px rgba(99,102,241,0.4);
    }
    .terminal {
      background: #0a0f1e; border: none; border-radius: 16px;
      padding: 24px; font-size: 13px; font-family: 'SF Mono', monospace;
      margin: 0;
    }
"""

html = html.replace('/* ── Features ── */', new_css + '\n    /* ── Features ── */')

# Hero modifications
html = html.replace('<p>A resilient, priority-aware job queue with retries, circuit breakers, dead-letter queues, and real-time monitoring — built for services that can\'t afford to miss a beat.</p>',
                    '<p>Background job processing system optimized with ultra modern, and ultra-modern production.</p>')
html = html.replace('<div class="hero-note"><i class="fas fa-check-circle"></i> Free to register &mdash; no credit card required</div>',
                    '<div class="hero-note">Note: You can register to get a free tier.</div>')

# Wrap terminal in glowing border
html = html.replace('<div class="terminal">', '<div class="terminal-wrapper"><div class="terminal">')
html = html.replace('<div class="stat-chips">', '</div>\n      <div class="stat-chips">') # Close terminal div before stat chips

# Replace the layout below hero
old_bottom = html.split('<!-- Features -->')[1]

new_bottom = """
<div class="bottom-layout">
  <!-- Features -->
  <section class="features" id="features">
    <div style="font-size: 24px; font-weight: 700; margin-bottom: 24px;">Why TaskQueue's Grid</div>
    <div class="feat-grid">
      <div class="feat-card">
        <div class="feat-icon" style="background:rgba(99,102,241,.15);color:var(--accent2)"><i class="fas fa-list-ol"></i></div>
        <h4>Priority Queues</h4>
      </div>
      <div class="feat-card">
        <div class="feat-icon" style="background:rgba(251,191,36,.15);color:var(--warn)"><i class="fas fa-sync"></i></div>
        <h4>Automatic Retries</h4>
      </div>
      <div class="feat-card">
        <div class="feat-icon" style="background:rgba(248,113,113,.15);color:var(--bad)"><i class="fas fa-bolt"></i></div>
        <h4>Circuit Breakers</h4>
      </div>
      <div class="feat-card">
        <div class="feat-icon" style="background:rgba(52,211,153,.15);color:var(--good)"><i class="fas fa-broadcast-tower"></i></div>
        <h4>Real-time SSE</h4>
      </div>
      <div class="feat-card">
        <div class="feat-icon" style="background:rgba(56,189,248,.15);color:var(--accent4)"><i class="fas fa-project-diagram"></i></div>
        <h4>DAG Dependencies</h4>
      </div>
      <div class="feat-card">
        <div class="feat-icon" style="background:rgba(192,132,252,.15);color:var(--accent3)"><i class="fas fa-satellite-dish"></i></div>
        <h4>Webhook Callbacks</h4>
      </div>
    </div>
  </section>

  <!-- Registration -->
  <section class="register-section" id="register">
    <div class="register-card" id="register-form-wrap">
      <div class="rc-head">
        <div class="rc-icon"><i class="fas fa-key"></i></div>
      </div>
      <div id="reg-alert" class="alert-box"></div>
      <div class="form-group">
        <label for="reg-tenant">Tenant ID</label>
        <input id="reg-tenant" type="text" placeholder="Tenant ID" autocomplete="off" maxlength="64" spellcheck="false">
      </div>
      <div class="form-group">
        <label for="reg-service">Service Name (optional)</label>
        <input id="reg-service" type="text" placeholder="Service Name (optional)" autocomplete="off">
      </div>
      <button class="btn-register" id="reg-btn" onclick="doRegister()">
        <span id="reg-label">Create Account & Get API Key</span>
      </button>
    </div>
  </section>
</div>

<!-- Footer -->
"""
# Note: footer doesn't have an ID, we'll split at <script> instead
old_scripts = html.split('<script>')[1]

html = html.split('<!-- Features -->')[0] + new_bottom + '<script>\n' + old_scripts

with open('internal/api/handler/client_ui_handler.go', 'w') as f:
    f.write(code[:match.start(2)] + html + code[match.end(2):])

print("Replaced!")
