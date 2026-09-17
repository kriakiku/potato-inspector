import { useEffect, useState } from 'react'
import { api } from '../api'

export default function Mitm() {
  const [data, setData] = useState(null)
  const [bypass, setBypass] = useState('')
  const [err, setErr] = useState('')
  const [msg, setMsg] = useState('')

  async function load() {
    const d = await api('/api/mitm')
    setData(d)
    setBypass((d.bypassSni || []).join('\n'))
  }

  useEffect(() => { load().catch((e) => setErr(e.message)) }, [])

  async function save(patch) {
    setErr('')
    setMsg('')
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
      setMsg('Saved')
      await load()
    } catch (e) {
      setErr(e.message)
    }
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
      {err && <p className="err">{err}</p>}
      {msg && <p className="muted">{msg}</p>}

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
                <td><input className="mono" value={r.hostRegex} onChange={(e) => {
                  const rules = [...data.rules]; rules[i] = { ...r, hostRegex: e.target.value }; setData({ ...data, rules })
                }} /></td>
                <td><input className="mono" value={r.pathRegex} onChange={(e) => {
                  const rules = [...data.rules]; rules[i] = { ...r, pathRegex: e.target.value }; setData({ ...data, rules })
                }} /></td>
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
    </>
  )
}
