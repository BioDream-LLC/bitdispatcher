import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Download, Image as ImageIcon, Square } from 'lucide-react'
import { api, errText } from '../api'
import { Field } from '../ui'
import { ago, cx, useNow } from '../util'
import type { PhotoInfo, PhotoPlan, View } from '../types'

const MILE = 1609.344

const DETAIL: { zoom: number; label: string }[] = [
  { zoom: 20, label: 'Zoom 20: 3-inch where the state has it' },
  { zoom: 19, label: 'Zoom 19: rooftops, driveways, cars' },
  { zoom: 18, label: 'Zoom 18: houses and yards' },
  { zoom: 17, label: 'Zoom 17: buildings and blocks' },
]

function size(b: number) {
  return b >= 1e9 ? `${(b / 1e9).toFixed(1)} GB` : `${Math.round(b / 1e6)} MB`
}
function dur(s: number) {
  if (s < 90) return `${Math.round(s)} s`
  if (s < 5400) return `${Math.round(s / 60)} min`
  return `${(s / 3600).toFixed(1)} h`
}

/** Aerial photos: recorded around the centre of the recorded map, in two levels of detail. */
export default function PhotosAdmin({ view }: { view: View }) {
  const [info, setInfo] = useState<PhotoInfo | null>(null)
  const area = view.health.map?.area
  const [core, setCore] = useState('10')
  const [coreZoom, setCoreZoom] = useState(19)
  const [outerZoom, setOuterZoom] = useState(16)
  const [rate, setRate] = useState('20')
  const [plan, setPlan] = useState<PhotoPlan | null>(null)
  const [planErr, setPlanErr] = useState('')
  const now = useNow()
  const job = info?.job
  const running = !!job && !job.finished
  const centre = area ? { lat: area.lat, lon: area.lon, radiusMiles: area.radiusM / MILE } : { lat: view.settings.mapCenter[0], lon: view.settings.mapCenter[1], radiusMiles: 25 }
  const req = { ...centre, coreRadiusMiles: Number(core), coreZoom, outerZoom, rate: Number(rate) }
  const key = JSON.stringify(req)

  const load = () => api.get<PhotoInfo>('/photos').then(setInfo).catch((e) => toast.error(errText(e)))
  useEffect(() => {
    load()
  }, [])
  useEffect(() => {
    if (!running) return
    const t = setInterval(load, 2000)
    return () => clearInterval(t)
  }, [running])
  // The estimate follows the form, so recording is one click.
  useEffect(() => {
    const t = setTimeout(() => {
      api.post<PhotoPlan>('/admin/photos/plan', req).then((p) => { setPlan(p); setPlanErr('') }).catch((e) => { setPlan(null); setPlanErr(errText(e)) })
    }, 300)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key])

  const record = async () => {
    try {
      await api.post('/admin/photos/record', req)
      toast.success('Recording photos: the nearest first')
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const cancel = () => api.post('/admin/photos/cancel').then(load).catch((e) => toast.error(errText(e)))
  const setOnline = async (on: boolean) => {
    try {
      await api.put('/admin/settings', { ...view.settings, photosRecordedOnly: !on })
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const st = info?.stats
  const frac = job && job.plan.tiles ? job.done / job.plan.tiles : 0
  return (
    <section className="panel mt-4 space-y-3 rounded-lg p-4">
      <h2 className="flex items-center gap-2 font-semibold text-slate-100"><ImageIcon size={16} aria-hidden /> Aerial photos</h2>
      <p className="text-xs text-slate-400">
        Dispatchers find places by rooftops. Photos are recorded around {area ? area.name : 'the console map centre'}: full detail near the centre, where
        most calls are, and less detail out to the edge. Consoles switch to photos with the Photos button or <kbd className="kbd">F7</kbd>; where nothing
        is recorded the server fetches the photo when it is asked for and keeps it{info && !info.online && ' (switched off below)'}.
      </p>
      <div className="grid gap-4 lg:grid-cols-2">
        <div className="space-y-2">
          {st && st.tiles > 0 ? (
            <div className="grid grid-cols-3 gap-2 text-center">
              {[['Photo tiles', st.tiles.toLocaleString()], ['Stored', size(st.bytes)], ['Most detail', `zoom ${st.maxZoom}`]].map(([k, v]) => (
                <div key={k} className="rounded border border-slate-800 bg-slate-950/50 px-2 py-1.5">
                  <div className="font-mono text-base font-bold text-slate-100">{v}</div>
                  <div className="text-[10px] uppercase tracking-wider text-slate-500">{k}</div>
                </div>
              ))}
            </div>
          ) : <p className="text-sm text-amber-300">No photos recorded yet.</p>}
          {info?.recorded && <p className="text-xs text-slate-400">Last finished recording: {Math.round(info.recorded.coreRadiusM / MILE)} mi at zoom {info.recorded.coreZoom}, {Math.round(info.recorded.radiusM / MILE)} mi at zoom {info.recorded.outerZoom}{info.recorded.date && <>, photos from {info.recorded.date}</>}.</p>}
          {job && (
            <div className={cx('rounded border p-2 text-xs', job.error ? 'border-rose-800 bg-rose-950/30' : running ? 'border-sky-800 bg-sky-950/30' : 'border-slate-800')}>
              <div className="flex items-center gap-2">
                <span className="font-semibold text-slate-200">{running ? `Recording zoom ${job.zoom}` : job.error ? 'Last recording stopped' : 'Last recording finished'}</span>
                <span className="text-slate-500">by {job.by}, started {ago(job.started, now)} ago</span>
                {running && <button className="btn ml-auto h-6 px-2 py-0 text-[11px]" onClick={cancel}><Square size={11} aria-hidden /> Stop</button>}
              </div>
              <div className="mt-1 h-1.5 overflow-hidden rounded bg-slate-800" role="progressbar" aria-valuenow={Math.round(frac * 100)} aria-valuemin={0} aria-valuemax={100}>
                <div className="h-full bg-amber-400 transition-all" style={{ width: `${Math.round(frac * 100)}%` }} />
              </div>
              <div className="mt-1 flex flex-wrap gap-x-3 font-mono text-[10px] text-slate-400">
                <span>{job.done.toLocaleString()} / {job.plan.tiles.toLocaleString()} tiles</span>
                <span>fetched {job.fetched.toLocaleString()} ({size(job.bytes)})</span>
                {job.missing > 0 && <span>no photo {job.missing.toLocaleString()}</span>}
                {job.failed > 0 && <span className="text-amber-300">failed {job.failed.toLocaleString()} (fetched next time)</span>}
              </div>
              {job.error && <div className="mt-1 text-rose-300">{job.error}. What was fetched is kept; record again to carry on.</div>}
            </div>
          )}
          <label className="flex items-center gap-2 text-xs text-slate-300">
            <input type="checkbox" checked={info?.online ?? true} onChange={(e) => setOnline(e.target.checked)} />
            Fetch photos that are not recorded when a console asks for them (and keep them)
          </label>
          <div className="space-y-1">
            {info?.sources.map((s) => (
              <div key={s.id} className="rounded border border-slate-800 p-2 text-[11px]">
                <div className="flex gap-2"><span className="font-semibold text-slate-200">{s.name}</span><span className="ml-auto text-slate-500">to zoom {s.maxZoom}</span></div>
                <div className="text-slate-400">{s.resolution}</div>
                <div className="text-slate-500">{s.terms}</div>
              </div>
            ))}
          </div>
        </div>
        <div className="space-y-2">
          <div className="grid grid-cols-2 gap-2">
            <Field label="Full detail within (miles)" htmlFor="pcore"><input id="pcore" type="number" min={0} max={100} className="input font-mono" value={core} onChange={(e) => setCore(e.target.value)} /></Field>
            <Field label="Full detail" htmlFor="pcz">
              <select id="pcz" className="input" value={coreZoom} onChange={(e) => setCoreZoom(Number(e.target.value))}>
                {DETAIL.map((d) => <option key={d.zoom} value={d.zoom}>{d.label}</option>)}
              </select>
            </Field>
            <Field label={`Out to ${Math.round(centre.radiusMiles)} miles`} htmlFor="poz">
              <select id="poz" className="input" value={outerZoom} onChange={(e) => setOuterZoom(Number(e.target.value))}>
                {[14, 15, 16, 17].map((z) => <option key={z} value={z}>Zoom {z}{z === 16 ? ': streets and houses' : z === 17 ? ': buildings (large)' : ''}</option>)}
              </select>
            </Field>
            <Field label="Requests per second" htmlFor="prate" hint="Kept low out of courtesy to the public services"><input id="prate" type="number" min={1} max={50} className="input font-mono" value={rate} onChange={(e) => setRate(e.target.value)} /></Field>
          </div>
          {plan ? (
            <div className="rounded border border-slate-800 p-2 text-xs">
              <div className="text-slate-300">
                About <b className="text-slate-100">{plan.tiles.toLocaleString()}</b> tiles, <b className="text-slate-100">{size(plan.bytes)}</b>, <b className="text-slate-100">{dur(plan.seconds)}</b> at {plan.rate}/s
                {plan.date && <> · photos at the centre: {plan.date}</>}. Tiles already recorded are skipped.
              </div>
              <div className="mt-1 flex flex-wrap gap-x-3 font-mono text-[10px] text-slate-500">
                {plan.levels.filter((l) => l.zoom >= 14).map((l) => <span key={l.zoom}>z{l.zoom} {Math.round(l.radiusM / MILE)} mi {l.tiles.toLocaleString()} · {l.metresPerPixel < 1 ? `${Math.round(l.metresPerPixel * 39.37)} in` : `${l.metresPerPixel.toFixed(1)} m`}/px</span>)}
              </div>
            </div>
          ) : planErr && <p className="text-xs text-rose-300">{planErr}</p>}
          <button className="btn btn-go" disabled={!plan || running} onClick={record}><Download size={14} aria-hidden /> Record photos</button>
        </div>
      </div>
    </section>
  )
}
