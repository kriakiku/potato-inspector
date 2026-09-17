import { useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../api'
import { notifyError, notifySuccess } from '../toast'

function statusClass(code) {
  if (!code) return 'st-zero'
  if (code < 300) return 'st-ok'
  if (code < 400) return 'st-redir'
  if (code < 500) return 'st-client'
  return 'st-server'
}

function eventUrl(ev) {
  const d = ev.detail || {}
  if (ev.type === 'http') {
    return d.url || `${d.host || ''}${d.path || ''}` || ev.summary
  }
  if (ev.type === 'tls') return d.sni || ev.summary
  if (ev.type === 'dns') return d.qname || ev.summary
  return ev.summary
}

function eventName(ev) {
  const d = ev.detail || {}
  if (ev.type === 'http') {
    const path = d.path || '/'
    const base = path.split('?')[0]
    const parts = base.split('/').filter(Boolean)
    return parts.length ? parts[parts.length - 1] || '/' : (d.host || '/')
  }
  if (ev.type === 'tls') return d.sni || 'TLS'
  if (ev.type === 'dns') return d.qname || 'DNS'
  return ev.summary
}

function formatBytes(n) {
  if (n == null || n === 0) return '—'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

function durationMs(ev) {
  const t = ev.detail?.timing
  if (!t?.timestampStart || !t?.timestampEnd) return null
  return Math.max(0, Math.round((t.timestampEnd - t.timestampStart) * 1000))
}

function HeaderTable({ headers }) {
  const entries = Object.entries(headers || {})
  if (!entries.length) return <p className="muted">No headers</p>
  return (
    <table className="hdr-table">
      <tbody>
        {entries.map(([k, v]) => (
          <tr key={k}>
            <th>{k}</th>
            <td className="mono">{v}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function DetailPane({ selected, tab, setTab }) {
  if (!selected) {
    return <div className="net-empty muted">Select a request to inspect headers and body</div>
  }
  const d = selected.detail || {}
  const tabs = selected.type === 'http'
    ? ['Headers', 'Preview', 'Timing']
    : ['Detail']

  return (
    <div className="net-detail">
      <div className="net-detail-title mono" title={eventUrl(selected)}>
        {selected.type === 'http' && (
          <>
            <span className="net-method">{d.method}</span>{' '}
            <span className={statusClass(d.status)}>{d.status || '—'}</span>{' '}
          </>
        )}
        <span className="net-detail-url">{eventUrl(selected)}</span>
      </div>
      <div className="net-tabs">
        {tabs.map((t) => (
          <button
            key={t}
            type="button"
            className={tab === t ? 'active' : ''}
            onClick={() => setTab(t)}
          >
            {t}
          </button>
        ))}
      </div>
      <div className="net-detail-body net-scroll">
        {selected.type === 'http' && tab === 'Headers' && (
          <>
            <h3>General</h3>
            <table className="hdr-table">
              <tbody>
                <tr><th>Request URL</th><td className="mono">{d.url || eventUrl(selected)}</td></tr>
                <tr><th>Method</th><td className="mono">{d.method}</td></tr>
                <tr><th>Status</th><td className={`mono ${statusClass(d.status)}`}>{d.status}</td></tr>
                <tr><th>Content-Type</th><td className="mono">{d.contentType || '—'}</td></tr>
                <tr><th>Body size</th><td className="mono">{formatBytes(d.bodyBytes)}</td></tr>
                {d.extraDelayMs ? <tr><th>Extra delay</th><td className="mono">{d.extraDelayMs} ms</td></tr> : null}
              </tbody>
            </table>
            <h3>Request Headers</h3>
            <HeaderTable headers={d.requestHeaders} />
            <h3>Response Headers</h3>
            <HeaderTable headers={d.responseHeaders} />
          </>
        )}
        {selected.type === 'http' && tab === 'Preview' && (
          <pre className="net-preview mono">{d.bodyPreview || '(empty body)'}</pre>
        )}
        {selected.type === 'http' && tab === 'Timing' && (
          <table className="hdr-table">
            <tbody>
              <tr><th>Duration</th><td className="mono">{durationMs(selected) != null ? `${durationMs(selected)} ms` : '—'}</td></tr>
              <tr><th>Start</th><td className="mono">{d.timing?.timestampStart ?? '—'}</td></tr>
              <tr><th>End</th><td className="mono">{d.timing?.timestampEnd ?? '—'}</td></tr>
              <tr><th>Extra delay</th><td className="mono">{d.extraDelayMs ? `${d.extraDelayMs} ms` : '—'}</td></tr>
            </tbody>
          </table>
        )}
        {selected.type !== 'http' && (
          <pre className="net-preview mono">{JSON.stringify(selected, null, 2)}</pre>
        )}
      </div>
    </div>
  )
}

export default function Inspector() {
  const [events, setEvents] = useState([])
  const [filter, setFilter] = useState('')
  const [selected, setSelected] = useState(null)
  const [tab, setTab] = useState('Headers')
  const [paused, setPaused] = useState(false)
  const [capture, setCapture] = useState(false)
  const [note, setNote] = useState('')
  const esRef = useRef(null)
  const listRef = useRef(null)

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
    refreshMeta().catch((e) => notifyError(e.message))
    load().catch((e) => notifyError(e.message))
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

  useEffect(() => {
    if (!selected) return
    setTab(selected.type === 'http' ? 'Headers' : 'Detail')
  }, [selected?.id])

  async function toggleCapture() {
    const next = !capture
    try {
      await api('/api/settings', { method: 'PUT', body: { captureEnabled: next } })
      setCapture(next)
      notifySuccess(next ? 'Capture on' : 'Capture off')
      await refreshMeta()
    } catch (e) {
      notifyError(e.message)
    }
  }

  async function clear() {
    try {
      await api('/api/inspector/clear', { method: 'POST' })
      setEvents([])
      setSelected(null)
      notifySuccess('Inspector cleared')
    } catch (e) {
      notifyError(e.message)
    }
  }

  const rows = useMemo(() => events, [events])

  return (
    <>
      <h1>Inspector</h1>
      <p className="lead">Network-style timeline (HTTP / TLS / DNS). In-memory ring ~2000 events.</p>
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

      <div className="net-shell">
        <div className="net-list panel-box" style={{ margin: 0 }}>
          <div className="net-list-head">
            <span className="col-status">Status</span>
            <span className="col-method">Method</span>
            <span className="col-name">Name</span>
            <span className="col-type">Type</span>
            <span className="col-size">Size</span>
            <span className="col-time">Time</span>
          </div>
          <div className="net-list-body net-scroll" ref={listRef}>
            {rows.length === 0 && <div className="net-empty muted">No events yet — enable capture and MITM (for HTTP/TLS)</div>}
            {rows.map((ev) => {
              const d = ev.detail || {}
              const dur = durationMs(ev)
              const active = selected?.id === ev.id
              return (
                <button
                  type="button"
                  key={ev.id}
                  className={`net-row ${active ? 'active' : ''}`}
                  onClick={() => setSelected(ev)}
                  title={eventUrl(ev)}
                >
                  <span className={`col-status mono ${ev.type === 'http' ? statusClass(d.status) : ''}`}>
                    {ev.type === 'http' ? (d.status || '—') : '—'}
                  </span>
                  <span className="col-method mono">{ev.type === 'http' ? d.method : ev.type.toUpperCase()}</span>
                  <span className="col-name">
                    <span className="net-name">{eventName(ev)}</span>
                    <span className="net-url mono">{eventUrl(ev)}</span>
                  </span>
                  <span className="col-type"><span className={`badge ${ev.type}`}>{ev.type}</span></span>
                  <span className="col-size mono">{ev.type === 'http' ? formatBytes(d.bodyBytes) : '—'}</span>
                  <span className="col-time mono">{dur != null ? `${dur} ms` : new Date(ev.ts).toLocaleTimeString()}</span>
                </button>
              )
            })}
          </div>
        </div>
        <div className="net-side panel-box" style={{ margin: 0 }}>
          <DetailPane selected={selected} tab={tab} setTab={setTab} />
        </div>
      </div>
    </>
  )
}
