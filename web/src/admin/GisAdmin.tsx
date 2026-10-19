import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Database, Download, Plus, Square, Trash2, Upload } from 'lucide-react'
import { api, errText } from '../api'
import { ago, cx, useNow } from '../util'
import { Page } from './Admin'

type Layer = { id: string; name: string; kind: string; source: 'arcgis' | 'geojson' | 'overture'; url?: string; file?: string; where?: string; fields: Record<string, string>; on: boolean }
type Info = { name: string; geometry: string; fields: string[]; count: number; sample: Record<string, unknown>[]; guess: Record<string, string>; kind: string }
type Job = { started: string; finished?: string; by: string; layer: string; done: number; total: number; error?: string; stats?: { byKind: Record<string, number>; skipped: number; seconds: number; ranges?: number } }
type GISInfo = { layers: Layer[]; kinds: Record<string, string[]>; job?: Job; built?: string; stats?: { addresses: number }; source?: string; uploads: string[]; hasArea: boolean }

const KINDS: Record<string, string> = { address: 'Address points', hydrant: 'Hydrants', zone: 'Emergency service zones / beats', place: 'Common place names', road: 'Road centrelines (names and address ranges)' }
const FIELD: Record<string, string> = { num: 'House number', street: 'Street', full: 'Full address', unit: 'Unit', city: 'Community', zip: 'ZIP', name: 'Name or ID', detail: 'Detail',
  fromLeft: 'From, left', toLeft: 'To, left', fromRight: 'From, right', toRight: 'To, right', cityLeft: 'Community, left', cityRight: 'Community, right', zipLeft: 'ZIP, left', zipRight: 'ZIP, right' }
const PRESETS = [
  { name: 'Virginia address points (VGIN, statewide)', url: 'https://vginmaps.vdem.virginia.gov/arcgis/rest/services/VA_Base_Layers/VA_Address_Points/FeatureServer/0' },
  { name: 'Virginia road centrelines with address ranges (VGIN)', url: 'https://vginmaps.vdem.virginia.gov/arcgis/rest/services/VA_Base_Layers/VBMP_RCL/FeatureServer/5' },
]

const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 40) || 'layer'

/** County GIS: the county's own address points, hydrants and zones, searched before OpenStreetMap. */
export default function GisAdmin() {
  const now = useNow()
  const [v, setV] = useState<GISInfo | null>(null)
  const [url, setUrl] = useState('')
  const [info, setInfo] = useState<Info | null>(null)
  const [draft, setDraft] = useState<Layer | null>(null)
  const [busy, setBusy] = useState(false)
  const load = () => api.get<GISInfo>('/admin/gis').then(setV).catch((e) => toast.error(errText(e)))
  useEffect(() => { load() }, [])
  const running = !!v?.job && !v.job.finished
  useEffect(() => {
    if (!running) return
    const t = setInterval(load, 1000)
    return () => clearInterval(t)
  }, [running])
  const propose = (i: Info, src: Partial<Layer>) => {
    setInfo(i)
    setDraft({ id: slug(i.name), name: i.name, kind: i.kind, source: 'arcgis', fields: i.guess, on: true, ...src } as Layer)
  }
  // Inspecting starts as soon as an address is pasted: one step fewer.
  const inspect = async (u = url) => {
    if (!u.trim()) return
    setBusy(true)
    try {
      propose(await api.post<Info>('/admin/gis/inspect', { url: u.trim() }), { source: 'arcgis', url: u.trim() })
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  const upload = async (f: File) => {
    const name = f.name.replace(/[^A-Za-z0-9._-]/g, '_')
    setBusy(true)
    try {
      const res = await fetch(`./api/admin/gis/upload?name=${encodeURIComponent(name)}`, { method: 'POST', body: f, headers: { 'X-BitDispatcher': '1' }, credentials: 'same-origin' })
      const body = await res.json()
      if (!res.ok) throw new Error(body.message ?? res.statusText)
      propose(body as Info, { source: 'geojson', file: name })
      load()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  const save = async (ls: Layer[]) => {
    try {
      await api.put('/admin/gis/layers', ls)
      load()
      return true
    } catch (e) {
      toast.error(errText(e))
      return false
    }
  }
  const add = async () => {
    if (!draft || !v) return
    let id = draft.id
    for (let i = 2; v.layers.some((l) => l.id === id); i++) id = `${draft.id}-${i}`
    if (await save([...v.layers, { ...draft, id }])) {
      setDraft(null)
      setInfo(null)
      setUrl('')
      toast.success('Layer added: Import now reads it')
    }
  }
  const importNow = async () => {
    try {
      await api.post('/admin/gis/import')
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  if (!v) return <p className="text-sm text-slate-500">Loading…</p>
  const job = v.job
  return (
    <Page
      title="County GIS"
      intro="Import the county's own layers: address points, hydrants, emergency service zones and place names, from an ArcGIS REST service or a GeoJSON file. They are read for the recorded map's area into a store of their own, kept when the map is recorded again, drawn on the dispatch map, and searched before OpenStreetMap: the county's address points are the authoritative ones. Centreline address ranges place numbers that have no point; Overture's open address points fill in after the county's."
      actions={<button className="btn btn-go" disabled={running || !v.hasArea || !v.layers.some((l) => l.on)} onClick={importNow}><Download size={14} aria-hidden /> Import now</button>}
    >
      {!v.hasArea && <p className="mb-3 rounded border border-amber-800 bg-amber-950/30 p-2 text-sm text-amber-200">Record the map first (Admin › Map): GIS layers are read for the recorded area.</p>}
      <div className="grid gap-4 lg:grid-cols-[1fr_24rem]">
        <div className="space-y-4">
          <section className="panel rounded-lg">
            <table className="w-full text-sm">
              <thead><tr className="border-b border-slate-800 text-left text-[11px] uppercase tracking-wider text-slate-500"><th className="px-3 py-2">On</th><th>Layer</th><th>Kind</th><th>Source</th><th>Fields</th><th /></tr></thead>
              <tbody className="divide-y divide-slate-800">
                {v.layers.length === 0 && <tr><td colSpan={6} className="px-3 py-3 text-slate-500">No layers yet: add one on the right.</td></tr>}
                {v.layers.map((l, i) => (
                  <tr key={l.id}>
                    <td className="px-3 py-1.5"><input type="checkbox" checked={l.on} onChange={(e) => save(v.layers.map((x, j) => (j === i ? { ...x, on: e.target.checked } : x)))} aria-label={`${l.name} on`} /></td>
                    <td className="font-semibold text-slate-100">{l.name}</td>
                    <td className="text-slate-300">{KINDS[l.kind] ?? l.kind}</td>
                    <td className="max-w-64 truncate font-mono text-[11px] text-slate-400" title={l.url ?? l.file}>{l.source === 'arcgis' ? l.url?.replace(/^https:\/\//, '') : l.source === 'overture' ? `Overture Maps, ${l.url || 'latest release'}` : l.file}</td>
                    <td className="text-[11px] text-slate-400">{Object.entries(l.fields).filter(([, s]) => s).map(([k, s]) => `${FIELD[k] ?? k}: ${s}`).join(' · ')}</td>
                    <td className="pr-2 text-right"><button className="btn btn-ghost px-1.5" aria-label={`Remove ${l.name}`} onClick={() => save(v.layers.filter((_, j) => j !== i))}><Trash2 size={13} aria-hidden /></button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
          <section className="panel space-y-2 rounded-lg p-3 text-sm">
            <h2 className="flex items-center gap-2 font-semibold text-slate-100"><Database size={15} aria-hidden /> In use</h2>
            {v.built ? (
              <p className="text-slate-300">Imported {new Date(v.built).toLocaleString()} ({ago(v.built, now)} ago): <b>{(v.stats?.addresses ?? 0).toLocaleString()}</b> address points. <span className="text-xs text-slate-500">{v.source}</span></p>
            ) : <p className="text-slate-500">Nothing imported yet.</p>}
            {job && (
              <div className={cx('rounded border p-2 text-xs', job.error ? 'border-rose-800 bg-rose-950/30' : running ? 'border-sky-800 bg-sky-950/30' : 'border-slate-800')}>
                <div className="flex items-center gap-2">
                  <span className="font-semibold text-slate-200">{running ? `Reading ${job.layer}` : job.error ? 'Last import failed' : 'Last import finished'}</span>
                  <span className="text-slate-500">by {job.by}, {ago(job.started, now)} ago</span>
                  {running && <button className="btn ml-auto h-6 px-2 py-0 text-[11px]" onClick={() => api.post('/admin/gis/cancel').then(load)}><Square size={11} aria-hidden /> Stop</button>}
                </div>
                {running && job.total > 0 && (
                  <div className="mt-1 h-1.5 overflow-hidden rounded bg-slate-800"><div className="h-full bg-sky-400" style={{ width: `${(job.done / job.total) * 100}%` }} /></div>
                )}
                {running && <div className="mt-1 font-mono text-[10px] text-slate-400">{job.done.toLocaleString()} / {job.total.toLocaleString()}</div>}
                {job.stats && <div className="mt-1 font-mono text-[10px] text-slate-400">{Object.entries(job.stats.byKind).map(([k, n]) => `${k} ${n.toLocaleString()}`).join(' · ')}{job.stats.ranges ? ` · address ranges ${job.stats.ranges.toLocaleString()}` : ''} · outside or incomplete {job.stats.skipped.toLocaleString()} · {job.stats.seconds} s</div>}
                {job.error && <div className="mt-1 text-rose-300">{job.error}</div>}
              </div>
            )}
          </section>
        </div>
        <section className="panel space-y-2 rounded-lg p-3">
          <h2 className="flex items-center gap-2 text-sm font-semibold text-slate-100"><Plus size={15} aria-hidden /> Add a layer</h2>
          <div className="flex flex-wrap gap-1">
            {PRESETS.map((p) => <button key={p.url} className="btn text-xs" onClick={() => { setUrl(p.url); inspect(p.url) }}>{p.name}</button>)}
            {!v.layers.some((l) => l.source === 'overture') && (
              <button className="btn text-xs" title="Open address points gathered by the Overture Maps Foundation (in the US largely the National Address Database): they fill in where the county has none, after the county's own"
                onClick={() => save([...v.layers, { id: 'overture-addresses', name: 'Overture addresses (open data)', kind: 'address', source: 'overture', fields: {}, on: true }])}>Overture addresses (open data, any area)</button>
            )}
          </div>
          <input className="input font-mono text-xs" placeholder="https://…/arcgis/rest/services/…/FeatureServer/0" value={url}
            onChange={(e) => setUrl(e.target.value)} onPaste={(e) => { const t = e.clipboardData.getData('text'); setTimeout(() => inspect(t), 0) }} onKeyDown={(e) => e.key === 'Enter' && inspect()} aria-label="ArcGIS layer address" />
          <div className="flex items-center gap-2">
            <button className="btn text-xs" disabled={busy || !url} onClick={() => inspect()}>Read the layer</button>
            <span className="text-xs text-slate-500">or</span>
            <label className="btn cursor-pointer text-xs"><Upload size={12} aria-hidden /> GeoJSON or CSV file<input type="file" accept=".geojson,.json,.csv,application/geo+json,text/csv" className="hidden" onChange={(e) => e.target.files?.[0] && upload(e.target.files[0])} /></label>
            {busy && <span className="text-xs text-slate-500">Reading…</span>}
          </div>
          {info && draft && (
            <div className="space-y-2 rounded border border-slate-700 p-2 text-xs">
              <div className="text-slate-300"><b className="text-slate-100">{info.name}</b> · {info.geometry} · {info.count.toLocaleString()} in the area</div>
              <label className="flex items-center gap-2 text-slate-400">Name <input className="input h-7 py-0 text-xs" value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} /></label>
              <label className="flex items-center gap-2 text-slate-400">Kind
                <select className="input h-7 py-0 text-xs" value={draft.kind} onChange={(e) => setDraft({ ...draft, kind: e.target.value, fields: {} })}>
                  {Object.entries(KINDS).map(([k, l]) => <option key={k} value={k}>{l}</option>)}
                </select>
              </label>
              {(v.kinds[draft.kind] ?? []).map((f) => (
                <label key={f} className="flex items-center gap-2 text-slate-400"><span className="w-24">{FIELD[f]}</span>
                  <select className="input h-7 py-0 font-mono text-xs" value={draft.fields[f] ?? ''} onChange={(e) => setDraft({ ...draft, fields: { ...draft.fields, [f]: e.target.value } })}>
                    <option value="">—</option>
                    {info.fields.map((x) => <option key={x} value={x}>{x}</option>)}
                  </select>
                  <span className="truncate font-mono text-[10px] text-slate-500">{draft.fields[f] ? String(info.sample[0]?.[draft.fields[f]] ?? '') : ''}</span>
                </label>
              ))}
              {draft.source === 'arcgis' && <label className="flex items-center gap-2 text-slate-400">Only <input className="input h-7 py-0 font-mono text-xs" placeholder="where, e.g. STATUS = 'Active'" value={draft.where ?? ''} onChange={(e) => setDraft({ ...draft, where: e.target.value })} /></label>}
              <button className="btn btn-go text-xs" onClick={add}><Plus size={12} aria-hidden /> Add layer</button>
            </div>
          )}
        </section>
      </div>
    </Page>
  )
}
