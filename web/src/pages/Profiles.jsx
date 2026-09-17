import { useEffect, useState } from 'react'
import { api } from '../api'
import { notifyError, notifySuccess } from '../toast'

const emptyForm = () => ({
  id: '',
  name: '',
  description: '',
  delayMs: 50,
  downloadMbps: 20,
  uploadMbps: 8,
  lossPercent: 0.5,
  passthrough: false,
})

function slugify(s) {
  return s
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
    .slice(0, 48)
}

export default function Profiles() {
  const [list, setList] = useState([])
  const [status, setStatus] = useState('')
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState(emptyForm())
  const [idTouched, setIdTouched] = useState(false)

  async function load() {
    setList(await api('/api/profiles'))
  }

  useEffect(() => {
    load().catch((e) => notifyError(e.message))
  }, [])

  function setField(key, value) {
    setForm((f) => {
      const next = { ...f, [key]: value }
      if (key === 'name' && !editing && !idTouched) {
        next.id = slugify(value) || f.id
      }
      return next
    })
  }

  function openCreate() {
    setEditing(false)
    setIdTouched(false)
    setForm(emptyForm())
    setFormOpen(true)
  }

  function openEdit(p) {
    if (p.builtin) return
    setEditing(true)
    setIdTouched(true)
    setForm({
      id: p.id,
      name: p.name,
      description: p.description || '',
      delayMs: p.delayMs ?? 0,
      downloadMbps: p.downloadMbps ?? 0,
      uploadMbps: p.uploadMbps ?? 0,
      lossPercent: p.lossPercent ?? 0,
      passthrough: !!p.passthrough,
    })
    setFormOpen(true)
  }

  function openDuplicate(p) {
    setEditing(false)
    setIdTouched(false)
    setForm({
      id: slugify(`${p.id}-custom`) || `custom-${Date.now()}`,
      name: `${p.name} (custom)`,
      description: p.description || '',
      delayMs: p.delayMs ?? 0,
      downloadMbps: p.downloadMbps ?? 0,
      uploadMbps: p.uploadMbps ?? 0,
      lossPercent: p.lossPercent ?? 0,
      passthrough: !!p.passthrough,
    })
    setFormOpen(true)
  }

  async function apply(id) {
    try {
      const res = await api('/api/profiles/apply', { method: 'POST', body: { id } })
      setStatus(res.status)
      notifySuccess('Profile applied')
    } catch (e) {
      notifyError(e.message)
    }
  }

  async function saveForm(e) {
    e?.preventDefault()
    if (!form.id.trim() || !form.name.trim()) {
      notifyError('ID and name are required')
      return
    }
    try {
      await api('/api/profiles', {
        method: 'POST',
        body: {
          ...form,
          id: form.id.trim(),
          name: form.name.trim(),
          delayMs: +form.delayMs || 0,
          downloadMbps: +form.downloadMbps || 0,
          uploadMbps: +form.uploadMbps || 0,
          lossPercent: +form.lossPercent || 0,
        },
      })
      notifySuccess(editing ? 'Profile updated' : 'Profile created')
      setFormOpen(false)
      await load()
    } catch (ex) {
      notifyError(ex.message)
    }
  }

  async function remove(p) {
    if (p.builtin) return
    if (!confirm(`Delete custom profile “${p.name}”?`)) return
    try {
      await api(`/api/profiles?id=${encodeURIComponent(p.id)}`, { method: 'DELETE' })
      if (formOpen && form.id === p.id) setFormOpen(false)
      notifySuccess('Deleted')
      await load()
    } catch (e) {
      notifyError(e.message)
    }
  }

  return (
    <>
      <h1>Profiles</h1>
      <p className="lead">
        Delay is <strong>one-way</strong> ms (RTT ≈ 2×). Bandwidth is download (internet→client) / upload (client→internet).
        Built-in profiles are read-only — duplicate them to customize.
      </p>
      {status && <p className="muted mono">Applied: {status}</p>}

      <div className="row" style={{ marginBottom: 12 }}>
        <button className="primary" type="button" onClick={openCreate}>New custom profile</button>
      </div>

      <div className="profile-list">
        {list.map((p) => (
          <div key={p.id} className="profile-card">
            <div className="profile-card-main">
              <div className="profile-card-title">
                <strong>{p.name}</strong>
                {p.builtin ? <span className="badge">builtin</span> : <span className="badge">custom</span>}
              </div>
              {p.description && <p className="muted profile-card-desc">{p.description}</p>}
              <div className="mono muted profile-card-id">{p.id}</div>
              <div className="profile-metrics mono">
                {p.passthrough ? (
                  <span>passthrough (no qdisc)</span>
                ) : (
                  <>
                    <span>{p.delayMs} ms one-way (~{p.delayMs * 2} ms RTT)</span>
                    <span>{p.downloadMbps}↓ / {p.uploadMbps}↑ Mbit/s</span>
                    <span>{p.lossPercent}% loss</span>
                  </>
                )}
              </div>
            </div>
            <div className="profile-card-actions">
              <button className="primary" type="button" onClick={() => apply(p.id)}>Apply</button>
              {p.builtin ? (
                <button type="button" onClick={() => openDuplicate(p)}>Duplicate</button>
              ) : (
                <>
                  <button type="button" onClick={() => openEdit(p)}>Edit</button>
                  <button type="button" className="danger" onClick={() => remove(p)}>Delete</button>
                </>
              )}
            </div>
          </div>
        ))}
      </div>

      {formOpen && (
        <div className="modal-backdrop" onClick={() => setFormOpen(false)} role="presentation">
          <form
            className="modal profile-form-modal"
            onClick={(e) => e.stopPropagation()}
            onSubmit={saveForm}
          >
            <h2 style={{ marginTop: 0, fontSize: '1.1rem' }}>
              {editing ? 'Edit custom profile' : 'New custom profile'}
            </h2>

            <div className="form-stack">
              <label className="form-field">
                <span className="form-label">Name</span>
                <input
                  required
                  value={form.name}
                  onChange={(e) => setField('name', e.target.value)}
                  placeholder="My 4G profile"
                  autoFocus
                />
              </label>

              <label className="form-field">
                <span className="form-label">ID {editing && <span className="muted">(locked)</span>}</span>
                <input
                  className="mono"
                  required
                  disabled={editing}
                  value={form.id}
                  onChange={(e) => {
                    setIdTouched(true)
                    setField('id', e.target.value.replace(/\s+/g, '-'))
                  }}
                  placeholder="my-4g-profile"
                />
              </label>

              <label className="form-field">
                <span className="form-label">Description</span>
                <textarea
                  rows={2}
                  value={form.description}
                  onChange={(e) => setField('description', e.target.value)}
                  placeholder="Optional notes"
                />
              </label>

              <label className="form-check">
                <input
                  type="checkbox"
                  checked={form.passthrough}
                  onChange={(e) => setField('passthrough', e.target.checked)}
                />
                <span>Passthrough (no shaping — ignores delay/rate/loss below)</span>
              </label>

              <fieldset className="form-fieldset" disabled={form.passthrough}>
                <legend>Shaping</legend>
                <div className="form-row-2">
                  <label className="form-field">
                    <span className="form-label">One-way delay (ms)</span>
                    <input
                      type="number"
                      min={0}
                      value={form.delayMs}
                      onChange={(e) => setField('delayMs', e.target.value)}
                    />
                    <span className="form-hint">≈ {(+form.delayMs || 0) * 2} ms RTT / ping</span>
                  </label>
                  <label className="form-field">
                    <span className="form-label">Loss (%)</span>
                    <input
                      type="number"
                      min={0}
                      max={100}
                      step={0.1}
                      value={form.lossPercent}
                      onChange={(e) => setField('lossPercent', e.target.value)}
                    />
                  </label>
                </div>
                <div className="form-row-2">
                  <label className="form-field">
                    <span className="form-label">Download (Mbit/s)</span>
                    <input
                      type="number"
                      min={0}
                      step={0.1}
                      value={form.downloadMbps}
                      onChange={(e) => setField('downloadMbps', e.target.value)}
                    />
                    <span className="form-hint">Internet → client</span>
                  </label>
                  <label className="form-field">
                    <span className="form-label">Upload (Mbit/s)</span>
                    <input
                      type="number"
                      min={0}
                      step={0.1}
                      value={form.uploadMbps}
                      onChange={(e) => setField('uploadMbps', e.target.value)}
                    />
                    <span className="form-hint">Client → internet</span>
                  </label>
                </div>
              </fieldset>
            </div>

            <div className="row" style={{ marginTop: 16, justifyContent: 'flex-end' }}>
              <button type="button" onClick={() => setFormOpen(false)}>Cancel</button>
              <button type="submit" className="primary">{editing ? 'Save changes' : 'Create profile'}</button>
            </div>
          </form>
        </div>
      )}
    </>
  )
}
