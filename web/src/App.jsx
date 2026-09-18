import { NavLink, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import WireGuard from './pages/WireGuard'
import Profiles from './pages/Profiles'
import Baseline from './pages/Baseline'
import Ignore from './pages/Ignore'
import Mitm from './pages/Mitm'
import Inspector from './pages/Inspector'
import Dns from './pages/Dns'
import Docs from './pages/Docs'
import Share from './pages/Share'

const tabs = [
  { to: '/', end: true, label: 'Inspector' },
  { to: '/profiles', label: 'Profiles' },
  { to: '/baseline', label: 'Baseline' },
  { to: '/ignore', label: 'Ignore' },
  { to: '/mitm', label: 'MITM' },
  { to: '/dns', label: 'DNS' },
  { to: '/wireguard', label: 'WireGuard' },
  { to: '/share', label: 'Share' },
  { to: '/docs/overview', label: 'Docs' },
]

export default function App() {
  const loc = useLocation()
  const fill = loc.pathname === '/' || loc.pathname === '/inspector' || loc.pathname === '/share'
  const docsActive = loc.pathname.startsWith('/docs')

  return (
    <div className="shell">
      <nav className="nav">
        <div className="brand"><span className="brand-logo" aria-hidden="true">🥔</span> PotatoInspector</div>
        <div className="nav-tabs">
          {tabs.map((t) => (
            <NavLink
              key={t.to}
              to={t.to}
              end={t.end}
              className={({ isActive }) => {
                if (t.label === 'Docs') return docsActive ? 'active' : undefined
                return isActive ? 'active' : undefined
              }}
            >
              {t.label}
            </NavLink>
          ))}
        </div>
      </nav>
      <main className={fill ? 'main main-fill' : 'main'}>
        <Routes>
          <Route path="/" element={<Inspector />} />
          <Route path="/inspector" element={<Navigate to="/" replace />} />
          <Route path="/profiles" element={<Profiles />} />
          <Route path="/baseline" element={<Baseline />} />
          <Route path="/ignore" element={<Ignore />} />
          <Route path="/mitm" element={<Mitm />} />
          <Route path="/dns" element={<Dns />} />
          <Route path="/wireguard" element={<WireGuard />} />
          <Route path="/share" element={<Share />} />
          <Route path="/docs" element={<Navigate to="/docs/overview" replace />} />
          <Route path="/docs/:slug" element={<Docs />} />
          <Route path="/peers" element={<Navigate to="/wireguard" replace />} />
          <Route path="/settings" element={<Navigate to="/wireguard" replace />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  )
}
