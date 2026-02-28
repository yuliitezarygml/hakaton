// ── clock ──
function tick() {
    const el = document.getElementById('hdr-clock');
    if (el) {
        const d = new Date();
        el.textContent = d.toLocaleTimeString('ru-RU');
    }
}
tick(); setInterval(tick, 1000);

// ── chart ──
let chart = null;
function updateChart(fakes, reals) {
    const total = fakes + reals;
    const pct = total > 0 ? Math.round(fakes / total * 100) : 0;
    document.getElementById('chart-pct').textContent = total > 0 ? pct + '%' : '—';
    document.getElementById('leg-fake').textContent = fakes;
    document.getElementById('leg-real').textContent = reals;
    const ctx = document.getElementById('ratioChart').getContext('2d');
    if (chart) chart.destroy();
    chart = new Chart(ctx, {
        type: 'doughnut',
        data: {
            datasets: [{
                data: total > 0 ? [fakes, reals] : [1, 1],
                backgroundColor: total > 0 ? ['#ff3355', '#00c96e'] : ['#1a1f2e', '#1a1f2e'],
                borderWidth: 0,
                hoverOffset: 3
            }]
        },
        options: {
            cutout: '72%',
            animation: { duration: 500 },
            plugins: { legend: { display: false }, tooltip: { enabled: total > 0 } }
        }
    });
}

// ── dashboard ──
async function loadDashboard() {
    const token = localStorage.getItem('adminToken');
    if (!token) return;
    try {
        const r = await fetch('/api/admin/stats', { headers: { 'X-Admin-Token': token } });
        if (r.status === 401) { return doLogout(); }
        const d = await r.json();

        document.getElementById('total-requests').textContent = d.total_requests ?? 0;
        document.getElementById('avg-score').textContent = (d.average_score ?? 0).toFixed(1);
        document.getElementById('ratio').textContent = `${d.fake_count ?? 0} / ${d.real_count ?? 0}`;

        const rows = d.recent_requests ?? [];
        document.getElementById('tbl-meta').textContent = rows.length + ' records';
        const body = document.getElementById('history-body');
        body.innerHTML = rows.length === 0
            ? `<tr><td colspan="4" class="empty-row">NO DATA</td></tr>`
            : rows.map(it => `
        <tr>
            <td class="td-url" title="${esc(it.url || '')}">${esc(it.url || 'Direct text')}</td>
            <td class="td-score">${it.score}/10</td>
            <td><span class="badge ${it.is_fake ? 'fake' : 'real'}">${it.is_fake ? 'FAKE' : 'OK'}</span></td>
            <td class="td-date">${new Date(it.created_at).toLocaleString('ru-RU')}</td>
        </tr>`).join('');

        updateChart(d.fake_count ?? 0, d.real_count ?? 0);
    } catch (e) { console.error(e); }
}

// ── limits ──
async function loadLimits() {
    const token = localStorage.getItem('adminToken');
    if (!token) return;
    try {
        const r = await fetch('/api/limits', { headers: { 'X-Admin-Token': token } });
        const d = await r.json();
        const grid = document.getElementById('limits-grid');

        const providers = Object.keys(d);
        if (providers.length === 0) {
            grid.innerHTML = '<div class="empty-row" style="grid-column: 1/-1">NO LIMIT DATA YET</div>';
            return;
        }

        grid.innerHTML = providers.map(p => {
            const l = d[p];
            const reqPct = l.limit_requests > 0 ? (l.remaining_requests / l.limit_requests * 100) : 100;
            const tokPct = l.limit_tokens > 0 ? (l.remaining_tokens / l.limit_tokens * 100) : 100;
            const isThrottled = l.throttled || l.status_code === 429;

            return `
                <div class="provider-card" style="${isThrottled ? 'border-color: var(--red);' : ''}">
                    <div class="pc-head">
                        <div class="pc-name">${p.toUpperCase()}</div>
                        <div class="pc-status" style="text-align: right;">
                            <div>${l.updated_ago}</div>
                            <div style="font-size: 0.45rem; color: ${isThrottled ? 'var(--red)' : 'var(--green)'};">
                                STATUS: ${l.status_code} ${isThrottled ? '[THROTTLED]' : '[OK]'}
                            </div>
                        </div>
                    </div>
                    
                    <div class="limit-item">
                        <div class="li-info">
                            <span class="li-lbl">Requests</span>
                            <span class="li-val">${l.remaining_requests} / ${l.limit_requests}</span>
                        </div>
                        <div class="progress-bg">
                            <div class="progress-fill ${reqPct < 20 ? 'warning' : ''}" style="width: ${reqPct}%"></div>
                        </div>
                    </div>
                    
                    <div class="limit-item">
                        <div class="li-info">
                            <span class="li-lbl">Tokens</span>
                            <span class="li-val">${formatN(l.remaining_tokens)} / ${formatN(l.limit_tokens)}</span>
                        </div>
                        <div class="progress-bg">
                            <div class="progress-fill ${tokPct < 20 ? 'warning' : ''}" style="width: ${tokPct}%"></div>
                        </div>
                    </div>
                    
                    <div class="reset-box">
                        <span>Reset Req: <b class="reset-timer" data-until="${l.reset_requests_at || 0}">${l.reset_requests || '—'}</b></span>
                        <span>Reset Tok: <b class="reset-timer" data-until="${l.reset_tokens_at || 0}">${l.reset_tokens || '—'}</b></span>
                    </div>
                </div>
            `;
        }).join('');
        updateResetTimers(); // Initial run
    } catch (e) { console.error(e); }
}

function formatN(n) {
    if (n < 0) return '—';
    if (n > 1000000) return (n / 1000000).toFixed(1) + 'M';
    if (n > 1000) return (n / 1000).toFixed(1) + 'k';
    return n;
}

// ── auth ──
function login() {
    const token = document.getElementById('admin-token').value.trim();
    if (!token) return;
    localStorage.setItem('adminToken', token);
    document.getElementById('auth-overlay').style.display = 'none';
    document.getElementById('app').style.display = 'flex';
    loadDashboard();
    connectLogs();
}

function doLogout() {
    localStorage.removeItem('adminToken');
    location.reload();
}

async function controlBackend(action) {
    const token = localStorage.getItem('adminToken');
    try {
        const res = await fetch(`/api/admin/${action}`, {
            method: 'POST',
            headers: { 'X-Admin-Token': token }
        });
        if (res.ok) {
            addLog(`Backend control command sent: ${action.toUpperCase()}`, 'info');
            checkBackendStatus();
        } else {
            addLog(`Backend control command failed: ${res.status}`, 'err');
        }
    } catch (e) {
        addLog(`Error controlling backend: ${e}`, 'err');
    }
}

async function checkBackendStatus() {
    const token = localStorage.getItem('adminToken');
    if (!token) return;
    try {
        const res = await fetch('/api/admin/status', {
            headers: { 'X-Admin-Token': token }
        });
        const data = await res.json();
        const tag = document.getElementById('backend-status-tag');
        if (data.is_paused) {
            tag.textContent = 'STATUS: PAUSED';
            tag.className = 'paused';
        } else {
            tag.textContent = 'STATUS: ACTIVE';
            tag.className = 'active';
        }
    } catch (e) { }
}

// ── logs ──
let ws = null, logLines = 0;

function connectLogs() {
    const token = localStorage.getItem('adminToken');
    if (!token) return;
    if (ws) ws.close();
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    ws = new WebSocket(`${proto}//${location.host}/api/admin/logs?token=${token}`);
    const wsSt = document.getElementById('ws-st');

    ws.onopen = () => {
        wsSt.textContent = 'ONLINE'; wsSt.className = 'on';
        addLog('WebSocket connected', 'info');
    };
    ws.onmessage = e => addLog(e.data);
    ws.onclose = () => {
        wsSt.textContent = 'OFFLINE'; wsSt.className = 'off';
        addLog('Connection closed — retry in 5s', 'err');
        setTimeout(connectLogs, 5000);
    };
}

function addLog(text, type = '') {
    const box = document.getElementById('log-console');
    const ts = new Date().toTimeString().slice(0, 8);
    const div = document.createElement('div');
    div.className = 'll' + (type ? ' ' + type : '');
    div.innerHTML = `<span class="ts">${ts}</span>${esc(text)}`;
    box.appendChild(div);
    box.scrollTop = box.scrollHeight;
    document.getElementById('log-meta').textContent = (++logLines) + ' lines';
}

// ── api tester ──
async function runTest() {
    const input = document.getElementById('test-url').value.trim();
    const btn = document.getElementById('test-btn');
    const out = document.getElementById('test-out');
    if (!input) return;
    btn.disabled = true; btn.textContent = '...';
    out.style.display = 'block'; out.textContent = '> Sending request...';
    try {
        const body = input.startsWith('http') ? { url: input } : { text: input };
        const r = await fetch('/api/analyze', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(body)
        });
        out.textContent = '> ' + JSON.stringify(await r.json(), null, 2);
        loadDashboard();
    } catch (e) {
        out.textContent = '> ERROR: ' + e.message;
    } finally {
        btn.disabled = false; btn.textContent = 'RUN ▶';
    }
}

function esc(s) {
    return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function updateResetTimers() {
    document.querySelectorAll('.reset-timer').forEach(el => {
        const until = parseInt(el.getAttribute('data-until'));
        if (!until || until <= Date.now()) return;

        const secs = Math.ceil((until - Date.now()) / 1000);
        if (secs <= 0) {
            el.textContent = 'READY';
            el.style.color = 'var(--green)';
            return;
        }

        const m = Math.floor(secs / 60);
        const s = secs % 60;
        el.textContent = `${m}:${s.toString().padStart(2, '0')}`;
        el.style.color = 'var(--red)';
    });
}

// ── countdown ──
let limitsSecs = 120;
function updateLimitsCountdown() {
    updateResetTimers(); // Update individual timers every second

    const el = document.getElementById('limits-next-update');
    if (!el) return;

    if (limitsSecs <= 0) {
        loadLimits();
        limitsSecs = 120;
    }

    const m = Math.floor(limitsSecs / 60);
    const s = limitsSecs % 60;
    el.textContent = `POLLING IN ${m}:${s.toString().padStart(2, '0')}`;
    limitsSecs--;
}

// ── init ──
const token = localStorage.getItem('adminToken');
if (token) {
    document.getElementById('auth-overlay').style.display = 'none';
    document.getElementById('app').style.display = 'flex';
    loadDashboard();
    connectLogs();
    loadLimits();
    checkBackendStatus();
    setInterval(loadDashboard, 30000);
    setInterval(updateLimitsCountdown, 1000);
    setInterval(checkBackendStatus, 10000);
}
