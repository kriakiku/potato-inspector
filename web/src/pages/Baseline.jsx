import { useEffect, useState } from 'react'
import { api } from '../api'
import { notifyError, notifySuccess } from '../toast'

export default function Baseline() {
  const [data, setData] = useState(null)
  const [draft, setDraft] = useState({})
  const [busy, setBusy] = useState(false)

  function applyPayload(d) {
    setData(d)
    const next = {}
    for (const row of d.destinations || []) {
      next[row.id] = row.rttMs ?? ''
    }
    setDraft(next)
  }

  async function load() {
    const d = await api('/api/catalog/baseline')
    const empty = !d.hostRtt || Object.keys(d.hostRtt).length === 0
    if (empty) {
      applyPayload(await api('/api/catalog/probe', { method: 'POST', body: {} }))
      return
    }
    applyPayload(d)
  }

  useEffect(() => {
    load().catch((e) => notifyError(e.message))
  }, [])

  async function runTest() {
    setBusy(true)
    try {
      applyPayload(await api('/api/catalog/probe', { method: 'POST', body: {} }))
      notifySuccess('Baseline measured')
    } catch (e) {
      notifyError(e.message)
    } finally {
      setBusy(false)
    }
  }

  function buildHostRtt() {
    const out = {}
    for (const [id, v] of Object.entries(draft)) {
      if (v === '' || v == null) continue
      const n = Number(v)
      if (!Number.isFinite(n) || n < 0) {
        throw new Error(`Invalid RTT for ${id}`)
      }
      out[id] = Math.round(n)
    }
    return out
  }

  async function save() {
    setBusy(true)
    try {
      applyPayload(await api('/api/catalog/baseline', {
        method: 'PUT',
        body: { hostRtt: buildHostRtt() },
      }))
      notifySuccess('Baseline saved')
    } catch (e) {
      notifyError(e.message)
    } finally {
      setBusy(false)
    }
  }

  if (!data) return <p className="muted">Loading…</p>

  return (
    <>
      <h1>Baseline</h1>
      <p className="lead">
        Host RTT from this machine to Cloudflare and AWS. Country profiles subtract these so
        last-mile delay is not stacked on your real path. Values live in settings and survive
        container restarts; run Test again only if the host moved.
      </p>

      <div className="row" style={{ marginBottom: 12, flexWrap: 'wrap', gap: 8 }}>
        <button className="primary" disabled={busy} onClick={runTest}>
          {busy ? 'Working…' : 'Test'}
        </button>
        <button disabled={busy} onClick={save}>Save</button>
        <span className="muted mono" style={{ fontSize: '0.8rem' }}>
          last probe {data.lastProbeAt || '—'}
        </span>
      </div>

      <div className="panel-box">
        <table className="table">
          <thead>
            <tr>
              <th>Dest</th>
              <th>Label</th>
              <th>Target</th>
              <th>RTT (ms)</th>
            </tr>
          </thead>
          <tbody>
            {(data.destinations || []).map((row) => (
              <tr key={row.id}>
                <td className="mono">{row.id}</td>
                <td>{row.label || '—'}</td>
                <td className="mono" style={{ fontSize: '0.85rem' }}>{row.target || '—'}</td>
                <td>
                  <input
                    type="number"
                    min={0}
                    style={{ maxWidth: 100 }}
                    value={draft[row.id] ?? ''}
                    onChange={(e) => setDraft({ ...draft, [row.id]: e.target.value })}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <p className="muted" style={{ marginTop: 10, marginBottom: 0 }}>
          Test measures TCP RTT from this host. Save stores manual edits. Changing a country
          profile does not re-measure — the host did not move.
        </p>
      </div>
    </>
  )
}
