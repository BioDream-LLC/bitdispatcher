import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { FilePlus2, Phone } from 'lucide-react'
import { api, errText } from '../api'
import { store } from '../store'
import { Modal } from '../ui'
import { cx, locText, phone, prio, uncertainty } from '../util'
import type { Call, SearchResult, View } from '../types'

export type NewSeed = { call?: Call; lat?: number; lon?: number; address?: string } | null

export default function NewIncident({ view, seed, onClose }: { view: View; seed: NewSeed; onClose: () => void }) {
  const [q, setQ] = useState('')
  const [type, setType] = useState('')
  const [priority, setPriority] = useState(0)
  const [address, setAddress] = useState('')
  const [lat, setLat] = useState('')
  const [lon, setLon] = useState('')
  const [callerName, setCallerName] = useState('')
  const [callback, setCallback] = useState('')
  const [narrative, setNarrative] = useState('')
  const [busy, setBusy] = useState(false)
  const [sugg, setSugg] = useState<SearchResult[]>([])
  const [verified, setVerified] = useState<SearchResult | null>(null)
  const call = seed?.call
  const mapped = !!view.health.map?.recorded
  // Address suggestions from the recorded map, nearest the caller (or the map centre) first.
  useEffect(() => {
    const s = address.trim()
    if (!mapped || s.length < 3 || (verified && verified.label === s)) {
      setSugg([])
      return
    }
    let live = true
    const near = call?.location?.lat !== undefined ? [call.location.lat, call.location.lon] : view.settings.mapCenter
    const t = setTimeout(() => {
      api.get<SearchResult[]>(`/map/search?q=${encodeURIComponent(s)}&lat=${near[0]}&lon=${near[1]}&limit=6`).then((r) => live && setSugg(r)).catch(() => live && setSugg([]))
    }, 150)
    return () => {
      live = false
      clearTimeout(t)
    }
  }, [address, mapped, verified, call, view.settings.mapCenter])
  useEffect(() => {
    if (!seed) return
    setQ('')
    setType('')
    setPriority(0)
    setNarrative('')
    setAddress(seed.address ?? call?.location?.address ?? '')
    setVerified(null)
    setLat(seed.lat?.toFixed(6) ?? (call?.location?.lat?.toFixed(6) ?? ''))
    setLon(seed.lon?.toFixed(6) ?? (call?.location?.lon?.toFixed(6) ?? ''))
    setCallerName(call?.callerName ?? '')
    setCallback(call?.callback ?? '')
  }, [seed, call])
  const types = useMemo(() => {
    const s = q.trim().toLowerCase()
    return view.callTypes.filter((c) => !s || c.code.toLowerCase().startsWith(s) || c.name.toLowerCase().includes(s)).sort((a, b) => a.priority - b.priority || a.name.localeCompare(b.name))
  }, [q, view.callTypes])
  const chosen = view.callTypes.find((c) => c.code === type)
  const submit = async () => {
    if (!type) return
    setBusy(true)
    const la = parseFloat(lat), lo = parseFloat(lon)
    const hasPt = !isNaN(la) && !isNaN(lo)
    const sameAsCall = call?.location && hasPt && call.location.lat === la && call.location.lon === lo && (call.location.address ?? '') === address
    try {
      const r = await api.post<{ number: string }>('/incidents', {
        type, priority: priority || undefined, call: call?.id, narrative,
        location: sameAsCall ? undefined : address || hasPt ? { address: address || undefined, lat: hasPt ? la : undefined, lon: hasPt ? lo : undefined, source: 'dispatcher' } : undefined,
        caller: call ? undefined : callerName || callback ? { name: callerName, callback } : undefined,
      })
      toast.success(`Created ${r.number}`)
      store.select(r.number)
      onClose()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal
      open={!!seed}
      onClose={onClose}
      wide
      title={call ? `New incident from ${phone(call.callback)}` : 'New incident'}
      icon={<FilePlus2 size={16} className="text-rose-300" aria-hidden />}
      footer={
        <>
          <span className="mr-auto text-[11px] text-slate-500">Enter in the type list chooses · Ctrl+Enter creates</span>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn-go" disabled={!type || busy} onClick={submit}>Create incident</button>
        </>
      }
    >
      <div className="grid gap-3 md:grid-cols-[1fr_1fr]" onKeyDown={(e) => e.key === 'Enter' && e.ctrlKey && submit()}>
        <div className="flex min-h-0 flex-col">
          <label className="label" htmlFor="ct">Call type</label>
          <input
            id="ct"
            className="input font-mono"
            autoFocus
            placeholder="Code or name: SF, chest, MVA…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.ctrlKey && types[0]) {
                e.preventDefault()
                setType(types[0].code)
                setQ(types[0].code)
              }
            }}
          />
          <ul className="mt-1 max-h-72 overflow-y-auto rounded border border-slate-800" role="listbox" aria-label="Call types">
            {types.map((c) => (
              <li key={c.code}>
                <button
                  type="button"
                  role="option"
                  aria-selected={type === c.code}
                  onClick={() => setType(c.code)}
                  className={cx('flex w-full items-center gap-2 px-2 py-1 text-left text-xs hover:bg-slate-800', type === c.code && 'bg-slate-800 ring-1 ring-inset ring-rose-400')}
                >
                  <span className={cx('chip font-mono', prio(c.priority).bg, prio(c.priority).fg)}>P{c.priority}</span>
                  <span className="w-16 font-mono font-bold text-slate-300">{c.code}</span>
                  <span className="truncate text-slate-100">{c.name}</span>
                  <span className="ml-auto truncate text-[10px] text-slate-500">{(c.runCard ?? []).map((r) => `${r.count}×${r.capability}`).join(' ')}</span>
                </button>
              </li>
            ))}
          </ul>
        </div>
        <div className="space-y-2">
          {call && (
            <div className="rounded border border-sky-900 bg-sky-950/30 p-2 text-xs">
              <div className="flex items-center gap-1 font-semibold text-sky-300"><Phone size={12} aria-hidden /> {phone(call.callback)} {call.class}</div>
              <div className="text-slate-200">{locText(call.location)}</div>
              <div className="font-mono text-[10px] text-slate-400">{uncertainty(call.location)}</div>
              <div className="mt-1 text-[10px] text-slate-500">The incident takes the call’s location and caller. Change them below if the caller says otherwise.</div>
            </div>
          )}
          <div>
            <span className="label">Priority</span>
            <div className="flex gap-1">
              <button type="button" className={cx('btn', priority === 0 && 'ring-1 ring-sky-400')} onClick={() => setPriority(0)}>
                Type’s {chosen ? `(P${chosen.priority})` : ''}
              </button>
              {[1, 2, 3, 4, 5].map((p) => (
                <button key={p} type="button" aria-pressed={priority === p} className={cx('btn w-9 font-mono', priority === p && cx(prio(p).bg, prio(p).fg))} onClick={() => setPriority(p)}>
                  P{p}
                </button>
              ))}
            </div>
          </div>
          <div>
            <label className="label" htmlFor="addr">Address or place</label>
            <input id="addr" className="input" value={address} onChange={(e) => { setAddress(e.target.value); setVerified(null) }} placeholder="1234 Williamson Rd, or Orange Ave & Williamson Rd" autoComplete="off" />
            {sugg.length > 0 && (
              <ul className="mt-1 max-h-40 overflow-y-auto rounded border border-slate-800 text-xs" role="listbox" aria-label="Addresses in the recorded map">
                {sugg.map((r) => (
                  <li key={`${r.label}${r.lat}`}>
                    <button
                      type="button"
                      role="option"
                      aria-selected={false}
                      className="flex w-full items-baseline gap-2 px-2 py-1 text-left hover:bg-slate-800"
                      onClick={() => {
                        setAddress(r.label)
                        setLat(r.lat.toFixed(6))
                        setLon(r.lon.toFixed(6))
                        setVerified(r)
                        setSugg([])
                      }}
                    >
                      <span className={cx('truncate', r.exact ? 'font-semibold text-slate-100' : 'text-slate-300')}>{r.label}</span>
                      <span className="ml-auto shrink-0 text-[10px] text-slate-500">{[r.source === 'county' ? (r.ranged ? 'county range: estimate' : 'county GIS') : r.source === 'overture' ? 'Overture' : '', r.detail, r.note].filter(Boolean).join(' · ')}</span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
            {verified && (
              <p className={cx('mt-1 text-[10px]', verified.exact ? 'text-lime-300' : 'text-amber-300')}>
                {verified.exact ? `✓ ${verified.kind === 'intersection' ? 'Intersection' : 'Address'} found in the recorded map; position set` : `Position set from ${verified.kind}: ${verified.note ?? 'not an exact match'}`}
              </p>
            )}
          </div>
          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className="label" htmlFor="lat">Latitude</label>
              <input id="lat" className="input font-mono" value={lat} onChange={(e) => setLat(e.target.value)} inputMode="decimal" />
            </div>
            <div>
              <label className="label" htmlFor="lon">Longitude</label>
              <input id="lon" className="input font-mono" value={lon} onChange={(e) => setLon(e.target.value)} inputMode="decimal" />
            </div>
          </div>
          <p className="text-[10px] text-slate-500">Without a position units cannot be ranked by distance. Right-click the map to place an incident.</p>
          {!call && (
            <div className="grid grid-cols-2 gap-2">
              <div>
                <label className="label" htmlFor="cn">Caller</label>
                <input id="cn" className="input" value={callerName} onChange={(e) => setCallerName(e.target.value)} />
              </div>
              <div>
                <label className="label" htmlFor="cb">Callback</label>
                <input id="cb" className="input font-mono" value={callback} onChange={(e) => setCallback(e.target.value)} />
              </div>
            </div>
          )}
          <div>
            <label className="label" htmlFor="nar">First note</label>
            <textarea id="nar" className="input" rows={3} value={narrative} onChange={(e) => setNarrative(e.target.value)} placeholder="What the caller reports" />
          </div>
        </div>
      </div>
    </Modal>
  )
}
