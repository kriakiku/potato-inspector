import { useEffect, useState } from 'react'
import { api } from '../api'
import { notifyError, notifySuccess } from '../toast'

export default function Ignore() {
  const [enabled, setEnabled] = useState(true)
  const [domains, setDomains] = useState([])
  const [custom, setCustom] = useState([])
  const [text, setText] = useState('')
  const [savedText, setSavedText] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  function applyPayload(d) {
    setEnabled(!!d.enabled)
    setDomains(d.domains || [])
    setCustom(Array.isArray(d.custom) ? d.custom.filter((e) => e && e.domain) : [])
    const t = typeof d.customText === 'string' ? d.customText : ''
    setText(t)
    setSavedText(t)
    setError('')
  }

  async function load() {
    applyPayload(await api('/api/ignore'))
  }

  useEffect(() => {
    load().catch((e) => notifyError(e.message))
  }, [])

  async function toggleSystem(next) {
    setBusy(true)
    try {
      applyPayload(await api('/api/ignore', { method: 'PUT', body: { enabled: next } }))
      notifySuccess(next ? 'System ignore on' : 'System ignore off')
    } catch (e) {
      notifyError(e.message)
    } finally {
      setBusy(false)
    }
  }

  async function saveText() {
    setBusy(true)
    setError('')
    try {
      applyPayload(await api('/api/ignore', { method: 'PUT', body: { customText: text } }))
      notifySuccess('Custom ignore saved')
    } catch (e) {
      setError(e.message)
      notifyError(e.message)
    } finally {
      setBusy(false)
    }
  }

  const dirty = text !== savedText

  return (
    <>
      <h1>Ignore</h1>
      <p className="lead">
        Skip MITM decrypt / inspector noise and last-mile delay for matched hosts (suffix match).
        Builtin pack covers OS/vendor domains; custom list is a simple hosts-style file —
        use it for cert-pinned apps too.
      </p>

      <div className="panel-box">
        <h2 style={{ marginTop: 0, fontSize: '1.05rem' }}>Custom domains</h2>
        <p className="muted" style={{ marginTop: 0 }}>
          Free-form hosts file. One domain per line (<span className="mono">github.com</span> or{' '}
          <span className="mono">*.github.com</span>). Prefix a domain with <span className="mono">#</span> to
          disable it. Text is saved exactly as typed.
        </p>
        <textarea
          className="ignore-hosts mono"
          rows={10}
          spellCheck={false}
          value={text}
          disabled={busy}
          onChange={(e) => {
            setText(e.target.value)
            setError('')
          }}
          placeholder={"# Example comment\n# github.com\n\ngithub.com # live"}
          aria-label="Custom ignore hosts"
        />
        {error ? <p className="ignore-domain-error">{error}</p> : null}
        <div style={{ marginTop: 8, display: 'flex', gap: 8, alignItems: 'center' }}>
          <button type="button" className="primary" disabled={busy || !dirty} onClick={saveText}>
            Save
          </button>
          {dirty ? <span className="muted" style={{ fontSize: '0.85rem' }}>Unsaved changes</span> : null}
        </div>

        <h3 style={{ marginTop: 16, marginBottom: 6, fontSize: '0.95rem' }}>
          Parsed domains ({custom.length})
        </h3>
        {custom.length === 0 ? (
          <p className="muted" style={{ margin: 0, fontSize: '0.85rem' }}>No domains yet.</p>
        ) : (
          <ul className="ignore-parsed-list mono">
            {custom.map((e) => (
              <li key={e.domain} className={e.enabled ? '' : 'is-disabled'}>
                <span className="ignore-parsed-flag">{e.enabled ? 'on' : 'off'}</span>
                *.{e.domain} <span className="muted">(+ {e.domain})</span>
                {e.comment ? <span className="muted"> — {e.comment}</span> : null}
              </li>
            ))}
          </ul>
        )}
      </div>

      <div className="panel-box" style={{ marginTop: 12 }}>
        <h2 style={{ marginTop: 0, fontSize: '1.05rem' }}>Builtin domains ({domains.length})</h2>
        <label className="form-check" style={{ marginBottom: 10 }}>
          <input
            type="checkbox"
            checked={enabled}
            disabled={busy}
            onChange={(e) => toggleSystem(e.target.checked)}
          />
          <span>
            System ignore
            <span className="muted" style={{ display: 'block', fontSize: '0.85rem', marginTop: 2 }}>
              Default on. Google / Apple / Microsoft / … pack. Shape exemption needs DNS intercept.
            </span>
          </span>
        </label>
        <p className="muted" style={{ marginTop: 0 }}>
          Matching is suffix-based: <span className="mono">foo.google.com</span> matches{' '}
          <span className="mono">google.com</span>. List is embedded in the binary (not editable).
        </p>
        <ul className="ignore-domain-list mono">
          {domains.map((d) => (
            <li key={d}>
              *.{d} <span className="muted">(+ {d})</span>
            </li>
          ))}
        </ul>
      </div>
    </>
  )
}
