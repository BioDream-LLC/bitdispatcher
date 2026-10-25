import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Ban, Car, Save } from 'lucide-react'
import { api, errText } from '../api'
import { can, useStore } from '../store'
import { Field } from '../ui'
import type { RoadsView, TrafficStatus, View } from '../types'
import { Page } from './Admin'

type TrafficCfg = { on: boolean; kind: 'waze' | 'geojson'; url: string; tokenSet: boolean; every: number; status: TrafficStatus }

/** Road closures and live traffic: every closure in force (and those lifted in the last day), and the
 * traffic feed. Dispatchers close roads from the map; here they are reviewed and lifted. */
export default function RoadsAdmin({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const [roads, setRoads] = useState<RoadsView | null>(null)
  const [cfg, setCfg] = useState<TrafficCfg | null>(null)
  const [token, setToken] = useState('')
  const load = () => {
    api.get<RoadsView>('/roads').then(setRoads).catch((e) => toast.error(errText(e)))
    if (can.admin(me)) api.get<TrafficCfg>('/admin/traffic').then(setCfg).catch(() => {})
  }
  useEffect(load, [view.next?.roads.version, view.next?.roads.closures.length])
  const act = async (fn: () => Promise<unknown>, ok: string) => {
    try {
      await fn()
      toast.success(ok)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const saveTraffic = async () => {
    if (!cfg) return
    await act(() => api.put('/admin/traffic', { on: cfg.on, kind: cfg.kind, url: cfg.url, every: cfg.every, token }), 'Traffic feed saved')
    setToken('')
  }
  return (
    <Page title="Road closures & traffic" intro="Closed roads and slow traffic change drive times and unit recommendations at once, without rebuilding the map. Dispatchers close a road by right-clicking the map; a closure ends at its time or when lifted, and stays in the ledger.">
      <section className="panel mb-4 rounded-lg p-4">
        <h2 className="mb-2 flex items-center gap-2 font-semibold text-slate-100"><Ban size={15} className="text-rose-400" aria-hidden /> Closed now · {roads?.closures.length ?? 0}</h2>
        {!roads?.closures.length ? <p className="text-sm text-slate-500">No road is closed. Right-click the dispatcher's map: Close a road from here.</p> : (
          <table className="w-full text-sm">
            <thead className="text-left text-[11px] uppercase text-slate-500"><tr><th>Why</th><th>Roads</th><th>Since</th><th>Until</th><th>Detour</th><th>By</th><th /></tr></thead>
            <tbody>
              {roads.closures.map((c) => (
                <tr key={c.id} className="border-t border-slate-800 align-top">
                  <td className="py-1.5 pr-2 text-slate-100">{c.reason}{c.incident && <div className="font-mono text-[10px] text-slate-500">{c.incident}</div>}</td>
                  <td className="pr-2 text-slate-300">{(c.roads ?? []).join(', ') || <span className="text-amber-300">no recorded road under it</span>}<div className="text-[10px] text-slate-500">{c.segments ?? 0} segments{c.center ? ` · circle ${c.radius} m` : ''}</div></td>
                  <td className="pr-2 font-mono text-xs text-slate-400">{new Date(c.from).toLocaleString()}</td>
                  <td className="pr-2 font-mono text-xs text-slate-400">{c.until ? new Date(c.until).toLocaleString() : 'until lifted'}</td>
                  <td className="pr-2 text-xs text-slate-300">{c.detour}</td>
                  <td className="pr-2 text-xs text-slate-400">{c.by}</td>
                  <td className="whitespace-nowrap text-right">
                    <button className="btn h-6 px-2 py-0 text-[11px]" onClick={() => act(() => api.post(`/roads/closures/${c.id}/lift`), 'Road open again')}>Lift</button>
                    <button className="btn btn-ghost h-6 px-2 py-0 text-[11px] text-slate-400" title="Entered by mistake: remove it (it stays in the ledger)" onClick={() => confirm('Remove this closure as a mistake? Lifting keeps it on record as a closure.') && act(() => api.del(`/roads/closures/${c.id}`), 'Removed')}>Remove</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {(roads?.lifted ?? []).length > 0 && (
          <div className="mt-3 text-xs text-slate-500">Ended in the last day: {roads!.lifted!.map((c) => c.reason + (c.liftedBy ? ` (lifted by ${c.liftedBy})` : ' (ran out)')).join(' · ')}</div>
        )}
      </section>
      {cfg && (
        <section className="panel space-y-3 rounded-lg p-4">
          <h2 className="flex items-center gap-2 font-semibold text-slate-100"><Car size={15} className="text-orange-300" aria-hidden /> Live traffic</h2>
          <p className="text-xs text-slate-400">
            <b>Waze for Cities</b> is Waze's free data-sharing programme for public agencies: once the agency joins, Waze gives it the address of a JSON traffic feed. Any other source can send a <b>GeoJSON</b> FeatureCollection of lines with <code>speed_kmh</code>, <code>factor</code> (times slower) or <code>closed: true</code>. Each slow road's drive time is its recorded time × how much slower it is, up to ten times; a road Waze reports closed is closed.
          </p>
          <label className="flex items-center gap-2 text-sm text-slate-200"><input type="checkbox" checked={cfg.on} onChange={(e) => setCfg({ ...cfg, on: e.target.checked })} /> Use live traffic</label>
          <div className="grid grid-cols-[10rem_1fr_8rem] gap-2">
            <Field label="Feed" htmlFor="tk"><select id="tk" className="input" value={cfg.kind} onChange={(e) => setCfg({ ...cfg, kind: e.target.value as TrafficCfg['kind'] })}><option value="waze">Waze for Cities</option><option value="geojson">GeoJSON</option></select></Field>
            <Field label="Address" htmlFor="tu"><input id="tu" className="input font-mono text-xs" value={cfg.url} onChange={(e) => setCfg({ ...cfg, url: e.target.value })} placeholder="https://www.waze.com/row-partnerhub-api/partners/…/waze-feeds/…?format=1" /></Field>
            <Field label="Every (seconds)" htmlFor="te"><input id="te" type="number" min={30} max={3600} className="input font-mono" value={cfg.every || 120} onChange={(e) => setCfg({ ...cfg, every: Number(e.target.value) })} /></Field>
          </div>
          <Field label={cfg.tokenSet ? 'Token (set; type a new one to change it, - to clear)' : 'Token, if the feed needs one (sent as Bearer)'} htmlFor="tt"><input id="tt" type="password" autoComplete="off" className="input w-96" value={token} onChange={(e) => setToken(e.target.value)} /></Field>
          <div className="flex items-center gap-3">
            <button className="btn btn-go" onClick={saveTraffic}><Save size={13} aria-hidden /> Save</button>
            <span className="text-xs text-slate-400">
              {cfg.status.error ? <span className="text-amber-300">Last fetch failed: {cfg.status.error}</span> : cfg.status.at ? `Last fetched ${new Date(cfg.status.at).toLocaleTimeString()}: ${view.next?.roads.jams ?? 0} slowdowns` : cfg.on ? 'Not fetched yet' : 'Off'}
            </span>
          </div>
        </section>
      )}
    </Page>
  )
}
