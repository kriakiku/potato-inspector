import { Highlight, themes } from 'prism-react-renderer'

const theme = {
  ...themes.nightOwl,
  plain: {
    ...themes.nightOwl.plain,
    backgroundColor: 'transparent',
  },
}

/** Map HTTP Content-Type to a Prism language (or 'form' for urlencoded). */
export function languageFromContentType(ct) {
  const s = String(ct || '').toLowerCase().split(';')[0].trim()
  if (!s) return 'plain'
  if (s.includes('json') || s.endsWith('+json')) return 'json'
  if (s === 'application/x-www-form-urlencoded') return 'form'
  if (s.includes('html')) return 'markup'
  if (s.includes('xml') || s.endsWith('+xml') || s.includes('svg')) return 'markup'
  if (s.includes('javascript') || s.includes('ecmascript') || s === 'text/js') return 'javascript'
  if (s.includes('css')) return 'css'
  return 'plain'
}

export function looksLikeJson(value) {
  const t = String(value || '').trim()
  return (t.startsWith('{') && t.endsWith('}')) || (t.startsWith('[') && t.endsWith(']'))
}

function prettyJson(code) {
  try {
    return JSON.stringify(JSON.parse(code), null, 2)
  } catch {
    return code
  }
}

/** Split a=1&b=2 into one field per line (keep encoding). */
function prettyForm(code) {
  const s = String(code || '')
  if (!s.includes('&') && !s.includes('=')) return s
  return s.split('&').join('\n')
}

function isOmittedStub(raw) {
  return (
    raw.startsWith('[binary omitted')
    || raw.startsWith('[multipart omitted')
    || raw.startsWith('[compressed body')
  )
}

export default function CodeBlock({ code, language = 'plain', className = '', compact = false }) {
  const raw = code == null || code === '' ? '' : String(code)
  if (!raw) {
    return <pre className={`code-block net-preview mono ${className}`.trim()}>(empty)</pre>
  }
  if (isOmittedStub(raw)) {
    return <pre className={`code-block net-preview mono muted ${className}`.trim()}>{raw}</pre>
  }

  let lang = language || 'plain'
  let text = raw
  if (lang === 'form') {
    text = prettyForm(raw)
    lang = 'plain'
  }
  if (lang === 'plain' && looksLikeJson(text)) {
    lang = 'json'
  }
  if (lang === 'json') {
    text = prettyJson(text)
  }

  if (lang === 'plain') {
    return (
      <pre className={`code-block net-preview mono ${compact ? 'compact' : ''} ${className}`.trim()}>
        {text}
      </pre>
    )
  }

  return (
    <Highlight theme={theme} code={text} language={lang}>
      {({ className: cls, style, tokens, getLineProps, getTokenProps }) => (
        <pre
          className={`code-block net-preview mono ${compact ? 'compact' : ''} ${cls} ${className}`.trim()}
          style={{ ...style, backgroundColor: 'transparent', margin: 0 }}
        >
          {tokens.map((line, i) => (
            <div key={i} {...getLineProps({ line })}>
              {line.map((token, key) => (
                <span key={key} {...getTokenProps({ token })} />
              ))}
            </div>
          ))}
        </pre>
      )}
    </Highlight>
  )
}
