import { useEffect, useState } from 'react'
import { api } from '../api'

export default function Profiles() {
  const [list, setList] = useState([])
  const [status, setStatus] = useState('')
  const [err, setErr] = useState('')
  const [custom, setCustom] = useState({
    id: '', name: '', description: '', delayMs: 50, downloadMbps: 20, uploadMbps: 8, lossPercent: 0.5, passthrough: false,
  })

  async function load() {
    setList(await api('/api/profiles'))
  }

  useEffect(() => { load().catch((e) => setErr(e.message)) }, [])

  async function apply(id) {
    setErr('')
    try {
      const res = await api('/api/profiles/apply', { method: 'POST', body: { id } })
      setStatus(res.status)
    } catch (e) {
      setErr(e.message)
    }
  }

  async function saveCustom() {
    await api('/api/profiles', { method: 'POST', body: custom })
    await load()
  }

  return (
    <>
      <h1>Profiles</h1>
      <p className="lead">
        Delay is <strong>one-way</strong> ms (RTT ≈ 2×). Bandwidth is download (internet→client) / upload (client→internet).
      </p>
      {err && <p className="err">{err}</p>}
      {status && <p className="muted mono">Applied: {status}</p>}
      <table className="table">
        <thead>
          <tr>
            <th>Profile</th>
            <th>Delay</th>
            <th>Down / Up</th>
            <th>Loss</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {list.map((p) => (
            <tr key={p.id}>
              <td>
                <div>{p.name}</div>
                <div className="muted" style={{ fontSize: '0.85rem' }}>{p.description}</div>
                <div className="mono muted">{p.id}{p.builtin ? ' · builtin' : ''}</div>
              </td>
              <td className="mono">
                {p.passthrough ? '—' : `${p.delayMs} ms (~${p.delayMs * 2} ms ping)`}
              </td>
              <td className="mono">{p.passthrough ? '—' : `${p.downloadMbps} / ${p.uploadMbps} Mbit/s`}</td>
              <td className="mono">{p.passthrough ? '—' : `${p.lossPercent}%`}</td>
              <td><button className="primary" onClick={() => apply(p.id)}>Apply</button></td>
            </tr>
          ))}
        </tbody>
      </table>

      <div className="panel-box" style={{ marginTop: 20 }}>
        <h2 style={{ marginTop: 0, fontSize: '1.05rem' }}>Custom profile</h2>
        <div className="grid">
          {['id', 'name', 'description'].map((k) => (
            <label key={k} className="muted">
              {k}
              <input value={custom[k]} onChange={(e) => setCustom({ ...custom, [k]: e.target.value })} />
            </label>
          ))}
          <label className="muted">delayMs (one-way)
            <input type="number" value={custom.delayMs} onChange={(e) => setCustom({ ...custom, delayMs: +e.target.value })} />
          </label>
          <label className="muted">downloadMbps
            <input type="number" step="0.1" value={custom.downloadMbps} onChange={(e) => setCustom({ ...custom, downloadMbps: +e.target.value })} />
          </label>
          <label className="muted">uploadMbps
            <input type="number" step="0.1" value={custom.uploadMbps} onChange={(e) => setCustom({ ...custom, uploadMbps: +e.target.value })} />
          </label>
          <label className="muted">lossPercent
            <input type="number" step="0.1" value={custom.lossPercent} onChange={(e) => setCustom({ ...custom, lossPercent: +e.target.value })} />
          </label>
        </div>
        <div className="row" style={{ marginTop: 10 }}>
          <button className="primary" onClick={saveCustom}>Save custom</button>
        </div>
      </div>
    </>
  )
}
