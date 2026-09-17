import { useEffect, useState } from 'react'
import { api } from '../api'
import RegexEditorModal from '../components/RegexEditorModal'
import { notifyError, notifySuccess } from '../toast'

export default function Mitm() {
  const [data, setData] = useState(null)
  const [bypass, setBypass] = useState('')
  const [regexEdit, setRegexEdit] = useState(null)

  async function load() {
    const d = await api('/api/mitm')
    setData(d)
    setBypass((d.bypassSni || []).join('\n'))
  }

  useEffect(() => { load().catch((e) => notifyError(e.message)) }, [])

  async function save(patch = {}) {
    try {
      const next = { ...data, ...patch }
      await api('/api/mitm', {
        method: 'PUT',
        body: {
          enabled: next.enabled,
          bypassSni: bypass.split('\n').map((s) => s.trim()).filter(Boolean),
          rules: next.rules,
          extraDelayMs: next.extraDelayMs,
          forceDisableCache: next.forceDisableCache,
        },
      })
      notifySuccess('Saved')
      await load()
    } catch (e) {
      notifyError(e.message)
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
        Install the CA on phones/laptops. UniFi as WG client does not install the CA.
        Cert pinning will fail unless the SNI is bypassed. UDP/443 (QUIC) is dropped when MITM is on.
      </p>

      <div className="panel-box">
        <div className="row">
          <button className="primary" onClick={() => save({ enabled: !data.enabled })}>
            {data.enabled ? 'Disable MITM' : 'Enable MITM'}
          </button>
          <a href="/api/mitm/ca.crt" download="potatoinspector-ca.crt">
            <button type="button">Download CA cert</button>
          </a>
          <span className="badge">{data.running ? 'proxy running' : 'proxy stopped'}</span>
        </div>
        <p className="muted" style={{ marginTop: 10 }}>
          iOS: Settings → General → VPN & Device Management → install profile, then enable full trust under Certificate Trust Settings.
          Android: install CA as user cert (varies by OEM); some apps ignore user CAs.
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
              Applies to every request through MITM — strips If-None-Match / If-Modified-Since so origins return full bodies (not 304), and weakens response cache validators.
            </span>
          </span>
        </label>
      </div>

      <div className="panel-box">
        <label className="muted">Global extra delay (ms) added on top of path-dest delta when a rule matches (0 = path delta only)
          <input
            type="number"
            style={{ maxWidth: 160, display: 'block', marginTop: 6 }}
            value={data.extraDelayMs}
            onChange={(e) => setData({ ...data, extraDelayMs: +e.target.value })}
          />
        </label>
        <button style={{ marginTop: 8 }} onClick={() => save({})}>Save delay</button>
      </div>

      <div className="panel-box">
        <h2 style={{ marginTop: 0, fontSize: '1.05rem' }}>Path rules</h2>
        <p className="muted" style={{ marginTop: 0 }}>
          Dest sets the remote endpoint (CF edge vs AWS region). Extra delay = path RTT delta vs CF (plus global extra). Delay override &gt; 0 replaces that calculation.
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
                  <div className="regex-field">
                    <input
                      className="mono"
                      readOnly
                      value={r.hostRegex}
                      title="Click to edit"
                      onClick={() => openRegex('host', i)}
                      onFocus={() => openRegex('host', i)}
                    />
                    <button type="button" onClick={() => openRegex('host', i)}>Edit</button>
                  </div>
                </td>
                <td>
                  <div className="regex-field">
                    <input
                      className="mono"
                      readOnly
                      value={r.pathRegex}
                      title="Click to edit"
                      onClick={() => openRegex('path', i)}
                      onFocus={() => openRegex('path', i)}
                    />
                    <button type="button" onClick={() => openRegex('path', i)}>Edit</button>
                  </div>
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
        <div className="row">
          <button onClick={() => setData({
            ...data,
            rules: [...(data.rules || []), { id: `rule-${Date.now()}`, name: 'new', hostRegex: '.*', pathRegex: '^/api', dest: 'aws-eu-central-1', extraDelayMs: 0, enabled: true }],
          })}>Add rule</button>
          <button className="primary" onClick={() => save({})}>Save rules</button>
        </div>
      </div>

      <div className="panel-box">
        <label className="muted">Bypass SNI (one per line — no MITM, for pinned apps)
          <textarea rows={4} style={{ marginTop: 6 }} value={bypass} onChange={(e) => setBypass(e.target.value)} />
        </label>
        <button style={{ marginTop: 8 }} className="primary" onClick={() => save({})}>Save bypass</button>
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
