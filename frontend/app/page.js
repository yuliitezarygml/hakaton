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
