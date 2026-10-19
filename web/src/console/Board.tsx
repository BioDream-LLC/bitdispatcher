import PeoplePanel from './People'
import { TDoSBanner } from './Dream'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import 'maplibre-gl/dist/maplibre-gl.css'
import * as maplibregl from 'maplibre-gl'
import { Delaunay } from 'd3-delaunay'
import { Activity, AlarmClock, ArrowLeft, Clock3, House, Maximize2, Phone, ShieldAlert, Siren, Timer, Truck } from 'lucide-react'
import { api } from '../api'
import { store, useStore } from '../store'
import { Logo } from '../ui'
import { checkDue, clock, cx, discipline, hasPoint, markOutType, ms, prio, statusMeta, typeName, useNow } from '../util'
import { applyLens, DISCIPLINES, lensName, parseLens } from '../lens'
import type { Incident, Unit, View } from '../types'
import Chart, { base, type ChartOption } from '../admin/Chart'
import { mapBaseStyle } from './MapView'
import { LensChips } from './ScreensDialog'
import { markOutIcon } from './MarkOut'

// ---- Areas: the county's zones when it has them, else each station's area (nearest station) ----

type Ring = [number, number][] // lon, lat
type Area = { id: string; name: string; short: string; polys: Ring[][]; label: [number, number] }
type AreaStats = { a: Area; open: Incident[]; pending: Incident[]; avail: Unit[]; busy: Unit[]; oldest: number; level: number; swamped: boolean }

function inRing(x: number, y: number, r: Ring): boolean {
  let inside = false
  for (let i = 0, j = r.length - 1; i < r.length; j = i++) {
    const [xi, yi] = r[i], [xj, yj] = r[j]
    if (yi > y !== yj > y && x < ((xj - xi) * (y - yi)) / (yj - yi) + xi) inside = !inside
  }
  return inside
}
const inPoly = (x: number, y: number, p: Ring[]) => inRing(x, y, p[0]) && !p.slice(1).some((h) => inRing(x, y, h))

function centroid(r: Ring): [number, number] {
  let a = 0, cx0 = 0, cy0 = 0
  for (let i = 0, j = r.length - 1; i < r.length; j = i++) {
    const f = r[j][0] * r[i][1] - r[i][0] * r[j][1]
    a += f
    cx0 += (r[j][0] + r[i][0]) * f
    cy0 += (r[j][1] + r[i][1]) * f
  }
  return a ? [cx0 / (3 * a), cy0 / (3 * a)] : r[0]
}

type Zones = { type: 'FeatureCollection'; features: { properties: { name: string }; geometry: { type: 'MultiPolygon'; coordinates: Ring[][] } }[] }

function useAreas(view: View, zones: Zones | null): { areas: Area[]; locate: (lat: number, lon: number) => number; kind: 'zones' | 'stations' } {
  const stations = view.stations
  return useMemo(() => {
    if (zones && zones.features.length) {
      const areas = zones.features.map((f, i) => {
        const polys = f.geometry.coordinates
        const big = polys.map((p) => p[0]).sort((a, b) => b.length - a.length)[0]
        return { id: 'z' + i, name: f.properties.name || `Zone ${i + 1}`, short: f.properties.name || `Z${i + 1}`, polys, label: centroid(big) }
      })
      return { areas, kind: 'zones' as const, locate: (lat: number, lon: number) => areas.findIndex((a) => a.polys.some((p) => inPoly(lon, lat, p))) }
    }
    // Station areas: the nearest station, on a plane scaled for the latitude.
    const pts = stations.filter((s) => s.lat || s.lon)
    if (!pts.length) return { areas: [], kind: 'stations' as const, locate: () => -1 }
    const lat0 = pts.reduce((m, s) => m + s.lat, 0) / pts.length
    const k = Math.cos((lat0 * Math.PI) / 180)
    const del = Delaunay.from(pts, (s) => s.lon * k, (s) => s.lat)
    const lons = pts.map((s) => s.lon), lats = pts.map((s) => s.lat)
    const pad = 0.12
    const vor = del.voronoi([(Math.min(...lons) - pad) * k, Math.min(...lats) - pad, (Math.max(...lons) + pad) * k, Math.max(...lats) + pad])
    const areas = pts.map((s, i) => {
      const cell = (vor.cellPolygon(i) ?? []).map(([x, y]) => [x / k, y] as [number, number])
      return { id: s.id, name: `${s.name}`, short: s.id, polys: [[cell]], label: [s.lon, s.lat] as [number, number] }
    })
    return { areas, kind: 'stations' as const, locate: (lat: number, lon: number) => (pts.length ? del.find(lon * k, lat) : -1) }
  }, [zones, stations])
}

/** Where a unit is: its AVL, else where it was marked out, else its station. */
function unitPoint(view: View, u: Unit): { lat: number; lon: number } | null {
  if (u.position) return u.position
  if (hasPoint(u.markOut?.location)) return { lat: u.markOut!.location!.lat!, lon: u.markOut!.location!.lon! }
  const st = view.stations.find((s) => s.id === (u.covering || u.station))
  return st ? { lat: st.lat, lon: st.lon } : null
}

function areaStats(view: View, areas: Area[], locate: (lat: number, lon: number) => number): AreaStats[] {
  const out: AreaStats[] = areas.map((a) => ({ a, open: [], pending: [], avail: [], busy: [], oldest: 0, level: 0, swamped: false }))
  for (const i of view.incidents) {
    if (i.status === 'closed' || !hasPoint(i.location)) continue
    const k = locate(i.location.lat, i.location.lon)
    if (k < 0) continue
    out[k].open.push(i)
    if (i.status === 'pending') {
      out[k].pending.push(i)
      out[k].oldest = out[k].oldest ? Math.min(out[k].oldest, ms(i.created)) : ms(i.created)
    }
  }
  for (const u of view.units) {
    const p = unitPoint(view, u)
    if (!p) continue
    const k = locate(p.lat, p.lon)
    if (k < 0) continue
    if (u.status === 'available') out[k].avail.push(u)
    else if (u.status !== 'oos') out[k].busy.push(u)
  }
  for (const s of out) {
    s.level = s.open.length + s.pending.length * 2
    s.swamped = s.pending.length > 0 && (s.avail.length === 0 || s.pending.length >= 2)
  }
  return out
}

// ---- The map ----

const LEVEL_COLOURS: [number, string][] = [[0, '#1e293b'], [1, '#854d0e'], [3, '#c2410c'], [5, '#e11d48'], [8, '#be123c']]

function BoardMap({ view, stats, kind }: { view: View; stats: AreaStats[]; kind: 'zones' | 'stations' }) {
  const box = useRef<HTMLDivElement>(null)
  const map = useRef<maplibregl.Map | null>(null)
  const labels = useRef(new Map<string, maplibregl.Marker>())
  const [ready, setReady] = useState(false)
  const now = useNow()
  useEffect(() => {
    if (!box.current) return
    const m = new maplibregl.Map({
      container: box.current, style: mapBaseStyle(view, 'night', false), center: [view.settings.mapCenter[1], view.settings.mapCenter[0]], zoom: view.settings.mapZoom - 0.6,
      attributionControl: { compact: true }, dragRotate: false, pitchWithRotate: false,
    })
    m.on('load', () => {
      m.addSource('areas', { type: 'geojson', data: { type: 'FeatureCollection', features: [] } })
      m.addSource('incs', { type: 'geojson', data: { type: 'FeatureCollection', features: [] } })
      m.addSource('units', { type: 'geojson', data: { type: 'FeatureCollection', features: [] } })
      m.addLayer({ id: 'area-fill', type: 'fill', source: 'areas', paint: { 'fill-color': ['step', ['get', 'level'], ...LEVEL_COLOURS.flatMap(([l, c], i) => (i === 0 ? [c] : [l, c]))] as unknown as maplibregl.ExpressionSpecification, 'fill-opacity': ['case', ['>', ['get', 'level'], 0], 0.42, 0.12] } })
      m.addLayer({ id: 'area-line', type: 'line', source: 'areas', paint: { 'line-color': ['case', ['get', 'swamped'], '#fb7185', '#64748b'], 'line-width': ['case', ['get', 'swamped'], 3, 1], 'line-opacity': 0.85 } })
      m.addLayer({ id: 'unit-dot', type: 'circle', source: 'units', paint: { 'circle-radius': 4.5, 'circle-color': ['get', 'c'], 'circle-stroke-color': '#020617', 'circle-stroke-width': 1.2 } })
      m.addLayer({ id: 'inc-halo', type: 'circle', source: 'incs', filter: ['==', ['get', 'pending'], true], paint: { 'circle-radius': 14, 'circle-color': ['get', 'c'], 'circle-opacity': 0.22 } })
      m.addLayer({ id: 'inc-dot', type: 'circle', source: 'incs', paint: { 'circle-radius': 7, 'circle-color': ['get', 'c'], 'circle-stroke-color': '#f8fafc', 'circle-stroke-width': ['case', ['get', 'pending'], 2.5, 1] } })
      setReady(true)
    })
    map.current = m
    return () => {
      m.remove()
      map.current = null
      labels.current.clear()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view.health.map?.built, view.health.map?.recorded, view.health.tiles])

  // Fit the areas with work in them, once there is something to show.
  const fitted = useRef(false)
  useEffect(() => {
    const m = map.current
    if (!m || !ready || fitted.current || !stats.length) return
    const b = new maplibregl.LngLatBounds()
    for (const s of stats) if (s.open.length || s.avail.length || s.busy.length) b.extend(s.a.label)
    view.incidents.forEach((i) => hasPoint(i.location) && i.status !== 'closed' && b.extend([i.location.lon, i.location.lat]))
    if (!b.isEmpty()) m.fitBounds(b, { padding: 60, maxZoom: 13.5, duration: 0 })
    fitted.current = true
  }, [ready, stats, view.incidents])

  useEffect(() => {
    const m = map.current
    if (!m || !ready) return
    ;(m.getSource('areas') as maplibregl.GeoJSONSource).setData({
      type: 'FeatureCollection',
      features: stats.map((s) => ({ type: 'Feature', properties: { level: s.level, swamped: s.swamped, name: s.a.name }, geometry: { type: 'MultiPolygon', coordinates: s.a.polys } })),
    })
    ;(m.getSource('incs') as maplibregl.GeoJSONSource).setData({
      type: 'FeatureCollection',
      features: view.incidents.filter((i) => i.status !== 'closed' && hasPoint(i.location)).map((i) => ({
        type: 'Feature', properties: { c: prio(i.priority).hex, pending: i.status === 'pending' }, geometry: { type: 'Point', coordinates: [i.location!.lon!, i.location!.lat!] },
      })),
    })
    ;(m.getSource('units') as maplibregl.GeoJSONSource).setData({
      type: 'FeatureCollection',
      features: view.units.filter((u) => u.status !== 'oos').flatMap((u) => {
        const p = unitPoint(view, u)
        return p ? [{ type: 'Feature' as const, properties: { c: statusMeta(u.status).dot }, geometry: { type: 'Point' as const, coordinates: [p.lon, p.lat] } }] : []
      }),
    })
    // Area labels: the name and its work, as HTML so they need no map fonts.
    const seen = new Set<string>()
    for (const s of stats) {
      if (!s.open.length && !s.avail.length && kind === 'zones') continue
      seen.add(s.a.id)
      let mk = labels.current.get(s.a.id)
      if (!mk) {
        const el = document.createElement('div')
        el.style.pointerEvents = 'none'
        mk = new maplibregl.Marker({ element: el, anchor: 'top', offset: [0, 10] }).setLngLat(s.a.label).addTo(m)
        labels.current.set(s.a.id, mk)
      }
      const el = mk.getElement()
      const html = `<div class="rounded-md border ${s.swamped ? 'border-rose-400 bg-rose-950/90 flash' : 'border-slate-700 bg-slate-950/85'} px-1.5 py-0.5 text-center shadow-xl">
        <div class="font-mono text-[11px] font-black text-slate-50">${s.a.short}</div>
        <div class="whitespace-nowrap font-mono text-[10px] ${s.open.length ? 'text-amber-200' : 'text-slate-500'}">${s.open.length} open${s.pending.length ? ` · <b class="text-rose-200">${s.pending.length} waiting</b>` : ''}</div>
        <div class="whitespace-nowrap font-mono text-[10px] ${s.avail.length ? 'text-emerald-300' : 'text-rose-300'}">${s.avail.length} available</div></div>`
      if (el.innerHTML !== html) el.innerHTML = html
    }
    labels.current.forEach((mk, k) => {
      if (!seen.has(k)) {
        mk.remove()
        labels.current.delete(k)
      }
    })
  }, [stats, ready, view, kind, now])

  // MapLibre makes its container position: relative, so the container sits inside the absolute box.
  return <div className="absolute inset-0"><div ref={box} className="h-full w-full" aria-label="Map of open incidents and available units by area" /></div>
}

// ---- Tiles ----

function Kpi({ icon, label, value, sub, tone = 'slate', alarm, children }: { icon: React.ReactNode; label: string; value: React.ReactNode; sub?: React.ReactNode; tone?: 'slate' | 'rose' | 'amber' | 'emerald' | 'sky' | 'teal' | 'orange'; alarm?: boolean; children?: React.ReactNode }) {
  const fg = { slate: 'text-slate-100', rose: 'text-rose-300', amber: 'text-amber-300', emerald: 'text-emerald-300', sky: 'text-sky-300', teal: 'text-teal-300', orange: 'text-orange-300' }[tone]
  return (
    <div className={cx('relative flex min-w-0 flex-col overflow-hidden rounded-xl border px-3 py-2', alarm ? 'border-rose-500/70 bg-rose-950/40' : 'border-slate-800 bg-slate-900/70')}>
      {alarm && <span className="absolute inset-x-0 top-0 h-1 bg-rose-500 live-dot" aria-hidden />}
      <div className="flex items-center gap-1.5 text-[11px] font-semibold uppercase tracking-wider text-slate-400">{icon}{label}</div>
      <div className={cx('mt-0.5 font-mono text-[44px] font-black leading-none tnum', fg)}>{value}</div>
      {sub && <div className="mt-1 truncate text-[12px] text-slate-400">{sub}</div>}
      {children}
    </div>
  )
}

/** A ring of statuses: each status its own arc, the number available in the middle. */
function Donut({ units, size = 112 }: { units: Unit[]; size?: number }) {
  const order = ['available', 'dispatched', 'enroute', 'onscene', 'transporting', 'atdestination', 'markout', 'oos']
  const r = size / 2 - 9, c = 2 * Math.PI * r
  let off = 0
  const av = units.filter((u) => u.status === 'available').length
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} role="img" aria-label={`${av} of ${units.length} available`}>
      <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="#1e293b" strokeWidth="13" />
      {order.map((s) => {
        const n = units.filter((u) => u.status === s).length
        if (!n) return null
        const len = (n / units.length) * c
        const el = <circle key={s} cx={size / 2} cy={size / 2} r={r} fill="none" stroke={statusMeta(s).dot} strokeWidth="13" strokeDasharray={`${Math.max(0, len - 1.5)} ${c}`} strokeDashoffset={-off} transform={`rotate(-90 ${size / 2} ${size / 2})`}><title>{n} {statusMeta(s).label}</title></circle>
        off += len
        return el
      })}
      <text x="50%" y="47%" textAnchor="middle" fontSize={size * 0.27} fontWeight="900" fill={av === 0 ? '#fda4af' : av <= 1 ? '#fcd34d' : '#6ee7b7'} fontFamily="ui-monospace, monospace">{av}</text>
      <text x="50%" y="66%" textAnchor="middle" fontSize={size * 0.1} fill="#94a3b8">of {units.length} free</text>
    </svg>
  )
}

function DisciplineCard({ view, d, units }: { view: View; d: (typeof DISCIPLINES)[number]; units: Unit[] }) {
  const types = view.settings.unitTypes.filter((t) => t.discipline === d.d && units.some((u) => u.type === t.code))
  return (
    <div className="flex gap-3 rounded-xl border border-slate-800 bg-slate-900/70 p-2.5" style={{ boxShadow: `inset 3px 0 0 ${d.color}` }}>
      <Donut units={units} size={104} />
      <div className="min-w-0 flex-1">
        <div className="mb-1 flex items-baseline gap-2">
          <span className="text-sm font-black uppercase tracking-wider" style={{ color: d.color }}>{d.label}</span>
          <span className="ml-auto flex flex-wrap justify-end gap-1">
            {['dispatched', 'enroute', 'onscene', 'transporting', 'markout', 'oos'].map((s) => {
              const n = units.filter((u) => u.status === s).length
              return n ? <span key={s} className={cx('chip h-4 px-1 font-mono text-[10px]', statusMeta(s).bg, statusMeta(s).fg)} title={statusMeta(s).label}>{statusMeta(s).code} {n}</span> : null
            })}
          </span>
        </div>
        <div className="space-y-1">
          {types.map((t) => {
            const us = units.filter((u) => u.type === t.code)
            const av = us.filter((u) => u.status === 'available').length
            return (
              <div key={t.code} className="grid grid-cols-[6.5rem_1fr_2.6rem] items-center gap-2 text-[12px]">
                <span className="truncate text-slate-300">{t.name}</span>
                <span className="flex h-2.5 overflow-hidden rounded-full bg-slate-800">
                  {['available', 'dispatched', 'enroute', 'onscene', 'transporting', 'atdestination', 'markout', 'oos'].map((s) => {
                    const n = us.filter((u) => u.status === s).length
                    return n ? <span key={s} style={{ width: `${(n / us.length) * 100}%`, background: statusMeta(s).dot }} /> : null
                  })}
                </span>
                <span className={cx('text-right font-mono font-bold tnum', av === 0 ? 'text-rose-300' : 'text-slate-200')}>{av}<span className="text-slate-500">/{us.length}</span></span>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

/** The supervisor's board: what is free, what is waiting, and where the work is piling up. Read-only. */
export default function Board() {
  const raw = useStore((s) => s.view)
  const lens = useStore((s) => s.lens)
  const loadError = useStore((s) => s.loadError)
  const stream = useStore((s) => s.stream)
  const now = useNow()
  const [zones, setZones] = useState<Zones | null>(null)
  const built = raw?.health.map?.built
  useEffect(() => {
    api.get<Zones>('/map/zones').then(setZones).catch(() => setZones(null))
  }, [built])
  useEffect(() => {
    document.title = 'Supervisor board · bitdispatcher'
    // Without a lens in its address the board shows what this tab chose, not the last screen's.
    if (!new URLSearchParams(location.hash.split('?')[1] ?? '').has('d')) store.set({ lens: parseLens(sessionStorage.getItem('bd.lens')) })
  }, [])
  const view = useMemo(() => (raw ? applyLens(raw, lens) : null), [raw, lens])
  const { areas, locate, kind } = useAreas(raw ?? ({ stations: [] } as unknown as View), zones)
  const stats = useMemo(() => (view ? areaStats(view, areas, locate) : []), [view, areas, locate])
  const spark = useMemo<ChartOption | null>(() => {
    if (!view) return null
    const step = 15 * 60000, n = 24
    const end = Math.ceil(now / step) * step
    const buckets = Array.from({ length: n }, () => [0, 0, 0])
    for (const i of view.incidents) {
      const k = Math.floor((ms(i.created) - (end - n * step)) / step)
      if (k >= 0 && k < n) buckets[k][i.priority <= 1 ? 0 : i.priority <= 2 ? 1 : 2]++
    }
    const labels = buckets.map((_, k) => new Date(end - (n - k) * step).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false }))
    return {
      ...base, grid: { left: 24, right: 8, top: 8, bottom: 18 },
      xAxis: { type: 'category', data: labels, axisLabel: { interval: 3, color: '#64748b', fontSize: 9 }, axisLine: { lineStyle: { color: '#334155' } }, axisTick: { show: false } },
      yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: '#1e293b' } }, axisLabel: { color: '#64748b', fontSize: 9 } },
      series: [['P1', '#f43f5e'], ['P2', '#f97316'], ['P3–5', '#38bdf8']].map(([name, color], j) => ({ name, type: 'bar', stack: 'n', data: buckets.map((b) => b[j]), itemStyle: { color }, barWidth: '70%' })),
    }
    // A new bucket every 15 minutes is enough.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [view, Math.floor(now / 60000)])

  if (!view || !raw) return <div className="grid h-full place-items-center text-sm text-slate-500">{loadError ? `Cannot load: ${loadError}` : 'Loading the board…'}</div>

  const calls = view.calls.filter((c) => c.state !== 'ended' && !c.incident)
  const ringing = calls.filter((c) => c.state === 'ringing').length
  const open = view.incidents.filter((i) => i.status !== 'closed')
  const pending = open.filter((i) => i.status === 'pending')
  const oldest = pending.reduce((m, i) => Math.min(m, ms(i.created)), Infinity)
  const active = open.filter((i) => i.status === 'active')
  const avail = view.units.filter((u) => u.status === 'available')
  const inService = view.units.filter((u) => u.status !== 'oos')
  const committed = view.units.filter((u) => u.incident)
  const marked = view.units.filter((u) => u.status === 'markout')
  const due = view.units.filter((u) => checkDue(view, u) && now > checkDue(view, u))
  const dueSoon = view.units.filter((u) => checkDue(view, u) && now <= checkDue(view, u) && checkDue(view, u) - now < 5 * 60000)
  const houses = view.stations.filter((s) => view.units.some((u) => u.station === s.id || u.covering === s.id))
  const emptyHouses = houses.filter((s) => !view.units.some((u) => (u.covering || u.station) === s.id && u.status === 'available'))
  const emergencies = raw.emergencies.filter((e) => !e.ackedAt)
  const byP = [1, 2, 3, 4, 5].map((p) => pending.filter((i) => i.priority === p).length)
  const discs = DISCIPLINES.filter((d) => view.units.some((u) => discipline(view, u) === d.d))
  const ranked = [...stats].filter((s) => s.open.length || s.pending.length).sort((a, b) => b.level - a.level || a.a.name.localeCompare(b.a.name))
  const quiet = stats.filter((s) => !s.open.length).length
  const unplaced = open.filter((i) => !hasPoint(i.location)).length
  const moKinds = new Map<string, number>()
  marked.forEach((u) => moKinds.set(u.markOut?.code ?? '?', (moKinds.get(u.markOut?.code ?? '?') ?? 0) + 1))
  const local = new Date(now)

  return (
    <div className="flex h-full min-h-[640px] flex-col overflow-hidden bg-slate-950 text-slate-200">
      <header className="flex h-14 shrink-0 items-center gap-3 border-b border-slate-800 px-4">
        <Logo size={24} />
        <div className="leading-tight">
          <div className="text-lg font-black tracking-tight text-slate-50">Supervisor board</div>
          <div className="text-[11px] text-slate-400">{raw.settings.agency || 'bitdispatcher'} · {lensName(lens)}</div>
        </div>
        <LensChips lens={lens} onChange={(l) => store.setLens(l)} size="xs" />
        <div className="ml-auto flex items-center gap-4">
          <span className={cx('inline-flex items-center gap-1.5 text-xs font-bold', stream === 'live' ? 'text-emerald-300' : 'text-rose-300')}>
            <span className={cx('h-2 w-2 rounded-full', stream === 'live' ? 'bg-emerald-400 live-dot' : 'bg-rose-500')} />{stream === 'live' ? 'LIVE' : 'NOT LIVE: reconnecting'}
          </span>
          <div className="text-right font-mono leading-tight tnum">
            <div className="text-[26px] font-black text-slate-50">{local.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })}</div>
            <div className="text-[10px] text-slate-500">{local.toLocaleDateString([], { weekday: 'long', month: 'short', day: 'numeric' })}</div>
          </div>
          <button className="btn btn-ghost px-1.5" onClick={() => document.documentElement.requestFullscreen?.().catch(() => {})} aria-label="Full screen" title="Full screen"><Maximize2 size={16} aria-hidden /></button>
          <Link to="/" className="btn btn-ghost px-1.5" aria-label="Back to the console" title="Back to the console"><ArrowLeft size={16} aria-hidden /></Link>
        </div>
      </header>
      {emergencies.length > 0 && (
        <div className="flash flex items-center gap-2 px-4 py-1.5 text-sm font-black text-white" role="alert">
          <Siren size={16} aria-hidden /> {emergencies.length} EMERGENCY {emergencies.map((e) => e.unit || e.alias || e.radioId).join(', ')}: not acknowledged
        </div>
      )}
      <TDoSBanner view={raw} />
      <div className="grid shrink-0 grid-cols-7 gap-2 p-2">
        <Kpi icon={<Phone size={13} aria-hidden />} label="911 calls" value={calls.length} tone={ringing ? 'rose' : calls.length ? 'sky' : 'slate'} alarm={ringing > 0}
          sub={ringing ? <span className="font-bold text-rose-200">{ringing} ringing</span> : calls.length ? `${calls.length} not yet on an incident` : 'none waiting'} />
        <Kpi icon={<Clock3 size={13} aria-hidden />} label="Waiting for a unit" value={pending.length} tone={byP[0] ? 'rose' : pending.length ? 'amber' : 'slate'} alarm={byP[0] > 0}
          sub={pending.length ? <>oldest <span className="font-mono font-bold text-amber-200">{clock(oldest, now)}</span></> : 'every incident has a unit'}>
          {pending.length > 0 && (
            <div className="mt-1.5 flex h-2 overflow-hidden rounded-full bg-slate-800" aria-label={byP.map((n, i) => `${n} P${i + 1}`).join(', ')}>
              {byP.map((n, i) => (n ? <span key={i} style={{ width: `${(n / pending.length) * 100}%`, background: prio(i + 1).hex }} title={`${n} P${i + 1}`} /> : null))}
            </div>
          )}
        </Kpi>
        <Kpi icon={<Activity size={13} aria-hidden />} label="Active incidents" value={active.length} tone="orange" sub={`${committed.length} unit${committed.length === 1 ? '' : 's'} committed`} />
        <Kpi icon={<Truck size={13} aria-hidden />} label="Units available" value={<>{avail.length}<span className="text-2xl text-slate-500">/{inService.length}</span></>} tone={avail.length === 0 ? 'rose' : avail.length <= 2 ? 'amber' : 'emerald'} alarm={avail.length === 0 && inService.length > 0}
          sub={`${Math.round((avail.length / Math.max(1, inService.length)) * 100)}% of units in service`} />
        <Kpi icon={<Timer size={13} aria-hidden />} label="Marked out" value={marked.length} tone="teal"
          sub={moKinds.size ? <span className="flex flex-wrap gap-1">{[...moKinds].map(([k, n]) => {
            const Icon = markOutIcon(k)
            return <span key={k} className="inline-flex items-center gap-0.5 font-mono text-[11px] text-teal-200" title={markOutType(view, k)?.name}><Icon size={11} aria-hidden />{k} {n}</span>
          })}</span> : 'none'} />
        <Kpi icon={<AlarmClock size={13} aria-hidden />} label="Status checks due" value={due.length} tone={due.length ? 'rose' : 'slate'} alarm={due.length > 0}
          sub={due.length ? <span className="font-mono text-rose-200">{due.map((u) => u.id).join(' ')}</span> : dueSoon.length ? `${dueSoon.length} due within 5 min` : 'none'} />
        <Kpi icon={<House size={13} aria-hidden />} label="Empty stations" value={<>{emptyHouses.length}<span className="text-2xl text-slate-500">/{houses.length}</span></>} tone={emptyHouses.length ? 'rose' : 'emerald'}
          sub={emptyHouses.length ? <span className="font-mono">{emptyHouses.map((s) => s.id).join(' ')}</span> : 'every house has a unit in'} />
      </div>
      <div className="grid min-h-0 flex-1 grid-cols-[1fr_minmax(26rem,32%)] gap-2 px-2 pb-2">
        <div className="flex min-h-0 flex-col gap-2">
          <section className="relative min-h-0 flex-1 overflow-hidden rounded-xl border border-slate-800">
            <BoardMap view={view} stats={stats} kind={kind} />
            <div className="pointer-events-none absolute left-2 top-2 rounded-lg border border-slate-700 bg-slate-950/85 px-2.5 py-1.5 shadow-xl">
              <div className="text-[13px] font-black text-slate-50">Where the work is</div>
              <div className="text-[10px] text-slate-400">{kind === 'zones' ? 'By the county’s zones' : 'By station area (nearest station); load the county’s zones in Admin › County GIS for beats'}</div>
              <div className="mt-1 flex items-center gap-1 text-[10px] text-slate-400">
                {LEVEL_COLOURS.map(([l, c], i) => <span key={l} className="flex items-center gap-0.5"><span className="h-2 w-3 rounded-sm" style={{ background: c, opacity: i ? 0.8 : 0.5 }} />{i === 0 ? 'quiet' : `${l}+`}</span>)}
                <span className="ml-1">· open + 2 × waiting</span>
              </div>
              <div className="mt-1 flex flex-wrap gap-2 text-[10px] text-slate-400">
                {[1, 2, 3].map((p) => <span key={p} className="flex items-center gap-0.5"><span className="h-2 w-2 rounded-full" style={{ background: prio(p).hex }} />P{p}</span>)}
                {['available', 'enroute', 'onscene', 'markout'].map((s) => <span key={s} className="flex items-center gap-0.5"><span className="h-1.5 w-1.5 rounded-full" style={{ background: statusMeta(s).dot }} />{statusMeta(s).label}</span>)}
              </div>
            </div>
          </section>
          <section className="h-32 shrink-0 rounded-xl border border-slate-800 bg-slate-900/70 px-2 pt-1.5">
            <div className="flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wider text-slate-400">New incidents, last 6 hours <span className="normal-case tracking-normal text-slate-500">per 15 minutes</span></div>
            {spark && <Chart option={spark} height={100} />}
          </section>
        </div>
        <div className="flex min-h-0 flex-col gap-2">
          {discs.map((d) => <DisciplineCard key={d.d} view={view} d={d} units={view.units.filter((u) => discipline(view, u) === d.d)} />)}
          <PeoplePanel view={raw} />
          <section className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-slate-800 bg-slate-900/70">
            <div className="flex items-center gap-2 border-b border-slate-800 px-2.5 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-slate-400">
              <ShieldAlert size={12} aria-hidden /> Busiest areas
              <span className="ml-auto normal-case tracking-normal text-slate-500">{quiet} quiet{unplaced ? ` · ${unplaced} incident${unplaced === 1 ? '' : 's'} not on the map` : ''}</span>
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto">
              {ranked.length === 0 ? <p className="p-3 text-sm text-slate-500">No open incident on the map.</p> : (
                <table className="w-full text-[12px]">
                  <thead className="text-[10px] uppercase tracking-wider text-slate-500">
                    <tr><th className="px-2 py-1 text-left">Area</th><th className="px-1 text-right">Open</th><th className="px-1 text-right">Waiting</th><th className="px-1 text-right">Free</th><th className="px-2 text-right">Longest wait</th></tr>
                  </thead>
                  <tbody>
                    {ranked.map((s) => {
                      const maxL = ranked[0].level || 1
                      return (
                        <tr key={s.a.id} className={cx('border-t border-slate-800/70', s.swamped && 'bg-rose-500/10')}>
                          <td className="px-2 py-1">
                            <div className="flex items-center gap-1.5">
                              <span className="truncate font-semibold text-slate-100">{s.a.name}</span>
                              {s.swamped && <span className="chip h-4 bg-rose-500 px-1 text-[9px] font-black text-white">SWAMPED</span>}
                            </div>
                            <div className="mt-0.5 h-1 rounded-full bg-slate-800"><div className="h-1 rounded-full" style={{ width: `${(s.level / maxL) * 100}%`, background: s.swamped ? '#f43f5e' : '#f59e0b' }} /></div>
                            <div className="mt-0.5 truncate text-[10px] text-slate-500">{[...new Set(s.open.map((i) => typeName(view, i.type)))].slice(0, 4).join(' · ')}</div>
                          </td>
                          <td className="px-1 text-right font-mono font-bold text-amber-200 tnum">{s.open.length}</td>
                          <td className={cx('px-1 text-right font-mono font-bold tnum', s.pending.length ? 'text-rose-300' : 'text-slate-600')}>{s.pending.length}</td>
                          <td className={cx('px-1 text-right font-mono font-bold tnum', s.avail.length ? 'text-emerald-300' : 'text-rose-300')}>{s.avail.length}</td>
                          <td className="px-2 text-right font-mono text-slate-300 tnum">{s.oldest ? clock(s.oldest, now) : '–'}</td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              )}
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}
