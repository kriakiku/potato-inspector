export async function api(path, opts = {}) {
  const headers = { ...(opts.headers || {}) }
  let body = opts.body
  if (body && typeof body === 'object' && !(body instanceof FormData)) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(body)
  }
  const res = await fetch(path, { ...opts, headers, body, credentials: 'same-origin' })
  const ct = res.headers.get('content-type') || ''
  if (!res.ok) {
    let msg = res.statusText
    if (ct.includes('json')) {
      const j = await res.json().catch(() => ({}))
      msg = j.error || msg
    }
    throw new Error(msg)
  }
  if (ct.includes('json')) return res.json()
  return res
}
