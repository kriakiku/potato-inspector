import { NavLink, Navigate, Route, Routes, useLocation } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import WireGuard from './pages/WireGuard'
import Profiles from './pages/Profiles'
import Baseline from './pages/Baseline'
import Mitm from './pages/Mitm'
import Inspector from './pages/Inspector'

const tabs = [
  { to: '/', end: true, label: 'Dashboard' },
  { to: '/inspector', label: 'Inspector' },
  { to: '/profiles', label: 'Profiles' },
  { to: '/baseline', label: 'Baseline' },
  { to: '/mitm', label: 'MITM' },
  { to: '/wireguard', label: 'WireGuard' },
]

export default function App() {
  const loc = useLocation()
  const fill = loc.pathname === '/inspector'

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
              className={({ isActive }) => (isActive ? 'active' : undefined)}
            >
              {t.label}
            </NavLink>
          ))}
        </div>
      </nav>
      <main className={fill ? 'main main-fill' : 'main'}>
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/inspector" element={<Inspector />} />
          <Route path="/profiles" element={<Profiles />} />
          <Route path="/baseline" element={<Baseline />} />
          <Route path="/mitm" element={<Mitm />} />
          <Route path="/wireguard" element={<WireGuard />} />
          <Route path="/peers" element={<Navigate to="/wireguard" replace />} />
          <Route path="/settings" element={<Navigate to="/wireguard" replace />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  )
}
