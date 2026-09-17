import { NavLink, Navigate, Route, Routes } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import Peers from './pages/Peers'
import Profiles from './pages/Profiles'
import Mitm from './pages/Mitm'
import Inspector from './pages/Inspector'
import Settings from './pages/Settings'

export default function App() {
  return (
    <div className="shell">
      <nav className="nav">
        <div className="brand">PotatoInspector</div>
        <NavLink to="/" end className={({ isActive }) => (isActive ? 'active' : undefined)}>Dashboard</NavLink>
        <NavLink to="/peers" className={({ isActive }) => (isActive ? 'active' : undefined)}>Peers</NavLink>
        <NavLink to="/profiles" className={({ isActive }) => (isActive ? 'active' : undefined)}>Profiles</NavLink>
        <NavLink to="/mitm" className={({ isActive }) => (isActive ? 'active' : undefined)}>MITM</NavLink>
        <NavLink to="/inspector" className={({ isActive }) => (isActive ? 'active' : undefined)}>Inspector</NavLink>
        <NavLink to="/settings" className={({ isActive }) => (isActive ? 'active' : undefined)}>Settings</NavLink>
      </nav>
      <main className="main">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/peers" element={<Peers />} />
          <Route path="/profiles" element={<Profiles />} />
          <Route path="/mitm" element={<Mitm />} />
          <Route path="/inspector" element={<Inspector />} />
          <Route path="/settings" element={<Settings />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  )
}
