import { useEffect, useState } from 'react'
import { api } from '../api'

export default function Settings() {
  const [s, setS] = useState(null)
  const [pw, setPw] = useState({ current: '', next: '' })
  const [msg, setMsg] = useState('')
  const [err, setErr] = useState('')

  useEffect(() => {
    api('/api/settings').then(setS).catch((e) => setErr(e.message))
  }, [])

  async function save() {
    setErr('')
    setMsg('')
    try {
      await api('/api/settings', {
        method: 'PUT',
        body: {
          wgEndpoint: s.wgEndpoint,
          clientDns: s.clientDns,
          dnsIntercept: s.dnsIntercept,
          captureEnabled: s.captureEnabled,
          extraDelayMs: s.extraDelayMs,
        },
      })
      setMsg('Saved')
    } catch (e) {
      setErr(e.message)
    }
  }

  async function changePassword() {
    setErr('')
    setMsg('')
    try {
      await api('/api/password', { method: 'POST', body: pw })
      setMsg('Password updated')
      setPw({ current: '', next: '' })
    } catch (e) {
      setErr(e.message)
    }
  }

  if (!s) return <p className="muted">Loading…</p>

  return (
    <>
      <h1>Settings</h1>
      <p className="lead">Public WireGuard endpoint (host:port as clients see it), DNS, and panel password.</p>
      {err && <p className="err">{err}</p>}
      {msg && <p className="muted">{msg}</p>}

      <div className="panel-box">
        <label className="muted">Public WG endpoint (e.g. 203.0.113.10:51820)
          <input value={s.wgEndpoint || ''} onChange={(e) => setS({ ...s, wgEndpoint: e.target.value })} />
        </label>
        <label className="muted" style={{ display: 'block', marginTop: 10 }}>Client DNS pushed in .conf
          <input value={s.clientDns || ''} onChange={(e) => setS({ ...s, clientDns: e.target.value })} />
        </label>
        <label className="muted row" style={{ marginTop: 10 }}>
          <input type="checkbox" checked={!!s.dnsIntercept} onChange={(e) => setS({ ...s, dnsIntercept: e.target.checked })} />
          DNS intercept (UDP/TCP 53 → logger)
        </label>
        <label className="muted row">
          <input type="checkbox" checked={!!s.captureEnabled} onChange={(e) => setS({ ...s, captureEnabled: e.target.checked })} />
          Capture into in-memory inspector ring (~2000 events)
        </label>
        <p className="muted mono">Subnet {s.wgSubnet} · WG port {s.wgPort} · uplink {s.uplink}</p>
        <button className="primary" onClick={save}>Save settings</button>
      </div>

      <div className="panel-box">
        <h2 style={{ marginTop: 0, fontSize: '1.05rem' }}>Change password</h2>
        <label className="muted">Current
          <input type="password" value={pw.current} onChange={(e) => setPw({ ...pw, current: e.target.value })} />
        </label>
        <label className="muted" style={{ display: 'block', marginTop: 8 }}>New
          <input type="password" value={pw.next} onChange={(e) => setPw({ ...pw, next: e.target.value })} />
        </label>
        <button style={{ marginTop: 10 }} onClick={changePassword}>Update password</button>
      </div>
    </>
  )
}
