const CONTEXT_MENU_ID = 'analyze-selection';

// ── Dev auto-reload ─────────────────────────────────────────────
// Polls /api/ext/hash every 3s; reloads extension when files change.
// Silently does nothing if backend is unreachable (production / offline).
let _extHash = null;
async function pollExtHash() {
  try {
    const r = await fetch('https://apich.sinkdev.dev/api/ext/hash', { signal: AbortSignal.timeout(2000) });
    if (!r.ok) return;
    const { hash } = await r.json();
    if (_extHash === null) { _extHash = hash; return; }
    if (hash !== _extHash) { chrome.runtime.reload(); }
  } catch { /* offline or backend down — ignore */ }
}
setInterval(pollExtHash, 3000);

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
