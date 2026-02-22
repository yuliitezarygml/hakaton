'use client'
import { useState } from 'react'

const THEME = {
  start:    { color: '#38bdf8', label: 'start' },
  progress: { color: '#34d399', label: 'progress' },
  result:   { color: '#a78bfa', label: 'result' },
  done:     { color: '#fb923c', label: 'done' },
  error:    { color: '#f87171', label: 'error' },
}

function formatJSON(str) {
  try { return JSON.stringify(JSON.parse(str), null, 2) }
  catch { return str }
}

function syntaxColor(str) {
  // Simple JSON syntax highlighting
  return str
    .replace(/("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(?:\s*:)?)/g, m =>
      m.endsWith(':')
        ? `<span style="color:#7dd3fc">${m}</span>`
        : `<span style="color:#86efac">${m}</span>`
    )
    .replace(/\b(true|false)\b/g, '<span style="color:#a78bfa">$1</span>')
    .replace(/\b(null)\b/g, '<span style="color:#f87171">$1</span>')
    .replace(/\b(-?\d+\.?\d*)\b/g, '<span style="color:#fbbf24">$1</span>')
}

export default function EventRow({ event }) {
  const [expanded, setExpanded] = useState(false)
  const t = THEME[event.type] || THEME.progress
  const isExpandable = event.type === 'result' || event.data.length > 90

  const timeStr = event.time.toLocaleTimeString('ru', {
    hour12: false,
    fractionalSecondDigits: 3,
  })

  const preview = event.type === 'result'
    ? event.data.slice(0, 60) + '...'
    : event.data

  return (
    <div className="row-enter" style={{ borderLeft: `2px solid ${t.color}18` }}>
      {/* ── Main row ── */}
      <div
        role={isExpandable ? 'button' : undefined}
        onClick={() => isExpandable && setExpanded(e => !e)}
        style={{
          display: 'grid',
          gridTemplateColumns: '14px 74px 1fr auto auto',
          alignItems: 'center',
          gap: '12px',
          padding: '8px 18px 8px 16px',
          borderBottom: '1px solid var(--border)',
          background: 'var(--bg-row)',
          cursor: isExpandable ? 'pointer' : 'default',
          transition: 'background .12s ease',
          userSelect: 'none',
        }}
        onMouseEnter={e => { if (isExpandable) e.currentTarget.style.background = 'var(--bg-row-hover)' }}
        onMouseLeave={e => e.currentTarget.style.background = 'var(--bg-row)'}
      >
        {/* Arrow */}
        <span style={{
          color: t.color,
          opacity: .5,
          fontSize: '10px',
          fontFamily: 'var(--font-mono)',
        }}>
          ↓
        </span>

        {/* Badge */}
        <span style={{
          fontFamily: 'var(--font-mono)',
          fontSize: '10px',
          fontWeight: '500',
          letterSpacing: '.04em',
          color: t.color,
          background: `${t.color}12`,
          border: `1px solid ${t.color}28`,
          borderRadius: '4px',
          padding: '2px 8px',
          textAlign: 'center',
          textTransform: 'lowercase',
        }}>
          {t.label}
        </span>

        {/* Message */}
        <span style={{
          fontFamily: 'var(--font-mono)',
          fontSize: '12.5px',
          color: 'var(--text-primary)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          letterSpacing: '.01em',
        }}>
          {preview}
        </span>

        {/* Time */}
        <span style={{
          fontFamily: 'var(--font-mono)',
          fontSize: '10.5px',
          color: 'var(--text-time)',
          flexShrink: 0,
          letterSpacing: '.02em',
        }}>
          {timeStr}
        </span>

        {/* Chevron */}
        {isExpandable && (
          <span style={{
            color: 'var(--text-muted)',
            fontSize: '11px',
            flexShrink: 0,
            transform: expanded ? 'rotate(90deg)' : 'rotate(0)',
            transition: 'transform .18s cubic-bezier(.16,1,.3,1)',
            display: 'inline-block',
          }}>
            ›
          </span>
        )}
      </div>

      {/* ── Expanded JSON ── */}
      {expanded && (
        <div
          className="expand-enter"
          style={{
            padding: '16px 20px',
            background: 'var(--bg-expand)',
            borderBottom: `1px solid var(--border)`,
            borderLeft: `2px solid ${t.color}30`,
            fontFamily: 'var(--font-mono)',
            fontSize: '11.5px',
            lineHeight: '1.7',
            maxHeight: '560px',
            overflowY: 'auto',
          }}
          dangerouslySetInnerHTML={{ __html: syntaxColor(formatJSON(event.data)) }}
        />
      )}
    </div>
  )
}
