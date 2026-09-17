import { useEffect, useState } from 'react'
import { api } from '../api'

export default function Peers() {
  const [peers, setPeers] = useState([])
  const [name, setName] = useState('')
  const [selected, setSelected] = useState(null)
  const [conf, setConf] = useState('')
  const [err, setErr] = useState('')

  async function load() {
    setPeers(await api('/api/peers'))
  }

  useEffect(() => { load().catch((e) => setErr(e.message)) }, [])

  async function add() {
    await api('/api/peers', { method: 'POST', body: { name: name || 'phone' } })
    setName('')
    await load()
  }

  async function revoke(id) {
    if (!confirm('Revoke this peer?')) return
    await api(`/api/peers/${id}`, { method: 'DELETE' })
    setSelected(null)
    setConf('')
    await load()
  }

  async function showConf(id) {
    const res = await api(`/api/peers/${id}/conf`)
    setConf(await res.text())
    setSelected(id)
  }

  return (
    <>
      <h1>Peers</h1>
      <p className="lead">Create WireGuard clients (full tunnel 0.0.0.0/0). Download .conf or scan QR.</p>
      {err && <p className="err">{err}</p>}
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
        <div className="panel-box" style={{ marginTop: 16 }}>
          <div className="row" style={{ justifyContent: 'space-between' }}>
            <strong>Client config</strong>
            <a href={`/api/peers/${selected}/conf`} download>Download .conf</a>
          </div>
          <pre className="detail">{conf}</pre>
        </div>
      )}
    </>
  )
}
