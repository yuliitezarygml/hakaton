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
  const [realInfoOpen, setRealInfoOpen] = useState(false)
  if (!result) return null

  const hasRealInfo = result.verification?.real_information ||
    (result.verification?.verified_sources?.length > 0)

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
        <div style={{ marginLeft: 'auto', display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
          {hasRealInfo && (
            <button
              onClick={() => setRealInfoOpen(o => !o)}
              style={{
                padding: '6px 14px',
                background: realInfoOpen ? '#1a3a2a' : '#0f2235',
                border: `1px solid ${realInfoOpen ? '#2a6040' : '#1a3a55'}`,
                borderRadius: '6px',
                color: realInfoOpen ? '#4ade80' : '#38bdf8',
                cursor: 'pointer',
                fontSize: '12px',
                fontFamily: 'var(--font-mono)',
                transition: 'all 0.15s',
              }}
            >
              {realInfoOpen ? '✕ Скрыть' : '🔍 Настоящая информация'}
            </button>
          )}
          {result.source_url && (
            <a
              href={result.source_url}
              target="_blank"
              rel="noopener noreferrer"
              style={{
                padding: '6px 14px',
                background: '#0f2a1a',
                border: '1px solid #1a4a2a',
                borderRadius: '6px',
                color: '#4ade80',
                fontSize: '12px',
                fontFamily: 'var(--font-mono)',
                textDecoration: 'none',
                display: 'inline-flex',
                alignItems: 'center',
                gap: '5px',
                transition: 'background 0.15s',
              }}
              onMouseEnter={e => e.currentTarget.style.background = '#162e20'}
              onMouseLeave={e => e.currentTarget.style.background = '#0f2a1a'}
            >
              ↗ Оригинал статьи
            </a>
          )}
          <button onClick={copyJSON} style={{
            padding: '6px 14px',
            background: '#1e1e1e',
            border: '1px solid var(--border)',
            borderRadius: '6px',
            color: copied ? '#4ade80' : '#888',
            cursor: 'pointer',
            fontSize: '12px',
            fontFamily: 'var(--font-mono)',
            transition: 'color 0.2s',
          }}>
            {copied ? '✓ Скопировано' : '⎘ Копировать JSON'}
          </button>
        </div>
      </div>

      {/* Real information panel */}
      {realInfoOpen && hasRealInfo && (
        <div className="expand-enter" style={{
          background: '#060f18',
          border: '1px solid #1a3a55',
          borderRadius: '8px',
          padding: '20px',
          marginBottom: '20px',
        }}>
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
            marginBottom: '16px',
            color: '#38bdf8',
            fontWeight: '600',
            fontSize: '14px',
          }}>
            <span>🔍</span>
            <span>Проверенная информация по теме</span>
          </div>

          {result.verification?.real_information && (
            <p style={{
              color: '#a0bcd0',
              fontSize: '13px',
              lineHeight: '1.7',
              marginBottom: '16px',
              fontFamily: 'var(--font-mono)',
              whiteSpace: 'pre-wrap',
            }}>
              {result.verification.real_information}
            </p>
          )}

          {result.verification?.verified_sources?.length > 0 && (
            <div>
              <div style={{ color: '#38bdf8', fontSize: '12px', marginBottom: '10px', opacity: 0.7 }}>
                Источники с достоверной информацией:
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                {result.verification.verified_sources.map((src, i) => {
                  const s = typeof src === 'string' ? { title: src, url: '#', description: '' } : src
                  return (
                    <a
                      key={i}
                      href={s.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      style={{
                        display: 'block',
                        padding: '10px 14px',
                        background: '#0c1e30',
                        border: '1px solid #1a3a55',
                        borderRadius: '6px',
                        textDecoration: 'none',
                        transition: 'border-color 0.15s, background 0.15s',
                      }}
                      onMouseEnter={e => {
                        e.currentTarget.style.borderColor = '#38bdf8'
                        e.currentTarget.style.background = '#101e2e'
                      }}
                      onMouseLeave={e => {
                        e.currentTarget.style.borderColor = '#1a3a55'
                        e.currentTarget.style.background = '#0c1e30'
                      }}
                    >
                      <div style={{ color: '#7dd3fc', fontSize: '13px', fontWeight: '500', marginBottom: s.description ? '4px' : 0 }}>
                        ↗ {s.title}
                      </div>
                      {s.description && (
                        <div style={{ color: '#4a6880', fontSize: '11px', fontFamily: 'var(--font-mono)' }}>
                          {s.description.length > 120 ? s.description.slice(0, 120) + '...' : s.description}
                        </div>
                      )}
                    </a>
                  )
                })}
              </div>
            </div>
          )}
        </div>
      )}

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

    </div>
  )
}
