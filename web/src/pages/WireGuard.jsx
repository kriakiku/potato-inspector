import { useEffect, useState } from 'react'
import { api } from '../api'
import { notifyError, notifySuccess } from '../toast'

function formatHandshake(ts) {
  if (!ts) return '—'
  const d = new Date(ts)
  // Go zero time serializes as 0001-01-01; treat as never.
  if (Number.isNaN(d.getTime()) || d.getUTCFullYear() < 2000) return '—'
  return d.toLocaleString()
}

export default function WireGuard() {
  const [peers, setPeers] = useState([])
  const [name, setName] = useState('')
  const [selected, setSelected] = useState(null)
  const [conf, setConf] = useState('')
  const [s, setS] = useState(null)
  const [qrPeer, setQrPeer] = useState(null) // { id, name }

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

  useEffect(() => {
    if (!qrPeer) return
    function onKey(e) {
      if (e.key === 'Escape') setQrPeer(null)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [qrPeer])

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
      if (qrPeer?.id === id) setQrPeer(null)
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
        body: { wgEndpoint: s.wgEndpoint },
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
      <p className="lead">Peers and public endpoint. DNS options live on the DNS page.</p>

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
                <td className="mono">{formatHandshake(p.lastHandshake)}</td>
                <td className="row">
                  <button type="button" onClick={() => showConf(p.id)}>Conf</button>
                  <button type="button" onClick={() => setQrPeer({ id: p.id, name: p.name })}>QR</button>
                  <button type="button" className="danger" onClick={() => revoke(p.id)}>Revoke</button>
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
        <h2 style={{ marginTop: 0, fontSize: '1.05rem' }}>Endpoint</h2>
        {!s ? (
          <p className="muted">Loading…</p>
        ) : (
          <>
            <label className="muted">Public WG endpoint (e.g. 203.0.113.10:51820)
              <input value={s.wgEndpoint || ''} onChange={(e) => setS({ ...s, wgEndpoint: e.target.value })} />
            </label>
            <p className="muted mono" style={{ marginTop: 12 }}>
              Subnet {s.wgSubnet} · WG port {s.wgPort} · uplink {s.uplink}
            </p>
            <button className="primary" onClick={saveSettings}>Save settings</button>
          </>
        )}
      </div>

      {qrPeer && (
        <div className="modal-backdrop" onClick={() => setQrPeer(null)} role="presentation">
          <div
            className="modal wg-qr-modal"
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-modal="true"
            aria-label={`WireGuard QR for ${qrPeer.name || qrPeer.id}`}
          >
            <div className="row" style={{ justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
              <h2 style={{ margin: 0, fontSize: '1.1rem' }}>
                QR · {qrPeer.name || qrPeer.id}
              </h2>
              <button type="button" onClick={() => setQrPeer(null)}>Close</button>
            </div>
            <p className="muted" style={{ marginTop: 0 }}>
              Scan with the WireGuard app on the phone.
            </p>
            <img
              className="wg-qr-img"
              src={`/api/peers/${qrPeer.id}/qr`}
              alt={`WireGuard QR code for ${qrPeer.name || qrPeer.id}`}
            />
          </div>
        </div>
      )}
    </>
  )
}
