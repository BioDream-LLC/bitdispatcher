import { useEffect, useRef, useState } from 'react'
import 'maplibre-gl/dist/maplibre-gl.css'
import * as maplibregl from 'maplibre-gl'
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?url'
import PlaceViews from '../views/PlaceViews'
import { Modal } from '../ui'
import { Ban, Box, Circle as CircleIcon, Crosshair, Image as ImageIcon, Layers, MapPinPlus, Moon, Move, Search, Sun, Undo2 } from 'lucide-react'
import { toast } from 'sonner'
import { api, errText } from '../api'
import { can, store, useStore } from '../store'
import { avl, cx, hasPoint, mins, ms, prio, statusMeta, useNow, overdue } from '../util'
import type { Recommendation, RoadClosure, RouteView, SearchResult, View } from '../types'
import { closureFeatures, ClosureCard, CloseRoad, drawnFeatures, trafficFeatures, useRoads } from './Roads'
import { dispatchStyle, type Theme } from './mapStyle'
import { alertAreas } from './PublicAlerts'

maplibregl.setWorkerUrl(workerUrl)

const base = () => location.origin + location.pathname.replace(/[^/]*$/, '')

// rgbOf resolves a theme colour to rgb(): MapLibre's style parser does not read oklch().
function rgbOf(cssVar: string, fallback: string): string {
  const v = getComputedStyle(document.documentElement).getPropertyValue(cssVar).trim()
  if (!v) return fallback
  const c = document.createElement('canvas')
  c.width = c.height = 1
  const g = c.getContext('2d')
  if (!g) return fallback
  g.fillStyle = v
  g.fillRect(0, 0, 1, 1)
  const [r, gr, b] = g.getImageData(0, 0, 1, 1).data
  return `rgb(${r}, ${gr}, ${b})`
}

// areaBounds is the recorded circle's bounding box, west, south, east, north.
function areaBounds(view: View): [number, number, number, number] | undefined {
  const a = view.health.map?.area
  if (!a) return undefined
  const dLat = a.radiusM / 111320
  const dLon = a.radiusM / (111320 * Math.cos((a.lat * Math.PI) / 180))
  return [a.lon - dLon, a.lat - dLat, a.lon + dLon, a.lat + dLat]
}

/** The map's base style for a view: the recorded map, photos, online tiles or a plain background. */
export function mapBaseStyle(view: View, theme: Theme, photos: boolean): maplibregl.StyleSpecification {
  return style(view, theme, photos)
}

function style(view: View, theme: Theme, photos: boolean, threeD = false): maplibregl.StyleSpecification {
  const ph = photos && view.health.photos?.available ? { attribution: view.health.photos.attribution } : undefined
  if (view.health.map?.recorded || ph) {
    return dispatchStyle({ theme, base: base(), online: view.health.tiles && !ph, recorded: !!view.health.map?.recorded, bounds: areaBounds(view), attribution: view.settings.mapAttribution, photos: ph, gis: !!view.health.map?.gis, threeD: threeD && !!view.health.map?.recorded })
  }
  const s: maplibregl.StyleSpecification = {
    version: 8,
    sources: {},
    layers: [{ id: 'bg', type: 'background', paint: { 'background-color': rgbOf('--color-slate-950', '#0b0f17') } }],
  }
  if (view.health.tiles) {
    s.sources.tiles = { type: 'raster', tiles: [base() + 'api/tiles/{z}/{x}/{y}'], tileSize: 256, maxzoom: 19, attribution: view.settings.mapAttribution || '' }
    s.layers.push({ id: 'tiles', type: 'raster', source: 'tiles', paint: { 'raster-saturation': -0.35, 'raster-brightness-max': 0.92 } })
  }
  return s
}

// circle approximates an uncertainty circle as a polygon.
function circle(lat: number, lon: number, r: number): [number, number][] {
  const pts: [number, number][] = []
  const dLat = r / 111320
  const dLon = r / (111320 * Math.cos((lat * Math.PI) / 180))
  for (let i = 0; i <= 64; i++) {
    const a = (i / 64) * 2 * Math.PI
    pts.push([lon + dLon * Math.cos(a), lat + dLat * Math.sin(a)])
  }
  return pts
}

function grid(view: View): GeoJSON.FeatureCollection {
  const [lat, lon] = view.settings.mapCenter
  const f: GeoJSON.Feature[] = []
  const step = 0.01
  for (let i = -60; i <= 60; i++) {
    f.push({ type: 'Feature', properties: { major: i % 10 === 0 }, geometry: { type: 'LineString', coordinates: [[lon - 0.6, +(lat + i * step).toFixed(4)], [lon + 0.6, +(lat + i * step).toFixed(4)]] } })
    f.push({ type: 'Feature', properties: { major: i % 10 === 0 }, geometry: { type: 'LineString', coordinates: [[+(lon + i * step).toFixed(4), lat - 0.6], [+(lon + i * step).toFixed(4), lat + 0.6]] } })
  }
  return { type: 'FeatureCollection', features: f }
}

type Menu = { x: number; y: number; lat: number; lon: number; place?: SearchResult | null } | null

const THEME_KEY = 'bd.mapTheme'
const PHOTOS_KEY = 'bd.mapPhotos'
const THREE_D_KEY = 'bd.map3d'

export default function MapView({ view, rec, onCreateAt }: { view: View; rec: Recommendation | null; onCreateAt: (lat: number, lon: number, address?: string) => void }) {
  const box = useRef<HTMLDivElement>(null)
  const map = useRef<maplibregl.Map | null>(null)
  const markers = useRef(new Map<string, maplibregl.Marker>())
  const [ready, setReady] = useState(false)
  const [menu, setMenu] = useState<Menu>(null)
  const [show, setShow] = useState({ stations: true, units: true, oos: false, calls: true })
  const now = useNow()
  const me = useStore((s) => s.me)
  const selected = useStore((s) => s.selected)
  const selCall = useStore((s) => s.selectedCall)
  const selUnit = useStore((s) => s.selectedUnit)
  const pick = useStore((s) => s.pick)
  const tilesOn = view.health.tiles
  const recorded = !!view.health.map?.recorded
  const builtAt = view.health.map?.built ?? ''
  const [theme, setTheme] = useState<Theme>(() => (localStorage.getItem(THEME_KEY) === 'day' ? 'day' : 'night'))
  const photosOK = !!view.health.photos?.available
  const [photosOn, setPhotosOn] = useState(() => localStorage.getItem(PHOTOS_KEY) === '1')
  const photos = photosOn && photosOK
  const [viewsAt, setViewsAt] = useState<[number, number] | null>(null)
  const [threeD, setThreeD] = useState(() => localStorage.getItem(THREE_D_KEY) === '1')
  const [photoDate, setPhotoDate] = useState('')
  // Where the dispatcher was looking, so switching photos or day and night does not move the map.
  const cam = useRef<{ center: [number, number]; zoom: number } | null>(null)
  const [routes, setRoutes] = useState<Record<string, RouteView>>({})
  const [found, setFound] = useState<SearchResult | null>(null)
  // Road closures: drawn on the map by a dispatcher; the closed roads and traffic drawn from the server.
  const roads = useRoads(view)
  const [draw, setDraw] = useState<[number, number][] | null>(null)
  const drawRef = useRef(draw)
  drawRef.current = draw
  const [closeDraft, setCloseDraft] = useState<{ line?: [number, number][]; center?: [number, number]; radius?: number } | null>(null)
  const [clicked, setClicked] = useState<RoadClosure | null>(null)
  const roadsRef = useRef(roads)
  roadsRef.current = roads

  // Create the map; again when its sources change (tiles on or off, a new recorded map, day or night).
  useEffect(() => {
    if (!box.current) return
    const m = new maplibregl.Map({
      container: box.current,
      style: style(view, theme, photos, threeD),
      center: cam.current?.center ?? [view.settings.mapCenter[1], view.settings.mapCenter[0]],
      zoom: cam.current?.zoom ?? view.settings.mapZoom,
      maxZoom: 21,
      attributionControl: { compact: true },
      // In 3D the map tilts and turns (right-drag or Ctrl-drag), to see a building's sides and height.
      dragRotate: threeD,
      pitchWithRotate: threeD,
      pitch: threeD ? 55 : 0,
      maxPitch: 70,
    })
    m.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'top-right')
    m.addControl(new maplibregl.ScaleControl({ unit: 'imperial' }), 'bottom-right')
    m.on('contextmenu', (e) => {
      const at = { x: e.point.x, y: e.point.y, lat: e.lngLat.lat, lon: e.lngLat.lng }
      setMenu({ ...at, place: undefined })
      // What is here: the nearest recorded address, or the road.
      api.get<SearchResult>(`/map/reverse?lat=${at.lat.toFixed(6)}&lon=${at.lon.toFixed(6)}`)
        .then((place) => setMenu((cur) => (cur && cur.lat === at.lat && cur.lon === at.lon ? { ...cur, place } : cur)))
        .catch(() => setMenu((cur) => (cur && cur.lat === at.lat && cur.lon === at.lon ? { ...cur, place: null } : cur)))
    })
    m.on('click', (e) => {
      setMenu(null)
      if (drawRef.current) {
        const pt: [number, number] = [e.lngLat.lng, e.lngLat.lat]
        setDraw((cur) => (cur ? [...cur, pt] : cur))
        return
      }
      // A closed road clicked: what closed it, and Lift.
      const layers = ['closure-road', 'closure-area', 'closure-drawn'].filter((l) => m.getLayer(l))
      const hit = layers.length ? m.queryRenderedFeatures(e.point, { layers })[0] : undefined
      const c = hit && roadsRef.current?.closures.find((x) => x.id === hit.properties?.id)
      setClicked(c ?? null)
    })
    m.on('moveend', () => {
      const c = m.getCenter()
      cam.current = { center: [c.lng, c.lat], zoom: m.getZoom() }
    })
    m.on('load', () => setReady(true))
    map.current = m
    ;(window as unknown as { __bdMap?: maplibregl.Map }).__bdMap = m // for the screenshot scripts
    const ro = new ResizeObserver(() => m.resize())
    ro.observe(box.current)
    return () => {
      ro.disconnect()
      markers.current.forEach((mk) => mk.remove())
      markers.current.clear()
      m.remove()
      map.current = null
      setReady(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tilesOn, recorded, builtAt, theme, photos, !!view.health.map?.gis, threeD])

  // How old the photo under the map centre is, so no one takes an old photo for today.
  useEffect(() => {
    const m = map.current
    if (!photos || !m) {
      setPhotoDate('')
      return
    }
    let t = 0
    const ask = () => {
      clearTimeout(t)
      t = window.setTimeout(() => {
        const c = m.getCenter()
        api.get<{ date: string }>(`/photos/date?lat=${c.lat.toFixed(3)}&lon=${c.lng.toFixed(3)}`).then((r) => setPhotoDate(r.date)).catch(() => setPhotoDate(''))
      }, 600)
    }
    ask()
    m.on('moveend', ask)
    return () => {
      clearTimeout(t)
      m.off('moveend', ask)
    }
  }, [photos, ready])

  // F7 switches photos on and off (letters belong to the command line).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'F7' || !photosOK) return
      e.preventDefault()
      setPhotosOn((v) => {
        localStorage.setItem(PHOTOS_KEY, v ? '0' : '1')
        return !v
      })
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [photosOK])

  // Road routes for the selected incident: assigned, recommended and chosen units.
  const inc0 = view.incidents.find((i) => i.number === selected)
  const routeKey = (() => {
    if (!recorded || !inc0 || !hasPoint(inc0.location) || inc0.status === 'closed') return ''
    const want = new Set([...(rec?.units ?? []), ...pick, ...inc0.units.filter((a) => !a.cleared).map((a) => a.unit)])
    const from: string[] = []
    for (const u of view.units) {
      if (!want.has(u.id)) continue
      const p = u.position && avl(view, u, now) !== 'none' ? u.position : view.stations.find((s) => s.id === u.station)
      // Positions rounded to about 50 m, so a moving unit is re-routed as it goes rather than every second.
      if (p) from.push(`${u.id}@${p.lat.toFixed(3)},${p.lon.toFixed(3)}`)
    }
    return `${inc0.number}|${inc0.location.lat!.toFixed(5)},${inc0.location.lon!.toFixed(5)}|${from.sort().join(';')}|${view.next?.roads.version ?? 0}`
  })()
  useEffect(() => {
    if (!routeKey) {
      setRoutes({})
      return
    }
    const [, to, list] = routeKey.split('|')
    const [tlat, tlon] = to.split(',').map(Number)
    const from = list ? list.split(';').map((x) => {
      const [id, ll] = x.split('@')
      const u = view.units.find((v) => v.id === id)
      const p = u && (u.position && avl(view, u, now) !== 'none' ? u.position : view.stations.find((s) => s.id === u.station))
      const [la, lo] = ll.split(',').map(Number)
      return { id, lat: p?.lat ?? la, lon: p?.lon ?? lo }
    }) : []
    if (!from.length) {
      setRoutes({})
      return
    }
    let live = true
    api.post<RouteView[]>('/map/routes', { to: { lat: tlat, lon: tlon }, from })
      .then((r) => live && setRoutes(Object.fromEntries(r.map((x) => [x.id, x]))))
      .catch(() => live && setRoutes({}))
    return () => {
      live = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [routeKey])

  // Overlays drawn as layers: the grid (no tiles), uncertainty circles, dispatch lines.
  useEffect(() => {
    const m = map.current
    if (!m || !ready) return
    const ensure = (id: string, data: GeoJSON.FeatureCollection, add: () => void) => {
      const src = m.getSource(id) as maplibregl.GeoJSONSource | undefined
      if (src) src.setData(data)
      else {
        m.addSource(id, { type: 'geojson', data })
        add()
      }
    }
    if (!tilesOn && !recorded) {
      ensure('grid', grid(view), () =>
        m.addLayer({ id: 'grid', type: 'line', source: 'grid', paint: { 'line-color': '#64748b', 'line-opacity': ['case', ['get', 'major'], 0.32, 0.12], 'line-width': ['case', ['get', 'major'], 1, 0.5] } }),
      )
    }
    // Areas (geofences): alert areas orange, caution areas yellow, under everything else.
    const fences: GeoJSON.Feature[] = (view.roadmap?.geofences ?? []).filter((g) => g.ring.length >= 3).map((g) => ({
      type: 'Feature', properties: { kind: g.kind, name: g.name }, geometry: { type: 'Polygon', coordinates: [[...g.ring, g.ring[0]]] },
    }))
    ensure('fences', { type: 'FeatureCollection', features: fences }, () => {
      m.addLayer({ id: 'fence-fill', type: 'fill', source: 'fences', paint: { 'fill-color': ['match', ['get', 'kind'], 'caution', '#facc15', '#fb923c'], 'fill-opacity': 0.07 } })
      m.addLayer({ id: 'fence-line', type: 'line', source: 'fences', paint: { 'line-color': ['match', ['get', 'kind'], 'caution', '#facc15', '#fb923c'], 'line-width': 1.4, 'line-dasharray': [4, 2], 'line-opacity': 0.7 } })
    })
    // Public alerts in force: red for real ones, amber for tests and exercises.
    ensure('palerts', { type: 'FeatureCollection', features: alertAreas(view.roadmap?.publicAlerts) }, () => {
      m.addLayer({ id: 'palert-fill', type: 'fill', source: 'palerts', paint: { 'fill-color': ['case', ['==', ['get', 'real'], 1], '#f43f5e', '#f59e0b'], 'fill-opacity': 0.12 } })
      m.addLayer({ id: 'palert-line', type: 'line', source: 'palerts', paint: { 'line-color': ['case', ['==', ['get', 'real'], 1], '#f43f5e', '#f59e0b'], 'line-width': 2, 'line-opacity': 0.8 } })
    })
    // Traffic slowdowns under the closures; closed roads in red over the streets.
    ensure('traffic', trafficFeatures(roads), () => {
      m.addLayer({ id: 'traffic', type: 'line', source: 'traffic', layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: { 'line-color': ['step', ['get', 'factor'], '#facc15', 2, '#fb923c', 4, '#ef4444', 10, '#7f1d1d'], 'line-width': 4, 'line-opacity': 0.75 } })
    })
    ensure('closures', closureFeatures(roads), () => {
      m.addLayer({ id: 'closure-area', type: 'fill', source: 'closures', filter: ['==', ['get', 'kind'], 'area'], paint: { 'fill-color': '#e11d48', 'fill-opacity': 0.12 } })
      m.addLayer({ id: 'closure-casing', type: 'line', source: 'closures', filter: ['==', ['get', 'kind'], 'road'], layout: { 'line-cap': 'round' }, paint: { 'line-color': '#020617', 'line-width': 8, 'line-opacity': 0.7 } })
      m.addLayer({ id: 'closure-road', type: 'line', source: 'closures', filter: ['==', ['get', 'kind'], 'road'], layout: { 'line-cap': 'round' },
        paint: { 'line-color': '#f43f5e', 'line-width': 5, 'line-dasharray': [1.2, 0.8] } })
      m.addLayer({ id: 'closure-drawn', type: 'line', source: 'closures', filter: ['==', ['get', 'kind'], 'drawn'], paint: { 'line-color': '#f43f5e', 'line-width': 2, 'line-dasharray': [2, 2] } })
    })
    ensure('drawing', drawnFeatures(draw ?? []), () => {
      m.addLayer({ id: 'drawing-line', type: 'line', source: 'drawing', paint: { 'line-color': '#fda4af', 'line-width': 3, 'line-dasharray': [2, 1] } })
      m.addLayer({ id: 'drawing-pt', type: 'circle', source: 'drawing', filter: ['==', ['geometry-type'], 'Point'], paint: { 'circle-radius': 5, 'circle-color': '#f43f5e', 'circle-stroke-color': '#fff', 'circle-stroke-width': 1.5 } })
    })
    const circles: GeoJSON.Feature[] = []
    for (const c of view.calls) {
      if (c.state === 'ended' || !hasPoint(c.location) || !c.location.radius || !show.calls) continue
      circles.push({ type: 'Feature', properties: { sel: c.id === selCall ? 1 : 0 }, geometry: { type: 'Polygon', coordinates: [circle(c.location.lat, c.location.lon, c.location.radius)] } })
    }
    for (const i of view.incidents) {
      if (i.status === 'closed' || !hasPoint(i.location) || !i.location.radius) continue
      circles.push({ type: 'Feature', properties: { sel: i.number === selected ? 1 : 0 }, geometry: { type: 'Polygon', coordinates: [circle(i.location.lat, i.location.lon, i.location.radius)] } })
    }
    ensure('unc', { type: 'FeatureCollection', features: circles }, () => {
      m.addLayer({ id: 'unc-fill', type: 'fill', source: 'unc', paint: { 'fill-color': '#38bdf8', 'fill-opacity': ['case', ['==', ['get', 'sel'], 1], 0.18, 0.08] } })
      m.addLayer({ id: 'unc-line', type: 'line', source: 'unc', paint: { 'line-color': '#38bdf8', 'line-width': 1.2, 'line-dasharray': [2, 2] } })
    })
    const lines: GeoJSON.Feature[] = []
    const inc = view.incidents.find((i) => i.number === selected)
    if (inc && hasPoint(inc.location)) {
      const want = new Set([...(rec?.units ?? []), ...pick])
      for (const u of view.units) {
        const assigned = u.incident === inc.number
        if (!want.has(u.id) && !assigned) continue
        const p = u.position && avl(view, u, now) !== 'none' ? u.position : view.stations.find((s) => s.id === u.station)
        if (!p) continue
        const kind = assigned ? 'assigned' : pick.includes(u.id) ? 'pick' : 'rec'
        const rt = routes[u.id]
        if (rt?.ok && rt.path) lines.push({ type: 'Feature', properties: { kind, road: 1 }, geometry: { type: 'LineString', coordinates: rt.path } })
        else lines.push({ type: 'Feature', properties: { kind, road: 0 }, geometry: { type: 'LineString', coordinates: [[p.lon, p.lat], [inc.location.lon, inc.location.lat]] } })
      }
    }
    // A moving caller's trail: every position received, oldest faint.
    const trails: GeoJSON.Feature[] = []
    for (const c of view.calls) {
      if (c.state === 'ended' || !show.calls) continue
      // The last quarter hour only, and only positions the caller could have moved between (not a far jump
      // between two sources' fixes).
      const fx = (c.fixes ?? []).filter((f) => now - Date.parse(f.at) < 15 * 60000)
      const path: [number, number][] = []
      for (const f of fx) {
        const prev = path[path.length - 1]
        if (prev && Math.abs(prev[0] - f.lon) + Math.abs(prev[1] - f.lat) > 0.03) continue
        path.push([f.lon, f.lat])
      }
      if (path.length < 2) continue
      trails.push({ type: 'Feature', properties: { sel: c.id === selCall ? 1 : 0 }, geometry: { type: 'LineString', coordinates: path } })
    }
    ensure('trails', { type: 'FeatureCollection', features: trails }, () => {
      m.addLayer({ id: 'trails', type: 'line', source: 'trails', layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: { 'line-color': '#e879f9', 'line-width': ['case', ['==', ['get', 'sel'], 1], 3, 2], 'line-dasharray': [1, 1.5], 'line-opacity': 0.85 } })
    })
    if (found) circles.push({ type: 'Feature', properties: { sel: 1 }, geometry: { type: 'Point', coordinates: [found.lon, found.lat] } })
    ensure('lines', { type: 'FeatureCollection', features: lines }, () => {
      // Road routes: a dark casing under a bright line, so they read over any map.
      m.addLayer({ id: 'lines-casing', type: 'line', source: 'lines', filter: ['==', ['get', 'road'], 1], layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: { 'line-color': '#020617', 'line-width': ['match', ['get', 'kind'], 'pick', 7, 6], 'line-opacity': 0.75 } })
      m.addLayer({
        id: 'lines', type: 'line', source: 'lines', layout: { 'line-join': 'round', 'line-cap': 'round' },
        paint: {
          'line-color': ['match', ['get', 'kind'], 'assigned', '#fb923c', 'pick', '#f43f5e', '#a3e635'],
          'line-width': ['case', ['==', ['get', 'road'], 1], ['match', ['get', 'kind'], 'pick', 4, 3.2], ['match', ['get', 'kind'], 'pick', 2.5, 1.8]],
          'line-dasharray': ['case', ['==', ['get', 'road'], 1], ['literal', [1, 0]], ['match', ['get', 'kind'], 'assigned', ['literal', [1, 0]], ['literal', [3, 2]]]],
          'line-opacity': 0.9,
        },
      })
    })
  }, [view, ready, selected, selCall, rec, pick, now, tilesOn, show.calls, routes, found, roads, draw])

  // Markers: DOM elements, so labels need no font server.
  useEffect(() => {
    const m = map.current
    if (!m || !ready) return
    const seen = new Set<string>()
    const put = (key: string, lat: number, lon: number, html: string, cls: string, title: string, onClick: () => void, z = 0) => {
      seen.add(key)
      let mk = markers.current.get(key)
      if (!mk) {
        const el = document.createElement('button')
        el.type = 'button'
        el.addEventListener('click', (ev) => {
          ev.stopPropagation()
          ;(el as HTMLButtonElement & { _go?: () => void })._go?.()
        })
        mk = new maplibregl.Marker({ element: el, anchor: 'center' }).setLngLat([lon, lat]).addTo(m)
        markers.current.set(key, mk)
      }
      const el = mk.getElement() as HTMLButtonElement & { _go?: () => void }
      // MapLibre positions the element by its own classes; ours go on an inner span.
      const inner = `<span class="${cls}">${html}</span>`
      if (el.innerHTML !== inner) el.innerHTML = inner
      el.title = title
      el.setAttribute('aria-label', title)
      el.style.zIndex = String(z)
      el._go = onClick
      mk.setLngLat([lon, lat])
    }
    if (show.stations) {
      for (const s of view.stations) {
        put('s:' + s.id, s.lat, s.lon, `<span>${s.id}</span>`, 'bd-mk rounded-sm border border-slate-500 bg-slate-800/90 px-1 text-[9px] font-bold text-slate-300 font-mono', `Station ${s.name}`, () => {}, 1)
      }
    }
    if (show.units) {
      for (const u of view.units) {
        if (u.status === 'oos' && !show.oos) continue
        const state = avl(view, u, now)
        const st = view.stations.find((s) => s.id === u.station)
        const mo = u.status === 'markout' && hasPoint(u.markOut?.location) ? { lat: u.markOut!.location!.lat!, lon: u.markOut!.location!.lon! } : null
        const p = (state === 'fresh' ? u.position : null) ?? mo ?? u.position ?? (st ? { lat: st.lat, lon: st.lon } : null)
        if (!p) continue
        if (state === 'none' && u.status === 'available' && st) continue // at the station: the station marker stands for it
        const meta = statusMeta(u.status)
        const over = overdue(view, u, now) > 0
        const picked = pick.includes(u.id)
        const rt = routes[u.id]
        const eta = rt?.ok && rt.seconds ? `<span class="ml-1 rounded-sm bg-slate-100 px-0.5 text-slate-950">${mins(rt.seconds)}</span>` : ''
        const html = `<span class="font-mono">${u.id}</span><span class="opacity-80 ml-0.5">${u.status === 'markout' && u.markOut ? u.markOut.code : meta.code}</span>${eta}`
        put(
          'u:' + u.id, p.lat, p.lon, html,
          cx(
            'bd-mk rounded px-1 py-px text-[10px] font-bold shadow-lg ring-1 whitespace-nowrap',
            u.emergency ? 'flash text-white ring-white' : '',
            state === 'stale' ? 'border border-dashed border-slate-400 bg-slate-900/70 text-slate-300 opacity-70' : 'bg-slate-950/90',
            !u.emergency && meta.fg, meta.ring,
            over && 'overdue', picked && 'ring-2 ring-rose-400', selUnit === u.id && 'ring-2 ring-sky-300',
          ),
          `${u.id} ${meta.label}${u.incident ? ' on ' + u.incident : ''}${state === 'stale' ? ' (AVL stale)' : state === 'none' ? ' (no AVL: shown at station)' : ''}`,
          () => {
            if (store.get().selected && can.dispatch(me) && u.status !== 'oos') store.togglePick(u.id)
            else store.selectUnit(u.id === store.get().selectedUnit ? null : u.id)
          },
          over || u.emergency ? 30 : 10,
        )
      }
    }
    for (const i of view.incidents) {
      if (i.status === 'closed' || !hasPoint(i.location)) continue
      const p = prio(i.priority)
      const sel = i.number === selected
      put(
        'i:' + i.number, i.location.lat, i.location.lon,
        `<span class="font-mono">${i.number.slice(-4)}</span><span class="ml-0.5">${i.type}</span>`,
        cx('bd-mk rounded-md border-2 px-1.5 py-0.5 text-[10px] font-black shadow-xl', p.border, sel ? `${p.bg} ${p.fg} scale-125` : 'bg-slate-950/95 text-slate-100'),
        `Incident ${i.number} ${i.type} P${i.priority}`,
        () => store.select(i.number),
        sel ? 50 : 20,
      )
    }
    if (show.calls) {
      for (const c of view.calls) {
        if (c.state === 'ended' || c.incident || !hasPoint(c.location)) continue
        const ring = c.state === 'ringing'
        put(
          'c:' + c.id, c.location.lat, c.location.lon,
          `<span class="relative flex h-4 w-4 items-center justify-center"><span class="absolute inset-0 rounded-full ${ring ? 'bg-rose-500' : 'bg-sky-400'}" style="animation:ping-ring 1.4s ease-out infinite"></span><span class="relative h-3 w-3 rounded-full border-2 border-white ${ring ? 'bg-rose-500' : 'bg-sky-500'}"></span></span>`,
          'bd-mk',
          `911 caller ${c.callback ?? ''}`,
          () => store.selectCall(c.id),
          40,
        )
      }
    }
    for (const d of view.drones ?? []) {
      if (d.lat == null || d.lon == null || d.status === 'docked') continue
      put('d:' + d.id, d.lat, d.lon, `<span>✈ ${d.name ?? d.id}</span>`,
        cx('bd-mk rounded-full border border-cyan-300 px-1.5 py-px text-[10px] font-bold shadow-lg whitespace-nowrap', d.status === 'onscene' ? 'bg-cyan-400 text-slate-950' : 'bg-slate-950/90 text-cyan-200'),
        `Drone ${d.name ?? d.id}: ${d.status}${d.incident ? ' for ' + d.incident : ''}`, () => d.incident && store.select(d.incident), 35)
    }
    if (found) {
      put('f:found', found.lat, found.lon, `<span class="block h-3.5 w-3.5 rotate-45 border-2 border-white bg-fuchsia-500 shadow-lg"></span>`, 'bd-mk', `Search: ${found.label}`,
        () => setMenu(null), 45)
    }
    markers.current.forEach((mk, k) => {
      if (!seen.has(k)) {
        mk.remove()
        markers.current.delete(k)
      }
    })
  }, [view, ready, now, selected, selUnit, pick, show, me, found, routes])

  // Bring the selected incident or call into view.
  useEffect(() => {
    const m = map.current
    if (!m || !ready) return
    const i = view.incidents.find((x) => x.number === selected)
    const c = view.calls.find((x) => x.id === selCall)
    const l = c?.location ?? i?.location
    if (hasPoint(l)) m.easeTo({ center: [l.lon, l.lat], zoom: Math.max(m.getZoom(), 14), duration: 500 })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selected, selCall, ready])

  const inc = view.incidents.find((x) => x.number === selected)
  const counts = view.statuses.map((st) => [st, view.units.filter((u) => u.status === st).length] as const).filter(([, n]) => n > 0)
  const staleN = view.units.filter((u) => u.status !== 'oos' && avl(view, u, now) === 'stale').length
  const noAvl = view.units.filter((u) => u.status !== 'oos' && avl(view, u, now) === 'none').length
  return (
    <section className="panel relative min-h-0 overflow-hidden" aria-label="Map">
      <div className="absolute inset-0">
        {/* MapLibre sets position: relative on its container, so the container fills a positioned wrapper. */}
        <div ref={box} className="h-full w-full" />
      </div>
      {/* Legend and counts: what is on the map, and what is not current. */}
      <div className="pointer-events-none absolute left-2 top-2 flex max-w-[70%] flex-wrap gap-1">
        {counts.map(([st, n]) => {
          const meta = statusMeta(st)
          return (
            <span key={st} className={cx('chip pointer-events-auto bg-slate-950/85 ring-1', meta.fg, meta.ring)}>
              {meta.code} {n}
            </span>
          )
        })}
        {staleN > 0 && <span className="chip bg-amber-500/20 text-amber-300">AVL stale {staleN}</span>}
        {noAvl > 0 && <span className="chip bg-slate-950/85 text-slate-400">no AVL {noAvl}</span>}
        {!tilesOn && !recorded && <span className="chip bg-slate-950/85 text-slate-400">grid 0.01° · no map recorded (Admin › Map)</span>}
        {view.health.map?.job && <span className="chip bg-sky-500/20 text-sky-300">recording map: {view.health.map.job} {Math.round((view.health.map.jobFrac ?? 0) * 100)}%</span>}
      </div>
      <div className="absolute bottom-2 left-2 flex gap-1">
        <button className={cx('btn h-6 px-1.5 py-0 text-[10px]', show.oos && 'ring-1 ring-sky-400')} onClick={() => setShow((s) => ({ ...s, oos: !s.oos }))} aria-pressed={show.oos}>
          <Layers size={11} aria-hidden /> OOS
        </button>
        <button className={cx('btn h-6 px-1.5 py-0 text-[10px]', show.stations && 'ring-1 ring-sky-400')} onClick={() => setShow((s) => ({ ...s, stations: !s.stations }))} aria-pressed={show.stations}>
          Stations
        </button>
        {photosOK && (
          <button
            className={cx('btn h-6 px-1.5 py-0 text-[10px]', photos && 'ring-1 ring-amber-400 text-amber-200')}
            title="Aerial photos under the streets (F7)"
            aria-pressed={photos}
            onClick={() => setPhotosOn((v) => {
              localStorage.setItem(PHOTOS_KEY, v ? '0' : '1')
              return !v
            })}
          >
            <ImageIcon size={11} aria-hidden /> Photos
          </button>
        )}
        {recorded && (
          <button
            className={cx('btn h-6 px-1.5 py-0 text-[10px]', threeD && 'ring-1 ring-sky-400 text-sky-200')}
            title="Buildings in 3D: the map tilts; right-drag turns it"
            aria-pressed={threeD}
            onClick={() => setThreeD((v) => {
              localStorage.setItem(THREE_D_KEY, v ? '0' : '1')
              return !v
            })}
          >
            <Box size={11} aria-hidden /> 3D
          </button>
        )}
        {photos && photoDate && <span className="chip h-6 bg-black/70 text-[10px] text-amber-200" title="When the photo at the map centre was taken">photo {photoDate}</span>}
        {recorded && !photos && (
          <button
            className="btn h-6 px-1.5 py-0 text-[10px]"
            title="Switch the map between night and day colours"
            onClick={() => setTheme((t) => {
              const n = t === 'night' ? 'day' : 'night'
              localStorage.setItem(THEME_KEY, n)
              return n
            })}
          >
            {theme === 'night' ? <Sun size={11} aria-hidden /> : <Moon size={11} aria-hidden />} {theme === 'night' ? 'Day' : 'Night'}
          </button>
        )}
        <button
          className="btn h-6 px-1.5 py-0 text-[10px]"
          onClick={() => {
            const m = map.current
            if (!m) return
            const b = new maplibregl.LngLatBounds()
            view.units.forEach((u) => u.position && b.extend([u.position.lon, u.position.lat]))
            view.stations.forEach((s) => b.extend([s.lon, s.lat]))
            view.incidents.forEach((i) => i.status !== 'closed' && hasPoint(i.location) && b.extend([i.location.lon, i.location.lat]))
            if (!b.isEmpty()) m.fitBounds(b, { padding: 50, maxZoom: 15, duration: 500 })
          }}
        >
          <Crosshair size={11} aria-hidden /> Fit all
        </button>
      </div>
      {recorded && (
        <MapSearch
          view={view}
          onPick={(r) => {
            setFound(r)
            setMenu(null)
            map.current?.easeTo({ center: [r.lon, r.lat], zoom: Math.max(map.current.getZoom(), r.kind === 'address' ? 17.5 : r.kind === 'place' ? 14 : 16.5), duration: 600 })
          }}
          onCreate={can.dispatch(me) ? (r) => onCreateAt(r.lat, r.lon, r.kind === 'address' || r.kind === 'intersection' ? r.label : `${r.label}${r.detail ? ', ' + r.detail : ''}`) : undefined}
          found={found}
          onClear={() => setFound(null)}
        />
      )}
      {selected && pick.length > 0 && (
        <div className="absolute right-12 top-11 rounded-md bg-rose-500/90 px-2 py-1 text-[11px] font-bold text-white shadow-lg">
          Click units to add or remove · {pick.length} chosen
        </div>
      )}
      {menu && can.dispatch(me) && (
        <div className="panel absolute z-20 min-w-48 rounded-md py-1 text-xs shadow-2xl" style={{ left: menu.x, top: menu.y }} role="menu">
          {recorded && (
            <div className="max-w-64 px-3 pt-1 text-[11px]">
              {menu.place === undefined ? <span className="text-slate-500">Looking up…</span> : menu.place ? (
                <>
                  <div className={cx('font-semibold', menu.place.exact ? 'text-slate-100' : 'text-slate-300')}>{menu.place.label}</div>
                  <div className="text-[10px] text-slate-500">{[menu.place.detail, menu.place.note].filter(Boolean).join(' · ')}</div>
                </>
              ) : <span className="text-slate-500">Outside the recorded map</span>}
            </div>
          )}
          <div className="px-3 py-1 font-mono text-[10px] text-slate-500">{menu.lat.toFixed(5)}, {menu.lon.toFixed(5)}</div>
          <button
            role="menuitem"
            className="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-slate-800"
            onClick={() => {
              const pl = menu.place
              onCreateAt(menu.lat, menu.lon, pl && pl.kind === 'address' ? pl.label + (pl.detail ? ', ' + pl.detail : '') : pl ? `${pl.label}${pl.detail ? ', ' + pl.detail : ''}` : undefined)
              setMenu(null)
            }}
          >
            <MapPinPlus size={13} aria-hidden /> New incident here
          </button>
          {inc && inc.status !== 'closed' && (
            <button
              role="menuitem"
              className="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-slate-800"
              onClick={async () => {
                const lat = menu.lat, lon = menu.lon
                setMenu(null)
                try {
                  await api.patch(`/incidents/${inc.number}`, { location: { lat, lon, address: inc.location?.address, source: 'dispatcher' }, reason: 'placed on map' })
                  toast.success(`${inc.number} moved`)
                } catch (e) {
                  toast.error(errText(e))
                }
              }}
            >
              <Move size={13} aria-hidden /> Move {inc.number} here
            </button>
          )}
          {inc && inc.status !== 'closed' && !hasPoint(inc.location) && <div className="px-3 pb-1 text-[10px] text-amber-300">{inc.number} has no map position yet</div>}
          {recorded && (
            <>
              <button role="menuitem" className="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-slate-800"
                onClick={() => { setDraw([[menu.lon, menu.lat]]); setClicked(null); setMenu(null) }}>
                <Ban size={13} aria-hidden /> Close a road from here…
              </button>
              <button role="menuitem" className="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-slate-800"
                onClick={() => { setCloseDraft({ center: [menu.lat, menu.lon], radius: 60 }); setMenu(null) }}>
                <CircleIcon size={13} aria-hidden /> Close the roads around here…
              </button>
            </>
          )}
          {view.placeViews && (
            <button role="menuitem" className="flex w-full items-center gap-2 px-3 py-1.5 text-left hover:bg-slate-800"
              onClick={() => { setViewsAt([menu.lat, menu.lon]); setMenu(null) }}>
              <ImageIcon size={13} aria-hidden /> Views of this place
            </button>
          )}
        </div>
      )}
      {draw && (
        <div className="absolute inset-x-0 top-11 z-20 mx-auto flex w-fit items-center gap-2 rounded-md bg-rose-600/95 px-3 py-1.5 text-xs font-semibold text-white shadow-xl" role="status">
          <Ban size={13} aria-hidden /> Click along the closed road · {draw.length} point{draw.length === 1 ? '' : 's'}
          <button className="btn h-6 px-2 py-0 text-[11px]" disabled={draw.length < 2} onClick={() => { setCloseDraft({ line: draw }); setDraw(null) }}>Done</button>
          <button className="btn h-6 px-2 py-0 text-[11px]" disabled={draw.length < 2} onClick={() => setDraw(draw.slice(0, -1))} aria-label="Undo the last point"><Undo2 size={11} aria-hidden /></button>
          <button className="btn h-6 px-2 py-0 text-[11px]" onClick={() => setDraw(null)}>Cancel</button>
        </div>
      )}
      {clicked && (
        <div className="panel absolute left-2 top-11 z-20 w-72 rounded-md p-2 shadow-2xl">
          <button className="absolute right-1 top-1 text-slate-500 hover:text-slate-200" onClick={() => setClicked(null)} aria-label="Close">×</button>
          <ClosureCard c={clicked} canLift={can.dispatch(me)} onDone={() => setClicked(null)} />
        </div>
      )}
      <CloseRoad draft={closeDraft} incident={inc && inc.status !== 'closed' ? inc.number : undefined} onClose={() => setCloseDraft(null)} />
      <Modal open={!!viewsAt} onClose={() => setViewsAt(null)} title="Views of this place" xl>
        {viewsAt && <PlaceViews lat={viewsAt[0]} lon={viewsAt[1]} big />}
      </Modal>
      {inc && !hasPoint(inc.location) && inc.status !== 'closed' && (
        <div className="absolute inset-x-0 bottom-10 mx-auto w-fit rounded-md bg-amber-500/90 px-3 py-1 text-xs font-semibold text-slate-950 shadow-lg">
          {inc.number} has no map position: right-click the map to place it (needed to rank units by distance)
        </div>
      )}
      <span className="sr-only">{`${view.units.length} units, ${view.incidents.filter((i) => i.status !== 'closed').length} open incidents, last change ${new Date(ms(view.serverTime)).toLocaleTimeString()}`}</span>
    </section>
  )
}

// MapSearch finds an address, intersection, street or landmark in the recorded map. Enter takes the first
// answer; the arrow keys move through the list.
function MapSearch({ view, onPick, onCreate, found, onClear }: {
  view: View; onPick: (r: SearchResult) => void; onCreate?: (r: SearchResult) => void; found: SearchResult | null; onClear: () => void
}) {
  const [q, setQ] = useState('')
  const [res, setRes] = useState<SearchResult[]>([])
  const [open, setOpen] = useState(false)
  const [hi, setHi] = useState(0)
  const [picked, setPicked] = useState('')
  const center = view.settings.mapCenter
  useEffect(() => {
    const s = q.trim()
    if (s.length < 2 || q === picked) {
      setRes([])
      return
    }
    let live = true
    const t = setTimeout(() => {
      api.get<SearchResult[]>(`/map/search?q=${encodeURIComponent(s)}&lat=${center[0]}&lon=${center[1]}&limit=8`)
        .then((r) => {
          if (live) {
            setRes(r)
            setHi(0)
            setOpen(true)
          }
        })
        .catch(() => live && setRes([]))
    }, 120)
    return () => {
      live = false
      clearTimeout(t)
    }
  }, [q, center, picked])
  const choose = (r: SearchResult) => {
    onPick(r)
    setPicked(q)
    setOpen(false)
  }
  const icon: Record<string, string> = { address: '#', intersection: '✕', street: '═', place: '◉', poi: '★', area: '▢' }
  return (
    <div className="absolute right-12 top-2 z-10 w-80">
      <div className="flex items-center gap-1 rounded-md border border-slate-700 bg-slate-950/90 px-2 shadow-lg focus-within:border-sky-500">
        <Search size={12} className="shrink-0 text-slate-500" aria-hidden />
        <input
          className="h-7 w-full bg-transparent text-xs text-slate-100 outline-none placeholder:text-slate-500"
          placeholder="Address, intersection (A & B), street or place"
          aria-label="Search the map"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onFocus={() => res.length && setOpen(true)}
          onKeyDown={(e) => {
            if (e.key === 'ArrowDown') {
              e.preventDefault()
              setHi((h) => Math.min(h + 1, res.length - 1))
            } else if (e.key === 'ArrowUp') {
              e.preventDefault()
              setHi((h) => Math.max(h - 1, 0))
            } else if (e.key === 'Enter' && res[hi]) {
              e.preventDefault()
              choose(res[hi])
            } else if (e.key === 'Escape') {
              setOpen(false)
              ;(e.target as HTMLInputElement).blur()
            }
          }}
        />
        {(q || found) && <button className="text-[10px] text-slate-500 hover:text-slate-200" onClick={() => { setQ(''); setRes([]); onClear() }} aria-label="Clear the search">✕</button>}
      </div>
      {open && res.length > 0 && (
        <ul className="panel mt-1 max-h-80 overflow-y-auto rounded-md py-1 text-xs shadow-2xl" role="listbox" aria-label="Search results">
          {res.map((r, i) => (
            <li key={`${r.label}${r.lat}${r.lon}`}>
              <button
                type="button"
                role="option"
                aria-selected={i === hi}
                onMouseEnter={() => setHi(i)}
                onClick={() => choose(r)}
                className={cx('flex w-full items-start gap-2 px-2 py-1 text-left', i === hi && 'bg-slate-800')}
              >
                <span className={cx('mt-0.5 w-3 shrink-0 text-center font-mono text-[10px]', r.exact ? 'text-lime-300' : 'text-slate-500')}>{icon[r.kind] ?? '·'}</span>
                <span className="min-w-0">
                  <span className={cx('block truncate', r.exact ? 'font-semibold text-slate-50' : 'text-slate-200')}>{r.label}</span>
                  <span className="block truncate text-[10px] text-slate-500">{[r.source === 'county' ? (r.ranged ? 'county range: estimate' : 'county GIS') : r.source === 'overture' ? 'Overture' : '', r.detail, r.note].filter(Boolean).join(' · ')}</span>
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {found && !open && (
        <div className="panel mt-1 flex items-center gap-2 rounded-md px-2 py-1 text-[11px] shadow-xl">
          <span className="min-w-0 flex-1 truncate"><span className="font-semibold text-fuchsia-300">{found.label}</span> <span className="text-slate-500">{found.detail}</span></span>
          {onCreate && <button className="btn h-5 shrink-0 px-1.5 py-0 text-[10px]" onClick={() => onCreate(found)}><MapPinPlus size={10} aria-hidden /> New incident</button>}
        </div>
      )}
    </div>
  )
}
