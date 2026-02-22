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
