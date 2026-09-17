import { useEffect, useState } from 'react'
import { api } from '../api'
import { notifyError, notifySuccess } from '../toast'

export default function Settings() {
  const [s, setS] = useState(null)

  useEffect(() => {
    api('/api/settings').then(setS).catch((e) => notifyError(e.message))
  }, [])

  async function save() {
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
      notifySuccess('Settings saved')
    } catch (e) {
      notifyError(e.message)
    }
  }

  if (!s) return <p className="muted">Loading…</p>

  return (
    <>
      <h1>Settings</h1>
      <p className="lead">Public WireGuard endpoint (host:port as clients see it) and DNS options.</p>

      <div className="panel-box">
        <label className="muted">Public WG endpoint (e.g. 203.0.113.10:51820)
          <input value={s.wgEndpoint || ''} onChange={(e) => setS({ ...s, wgEndpoint: e.target.value })} />
        </label>
        <label className="muted" style={{ display: 'block', marginTop: 10 }}>Client DNS pushed in .conf
          <input value={s.clientDns || ''} onChange={(e) => setS({ ...s, clientDns: e.target.value })} />
        </label>
        <label className="form-check" style={{ marginTop: 12 }}>
          <input type="checkbox" checked={!!s.dnsIntercept} onChange={(e) => setS({ ...s, dnsIntercept: e.target.checked })} />
          <span className="muted">DNS intercept (UDP/TCP 53 → logger)</span>
        </label>
        <label className="form-check" style={{ marginTop: 8 }}>
          <input type="checkbox" checked={!!s.captureEnabled} onChange={(e) => setS({ ...s, captureEnabled: e.target.checked })} />
          <span className="muted">Capture into in-memory inspector ring (~2000 events)</span>
        </label>
        <p className="muted mono">Subnet {s.wgSubnet} · WG port {s.wgPort} · uplink {s.uplink}</p>
        <button className="primary" onClick={save}>Save settings</button>
      </div>
    </>
  )
}
