const API_BASE = 'https://apich.sinkdev.dev';
const API_STREAM = API_BASE + '/api/analyze/stream';
const API_HEALTH = API_BASE + '/api/health';
const CACHE_TTL = 7 * 24 * 60 * 60 * 1000; // 7 days
const MAX_HISTORY = 30;

// ── Domain reputation ────────────────────────────────────────────
async function loadDomainRep(pageUrl) {
  const el = document.getElementById('domain-rep');
  if (!el || !pageUrl) return;
  try {
    const u = new URL(pageUrl);
    const domain = u.hostname.replace(/^www\./, '');
    const res = await fetch(`${API_BASE}/api/domain/${domain}`);
    if (!res.ok) return; // domain not yet in DB — silently skip
    const d = await res.json();
    const avg = d.avg_score.toFixed(1);
    const cls = d.avg_score >= 7 ? 'rep-good' : d.avg_score >= 4 ? 'rep-mid' : 'rep-bad';
    const emoji = d.avg_score >= 7 ? '🟢' : d.avg_score >= 4 ? '🟡' : '🔴';
    el.className = `domain-rep ${cls}`;
    el.innerHTML = `<span class="rep-score">${emoji} ${avg}/10</span><span><b>${domain}</b><br><span class="rep-info">${d.total_analyses} проверок · ${d.verdict}</span></span>`;
    el.style.display = 'flex';
  } catch (e) {
    // Network error or invalid URL — silently ignore
  }
}

// ── DOM refs ────────────────────────────────────────────────────
const statusDot = document.getElementById('status-dot');
const viewIdle = document.getElementById('view-idle');
const viewAuto = document.getElementById('view-auto');
const viewScanning = document.getElementById('view-scanning');
const viewAnalyzing = document.getElementById('view-analyzing');
const viewResult = document.getElementById('view-result');
const viewHistory = document.getElementById('view-history');
const btnAnalyzeUrl = document.getElementById('btn-analyze-url');
const btnCancelScan = document.getElementById('btn-cancel-scan');
const btnCancelAuto = document.getElementById('btn-cancel-auto');
const btnStop = document.getElementById('btn-stop');
const btnNew = document.getElementById('btn-new');
const btnHistoryTab = document.getElementById('btn-history');
const btnBack = document.getElementById('btn-back');
const btnClearHist = document.getElementById('btn-clear-history');
const scanFeed = document.getElementById('scan-feed');
const scanChars = document.getElementById('scan-chars');
const scanBarFill = document.getElementById('scan-bar-fill');
const eventLog = document.getElementById('event-log');
const resultScore = document.getElementById('result-score');
const resultVerdict = document.getElementById('result-verdict');
const resultSummary = document.getElementById('result-summary');
const resultSections = document.getElementById('result-sections');
const resultCacheLabel = document.getElementById('result-cache-label');
const autoMsg = document.getElementById('auto-msg');
const historyList = document.getElementById('history-list');

let currentReader = null;
let scanCancelled = false;
let autoCancelled = false;
let msgListener = null;
let userId = null;
let currentTabUrl = null;
let lastResult = null;

// ── User ID ─────────────────────────────────────────────────────
async function ensureUserId() {
  const data = await chrome.storage.local.get('userId');
  if (data.userId) return data.userId;
  const id = crypto.randomUUID();
  await chrome.storage.local.set({ userId: id });
  return id;
}

// ── History ─────────────────────────────────────────────────────
async function getHistory() {
  const data = await chrome.storage.local.get('history');
  return data.history || [];
}

async function saveToHistory(url, result) {
  const history = await getHistory();
  const entry = {
    url: url || 'direct-text',
    score: result.credibility_score,
    verdict: verdict(result.credibility_score),
    summary: result.summary || '',
    result,
    date: new Date().toISOString(),
  };
  // Replace existing entry for same URL
  const idx = history.findIndex(h => h.url === entry.url);
  if (idx >= 0) history.splice(idx, 1);
  history.unshift(entry);
  if (history.length > MAX_HISTORY) history.splice(MAX_HISTORY);
  await chrome.storage.local.set({ history });
}

async function getCached(url) {
  if (!url) return null;
  const history = await getHistory();
  const item = history.find(h => h.url === url);
  if (!item) return null;
  const age = Date.now() - new Date(item.date).getTime();
  if (age > CACHE_TTL) return null;
  return item;
}

// ── View switching ───────────────────────────────────────────────
function showView(name) {
  viewIdle.classList.toggle('hidden', name !== 'idle');
  viewAuto.classList.toggle('hidden', name !== 'auto');
  viewScanning.classList.toggle('hidden', name !== 'scanning');
  viewAnalyzing.classList.toggle('hidden', name !== 'analyzing');
  viewResult.classList.toggle('hidden', name !== 'result');
  viewHistory.classList.toggle('hidden', name !== 'history');
}

// ── Health check ─────────────────────────────────────────────────
async function checkHealth() {
  try {
    const resp = await fetch(API_HEALTH, { signal: AbortSignal.timeout(3000) });
    if (resp.ok) {
      statusDot.className = 'status-dot online';
      return true;
    }
  } catch { }
  statusDot.className = 'status-dot offline';
  btnAnalyzeUrl.disabled = true;
  btnAnalyzeUrl.textContent = '⚠️ Backend indisponibil';
  return false;
}

// ── Init ─────────────────────────────────────────────────────────
async function init() {
  userId = await ensureUserId();
  const online = await checkHealth();

  // Context menu autostart
  const params = new URLSearchParams(window.location.search);
  if (params.get('autostart') === '1') {
    const result = await chrome.storage.session.get('pendingText');
    if (result.pendingText) {
      await chrome.storage.session.remove('pendingText');
      if (online) {
        startAnalysis({ text: result.pendingText });
        return;
      }
    }
  }

  // Auto-scan current page on popup open
  if (online) {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
    if (tab?.url?.startsWith('http')) {
      currentTabUrl = tab.url;
      loadDomainRep(tab.url);

      // Check cache first
      const cached = await getCached(tab.url);
      if (cached) {
        renderResult(cached.result, true);
        return;
      }

      // Silent auto-scan
      autoCancelled = false;
      autoMsg.textContent = 'Scanarea paginii...';
      showView('auto');
      startScanSilent(tab);
      return;
    }
  }

  showView('idle');
}

// ── Silent scan (no animation on page) ──────────────────────────
async function startScanSilent(tab) {
  if (!tab?.id) { showView('idle'); return; }

  if (msgListener) {
    chrome.runtime.onMessage.removeListener(msgListener);
    msgListener = null;
  }

  // Set silent flag in tab, then inject content.js
  try {
    await chrome.scripting.executeScript({
      target: { tabId: tab.id },
      func: () => { window.__taSilentMode = true; window.__taRunning = false; },
    });
  } catch (e) {
    showView('idle');
    return;
  }

  const scanResult = await new Promise((resolve, reject) => {
    msgListener = (msg) => {
      if (autoCancelled) { reject(new Error('cancelled')); return; }
      if (msg.type === 'scan_done') {
        chrome.runtime.onMessage.removeListener(msgListener);
        msgListener = null;
        resolve(msg);
      }
    };
    chrome.runtime.onMessage.addListener(msgListener);

    chrome.scripting.executeScript({
      target: { tabId: tab.id },
      files: ['content.js'],
    }).catch(err => {
      chrome.runtime.onMessage.removeListener(msgListener);
      msgListener = null;
      reject(err);
    });

    // Timeout after 10s
    setTimeout(() => reject(new Error('timeout')), 10000);
  }).catch(err => {
    if (err.message !== 'cancelled') {
      console.error('[auto-scan] error:', err);
      showView('idle');
    } else {
      showView('idle');
    }
    return null;
  });

  if (!scanResult || autoCancelled) return;

  autoMsg.textContent = 'Analizez...';
  startAnalysis({ text: scanResult.text });
}

// ── Manual scan (with page animation) ───────────────────────────
async function startScan(tab) {
  if (!tab?.id) return;

  scanCancelled = false;
  scanFeed.innerHTML = '';
  scanChars.textContent = '0 caract.';
  scanBarFill.style.width = '0%';
  showView('scanning');

  if (msgListener) {
    chrome.runtime.onMessage.removeListener(msgListener);
    msgListener = null;
  }

  // Ensure animated mode
  try {
    await chrome.scripting.executeScript({
      target: { tabId: tab.id },
      func: () => { window.__taSilentMode = false; window.__taRunning = false; },
    });
  } catch { }

  const scanResult = await new Promise((resolve, reject) => {
    msgListener = (msg) => {
      if (scanCancelled) { reject(new Error('cancelled')); return; }
      if (msg.type === 'scan_chunk') {
        addScanChunk(msg.tag, msg.text);
        scanChars.textContent = msg.charCount.toLocaleString('ro') + ' caract.';
        if (msg.total > 0)
          scanBarFill.style.width = Math.round(((msg.index + 1) / msg.total) * 100) + '%';
      } else if (msg.type === 'scan_done') {
        scanBarFill.style.width = '100%';
        scanChars.textContent = msg.charCount.toLocaleString('ro') + ' caract.';
        chrome.runtime.onMessage.removeListener(msgListener);
        msgListener = null;
        resolve(msg);
      }
    };
    chrome.runtime.onMessage.addListener(msgListener);

    chrome.scripting.executeScript({
      target: { tabId: tab.id },
      files: ['content.js'],
    }).catch(err => {
      chrome.runtime.onMessage.removeListener(msgListener);
      msgListener = null;
      reject(err);
    });
  }).catch(err => {
    if (err.message !== 'cancelled') {
      addEvent('error', 'Eroare de scanare: ' + err.message);
      showView('analyzing');
    } else {
      showView('idle');
    }
    return null;
  });

  if (!scanResult || scanCancelled) return;

  addScanChunk('✓', 'Scanare finalizată, se trimite pentru analiză...');
  await new Promise(r => setTimeout(r, 400));
  startAnalysis({ text: scanResult.text });
}

// ── SSE analysis ─────────────────────────────────────────────────
async function startAnalysis(body) {
  eventLog.innerHTML = '';
  showView('analyzing');

  // Include userId for server-side tracking
  const payload = { ...body, user_id: userId };

  try {
    const resp = await fetch(API_STREAM, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });

    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    if (!resp.body) throw new Error('No response body');

    const reader = resp.body.getReader();
    currentReader = reader;
    const decoder = new TextDecoder();
    let buffer = '';
    let eventType = null;
    let eventData = null;

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() ?? '';

      for (const line of lines) {
        if (line.startsWith('event:')) {
          eventType = line.slice(6).trim();
        } else if (line.startsWith('data:')) {
          eventData = line.slice(5).trim();
        } else if (line === '' && eventType && eventData !== null) {
          addEvent(eventType, eventData);
          if (eventType === 'result') {
            try {
              const parsed = JSON.parse(eventData);
              renderResult(parsed, false);
            } catch (e) {
              console.error('Parse error:', e);
            }
          }
          eventType = null;
          eventData = null;
        }
      }
    }
  } catch (err) {
    if (err.name !== 'AbortError') {
      addEvent('error', err.message);
    }
  } finally {
    currentReader = null;
  }
}

// ── Render result ────────────────────────────────────────────────
async function renderResult(result, fromCache) {
  lastResult = result;
  const color = scoreColor(result.credibility_score);

  resultCacheLabel.classList.toggle('hidden', !fromCache);
  btnNew.textContent = fromCache ? '🔄 Reanalizează' : '↺ Analiză nouă';
  resultScore.textContent = result.credibility_score + '/10';
  resultScore.style.color = color;
  resultVerdict.textContent = verdict(result.credibility_score);
  resultVerdict.style.color = color;
  resultSummary.textContent = result.summary || '';

  resultSections.innerHTML = '';
  if (result.manipulations?.length)
    addSection('Manipulări', result.manipulations, '#fbbf24');
  if (result.logical_issues?.length)
    addSection('Erori logice', result.logical_issues, '#f97316');
  if (result.fact_check?.missing_evidence?.length)
    addSection('Afirmații fără dovezi', result.fact_check.missing_evidence, '#818cf8');
  if (result.fact_check?.opinions_as_facts?.length)
    addSection('Opinii ca fapte', result.fact_check.opinions_as_facts, '#a78bfa');

  showView('result');

  if (!fromCache) {
    // Save to history
    await saveToHistory(currentTabUrl, result);

    // Inject page notification
    if (currentTabUrl) {
      const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
      if (tab?.id) {
        chrome.scripting.executeScript({
          target: { tabId: tab.id },
          func: injectPageNotification,
          args: [result.credibility_score, verdict(result.credibility_score), scoreColor(result.credibility_score)],
        }).catch(() => { });
      }
    }
  }
}

// ── Page notification (injected into the page) ───────────────────
// This function runs in the page context (NOT in popup)
function injectPageNotification(score, verdictText, color) {
  const existing = document.getElementById('__ta_notify');
  if (existing) existing.remove();

  const styleEl = document.createElement('style');
  styleEl.id = '__ta_notify_style';
  styleEl.textContent = `
    @keyframes __ta_in {
      from { opacity:0; transform:translateY(16px); }
      to   { opacity:1; transform:translateY(0); }
    }
  `;
  document.head.appendChild(styleEl);

  const div = document.createElement('div');
  div.id = '__ta_notify';
  div.style.cssText = `
    position: fixed;
    bottom: 20px; right: 20px;
    background: rgba(8,8,14,0.96);
    border: 1px solid ${color};
    border-radius: 10px;
    padding: 12px 14px 12px 14px;
    font-family: 'Consolas','JetBrains Mono',monospace;
    color: ${color};
    z-index: 2147483647;
    box-shadow: 0 4px 28px rgba(0,0,0,0.6), 0 0 16px ${color}22;
    display: flex; align-items: center; gap: 12px;
    animation: __ta_in 0.3s ease;
    max-width: 300px;
    min-width: 200px;
  `;

  const emoji = score <= 3 ? '🔴' : score <= 6 ? '🟡' : '🟢';

  div.innerHTML = `
    <span style="font-size:22px;line-height:1">${emoji}</span>
    <div style="flex:1;min-width:0">
      <div style="font-weight:700;font-size:17px;line-height:1">${score}/10</div>
      <div style="font-size:11px;opacity:0.75;margin-top:3px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">${verdictText}</div>
    </div>
    <button id="__ta_close" style="background:none;border:none;color:${color};cursor:pointer;opacity:0.5;font-size:16px;padding:0;line-height:1;flex-shrink:0">✕</button>
  `;

  document.body.appendChild(div);

  document.getElementById('__ta_close').addEventListener('click', () => {
    div.style.transition = 'opacity 0.3s, transform 0.3s';
    div.style.opacity = '0';
    div.style.transform = 'translateY(16px)';
    setTimeout(() => { div.remove(); styleEl.remove(); }, 300);
  });

  // Auto-dismiss after 8s
  setTimeout(() => {
    if (!div.parentNode) return;
    div.style.transition = 'opacity 0.4s, transform 0.4s';
    div.style.opacity = '0';
    div.style.transform = 'translateY(16px)';
    setTimeout(() => { div.remove(); styleEl.remove(); }, 400);
  }, 8000);
}

// ── History rendering ────────────────────────────────────────────
async function showHistoryView() {
  const history = await getHistory();
  if (history.length === 0) {
    historyList.innerHTML = '<p class="hint" style="margin-top:16px">Istoricul este gol</p>';
  } else {
    historyList.innerHTML = history.map((item, i) => {
      const color = scoreColor(item.score);
      const url = item.url === 'direct-text' ? 'Text direct' : item.url;
      const date = new Date(item.date).toLocaleString('ro-RO', {
        day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit'
      });
      return `
        <div class="history-item" data-index="${i}">
          <span class="history-score" style="color:${color}">${item.score}</span>
          <div class="history-info">
            <div class="history-url" title="${escHtml(item.url)}">${escHtml(url)}</div>
            <div class="history-verdict">${escHtml(item.verdict)}</div>
          </div>
          <div class="history-date">${date}</div>
        </div>
      `;
    }).join('');

    // Click to re-view result
    historyList.querySelectorAll('.history-item').forEach(el => {
      el.addEventListener('click', () => {
        const item = history[parseInt(el.dataset.index)];
        if (item?.result) {
          renderResult(item.result, true);
        }
      });
    });
  }
  showView('history');
}

// ── Helpers ──────────────────────────────────────────────────────
function scoreColor(n) {
  if (n <= 3) return '#ef4444';
  if (n <= 6) return '#eab308';
  return '#22c55e';
}

function verdict(n) {
  if (n <= 3) return '🔴 DEZINFORMARE';
  if (n <= 6) return '🟡 INCERT';
  return '🟢 CREDIBIL';
}

function escHtml(s) {
  return String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function addScanChunk(tag, text) {
  const row = document.createElement('div');
  row.className = 'scan-chunk';
  const tagSpan = document.createElement('span');
  const tagKey = tag === '§' ? '§' : tag.toUpperCase();
  tagSpan.className = `scan-tag scan-tag-${tagKey}`;
  tagSpan.textContent = tagKey;
  const msgSpan = document.createElement('span');
  msgSpan.className = 'scan-text';
  msgSpan.textContent = text;
  row.appendChild(tagSpan);
  row.appendChild(msgSpan);
  scanFeed.appendChild(row);
  scanFeed.scrollTop = scanFeed.scrollHeight;
}

function addEvent(type, data) {
  const row = document.createElement('div');
  row.className = 'event-row';
  const badge = document.createElement('span');
  badge.className = `event-badge badge-${type}`;
  badge.textContent = type;
  const msg = document.createElement('span');
  msg.className = 'event-msg';
  msg.textContent = type === 'result' ? '📊 Rezultat primit' : data;
  row.appendChild(badge);
  row.appendChild(msg);
  eventLog.appendChild(row);
  eventLog.scrollTop = eventLog.scrollHeight;
}

function addSection(title, items, color) {
  const section = document.createElement('div');
  section.className = 'result-section';
  const h = document.createElement('div');
  h.className = 'section-title';
  h.style.color = color;
  h.textContent = title;
  const ul = document.createElement('ul');
  ul.className = 'section-list';
  items.slice(0, 6).forEach(item => {
    const li = document.createElement('li');
    li.textContent = item;
    ul.appendChild(li);
  });
  section.appendChild(h);
  section.appendChild(ul);
  resultSections.appendChild(section);
}

// ── Event listeners ──────────────────────────────────────────────
btnAnalyzeUrl.addEventListener('click', async () => {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  currentTabUrl = tab?.url || null;
  startScan(tab);
});

btnCancelScan.addEventListener('click', () => {
  scanCancelled = true;
  if (msgListener) {
    chrome.runtime.onMessage.removeListener(msgListener);
    msgListener = null;
  }
  showView('idle');
});

btnCancelAuto.addEventListener('click', () => {
  autoCancelled = true;
  if (msgListener) {
    chrome.runtime.onMessage.removeListener(msgListener);
    msgListener = null;
  }
  showView('idle');
});

btnStop.addEventListener('click', () => {
  currentReader?.cancel();
  currentReader = null;
  showView('idle');
});

btnNew.addEventListener('click', async () => {
  // If result was from cache — force rescan, otherwise go to idle
  if (resultCacheLabel.classList.contains('hidden') === false) {
    // Force fresh scan
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
    if (tab?.url?.startsWith('http')) {
      currentTabUrl = tab.url;
      // Remove cache entry for this URL so it rescans
      const history = await getHistory();
      const filtered = history.filter(h => h.url !== tab.url);
      await chrome.storage.local.set({ history: filtered });
      autoCancelled = false;
      autoMsg.textContent = 'Scanarea paginii...';
      showView('auto');
      startScanSilent(tab);
      return;
    }
  }
  showView('idle');
});

btnHistoryTab.addEventListener('click', () => showHistoryView());

btnBack.addEventListener('click', () => showView('idle'));

btnClearHist.addEventListener('click', async () => {
  await chrome.storage.local.remove('history');
  historyList.innerHTML = '<p class="hint" style="margin-top:16px">Istoricul este gol</p>';
});

// ── Start ────────────────────────────────────────────────────────
init();
