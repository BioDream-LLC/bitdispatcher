import 'maplibre-gl/dist/maplibre-gl.css'
import * as maplibregl from 'maplibre-gl'
import { useEffect, useMemo, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Pause, Play, Rewind } from 'lucide-react'
import { api, errText } from '../api'
import { Modal } from '../ui'
import type { Entry, Position, View } from '../types'
import { dispatchStyle } from './mapStyle'

type Replay = { incident: string; start: string; end: string; lat?: number; lon?: number; timeline: Entry[]; tracks: { unit: string; points: Position[] }[] }

const base = () => location.origin + location.pathname.replace(/[^/]*$/, '')
const SPEEDS = [10, 30, 60, 120]

// where is a unit's place at time t: between its two recorded points, or at the nearest end.
function where(ps: Position[], t: number): [number, number] | null {
  if (!ps.length) return null
  const ts = ps.map((p) => Date.parse(p.at))
  if (t <= ts[0]) return null // not yet recorded
  for (let i = 1; i < ps.length; i++) {
    if (t <= ts[i]) {
      const f = (t - ts[i - 1]) / Math.max(1, ts[i] - ts[i - 1])
      return [ps[i - 1].lon + (ps[i].lon - ps[i - 1].lon) * f, ps[i - 1].lat + (ps[i].lat - ps[i - 1].lat) * f]
    }
  }
  return [ps[ps.length - 1].lon, ps[ps.length - 1].lat]
}

/** ReplayDialog plays an incident back from the ledger: units move on the map along their recorded
 * positions while the timeline fills in, at 10× to 120×, with a slider to go to any moment. */
export default function ReplayDialog({ number, view, onClose }: { number: string; view: View; onClose: () => void }) {
  const [rp, setRp] = useState<Replay | null>(null)
  const [t, setT] = useState(0)
  const [playing, setPlaying] = useState(false)
  const [speed, setSpeed] = useState(30)
  const box = useRef<HTMLDivElement>(null)
  const map = useRef<maplibregl.Map | null>(null)
  const marks = useRef<Record<string, maplibregl.Marker>>({})
  const close = useRef(onClose)
  close.current = onClose
  // Read once per incident: the console re-renders on every change, which must not restart the replay.
  useEffect(() => {
    api.get<Replay>(`/incidents/${encodeURIComponent(number)}/replay`).then((r) => { setRp(r); setT(Date.parse(r.start)) }).catch((e) => { toast.error(errText(e)); close.current() })
  }, [number])
  const start = rp ? Date.parse(rp.start) : 0
  const end = rp ? Date.parse(rp.end) : 0
  const style = useMemo(() => dispatchStyle({ theme: 'night', base: base(), online: !!view.health.tiles, recorded: !!view.health.map?.recorded, attribution: '' }), [view.health.tiles, view.health.map?.recorded])
  useEffect(() => {
    if (!rp || !box.current) return
    const c: [number, number] = rp.lat != null && rp.lon != null ? [rp.lon, rp.lat] : [view.settings.mapCenter[1], view.settings.mapCenter[0]]
    const mp = new maplibregl.Map({ container: box.current, style, center: c, zoom: 13, attributionControl: { compact: true }, dragRotate: false })
    map.current = mp
    if (rp.lat != null && rp.lon != null) {
      const el = document.createElement('div')
      el.className = 'h-4 w-4 rounded-full border-2 border-white bg-rose-600'
      el.title = number
      new maplibregl.Marker({ element: el }).setLngLat(c).addTo(mp)
    }
    const b = new maplibregl.LngLatBounds(c, c)
    for (const tr of rp.tracks) for (const p of tr.points) b.extend([p.lon, p.lat])
    if (rp.tracks.length) mp.fitBounds(b, { padding: 50, maxZoom: 15, duration: 0 })
    return () => { mp.remove(); map.current = null; marks.current = {} }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rp, style, number])
  // Units at time t.
  useEffect(() => {
    const mp = map.current
    if (!mp || !rp) return
    for (const tr of rp.tracks) {
      const at = where(tr.points, t)
      let m = marks.current[tr.unit]
      if (!at) { m?.remove(); delete marks.current[tr.unit]; continue }
      if (!m) {
        const el = document.createElement('div')
        el.className = 'rounded bg-sky-600 px-1 font-mono text-[11px] font-bold text-white shadow'
        el.textContent = tr.unit
        m = new maplibregl.Marker({ element: el }).setLngLat(at).addTo(mp)
        marks.current[tr.unit] = m
      } else m.setLngLat(at)
    }
  }, [t, rp])
  useEffect(() => {
    if (!playing) return
    const id = setInterval(() => setT((x) => { const n = x + 250 * speed; if (n >= end) { setPlaying(false); return end } return n }), 250)
    return () => clearInterval(id)
  }, [playing, speed, end])
  const shown = (rp?.timeline ?? []).filter((e) => Date.parse(e.at) <= t)
  const clock = (ms: number) => new Date(ms).toLocaleTimeString()
  return (
    <Modal open onClose={onClose} xl title={`Replay ${number}`} icon={<Play size={16} aria-hidden />}>
      {!rp ? <p className="text-sm text-slate-400">Reading the ledger…</p> : (
        <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_22rem]">
          <div className="space-y-2">
            <div ref={box} className="h-[52vh] rounded border border-slate-800" aria-label="Replay map" role="img" />
            <div className="flex items-center gap-2">
              <button className="btn h-8" onClick={() => { setT(start); setPlaying(false) }} aria-label="To the start"><Rewind size={14} aria-hidden /></button>
              <button className="btn btn-go h-8 w-20" onClick={() => { if (t >= end) setT(start); setPlaying(!playing) }}>{playing ? <><Pause size={14} aria-hidden /> Pause</> : <><Play size={14} aria-hidden /> Play</>}</button>
              <input type="range" className="flex-1" min={start} max={end} step={1000} value={t} onChange={(e) => setT(Number(e.target.value))} aria-label="Time" />
              <span className="w-24 text-right font-mono text-sm text-slate-200">{clock(t)}</span>
              <select className="input h-8 w-auto text-xs" value={speed} onChange={(e) => setSpeed(Number(e.target.value))} aria-label="Speed">{SPEEDS.map((x) => <option key={x} value={x}>{x}×</option>)}</select>
            </div>
            {!rp.tracks.length && <p className="text-xs text-slate-500">No unit positions were recorded for this incident's units in its time: the timeline plays alone.</p>}
          </div>
          <ol className="max-h-[60vh] space-y-1 overflow-y-auto text-xs" aria-label="Timeline so far">
            {shown.map((e) => (
              <li key={e.seq} className="rounded border border-slate-800 px-2 py-1"><span className="font-mono text-slate-400">{new Date(e.at).toLocaleTimeString()}</span> <span className="text-slate-500">{e.actor}</span><div className="text-slate-100">{e.text}</div></li>
            ))}
          </ol>
        </div>
      )}
    </Modal>
  )
}
