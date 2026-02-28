// Text Analyzer — content script (injected on demand)
(function () {
  if (window.__taRunning) return;
  window.__taRunning = true;

  // ── Facebook detection ──────────────────────────────────────────
  const onFacebook = /(?:^|\.)facebook\.com$/.test(location.hostname);

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
    transition: opacity 0.4s;
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

  // ── Junk selectors (non-Facebook pages) ────────────────────────
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
  let collected;

  if (onFacebook) {
    // Facebook uses div[dir="auto"] / span[dir="auto"] for all user-generated text.
    // We gather them inside [role="article"] (feed posts / single post page).
    // Deduplication: if element A is a descendant of element B (both selected),
    // keep only B (the outermost container with the full text).
    let candidates = Array.from(
      document.querySelectorAll('[role="article"] div[dir="auto"], [role="article"] span[dir="auto"]')
    ).filter(el => {
      const text = el.innerText?.trim() ?? '';
      if (text.length < 30) return false;
      // Skip navigation / chrome UI inside articles
      if (el.closest('nav, [role="navigation"], [role="banner"]')) return false;
      return true;
    });

    // Keep only outermost elements (drop descendants if ancestor already selected)
    candidates = candidates.filter(el =>
      !candidates.some(other => other !== el && other.contains(el))
    );

    // If no [role="article"] found (e.g. profile page), fall back to any dir="auto" divs
    if (candidates.length === 0) {
      candidates = Array.from(
        document.querySelectorAll('div[dir="auto"]')
      ).filter(el => {
        const text = el.innerText?.trim() ?? '';
        return text.length >= 40 &&
          !el.closest('nav, header, footer, [role="navigation"], [role="banner"]');
      });
      candidates = candidates.filter(el =>
        !candidates.some(other => other !== el && other.contains(el))
      );
    }

    const pageH = Math.max(document.documentElement.scrollHeight, 1);
    collected = candidates.map((el, idx) => {
      const rect = el.getBoundingClientRect();
      const top = (rect.top + window.scrollY) / pageH;
      const text = el.innerText?.trim() ?? '';
      return {
        el,
        top,
        tag: '§',
        text: text.slice(0, 140),
        len: text.length,
      };
    }).sort((a, b) => a.top - b.top);

  } else {
    // ── Standard pages ─────────────────────────────────────────────
    const pageH = Math.max(document.documentElement.scrollHeight, 1);
    collected = Array.from(
      document.querySelectorAll('h1,h2,h3,h4,p,blockquote,figcaption')
    ).filter(el => {
      const text = el.innerText?.trim() ?? '';
      return text.length >= 25 && !isJunk(el);
    }).map(el => {
      const rect = el.getBoundingClientRect();
      const top = (rect.top + window.scrollY) / pageH;
      return {
        el,
        top,
        tag: el.tagName.startsWith('H') ? el.tagName : '§',
        text: (el.innerText?.trim() ?? '').slice(0, 140),
        len:  (el.innerText?.trim() ?? '').length,
      };
    }).sort((a, b) => a.top - b.top);
  }

  const elData = collected;

  // ── Smooth sweep animation ──────────────────────────────────────
  const pageH = Math.max(document.documentElement.scrollHeight, 1);
  const SCAN_DURATION = 3000; // ms for full top→bottom sweep
  const SCAN_LINE_VH  = 0.35; // scan line stays at 35% of viewport height
  const savedScrollY  = window.scrollY;
  const startTime     = performance.now();
  let nextIdx   = 0;
  let charCount = 0;
  let fullText  = '';
  let rafId;

  // Scan line always at fixed viewport position; page scrolls beneath it
  scanLine.style.top = (SCAN_LINE_VH * 100) + '%';

  function tick(now) {
    const t = Math.min((now - startTime) / SCAN_DURATION, 1); // 0..1

    // Logical scan position on the full page (pixels)
    const scanPageY = t * pageH;

    // Scroll so that scan line aligns with current scan position
    const targetScroll = Math.max(0, scanPageY - window.innerHeight * SCAN_LINE_VH);
    window.scrollTo({ top: targetScroll, behavior: 'instant' });

    badgeText.textContent = `🔍 ${Math.round(t * 100)}%`;

    // Highlight elements whose page-Y is now behind the scan line
    while (nextIdx < elData.length && elData[nextIdx].top * pageH <= scanPageY) {
      const d = elData[nextIdx];
      d.el.classList.add('__ta_scanned');
      fullText  += (d.el.innerText?.trim() ?? '') + '\n\n';
      charCount += d.len;

      const idx = nextIdx;
      setTimeout(() => {
        d.el.classList.remove('__ta_scanned');
        d.el.classList.add('__ta_done');
      }, 300);

      try {
        chrome.runtime.sendMessage({
          type:      'scan_chunk',
          tag:       d.tag,
          text:      d.text,
          charCount,
          index:     idx,
          total:     elData.length,
        });
      } catch (_) { /* popup may have closed */ }

      nextIdx++;
    }

    if (t < 1) {
      rafId = requestAnimationFrame(tick);
    } else {
      // ── Sweep complete ──────────────────────────────────────────
      scanLine.style.top = '100%';
      badgeText.textContent = `✓ ${charCount.toLocaleString('ru')} симв. найдено`;

      try {
        chrome.runtime.sendMessage({
          type:      'scan_done',
          text:      fullText.trim().slice(0, 30000),
          charCount,
          total:     elData.length,
        });
      } catch (_) { /* ignore */ }

      // Restore scroll position, fade out overlay & badge
      window.scrollTo({ top: savedScrollY, behavior: 'smooth' });
      setTimeout(() => {
        overlay.style.opacity = '0';
        badge.style.opacity   = '0';
        setTimeout(() => {
          overlay.remove();
          badge.remove();
          style.remove();
          elData.forEach(d => d.el.classList.remove('__ta_scanned', '__ta_done'));
          window.__taRunning = false;
        }, 400);
      }, 800);
    }
  }

  requestAnimationFrame(tick);
})();
