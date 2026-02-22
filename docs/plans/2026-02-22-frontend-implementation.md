# Next.js Frontend Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a Next.js 14 (JavaScript) single-page frontend for the text analyzer backend at `D:/project/openrouter-web/frontend/`.

**Architecture:** App Router, `useAnalyzer` hook handles all SSE logic via fetch+ReadableStream. Components: InputForm, EventLog, EventRow, ResultCard, StatusBar. Dark theme, n8n-style log with newest events on top.

**Tech Stack:** Next.js 14, JavaScript (no TypeScript), Tailwind CSS, native fetch API

---

### Task 1: Initialize Next.js project

**Files:**
- Create: `frontend/` (entire project)

**Step 1: Create Next.js app**

```bash
cd /d/project/openrouter-web
npx create-next-app@latest frontend --js --tailwind --app --no-eslint --no-src-dir --import-alias "@/*"
```
When prompted, accept all defaults. Do NOT enable TypeScript.

Expected output: "Success! Created frontend"

**Step 2: Clean default boilerplate**

Delete default content from `frontend/app/page.js` — replace with empty placeholder:

```jsx
export default function Home() {
  return <div>Text Analyzer</div>
}
```

Delete `frontend/app/page.module.css` if it exists.

**Step 3: Verify dev server starts**

```bash
cd /d/project/openrouter-web/frontend
npm run dev
```
Expected: Server on http://localhost:3000 — page shows "Text Analyzer"

Stop the server (Ctrl+C).

---

### Task 2: Configure global styles and layout

**Files:**
- Modify: `frontend/app/globals.css`
- Modify: `frontend/app/layout.js`

**Step 1: Replace globals.css**

```css
@tailwind base;
@tailwind components;
@tailwind utilities;

:root {
  --bg-primary: #0d0d0d;
  --bg-panel: #141414;
  --bg-row: #1a1a1a;
  --bg-row-hover: #222222;
  --border: #2a2a2a;
  --text-primary: #e5e5e5;
  --text-muted: #666666;
  --text-time: #555555;
}

* {
  box-sizing: border-box;
  margin: 0;
  padding: 0;
}

body {
  background-color: var(--bg-primary);
  color: var(--text-primary);
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
  -webkit-font-smoothing: antialiased;
}

::-webkit-scrollbar {
  width: 6px;
}
::-webkit-scrollbar-track {
  background: #111;
}
::-webkit-scrollbar-thumb {
  background: #333;
  border-radius: 3px;
}
```

**Step 2: Replace layout.js**

```jsx
import './globals.css'

export const metadata = {
  title: 'Text Analyzer',
  description: 'Анализ текстов на дезинформацию и манипуляции',
}

export default function RootLayout({ children }) {
  return (
    <html lang="ru">
      <body>{children}</body>
    </html>
  )
}
```

**Step 3: Run dev server and verify dark background**

```bash
npm run dev
```
Expected: http://localhost:3000 shows dark page (`#0d0d0d` background).

---

### Task 3: Create useAnalyzer hook

**Files:**
- Create: `frontend/hooks/useAnalyzer.js`

**Step 1: Create hooks directory and file**

```js
'use client'
import { useState, useCallback, useRef } from 'react'

const API_URL = '/api/analyze/stream'

export function useAnalyzer() {
  const [events, setEvents] = useState([])
  const [result, setResult] = useState(null)
  const [status, setStatus] = useState('idle') // idle | connecting | connected | closed | error
  const readerRef = useRef(null)

  const startAnalysis = useCallback(async (input) => {
    setEvents([])
    setResult(null)
    setStatus('connecting')

    const body = input.trim().startsWith('http')
      ? { url: input.trim() }
      : { text: input.trim() }

    try {
      const response = await fetch(API_URL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })

      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      if (!response.body) throw new Error('No response body')

      setStatus('connected')

      const reader = response.body.getReader()
      readerRef.current = reader
      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() ?? ''

        let eventType = null
        let eventData = null

        for (const line of lines) {
          if (line.startsWith('event:')) {
            eventType = line.slice(6).trim()
          } else if (line.startsWith('data:')) {
            eventData = line.slice(5).trim()
          } else if (line === '' && eventType && eventData !== null) {
            const event = {
              id: `${Date.now()}-${Math.random()}`,
              type: eventType,
              data: eventData,
              time: new Date(),
            }

            setEvents(prev => [event, ...prev])

            if (eventType === 'result') {
              try { setResult(JSON.parse(eventData)) } catch {}
            }
            if (eventType === 'done' || eventType === 'error') {
              setStatus('closed')
            }

            eventType = null
            eventData = null
          }
        }
      }

      setStatus(prev => prev === 'connected' ? 'closed' : prev)
    } catch (err) {
      if (err.name !== 'AbortError') {
        setEvents(prev => [{
          id: `${Date.now()}-err`,
          type: 'error',
          data: err.message,
          time: new Date(),
        }, ...prev])
        setStatus('error')
      }
    }
  }, [])

  const stopAnalysis = useCallback(() => {
    if (readerRef.current) {
      readerRef.current.cancel()
      readerRef.current = null
    }
    setStatus('closed')
  }, [])

  return { events, result, status, startAnalysis, stopAnalysis }
}
```

---

### Task 4: Create StatusBar component

**Files:**
- Create: `frontend/components/StatusBar.js`

**Step 1: Create components directory and StatusBar.js**

```jsx
export default function StatusBar({ status, lastTime }) {
  const config = {
    connecting: { icon: '⟳', text: 'Connecting...', color: '#888' },
    connected:  { icon: '●', text: 'Connected',      color: '#22c55e' },
    closed:     { icon: 'ⓘ', text: 'Connection closed', color: '#888' },
    error:      { icon: '✕', text: 'Connection error',  color: '#ef4444' },
  }

  const info = config[status]
  if (!info) return null

  const timeStr = lastTime
    ? lastTime.toLocaleTimeString('ru', { hour12: false, fractionalSecondDigits: 3 })
    : ''

  return (
    <div style={{
      display: 'flex',
      justifyContent: 'space-between',
      alignItems: 'center',
      padding: '8px 16px',
      borderBottom: '1px solid var(--border)',
      fontFamily: 'monospace',
      fontSize: '13px',
      color: info.color,
    }}>
      <span>{info.icon}&nbsp;&nbsp;{info.text}</span>
      {timeStr && <span style={{ color: 'var(--text-time)' }}>{timeStr}</span>}
    </div>
  )
}
```

---

### Task 5: Create EventRow component

**Files:**
- Create: `frontend/components/EventRow.js`

**Step 1: Create EventRow.js**

```jsx
'use client'
import { useState } from 'react'

const BADGE = {
  start:    { bg: '#1e3a5f', border: '#3b82f644', text: '#60a5fa' },
  progress: { bg: '#14401e', border: '#22c55e44', text: '#4ade80' },
  result:   { bg: '#3b1060', border: '#a855f744', text: '#c084fc' },
  done:     { bg: '#431407', border: '#f9731644', text: '#fb923c' },
  error:    { bg: '#450a0a', border: '#ef444444', text: '#f87171' },
}

function formatJSON(str) {
  try { return JSON.stringify(JSON.parse(str), null, 2) }
  catch { return str }
}

export default function EventRow({ event }) {
  const [expanded, setExpanded] = useState(false)
  const b = BADGE[event.type] || BADGE.progress
  const isExpandable = event.type === 'result' || event.data.length > 80

  const timeStr = event.time.toLocaleTimeString('ru', {
    hour12: false,
    fractionalSecondDigits: 3,
  })

  const preview = event.type === 'result'
    ? event.data.slice(0, 55) + '...'
    : event.data

  return (
    <div>
      <div
        onClick={() => isExpandable && setExpanded(e => !e)}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '10px',
          padding: '7px 16px',
          borderBottom: '1px solid var(--border)',
          background: 'var(--bg-row)',
          cursor: isExpandable ? 'pointer' : 'default',
          fontFamily: 'monospace',
          fontSize: '13px',
          userSelect: 'none',
        }}
        onMouseEnter={e => { if (isExpandable) e.currentTarget.style.background = 'var(--bg-row-hover)' }}
        onMouseLeave={e => e.currentTarget.style.background = 'var(--bg-row)'}
      >
        <span style={{ color: 'var(--text-muted)', flexShrink: 0 }}>↓</span>
        <span style={{
          background: b.bg,
          color: b.text,
          border: `1px solid ${b.border}`,
          borderRadius: '4px',
          padding: '1px 7px',
          fontSize: '11px',
          minWidth: '68px',
          textAlign: 'center',
          flexShrink: 0,
        }}>
          {event.type}
        </span>
        <span style={{
          flex: 1,
          color: 'var(--text-primary)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}>
          {preview}
        </span>
        <span style={{ color: 'var(--text-time)', flexShrink: 0, fontSize: '12px' }}>
          {timeStr}
        </span>
        {isExpandable && (
          <span style={{ color: 'var(--text-muted)', flexShrink: 0 }}>
            {expanded ? '˅' : '›'}
          </span>
        )}
      </div>

      {expanded && (
        <div style={{
          padding: '14px 16px',
          background: '#0a0a0a',
          borderBottom: '1px solid var(--border)',
          fontFamily: 'monospace',
          fontSize: '12px',
          color: '#aaa',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-all',
          maxHeight: '500px',
          overflowY: 'auto',
          lineHeight: '1.6',
        }}>
          {formatJSON(event.data)}
        </div>
      )}
    </div>
  )
}
```

---

### Task 6: Create EventLog component

**Files:**
- Create: `frontend/components/EventLog.js`

**Step 1: Create EventLog.js**

```jsx
import EventRow from './EventRow'
import StatusBar from './StatusBar'

export default function EventLog({ events, status }) {
  if (status === 'idle' && events.length === 0) return null

  const lastTime = events.length > 0 ? events[0].time : null

  return (
    <div style={{
      background: 'var(--bg-panel)',
      border: '1px solid var(--border)',
      borderRadius: '8px',
      overflow: 'hidden',
      marginTop: '16px',
    }}>
      <StatusBar status={status} lastTime={lastTime} />
      <div>
        {events.map(event => (
          <EventRow key={event.id} event={event} />
        ))}
      </div>
    </div>
  )
}
```

---

### Task 7: Create ResultCard component

**Files:**
- Create: `frontend/components/ResultCard.js`

**Step 1: Create ResultCard.js**

```jsx
'use client'
import { useState } from 'react'

function scoreColor(n) {
  if (n <= 3) return '#ef4444'
  if (n <= 6) return '#eab308'
  return '#22c55e'
}

function verdict(n) {
  if (n <= 3) return '🔴 ВЕРОЯТНАЯ ДЕЗИНФОРМАЦИЯ'
  if (n <= 6) return '🟡 СОМНИТЕЛЬНЫЙ КОНТЕНТ'
  return '🟢 ДОСТОВЕРНЫЙ КОНТЕНТ'
}

function Section({ title, color, children }) {
  return (
    <div style={{ marginBottom: '18px' }}>
      <div style={{ color, fontWeight: '600', marginBottom: '8px', fontSize: '14px' }}>
        {title}
      </div>
      {children}
    </div>
  )
}

function SourceLink({ src }) {
  const s = typeof src === 'string' ? { title: src, url: '#', description: '' } : src
  return (
    <li style={{ marginBottom: '6px' }}>
      <a href={s.url} target="_blank" rel="noopener noreferrer"
        style={{ color: '#93c5fd', textDecoration: 'none' }}>
        {s.title}
      </a>
      {s.description && (
        <span style={{ color: '#888', fontSize: '12px' }}> — {s.description}</span>
      )}
    </li>
  )
}

export default function ResultCard({ result }) {
  const [copied, setCopied] = useState(false)
  if (!result) return null

  const color = scoreColor(result.credibility_score)

  const copyJSON = () => {
    navigator.clipboard.writeText(JSON.stringify(result, null, 2))
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div style={{
      background: 'var(--bg-panel)',
      border: '1px solid var(--border)',
      borderRadius: '8px',
      padding: '24px',
      marginTop: '16px',
    }}>
      {/* Header: score + verdict + copy button */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '16px', marginBottom: '20px', flexWrap: 'wrap' }}>
        <span style={{ fontSize: '38px', fontWeight: 'bold', color, fontFamily: 'monospace', lineHeight: 1 }}>
          {result.credibility_score}/10
        </span>
        <span style={{ fontSize: '15px', fontWeight: '600', color }}>
          {verdict(result.credibility_score)}
        </span>
        <button onClick={copyJSON} style={{
          marginLeft: 'auto',
          padding: '6px 14px',
          background: '#1e1e1e',
          border: '1px solid var(--border)',
          borderRadius: '6px',
          color: copied ? '#4ade80' : '#888',
          cursor: 'pointer',
          fontSize: '12px',
          fontFamily: 'monospace',
          transition: 'color 0.2s',
        }}>
          {copied ? '✓ Скопировано' : '⎘ Копировать JSON'}
        </button>
      </div>

      {/* Summary */}
      {result.summary && (
        <p style={{ color: '#ccc', lineHeight: '1.7', marginBottom: '20px', fontSize: '14px' }}>
          {result.summary}
        </p>
      )}

      {/* Reasoning */}
      {result.reasoning && (
        <Section title="💬 Обоснование" color="#a0a0a0">
          <p style={{ color: '#aaa', fontSize: '13px', lineHeight: '1.6' }}>{result.reasoning}</p>
        </Section>
      )}

      {/* Fake warning */}
      {result.verification?.is_fake && (
        <div style={{
          background: '#ef444418',
          border: '1px solid #ef444430',
          borderRadius: '6px',
          padding: '14px 16px',
          marginBottom: '18px',
        }}>
          <div style={{ color: '#ef4444', fontWeight: '600', marginBottom: '10px' }}>🚨 Почему фейк</div>
          <ul style={{ paddingLeft: '20px', color: '#fca5a5', fontSize: '13px' }}>
            {result.verification.fake_reasons?.map((r, i) => <li key={i} style={{ marginBottom: '4px' }}>{r}</li>)}
          </ul>
          {result.verification.real_information && (
            <p style={{ marginTop: '10px', color: '#fca5a5', fontSize: '13px', lineHeight: '1.5' }}>
              <strong>Реальная информация:</strong> {result.verification.real_information}
            </p>
          )}
        </div>
      )}

      {/* Manipulations */}
      {result.manipulations?.length > 0 && (
        <Section title="⚠ Манипуляции" color="#fbbf24">
          <ul style={{ paddingLeft: '20px', color: '#e5e5e5', fontSize: '13px' }}>
            {result.manipulations.map((m, i) => <li key={i} style={{ marginBottom: '4px' }}>{m}</li>)}
          </ul>
        </Section>
      )}

      {/* Logical issues */}
      {result.logical_issues?.length > 0 && (
        <Section title="⚡ Логические ошибки" color="#f97316">
          <ul style={{ paddingLeft: '20px', color: '#e5e5e5', fontSize: '13px' }}>
            {result.logical_issues.map((l, i) => <li key={i} style={{ marginBottom: '4px' }}>{l}</li>)}
          </ul>
        </Section>
      )}

      {/* Fact check */}
      {result.fact_check && (
        <Section title="🔍 Фактчек" color="#818cf8">
          {result.fact_check.opinions_as_facts?.length > 0 && (
            <div style={{ marginBottom: '10px' }}>
              <div style={{ color: '#888', fontSize: '12px', marginBottom: '4px' }}>Мнения, поданные как факты:</div>
              <ul style={{ paddingLeft: '20px', color: '#c7d2fe', fontSize: '13px' }}>
                {result.fact_check.opinions_as_facts.map((o, i) => <li key={i} style={{ marginBottom: '3px' }}>{o}</li>)}
              </ul>
            </div>
          )}
          {result.fact_check.missing_evidence?.length > 0 && (
            <div>
              <div style={{ color: '#888', fontSize: '12px', marginBottom: '4px' }}>Утверждения без доказательств:</div>
              <ul style={{ paddingLeft: '20px', color: '#c7d2fe', fontSize: '13px' }}>
                {result.fact_check.missing_evidence.map((e, i) => <li key={i} style={{ marginBottom: '3px' }}>{e}</li>)}
              </ul>
            </div>
          )}
        </Section>
      )}

      {/* Sources */}
      {result.sources?.length > 0 && (
        <Section title="📚 Источники для проверки" color="#60a5fa">
          <ul style={{ paddingLeft: '20px', listStyle: 'none' }}>
            {result.sources.map((s, i) => <SourceLink key={i} src={s} />)}
          </ul>
        </Section>
      )}

      {/* Verified sources */}
      {result.verification?.verified_sources?.length > 0 && (
        <Section title="📚 Реальные источники" color="#60a5fa">
          <ul style={{ paddingLeft: '20px', listStyle: 'none' }}>
            {result.verification.verified_sources.map((s, i) => <SourceLink key={i} src={s} />)}
          </ul>
        </Section>
      )}
    </div>
  )
}
```

---

### Task 8: Create InputForm component

**Files:**
- Create: `frontend/components/InputForm.js`

**Step 1: Create InputForm.js**

```jsx
'use client'
import { useState } from 'react'

export default function InputForm({ onAnalyze, onStop, isAnalyzing }) {
  const [input, setInput] = useState('')
  const canSubmit = input.trim().length > 0 && !isAnalyzing

  const handleSubmit = () => {
    if (canSubmit) onAnalyze(input.trim())
  }

  const handleKeyDown = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSubmit()
    }
  }

  return (
    <div style={{ display: 'flex', gap: '10px', alignItems: 'flex-start' }}>
      <textarea
        value={input}
        onChange={e => setInput(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Введите URL статьи или текст для анализа... (Enter — запуск, Shift+Enter — новая строка)"
        disabled={isAnalyzing}
        rows={3}
        style={{
          flex: 1,
          padding: '12px 16px',
          background: '#181818',
          border: '1px solid var(--border)',
          borderRadius: '8px',
          color: 'var(--text-primary)',
          fontSize: '14px',
          resize: 'vertical',
          outline: 'none',
          fontFamily: 'inherit',
          lineHeight: '1.6',
          transition: 'border-color 0.15s',
          opacity: isAnalyzing ? 0.6 : 1,
        }}
        onFocus={e => e.target.style.borderColor = '#444'}
        onBlur={e => e.target.style.borderColor = 'var(--border)'}
      />
      <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', flexShrink: 0 }}>
        {!isAnalyzing ? (
          <button
            onClick={handleSubmit}
            disabled={!canSubmit}
            style={{
              padding: '12px 20px',
              background: canSubmit ? '#2563eb' : '#1e2d4a',
              border: 'none',
              borderRadius: '8px',
              color: canSubmit ? '#fff' : '#4a6899',
              cursor: canSubmit ? 'pointer' : 'not-allowed',
              fontSize: '14px',
              fontWeight: '500',
              whiteSpace: 'nowrap',
              transition: 'background 0.15s, color 0.15s',
            }}
            onMouseEnter={e => { if (canSubmit) e.currentTarget.style.background = '#1d4ed8' }}
            onMouseLeave={e => { if (canSubmit) e.currentTarget.style.background = '#2563eb' }}
          >
            Анализировать
          </button>
        ) : (
          <button
            onClick={onStop}
            style={{
              padding: '12px 20px',
              background: '#7f1d1d',
              border: '1px solid #991b1b',
              borderRadius: '8px',
              color: '#fca5a5',
              cursor: 'pointer',
              fontSize: '14px',
              fontWeight: '500',
              whiteSpace: 'nowrap',
            }}
            onMouseEnter={e => e.currentTarget.style.background = '#991b1b'}
            onMouseLeave={e => e.currentTarget.style.background = '#7f1d1d'}
          >
            ⬛ Стоп
          </button>
        )}
        {isAnalyzing && (
          <div style={{
            textAlign: 'center',
            fontSize: '11px',
            color: '#555',
            fontFamily: 'monospace',
          }}>
            анализ...
          </div>
        )}
      </div>
    </div>
  )
}
```

---

### Task 9: Wire everything in page.js

**Files:**
- Modify: `frontend/app/page.js`

**Step 1: Replace page.js with full composition**

```jsx
'use client'
import { useAnalyzer } from '../hooks/useAnalyzer'
import InputForm from '../components/InputForm'
import EventLog from '../components/EventLog'
import ResultCard from '../components/ResultCard'

export default function Home() {
  const { events, result, status, startAnalysis, stopAnalysis } = useAnalyzer()
  const isAnalyzing = status === 'connecting' || status === 'connected'

  return (
    <main style={{
      maxWidth: '960px',
      margin: '0 auto',
      padding: '48px 20px 80px',
    }}>
      <div style={{ marginBottom: '28px' }}>
        <h1 style={{
          fontSize: '22px',
          fontWeight: '600',
          color: 'var(--text-primary)',
          marginBottom: '4px',
        }}>
          Text Analyzer
        </h1>
        <p style={{ color: 'var(--text-muted)', fontSize: '13px' }}>
          Анализ текстов и статей на дезинформацию, манипуляции и логические ошибки
        </p>
      </div>

      <InputForm
        onAnalyze={startAnalysis}
        onStop={stopAnalysis}
        isAnalyzing={isAnalyzing}
      />

      <EventLog events={events} status={status} />
      <ResultCard result={result} />
    </main>
  )
}
```

---

### Task 10: Configure Next.js API proxy + update hook

**Files:**
- Modify: `frontend/next.config.mjs` (or `next.config.js`)

**Step 1: Add rewrites to proxy `/api/*` → backend**

Read the existing `next.config.mjs` first, then add rewrites:

```js
/** @type {import('next').NextConfig} */
const nextConfig = {
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: 'http://localhost:8080/api/:path*',
      },
    ]
  },
}

export default nextConfig
```

**Step 2: Verify API_URL in useAnalyzer.js is `/api/analyze/stream` (relative)**

The hook already uses `/api/analyze/stream`. If it still has the full `http://localhost:8080` URL, change it to `/api/analyze/stream`.

**Step 3: Start both servers and do end-to-end test**

Terminal 1 (backend):
```bash
cd /d/project/openrouter-web
go run main.go
```
Expected: "Server started on :8080"

Terminal 2 (frontend):
```bash
cd /d/project/openrouter-web/frontend
npm run dev
```
Expected: "Ready on http://localhost:3000"

**Step 4: Full smoke test**

1. Open http://localhost:3000
2. Paste a URL (e.g., `https://stopfals.md/ro/article/fals-nato-construieste`) into the field
3. Click "Анализировать" or press Enter
4. Verify:
   - Status bar shows "Connecting..." → "Connected"
   - Events appear in log (newest on top)
   - `[start]` badge is blue, `[progress]` green, `[result]` purple, `[done]` orange
   - Clicking `[result]` row expands full JSON
   - After `[done]`, ResultCard appears with score, verdict, manipulations
   - "Стоп" button appears during analysis
   - Status changes to "Connection closed" when done
5. Test with plain text: paste a paragraph of text, verify it works

**Step 5: Final commit**

```bash
cd /d/project/openrouter-web/frontend
git add .
git commit -m "feat: add Next.js frontend for text analyzer"
```
