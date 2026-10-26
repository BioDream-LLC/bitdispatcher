import { useEffect, useRef, useState } from 'react'
import { AlertTriangle, ArrowLeft, Eye, Map as MapIcon, RefreshCw, Satellite } from 'lucide-react'
import { api, errText } from '../api'

type SV = {
  lat: number; lon: number; available: boolean | null; why?: string; date?: string; embed?: string; panoLat?: number; panoLon?: number
  link: boolean; linkUrl?: string; mapLink: string
}

/** Street View docked beside the call: built from the incident's current location (address and
 * coordinates shown with it), checked with Google first so a place without imagery says so, and never a
 * blank page: a timeout, an error or no imagery stops loading and offers Retry, the map, the aerial photos
 * and Return to call. Opening or closing it never touches the call. */
export default function StreetViewDock({ lat, lon, address, onReturn, onAerial }: { lat?: number; lon?: number; address?: string; onReturn: () => void; onAerial: () => void }) {
  const [sv, setSv] = useState<SV | null>(null)
  const [err, setErr] = useState('')
  const [loaded, setLoaded] = useState(false)
  const [timedOut, setTimedOut] = useState(false)
  const [n, setN] = useState(0)
  const was = useRef<string>('')
  const key = lat != null && lon != null ? `${lat.toFixed(6)},${lon.toFixed(6)}` : ''
  const moved = was.current !== '' && key !== '' && was.current !== key
  useEffect(() => {
    if (!key) return
    let live = true
    setSv(null); setErr(''); setLoaded(false); setTimedOut(false)
    api.get<SV>(`/views/streetview?lat=${lat}&lon=${lon}`).then((r) => { if (live) setSv(r) }).catch((e) => { if (live) setErr(errText(e)) })
    const t = window.setTimeout(() => { if (live) setTimedOut(true) }, 10000)
    return () => { live = false; window.clearTimeout(t) }
  }, [key, n]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const t = window.setTimeout(() => { was.current = key }, 4000)
    return () => window.clearTimeout(t)
  }, [key])
  if (!key) return <p className="text-xs text-slate-500">Street View appears once the incident has a map position.</p>
  const failed = err || (sv && sv.available !== true) || (sv?.embed && timedOut && !loaded)
  const why = err || (sv?.available === false ? sv.why : sv?.available == null ? sv?.why : timedOut && !loaded ? 'Street View did not load within 10 seconds (the network or the browser may block it)' : '')
  return (
    <section className="space-y-1.5" aria-label="Street View">
      <div className="flex flex-wrap items-center gap-2 text-[11px] text-slate-300">
        <Eye size={12} aria-hidden /> <b>Street View</b>
        <span className="truncate">{address || 'no address'} · <span className="font-mono">{key}</span></span>
        {sv?.date && <span className="text-slate-400">imagery {sv.date}</span>}
        {sv?.panoLat != null && sv.panoLon != null && <span className="text-slate-500">camera {Math.round(dist(lat!, lon!, sv.panoLat, sv.panoLon))} m from the point</span>}
        <button type="button" className="btn btn-ghost ml-auto h-6 px-1.5 text-[11px]" onClick={onReturn}><ArrowLeft size={11} aria-hidden /> Return to call</button>
      </div>
      {moved && <p className="rounded border border-amber-700 bg-amber-950/40 px-2 py-0.5 text-[11px] text-amber-100" role="status">The location changed: this is the new place, not the earlier one.</p>}
      {!sv && !err && !timedOut && <div className="h-56 animate-pulse rounded-md bg-slate-800" aria-label="Checking Street View" />}
      {sv?.available && sv.embed && !(timedOut && !loaded) && (
        <iframe key={sv.embed + n} title={`Street View at ${address || key}`} src={sv.embed} className="h-64 w-full rounded-md border border-slate-800" loading="lazy" referrerPolicy="no-referrer-when-downgrade"
          allowFullScreen onLoad={() => setLoaded(true)} />
      )}
      {failed && (
        <div className="rounded-md border border-slate-700 bg-slate-900 p-2 text-xs text-slate-200" role="status">
          <p className="flex items-start gap-1.5"><AlertTriangle size={13} className="mt-0.5 shrink-0 text-amber-300" aria-hidden /> {sv?.available === false ? 'Street View is not available here.' : 'Street View could not be shown.'} {why}</p>
          <div className="mt-1.5 flex flex-wrap gap-1.5">
            <button type="button" className="btn h-6 px-2 text-[11px]" onClick={() => setN((x) => x + 1)}><RefreshCw size={11} aria-hidden /> Retry</button>
            <button type="button" className="btn h-6 px-2 text-[11px]" onClick={onAerial}><Satellite size={11} aria-hidden /> Aerial photos on the map</button>
            {sv?.mapLink && <a className="btn h-6 px-2 text-[11px]" href={sv.mapLink} target="bd-map" rel="noreferrer noopener"><MapIcon size={11} aria-hidden /> Open the map (another window)</a>}
            <button type="button" className="btn h-6 px-2 text-[11px]" onClick={onReturn}><ArrowLeft size={11} aria-hidden /> Return to call</button>
          </div>
        </div>
      )}
    </section>
  )
}

function dist(a: number, b: number, c: number, d: number) {
  const r = 6371000, p = Math.PI / 180
  const x = Math.sin(((c - a) * p) / 2) ** 2 + Math.cos(a * p) * Math.cos(c * p) * Math.sin(((d - b) * p) / 2) ** 2
  return 2 * r * Math.atan2(Math.sqrt(x), Math.sqrt(1 - x))
}
