import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { notifyError, notifySuccess } from '../toast'

export default function Baseline() {
  const [data, setData] = useState(null)
  const [draft, setDraft] = useState({})
  const [busy, setBusy] = useState(false)

  async function load() {
    const d = await api('/api/catalog/baseline')
    setData(d)
    const next = {}
    for (const row of d.destinations || []) {
      next[row.id] = row.rttMs ?? ''
    }
    setDraft(next)
  }

  useEffect(() => {
    load().catch((e) => notifyError(e.message))
  }, [])

  async function reProbe() {
    setBusy(true)
    try {
      await api('/api/catalog/probe', { method: 'POST', body: {} })
      notifySuccess('Re-probed (auto mode)')
      await load()
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

  async function save(pinned) {
    setBusy(true)
    try {
      const hostRtt = buildHostRtt()
      await api('/api/catalog/baseline', {
        method: 'PUT',
        body: { hostRtt, pinned },
      })
      notifySuccess(pinned ? 'Pinned baseline saved' : 'Baseline saved (auto)')
      await load()
    } catch (e) {
      notifyError(e.message)
    } finally {
      setBusy(false)
    }
  }

  async function pinCurrent() {
    setBusy(true)
    try {
      const hostRtt = data?.hostRtt || buildHostRtt()
      await api('/api/catalog/baseline', {
        method: 'PUT',
        body: { hostRtt, pinned: true },
      })
      notifySuccess('Pinned current values')
      await load()
    } catch (e) {
      notifyError(e.message)
    } finally {
      setBusy(false)
    }
  }

  async function unpin() {
    setBusy(true)
    try {
      const hostRtt = buildHostRtt()
      await api('/api/catalog/baseline', {
        method: 'PUT',
        body: { hostRtt, pinned: false },
      })
      notifySuccess('Unpinned — apply may re-probe')
      await load()
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
        Host RTT from this machine to Cloudflare and AWS endpoints. Catalog country profiles
        subtract these so last-mile delay is not stacked on your real path. Pin to keep values
        if the host moves (or to force a fixed baseline).
      </p>

      <div className="row" style={{ marginBottom: 12, flexWrap: 'wrap', gap: 8 }}>
        <span className={`badge ${data.pinned ? 'http' : ''}`}>
          {data.pinned ? 'Pinned' : 'Auto'}
        </span>
        <span className="muted mono" style={{ fontSize: '0.8rem' }}>
          last probe {data.lastProbeAt || '—'}
        </span>
        <button className="primary" disabled={busy} onClick={reProbe}>
          {busy ? 'Working…' : 'Re-probe'}
        </button>
        {data.pinned ? (
          <button disabled={busy} onClick={unpin}>Unpin</button>
        ) : (
          <button disabled={busy} onClick={pinCurrent}>Pin current</button>
        )}
        <button disabled={busy} onClick={() => save(true)}>Save as pinned</button>
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
          Re-probe always switches to Auto and overwrites with fresh TCP/HTTP samples.
          Save as pinned locks the numbers in settings across restarts and catalog apply.
        </p>
      </div>

      <p className="muted" style={{ marginTop: 16 }}>
        See also <Link to="/profiles">Profiles</Link> (country catalog) and Dashboard host CF RTT.
      </p>
    </>
  )
}
