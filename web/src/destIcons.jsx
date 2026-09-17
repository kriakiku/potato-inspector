import { FaAws } from 'react-icons/fa6'
import { SiCloudflare } from 'react-icons/si'

/** Cloudflare Edge vs AWS region from catalog destination id. */
export function destKind(id) {
  const s = String(id || '').toLowerCase()
  if (s === 'cf' || s.startsWith('cf-') || s.startsWith('cloudflare')) return 'cf'
  if (s.startsWith('aws')) return 'aws'
  return ''
}

export function DestIcon({ id, className = '' }) {
  const kind = destKind(id)
  if (kind === 'cf') {
    return (
      <SiCloudflare
        className={`dest-logo dest-logo-cf ${className}`.trim()}
        aria-hidden
        title="Cloudflare"
      />
    )
  }
  if (kind === 'aws') {
    return (
      <FaAws
        className={`dest-logo dest-logo-aws ${className}`.trim()}
        aria-hidden
        title="AWS"
      />
    )
  }
  return null
}

/** Icon + label for destination rows. */
export function DestLabel({ id, label, mono = false }) {
  const text = label || id || '—'
  return (
    <span className={`dest-label${mono ? ' mono' : ''}`}>
      <DestIcon id={id} />
      <span>{text}</span>
    </span>
  )
}
