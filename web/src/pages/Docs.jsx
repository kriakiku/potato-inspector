import { NavLink, Navigate, useParams } from 'react-router-dom'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

import overview from '@docs/overview.md?raw'
import wireguard from '@docs/wireguard.md?raw'
import profiles from '@docs/profiles.md?raw'
import caAndroid from '@docs/ca-android.md?raw'
import caIos from '@docs/ca-ios.md?raw'
import caMacos from '@docs/ca-macos.md?raw'
import caWindows from '@docs/ca-windows.md?raw'

const PAGES = [
  { slug: 'overview', title: 'Overview', source: overview },
  { slug: 'wireguard', title: 'WireGuard', source: wireguard },
  { slug: 'profiles', title: 'Profiles', source: profiles },
  { slug: 'ca-android', title: 'CA · Android', source: caAndroid },
  { slug: 'ca-ios', title: 'CA · iOS', source: caIos },
  { slug: 'ca-macos', title: 'CA · macOS', source: caMacos },
  { slug: 'ca-windows', title: 'CA · Windows', source: caWindows },
]

const bySlug = Object.fromEntries(PAGES.map((p) => [p.slug, p]))

function docHref(href) {
  if (!href) return href
  if (href.startsWith('/docs/') || href === '/docs') return href
  if (href.startsWith('/api/')) return href
  return href
}

function MarkdownLink({ href, children, ...props }) {
  const to = docHref(href)
  const external = to && /^(https?:|mailto:)/i.test(to)
  if (external) {
    return (
      <a href={to} target="_blank" rel="noopener noreferrer" {...props}>
        {children}
      </a>
    )
  }
  if (to?.startsWith('/docs')) {
    return (
      <NavLink to={to === '/docs' ? '/docs/overview' : to} {...props}>
        {children}
      </NavLink>
    )
  }
  return (
    <a href={to} {...props}>
      {children}
    </a>
  )
}

export default function Docs() {
  const { slug } = useParams()
  const page = bySlug[slug || 'overview']
  if (!page) {
    return <Navigate to="/docs/overview" replace />
  }

  return (
    <div className="docs-shell">
      <aside className="docs-nav panel-box">
        <div className="docs-nav-title">Docs</div>
        <nav>
          {PAGES.map((p) => (
            <NavLink
              key={p.slug}
              to={`/docs/${p.slug}`}
              className={({ isActive }) => (isActive ? 'docs-nav-link active' : 'docs-nav-link')}
            >
              {p.title}
            </NavLink>
          ))}
        </nav>
      </aside>
      <article className="docs-article panel-box">
        <ReactMarkdown
          remarkPlugins={[remarkGfm]}
          components={{
            a: MarkdownLink,
          }}
        >
          {page.source}
        </ReactMarkdown>
      </article>
    </div>
  )
}
