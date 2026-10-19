import { useEffect, useMemo, useRef, useState } from 'react'
import 'maplibre-gl/dist/maplibre-gl.css'
import * as maplibregl from 'maplibre-gl'
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?url'
import { toast } from 'sonner'
import { FileText, Image as ImageIcon, LocateFixed, LogOut, MapPin, Navigation, Phone, Send, ShieldAlert, Siren } from 'lucide-react'
import { Link, useNavigate } from 'react-router-dom'
import NcicPanel from '../rms/NcicPanel'
import type { Meta } from '../rms/Rms'
import { api, errText } from '../api'
import { signOut, useStore } from '../store'
import { Logo, StatusChip } from '../ui'
import PremiseBox from '../console/PremiseBox'
import { clock, cx, mi, prio, statusMeta, useNow } from '../util'
import type { MarkOutType, Incident, Position, Premise, RouteView, Station, Unit, View } from '../types'
import { dispatchStyle } from '../console/mapStyle'

maplibregl.setWorkerUrl(workerUrl)

type MDTView = {
  serverTime: string; unit: Unit; station?: Station; incident?: Incident; typeName?: string; summary?: string; partners: Unit[]; premises?: Premise[]
  emergency?: { key: string; raised: string; ackedBy?: string }; pending: Incident[]; statuses: string[]
  settings: { agency: string; mapCenter: [number, number]; mapZoom: number; avlStaleSeconds: number; markOuts?: MarkOutType[] }
  health: View['health']
}

// The next status a crew normally gives, so it is the big button.
const NEXT: Record<string, string> = { dispatched: 'enroute', enroute: 'onscene', onscene: 'available', transporting: 'atdestination', atdestination: 'available' }
const BUTTONS: { s: string; label: string; key: string }[] = [
  { s: 'enroute', label: 'En route', key: 'F1' }, { s: 'onscene', label: 'On scene', key: 'F2' }, { s: 'transporting', label: 'Transporting', key: 'F3' },
  { s: 'atdestination', label: 'At hospital', key: 'F4' }, { s: 'available', label: 'Available', key: 'F5' }, { s: 'oos', label: 'Out of service', key: 'F6' },
]

const base = () => location.origin + location.pathname.replace(/[^/]*$/, '')

/** The mobile data terminal: one screen for the crew, big targets, one tap per status. */
export default function Mdt() {
  const me = useStore((s) => s.me)
  const view = useStore((s) => s.view)
  const [m, setM] = useState<MDTView | null>(null)
  const [err, setErr] = useState('')
  const [note, setNote] = useState('')
  const [gps, setGps] = useState<'off' | 'on' | 'denied' | 'none'>('off')
  const [route, setRoute] = useState<RouteView | null>(null)
  const now = useNow()
  const nav = useNavigate()
  const [ncic, setNcic] = useState(false)
  const [rmsMeta, setRmsMeta] = useState<Meta | null>(null)
  const unitParam = me?.role === 'field' ? '' : `?unit=${encodeURIComponent(me?.console ?? '')}`
  const load = () => api.get<MDTView>('/mdt' + unitParam).then((v) => { setM(v); setErr('') }).catch((e) => setErr(errText(e)))
  // Follow every change the server announces.
  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view?.seq])

  // GPS from the device, at most every 10 seconds; the server keeps a ledger entry every 30 s or 1 km.
  useEffect(() => {
    if (!('geolocation' in navigator)) {
      setGps('none')
      return
    }
    let last = 0
    const id = navigator.geolocation.watchPosition(
      (p) => {
        setGps('on')
        if (Date.now() - last < 10000) return
        last = Date.now()
        api.post('/mdt/position' + unitParam, { lat: p.coords.latitude, lon: p.coords.longitude, accuracy: p.coords.accuracy, speed: p.coords.speed ?? 0, heading: p.coords.heading ?? 0 }).catch(() => {})
      },
      (e) => setGps(e.code === e.PERMISSION_DENIED ? 'denied' : 'off'),
      { enableHighAccuracy: true, maximumAge: 5000 },
    )
    return () => navigator.geolocation.clearWatch(id)
  }, [unitParam])

  const u = m?.unit
  const inc = m?.incident
  const here: Position | { lat: number; lon: number } | undefined = u?.position ?? (m?.station ? { lat: m.station.lat, lon: m.station.lon } : undefined)
  // The road route from the unit to its incident.
  const rkey = inc?.location?.lat != null && here ? `${here.lat.toFixed(3)},${here.lon.toFixed(3)}>${inc.location.lat.toFixed(5)},${inc.location.lon!.toFixed(5)}` : ''
  useEffect(() => {
    if (!rkey || !inc?.location?.lat || !here || !m?.health.map?.recorded) {
      setRoute(null)
      return
    }
    api.post<RouteView[]>('/map/routes', { to: { lat: inc.location.lat, lon: inc.location.lon }, from: [{ id: u!.id, lat: here.lat, lon: here.lon }] })
      .then((r) => setRoute(r[0]?.ok ? r[0] : null)).catch(() => setRoute(null))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rkey])

  const setStatus = async (s: string) => {
    try {
      await api.post('/mdt/status' + unitParam, { status: s })
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const [moOpen, setMoOpen] = useState(false)
  const [moNote, setMoNote] = useState('')
  const markOut = async (code: string) => {
    try {
      await api.post('/mdt/markout' + unitParam, { code, note: moNote })
      setMoOpen(false)
      setMoNote('')
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const sendNote = async () => {
    if (!note.trim()) return
    try {
      await api.post('/mdt/note' + unitParam, { text: note })
      setNote('')
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  // Function keys: one press per status, as on a console.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'F7') {
        e.preventDefault()
        setMoOpen((o) => !o)
        return
      }
      const b = BUTTONS.find((x) => x.key === e.key)
      if (!b) return
      e.preventDefault()
      setStatus(b.s)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  if (!m) return <div className="grid h-full place-items-center text-sm text-slate-400">{err || 'Starting the terminal…'}</div>
  const next = NEXT[u!.status]
  const notes = (inc?.timeline ?? []).filter((e) => e.kind === 'note' || e.kind === 'priority-note' || e.kind === 'created').slice().reverse()
  return (
    <div className="flex h-full flex-col bg-slate-950 text-slate-100">
      <header className="flex items-center gap-3 border-b border-slate-800 bg-slate-900 px-3 py-2">
        <Logo size={22} />
        <span className="font-mono text-2xl font-black tracking-tight">{u!.id}</span>
        <StatusChip status={u!.status} />
        <span className="text-xs text-slate-400">{u!.covering ? `covering ${u!.covering}` : m.station?.name}</span>
        <span className={cx('chip ml-2', gps === 'on' ? 'bg-emerald-500/15 text-emerald-300' : 'bg-amber-500/15 text-amber-300')} title="Position from this device">
          <LocateFixed size={12} aria-hidden /> GPS {gps === 'on' ? 'sending' : gps === 'denied' ? 'blocked: allow location' : gps === 'none' ? 'not available' : 'waiting'}
        </span>
        <span className="ml-auto font-mono text-xl tnum">{new Date(now).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}</span>
        {inc && <button className="btn h-9" onClick={async () => {
          if (inc.reports?.[0]) return nav(`/rms/${inc.reports[0]}`)
          try {
            const r = await api.post<{ report: { number: string } }>('/rms/reports', { incident: inc.number, offenses: [], people: [], property: [], vehicles: [] })
            nav(`/rms/${r.report.number}`)
          } catch (e) { toast.error(errText(e)) }
        }}><FileText size={14} aria-hidden /> Report</button>}
        <button className="btn h-9" onClick={() => api.get<Meta>('/rms/meta').then((x) => { setRmsMeta(x); setNcic(true) }).catch((e) => toast.error(errText(e)))}><ShieldAlert size={14} aria-hidden /> NCIC</button>
        <Link to="/rms" className="btn h-9">My reports</Link>
        <span className="text-xs text-slate-500">{me?.name}</span>
        <button className="btn" onClick={signOut} aria-label="Sign out"><LogOut size={14} aria-hidden /></button>
      </header>
      {m.emergency && !m.emergency.ackedBy && (
        <div role="alert" className="animate-pulse bg-rose-700 px-3 py-2 text-center text-lg font-black tracking-wide">EMERGENCY SENT {clock(m.emergency.raised, now)} · waiting for dispatch to acknowledge</div>
      )}
      <div className="grid min-h-0 flex-1 grid-cols-1 lg:grid-cols-[minmax(24rem,36rem)_1fr]">
        <section className="flex min-h-0 flex-col gap-2 overflow-y-auto p-3">
          {inc ? (
            <>
              <div className="rounded-lg border border-slate-700 bg-slate-900 p-3">
                <div className="flex items-center gap-2">
                  <span className={cx('rounded px-2 py-0.5 font-mono text-lg font-black', prio(inc.priority).bg, prio(inc.priority).fg)}>P{inc.priority}</span>
                  <span className="text-xl font-bold">{m.typeName}</span>
                  <span className="ml-auto font-mono text-sm text-slate-400">{inc.number}</span>
                </div>
                <div className="mt-2 flex items-start gap-2 text-2xl font-bold leading-tight text-amber-200">
                  <MapPin size={22} className="mt-1 shrink-0" aria-hidden /> {inc.location?.address || (inc.location?.lat != null ? `${inc.location.lat.toFixed(5)}, ${inc.location.lon!.toFixed(5)}` : 'No location yet')}
                </div>
                {route && (
                  <div className="mt-1 flex items-center gap-2 text-sm text-sky-200">
                    <Navigation size={14} aria-hidden /> {Math.max(1, Math.round((route.seconds ?? 0) / 60))} min · {mi(route.metres ?? 0)}{route.via?.length ? ` via ${route.via.join(', ')}` : ''}
                  </div>
                )}
                {inc.caller?.callback && (
                  <a href={`tel:${inc.caller.callback}`} className="mt-1 flex items-center gap-2 text-sm text-slate-300"><Phone size={14} aria-hidden /> {inc.caller.name ? `${inc.caller.name} · ` : ''}{inc.caller.callback}</a>
                )}
                {m.summary && <p className="mt-2 rounded border border-sky-900/60 bg-sky-950/30 p-2 text-sm leading-snug text-slate-100" aria-label="Summary">{m.summary}</p>}
                {(m.premises?.length ?? 0) > 0 && <div className="mt-2"><PremiseBox premises={m.premises} compact /></div>}
                {m.partners.length > 0 && (
                  <div className="mt-2 flex flex-wrap gap-1">
                    {m.partners.map((p) => <span key={p.id} className={cx('chip ring-1', statusMeta(p.status).fg, statusMeta(p.status).ring)}>{p.id} {statusMeta(p.status).code}</span>)}
                  </div>
                )}
              </div>
              <div className="rounded-lg border border-slate-800 bg-slate-900/60 p-2">
                <div className="mb-1 text-[11px] font-bold uppercase tracking-wider text-slate-400">Notes from dispatch</div>
                <ul className="space-y-1 text-base">
                  {notes.length === 0 && <li className="text-sm text-slate-500">No notes yet.</li>}
                  {notes.map((n) => (
                    <li key={n.seq} className={cx('rounded px-2 py-1', n.priority ? 'bg-rose-950/60 font-semibold text-rose-100 ring-1 ring-rose-700' : 'bg-slate-950/60')}>
                      <span className="mr-2 font-mono text-xs text-slate-500">{new Date(n.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>{n.text}
                    </li>
                  ))}
                </ul>
                <form className="mt-2 flex gap-1" onSubmit={(e) => { e.preventDefault(); sendNote() }}>
                  <input className="input h-11 text-base" placeholder="Note to dispatch" value={note} onChange={(e) => setNote(e.target.value)} aria-label="Note to dispatch" />
                  <button className="btn h-11 px-4" disabled={!note.trim()}><Send size={16} aria-hidden /> Send</button>
                </form>
              </div>
            </>
          ) : (
            <div className="rounded-lg border border-slate-800 bg-slate-900 p-4">
              <div className={cx('text-2xl font-bold', u!.status === 'markout' ? 'text-teal-300' : 'text-emerald-300')}>
                {u!.status === 'available' ? 'Available' : u!.status === 'markout' ? `Marked out: ${m.settings.markOuts?.find((k) => k.code === u!.markOut?.code)?.name ?? u!.markOut?.code}` : statusMeta(u!.status).label}
              </div>
              {u!.status === 'markout' && u!.markOut?.location?.address && <div className="text-sm text-slate-300">{u!.markOut.location.address}{u!.markOut.note ? ` · ${u!.markOut.note}` : ''}</div>}
              <div className="text-sm text-slate-400">No assignment. Dispatch sends you a call here; this screen changes at once.</div>
              {m.pending.length > 0 && (
                <div className="mt-3">
                  <div className="text-[11px] font-bold uppercase tracking-wider text-amber-300">Waiting for a unit ({m.pending.length})</div>
                  <ul className="mt-1 space-y-1 text-sm">
                    {m.pending.slice(0, 8).map((p) => <li key={p.number} className="flex gap-2"><span className="font-mono font-bold">P{p.priority}</span><span>{p.type}</span><span className="truncate text-slate-400">{p.location?.address}</span></li>)}
                  </ul>
                </div>
              )}
            </div>
          )}
          {moOpen && (
            <div className="mt-auto space-y-2 rounded-lg border border-teal-700 bg-teal-950/30 p-2" aria-label="Mark out">
              <div className="flex items-center gap-2">
                <span className="text-sm font-bold text-teal-200">Mark out: what are you doing?</span>
                <input className="input ml-auto h-9 w-64 text-sm" placeholder="Note (optional)" value={moNote} onChange={(e) => setMoNote(e.target.value)} aria-label="Mark-out note" />
                <button className="btn h-9" onClick={() => setMoOpen(false)}>Cancel</button>
              </div>
              <div className="grid grid-cols-3 gap-2">
                {(m.settings.markOuts ?? []).map((k) => (
                  <button key={k.code} className="flex h-14 flex-col items-center justify-center rounded-lg border border-slate-700 bg-slate-900 font-bold text-slate-100 hover:border-teal-500 hover:bg-teal-900/40" onClick={() => markOut(k.code)}>
                    {k.name}<span className="font-mono text-[10px] font-normal text-slate-400">{k.code}{k.minutes ? ` · check ${k.minutes} min` : ''}</span>
                  </button>
                ))}
              </div>
            </div>
          )}
          <div className={cx('grid grid-cols-3 gap-2', !moOpen && 'mt-auto')}>
            {BUTTONS.map((b) => (
              <button
                key={b.s}
                className={cx('flex h-16 flex-col items-center justify-center rounded-lg border text-lg font-bold transition',
                  u!.status === b.s ? 'border-slate-600 bg-slate-700 text-white' : b.s === next ? 'border-sky-400 bg-sky-600 text-white shadow-lg shadow-sky-900' : 'border-slate-700 bg-slate-900 text-slate-200 hover:bg-slate-800')}
                aria-pressed={u!.status === b.s}
                onClick={() => setStatus(b.s)}
                disabled={!inc && ['enroute', 'onscene', 'transporting', 'atdestination'].includes(b.s)}
              >
                {b.label}<span className="font-mono text-[10px] font-normal text-slate-300/70">{b.key}</span>
              </button>
            ))}
          </div>
          <button className={cx('flex h-12 items-center justify-center gap-2 rounded-lg border text-base font-bold', u!.status === 'markout' ? 'border-teal-500 bg-teal-800/50 text-white' : 'border-slate-700 bg-slate-900 text-teal-200 hover:bg-slate-800')}
            onClick={() => setMoOpen(!moOpen)} disabled={!m.settings.markOuts?.length} aria-expanded={moOpen}>
            Mark out…<span className="font-mono text-[10px] font-normal text-slate-300/70">F7</span>
          </button>
          <EmergencyButton unitParam={unitParam} onSent={load} />
        </section>
        <MdtMap m={m} route={route} here={here} />
      </div>
      {rmsMeta && <NcicPanel open={ncic} onOpenChange={setNcic} meta={rmsMeta} reason={inc?.number ?? ''} />}
    </div>
  )
}

/** Hold for two seconds: an emergency must never be sent by brushing the screen. */
function EmergencyButton({ unitParam, onSent }: { unitParam: string; onSent: () => void }) {
  const [held, setHeld] = useState(0)
  const timer = useRef<number>(0)
  const start = () => {
    const t0 = Date.now()
    timer.current = window.setInterval(async () => {
      const f = Math.min(1, (Date.now() - t0) / 2000)
      setHeld(f)
      if (f >= 1) {
        clearInterval(timer.current)
        setHeld(0)
        try {
          await api.post('/mdt/emergency' + unitParam)
          toast.error('EMERGENCY sent to dispatch')
          onSent()
        } catch (e) {
          toast.error(errText(e))
        }
      }
    }, 50)
  }
  const stop = () => {
    clearInterval(timer.current)
    setHeld(0)
  }
  return (
    <button className="relative h-12 overflow-hidden rounded-lg border border-rose-700 bg-rose-950 text-base font-black tracking-wider text-rose-200"
      onPointerDown={start} onPointerUp={stop} onPointerLeave={stop} aria-label="Emergency: hold for two seconds">
      <span className="absolute inset-y-0 left-0 bg-rose-600" style={{ width: `${held * 100}%` }} />
      <span className="relative flex items-center justify-center gap-2"><Siren size={18} aria-hidden /> HOLD FOR EMERGENCY</span>
    </button>
  )
}

function MdtMap({ m, route, here }: { m: MDTView; route: RouteView | null; here?: { lat: number; lon: number } }) {
  const box = useRef<HTMLDivElement>(null)
  const map = useRef<maplibregl.Map | null>(null)
  const [ready, setReady] = useState(false)
  const [photos, setPhotos] = useState(() => localStorage.getItem('bd.mdtPhotos') === '1')
  const marks = useRef<maplibregl.Marker[]>([])
  const recorded = !!m.health.map?.recorded
  const ph = photos && m.health.photos?.available
  const style = useMemo(() => dispatchStyle({ theme: 'night', base: base(), online: m.health.tiles && !ph, recorded, attribution: '', photos: ph ? { attribution: m.health.photos!.attribution } : undefined }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [recorded, ph, m.health.tiles])
  useEffect(() => {
    if (!box.current) return
    const mp = new maplibregl.Map({ container: box.current, style, center: [m.settings.mapCenter[1], m.settings.mapCenter[0]], zoom: m.settings.mapZoom, attributionControl: { compact: true }, dragRotate: false })
    mp.on('load', () => setReady(true))
    map.current = mp
    const ro = new ResizeObserver(() => mp.resize())
    ro.observe(box.current)
    return () => {
      ro.disconnect()
      mp.remove()
      map.current = null
      setReady(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [style])
  const inc = m.incident
  const key = `${here?.lat},${here?.lon}|${inc?.location?.lat},${inc?.location?.lon}|${route?.path?.length}`
  useEffect(() => {
    const mp = map.current
    if (!mp || !ready) return
    marks.current.forEach((x) => x.remove())
    marks.current = []
    const b = new maplibregl.LngLatBounds()
    if (here) {
      const el = document.createElement('div')
      el.innerHTML = `<span style="display:block;padding:2px 6px;border-radius:6px;background:#0ea5e9;color:#fff;font:700 13px ui-monospace,monospace;box-shadow:0 0 0 2px #000">${m.unit.id}</span>`
      marks.current.push(new maplibregl.Marker({ element: el }).setLngLat([here.lon, here.lat]).addTo(mp))
      b.extend([here.lon, here.lat])
    }
    if (inc?.location?.lat != null) {
      const el = document.createElement('div')
      el.innerHTML = '<span style="display:block;width:18px;height:18px;transform:rotate(45deg);background:#f43f5e;border:2px solid #fff;box-shadow:0 0 0 2px #000"></span>'
      marks.current.push(new maplibregl.Marker({ element: el }).setLngLat([inc.location.lon!, inc.location.lat]).addTo(mp))
      b.extend([inc.location.lon!, inc.location.lat])
    }
    const data: GeoJSON.FeatureCollection = { type: 'FeatureCollection', features: route?.path ? [{ type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: route.path } }] : [] }
    const src = mp.getSource('route') as maplibregl.GeoJSONSource | undefined
    if (src) src.setData(data)
    else {
      mp.addSource('route', { type: 'geojson', data })
      mp.addLayer({ id: 'route-casing', type: 'line', source: 'route', paint: { 'line-color': '#000', 'line-width': 9, 'line-opacity': 0.7 }, layout: { 'line-cap': 'round', 'line-join': 'round' } })
      mp.addLayer({ id: 'route', type: 'line', source: 'route', paint: { 'line-color': '#22d3ee', 'line-width': 5 }, layout: { 'line-cap': 'round', 'line-join': 'round' } })
    }
    route?.path?.forEach((p) => b.extend(p as [number, number]))
    if (!b.isEmpty()) mp.fitBounds(b, { padding: 60, maxZoom: 17, duration: 400 })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ready, key])
  return (
    <div className="relative min-h-72">
      {/* MapLibre makes its container position: relative, so it sits inside an absolute box. */}
      <div className="absolute inset-0"><div ref={box} className="h-full w-full" /></div>
      {m.health.photos?.available && (
        <button className={cx('btn absolute left-2 top-2 h-9', ph && 'ring-1 ring-amber-400 text-amber-200')} onClick={() => setPhotos((v) => { localStorage.setItem('bd.mdtPhotos', v ? '0' : '1'); return !v })}>
          <ImageIcon size={14} aria-hidden /> Photos
        </button>
      )}
    </div>
  )
}
