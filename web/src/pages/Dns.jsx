import { useEffect, useState } from 'react'
import { api } from '../api'
import { notifyError, notifySuccess } from '../toast'

function newRule() {
  return { id: `dns-${Date.now()}`, pattern: '*.local', ip: '10.8.0.1', enabled: true }
}

export default function Dns() {
  const [s, setS] = useState(null)

  async function load() {
    setS(await api('/api/settings'))
  }

  useEffect(() => {
    load().catch((e) => notifyError(e.message))
  }, [])

  async function save() {
    try {
      const res = await api('/api/settings', {
        method: 'PUT',
        body: {
          clientDns: s.clientDns,
          dnsZeroTtl: s.dnsZeroTtl,
          dnsRewriteRules: s.dnsRewriteRules || [],
        },
      })
      if (res?.systemDnsWarning) {
        notifySuccess(`DNS saved (container resolv.conf: ${res.systemDnsWarning})`)
      } else {
        notifySuccess('DNS settings saved')
      }
      await load()
    } catch (e) {
      notifyError(e.message)
    }
  }

  const rules = s?.dnsRewriteRules || []

  function setRules(next) {
    setS({ ...s, dnsRewriteRules: next })
  }

  return (
    <>
      <h1>DNS</h1>
      <p className="lead">Upstream resolver for the container and tunnel clients, rewrite rules, and TTL clamp.</p>

      {!s ? (
        <div className="panel-box"><p className="muted">Loading…</p></div>
      ) : (
        <>
          <div className="panel-box">
            <h2 style={{ marginTop: 0, fontSize: '1.05rem' }}>Upstream</h2>
            <label className="muted">
              Upstream DNS
              <span className="muted" style={{ display: 'block', fontSize: '0.85rem', marginTop: 2, fontWeight: 400 }}>
                Used by this container for its own lookups, and as the forwarder for tunnel queries that do not match a rewrite rule.
                Peer .conf always uses the WG gateway as DNS (tunnel traffic is intercepted).
              </span>
              <input
                className="mono"
                value={s.clientDns || ''}
                onChange={(e) => setS({ ...s, clientDns: e.target.value })}
                placeholder="1.1.1.1"
              />
            </label>
            <label className="form-check" style={{ marginTop: 14 }}>
              <input
                type="checkbox"
                checked={!!s.dnsZeroTtl}
                onChange={(e) => setS({ ...s, dnsZeroTtl: e.target.checked })}
              />
              <span>
                Force DNS TTL=0
                <span className="muted" style={{ display: 'block', fontSize: '0.85rem', marginTop: 2 }}>
                  Clamp TTL on all forwarded answers (rewrite answers always use TTL 0).
                </span>
              </span>
            </label>
            <div className="row" style={{ marginTop: 12 }}>
              <button className="primary" onClick={save}>Save</button>
            </div>
          </div>

          <div className="panel-box">
            <h2 style={{ marginTop: 0, fontSize: '1.05rem' }}>Rewrite rules</h2>
            <p className="muted" style={{ marginTop: 0, fontSize: '0.85rem' }}>
              First match wins. Patterns: <span className="mono">example.com</span> (exact) or{' '}
              <span className="mono">*.domain.com</span> / <span className="mono">*.local</span> (suffix). IPv4 only.
            </p>
            <table className="table">
              <thead>
                <tr>
                  <th>Pattern</th>
                  <th>IP</th>
                  <th>On</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {rules.map((r, i) => (
                  <tr key={r.id || i}>
                    <td>
                      <input
                        className="mono"
                        value={r.pattern || ''}
                        onChange={(e) => {
                          const next = [...rules]
                          next[i] = { ...r, pattern: e.target.value }
                          setRules(next)
                        }}
                      />
                    </td>
                    <td>
                      <input
                        className="mono"
                        value={r.ip || ''}
                        onChange={(e) => {
                          const next = [...rules]
                          next[i] = { ...r, ip: e.target.value }
                          setRules(next)
                        }}
                      />
                    </td>
                    <td>
                      <input
                        type="checkbox"
                        checked={!!r.enabled}
                        onChange={(e) => {
                          const next = [...rules]
                          next[i] = { ...r, enabled: e.target.checked }
                          setRules(next)
                        }}
                      />
                    </td>
                    <td>
                      <button
                        type="button"
                        className="danger"
                        onClick={() => setRules(rules.filter((_, j) => j !== i))}
                      >
                        Delete
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <div className="row" style={{ marginTop: 12 }}>
              <button type="button" onClick={() => setRules([...rules, newRule()])}>Add rule</button>
              <button className="primary" onClick={save}>Save</button>
            </div>
          </div>
        </>
      )}
    </>
  )
}
