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
