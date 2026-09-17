import { useEffect, useState } from 'react'
import { NavLink, Navigate, Route, Routes, useNavigate } from 'react-router-dom'
import Dashboard from './pages/Dashboard'
import Peers from './pages/Peers'
import Profiles from './pages/Profiles'
import Mitm from './pages/Mitm'
import Inspector from './pages/Inspector'
import Settings from './pages/Settings'
import { api } from './api'

function Login({ onOk }) {
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  async function submit(e) {
    e.preventDefault()
    setErr('')
    try {
      await api('/api/login', { method: 'POST', body: { password } })
      onOk()
    } catch (ex) {
      setErr(ex.message || 'Login failed')
    }
  }
  return (
    <div className="login">
      <form className="login-card" onSubmit={submit}>
        <h1>PotatoInspector</h1>
        <p className="muted">Panel password required.</p>
        <label className="muted" style={{ display: 'block', marginBottom: 6 }}>Password</label>
        <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoFocus />
        {err && <div className="err">{err}</div>}
        <div style={{ marginTop: 12 }}>
          <button className="primary" type="submit">Sign in</button>
        </div>
      </form>
    </div>
  )
}

function Shell({ onLogout }) {
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
        <button style={{ marginTop: 'auto' }} onClick={onLogout}>Log out</button>
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

export default function App() {
  const [authed, setAuthed] = useState(null)
  const nav = useNavigate()

  useEffect(() => {
    api('/api/status')
      .then(() => setAuthed(true))
      .catch(() => setAuthed(false))
  }, [])

  async function logout() {
    try { await api('/api/logout', { method: 'POST' }) } catch {}
    setAuthed(false)
    nav('/')
  }

  if (authed === null) return <div className="login"><p className="muted">Loading…</p></div>
  if (!authed) return <Login onOk={() => setAuthed(true)} />
  return <Shell onLogout={logout} />
}
