// Text Analyzer — content script (injected on demand)
(function () {
  if (window.__taRunning) return;
  window.__taRunning = true;

  // ── Overlay ────────────────────────────────────────────────────
  const overlay = document.createElement('div');
  overlay.style.cssText = `
    position: fixed; inset: 0;
    background: rgba(10, 10, 20, 0.55);
    z-index: 2147483646;
    pointer-events: none;
    backdrop-filter: blur(1px);
    -webkit-backdrop-filter: blur(1px);
    transition: opacity 0.4s;
  `;

  const scanLine = document.createElement('div');
  scanLine.style.cssText = `
    position: absolute; left: 0; width: 100%; height: 3px;
    background: linear-gradient(90deg,
      transparent 0%,
      rgba(56,189,248,0.3) 10%,
      #38bdf8 40%,
      #7dd3fc 50%,
      #38bdf8 60%,
      rgba(56,189,248,0.3) 90%,
      transparent 100%);
    box-shadow: 0 0 16px 4px rgba(56,189,248,0.7), 0 0 40px 8px rgba(56,189,248,0.2);
    top: 0;
    transition: top 0.15s linear;
  `;
  overlay.appendChild(scanLine);

  // ── Badge (top-right corner) ────────────────────────────────────
  const badge = document.createElement('div');
  badge.style.cssText = `
    position: fixed; top: 18px; right: 18px;
    background: rgba(10,10,20,0.92);
    border: 1px solid rgba(56,189,248,0.6);
    border-radius: 8px;
    padding: 8px 14px 8px 10px;
    color: #38bdf8;
    font: 600 12px/1.4 'Consolas', 'JetBrains Mono', monospace;
    z-index: 2147483647;
    pointer-events: none;
    display: flex; align-items: center; gap: 8px;
    box-shadow: 0 0 20px rgba(56,189,248,0.2);
  `;

  const dot = document.createElement('div');
  dot.style.cssText = `
    width: 7px; height: 7px; border-radius: 50%;
    background: #38bdf8;
    animation: __ta_pulse 1s infinite;
  `;

  const style = document.createElement('style');
  style.textContent = `
    @keyframes __ta_pulse {
      0%, 100% { opacity: 1; transform: scale(1); }
      50%       { opacity: 0.4; transform: scale(0.75); }
    }
    .__ta_scanned {
      outline: 2px solid rgba(56,189,248,0.75) !important;
      box-shadow: 0 0 14px rgba(56,189,248,0.25) !important;
      transition: outline 0.2s, box-shadow 0.2s !important;
    }
    .__ta_done {
      outline: 1px solid rgba(56,189,248,0.2) !important;
      box-shadow: none !important;
    }
  `;
  document.head.appendChild(style);

  const badgeText = document.createElement('span');
  badgeText.textContent = '🔍 Сканирование...';
  badge.appendChild(dot);
  badge.appendChild(badgeText);

  document.body.appendChild(overlay);
  document.body.appendChild(badge);

  // ── Junk selectors to skip ──────────────────────────────────────
  const SKIP = [
    'nav', 'header', 'footer', 'aside',
    'script', 'style', 'noscript', 'iframe', 'form',
    '[role="banner"]', '[role="navigation"]', '[role="complementary"]',
    '[aria-hidden="true"]',
  ];
  const SKIP_CLASS_RX = /\b(ad|ads|advert|banner|sidebar|widget|cookie|gdpr|subscribe|newsletter|social|sharing|comment|promo|popup|modal|overlay|related|recommend|sponsored)\b/i;

  function isJunk(el) {
    if (SKIP.some(s => el.matches(s))) return true;
    if (SKIP.some(s => el.closest(s))) return true;
    const cls = (el.className || '') + ' ' + (el.id || '');
    return SKIP_CLASS_RX.test(cls);
  }

  // ── Collect elements ────────────────────────────────────────────
  const allEls = Array.from(
    document.querySelectorAll('h1, h2, h3, h4, p, blockquote, figcaption')
  ).filter(el => {
    const text = el.innerText?.trim() ?? '';
    if (text.length < 25) return false;
    if (isJunk(el)) return false;
    return true;
  });

  // ── Scan loop ───────────────────────────────────────────────────
  const pageH = Math.max(document.documentElement.scrollHeight, 1);
  let charCount = 0;
  let fullText = '';

  function delay(ms) {
    return new Promise(r => setTimeout(r, ms));
  }

  async function run() {
    for (let i = 0; i < allEls.length; i++) {
      const el = allEls[i];
      const rect = el.getBoundingClientRect();
      const elTop = rect.top + window.scrollY;

      // Move scan line
      const pct = Math.min((elTop / pageH) * 100, 99);
      scanLine.style.top = pct + '%';

      // Highlight element
      el.classList.add('__ta_scanned');

      const text = el.innerText?.trim() ?? '';
      const tag = el.tagName.startsWith('H') ? el.tagName : '§';
      fullText += text + '\n\n';
      charCount += text.length;

      badgeText.textContent = `🔍 ${charCount.toLocaleString('ru')} симв.`;

      // Send chunk to popup
      try {
        chrome.runtime.sendMessage({
          type: 'scan_chunk',
          tag,
          text: text.slice(0, 140),
          charCount,
          index: i,
          total: allEls.length,
        });
      } catch (_) { /* popup may have closed */ }

      await delay(90);

      // Transition to "done" style
      el.classList.remove('__ta_scanned');
      el.classList.add('__ta_done');
    }

    // Scan line to bottom
    scanLine.style.top = '100%';
    badgeText.textContent = `✓ ${charCount.toLocaleString('ru')} симв. найдено`;

    // Send done
    try {
      chrome.runtime.sendMessage({
        type: 'scan_done',
        text: fullText.trim().slice(0, 30000),
        charCount,
        total: allEls.length,
      });
    } catch (_) { /* ignore */ }

    // Clean up after short delay
    await delay(900);
    overlay.style.opacity = '0';
    badge.style.opacity = '0';
    await delay(400);
    overlay.remove();
    badge.remove();
    style.remove();
    allEls.forEach(el => el.classList.remove('__ta_scanned', '__ta_done'));
    window.__taRunning = false;
  }

  run();
})();
