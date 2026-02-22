export default function StatusBar({ status, lastTime }) {
  const config = {
    connecting: { dot: 'spin',    text: 'Connecting...',     color: '#38bdf8' },
    connected:  { dot: 'pulse',   text: 'Connected',         color: '#34d399' },
    closed:     { dot: 'static',  text: 'Connection closed', color: '#364f66' },
    error:      { dot: 'static',  text: 'Connection error',  color: '#f87171' },
  }

  const info = config[status]
  if (!info) return null

  const timeStr = lastTime
    ? lastTime.toLocaleTimeString('ru', { hour12: false, fractionalSecondDigits: 3 })
    : ''

  const dotStyle = {
    width: 7,
    height: 7,
    borderRadius: '50%',
    background: info.color,
    flexShrink: 0,
    ...(info.dot === 'pulse' && {
      animation: 'pulseDot 1.4s ease-in-out infinite',
    }),
  }

  const spinnerStyle = {
    width: 10,
    height: 10,
    borderRadius: '50%',
    border: `1.5px solid ${info.color}33`,
    borderTopColor: info.color,
    flexShrink: 0,
    animation: 'spin .7s linear infinite',
  }

  return (
    <div style={{
      display: 'flex',
      justifyContent: 'space-between',
      alignItems: 'center',
      padding: '9px 18px',
      borderBottom: '1px solid var(--border)',
      background: 'rgba(11,16,24,.8)',
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
        {info.dot === 'spin'
          ? <div style={spinnerStyle} />
          : <div style={dotStyle} />
        }
        <span style={{
          fontFamily: 'var(--font-mono)',
          fontSize: '11px',
          color: info.color,
          letterSpacing: '.02em',
        }}>
          {info.text}
        </span>
      </div>
      {timeStr && (
        <span style={{
          fontFamily: 'var(--font-mono)',
          fontSize: '11px',
          color: 'var(--text-time)',
          letterSpacing: '.02em',
        }}>
          {timeStr}
        </span>
      )}
    </div>
  )
}
