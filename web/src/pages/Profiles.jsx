import { useEffect, useState } from 'react'
import { api } from '../api'
import { notifyError, notifySuccess } from '../toast'

const TIERS = ['stable', 'typical', 'poor']

export default function Profiles() {
  const [catalog, setCatalog] = useState(null)
  const [customs, setCustoms] = useState([])
  const [busy, setBusy] = useState(false)

  async function load() {
    const [cat, list] = await Promise.all([
      api('/api/catalog'),
      api('/api/profiles'),
    ])
    setCatalog(cat)
    setCustoms((list || []).filter((p) => !p.builtin && !String(p.id || '').startsWith('catalog:')))
  }

  useEffect(() => {
    load().catch((e) => notifyError(e.message))
  }, [])

  async function refreshCatalog() {
    setBusy(true)
    try {
      await api('/api/catalog/refresh', { method: 'POST', body: {} })
      notifySuccess('Catalog updated')
      await load()
    } catch (e) {
      notifyError(e.message)
    } finally {
      setBusy(false)
    }
  }

  async function apply(country, tier) {
    try {
      await api('/api/catalog/apply', {
        method: 'POST',
        body: { country, tier, probe: true },
      })
      notifySuccess(`Applied ${country} · ${tier}`)
    } catch (e) {
      notifyError(e.message)
    }
  }

  async function applyCustom(id) {
    try {
      await api('/api/profiles/apply', { method: 'POST', body: { id } })
      notifySuccess(`Applied ${id}`)
    } catch (e) {
      notifyError(e.message)
    }
  }

  async function removeCustom(id) {
    if (!confirm(`Delete ${id}?`)) return
    try {
      await api(`/api/profiles?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
      notifySuccess('Deleted')
      await load()
    } catch (e) {
      notifyError(e.message)
    }
  }

  if (!catalog) return <p className="muted">Loading…</p>

  const destIds = Object.keys(catalog.destinations || {})

  return (
    <>
      <h1>Profiles</h1>
      <p className="lead">
        Countries with speed tiers (last-mile) and RTT to Cloudflare Edge / AWS regions
        (Radar last-mile + CloudPing backbone). Apply from here or the Inspector status bar.
        MITM path rules pick the destination (cf vs aws-…).
      </p>

      <div className="row" style={{ marginBottom: 12 }}>
        <button className="primary" disabled={busy} onClick={refreshCatalog}>
          {busy ? 'Updating…' : 'Update catalog from GitHub'}
        </button>
        <span className="muted mono" style={{ fontSize: '0.8rem' }}>
          {catalog.source || '—'} · {catalog.generatedAt || ''}
        </span>
      </div>

      <div className="profile-list">
        {(catalog.countries || []).map((c) => (
          <div key={c.id} className="profile-card">
            <div className="profile-card-main">
              <div className="profile-card-title">
                <strong>{c.flag ? `${c.flag} ` : ''}{c.name}</strong>
                <span className="badge">{c.id}</span>
              </div>
              <div className="profile-metrics" style={{ marginTop: 10 }}>
                {TIERS.map((t) => {
                  const tier = c.tiers?.[t]
                  if (!tier) return null
                  return (
                    <div key={t} style={{ minWidth: '10rem' }}>
                      <div className="muted" style={{ fontSize: '0.75rem', textTransform: 'uppercase' }}>{t}</div>
                      <div className="mono" style={{ fontSize: '0.8rem' }}>
                        ↓{tier.downloadMbps} ↑{tier.uploadMbps} · loss {tier.lossPercent}%
                      </div>
                      <div className="mono muted" style={{ fontSize: '0.72rem', marginTop: 2 }}>
                        CF RTT {tier.rttToDest?.cf ?? '—'} ms
                      </div>
                      <button style={{ marginTop: 6 }} onClick={() => apply(c.id, t)}>Apply {t}</button>
                    </div>
                  )
                })}
              </div>
              <details style={{ marginTop: 12 }}>
                <summary className="muted" style={{ cursor: 'pointer', fontSize: '0.85rem' }}>RTT by destination (typical)</summary>
                <table className="table" style={{ marginTop: 8, fontSize: '0.82rem' }}>
                  <thead>
                    <tr>
                      <th>Dest</th>
                      {TIERS.map((t) => <th key={t}>{t}</th>)}
                    </tr>
                  </thead>
                  <tbody>
                    {destIds.map((d) => (
                      <tr key={d}>
                        <td>{catalog.destinations[d]?.label || d}</td>
                        {TIERS.map((t) => (
                          <td key={t} className="mono">{c.tiers?.[t]?.rttToDest?.[d] ?? '—'} ms</td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </details>
            </div>
          </div>
        ))}
      </div>

      <h2 style={{ marginTop: '2rem', fontSize: '1.1rem' }}>Custom profiles</h2>
      <p className="muted">Hand-tuned shaping (passthrough / legacy). Builtin demos remain available via API.</p>
      {customs.length === 0 ? (
        <p className="muted">No custom profiles yet.</p>
      ) : (
        <div className="profile-list">
          {customs.map((p) => (
            <div key={p.id} className="profile-card">
              <div className="profile-card-main">
                <div className="profile-card-title">
                  <strong>{p.name}</strong>
                  <span className="badge">custom</span>
                </div>
                <p className="profile-card-id mono muted">{p.id}</p>
                <div className="profile-metrics">
                  <span>{p.passthrough ? 'passthrough' : `${p.delayMs} ms · ↓${p.downloadMbps} ↑${p.uploadMbps}`}</span>
                </div>
              </div>
              <div className="profile-card-actions">
                <button className="primary" onClick={() => applyCustom(p.id)}>Apply</button>
                <button className="danger" onClick={() => removeCustom(p.id)}>Delete</button>
              </div>
            </div>
          ))}
        </div>
      )}
    </>
  )
}
