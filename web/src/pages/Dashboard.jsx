import { useEffect, useState } from 'react'
import { api } from '../api'

export default function Dashboard() {
  const [st, setSt] = useState(null)
  const [err, setErr] = useState('')

  async function load() {
    try {
      setSt(await api('/api/status'))
      setErr('')
    } catch (e) {
      setErr(e.message)
    }
  }

  useEffect(() => {
    load()
    const t = setInterval(load, 4000)
    return () => clearInterval(t)
  }, [])

  if (!st) return <p className="muted">{err || 'Loading…'}</p>

  const rtt = st.profile?.delayMs ? st.profile.delayMs * 2 : 0

  return (
    <>
      <h1>Dashboard</h1>
      <p className="lead">Live status for WireGuard peers, last-mile shaping, MITM, and capture.</p>
      <div className="grid">
        <div className="stat">
          <div className="label">Active profile</div>
          <div className="value">{st.activeProfileId}</div>
        </div>
        <div className="stat">
          <div className="label">One-way / approx RTT</div>
          <div className="value">{st.profile?.delayMs ?? 0} ms / ~{rtt} ms</div>
        </div>
        <div className="stat">
          <div className="label">MITM</div>
          <div className="value">{st.mitmEnabled ? 'on' : 'off'}</div>
        </div>
        <div className="stat">
          <div className="label">Capture</div>
          <div className="value">{st.captureEnabled ? 'on' : 'off'}</div>
        </div>
        <div className="stat">
          <div className="label">Peers</div>
          <div className="value">{st.peerCount} ({st.peersHandshaking} handshake)</div>
        </div>
        <div className="stat">
          <div className="label">DNS intercept</div>
          <div className="value">{st.dnsIntercept ? 'on' : 'off'}</div>
        </div>
      </div>
      <div className="panel-box" style={{ marginTop: 16 }}>
        <div className="label muted">qdisc</div>
        <div className="mono">{st.qdisc}</div>
        <pre className="detail" style={{ marginTop: 8 }}>{st.qdiscDump || '(none)'}</pre>
      </div>
      <p className="muted">{st.inspectorNote}</p>
    </>
  )
}
