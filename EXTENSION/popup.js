const API_BASE   = 'https://apich.sinkdev.dev';
const API_STREAM = API_BASE + '/api/analyze/stream';
const API_HEALTH = API_BASE + '/api/health';

// DOM refs
const statusDot      = document.getElementById('status-dot');
const viewIdle       = document.getElementById('view-idle');
const viewScanning   = document.getElementById('view-scanning');
const viewAnalyzing  = document.getElementById('view-analyzing');
const viewResult     = document.getElementById('view-result');
const viewChain      = document.getElementById('view-chain');
const chainNodes     = document.getElementById('chain-nodes');
const chainStatus    = document.getElementById('chain-status');
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
let chainReader   = null;

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
      statusDot.title = 'Бэкенд онлайн (apich.sinkdev.dev)';
      return true;
    }
  } catch { /* offline */ }

  statusDot.className = 'status-dot offline';
  statusDot.title = 'Бэкенд недоступен';
  btnAnalyzeUrl.disabled = true;
  btnAnalyzeUrl.textContent = '⚠ Бэкенд недоступен';
  return false;
}

// ── View switching ─────────────────────────────────────────────
function showView(name) {
  viewIdle.classList.toggle('hidden',      name !== 'idle');
  viewScanning.classList.toggle('hidden',  name !== 'scanning');
  viewAnalyzing.classList.toggle('hidden', name !== 'analyzing');
  viewResult.classList.toggle('hidden',    name !== 'result');
  viewChain.classList.toggle('hidden',     name !== 'chain');
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

  // Facebook can't be fetched server-side (login wall) — use text extracted by content.js.
  // For all other sites send the URL so the backend can fetch the full article content.
  const isFacebook = /(?:^|\.)facebook\.com$/.test(new URL(tab.url).hostname);
  if (isFacebook && scanResult.text && scanResult.text.length >= 30) {
    addScanChunk('✓', `Facebook: использую текст со страницы (${scanResult.charCount?.toLocaleString('ru')} симв.)`);
    await new Promise(r => setTimeout(r, 400));
    startAnalysis({ text: scanResult.text });
  } else {
    addScanChunk('✓', `Сканирование завершено · запускаю анализ по URL...`);
    await new Promise(r => setTimeout(r, 400));
    startAnalysis({ url: tab.url });
  }
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
  resultSections.dataset.result = JSON.stringify(result);
  
  if (result.fact_check?.found_evidence?.length)
    addSection('✅ Найдены доказательства', result.fact_check.found_evidence, '#4ade80');
  
  if (result.manipulations?.length)
    addSection('Манипуляции', result.manipulations, '#fbbf24');
  if (result.logical_issues?.length)
    addSection('Логические ошибки', result.logical_issues, '#f97316');
  if (result.fact_check?.missing_evidence?.length)
    addSection('Утверждения без доказательств', result.fact_check.missing_evidence, '#818cf8');
  if (result.fact_check?.opinions_as_facts?.length)
    addSection('Мнения как факты', result.fact_check.opinions_as_facts, '#a78bfa');

  addEvidenceSection(result);

  const sources = extractSources(result);
  if (sources.length) addSourcesSection(sources);

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

// ── Source chain ───────────────────────────────────────────────
async function startChain(url) {
  chainNodes.innerHTML = '';
  chainStatus.textContent = '🔍 Инициализация...';
  showView('chain');

  try {
    const resp = await fetch(API_STREAM.replace('/analyze/stream', '/chain/stream'), {
      method:  'POST',
      headers: { 'Content-Type': 'application/json' },
      body:    JSON.stringify({ url }),
    });
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    if (!resp.body) throw new Error('No response body');

    const reader  = resp.body.getReader();
    chainReader   = reader;
    const decoder = new TextDecoder();
    let buffer    = '';
    let eventType = null;

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() ?? '';
      for (const line of lines) {
        if (line.startsWith('event:')) {
          eventType = line.slice(6).trim();
        } else if (line.startsWith('data:') && eventType) {
          try {
            handleChainEvent(eventType, JSON.parse(line.slice(5).trim()));
          } catch { /* ignore parse errors */ }
          eventType = null;
        }
      }
    }
  } catch (err) {
    if (err.name !== 'AbortError') {
      chainStatus.textContent = '❌ ' + err.message;
    }
  } finally {
    chainReader = null;
  }
}

function handleChainEvent(type, ev) {
  switch (type) {
    case 'chain_start':
    case 'chain_progress':
      chainStatus.textContent = ev.message ?? '';
      break;
    case 'chain_node':
      if (ev.node) renderChainNode(ev.node);
      break;
    case 'chain_done':
      chainStatus.textContent = ev.message ?? '✅ Готово';
      break;
    case 'chain_error':
      chainStatus.textContent = '❌ ' + (ev.message || 'Ошибка');
      break;
  }
}

function renderChainNode(node) {
  const div = document.createElement('div');
  div.className = 'chain-node' + (node.is_original ? ' is-original' : '');

  const ds = node.distortion_score ?? 0;
  const scoreClass = ds <= 2 ? 'low' : ds <= 5 ? 'med' : 'high';

  const scoreBadge = node.is_original
    ? '<span class="chain-badge-original">ОРИГИНАЛ</span>'
    : `<span class="chain-score ${scoreClass}">${ds > 0 ? '+' + ds : '✓'} иск.</span>`;

  let distHTML = '';
  if (node.distortions?.length) {
    distHTML = '<div class="chain-distortions">' +
      node.distortions.slice(0, 4).map(d => `
        <div class="chain-distortion">
          <span class="chain-dist-type ${d.type ?? ''}">${distLabel(d.type)}</span>
          <span class="chain-dist-text">
            <span class="chain-dist-orig">${esc(d.original)}</span>
            <span class="chain-dist-arrow">→</span>
            <span class="chain-dist-new">${esc(d.changed)}</span>
          </span>
        </div>`).join('') +
      '</div>';
  }

  div.innerHTML = `
    <div class="chain-node-header">
      <span class="chain-domain">${esc(node.domain)}</span>
      ${scoreBadge}
    </div>
    ${node.title ? `<div class="chain-node-title">${esc(node.title)}</div>` : ''}
    ${node.summary ? `<div class="chain-node-summary">${esc(node.summary)}</div>` : ''}
    ${distHTML}
  `;
  chainNodes.appendChild(div);
  chainNodes.scrollTop = chainNodes.scrollHeight;
}

function distLabel(type) {
  const labels = { exaggeration: 'ПРЕУВ.', omission: 'УМОЛЧ.', addition: 'ДОБАВЛ.', change: 'ИЗМЕНЁН' };
  return labels[type] ?? (type ?? '').toUpperCase().slice(0, 6);
}

function esc(s) {
  return (s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// ── Event listeners ────────────────────────────────────────────
document.getElementById('btn-chain')?.addEventListener('click', async () => {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (tab?.url) startChain(tab.url);
  else chainStatus && (chainStatus.textContent = '❌ Не удалось получить URL вкладки');
});

document.getElementById('btn-cancel-chain')?.addEventListener('click', () => {
  chainReader?.cancel();
  chainReader = null;
  showView('idle');
});

document.getElementById('btn-chain-new')?.addEventListener('click', () => showView('idle'));

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

// ── Evidence section ───────────────────────────────────────────
function addEvidenceSection(result) {
  const hasBreakdown  = result.score_breakdown?.trim();
  const hasReasoning  = result.reasoning?.trim();
  const hasVerdict    = result.verdict_explanation?.trim();
  const fakeReasons   = result.verification?.fake_reasons || [];
  const hasReal       = result.verification?.real_information?.trim();

  if (!hasBreakdown && !hasReasoning && !hasVerdict && !fakeReasons.length && !hasReal) return;

  const section = document.createElement('div');
  section.className = 'result-section';

  const h = document.createElement('div');
  h.className = 'section-title';
  h.style.color = '#f97316';
  h.textContent = '🔬 Доказательства';
  section.appendChild(h);

  const body = document.createElement('div');
  body.className = 'evidence-body';

  // Score breakdown — how the score was calculated
  if (hasBreakdown) {
    const block = document.createElement('div');
    block.className = 'evidence-block';
    block.innerHTML = `<div class="evidence-label">Расчёт оценки</div><div class="evidence-text">${esc(result.score_breakdown)}</div>`;
    body.appendChild(block);
  }

  // Reasoning — AI's detailed chain of thought
  if (hasReasoning) {
    const block = document.createElement('div');
    block.className = 'evidence-block';
    block.innerHTML = `<div class="evidence-label">Обоснование</div><div class="evidence-text">${esc(result.reasoning)}</div>`;
    body.appendChild(block);
  }

  // Verdict explanation
  if (hasVerdict) {
    const block = document.createElement('div');
    block.className = 'evidence-block';
    block.innerHTML = `<div class="evidence-label">Вердикт</div><div class="evidence-text">${esc(result.verdict_explanation)}</div>`;
    body.appendChild(block);
  }

  // Fake reasons list
  if (fakeReasons.length) {
    const block = document.createElement('div');
    block.className = 'evidence-block';
    const items = fakeReasons.map(r => `<div class="evidence-reason">⚠ ${esc(r)}</div>`).join('');
    block.innerHTML = `<div class="evidence-label">Признаки проблем</div>${items}`;
    body.appendChild(block);
  }

  // What the internet says (real_information)
  if (hasReal) {
    const block = document.createElement('div');
    block.className = 'evidence-block';
    block.innerHTML = `<div class="evidence-label">Что нашли в сети</div><div class="evidence-text evidence-real">${esc(result.verification.real_information)}</div>`;
    body.appendChild(block);
  }

  section.appendChild(body);
  resultSections.appendChild(section);
}

// ── Sources helpers ────────────────────────────────────────────
function extractSources(result) {
  const raw = result.verification?.verified_sources || result.sources || [];
  if (!Array.isArray(raw)) return [];
  return raw.map(s => {
    if (typeof s === 'string') return { title: s, url: s, description: '' };
    return { title: s.title || '', url: s.url || '', description: s.description || '' };
  }).filter(s => {
    try { new URL(s.url); return true; } catch { return false; }
  });
}

function addSourcesSection(sources) {
  const section = document.createElement('div');
  section.className = 'result-section';

  const h = document.createElement('div');
  h.className = 'section-title';
  h.style.color = '#38bdf8';
  h.textContent = '📎 Источники проверки';
  section.appendChild(h);

  const list = document.createElement('div');
  list.className = 'source-list';

  sources.slice(0, 9).forEach(s => {
    const item = document.createElement('div');
    item.className = 'source-item';

    // Domain badge
    try {
      const domain = new URL(s.url).hostname.replace(/^www\./, '');
      const badge = document.createElement('span');
      badge.className = 'source-domain';
      badge.textContent = domain;
      item.appendChild(badge);
    } catch {}

    // Title link
    const link = document.createElement('a');
    link.className = 'source-link';
    link.href = s.url;
    link.target = '_blank';
    link.rel = 'noopener noreferrer';
    link.textContent = s.title || s.url;
    item.appendChild(link);

    // Evidence snippet
    if (s.description) {
      const desc = document.createElement('div');
      desc.className = 'source-desc';
      desc.textContent = s.description;
      item.appendChild(desc);
    }

    list.appendChild(item);
  });

  section.appendChild(list);
  resultSections.appendChild(section);
}

// ── Start ──────────────────────────────────────────────────────
init();
