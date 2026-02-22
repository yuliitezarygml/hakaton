const API_BASE   = 'http://localhost:8080';
const API_STREAM = API_BASE + '/api/analyze/stream';
const API_HEALTH = API_BASE + '/api/health';

// DOM refs
const statusDot      = document.getElementById('status-dot');
const viewIdle       = document.getElementById('view-idle');
const viewScanning   = document.getElementById('view-scanning');
const viewAnalyzing  = document.getElementById('view-analyzing');
const viewResult     = document.getElementById('view-result');
const btnAnalyzeUrl  = document.getElementById('btn-analyze-url');
const btnCancelScan  = document.getElementById('btn-cancel-scan');
const btnStop        = document.getElementById('btn-stop');
const btnNew         = document.getElementById('btn-new');
const scanFeed       = document.getElementById('scan-feed');
const scanChars      = document.getElementById('scan-chars');
const scanBarFill    = document.getElementById('scan-bar-fill');
const eventLog       = document.getElementById('event-log');
const resultScore    = document.getElementById('result-score');
const resultVerdict  = document.getElementById('result-verdict');
const resultSummary  = document.getElementById('result-summary');
const resultSections = document.getElementById('result-sections');

let currentReader = null;
let scanCancelled = false;
let msgListener   = null;

// ── Initialization ─────────────────────────────────────────────
async function init() {
  const online = await checkHealth();

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

  showView('idle');
}

// ── Health check ───────────────────────────────────────────────
async function checkHealth() {
  try {
    const resp = await fetch(API_HEALTH, { signal: AbortSignal.timeout(3000) });
    if (resp.ok) {
      statusDot.className = 'status-dot online';
      statusDot.title = 'Бэкенд онлайн (localhost:8080)';
      return true;
    }
  } catch { /* offline */ }

  statusDot.className = 'status-dot offline';
  statusDot.title = 'Бэкенд недоступен';
  btnAnalyzeUrl.disabled = true;
  btnAnalyzeUrl.textContent = '⚠ Запустите go run main.go';
  return false;
}

// ── View switching ─────────────────────────────────────────────
function showView(name) {
  viewIdle.classList.toggle('hidden',      name !== 'idle');
  viewScanning.classList.toggle('hidden',  name !== 'scanning');
  viewAnalyzing.classList.toggle('hidden', name !== 'analyzing');
  viewResult.classList.toggle('hidden',    name !== 'result');
}

// ── Scan feed chunk ────────────────────────────────────────────
function addScanChunk(tag, text) {
  const row = document.createElement('div');
  row.className = 'scan-chunk';

  const tagSpan = document.createElement('span');
  const tagKey  = tag === '§' ? '§' : tag.toUpperCase();
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

// ── Page scan (content script) ─────────────────────────────────
async function startScan(tab) {
  if (!tab?.id) return;

  scanCancelled = false;
  scanFeed.innerHTML = '';
  scanChars.textContent = '0 симв.';
  scanBarFill.style.width = '0%';
  showView('scanning');

  // Remove previous listener if any
  if (msgListener) {
    chrome.runtime.onMessage.removeListener(msgListener);
    msgListener = null;
  }

  // Promise that resolves when scan_done arrives (or rejects on cancel)
  const scanResult = await new Promise((resolve, reject) => {
    msgListener = (msg) => {
      if (scanCancelled) { reject(new Error('cancelled')); return; }

      if (msg.type === 'scan_chunk') {
        addScanChunk(msg.tag, msg.text);
        scanChars.textContent = msg.charCount.toLocaleString('ru') + ' симв.';
        if (msg.total > 0) {
          scanBarFill.style.width = Math.round(((msg.index + 1) / msg.total) * 100) + '%';
        }
      } else if (msg.type === 'scan_done') {
        scanBarFill.style.width = '100%';
        scanChars.textContent = msg.charCount.toLocaleString('ru') + ' симв.';
        chrome.runtime.onMessage.removeListener(msgListener);
        msgListener = null;
        resolve(msg);
      }
    };
    chrome.runtime.onMessage.addListener(msgListener);

    // Inject content script
    chrome.scripting.executeScript({
      target: { tabId: tab.id },
      files:  ['content.js'],
    }).catch(err => {
      chrome.runtime.onMessage.removeListener(msgListener);
      msgListener = null;
      reject(err);
    });
  }).catch(err => {
    if (err.message !== 'cancelled') {
      addEvent('error', 'Не удалось просканировать страницу: ' + err.message);
      showView('analyzing');
    } else {
      showView('idle');
    }
    return null;
  });

  if (!scanResult || scanCancelled) return;

  if (!scanResult.text || scanResult.charCount < 50) {
    // Fallback to URL
    addEvent('progress', '⚠ Мало текста на странице — анализирую по URL...');
    startAnalysis({ url: tab.url });
    return;
  }

  // Show brief transition
  addScanChunk('✓', `Найдено ${scanResult.total} блоков · ${scanResult.charCount.toLocaleString('ru')} символов`);
  await new Promise(r => setTimeout(r, 400));

  // Start SSE analysis with extracted text
  startAnalysis({ text: scanResult.text });
}

// ── SSE streaming ──────────────────────────────────────────────
async function startAnalysis(body) {
  eventLog.innerHTML = '';
  showView('analyzing');

  try {
    const resp = await fetch(API_STREAM, {
      method:  'POST',
      headers: { 'Content-Type': 'application/json' },
      body:    JSON.stringify(body),
    });

    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    if (!resp.body) throw new Error('No response body');

    const reader  = resp.body.getReader();
    currentReader = reader;
    const decoder = new TextDecoder();
    let buffer    = '';
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
            try { renderResult(JSON.parse(eventData)); } catch { /* ignore */ }
          }
          eventType = null;
          eventData = null;
        }
      }
    }
  } catch (err) {
    if (err.name !== 'AbortError') addEvent('error', err.message);
  } finally {
    currentReader = null;
  }
}

// ── Event log row ──────────────────────────────────────────────
function addEvent(type, data) {
  const row   = document.createElement('div');
  row.className = 'event-row';

  const badge = document.createElement('span');
  badge.className = `event-badge badge-${type}`;
  badge.textContent = type;

  const msg   = document.createElement('span');
  msg.className = 'event-msg';
  msg.textContent = type === 'result' ? '📊 Результат получен' : data;

  row.appendChild(badge);
  row.appendChild(msg);
  eventLog.appendChild(row);
  eventLog.scrollTop = eventLog.scrollHeight;
}

// ── Score helpers ──────────────────────────────────────────────
function scoreColor(n) {
  if (n <= 3) return '#ef4444';
  if (n <= 6) return '#eab308';
  return '#22c55e';
}
function verdict(n) {
  if (n <= 3) return '🔴 ВЕРОЯТНАЯ ДЕЗИНФОРМАЦИЯ';
  if (n <= 6) return '🟡 СОМНИТЕЛЬНЫЙ КОНТЕНТ';
  return '🟢 ДОСТОВЕРНЫЙ КОНТЕНТ';
}

// ── Render result ──────────────────────────────────────────────
function renderResult(result) {
  const color = scoreColor(result.credibility_score);
  resultScore.textContent = result.credibility_score + '/10';
  resultScore.style.color = color;
  resultVerdict.textContent = verdict(result.credibility_score);
  resultVerdict.style.color = color;
  resultSummary.textContent = result.summary || '';

  resultSections.innerHTML = '';
  if (result.manipulations?.length)
    addSection('Манипуляции', result.manipulations, '#fbbf24');
  if (result.logical_issues?.length)
    addSection('Логические ошибки', result.logical_issues, '#f97316');
  if (result.fact_check?.missing_evidence?.length)
    addSection('Утверждения без доказательств', result.fact_check.missing_evidence, '#818cf8');
  if (result.fact_check?.opinions_as_facts?.length)
    addSection('Мнения как факты', result.fact_check.opinions_as_facts, '#a78bfa');

  showView('result');
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

// ── Event listeners ────────────────────────────────────────────
btnAnalyzeUrl.addEventListener('click', async () => {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
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

btnStop.addEventListener('click', () => {
  currentReader?.cancel();
  currentReader = null;
  showView('idle');
});

btnNew.addEventListener('click', () => showView('idle'));

// ── Start ──────────────────────────────────────────────────────
init();
