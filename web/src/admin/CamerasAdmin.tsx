import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Camera as CameraIcon, Pencil, Plus, Save, Trash2 } from 'lucide-react'
import { api, errText } from '../api'
import { can, useStore } from '../store'
import { Field, Modal } from '../ui'
import type { Camera, CameraLook, View } from '../types'
import { Page } from './Admin'

const blank = (): Camera => ({ id: '', name: '', owner: '', lat: 0, lon: 0, heading: null, fov: 70, range: 150, live: '', still: '', note: '' })

/** Cameras: the agency's list of cameras dispatchers may open for an incident, the policy, and the record
 * of every look. */
export default function CamerasAdmin({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const admin = can.admin(me)
  const [items, setItems] = useState<Camera[]>([])
  const [looks, setLooks] = useState<CameraLook[]>([])
  const [d, setD] = useState<Camera | null>(null)
  const pol = view.settings.next?.cameras ?? {}
  const load = () => {
    api.get<{ items: Camera[] }>('/admin/cameras').then((r) => setItems(r.items)).catch((e) => toast.error(errText(e)))
    api.get<{ items: CameraLook[] }>('/admin/cameras/looks').then((r) => setLooks(r.items)).catch(() => {})
  }
  useEffect(load, [])
  const savePolicy = async (p: { on?: boolean; radius?: number }) => {
    try {
      await api.put('/admin/settings', { ...view.settings, next: { ...view.settings.next, cameras: { ...pol, ...p } } })
      toast.success('Saved')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const save = async () => {
    if (!d) return
    try {
      await api.put('/admin/cameras', { ...d, heading: d.heading === null || d.heading === undefined || Number.isNaN(d.heading) ? undefined : d.heading })
      toast.success('Camera saved')
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const num = (v: string) => (v === '' ? null : Number(v))
  return (
    <Page title="Cameras" intro="Cameras a dispatcher may open for an incident: traffic cameras, the agency's buildings, schools and partners that agreed to share. Off until switched on. Opening one asks why and is recorded; bitdispatcher keeps no video."
      actions={admin && <button className="btn" onClick={() => setD(blank())}><Plus size={13} aria-hidden /> Add a camera</button>}>
      <section className="panel mb-4 space-y-2 rounded-lg p-4 text-sm">
        <label className="flex items-center gap-2 text-slate-200"><input type="checkbox" disabled={!admin} checked={!!pol.on} onChange={(e) => savePolicy({ on: e.target.checked })} /> Offer cameras near incidents</label>
        <div className="flex items-center gap-2 text-slate-300">
          Within <input type="number" min={50} max={5000} disabled={!admin} className="input w-24 font-mono" defaultValue={pol.radius || 400} onBlur={(e) => savePolicy({ radius: Number(e.target.value) })} /> metres of the incident
        </div>
        <p className="text-xs text-slate-500">Before switching this on, agree a policy: which cameras, for which calls, and who reviews the record below. Several cities have stepped back from camera networks after misuse; the record of who looked and why is what keeps this to calls.</p>
      </section>
      <section className="panel mb-4 rounded-lg p-4">
        <h2 className="mb-2 flex items-center gap-2 font-semibold text-slate-100"><CameraIcon size={15} aria-hidden /> {items.length} camera{items.length === 1 ? '' : 's'}</h2>
        <table className="w-full text-sm">
          <thead className="text-left text-[11px] uppercase text-slate-500"><tr><th>Name</th><th>Owner</th><th>Where</th><th>Looks</th><th>Has</th><th /></tr></thead>
          <tbody>
            {items.map((c) => (
              <tr key={c.id} className="border-t border-slate-800">
                <td className="py-1.5 text-slate-100">{c.name}{c.off && <span className="ml-1 text-[10px] text-amber-300">off</span>}</td>
                <td className="text-slate-400">{c.owner}</td>
                <td className="font-mono text-xs text-slate-400">{c.lat.toFixed(5)}, {c.lon.toFixed(5)}</td>
                <td className="text-xs text-slate-400">{c.heading == null ? 'turns / unknown' : `${c.heading}° · ${c.fov || 70}° wide · ${c.range || 150} m`}</td>
                <td className="text-xs text-slate-400">{[c.live && 'live view', c.still && 'still picture'].filter(Boolean).join(', ')}</td>
                <td className="whitespace-nowrap text-right">{admin && <>
                  <button className="btn btn-ghost h-6 px-1.5" aria-label={`Edit ${c.name}`} onClick={() => setD({ ...c })}><Pencil size={12} aria-hidden /></button>
                  <button className="btn btn-ghost h-6 px-1.5" aria-label={`Remove ${c.name}`} onClick={async () => { if (confirm(`Remove ${c.name}?`)) { try { await api.del(`/admin/cameras/${c.id}`); load() } catch (e) { toast.error(errText(e)) } } }}><Trash2 size={12} aria-hidden /></button>
                </>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>
      <section className="panel rounded-lg p-4">
        <h2 className="mb-2 font-semibold text-slate-100">Who looked, and why</h2>
        {!looks.length ? <p className="text-sm text-slate-500">No camera has been opened.</p> : (
          <table className="w-full text-sm">
            <thead className="text-left text-[11px] uppercase text-slate-500"><tr><th>When</th><th>Who</th><th>Camera</th><th>Why</th></tr></thead>
            <tbody>{looks.map((l, i) => <tr key={i} className="border-t border-slate-800"><td className="py-1 font-mono text-xs text-slate-400">{new Date(l.at).toLocaleString()}</td><td>{l.user}</td><td>{l.name}</td><td className="text-slate-300">{l.purpose}</td></tr>)}</tbody>
          </table>
        )}
      </section>
      <Modal open={!!d} onClose={() => setD(null)} title={d?.id ? 'Edit camera' : 'Add a camera'} wide footer={<><button className="btn" onClick={() => setD(null)}>Cancel</button><button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save</button></>}>
        {d && (
          <div className="grid grid-cols-2 gap-x-3 gap-y-2 text-sm">
            <Field label="Name" htmlFor="cn"><input id="cn" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} placeholder="Orange Ave at 10th St" /></Field>
            <Field label="Owner" htmlFor="co"><input id="co" className="input" value={d.owner ?? ''} onChange={(e) => setD({ ...d, owner: e.target.value })} placeholder="VDOT" /></Field>
            <Field label="Latitude" htmlFor="cla"><input id="cla" type="number" step="0.00001" className="input font-mono" value={d.lat || ''} onChange={(e) => setD({ ...d, lat: Number(e.target.value) })} /></Field>
            <Field label="Longitude" htmlFor="clo"><input id="clo" type="number" step="0.00001" className="input font-mono" value={d.lon || ''} onChange={(e) => setD({ ...d, lon: Number(e.target.value) })} /></Field>
            <Field label="Looks toward (degrees from north; empty if it turns)" htmlFor="ch"><input id="ch" type="number" min={0} max={359} className="input font-mono" value={d.heading ?? ''} onChange={(e) => setD({ ...d, heading: num(e.target.value) })} /></Field>
            <div className="grid grid-cols-2 gap-2">
              <Field label="Width (°)" htmlFor="cf"><input id="cf" type="number" min={1} max={360} className="input font-mono" value={d.fov || 70} onChange={(e) => setD({ ...d, fov: Number(e.target.value) })} /></Field>
              <Field label="Sees (m)" htmlFor="cr"><input id="cr" type="number" min={10} max={5000} className="input font-mono" value={d.range || 150} onChange={(e) => setD({ ...d, range: Number(e.target.value) })} /></Field>
            </div>
            <Field label="Live view address (opens in the camera system)" htmlFor="cl"><input id="cl" className="input font-mono text-xs" value={d.live ?? ''} onChange={(e) => setD({ ...d, live: e.target.value })} placeholder="https://vms.example.gov/cameras/17" /></Field>
            <Field label="Still picture address (a JPEG of now)" htmlFor="cs"><input id="cs" className="input font-mono text-xs" value={d.still ?? ''} onChange={(e) => setD({ ...d, still: e.target.value })} placeholder="https://511.example.gov/cams/17.jpg" /></Field>
            <Field label="Note" htmlFor="cno"><input id="cno" className="input" value={d.note ?? ''} onChange={(e) => setD({ ...d, note: e.target.value })} /></Field>
            <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!d.off} onChange={(e) => setD({ ...d, off: e.target.checked })} /> Out of service</label>
          </div>
        )}
      </Modal>
    </Page>
  )
}
