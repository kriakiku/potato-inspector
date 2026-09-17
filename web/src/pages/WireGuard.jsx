import { useEffect, useState } from 'react'
import { api } from '../api'
import { notifyError, notifySuccess } from '../toast'

export default function WireGuard() {
  const [peers, setPeers] = useState([])
  const [name, setName] = useState('')
  const [selected, setSelected] = useState(null)
  const [conf, setConf] = useState('')
  const [s, setS] = useState(null)

  async function loadPeers() {
    setPeers(await api('/api/peers'))
  }

  async function loadSettings() {
    setS(await api('/api/settings'))
  }

  useEffect(() => {
    loadPeers().catch((e) => notifyError(e.message))
    loadSettings().catch((e) => notifyError(e.message))
  }, [])

  async function add() {
    try {
      await api('/api/peers', { method: 'POST', body: { name: name || 'phone' } })
      setName('')
      notifySuccess('Peer added')
      await loadPeers()
    } catch (e) {
      notifyError(e.message)
    }
  }

  async function revoke(id) {
    if (!confirm('Revoke this peer?')) return
    try {
      await api(`/api/peers/${id}`, { method: 'DELETE' })
      setSelected(null)
      setConf('')
      notifySuccess('Peer revoked')
      await loadPeers()
    } catch (e) {
      notifyError(e.message)
    }
  }

  async function showConf(id) {
    try {
      const res = await api(`/api/peers/${id}/conf`)
      setConf(await res.text())
      setSelected(id)
    } catch (e) {
      notifyError(e.message)
    }
  }

  async function saveSettings() {
    try {
      await api('/api/settings', {
        method: 'PUT',
        body: {
          wgEndpoint: s.wgEndpoint,
          clientDns: s.clientDns,
          dnsIntercept: s.dnsIntercept,
        },
      })
      notifySuccess('Settings saved')
      await loadSettings()
    } catch (e) {
      notifyError(e.message)
    }
  }

  return (
    <>
      <h1>WireGuard</h1>
      <p className="lead">Peers, public endpoint, and DNS options for the tunnel.</p>

      <div className="panel-box">
        <h2 style={{ marginTop: 0, fontSize: '1.05rem' }}>Peers</h2>
        <p className="muted" style={{ marginTop: 0 }}>
          Create WireGuard clients (full tunnel 0.0.0.0/0). Download .conf or scan QR.
        </p>
        <div className="row" style={{ marginBottom: 12 }}>
          <input style={{ maxWidth: 220 }} placeholder="Name" value={name} onChange={(e) => setName(e.target.value)} />
          <button className="primary" onClick={add}>Add peer</button>
        </div>
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>IP</th>
              <th>Handshake</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {peers.map((p) => (
              <tr key={p.id}>
                <td>{p.name}</td>
                <td className="mono">{p.allowedIP}</td>
                <td className="mono">{p.lastHandshake ? new Date(p.lastHandshake).toLocaleString() : '—'}</td>
                <td className="row">
                  <button onClick={() => showConf(p.id)}>Conf</button>
                  <a href={`/api/peers/${p.id}/qr`} target="_blank" rel="noreferrer">
                    <button type="button">QR</button>
                  </a>
                  <button className="danger" onClick={() => revoke(p.id)}>Revoke</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {selected && (
          <div style={{ marginTop: 16 }}>
            <div className="row" style={{ justifyContent: 'space-between' }}>
              <strong>Client config</strong>
              <a href={`/api/peers/${selected}/conf`} download>Download .conf</a>
            </div>
            <pre className="detail">{conf}</pre>
          </div>
        )}
      </div>

      <div className="panel-box">
        <h2 style={{ marginTop: 0, fontSize: '1.05rem' }}>Endpoint &amp; DNS</h2>
        {!s ? (
          <p className="muted">Loading…</p>
        ) : (
          <>
            <label className="muted">Public WG endpoint (e.g. 203.0.113.10:51820)
              <input value={s.wgEndpoint || ''} onChange={(e) => setS({ ...s, wgEndpoint: e.target.value })} />
            </label>
            <label className="muted" style={{ display: 'block', marginTop: 10 }}>Client DNS pushed in .conf
              <input value={s.clientDns || ''} onChange={(e) => setS({ ...s, clientDns: e.target.value })} />
            </label>
            <label className="form-check" style={{ marginTop: 12 }}>
              <input
                type="checkbox"
                checked={!!s.dnsIntercept}
                onChange={(e) => setS({ ...s, dnsIntercept: e.target.checked })}
              />
              <span className="muted">DNS intercept (UDP/TCP 53 → logger)</span>
            </label>
            <p className="muted mono">Subnet {s.wgSubnet} · WG port {s.wgPort} · uplink {s.uplink}</p>
            <button className="primary" onClick={saveSettings}>Save settings</button>
          </>
        )}
      </div>
    </>
  )
}
