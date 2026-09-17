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

  async function save(patch) {
    try {
      await api('/api/mitm', {
        method: 'PUT',
        body: {
          ...patch,
          bypassSni: bypass.split('\n').map((s) => s.trim()).filter(Boolean),
          rules: data.rules,
          extraDelayMs: data.extraDelayMs,
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
        <label className="muted">Extra origin delay (ms) for matching API paths
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
        <p className="muted" style={{ marginTop: 0 }}>Click a Host/Path regex field (or Edit) to open the regex playground.</p>
        <table className="table">
          <thead>
            <tr><th>Name</th><th>Host regex</th><th>Path regex</th><th>Delay override</th><th>On</th></tr>
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
            rules: [...(data.rules || []), { id: `rule-${Date.now()}`, name: 'new', hostRegex: '.*', pathRegex: '^/api', extraDelayMs: 0, enabled: true }],
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
