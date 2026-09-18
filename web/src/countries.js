export const TIER_EMOJI = {
  stable: '📶',
  typical: '📱',
  poor: '🥔',
}

/** ISO 3166-1 alpha-2 → regional-indicator flag emoji (no extra dependency). */
export function countryFlagEmoji(code) {
  const cc = String(code || '')
    .trim()
    .toUpperCase()
  if (!/^[A-Z]{2}$/.test(cc)) return ''
  const A = 0x1f1e6
  return String.fromCodePoint(
    A + cc.charCodeAt(0) - 65,
    A + cc.charCodeAt(1) - 65,
  )
}

/** Prefer catalog flag; otherwise derive from country id. */
export function countryFlag(country) {
  if (country?.flag) return country.flag
  return countryFlagEmoji(country?.id)
}

export function tierLabel(tier) {
  const e = TIER_EMOJI[tier] || ''
  return e ? `${e} ${tier}` : tier
}

/** Margin (ms): country CF RTT ≤ host CF + this → delay clamps to ~0, not emulatable. */
export const BASELINE_MARGIN_MS = 5

export const TOO_CLOSE_TOOLTIP =
  'This country’s CF RTT is better than or about the same as your host baseline. Last-mile delay would clamp to 0 — you can’t meaningfully emulate a closer/faster location from here.'

export function countryCfRtt(country, tier = 'typical') {
  const tiers = country?.tiers || {}
  const preferred = tiers[tier] || tiers.typical || tiers.stable || tiers.poor
  const n = preferred?.rttToDest?.cf
  return typeof n === 'number' && n > 0 ? n : 0
}

export function isTooCloseToBaseline(country, hostCfRtt, marginMs = BASELINE_MARGIN_MS) {
  const host = Number(hostCfRtt) || 0
  if (host <= 0) return false
  const cf = countryCfRtt(country)
  if (cf <= 0) return false
  return cf <= host + marginMs
}

/** Favorites (emulatable) → others (emulatable) → too-close at bottom (favs first). */
export function sortCountries(list, favorites, hostCfRtt) {
  const favSet = new Set(favorites || [])
  const favIndex = new Map((favorites || []).map((id, i) => [id, i]))
  return [...(list || [])].sort((a, b) => {
    const aBad = isTooCloseToBaseline(a, hostCfRtt)
    const bBad = isTooCloseToBaseline(b, hostCfRtt)
    if (aBad !== bBad) return aBad ? 1 : -1
    const af = favSet.has(a.id)
    const bf = favSet.has(b.id)
    if (af && bf) return (favIndex.get(a.id) ?? 0) - (favIndex.get(b.id) ?? 0)
    if (af) return -1
    if (bf) return 1
    return (a.name || a.id).localeCompare(b.name || b.id)
  })
}

export function countrySelectLabel(country, { favorite, tooClose } = {}) {
  const parts = []
  if (favorite) parts.push('⭐')
  const flag = countryFlag(country)
  if (flag) parts.push(flag)
  parts.push(country.name || country.id)
  let label = parts.join(' ')
  if (tooClose) label = `${label}  ·  💩`
  return label
}
