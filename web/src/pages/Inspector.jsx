import { useEffect, useRef, useState } from 'react'
import { api } from '../api'

export default function Inspector() {
  const [events, setEvents] = useState([])
  const [filter, setFilter] = useState('')
  const [selected, setSelected] = useState(null)
  const [paused, setPaused] = useState(false)
  const [capture, setCapture] = useState(false)
  const [note, setNote] = useState('')
  const esRef = useRef(null)

  async function refreshMeta() {
    const st = await api('/api/status')
    setCapture(st.captureEnabled)
    setNote(st.inspectorNote)
  }

  async function load() {
    const q = filter ? `?type=${filter}` : ''
    setEvents(await api(`/api/inspector${q}`))
  }

  useEffect(() => {
    refreshMeta().catch(() => {})
    load().catch(() => {})
  }, [filter])

  useEffect(() => {
    if (paused) {
      if (esRef.current) { esRef.current.close(); esRef.current = null }
      return
    }
    const es = new EventSource('/api/inspector/stream')
    esRef.current = es
    es.onmessage = (msg) => {
      try {
        const ev = JSON.parse(msg.data)
        if (filter && ev.type !== filter) return
        setEvents((prev) => [ev, ...prev].slice(0, 500))
      } catch {}
    }
    return () => es.close()
  }, [paused, filter])

  async function toggleCapture() {
    const next = !capture
    await api('/api/settings', { method: 'PUT', body: { captureEnabled: next } })
    setCapture(next)
    await refreshMeta()
  }

  async function clear() {
    await api('/api/inspector/clear', { method: 'POST' })
    setEvents([])
    setSelected(null)
  }

  return (
    <>
      <h1>Inspector</h1>
      <p className="lead">Mixed timeline of HTTP, TLS, and DNS (in-memory ring, ~2000 events). Not mitmweb — events come from the in-container MITM and DNS logger.</p>
      <p className="muted">{note}</p>
      <div className="row" style={{ marginBottom: 12 }}>
        <button className="primary" onClick={toggleCapture}>{capture ? 'Capture on' : 'Capture off'}</button>
        <button onClick={() => setPaused((p) => !p)}>{paused ? 'Resume' : 'Pause'}</button>
        <button onClick={clear}>Clear</button>
        <a href="/api/inspector/har" download="potatoinspector.har"><button type="button">Export HAR</button></a>
        <select value={filter} onChange={(e) => setFilter(e.target.value)} style={{ width: 'auto' }}>
          <option value="">All types</option>
          <option value="http">HTTP</option>
          <option value="tls">TLS</option>
          <option value="dns">DNS</option>
        </select>
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
        <div className="panel-box" style={{ maxHeight: 520, overflow: 'auto', margin: 0 }}>
          <table className="table">
            <thead>
              <tr><th>Type</th><th>Time</th><th>Summary</th></tr>
            </thead>
            <tbody>
              {events.map((ev) => (
                <tr key={ev.id} onClick={() => setSelected(ev)} style={{ cursor: 'pointer', background: selected?.id === ev.id ? 'var(--bg2)' : undefined }}>
                  <td><span className={`badge ${ev.type}`}>{ev.type}</span></td>
                  <td className="mono">{new Date(ev.ts).toLocaleTimeString()}</td>
                  <td>{ev.summary}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="panel-box" style={{ margin: 0 }}>
          <strong>Detail</strong>
          {selected ? (
            <pre className="detail">{JSON.stringify(selected, null, 2)}</pre>
          ) : (
            <p className="muted">Select an event</p>
          )}
        </div>
      </div>
    </>
  )
}
