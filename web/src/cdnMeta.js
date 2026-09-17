/** Case-insensitive header lookup (values may be string or string[]). */
export function headerGet(headers, name) {
  if (!headers) return ''
  const want = name.toLowerCase()
  for (const [k, v] of Object.entries(headers)) {
    if (String(k).toLowerCase() === want) {
      if (Array.isArray(v)) return String(v[0] ?? '')
      return String(v ?? '')
    }
  }
  return ''
}

/** @see https://developers.cloudflare.com/cache/concepts/cache-responses/ */
export const CF_CACHE_META = {
  HIT: {
    tone: 'hit',
    tip: 'HIT — resource found in Cloudflare’s cache.',
  },
  MISS: {
    tone: 'miss',
    tip: 'MISS — eligible for cache but not present; served from origin.',
  },
  EXPIRED: {
    tone: 'stale',
    tip: 'EXPIRED — found in cache but expired; served from origin.',
  },
  STALE: {
    tone: 'stale',
    tip: 'STALE — served expired from cache; origin could not be reached.',
  },
  BYPASS: {
    tone: 'bypass',
    tip: 'BYPASS — eligible at request time, but origin response was not cacheable.',
  },
  REVALIDATED: {
    tone: 'reval',
    tip: 'REVALIDATED — origin confirmed cache via conditional request; served from cache.',
  },
  UPDATING: {
    tone: 'stale',
    tip: 'UPDATING — served from cache while origin revalidates in the background.',
  },
  DYNAMIC: {
    tone: 'dynamic',
    tip: 'DYNAMIC — not eligible for cache at request time; went to origin.',
  },
  NONE: {
    tone: 'unknown',
    tip: 'NONE/UNKNOWN — response generated before cache (Worker, WAF, redirect, …).',
  },
  UNKNOWN: {
    tone: 'unknown',
    tip: 'NONE/UNKNOWN — response generated before cache (Worker, WAF, redirect, …).',
  },
}

export function normalizeCfCacheStatus(raw) {
  if (!raw) return ''
  const s = String(raw).trim().toUpperCase()
  if (s === 'NONE/UNKNOWN' || s === 'NONE' || s === 'UNKNOWN') return 'NONE'
  return s
}

export function analyzeCdn(responseHeaders) {
  const h = responseHeaders || {}
  const cfCacheRaw = headerGet(h, 'cf-cache-status')
  const cfRay = headerGet(h, 'cf-ray')
  const server = headerGet(h, 'server').toLowerCase()
  const via = headerGet(h, 'via')
  const amzPop = headerGet(h, 'x-amz-cf-pop')
  const amzId = headerGet(h, 'x-amz-cf-id')
  const xCache = headerGet(h, 'x-cache')

  const cfCache = normalizeCfCacheStatus(cfCacheRaw)
  const cloudflare = !!(cfCache || cfRay || server.includes('cloudflare'))

  const viaCf = /cloudfront\.net|\(cloudfront\)/i.test(via)
  const cloudfront = !!(amzPop || amzId || viaCf || /cloudfront/i.test(xCache))

  let cloudfrontVia = ''
  if (viaCf) {
    const m = via.match(/([\w.-]+\.cloudfront\.net)/i)
    cloudfrontVia = m ? m[1] : via
  }

  return {
    cloudflare,
    cfCache,
    cfCacheMeta: CF_CACHE_META[cfCache] || (cfCache
      ? { tone: 'unknown', tip: `CF-Cache-Status: ${cfCache}` }
      : null),
    cfRay: cfRay || '',
    cloudfront,
    cloudfrontPop: amzPop || '',
    cloudfrontVia,
    cloudfrontId: amzId || '',
    xCache: xCache || '',
  }
}
