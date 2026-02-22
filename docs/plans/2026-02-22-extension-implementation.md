# Browser Extension Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a Chrome Manifest V3 extension that lets users analyze any webpage (by URL) or selected text (via context menu) for disinformation using the existing Go backend at `localhost:8080`.

**Architecture:** Vanilla JS + MV3 with no build step. `background.js` (service worker) handles context menu registration and opens a popup window when text is selected. `popup.html/js/css` handles all UI states (idle → analyzing → result) and streams SSE directly from the Go backend.

**Tech Stack:** Chrome Extension MV3, Vanilla JS, SSE via fetch+ReadableStream (same pattern as main frontend's `useAnalyzer.js`)

---

### Task 1: Create extension directory and manifest.json

**Files:**
- Create: `extension/manifest.json`

**Step 1: Create the directory**

```bash
mkdir -p D:/project/openrouter-web/extension
```

**Step 2: Write manifest.json**

Create `extension/manifest.json`:

```json
{
  "manifest_version": 3,
  "name": "Text Analyzer",
  "version": "1.0.0",
  "description": "Анализ текстов и статей на дезинформацию, манипуляции и логические ошибки",
  "permissions": ["contextMenus", "storage", "activeTab"],
  "host_permissions": ["http://localhost:8080/*"],
  "background": {
    "service_worker": "background.js"
  },
  "action": {
    "default_popup": "popup.html",
    "default_title": "Text Analyzer"
  }
}
```

**Step 3: Verify**

Open Chrome → `chrome://extensions/` → Enable Developer mode → Load unpacked → select `extension/` folder.

Expected: Extension appears in the list. The icon is a grey puzzle piece (no custom icon yet — that's fine).

---

### Task 2: Create background.js (service worker)

**Files:**
- Create: `extension/background.js`

**Step 1: Write background.js**

```js
const CONTEXT_MENU_ID = 'analyze-selection';

// Register context menu item once on install
chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.create({
    id: CONTEXT_MENU_ID,
    title: 'Проанализировать: "%s"',
    contexts: ['selection'],
  });
});

// Handle context menu click
chrome.contextMenus.onClicked.addListener(async (info) => {
  if (info.menuItemId !== CONTEXT_MENU_ID) return;

  const text = info.selectionText?.trim();
  if (!text) return;

  // Store text so popup can pick it up
  await chrome.storage.session.set({ pendingText: text });

  // Open popup.html as a small popup window
  chrome.windows.create({
    url: chrome.runtime.getURL('popup.html') + '?autostart=1',
    type: 'popup',
    width: 440,
    height: 600,
    focused: true,
  });
});
```

**Step 2: Reload extension in Chrome**

In `chrome://extensions/` click the reload (↺) icon on the extension card.

**Step 3: Verify**

Go to any webpage → select some text → right-click.
Expected: "Проанализировать: «выделенный текст»" appears in the context menu.

---

### Task 3: Create popup.html

**Files:**
- Create: `extension/popup.html`

**Step 1: Write popup.html**

```html
<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8">
  <title>Text Analyzer</title>
  <link rel="stylesheet" href="popup.css">
</head>
<body>
  <div id="app">

    <header>
      <span class="title">🔍 Text Analyzer</span>
      <span id="status-dot" class="status-dot" title="Статус бэкенда"></span>
    </header>

    <!-- State: idle -->
    <div id="view-idle" class="view hidden">
      <button id="btn-analyze-url" class="btn-primary">
        Анализировать текущую страницу
      </button>
      <p class="hint">
        Или выделите текст на любой странице<br>
        → правый клик → Проанализировать
      </p>
    </div>

    <!-- State: analyzing (SSE streaming) -->
    <div id="view-analyzing" class="view hidden">
      <div id="event-log"></div>
      <button id="btn-stop" class="btn-secondary">✕ Остановить</button>
    </div>

    <!-- State: result -->
    <div id="view-result" class="view hidden">
      <div id="result-header">
        <span id="result-score"></span>
        <span id="result-verdict"></span>
      </div>
      <p id="result-summary"></p>
      <div id="result-sections"></div>
      <button id="btn-new" class="btn-secondary">↺ Новый анализ</button>
    </div>

  </div>
  <script src="popup.js"></script>
</body>
</html>
```

---

### Task 4: Create popup.css

**Files:**
- Create: `extension/popup.css`

**Step 1: Write popup.css**

```css
* { box-sizing: border-box; margin: 0; padding: 0; }

body {
  width: 420px;
  min-height: 120px;
  max-height: 580px;
  background: #0d0d0d;
  color: #e5e5e5;
  font-family: 'Inter', system-ui, -apple-system, sans-serif;
  font-size: 13px;
  overflow-y: auto;
}

/* ── Header ── */
header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px 12px;
  border-bottom: 1px solid #1e1e1e;
}
.title { font-size: 14px; font-weight: 600; color: #f0f0f0; }
.status-dot {
  width: 8px; height: 8px;
  border-radius: 50%;
  background: #3a3a3a;
  transition: background 0.3s;
}
.status-dot.online  { background: #22c55e; }
.status-dot.offline { background: #ef4444; }

/* ── Views ── */
.view { padding: 16px; }
.hidden { display: none !important; }

/* ── Buttons ── */
.btn-primary {
  width: 100%;
  padding: 10px;
  background: #0f2235;
  border: 1px solid #1a3a55;
  border-radius: 6px;
  color: #7dd3fc;
  font-size: 13px;
  cursor: pointer;
  transition: background 0.15s;
}
.btn-primary:hover:not(:disabled) { background: #152c45; }
.btn-primary:disabled { opacity: 0.45; cursor: not-allowed; }

.btn-secondary {
  padding: 7px 14px;
  background: #1a1a1a;
  border: 1px solid #2e2e2e;
  border-radius: 6px;
  color: #888;
  font-size: 12px;
  cursor: pointer;
  transition: background 0.15s;
  margin-top: 10px;
}
.btn-secondary:hover { background: #222; color: #aaa; }

.hint {
  margin-top: 14px;
  color: #444;
  font-size: 11px;
  text-align: center;
  line-height: 1.6;
}

/* ── Event log ── */
#event-log {
  max-height: 400px;
  overflow-y: auto;
  background: #0a0a0a;
  border: 1px solid #1e1e1e;
  border-radius: 6px;
  display: flex;
  flex-direction: column;
}

.event-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 6px 10px;
  border-bottom: 1px solid #141414;
  font-family: 'JetBrains Mono', 'Fira Code', 'Consolas', monospace;
  font-size: 11px;
}
.event-row:last-child { border-bottom: none; }

.event-badge {
  flex-shrink: 0;
  padding: 1px 6px;
  border-radius: 3px;
  font-size: 10px;
  font-weight: 500;
  text-transform: lowercase;
}

.badge-start    { color: #38bdf8; background: rgba(56,189,248,.08); border: 1px solid rgba(56,189,248,.18); }
.badge-progress { color: #34d399; background: rgba(52,211,153,.08); border: 1px solid rgba(52,211,153,.18); }
.badge-result   { color: #a78bfa; background: rgba(167,139,250,.08); border: 1px solid rgba(167,139,250,.18); }
.badge-done     { color: #fb923c; background: rgba(251,146,60,.08);  border: 1px solid rgba(251,146,60,.18); }
.badge-error    { color: #f87171; background: rgba(248,113,113,.08); border: 1px solid rgba(248,113,113,.18); }

.event-msg {
  flex: 1;
  color: #ccc;
  line-height: 1.4;
  word-break: break-word;
}

/* ── Result ── */
#result-header {
  display: flex;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 14px;
  padding-bottom: 12px;
  border-bottom: 1px solid #1e1e1e;
}
#result-score {
  font-size: 30px;
  font-weight: bold;
  font-family: monospace;
  line-height: 1;
}
#result-verdict { font-size: 13px; font-weight: 600; }

#result-summary {
  color: #999;
  font-size: 12px;
  line-height: 1.6;
  margin-bottom: 14px;
}

.result-section { margin-bottom: 14px; }
.section-title  { font-size: 11px; font-weight: 600; margin-bottom: 6px; text-transform: uppercase; letter-spacing: .04em; }
.section-list   { list-style: none; display: flex; flex-direction: column; gap: 4px; }
.section-list li {
  font-size: 11.5px;
  color: #bbb;
  padding-left: 12px;
  position: relative;
  line-height: 1.5;
}
.section-list li::before { content: '•'; position: absolute; left: 0; color: #555; }
```

---

### Task 5: Create popup.js (main logic)

**Files:**
- Create: `extension/popup.js`

**Step 1: Write popup.js**

```js
const API_BASE   = 'http://localhost:8080';
const API_STREAM = API_BASE + '/api/analyze/stream';
const API_HEALTH = API_BASE + '/api/health';

// DOM refs
const statusDot    = document.getElementById('status-dot');
const viewIdle     = document.getElementById('view-idle');
const viewAnalyzing = document.getElementById('view-analyzing');
const viewResult   = document.getElementById('view-result');
const btnAnalyzeUrl = document.getElementById('btn-analyze-url');
const btnStop      = document.getElementById('btn-stop');
const btnNew       = document.getElementById('btn-new');
const eventLog     = document.getElementById('event-log');
const resultScore  = document.getElementById('result-score');
const resultVerdict = document.getElementById('result-verdict');
const resultSummary = document.getElementById('result-summary');
const resultSections = document.getElementById('result-sections');

let currentReader = null;

// ── Initialization ─────────────────────────────────────────────
async function init() {
  const online = await checkHealth();

  // Context menu flow: popup opened with ?autostart=1
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
  viewIdle.classList.toggle('hidden', name !== 'idle');
  viewAnalyzing.classList.toggle('hidden', name !== 'analyzing');
  viewResult.classList.toggle('hidden', name !== 'result');
}

// ── Event log row ──────────────────────────────────────────────
function addEvent(type, data) {
  const row = document.createElement('div');
  row.className = 'event-row';

  const badge = document.createElement('span');
  badge.className = `event-badge badge-${type}`;
  badge.textContent = type;

  const msg = document.createElement('span');
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

  if (result.manipulations?.length) {
    addSection('Манипуляции', result.manipulations, '#fbbf24');
  }
  if (result.logical_issues?.length) {
    addSection('Логические ошибки', result.logical_issues, '#f97316');
  }
  if (result.fact_check?.missing_evidence?.length) {
    addSection('Утверждения без доказательств', result.fact_check.missing_evidence, '#818cf8');
  }
  if (result.fact_check?.opinions_as_facts?.length) {
    addSection('Мнения как факты', result.fact_check.opinions_as_facts, '#a78bfa');
  }

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

// ── SSE streaming ──────────────────────────────────────────────
async function startAnalysis(body) {
  eventLog.innerHTML = '';
  showView('analyzing');

  try {
    const resp = await fetch(API_STREAM, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
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
            try { renderResult(JSON.parse(eventData)); } catch { /* ignore */ }
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

// ── Event listeners ────────────────────────────────────────────
btnAnalyzeUrl.addEventListener('click', async () => {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (tab?.url) {
    startAnalysis({ url: tab.url });
  }
});

btnStop.addEventListener('click', () => {
  currentReader?.cancel();
  currentReader = null;
  showView('idle');
});

btnNew.addEventListener('click', () => showView('idle'));

// ── Start ──────────────────────────────────────────────────────
init();
```

**Step 2: Reload extension in Chrome**

In `chrome://extensions/` reload the extension.

---

### Task 6: End-to-end manual test — URL analysis

**Precondition:** Go backend is running (`go run main.go` in `D:/project/openrouter-web`)

**Step 1: Open the popup**

Click the extension icon in Chrome toolbar while on any news article page.

**Expected (idle state):**
- Green dot in header
- "Анализировать текущую страницу" button is enabled

**Step 2: Click "Анализировать текущую страницу"**

**Expected (analyzing state):**
- Event log shows `start` → `progress` rows appearing one by one
- Badge colors match: start=blue, progress=green

**Step 3: Wait for result**

**Expected (result state):**
- Score shown as `N/10` in red/yellow/green
- Verdict text below score
- Summary paragraph
- Sections for manipulations, logical errors if present

---

### Task 7: End-to-end manual test — context menu

**Step 1:** Go to any webpage, select 2-3 sentences of text.

**Step 2:** Right-click → "Проанализировать: «...»"

**Expected:** A small popup window opens (420×600px) and immediately starts analyzing. Event log shows progress.

**Step 3:** Wait for result.

**Expected:** Same result view as URL analysis.

---

### Task 8: Error state test

**Step 1:** Stop the Go backend.

**Step 2:** Click the extension icon.

**Expected:**
- Red dot in header
- Button text becomes "⚠ Запустите go run main.go"
- Button is disabled

**Step 3:** Restart the backend, click the extension icon again.

**Expected:** Green dot, button enabled. *(Note: popup must be reopened — no auto-reconnect.)*

---

### Task 9 (Optional): Add extension icons

**Files:**
- Create: `extension/generate-icons.js`
- Create: `extension/icons/` (directory)

**Step 1: Write icon generator**

Create `extension/generate-icons.js`:

```js
// Run with: node generate-icons.js
// Requires: npm install canvas
const { createCanvas } = require('canvas');
const fs = require('fs');

fs.mkdirSync('icons', { recursive: true });

for (const size of [16, 48, 128]) {
  const canvas = createCanvas(size, size);
  const ctx = canvas.getContext('2d');

  // Background
  ctx.fillStyle = '#0d0d0d';
  ctx.beginPath();
  ctx.roundRect(0, 0, size, size, size * 0.2);
  ctx.fill();

  // Magnifying glass icon
  const cx = size * 0.42, cy = size * 0.42, r = size * 0.25;
  ctx.strokeStyle = '#38bdf8';
  ctx.lineWidth = size * 0.1;
  ctx.beginPath();
  ctx.arc(cx, cy, r, 0, Math.PI * 2);
  ctx.stroke();

  // Handle
  ctx.beginPath();
  ctx.moveTo(cx + r * 0.7, cy + r * 0.7);
  ctx.lineTo(size * 0.85, size * 0.85);
  ctx.stroke();

  fs.writeFileSync(`icons/icon${size}.png`, canvas.toBuffer('image/png'));
  console.log(`✓ icon${size}.png`);
}
```

**Step 2: Run**

```bash
cd extension
npm init -y
npm install canvas
node generate-icons.js
```

**Step 3: Add icons to manifest.json**

Add to `extension/manifest.json`:

```json
"icons": {
  "16": "icons/icon16.png",
  "48": "icons/icon48.png",
  "128": "icons/icon128.png"
},
"action": {
  "default_popup": "popup.html",
  "default_title": "Text Analyzer",
  "default_icon": {
    "16": "icons/icon16.png",
    "48": "icons/icon48.png",
    "128": "icons/icon128.png"
  }
}
```

**Step 4:** Reload extension. The grey puzzle piece is replaced with the custom icon.
