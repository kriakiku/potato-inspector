import { useEffect, useMemo, useState } from 'react'
import { api } from '../api'

function tokenizeRegex(pattern) {
  const tokens = []
  let i = 0
  while (i < pattern.length) {
    const c = pattern[i]
    if (c === '\\' && i + 1 < pattern.length) {
      tokens.push({ t: 'esc', v: pattern.slice(i, i + 2) })
      i += 2
      continue
    }
    if (c === '[') {
      let j = i + 1
      if (j < pattern.length && pattern[j] === '^') j++
      if (j < pattern.length && pattern[j] === ']') j++
      while (j < pattern.length && pattern[j] !== ']') {
        if (pattern[j] === '\\' && j + 1 < pattern.length) j += 2
        else j++
      }
      if (j < pattern.length) j++
      tokens.push({ t: 'class', v: pattern.slice(i, j) })
      i = j
      continue
    }
    if ('()'.includes(c)) {
      tokens.push({ t: 'group', v: c })
      i++
      continue
    }
    if (c === '|') {
      tokens.push({ t: 'alt', v: c })
      i++
      continue
    }
    if ('^$'.includes(c)) {
      tokens.push({ t: 'anchor', v: c })
      i++
      continue
    }
    if ('*+?'.includes(c)) {
      tokens.push({ t: 'quant', v: c })
      i++
      continue
    }
    if (c === '{') {
      let j = i + 1
      while (j < pattern.length && /[0-9,]/.test(pattern[j])) j++
      if (j < pattern.length && pattern[j] === '}') {
        tokens.push({ t: 'quant', v: pattern.slice(i, j + 1) })
        i = j + 1
        continue
      }
    }
    tokens.push({ t: 'lit', v: c })
    i++
  }
  return tokens
}

function HighlightedPattern({ value }) {
  const tokens = useMemo(() => tokenizeRegex(value || ''), [value])
  return (
    <pre className="regex-highlight" aria-hidden>
      {tokens.map((tok, idx) => (
        <span key={idx} className={`rx-${tok.t}`}>{tok.v}</span>
      ))}
      {(!value || value.length === 0) && <span className="rx-placeholder"> </span>}
    </pre>
  )
}

function renderMatchedText(text, span) {
  if (!span || span.length !== 2) {
    return <span className="mono">{text || '(empty)'}</span>
  }
  const [a, b] = span
  return (
    <span className="mono">
      {text.slice(0, a)}
      <mark className="rx-match">{text.slice(a, b) || '∅'}</mark>
      {text.slice(b)}
    </span>
  )
}

export default function RegexEditorModal({
  open,
  title,
  initialPattern,
  ignoreCase,
  samplePlaceholder,
  engine = 'python',
  onApply,
  onClose,
}) {
  const [pattern, setPattern] = useState(initialPattern || '')
  const [sample, setSample] = useState('')
  const [result, setResult] = useState(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!open) return
    setPattern(initialPattern || '')
    setSample('')
    setResult(null)
  }, [open, initialPattern])

  useEffect(() => {
    if (!open) return
    const t = setTimeout(async () => {
      setBusy(true)
      try {
        if (engine === 'js') {
          try {
            const re = new RegExp(pattern, ignoreCase ? 'i' : '')
            const m = sample === '' ? null : sample.match(re)
            setResult({
              ok: true,
              matched: !!m,
              group: m ? m[0] : null,
              span: m && m.index != null ? [m.index, m.index + m[0].length] : null,
              error: '',
            })
          } catch (e) {
            setResult({ ok: false, matched: false, error: e.message })
          }
        } else {
          const res = await api('/api/mitm/test-regex', {
            method: 'POST',
            body: { pattern, text: sample, ignoreCase: !!ignoreCase },
          })
          setResult(res)
        }
      } catch (e) {
        setResult({ ok: false, matched: false, error: e.message })
      } finally {
        setBusy(false)
      }
    }, 200)
    return () => clearTimeout(t)
  }, [open, pattern, sample, ignoreCase, engine])

  if (!open) return null

  let status = 'idle'
  let statusText = busy ? 'Testing…' : 'Enter a sample to test'
  if (result) {
    if (!result.ok || result.error) {
      status = 'err'
      statusText = result.error || 'Invalid pattern'
    } else if (result.matched) {
      status = 'ok'
      statusText = `Match${result.group != null ? `: ${JSON.stringify(result.group)}` : ''}`
    } else if (sample === '') {
      status = 'idle'
      statusText = pattern ? 'Pattern OK — enter a sample to test' : 'Enter a sample to test'
    } else {
      status = 'miss'
      statusText = 'No match'
    }
  }

  const engineHint = engine === 'js'
    ? `JavaScript RegExp${ignoreCase ? ' (ignore case)' : ''}.`
    : `Python re (same as MITM). ${ignoreCase ? 'IGNORECASE on (host).' : 'Case-sensitive (path).'}`

  return (
    <div className="modal-backdrop" onClick={onClose} role="presentation">
      <div className="modal" onClick={(e) => e.stopPropagation()} role="dialog" aria-modal="true">
        <h2 style={{ marginTop: 0, fontSize: '1.1rem' }}>{title}</h2>
        <p className="muted" style={{ marginTop: 0 }}>
          {engineHint}
        </p>

        <label className="muted">Pattern</label>
        <div className="regex-editor">
          <HighlightedPattern value={pattern} />
          <textarea
            className="regex-input mono"
            rows={3}
            spellCheck={false}
            value={pattern}
            onChange={(e) => setPattern(e.target.value)}
            autoFocus
          />
        </div>

        <label className="muted" style={{ display: 'block', marginTop: 12 }}>Test string</label>
        <textarea
          className="mono"
          rows={3}
          spellCheck={false}
          placeholder={samplePlaceholder}
          value={sample}
          onChange={(e) => setSample(e.target.value)}
        />

        <div className={`regex-status regex-status-${status}`} style={{ marginTop: 10 }}>
          {statusText}
        </div>
        {result?.matched && (
          <div className="panel-box" style={{ marginTop: 8, marginBottom: 0 }}>
            <div className="muted" style={{ marginBottom: 4 }}>Highlight</div>
            {renderMatchedText(sample, result.span)}
          </div>
        )}

        <div className="row" style={{ marginTop: 16, justifyContent: 'flex-end' }}>
          <button type="button" onClick={onClose}>Cancel</button>
          <button
            type="button"
            className="primary"
            disabled={!!(result && result.error)}
            onClick={() => onApply(pattern)}
          >
            Apply
          </button>
        </div>
      </div>
    </div>
  )
}
