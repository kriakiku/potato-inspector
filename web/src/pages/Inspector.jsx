import { useEffect, useMemo, useRef, useState } from 'react'
import { FaAws } from 'react-icons/fa6'
import { SiCloudflare } from 'react-icons/si'
import { api } from '../api'
import { analyzeCdn } from '../cdnMeta'
import RegexEditorModal from '../components/RegexEditorModal'
import CodeBlock, { languageFromContentType, looksLikeJson } from '../components/CodeBlock'
import {
  countrySelectLabel,
  isTooCloseToBaseline,
  sortCountries,
  tierLabel,
  TOO_CLOSE_TOOLTIP,
} from '../countries'
import { notifyError, notifySuccess } from '../toast'
import {
  apiTypeForChip,
  eventMatchesChip,
  INSPECTOR_CHIPS,
  isProtocolChip,
} from '../resourceType'

function statusClass(code) {
  if (!code) return 'st-zero'
  if (code < 300) return 'st-ok'
  if (code < 400) return 'st-redir'
  if (code < 500) return 'st-client'
  return 'st-server'
}

function eventUrl(ev) {
  const d = ev.detail || {}
  if (ev.type === 'http') {
    return d.url || `${d.host || ''}${d.path || ''}` || ev.summary
  }
  if (ev.type === 'tls') return d.sni || ev.summary
  if (ev.type === 'dns') return d.qname || ev.summary
  return ev.summary
}

/** Host + full URL (incl. query) + summary — what the Inspector search matches against. */
function eventSearchText(ev) {
  const d = ev.detail || {}
  const parts = []
  const add = (s) => {
    if (s != null && String(s).trim() !== '') parts.push(String(s))
  }
  add(eventUrl(ev))
  add(ev.summary)
  add(d.url)
  add(d.host)
  add(d.path)
  add(d.sni)
  add(d.qname)
  return parts.join('\n')
}

function compileSearch(query, asRegex) {
  const q = (query || '').trim()
  if (!q) return { ok: true, test: () => true }
  if (!asRegex) {
    const needle = q.toLowerCase()
    return {
      ok: true,
      test: (ev) => eventSearchText(ev).toLowerCase().includes(needle),
    }
  }
  try {
    const re = new RegExp(q, 'i')
    return {
      ok: true,
      test: (ev) => re.test(eventSearchText(ev)),
    }
  } catch (e) {
    return { ok: false, error: e.message, test: () => false }
  }
}

function formatBytes(n) {
  if (n == null || n === 0) return '—'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

function durationMs(ev) {
  const t = ev.detail?.timing
  if (!t?.timestampStart || !t?.timestampEnd) return null
  return Math.max(0, Math.round((t.timestampEnd - t.timestampStart) * 1000))
}

const PROXY_TIME_TIP =
  'MITM ↔ origin only (after decrypt). Last-mile netem on the tunnel is outside this interval — see status-bar delay.'


function HeaderTable({ headers }) {
  const entries = Object.entries(headers || {})
  if (!entries.length) return <p className="muted">No headers</p>
  return (
    <table className="hdr-table">
      <tbody>
        {entries.map(([k, v]) => (
          <tr key={k}>
            <th>{k}</th>
            <td className="mono">
              {looksLikeJson(v) ? (
                <CodeBlock code={String(v)} language="json" compact className="hdr-json" />
              ) : (
                v
              )}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function CdnBadges({ detail, compact = false }) {
  if (!detail) return null
  const cdn = analyzeCdn(detail.responseHeaders)
  if (!cdn.cloudflare && !cdn.cloudfront) return null

  return (
    <span className={`net-cdn ${compact ? 'compact' : ''}`}>
      {cdn.cloudflare && (
        <span
          className={`cdn-chip cf ${cdn.cfCacheMeta?.tone || 'unknown'}`}
          title={[
            'Proxied by Cloudflare',
            cdn.cfCacheMeta?.tip,
            cdn.cfRay ? `cf-ray: ${cdn.cfRay}` : '',
          ].filter(Boolean).join('\n')}
        >
          <SiCloudflare className="cdn-logo" aria-hidden />
          <span className="cdn-chip-text">{cdn.cfCache || 'CF'}</span>
        </span>
      )}
      {cdn.cloudfront && (
        <span
          className="cdn-chip cloudfront"
          title={[
            'Amazon CloudFront',
            cdn.cloudfrontPop ? `x-amz-cf-pop: ${cdn.cloudfrontPop}` : '',
            cdn.cloudfrontVia ? `via: ${cdn.cloudfrontVia}` : '',
            cdn.xCache ? `x-cache: ${cdn.xCache}` : '',
          ].filter(Boolean).join('\n')}
        >
          <FaAws className="cdn-logo" aria-hidden />
          <span className="cdn-chip-text">{cdn.cloudfrontPop || 'CloudFront'}</span>
        </span>
      )}
    </span>
  )
}

function DetailPane({ selected, tab, setTab, lastMileDelayMs, direct }) {
  if (!selected) {
    return <div className="net-empty muted">Select a request to inspect headers and body</div>
  }
  const d = selected.detail || {}
  const cdn = selected.type === 'http' ? analyzeCdn(d.responseHeaders) : null
  const tabs = selected.type === 'http'
    ? ['Headers', 'Preview', 'Timing']
    : ['Detail']
  const proxyMs = durationMs(selected)

  return (
    <div className="net-detail">
      <div className="net-detail-title mono" title={eventUrl(selected)}>
        {selected.type === 'http' && (
          <>
            <span className="net-method">{d.method}</span>{' '}
            <span className={statusClass(d.status)}>{d.status || '—'}</span>{' '}
          </>
        )}
        <span className="net-detail-url">{eventUrl(selected)}</span>
      </div>
      <div className="net-tabs">
        {tabs.map((t) => (
          <button
            key={t}
            type="button"
            className={tab === t ? 'active' : ''}
            onClick={() => setTab(t)}
          >
            {t}
          </button>
        ))}
      </div>
      <div className="net-detail-body net-scroll">
        {selected.type === 'http' && tab === 'Headers' && (
          <>
            <h3>General</h3>
            <table className="hdr-table">
              <tbody>
                <tr><th>Request URL</th><td className="mono">{d.url || eventUrl(selected)}</td></tr>
                <tr><th>Method</th><td className="mono">{d.method}</td></tr>
                <tr><th>Status</th><td className={`mono ${statusClass(d.status)}`}>{d.status}</td></tr>
                <tr><th>Content-Type</th><td className="mono">{d.contentType || '—'}</td></tr>
                <tr><th>Body size</th><td className="mono">{formatBytes(d.bodyBytes)}</td></tr>
                {d.extraDelayMs ? <tr><th>Extra delay</th><td className="mono">{d.extraDelayMs} ms</td></tr> : null}
                {cdn?.cloudflare ? (
                  <tr>
                    <th>Cloudflare</th>
                    <td>
                      <CdnBadges detail={d} />
                      {cdn.cfCacheMeta?.tip ? (
                        <div className="muted" style={{ marginTop: 6, fontSize: '0.8rem' }}>{cdn.cfCacheMeta.tip}</div>
                      ) : null}
                    </td>
                  </tr>
                ) : null}
                {cdn?.cloudfront ? (
                  <tr>
                    <th>CloudFront</th>
                    <td className="mono">
                      {[
                        cdn.cloudfrontPop && `pop ${cdn.cloudfrontPop}`,
                        cdn.cloudfrontVia,
                        cdn.xCache && `x-cache ${cdn.xCache}`,
                      ].filter(Boolean).join(' · ') || 'detected'}
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
            <h3>Request Headers</h3>
            <HeaderTable headers={d.requestHeaders} />
            <h3>Response Headers</h3>
            <HeaderTable headers={d.responseHeaders} />
          </>
        )}
        {selected.type === 'http' && tab === 'Preview' && (
          <>
            <h3>Request</h3>
            <CodeBlock
              code={d.requestBodyPreview || '(empty body)'}
              language={
                d.requestBodyPreview
                  ? languageFromContentType(d.requestContentType)
                  : 'plain'
              }
            />
            <h3>Response</h3>
            <CodeBlock
              code={d.bodyPreview || '(empty body)'}
              language={d.bodyPreview ? languageFromContentType(d.contentType) : 'plain'}
            />
          </>
        )}
        {selected.type === 'http' && tab === 'Timing' && (
          <table className="hdr-table">
            <tbody>
              <tr>
                <th title={PROXY_TIME_TIP}>Proxy duration</th>
                <td className="mono" title={PROXY_TIME_TIP}>
                  {proxyMs != null ? `${proxyMs} ms` : '—'}
                </td>
              </tr>
              <tr>
                <th title="One-way tc netem on the WireGuard TUN (status bar). ≈2× on the phone RTT. Not included in Proxy duration.">
                  Last-mile one-way
                </th>
                <td className="mono">
                  {lastMileDelayMs > 0
                    ? `${lastMileDelayMs} ms`
                    : direct
                      ? '0 ms (passthrough / Direct)'
                      : '0 ms (clamped vs host CF baseline)'}
                </td>
              </tr>
              <tr>
                <th title="MITM path-rule sleep after decrypt (included in Proxy duration).">
                  Extra MITM delay
                </th>
                <td className="mono">{d.extraDelayMs ? `${d.extraDelayMs} ms` : '—'}</td>
              </tr>
              <tr><th>Start</th><td className="mono">{d.timing?.timestampStart ?? '—'}</td></tr>
              <tr><th>End</th><td className="mono">{d.timing?.timestampEnd ?? '—'}</td></tr>
            </tbody>
          </table>
        )}
        {selected.type !== 'http' && (
          <pre className="net-preview mono">{JSON.stringify(selected, null, 2)}</pre>
        )}
      </div>
    </div>
  )
}

export default function Inspector() {
  const [events, setEvents] = useState([])
  const [typeChip, setTypeChip] = useState('all')
  const [search, setSearch] = useState('')
  const [searchRegex, setSearchRegex] = useState(false)
  const [regexOpen, setRegexOpen] = useState(false)
  const [selected, setSelected] = useState(null)
  const [tab, setTab] = useState('Headers')
  const [paused, setPaused] = useState(false)
  const [forceDisableCache, setForceDisableCache] = useState(false)
  const [dnsShortTtl, setDnsShortTtl] = useState(false)
  const [catalog, setCatalog] = useState(null)
  const [country, setCountry] = useState('BD')
  const [tier, setTier] = useState('typical')
  const [direct, setDirect] = useState(false)
  const [savedCountry, setSavedCountry] = useState('BD')
  const [savedTier, setSavedTier] = useState('typical')
  const [mitmOn, setMitmOn] = useState(false)
  const [rttMs, setRttMs] = useState(0)
  const [hostCfRtt, setHostCfRtt] = useState(0)
  const [appliedDelay, setAppliedDelay] = useState(0)
  const [favorites, setFavorites] = useState([])
  const esRef = useRef(null)
  const listRef = useRef(null)
  const typeChipRef = useRef(typeChip)
  typeChipRef.current = typeChip

  async function refreshMeta() {
    const [st, cat] = await Promise.all([
      api('/api/status'),
      api('/api/catalog'),
    ])
    setForceDisableCache(!!st.forceDisableCache)
    setDnsShortTtl(!!st.dnsShortTtl)
    setMitmOn(!!st.mitmEnabled)
    setPaused(!!st.capturePaused)
    setCatalog(cat)
    setFavorites(st.favoriteCountries || [])
    const isDirect = st.activeCountry === 'direct' || st.activeProfileId === 'passthrough'
    setDirect(isDirect)
    if (isDirect) {
      // keep country/tier selects on last real profile for when Direct is unchecked
    } else if (st.activeCountry) {
      setCountry(st.activeCountry)
      setSavedCountry(st.activeCountry)
      if (st.activeTier) {
        setTier(st.activeTier)
        setSavedTier(st.activeTier)
      }
    } else if (st.activeTier) {
      setTier(st.activeTier)
      setSavedTier(st.activeTier)
    }
    setAppliedDelay(st.appliedDelayMs || st.profile?.delayMs || 0)
    setRttMs((st.appliedDelayMs || st.profile?.delayMs || 0) * 2)
    setHostCfRtt(st.hostRtt?.cf || 0)
  }

  async function load() {
    const apiType = apiTypeForChip(typeChip)
    const q = apiType ? `?type=${apiType}` : ''
    setEvents(await api(`/api/inspector${q}`))
  }

  useEffect(() => {
    refreshMeta().catch((e) => notifyError(e.message))
    load().catch((e) => notifyError(e.message))
    const t = setInterval(() => {
      refreshMeta().catch(() => {})
    }, 5000)
    return () => clearInterval(t)
  }, [typeChip])

  useEffect(() => {
    if (paused) {
      if (esRef.current) { esRef.current.close(); esRef.current = null }
      return
    }
    const es = new EventSource('/api/inspector/stream')
    esRef.current = es
    es.onmessage = (msg) => {
      try {
        const ev = JSON.parse(msg.data)
        const chip = typeChipRef.current
        // Protocol chips: gate at SSE to match API ?type= buffer.
        // Resource / All: keep all events; rows filter client-side.
        if (isProtocolChip(chip) && ev.type !== chip) return
        setEvents((prev) => [ev, ...prev].slice(0, 500))
      } catch {}
    }
    return () => es.close()
  }, [paused, typeChip])

  useEffect(() => {
    if (!selected) return
    setTab(selected.type === 'http' ? 'Headers' : 'Detail')
  }, [selected?.id])

  async function togglePause() {
    const next = !paused
    try {
      await api('/api/inspector/pause', { method: 'POST', body: { paused: next } })
      setPaused(next)
      notifySuccess(next ? 'Capture paused' : 'Capture resumed')
    } catch (e) {
      notifyError(e.message)
    }
  }

  async function toggleForceDisableCache() {
    const next = !forceDisableCache
    try {
      await api('/api/mitm', { method: 'PUT', body: { forceDisableCache: next } })
      setForceDisableCache(next)
      notifySuccess(next ? 'Force disable cache on' : 'Force disable cache off')
      await refreshMeta()
    } catch (e) {
      notifyError(e.message)
    }
  }

  async function toggleDnsShortTtl() {
    const next = !dnsShortTtl
    try {
      await api('/api/settings', { method: 'PUT', body: { dnsShortTtl: next } })
      setDnsShortTtl(next)
      notifySuccess(next ? 'Short DNS TTL on' : 'Short DNS TTL off')
      await refreshMeta()
    } catch (e) {
      notifyError(e.message)
    }
  }

  async function applyCountryTier(nextCountry, nextTier) {
    try {
      const res = await api('/api/catalog/apply', {
        method: 'POST',
        body: { country: nextCountry, tier: nextTier, probe: true },
      })
      setDirect(false)
      setCountry(nextCountry)
      setTier(nextTier)
      setSavedCountry(nextCountry)
      setSavedTier(nextTier)
      setAppliedDelay(res.profile?.delayMs || 0)
      setRttMs((res.profile?.delayMs || 0) * 2)
      setHostCfRtt(res.hostRtt?.cf || 0)
      notifySuccess(`${nextCountry} · ${nextTier}`)
      await refreshMeta()
    } catch (e) {
      notifyError(e.message)
    }
  }

  async function toggleDirect(on) {
    try {
      if (on) {
        setSavedCountry(country !== 'direct' ? country : savedCountry)
        setSavedTier(tier || savedTier)
        const res = await api('/api/catalog/apply', { method: 'POST', body: { direct: true } })
        setDirect(true)
        setAppliedDelay(res.profile?.delayMs || 0)
        setRttMs(0)
        notifySuccess('Direct — no delays')
      } else {
        const c = savedCountry && savedCountry !== 'direct' ? savedCountry : 'BD'
        const t = savedTier || 'typical'
        await applyCountryTier(c, t)
        return
      }
      await refreshMeta()
    } catch (e) {
      notifyError(e.message)
    }
  }

  async function clear() {
    try {
      await api('/api/inspector/clear', { method: 'POST' })
      setEvents([])
      setSelected(null)
      notifySuccess('Inspector cleared')
    } catch (e) {
      notifyError(e.message)
    }
  }

  const searchCompiled = useMemo(() => compileSearch(search, searchRegex), [search, searchRegex])
  const rows = useMemo(
    () => events.filter((ev) => eventMatchesChip(ev, typeChip) && searchCompiled.test(ev)),
    [events, typeChip, searchCompiled],
  )
  const httpCount = useMemo(() => rows.filter((e) => e.type === 'http').length, [rows])
  const countries = useMemo(
    () => sortCountries(catalog?.countries, favorites, hostCfRtt),
    [catalog, favorites, hostCfRtt],
  )
  const typeChipLabel = INSPECTOR_CHIPS.find((c) => c.id === typeChip)?.label || typeChip

  useEffect(() => {
    if (selected && !rows.some((e) => e.id === selected.id)) {
      setSelected(null)
    }
  }, [rows, selected])

  return (
    <div className="insp-root">
      <div className="insp-toolbar">
        <button onClick={togglePause}>{paused ? 'Resume' : 'Pause'}</button>
        <button onClick={clear}>Clear</button>
        <a href="/api/inspector/har" download="potatoinspector.har">
          <button type="button">Export HAR</button>
        </a>
        <div className={`insp-search ${searchCompiled.ok ? '' : 'is-invalid'} ${searchRegex ? 'is-regex' : ''}`}>
          <input
            type="search"
            className="mono"
            placeholder={searchRegex
              ? 'Regex · host / full URL…'
              : 'Filter · host / https://host/path?q=…'}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            aria-label="Filter requests"
            spellCheck={false}
          />
          <button
            type="button"
            className={`insp-search-mode ${searchRegex ? 'on' : ''}`}
            title={searchRegex ? 'Regex mode on — click for plain text' : 'Plain text — click for regex'}
            aria-pressed={searchRegex}
            onClick={() => setSearchRegex((v) => !v)}
          >
            .*
          </button>
          <button
            type="button"
            className="insp-search-edit"
            title="Edit regex"
            onClick={() => {
              setSearchRegex(true)
              setRegexOpen(true)
            }}
          >
            Edit
          </button>
          {search ? (
            <button
              type="button"
              className="insp-search-clear"
              title="Clear filter"
              onClick={() => setSearch('')}
            >
              ×
            </button>
          ) : null}
        </div>
        <div className="insp-chips" role="toolbar" aria-label="Event type filter">
          {INSPECTOR_CHIPS.map((c) => (
            <button
              key={c.id}
              type="button"
              className={`insp-chip ${typeChip === c.id ? 'on' : ''}`}
              aria-pressed={typeChip === c.id}
              onClick={() => setTypeChip(c.id)}
            >
              {c.label}
            </button>
          ))}
        </div>
        {!searchCompiled.ok && (
          <span className="insp-search-err" title={searchCompiled.error}>
            Bad regex
          </span>
        )}
      </div>

      <div className="net-shell">
        <div className="net-list panel-box">
          <div className="net-list-head">
            <span className="col-status">Status</span>
            <span className="col-method">Method</span>
            <span className="col-name">Request</span>
            <span className="col-size">Size</span>
            <span className="col-time" title={PROXY_TIME_TIP}>Proxy</span>
          </div>
          <div className="net-list-scroll net-scroll" ref={listRef}>
            {rows.length === 0 && (
              <div className="net-empty muted">
                {events.length === 0
                  ? 'No events yet — traffic appears here when MITM/DNS intercept is active'
                  : 'No events match this filter'}
              </div>
            )}
            {rows.map((ev) => {
              const d = ev.detail || {}
              const dur = durationMs(ev)
              const active = selected?.id === ev.id
              const url = eventUrl(ev)
              return (
                <button
                  type="button"
                  key={ev.id}
                  className={`net-row ${active ? 'active' : ''}`}
                  onClick={() => setSelected(ev)}
                  title={url}
                >
                  <span className={`col-status mono ${ev.type === 'http' ? statusClass(d.status) : ''}`}>
                    {ev.type === 'http' ? (d.status || '—') : '—'}
                  </span>
                  <span className="col-method mono">{ev.type === 'http' ? d.method : ev.type.toUpperCase()}</span>
                  <span className="col-name">
                    <span className="net-url-full mono">{url}</span>
                    <span className="net-row-meta">
                      <span className={`badge ${ev.type}`}>{ev.type}</span>
                      {ev.type === 'http' && <CdnBadges detail={d} compact />}
                    </span>
                  </span>
                  <span className="col-size mono">{ev.type === 'http' ? formatBytes(d.bodyBytes) : '—'}</span>
                  <span
                    className="col-time mono"
                    title={dur != null ? PROXY_TIME_TIP : undefined}
                  >
                    {dur != null ? `${dur} ms` : new Date(ev.ts).toLocaleTimeString()}
                  </span>
                </button>
              )
            })}
          </div>
        </div>
        <div className="net-side panel-box">
          <DetailPane
            selected={selected}
            tab={tab}
            setTab={setTab}
            lastMileDelayMs={direct ? 0 : appliedDelay}
            direct={direct}
          />
        </div>
      </div>

      <div className="insp-statusbar">
        <label
          className="form-check"
          title="Strip conditional request headers so origins return full bodies (not 304)"
        >
          <input type="checkbox" checked={forceDisableCache} onChange={toggleForceDisableCache} />
          <span>No cache</span>
        </label>
        <label
          className="form-check"
          title="Clamp TTL on forwarded DNS answers to the Short DNS TTL value from the DNS page (rewrite answers always use that value)"
        >
          <input type="checkbox" checked={dnsShortTtl} onChange={toggleDnsShortTtl} />
          <span>Short DNS TTL</span>
        </label>
        <span className="insp-status-sep" />
        <span>
          {search.trim()
            ? `${rows.length} shown · ${events.length} total`
            : `${rows.length} events`}
          {typeChip !== 'all' ? ` · ${typeChipLabel}` : ''}
          {httpCount !== rows.length ? ` · ${httpCount} http` : ''}
          {searchRegex && search.trim() ? ' · regex' : ''}
        </span>
        <span className="insp-status-sep" />
        <label
          className="form-check"
          title="No last-mile shaping and no MITM path-dest delay"
        >
          <input
            type="checkbox"
            checked={direct}
            onChange={(e) => toggleDirect(e.target.checked)}
          />
          <span>Direct</span>
        </label>
        <label className="row" style={{ gap: '0.35rem', margin: 0 }}>
          <span>Country</span>
          <select
            value={country === 'direct' ? savedCountry : country}
            disabled={direct}
            onChange={(e) => applyCountryTier(e.target.value, tier)}
            title="Last-mile country. 💩 = CF RTT ≤ host baseline (not emulatable)."
          >
            {countries.map((c) => {
              const tooClose = isTooCloseToBaseline(c, hostCfRtt)
              return (
                <option
                  key={c.id}
                  value={c.id}
                  title={tooClose ? TOO_CLOSE_TOOLTIP : undefined}
                >
                  {countrySelectLabel(c, {
                    favorite: favorites.includes(c.id),
                    tooClose,
                  })}
                </option>
              )
            })}
          </select>
        </label>
        <label className="row" style={{ gap: '0.35rem', margin: 0 }}>
          <span>Speed</span>
          <select
            value={tier}
            disabled={direct}
            onChange={(e) => applyCountryTier(country === 'direct' ? savedCountry : country, e.target.value)}
            title="Speed tier"
          >
            <option value="stable">{tierLabel('stable')}</option>
            <option value="typical">{tierLabel('typical')}</option>
            <option value="poor">{tierLabel('poor')}</option>
          </select>
        </label>
        <span className="insp-status-grow" />
        <span>shape ~{rttMs} ms RTT</span>
        <span className="insp-status-sep" />
        <span>delay {appliedDelay} ms</span>
        <span className="insp-status-sep" />
        <span>host CF {hostCfRtt || '—'} ms</span>
        <span className="insp-status-sep" />
        <span>MITM {mitmOn ? 'on' : 'off'}</span>
        {paused && (
          <>
            <span className="insp-status-sep" />
            <span style={{ color: 'var(--warn)' }}>Paused</span>
          </>
        )}
      </div>

      <RegexEditorModal
        open={regexOpen}
        title="Inspector filter regex"
        initialPattern={search}
        ignoreCase
        engine="js"
        samplePlaceholder="https://api.example.com/v1/users?id=42"
        onClose={() => setRegexOpen(false)}
        onApply={(pattern) => {
          setSearch(pattern)
          setSearchRegex(true)
          setRegexOpen(false)
        }}
      />
    </div>
  )
}
