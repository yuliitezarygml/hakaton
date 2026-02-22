# Frontend Design — Text Analyzer

**Date:** 2026-02-22
**Stack:** Next.js 14 (JavaScript), Tailwind CSS
**Location:** `D:/project/openrouter-web/frontend/`

---

## Goal

Build a Next.js frontend for the text analyzer backend per README.md specs.
Single page, dark theme, n8n-style event log, SSE streaming from `/api/analyze/stream`.

---

## Architecture

App Router (Next.js 14), component-based, no TypeScript.

```
frontend/
├── app/
│   ├── layout.js          # Root layout, fonts, global styles
│   ├── page.js            # Main page (composes components)
│   └── globals.css        # Tailwind + CSS variables
├── components/
│   ├── InputForm.js       # URL/text input, Analyze/Stop buttons
│   ├── EventLog.js        # n8n-style event log (new events prepend)
│   ├── EventRow.js        # Single log row, click to expand JSON
│   ├── ResultCard.js      # Final result card with score
│   └── StatusBar.js       # Connecting... / Connected / Closed
├── hooks/
│   └── useAnalyzer.js     # SSE logic, analysis state
└── package.json
```

---

## Components

### `useAnalyzer.js` (hook)
- State: `events[]`, `result`, `status` (idle/connecting/connected/closed/error)
- `startAnalysis(input)` — auto-detect URL vs text, open EventSource (POST via fetch+ReadableStream)
- `stopAnalysis()` — close SSE connection
- Parses SSE events: start, progress, result, done, error

### `InputForm.js`
- Textarea for URL or text input
- Auto-detect: starts with `http` → URL, else → text
- "Анализировать" button → `startAnalysis()`
- "Стоп" button (visible during analysis) → `stopAnalysis()`
- Enter key triggers analysis

### `StatusBar.js`
- Shows connection status with icon
- States: `Connecting...` → `Connected` → `Connection closed`

### `EventLog.js`
- Dark background, monospace font
- New events prepend (newest on top)
- Each row: `↓ [badge] message  HH:MM:SS.mmm ›`
- Delegates to `EventRow`

### `EventRow.js`
- Badge colors: start=blue, progress=green, result=purple, done=orange, error=red
- Click to expand/collapse full event data as formatted JSON

### `ResultCard.js`
- Appears after `result` event
- Shows: credibility_score (0-10), verdict, manipulations list, sources
- Score colors: 0-3 red, 4-6 yellow, 7-10 green
- If `is_fake: true` → show fake reasons + real information section
- Copy JSON button

---

## Data Flow

```
User Input
    ↓
InputForm → startAnalysis(input)
    ↓
useAnalyzer: POST /api/analyze/stream (SSE)
    ↓
EventSource receives events
    ↓
events[] state updates → EventLog re-renders (prepend)
    ↓
on 'result' event → result state → ResultCard renders
    ↓
on 'done'/'error' → status = closed/error
```

---

## API Integration

- **Endpoint:** `http://localhost:8080/api/analyze/stream`
- **Method:** POST with JSON body `{"url":"..."}` or `{"text":"..."}`
- **Protocol:** SSE (text/event-stream)
- **Note:** SSE with POST requires fetch + ReadableStream (not native EventSource which only supports GET)

---

## Styling

- Dark theme: `#0d0d0d` background, `#1a1a1a` panels
- Monospace font for event log
- Tailwind CSS utility classes
- No external UI component library (keep it lean)
