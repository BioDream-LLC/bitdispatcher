import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Ban, Car } from 'lucide-react'
import { api, errText } from '../api'
import { Field, Modal } from '../ui'
import type { RoadClosure, RoadsView, View } from '../types'

/** useRoads is the road closures and traffic with their shapes, fetched again whenever they change. */
export function useRoads(view: View): RoadsView | null {
  const version = view.next?.roads.version ?? 0
  const count = view.next?.roads.closures.length ?? 0
  const at = view.next?.roads.traffic.at ?? ''
  const [roads, setRoads] = useState<RoadsView | null>(null)
  useEffect(() => {
    let live = true
    api.get<RoadsView>('/roads').then((r) => live && setRoads(r)).catch(() => {})
    return () => {
      live = false
    }
  }, [version, count, at])
  return roads
}

function circleRing(lat: number, lon: number, r: number): [number, number][] {
  const out: [number, number][] = []
  for (let i = 0; i <= 48; i++) {
    const a = (i / 48) * 2 * Math.PI
    out.push([lon + (r / (111320 * Math.cos((lat * Math.PI) / 180))) * Math.sin(a), lat + (r / 110540) * Math.cos(a)])
  }
  return out
}

/** closureFeatures are the closed roads as the map draws them: the road segments taken (red), the
 * drawn line or circle (faint) where the map has no roads for it. */
export function closureFeatures(roads: RoadsView | null): GeoJSON.FeatureCollection {
  const f: GeoJSON.Feature[] = []
  for (const c of roads?.closures ?? []) {
    const props = { id: c.id, reason: c.reason }
    for (const s of c.shapes ?? []) f.push({ type: 'Feature', properties: { ...props, kind: 'road' }, geometry: { type: 'LineString', coordinates: s } })
    if (c.center && c.radius) f.push({ type: 'Feature', properties: { ...props, kind: 'area' }, geometry: { type: 'Polygon', coordinates: [circleRing(c.center[0], c.center[1], c.radius)] } })
    else if (c.line && c.line.length > 1 && !(c.shapes ?? []).length) f.push({ type: 'Feature', properties: { ...props, kind: 'drawn' }, geometry: { type: 'LineString', coordinates: c.line } })
  }
  return { type: 'FeatureCollection', features: f }
}

/** trafficFeatures are the slowdowns: orange when twice as slow, red at four times, dark red for a standstill. */
export function trafficFeatures(roads: RoadsView | null): GeoJSON.FeatureCollection {
  return {
    type: 'FeatureCollection',
    features: (roads?.traffic.jams ?? []).map((j) => ({
      type: 'Feature', properties: { factor: j.closed ? 99 : j.factor || 1, street: j.street ?? '', kmh: j.kmh }, geometry: { type: 'LineString', coordinates: j.line },
    })),
  }
}

/** drawnFeatures is the closure being drawn. */
export function drawnFeatures(points: [number, number][], radius?: number): GeoJSON.FeatureCollection {
  const f: GeoJSON.Feature[] = points.map((p) => ({ type: 'Feature', properties: {}, geometry: { type: 'Point', coordinates: p } }))
  if (radius && points.length) f.push({ type: 'Feature', properties: {}, geometry: { type: 'Polygon', coordinates: [circleRing(points[0][1], points[0][0], radius)] } })
  else if (points.length > 1) f.push({ type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: points } })
  return { type: 'FeatureCollection', features: f }
}

const local = (d: Date) => new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16)

/** CloseRoad asks why a drawn road is closed, until when, and the detour, then closes it. */
export function CloseRoad({ draft, incident, onClose }: { draft: { line?: [number, number][]; center?: [number, number]; radius?: number } | null; incident?: string; onClose: (done: boolean) => void }) {
  const [reason, setReason] = useState('')
  const [until, setUntil] = useState('')
  const [detour, setDetour] = useState('')
  const [radius, setRadius] = useState(draft?.radius ?? 60)
  const [onInc, setOnInc] = useState(!!incident)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    setReason('')
    setUntil('')
    setDetour('')
    setRadius(draft?.radius ?? 60)
    setOnInc(!!incident)
  }, [draft, incident])
  const save = async () => {
    if (!draft) return
    setBusy(true)
    try {
      const body: Partial<RoadClosure> = { reason, detour, incident: onInc ? incident : undefined, until: until ? new Date(until).toISOString() : undefined }
      if (draft.center) Object.assign(body, { center: draft.center, radius })
      else body.line = draft.line
      const r = await api.put<{ segments: number; roads: string[] }>('/roads/closures', body)
      toast.success(r.segments ? `Closed: ${r.roads.join(', ') || `${r.segments} road segments`}. Drive times now go around it.` : 'Closed, but the recorded map has no road there: draw it along the road')
      onClose(true)
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal open={!!draft} onClose={() => onClose(false)} title={draft?.center ? 'Close the roads in a circle' : 'Close this stretch of road'} icon={<Ban size={16} className="text-rose-400" aria-hidden />}
      footer={<><button className="btn" onClick={() => onClose(false)}>Cancel</button><button className="btn btn-go" disabled={busy || !reason.trim()} onClick={save}>Close the road</button></>}>
      <div className="space-y-3 text-sm">
        <p className="text-xs text-slate-400">While it is in force, drive times and unit recommendations go around it, the map draws it in red, and crews see it on the mobile terminal. Lift it from the map or Admin › Road closures.</p>
        <Field label="Why it is closed" htmlFor="rc-reason"><input id="rc-reason" className="input" autoFocus value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Crash, all lanes blocked" maxLength={200} /></Field>
        {draft?.center && (
          <Field label="Radius (metres)" htmlFor="rc-radius" hint="Every road with any part inside the circle is closed."><input id="rc-radius" type="number" min={5} max={3000} className="input w-32" value={radius} onChange={(e) => setRadius(Number(e.target.value))} /></Field>
        )}
        <Field label="Until (empty: until someone lifts it)" htmlFor="rc-until"><input id="rc-until" type="datetime-local" className="input w-60" value={until} min={local(new Date())} onChange={(e) => setUntil(e.target.value)} /></Field>
        <Field label="Detour (optional)" htmlFor="rc-detour"><input id="rc-detour" className="input" value={detour} onChange={(e) => setDetour(e.target.value)} placeholder="Use Orange Ave to 10th St" maxLength={400} /></Field>
        {incident && <label className="flex items-center gap-2 text-xs text-slate-300"><input type="checkbox" checked={onInc} onChange={(e) => setOnInc(e.target.checked)} /> Note it on {incident}</label>}
      </div>
    </Modal>
  )
}

/** ClosureCard is a closure clicked on the map: what, since when, and Lift. */
export function ClosureCard({ c, canLift, onDone }: { c: RoadClosure; canLift: boolean; onDone: () => void }) {
  const lift = async () => {
    try {
      await api.post(`/roads/closures/${c.id}/lift`)
      toast.success('Road open again: drive times use it')
      onDone()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <div className="space-y-1 text-xs">
      <div className="flex items-center gap-1 font-semibold text-rose-300"><Ban size={12} aria-hidden /> Road closed · {c.reason}</div>
      {(c.roads ?? []).length > 0 && <div className="text-slate-300">{c.roads!.join(', ')}</div>}
      {c.detour && <div className="text-slate-300">Detour: {c.detour}</div>}
      <div className="text-[10px] text-slate-500">by {c.by} · {new Date(c.at).toLocaleString()}{c.until ? ` · until ${new Date(c.until).toLocaleString()}` : ' · until lifted'}{c.incident ? ` · ${c.incident}` : ''}</div>
      {canLift && <button className="btn h-6 px-2 py-0 text-[11px]" onClick={lift}>Lift: open the road</button>}
    </div>
  )
}

/** TrafficNote says where drive times come from when traffic or closures change them. */
export function TrafficNote({ view }: { view: View }) {
  const r = view.next?.roads
  if (!r || (!r.closures.length && !r.traffic.on)) return null
  return (
    <span className="flex items-center gap-1 text-[10px] text-slate-400" title={r.traffic.error ? `Traffic feed: ${r.traffic.error}` : undefined}>
      {r.closures.length > 0 && <span className="chip bg-rose-500/20 text-rose-200"><Ban size={10} aria-hidden /> {r.closures.length} closed</span>}
      {r.traffic.on && (
        <span className={r.traffic.error ? 'chip bg-amber-500/20 text-amber-200' : 'chip bg-orange-500/20 text-orange-200'}>
          <Car size={10} aria-hidden /> {r.traffic.error ? 'traffic feed failing' : `traffic · ${r.jams ?? 0}`}
        </span>
      )}
    </span>
  )
}
