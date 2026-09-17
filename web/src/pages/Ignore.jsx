import { useEffect, useState } from 'react'
import { api } from '../api'
import { notifyError, notifySuccess } from '../toast'

export default function Ignore() {
  const [enabled, setEnabled] = useState(true)
  const [domains, setDomains] = useState([])
  const [text, setText] = useState('')
  const [savedText, setSavedText] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function load() {
    const d = await api('/api/ignore')
    setEnabled(!!d.enabled)
    setDomains(d.domains || [])
    const t = typeof d.customText === 'string' ? d.customText : ''
    setText(t)
    setSavedText(t)
    setError('')
  }

  useEffect(() => {
    load().catch((e) => notifyError(e.message))
  }, [])

  async function toggleSystem(next) {
    setBusy(true)
    try {
      const d = await api('/api/ignore', { method: 'PUT', body: { enabled: next } })
      setEnabled(!!d.enabled)
      setDomains(d.domains || [])
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
      const d = await api('/api/ignore', { method: 'PUT', body: { customText: text } })
      const t = typeof d.customText === 'string' ? d.customText : text
      setText(t)
      setSavedText(t)
      setDomains(d.domains || [])
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
        <label className="form-check">
          <input
            type="checkbox"
            checked={enabled}
            disabled={busy}
            onChange={(e) => toggleSystem(e.target.checked)}
          />
          <span>
            System ignore
            <span className="muted" style={{ display: 'block', fontSize: '0.85rem', marginTop: 2 }}>
              Default on. Builtin Google / Apple / Microsoft / … pack. Shape exemption needs DNS intercept.
            </span>
          </span>
        </label>
      </div>

      <div className="panel-box" style={{ marginTop: 12 }}>
        <h2 style={{ marginTop: 0, fontSize: '1.05rem' }}>Custom domains</h2>
        <p className="muted" style={{ marginTop: 0 }}>
          One domain per line (<span className="mono">github.com</span> or <span className="mono">*.github.com</span>).
          Comment with <span className="mono">#</span>. Prefix a line with <span className="mono">#</span> to disable it.
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
          placeholder={'# github.com # Example — uncomment to enable\ngithub.com # live'}
          aria-label="Custom ignore hosts"
        />
        {error ? <p className="ignore-domain-error">{error}</p> : null}
        <div style={{ marginTop: 8, display: 'flex', gap: 8, alignItems: 'center' }}>
          <button type="button" className="primary" disabled={busy || !dirty} onClick={saveText}>
            Save
          </button>
          {dirty ? <span className="muted" style={{ fontSize: '0.85rem' }}>Unsaved changes</span> : null}
        </div>
      </div>

      <div className="panel-box" style={{ marginTop: 12 }}>
        <h2 style={{ marginTop: 0, fontSize: '1.05rem' }}>Builtin domains ({domains.length})</h2>
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
