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
