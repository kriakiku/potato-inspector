import { useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import {
  isTooCloseToBaseline,
  sortCountries,
  tierLabel,
  TOO_CLOSE_TOOLTIP,
} from '../countries'
import { notifyError, notifySuccess } from '../toast'

const TIERS = ['stable', 'typical', 'poor']

export default function Profiles() {
  const [catalog, setCatalog] = useState(null)
  const [favorites, setFavorites] = useState([])
  const [hostCfRtt, setHostCfRtt] = useState(0)
  const [busy, setBusy] = useState(false)

  async function load() {
    const [cat, st] = await Promise.all([
      api('/api/catalog'),
      api('/api/status'),
    ])
    setCatalog(cat)
    setFavorites(st.favoriteCountries || [])
    setHostCfRtt(st.hostRtt?.cf || 0)
  }

  useEffect(() => {
    load().catch((e) => notifyError(e.message))
  }, [])

  const countries = useMemo(
    () => sortCountries(catalog?.countries, favorites, hostCfRtt),
    [catalog, favorites, hostCfRtt],
  )

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

  async function toggleFavorite(id) {
    const next = favorites.includes(id)
      ? favorites.filter((x) => x !== id)
      : [...favorites, id]
    try {
      const res = await api('/api/settings', {
        method: 'PUT',
        body: { favoriteCountries: next },
      })
      setFavorites(res.favoriteCountries || next)
    } catch (e) {
      notifyError(e.message)
    }
  }

  async function apply(country, tier) {
    try {
      await api('/api/catalog/apply', {
        method: 'POST',
        body: { country, tier, probe: true },
      })
      notifySuccess(`Applied ${country} · ${tier}`)
      await load()
    } catch (e) {
      notifyError(e.message)
    }
  }

  if (!catalog) return <p className="muted">Loading…</p>

  const destIds = Object.keys(catalog.destinations || {})
  const favSet = new Set(favorites)
  const tooCloseCount = countries.filter((c) => isTooCloseToBaseline(c, hostCfRtt)).length

  return (
    <>
      <h1>Profiles</h1>
      <p className="lead">
        Countries with speed tiers (last-mile) and RTT to Cloudflare Edge / AWS regions
        (Radar last-mile + CloudPing backbone). Star favorites to pin them at the top here
        and in the Inspector. Locations marked 💩 are as fast or faster than your host
        baseline (CF {hostCfRtt || '—'} ms) — delay would clamp to 0. MITM path rules pick dest.
      </p>

      <div className="row" style={{ marginBottom: 12 }}>
        <button className="primary" disabled={busy} onClick={refreshCatalog}>
          {busy ? 'Updating…' : 'Update catalog from GitHub'}
        </button>
        <span className="muted mono" style={{ fontSize: '0.8rem' }}>
          {catalog.source || '—'} · {catalog.generatedAt || ''}
          {favorites.length ? ` · ${favorites.length} ⭐` : ''}
          {tooCloseCount ? ` · ${tooCloseCount} 💩` : ''}
        </span>
      </div>

      <div className="profile-list">
        {countries.map((c) => {
          const starred = favSet.has(c.id)
          const tooClose = isTooCloseToBaseline(c, hostCfRtt)
          return (
            <div
              key={c.id}
              className={`profile-card ${starred ? 'profile-card-fav' : ''} ${tooClose ? 'profile-card-tooclose' : ''}`}
              title={tooClose ? TOO_CLOSE_TOOLTIP : undefined}
            >
              <div className="profile-card-main">
                <div className="profile-card-title">
                  <button
                    type="button"
                    className={`fav-star ${starred ? 'on' : ''}`}
                    title={starred ? 'Remove from favorites' : 'Add to favorites'}
                    aria-label={starred ? 'Unfavorite' : 'Favorite'}
                    aria-pressed={starred}
                    onClick={() => toggleFavorite(c.id)}
                  >
                    {starred ? '★' : '☆'}
                  </button>
                  <strong>{c.flag ? `${c.flag} ` : ''}{c.name}</strong>
                  <span className="badge">{c.id}</span>
                  {tooClose && (
                    <span
                      className="too-close-mark"
                      title={TOO_CLOSE_TOOLTIP}
                      aria-label={TOO_CLOSE_TOOLTIP}
                    >
                      ≤ baseline · 💩
                    </span>
                  )}
                </div>
                <div className="profile-metrics" style={{ marginTop: 10 }}>
                  {TIERS.map((t) => {
                    const tier = c.tiers?.[t]
                    if (!tier) return null
                    return (
                      <div key={t} style={{ minWidth: '10rem' }}>
                        <div className="muted" style={{ fontSize: '0.75rem', textTransform: 'uppercase' }}>
                          {tierLabel(t)}
                        </div>
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
                        {TIERS.map((t) => <th key={t}>{tierLabel(t)}</th>)}
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
          )
        })}
      </div>
    </>
  )
}
