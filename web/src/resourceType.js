/** DevTools-like Inspector resource / protocol filters. */

export const INSPECTOR_CHIPS = [
  { id: 'all', label: 'All' },
  { id: 'http', label: 'HTTP' },
  { id: 'tls', label: 'TLS' },
  { id: 'dns', label: 'DNS' },
  { id: 'doc', label: 'Doc' },
  { id: 'css', label: 'CSS' },
  { id: 'js', label: 'JS' },
  { id: 'json', label: 'JSON' },
  { id: 'font', label: 'Font' },
  { id: 'img', label: 'Img' },
  { id: 'media', label: 'Media' },
  { id: 'ws', label: 'WS' },
  { id: 'other', label: 'Other' },
]

const PROTOCOL_CHIPS = new Set(['http', 'tls', 'dns'])
const RESOURCE_CHIPS = new Set(['doc', 'css', 'js', 'json', 'font', 'img', 'media', 'ws', 'other'])

export function isProtocolChip(id) {
  return PROTOCOL_CHIPS.has(id)
}

export function isResourceChip(id) {
  return RESOURCE_CHIPS.has(id)
}

/** Whether load/SSE should use API ?type= (protocol-only chips). */
export function apiTypeForChip(chip) {
  if (isProtocolChip(chip)) return chip
  return ''
}

function headerValue(headers, name) {
  if (!headers || typeof headers !== 'object') return ''
  const want = name.toLowerCase()
  for (const [k, v] of Object.entries(headers)) {
    if (String(k).toLowerCase() === want) return String(v || '')
  }
  return ''
}

function contentTypeOf(ev) {
  const d = ev.detail || {}
  let ct = String(d.contentType || '').trim()
  if (!ct) {
    ct = headerValue(d.responseHeaders, 'Content-Type')
      || headerValue(d.responseHeaders, 'content-type')
  }
  return ct.toLowerCase().split(';')[0].trim()
}

function pathExt(ev) {
  const d = ev.detail || {}
  const raw = String(d.path || d.url || '')
  try {
    const path = raw.includes('://') ? new URL(raw).pathname : raw.split('?')[0]
    const m = path.match(/\.([a-z0-9]+)$/i)
    return m ? m[1].toLowerCase() : ''
  } catch {
    return ''
  }
}

function isWebsocket(ev) {
  const d = ev.detail || {}
  const ct = contentTypeOf(ev)
  if (ct.includes('websocket')) return true
  const upReq = headerValue(d.requestHeaders, 'Upgrade').toLowerCase()
  const upRes = headerValue(d.responseHeaders, 'Upgrade').toLowerCase()
  if (upReq.includes('websocket') || upRes.includes('websocket')) return true
  const conn = `${headerValue(d.requestHeaders, 'Connection')} ${headerValue(d.responseHeaders, 'Connection')}`.toLowerCase()
  if (conn.includes('upgrade') && (upReq || upRes)) return true
  return false
}

/** Classify an HTTP event into a resource chip id. */
export function httpResourceKind(ev) {
  if (ev?.type !== 'http') return ''
  if (isWebsocket(ev)) return 'ws'

  const ct = contentTypeOf(ev)
  if (ct) {
    if (ct.includes('text/html') || ct.includes('application/xhtml')) return 'doc'
    if (ct.includes('text/css') || ct === 'css') return 'css'
    if (
      ct.includes('application/json')
      || ct.includes('text/json')
      || ct.includes('ld+json')
      || ct.endsWith('+json')
      || (ct.includes('json') && !ct.includes('javascript'))
    ) {
      return 'json'
    }
    if (
      ct.includes('javascript')
      || ct.includes('ecmascript')
      || ct === 'text/js'
      || ct.includes('text/javascript')
    ) {
      return 'js'
    }
    if (ct.startsWith('font/') || ct.includes('woff') || ct.includes('font-')) return 'font'
    if (ct.startsWith('image/')) return 'img'
    if (ct.startsWith('audio/') || ct.startsWith('video/')) return 'media'
  }

  const ext = pathExt(ev)
  if (ext) {
    if (['html', 'htm', 'xhtml'].includes(ext)) return 'doc'
    if (ext === 'css') return 'css'
    if (['js', 'mjs', 'cjs'].includes(ext)) return 'js'
    if (ext === 'json') return 'json'
    if (['woff', 'woff2', 'ttf', 'otf', 'eot'].includes(ext)) return 'font'
    if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'ico', 'avif', 'bmp'].includes(ext)) return 'img'
    if (['mp4', 'webm', 'mp3', 'wav', 'ogg', 'm4a', 'mov'].includes(ext)) return 'media'
  }

  return 'other'
}

/** Whether event passes the active Inspector chip. */
export function eventMatchesChip(ev, chip) {
  if (!chip || chip === 'all') return true
  if (isProtocolChip(chip)) return ev.type === chip
  if (isResourceChip(chip)) {
    if (ev.type !== 'http') return false
    return httpResourceKind(ev) === chip
  }
  return true
}
