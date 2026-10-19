import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Download, Map as MapIcon, RefreshCw, Square } from 'lucide-react'
import { api, errText } from '../api'
import { Field } from '../ui'
import { ago, cx, useNow } from '../util'
import type { MapExtract, MapInfo, View } from '../types'
import { Page } from './Admin'
import PhotosAdmin from './PhotosAdmin'

const MILE = 1609.344

const PRESETS = [{ name: 'Roanoke, Virginia (development map)', lat: 37.271, lon: -79.9414 }]

function mb(b?: number) {
  return b ? `${(b / 1e6).toFixed(0)} MB` : 'size unknown'
}

export default function MapAdmin({ view }: { view: View }) {
  const [info, setInfo] = useState<MapInfo | null>(null)
  const [name, setName] = useState('')
  const [lat, setLat] = useState('')
  const [lon, setLon] = useState('')
  const [radius, setRadius] = useState('100')
  const [plan, setPlan] = useState<MapExtract[] | null>(null)
  const [busy, setBusy] = useState(false)
  const now = useNow()
  const running = !!info?.job && !info.job.finished

  const load = () => api.get<MapInfo>('/map').then(setInfo).catch((e) => toast.error(errText(e)))
  useEffect(() => {
    load()
  }, [])
  // While recording, follow its progress.
  useEffect(() => {
    if (!running) return
    const t = setInterval(load, 1000)
    return () => clearInterval(t)
  }, [running])
  // Start the form from the recorded area, or the console's map centre.
  useEffect(() => {
    if (!info || name || lat) return
    if (info.area) {
      setName(info.area.name)
      setLat(String(info.area.lat))
      setLon(String(info.area.lon))
      setRadius(String(Math.round(info.area.radiusM / MILE)))
    } else {
      setLat(String(view.settings.mapCenter[0]))
      setLon(String(view.settings.mapCenter[1]))
    }
  }, [info, view.settings.mapCenter, name, lat])

  const area = () => ({ name, lat: Number(lat), lon: Number(lon), radiusMiles: Number(radius) })
  // The download list follows the form, so recording is one click.
  useEffect(() => {
    if (!lat || !lon || !radius) return
    const t = setTimeout(check, 500)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lat, lon, radius])
  const check = async () => {
    setBusy(true)
    setPlan(null)
    try {
      const r = await api.post<{ extracts: MapExtract[] }>('/admin/map/plan', area())
      setPlan(r.extracts)
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  const record = async () => {
    if (!plan) return
    setBusy(true)
    try {
      await api.post('/admin/map/record', { ...area(), extracts: plan })
      toast.success('Recording started')
      setPlan(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  const cancel = async () => {
    try {
      await api.post('/admin/map/cancel')
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const job = info?.job
  const st = info?.stats
  const total = plan?.reduce((n, e) => n + (e.bytes ?? 0), 0) ?? 0
  return (
    <Page
      title="Map"
      intro="bitdispatcher records the OpenStreetMap data for a circle around your centre into its own store, then draws the dispatch map, routes units by road and finds addresses from that store, with no internet needed. Record again whenever you want newer roads and addresses; the map in use stays until the new one is complete."
    >
      <div className="grid gap-4 lg:grid-cols-2">
        <section className="panel space-y-3 rounded-lg p-4">
          <h2 className="flex items-center gap-2 font-semibold text-slate-100"><MapIcon size={16} aria-hidden /> Recorded map</h2>
          {!info ? <p className="text-sm text-slate-500">Loading…</p> : !info.recorded ? (
            <p className="text-sm text-amber-300">No map is recorded yet. Consoles show the online map only, and drive times are straight-line estimates.</p>
          ) : (
            <>
              <dl className="grid grid-cols-[9rem_1fr] gap-x-3 gap-y-1 text-sm">
                <dt className="text-slate-500">Area</dt><dd className="text-slate-100">{info.area?.name}</dd>
                <dt className="text-slate-500">Centre</dt><dd className="font-mono text-slate-300">{info.area?.lat.toFixed(4)}, {info.area?.lon.toFixed(4)}</dd>
                <dt className="text-slate-500">Radius</dt><dd className="text-slate-300">{Math.round((info.area?.radiusM ?? 0) / MILE)} miles</dd>
                <dt className="text-slate-500">Recorded</dt><dd className="text-slate-300">{info.built && new Date(info.built).toLocaleString()} <span className="text-slate-500">({info.built && ago(info.built, now)} ago)</span></dd>
                <dt className="text-slate-500">Source</dt><dd className="break-all text-xs text-slate-400">{info.source}</dd>
              </dl>
              {st && (
                <div className="grid grid-cols-3 gap-2 text-center">
                  {[
                    ['Road miles', Math.round(st.roadKm / 1.609344).toLocaleString()],
                    ['Road segments', st.roadSegments.toLocaleString()],
                    ['Intersections', st.intersections.toLocaleString()],
                    ['Addresses', st.addresses.toLocaleString()],
                    ['Buildings', (st.byLayer.building ?? 0).toLocaleString()],
                    ['Landmarks & hydrants', (st.byLayer.poi ?? 0).toLocaleString()],
                  ].map(([k, v]) => (
                    <div key={k} className="rounded border border-slate-800 bg-slate-950/50 px-2 py-1.5">
                      <div className="font-mono text-base font-bold text-slate-100">{v}</div>
                      <div className="text-[10px] uppercase tracking-wider text-slate-500">{k}</div>
                    </div>
                  ))}
                </div>
              )}
              <div className="flex items-center gap-2">
                <button className="btn text-xs" disabled={running} onClick={async () => { try { await api.post('/admin/map/update'); load() } catch (e) { toast.error(errText(e)) } }}>
                  <RefreshCw size={12} aria-hidden /> Bring up to date
                </button>
                <span className="text-[11px] text-slate-500">Applies OpenStreetMap's daily change files since the recording: megabytes, not the whole extracts.</span>
              </div>
              <p className="text-[11px] text-slate-500">Map data © OpenStreetMap contributors, under the Open Database License. Recorded in {st?.seconds ?? '?'} s from {st?.nodes.toLocaleString()} map points.</p>
            </>
          )}
          {job && (
            <div className={cx('rounded border p-2 text-xs', job.error ? 'border-rose-800 bg-rose-950/30' : running ? 'border-sky-800 bg-sky-950/30' : 'border-slate-800')}>
              <div className="flex items-center gap-2">
                <span className="font-semibold text-slate-200">{running ? (job.update ? 'Updating' : 'Recording') : job.error ? 'Last recording failed' : job.update ? `Last update${job.changes ? `: ${job.changes.toLocaleString()} changes` : ''}` : 'Last recording'}: {job.area.name}</span>
                {!running && !job.error && job.progress.phase === 'Already up to date' && <span className="text-emerald-300">already up to date</span>}
                <span className="text-slate-500">by {job.by}, started {ago(job.started, now)} ago</span>
                {running && <button className="btn ml-auto h-6 px-2 py-0 text-[11px]" onClick={cancel}><Square size={11} aria-hidden /> Cancel</button>}
              </div>
              {running && (
                <>
                  <div className="mt-1 text-slate-300">{job.progress.phase} <span className="text-slate-500">{job.progress.detail}</span></div>
                  <div className="mt-1 h-1.5 overflow-hidden rounded bg-slate-800" role="progressbar" aria-valuenow={Math.round(job.progress.fraction * 100)} aria-valuemin={0} aria-valuemax={100}>
                    <div className="h-full bg-sky-400 transition-all" style={{ width: `${Math.round(job.progress.fraction * 100)}%` }} />
                  </div>
                  {job.progress.counts && (
                    <div className="mt-1 flex flex-wrap gap-x-3 font-mono text-[10px] text-slate-400">
                      {Object.entries(job.progress.counts).sort().map(([k, v]) => <span key={k}>{k} {v.toLocaleString()}</span>)}
                    </div>
                  )}
                </>
              )}
              {job.error && <div className="mt-1 text-rose-300">{job.error}</div>}
            </div>
          )}
        </section>
        <section className="panel space-y-3 rounded-lg p-4">
          <h2 className="flex items-center gap-2 font-semibold text-slate-100"><RefreshCw size={16} aria-hidden /> {info?.recorded ? 'Record again or record a new area' : 'Record an area'}</h2>
          <div className="flex flex-wrap gap-1">
            {PRESETS.map((p) => (
              <button key={p.name} className="btn text-xs" onClick={() => { setName(p.name); setLat(String(p.lat)); setLon(String(p.lon)); setPlan(null) }}>{p.name}</button>
            ))}
            <button className="btn text-xs" onClick={() => { setLat(String(view.settings.mapCenter[0])); setLon(String(view.settings.mapCenter[1])); setPlan(null) }}>Console map centre</button>
          </div>
          <Field label="Name" htmlFor="mn" hint="Shown on the map and in the ledger"><input id="mn" className="input" value={name} onChange={(e) => setName(e.target.value)} placeholder="Roanoke Valley" /></Field>
          <div className="grid grid-cols-3 gap-2">
            <Field label="Centre latitude" htmlFor="mlat"><input id="mlat" className="input font-mono" value={lat} onChange={(e) => { setLat(e.target.value); setPlan(null) }} inputMode="decimal" /></Field>
            <Field label="Centre longitude" htmlFor="mlon"><input id="mlon" className="input font-mono" value={lon} onChange={(e) => { setLon(e.target.value); setPlan(null) }} inputMode="decimal" /></Field>
            <Field label="Radius (miles)" htmlFor="mrad" hint="100 covers a county and its mutual-aid neighbours"><input id="mrad" type="number" min={1} max={250} className="input font-mono" value={radius} onChange={(e) => { setRadius(e.target.value); setPlan(null) }} /></Field>
          </div>
          <div className="flex gap-2">
            {!plan && <span className="text-xs text-slate-500">{busy ? 'Listing the downloads…' : ''}</span>}
            {plan && <button className="btn btn-go" disabled={busy || running} onClick={record}><Download size={14} aria-hidden /> Record {Number(radius)} miles around {name || 'this centre'}</button>}
          </div>
          {plan && (
            <div className="rounded border border-slate-800 p-2 text-xs">
              <div className="mb-1 text-slate-400">The area needs these OpenStreetMap extracts from Geofabrik ({mb(total)} in all). Copies already downloaded are checked and reused.</div>
              <ul className="space-y-0.5">
                {plan.map((e) => <li key={e.id} className="flex gap-2"><span className="text-slate-200">{e.name}</span><span className="ml-auto font-mono text-slate-500">{mb(e.bytes)}</span></li>)}
              </ul>
              <p className="mt-1 text-[10px] text-slate-500">Recording takes a few minutes for a 100-mile circle; dispatching carries on meanwhile with the current map.</p>
            </div>
          )}
        </section>
      </div>
      <PhotosAdmin view={view} />
    </Page>
  )
}
