import 'maplibre-gl/dist/maplibre-gl.css'
import * as maplibregl from 'maplibre-gl'
import { useEffect, useMemo, useRef, useState } from 'react'
import { toast } from 'sonner'
import { MapPin, Pencil, Plus, Save, Trash2, Undo2 } from 'lucide-react'
import { api, errText } from '../api'
import { Field, Modal } from '../ui'
import { cx } from '../util'
import type { Geofence, View } from '../types'
import { dispatchStyle } from '../console/mapStyle'
import { Page } from './Admin'

const base = () => location.origin + location.pathname.replace(/[^/]*$/, '')

const DISCIPLINES: [string, string][] = [
  ['fire', 'Fire'],
  ['ems', 'EMS'],
  ['police', 'Police'],
]

const blank = (): Draft => ({ id: '', name: '', kind: 'alert', ring: [], note: '', enter: true, leave: false, disciplines: [], units: [], off: false, isNew: true })

type Draft = Geofence & { isNew: boolean }

// A ring the server stores is open ([lon, lat][]); a GeoJSON polygon must be closed. Close it for drawing.
const closed = (ring: [number, number][]): [number, number][] => (ring.length >= 3 ? [...ring, ring[0]] : ring)

const fenceFeature = (g: Geofence, editingId: string | null): GeoJSON.Feature => ({
  type: 'Feature',
  properties: { id: g.id, name: g.name, kind: g.kind, editing: g.id === editingId },
  geometry: { type: 'Polygon', coordinates: [closed(g.ring)] },
})

const toggle = (list: string[] | undefined, code: string) => {
  const l = list ?? []
  return l.includes(code) ? l.filter((x) => x !== code) : [...l, code]
}

/** Geofences: areas on the map. An alert area tells dispatch when a unit enters or leaves it; a
 * caution area puts its note on every incident inside it. Draw each area on the map. */
export default function Geofences({ view }: { view: View }) {
  const [items, setItems] = useState<Geofence[]>([])
  const [selected, setSelected] = useState<string | null>(null)
  const [d, setD] = useState<Draft | null>(null)
  const [drawing, setDrawing] = useState(false)
  const [corners, setCorners] = useState<[number, number][]>([])

  const box = useRef<HTMLDivElement>(null)
  const map = useRef<maplibregl.Map | null>(null)
  const ready = useRef(false)
  // Keep live values reachable from the map click handler without re-binding it.
  const drawingRef = useRef(drawing)
  drawingRef.current = drawing

  const load = async () => {
    try {
      const r = await api.get<{ items: Geofence[] }>('/admin/geofences')
      setItems(r.items ?? [])
    } catch (e) {
      toast.error(errText(e))
    }
  }
  useEffect(() => {
    load()
  }, [])

  const style = useMemo(
    () => dispatchStyle({ theme: 'night', base: base(), online: !!view.health.tiles, recorded: !!view.health.map?.recorded, attribution: '' }),
    [view.health.tiles, view.health.map?.recorded],
  )

  // Build the map once.
  useEffect(() => {
    if (!box.current) return
    const center: [number, number] = [view.settings.mapCenter[1], view.settings.mapCenter[0]]
    const mp = new maplibregl.Map({ container: box.current, style, center, zoom: view.settings.mapZoom, attributionControl: { compact: true }, dragRotate: false })
    map.current = mp
    mp.on('load', () => {
      ready.current = true
      mp.addSource('fences', { type: 'geojson', data: { type: 'FeatureCollection', features: [] } })
      mp.addLayer({
        id: 'fences-fill',
        type: 'fill',
        source: 'fences',
        paint: {
          'fill-color': ['case', ['get', 'editing'], '#22d3ee', ['==', ['get', 'kind'], 'caution'], '#fde047', '#f97316'],
          'fill-opacity': ['case', ['get', 'editing'], 0.35, 0.18],
        },
      })
      mp.addLayer({
        id: 'fences-line',
        type: 'line',
        source: 'fences',
        paint: {
          'line-color': ['case', ['get', 'editing'], '#22d3ee', ['==', ['get', 'kind'], 'caution'], '#fde047', '#f97316'],
          'line-width': ['case', ['get', 'editing'], 3, 1.5],
        },
      })
      // The area being drawn: a line and its corner points.
      mp.addSource('draw', { type: 'geojson', data: { type: 'FeatureCollection', features: [] } })
      mp.addLayer({ id: 'draw-line', type: 'line', source: 'draw', filter: ['==', ['geometry-type'], 'LineString'], paint: { 'line-color': '#22d3ee', 'line-width': 2, 'line-dasharray': [2, 1] } })
      mp.addLayer({ id: 'draw-point', type: 'circle', source: 'draw', filter: ['==', ['geometry-type'], 'Point'], paint: { 'circle-radius': 4, 'circle-color': '#22d3ee', 'circle-stroke-color': '#0b0f17', 'circle-stroke-width': 1 } })
    })
    mp.on('click', (e) => {
      if (!drawingRef.current) return
      setCorners((cs) => [...cs, [e.lngLat.lng, e.lngLat.lat]])
    })
    return () => {
      mp.remove()
      map.current = null
      ready.current = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [style])

  // Redraw every area whenever the list or the selection changes.
  useEffect(() => {
    const mp = map.current
    if (!mp || !ready.current) return
    const src = mp.getSource('fences') as maplibregl.GeoJSONSource | undefined
    if (!src) return
    const features = items.filter((g) => g.ring.length >= 3).map((g) => fenceFeature(g, d ? d.id : selected))
    src.setData({ type: 'FeatureCollection', features })
  }, [items, selected, d])

  // Redraw the corners being placed.
  useEffect(() => {
    const mp = map.current
    if (!mp || !ready.current) return
    const src = mp.getSource('draw') as maplibregl.GeoJSONSource | undefined
    if (!src) return
    const features: GeoJSON.Feature[] = corners.map((c, i) => ({ type: 'Feature', properties: { i }, geometry: { type: 'Point', coordinates: c } }))
    if (corners.length >= 2) features.push({ type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: corners } })
    src.setData({ type: 'FeatureCollection', features })
  }, [corners])

  const fit = (ring: [number, number][]) => {
    const mp = map.current
    if (!mp || ring.length < 2) return
    const b = new maplibregl.LngLatBounds(ring[0], ring[0])
    for (const c of ring) b.extend(c)
    mp.fitBounds(b, { padding: 60, maxZoom: 16, duration: 400 })
  }

  const startDraw = () => {
    setD(null)
    setSelected(null)
    setCorners([])
    setDrawing(true)
    toast.info('Click the map to add each corner, then Finish.')
  }
  const undoCorner = () => setCorners((cs) => cs.slice(0, -1))
  const cancelDraw = () => {
    setDrawing(false)
    setCorners([])
  }
  const finishDraw = () => {
    if (corners.length < 3) {
      toast.error('An area needs at least three corners.')
      return
    }
    const ring = corners.slice()
    setDrawing(false)
    setCorners([])
    // A fresh area, or the redraw of the one being edited.
    setD((cur) => (cur ? { ...cur, ring } : { ...blank(), ring }))
  }

  const selectFence = (g: Geofence) => {
    if (drawing) return
    setSelected(g.id)
    setD({ ...g, disciplines: [...(g.disciplines ?? [])], units: [...(g.units ?? [])], isNew: false })
    fit(g.ring)
  }

  const redraw = () => {
    // Keep the form open; draw mode will fill d.ring on Finish.
    setCorners([])
    setDrawing(true)
    toast.info('Click the map to place the new corners, then Finish.')
  }

  const save = async () => {
    if (!d) return
    if (d.ring.length < 3) {
      toast.error('Draw the area first.')
      return
    }
    if (d.kind === 'caution' && !d.note?.trim()) {
      toast.error('A caution area needs a note.')
      return
    }
    try {
      const { isNew, ...g } = d
      const body: Geofence = {
        id: g.id,
        name: g.name.trim(),
        kind: g.kind,
        ring: g.ring,
        note: g.note?.trim() || undefined,
        enter: g.kind === 'alert' ? !!g.enter : undefined,
        leave: g.kind === 'alert' ? !!g.leave : undefined,
        disciplines: g.disciplines?.length ? g.disciplines : undefined,
        units: g.units?.length ? g.units : undefined,
        off: g.off,
      }
      const r = await api.put<{ id: string }>('/admin/geofences', body)
      toast.success(`${g.name.trim() || 'Area'} saved`)
      setD(null)
      setSelected(r.id)
      await load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const remove = async (id: string) => {
    if (!confirm('Remove this area?')) return
    try {
      await api.del(`/admin/geofences/${encodeURIComponent(id)}`)
      setD(null)
      setSelected(null)
      await load()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const unitsText = (d?.units ?? []).join(', ')
  return (
    <Page
      title="Areas on the map (geofences)"
      intro="An alert area tells dispatch when a unit enters or leaves it; a caution area puts its note on every incident inside it. Draw each area on the map, then say what it is for."
      actions={<button className="btn btn-go" onClick={startDraw} disabled={drawing}><Plus size={14} aria-hidden /> Draw a new area</button>}
    >
      <div className="grid gap-4 lg:grid-cols-[2fr_3fr]">
        <div className="space-y-2">
          <div className="panel overflow-hidden rounded-lg">
            <ul className="divide-y divide-slate-800">
              {items.length === 0 && <li className="p-3 text-sm text-slate-500">No areas yet. Draw one on the map.</li>}
              {items.map((g) => (
                <li key={g.id}>
                  <button type="button" className={cx('flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-slate-800/50', selected === g.id && 'bg-slate-800/70')} onClick={() => selectFence(g)} disabled={drawing}>
                    <span className="min-w-0 flex-1 truncate text-slate-100">{g.name || g.id}</span>
                    <span className={cx('chip', g.kind === 'caution' ? 'bg-amber-500/20 text-amber-200' : 'bg-orange-500/20 text-orange-200')}>{g.kind === 'caution' ? 'Caution' : 'Alert'}</span>
                    {g.kind === 'alert' && <span className="text-xs text-slate-500">{[g.enter && 'enter', g.leave && 'leave'].filter(Boolean).join('/') || '—'}</span>}
                    {g.disciplines?.length ? <span className="text-xs text-slate-500">{g.disciplines.join(', ')}</span> : <span className="text-xs text-slate-500">all units</span>}
                    <span className="text-xs text-slate-500">{g.ring.length} corners</span>
                    {g.off && <span className="chip bg-slate-800 text-slate-400">off</span>}
                  </button>
                </li>
              ))}
            </ul>
          </div>
        </div>

        <div className="space-y-2">
          <div ref={box} className="h-[560px] overflow-hidden rounded-lg border border-slate-800" aria-label="Areas map" />
          {drawing ? (
            <div className="flex flex-wrap items-center gap-2 rounded-md border border-cyan-500/40 bg-cyan-500/10 p-2 text-sm text-cyan-100">
              <span>Drawing: {corners.length} corner{corners.length === 1 ? '' : 's'}.</span>
              <button className="btn ml-auto" onClick={undoCorner} disabled={corners.length === 0}><Undo2 size={13} aria-hidden /> Undo corner</button>
              <button className="btn btn-go" onClick={finishDraw} disabled={corners.length < 3}>Finish</button>
              <button className="btn" onClick={cancelDraw}>Cancel</button>
            </div>
          ) : (
            <p className="text-xs text-slate-500">Alert areas are orange, caution areas yellow, the one being edited is cyan. Pick an area on the left to edit it, or draw a new one.</p>
          )}
        </div>
      </div>

      {d && !drawing && (
        <Modal
          open
          onClose={() => { setD(null); setSelected(null) }}
          title={d.isNew ? 'New area' : d.name || d.id}
          icon={<MapPin size={16} className="text-cyan-300" aria-hidden />}
          footer={
            <>
              {!d.isNew && <button className="btn mr-auto text-rose-300" onClick={() => remove(d.id)}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => { setD(null); setSelected(null) }}>Cancel</button>
              <button className="btn btn-go" onClick={save} disabled={!d.name.trim()}><Save size={13} aria-hidden /> Save</button>
            </>
          }
        >
          <div className="space-y-4">
            <div className="flex items-center justify-between gap-2 rounded border border-slate-800 p-2 text-sm text-slate-300">
              <span>{d.ring.length} corners drawn.</span>
              <button className="btn" onClick={redraw}><Pencil size={13} aria-hidden /> Redraw</button>
            </div>
            <Field label="Name" htmlFor="gf-name"><input id="gf-name" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} placeholder="Downtown" /></Field>
            <fieldset>
              <legend className="label">Kind</legend>
              <div className="flex gap-4 text-sm text-slate-300">
                <label className="flex items-center gap-2"><input type="radio" name="gf-kind" checked={d.kind === 'alert'} onChange={() => setD({ ...d, kind: 'alert' })} /> Alert — tell dispatch on enter or leave</label>
                <label className="flex items-center gap-2"><input type="radio" name="gf-kind" checked={d.kind === 'caution'} onChange={() => setD({ ...d, kind: 'caution' })} /> Caution — put a note on every incident inside</label>
              </div>
            </fieldset>
            <Field label={d.kind === 'caution' ? 'Note (required)' : 'Note'} htmlFor="gf-note" hint={d.kind === 'caution' ? 'Shown on every incident inside this area' : 'Optional'}>
              <input id="gf-note" className="input" value={d.note ?? ''} onChange={(e) => setD({ ...d, note: e.target.value })} />
            </Field>
            {d.kind === 'alert' && (
              <fieldset>
                <legend className="label">Alert when units</legend>
                <div className="flex gap-4 text-sm text-slate-300">
                  <label className="flex items-center gap-2"><input type="checkbox" checked={!!d.enter} onChange={(e) => setD({ ...d, enter: e.target.checked })} /> enter</label>
                  <label className="flex items-center gap-2"><input type="checkbox" checked={!!d.leave} onChange={(e) => setD({ ...d, leave: e.target.checked })} /> leave</label>
                </div>
              </fieldset>
            )}
            <fieldset>
              <legend className="label">Disciplines (none = all units)</legend>
              <div className="flex flex-wrap gap-1">
                {DISCIPLINES.map(([code, label]) => (
                  <button key={code} type="button" aria-pressed={(d.disciplines ?? []).includes(code)} className={cx('chip', (d.disciplines ?? []).includes(code) ? 'bg-sky-600 text-white' : 'bg-slate-800 text-slate-300')} onClick={() => setD({ ...d, disciplines: toggle(d.disciplines, code) })}>{label}</button>
                ))}
              </div>
            </fieldset>
            <Field label="Specific units (optional)" htmlFor="gf-units" hint="Comma-separated unit IDs; leave empty for all units in the disciplines above">
              <input id="gf-units" className="input font-mono" value={unitsText} onChange={(e) => setD({ ...d, units: e.target.value.split(',').map((x) => x.trim()).filter(Boolean) })} placeholder="E1, M2" />
            </Field>
            <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!d.off} onChange={(e) => setD({ ...d, off: e.target.checked })} /> Turned off</label>
          </div>
        </Modal>
      )}
    </Page>
  )
}
