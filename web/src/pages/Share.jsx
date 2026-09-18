import { useEffect, useRef, useState } from 'react'
import { api } from '../api'
import { notifyError, notifySuccess } from '../toast'

const DEBOUNCE_MS = 300

export default function Share() {
  const [text, setText] = useState('')
  const [version, setVersion] = useState(0)
  const [ready, setReady] = useState(false)
  const versionRef = useRef(0)
  const debounceRef = useRef(null)
  const applyingRemote = useRef(false)

  useEffect(() => {
    let es
    let cancelled = false

    async function boot() {
      try {
        const snap = await api('/api/share')
        if (cancelled) return
        versionRef.current = snap.version || 0
        setVersion(snap.version || 0)
        setText(snap.text || '')
        setReady(true)
      } catch (e) {
        notifyError(e.message)
      }
    }

    boot()

    es = new EventSource('/api/share/stream')
    es.onmessage = (ev) => {
      try {
        const snap = JSON.parse(ev.data)
        if ((snap.version || 0) <= versionRef.current) return
        versionRef.current = snap.version
        setVersion(snap.version)
        applyingRemote.current = true
        setText(snap.text || '')
        queueMicrotask(() => {
          applyingRemote.current = false
        })
      } catch {
        /* ignore */
      }
    }
    es.onerror = () => {
      /* browser reconnects */
    }

    return () => {
      cancelled = true
      if (debounceRef.current) clearTimeout(debounceRef.current)
      es?.close()
    }
  }, [])

  function pushText(next) {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(async () => {
      try {
        const snap = await api('/api/share', { method: 'PUT', body: { text: next } })
        versionRef.current = snap.version || 0
        setVersion(snap.version || 0)
      } catch (e) {
        notifyError(e.message)
      }
    }, DEBOUNCE_MS)
  }

  function onChange(e) {
    const next = e.target.value
    setText(next)
    if (applyingRemote.current) return
    pushText(next)
  }

  async function clear() {
    setText('')
    pushText('')
    notifySuccess('Cleared')
  }

  async function copy() {
    try {
      await navigator.clipboard.writeText(text)
      notifySuccess('Copied')
    } catch {
      notifyError('Clipboard copy failed')
    }
  }

  async function paste() {
    try {
      const clip = await navigator.clipboard.readText()
      setText(clip)
      pushText(clip)
      notifySuccess('Pasted')
    } catch {
      notifyError('Clipboard paste failed (permission?)')
    }
  }

  return (
    <div className="share-shell">
      <div className="share-toolbar">
        <h1>Share</h1>
        <p className="lead share-lead">Live notepad — edits sync to every open tab (last write wins).</p>
        <div className="row share-actions">
          <button type="button" onClick={clear}>Clear</button>
          <button type="button" onClick={copy}>Copy</button>
          <button type="button" className="primary" onClick={paste}>Paste</button>
          <span className="muted mono share-meta">v{version}</span>
        </div>
      </div>
      <textarea
        className="share-textarea mono"
        value={text}
        onChange={onChange}
        disabled={!ready}
        placeholder={ready ? 'Type here…' : 'Loading…'}
        spellCheck={false}
      />
    </div>
  )
}
