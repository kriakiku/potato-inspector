import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import RegexEditorModal from '../components/RegexEditorModal'
import { notifyError, notifySuccess } from '../toast'

export default function Mitm() {
  const [data, setData] = useState(null)
  const [regexEdit, setRegexEdit] = useState(null)
  const [busy, setBusy] = useState(false)

  async function load() {
    setData(await api('/api/mitm'))
  }

  useEffect(() => { load().catch((e) => notifyError(e.message)) }, [])

  async function save(patch = {}) {
    try {
      const next = { ...data, ...patch }
      await api('/api/mitm', {
        method: 'PUT',
        body: {
          rules: next.rules,
          forceDisableCache: next.forceDisableCache,
        },
      })
      notifySuccess('Saved')
      await load()
    } catch (e) {
      notifyError(e.message)
    }
  }

  async function regenerateCA() {
    if (!confirm('Regenerate CA? All clients must re-install the new certificate. MITM will restart briefly.')) {
      return
    }
    setBusy(true)
    try {
      const res = await api('/api/mitm/ca/regenerate', { method: 'POST', body: {} })
      notifySuccess(res.note || 'CA regenerated — download and re-install on clients')
      await load()
    } catch (e) {
      notifyError(e.message)
    } finally {
      setBusy(false)
    }
  }

  function openRegex(kind, index) {
    const r = data.rules[index]
    setRegexEdit({
      kind,
      index,
      pattern: kind === 'host' ? r.hostRegex : r.pathRegex,
      title: `${kind === 'host' ? 'Host regex' : 'Path regex'}${r.name ? ` · ${r.name}` : ''}`,
      ignoreCase: kind === 'host',
      placeholder: kind === 'host' ? 'api.example.com' : '/api/v1/users',
    })
  }

  if (!data) return <p className="muted">Loading…</p>

  const destEntries = Object.entries(data.destinations || { cf: { label: 'Cloudflare Edge' } })
    .sort(([a], [b]) => (a === 'cf' ? -1 : b === 'cf' ? 1 : a.localeCompare(b)))

  return (
    <>
      <h1>MITM</h1>
      <p className="lead">
        Transparent MITM decrypts HTTPS on this tunnel for path-based extra delay and the inspector.
        MITM stays on whenever WireGuard is up.
      </p>

      <div className="panel-box">
        <div className="row">
          <a href="/api/mitm/ca.crt" download="potatoinspector-ca.crt">
            <button type="button" className="primary">Download CA cert</button>
          </a>
          <button
            type="button"
            className="danger"
            disabled={busy}
            onClick={regenerateCA}
          >
            {busy ? 'Regenerating…' : 'Regenerate CA'}
          </button>
          <span className="badge">{data.running ? 'proxy running' : 'proxy starting…'}</span>
        </div>
        <p className="muted" style={{ marginTop: 10 }}>
          MITM is always on for this tunnel (QUIC/UDP 443 dropped). Install and trust the CA on phones/laptops.
          UniFi as WG client does not install the CA. After regenerating, remove the old CA and install the new one.
          Apps with cert pinning will fail unless their domains are on the{' '}
          <Link to="/ignore">Ignore</Link> list (no MITM decrypt + no last-mile delay).
        </p>
        <p className="muted" style={{ marginTop: 6 }}>
          Easiest on a tunnel client: open <a href="http://potato.local" target="_blank" rel="noreferrer"><strong>http://potato.local</strong></a>{' '}
          — OS-specific install steps and CA download. Once the CA is trusted, that page switches to the live Share notepad
          (HTTPS API at <code>potato-share.local</code>). See also{' '}
          <Link to="/docs/ca-android">Docs → CA</Link>.
        </p>
      </div>

      <div className="panel-box">
        <label className="form-check">
          <input
            type="checkbox"
            checked={!!data.forceDisableCache}
            onChange={(e) => {
              const on = e.target.checked
              setData({ ...data, forceDisableCache: on })
              save({ forceDisableCache: on })
            }}
          />
          <span>
            Force disable cache (all requests)
            <span className="muted" style={{ display: 'block', fontSize: '0.85rem', marginTop: 2 }}>
              Applies to every request through MITM — strips If-None-Match / If-Modified-Since so origins return full bodies (not 304); drops ETag / Expires / Last-Modified on responses and rewrites Cache-Control to no-store, no-cache, must-revalidate.
            </span>
          </span>
        </label>
      </div>

      <div className="panel-box">
        <h2 style={{ marginTop: 0, fontSize: '1.05rem' }}>Path rules</h2>
        <p className="muted" style={{ marginTop: 0 }}>
          Dest sets the remote endpoint (CF edge vs AWS region). Extra delay = path RTT delta vs CF (one-way).
          Delay override &gt; 0 replaces that calculation.
        </p>
        <table className="table">
          <thead>
            <tr><th>Name</th><th>Host regex</th><th>Path regex</th><th>Dest</th><th>Delay override</th><th>On</th></tr>
          </thead>
          <tbody>
            {(data.rules || []).map((r, i) => (
              <tr key={r.id || i}>
                <td><input value={r.name} onChange={(e) => {
                  const rules = [...data.rules]; rules[i] = { ...r, name: e.target.value }; setData({ ...data, rules })
                }} /></td>
                <td>
                  <input
                    className="mono"
                    readOnly
                    value={r.hostRegex}
                    title="Click to edit"
                    onClick={() => openRegex('host', i)}
                    onFocus={() => openRegex('host', i)}
                  />
                </td>
                <td>
                  <input
                    className="mono"
                    readOnly
                    value={r.pathRegex}
                    title="Click to edit"
                    onClick={() => openRegex('path', i)}
                    onFocus={() => openRegex('path', i)}
                  />
                </td>
                <td>
                  <select
                    value={r.dest || 'cf'}
                    onChange={(e) => {
                      const rules = [...data.rules]; rules[i] = { ...r, dest: e.target.value }; setData({ ...data, rules })
                    }}
                    style={{ width: 'auto', minWidth: '9rem' }}
                  >
                    {destEntries.map(([id, d]) => (
                      <option key={id} value={id}>{d.label || id}</option>
                    ))}
                  </select>
                </td>
                <td><input type="number" value={r.extraDelayMs} onChange={(e) => {
                  const rules = [...data.rules]; rules[i] = { ...r, extraDelayMs: +e.target.value }; setData({ ...data, rules })
                }} /></td>
                <td><input type="checkbox" checked={r.enabled} onChange={(e) => {
                  const rules = [...data.rules]; rules[i] = { ...r, enabled: e.target.checked }; setData({ ...data, rules })
                }} /></td>
              </tr>
            ))}
          </tbody>
        </table>
        <div className="row" style={{ marginTop: 12 }}>
          <button onClick={() => setData({
            ...data,
            rules: [...(data.rules || []), { id: `rule-${Date.now()}`, name: 'new', hostRegex: '.*', pathRegex: '^/api', dest: 'aws-eu-central-1', extraDelayMs: 0, enabled: true }],
          })}>Add rule</button>
          <button className="primary" onClick={() => save({})}>Save rules</button>
        </div>
      </div>

      <RegexEditorModal
        open={!!regexEdit}
        title={regexEdit?.title || 'Regex'}
        initialPattern={regexEdit?.pattern || ''}
        ignoreCase={!!regexEdit?.ignoreCase}
        samplePlaceholder={regexEdit?.placeholder}
        onClose={() => setRegexEdit(null)}
        onApply={(pattern) => {
          if (!regexEdit) return
          const rules = [...data.rules]
          const r = { ...rules[regexEdit.index] }
          if (regexEdit.kind === 'host') r.hostRegex = pattern
          else r.pathRegex = pattern
          rules[regexEdit.index] = r
          setData({ ...data, rules })
          setRegexEdit(null)
        }}
      />
    </>
  )
}
