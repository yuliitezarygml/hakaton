# Browser Extension Design — Text Analyzer

**Date:** 2026-02-22
**Status:** Approved

## Overview

A Chrome/Firefox browser extension (Manifest V3) that lets users analyze any webpage or selected text for disinformation, manipulations, and logical errors using the existing Go backend at `localhost:8080`.

## Architecture

**Approach:** Vanilla JS + MV3 (no build step, no framework)

**Backend dependency:** Go backend must be running at `http://localhost:8080`

## File Structure

```
extension/
├── manifest.json          # MV3 manifest
├── background.js          # Service worker — context menu registration, message routing
├── content.js             # Injected into pages — extracts selected text
├── popup.html             # Popup shell
├── popup.js               # Popup logic — SSE streaming, UI state machine
├── popup.css              # Dark theme styles (matches main frontend)
└── icons/
    ├── icon16.png
    ├── icon48.png
    └── icon128.png
```

## User Flows

### Flow 1: Analyze current page by URL
1. User clicks extension icon in toolbar
2. Popup opens in "idle" state
3. User clicks "Анализировать страницу"
4. `popup.js` sends message to `background.js` with current tab URL
5. `background.js` (or `popup.js` directly) POSTs to `POST /api/analyze/stream` with `{ url: "..." }`
6. SSE events stream into popup — progress log updates in real time
7. On `result` event — show ResultCard UI
8. On `done`/`error` — finalize state

### Flow 2: Analyze selected text (context menu)
1. User selects text on any page
2. Right-click → "Проанализировать текст" in context menu
3. `background.js` captures selected text via `chrome.contextMenus` API
4. Saves text to `chrome.storage.session`
5. Opens/focuses popup
6. `popup.js` on open checks `chrome.storage.session` for pending text
7. Auto-starts analysis with `{ text: "..." }`

## Popup UI States

### Idle
```
┌─────────────────────────────────────┐
│ 🔍 Text Analyzer          ● online  │
│─────────────────────────────────────│
│                                     │
│  [  Анализировать страницу  ]       │
│                                     │
│  Или выделите текст на странице     │
│  и нажмите ПКМ → Проанализировать  │
└─────────────────────────────────────┘
```

### Analyzing (SSE streaming)
```
┌─────────────────────────────────────┐
│ 🔍 Text Analyzer         ⏳ анализ  │
│─────────────────────────────────────│
│ ↓ start    🚀 Начинаю анализ...    │
│ ↓ progress 📝 ШАГ 1/4 — Получен..  │
│ ↓ progress 🤖 ШАГ 3/4 — Отправл.. │
│                                     │
│  [ ✕ Стоп ]                        │
└─────────────────────────────────────┘
```

### Result
```
┌─────────────────────────────────────┐
│ 🔍 Text Analyzer                    │
│─────────────────────────────────────│
│  3/10  🔴 ВЕРОЯТНАЯ ДЕЗИНФОРМАЦИЯ  │
│                                     │
│  Краткое резюме текста...           │
│                                     │
│  ▸ Манипуляции (3)                 │
│  ▸ Логические ошибки (2)           │
│  ▸ Фактчек                         │
│                                     │
│  [ Новый анализ ]                   │
└─────────────────────────────────────┘
```

## Popup Dimensions

- Width: `420px`
- Height: `580px` (scrollable content)
- Theme: Dark (`#0d0d0d` background, matches main frontend)

## API Usage

Extension calls the **existing** backend endpoints unchanged:
- `POST http://localhost:8080/api/analyze/stream` — SSE streaming (main flow)
- `GET http://localhost:8080/api/health` — check backend status on popup open

## Permissions (manifest.json)

```json
"permissions": ["contextMenus", "storage", "tabs", "scripting"],
"host_permissions": ["http://localhost:8080/*"]
```

## Error States

- Backend offline → show "Бэкенд недоступен. Запустите `go run main.go`"
- Analysis error → SSE `error` event displayed in log
- Network error → show inline error message

## Out of Scope

- Settings page (backend URL is hardcoded to localhost:8080)
- History of analyses
- Firefox support (Chrome-first, Firefox compatibility later)
