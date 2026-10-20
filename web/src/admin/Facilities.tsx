import { useState } from 'react'
import { toast } from 'sonner'
import { Hospital, Plus, Save, Trash2 } from 'lucide-react'
import { api, errText } from '../api'
import { Field, Modal } from '../ui'
import { cx } from '../util'
import type { Facility, View } from '../types'
import { Page, Table } from './Admin'

const KINDS: [string, string][] = [
  ['hospital', 'Hospital'],
  ['trauma', 'Trauma centre'],
  ['burn', 'Burn centre'],
  ['stroke', 'Stroke centre'],
  ['cardiac', 'Cardiac centre'],
  ['psychiatric', 'Psychiatric'],
  ['other', 'Other'],
]
const STATUS: Record<Facility['status'], { label: string; cls: string }> = {
  open: { label: 'Taking patients', cls: 'bg-emerald-500/20 text-emerald-200' },
  diversion: { label: 'On diversion', cls: 'bg-amber-500/20 text-amber-200' },
  closed: { label: 'Closed', cls: 'bg-rose-500/20 text-rose-200' },
}
const blank = (): Facility => ({ id: '', name: '', kind: 'hospital', status: 'open' })

type Draft = Omit<Facility, 'lat' | 'lon'> & { isNew: boolean; lat: string; lon: string }

/** Receiving facilities: hospitals and specialty centres, and whether each is taking patients. */
export default function Facilities({ view }: { view: View }) {
  const list = view.entry?.facilities ?? []
  const [d, setD] = useState<Draft | null>(null)
  const open = (f?: Facility) => {
    const base = f ?? blank()
    setD({ ...base, isNew: !f, lat: base.lat != null ? String(base.lat) : '', lon: base.lon != null ? String(base.lon) : '' })
  }
  const save = async () => {
    if (!d) return
    try {
      await api.put(`/admin/facilities/${encodeURIComponent(d.id.trim())}`, {
        id: d.id.trim(), name: d.name, kind: d.kind, lat: d.lat ? Number(d.lat) : undefined, lon: d.lon ? Number(d.lon) : undefined, phone: d.phone, status: d.status,
      })
      toast.success(`${d.name || d.id} saved`)
      setD(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const remove = async (id: string) => {
    if (!confirm('Remove this facility?')) return
    try {
      await api.del(`/admin/facilities/${encodeURIComponent(id)}`)
      setD(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const setStatus = async (f: Facility, status: Facility['status']) => {
    try {
      await api.post(`/facilities/${encodeURIComponent(f.id)}/status`, { status, note: '' })
      toast.success(`${f.name || f.id}: ${STATUS[status].label.toLowerCase()}`)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const kindName = (k?: string) => KINDS.find(([c]) => c === k)?.[1] ?? k ?? '—'
  return (
    <Page
      title="Receiving facilities"
      intro="Hospitals and specialty centres, and whether each is taking patients. Crews and the console see the status beside a transport decision; a quick button changes it, and the change is an attributed event."
      actions={<button className="btn btn-go" onClick={() => open()}><Plus size={14} aria-hidden /> Add a facility</button>}
    >
      <Table head={['Name', 'Kind', 'Phone', 'Status', 'Set']} empty={list.length === 0 ? <p className="p-3 text-sm text-slate-500">No facilities yet.</p> : undefined}>
        {list.map((f) => (
          <tr key={f.id} className="hover:bg-slate-800/50">
            <td className="cursor-pointer px-3 py-1.5 text-slate-100" onClick={() => open(f)}>{f.name || f.id}</td>
            <td className="cursor-pointer px-3 text-slate-400" onClick={() => open(f)}>{kindName(f.kind)}</td>
            <td className="cursor-pointer px-3 text-slate-400" onClick={() => open(f)}>{f.phone ?? ''}</td>
            <td className="px-3"><span className={cx('chip', STATUS[f.status].cls)}>{STATUS[f.status].label}</span></td>
            <td className="px-3">
              <div className="flex gap-1">
                {(['open', 'diversion', 'closed'] as const).map((s) => (
                  <button key={s} className={cx('chip cursor-pointer', f.status === s ? STATUS[s].cls : 'bg-slate-800 text-slate-400 hover:bg-slate-700')} onClick={() => setStatus(f, s)} aria-pressed={f.status === s} title={STATUS[s].label}>{s === 'open' ? 'Open' : s === 'diversion' ? 'Divert' : 'Closed'}</button>
                ))}
              </div>
            </td>
          </tr>
        ))}
      </Table>

      {d && (
        <Modal
          open
          onClose={() => setD(null)}
          title={d.isNew ? 'New facility' : d.name || d.id}
          icon={<Hospital size={16} className="text-rose-300" aria-hidden />}
          footer={
            <>
              {!d.isNew && <button className="btn mr-auto text-rose-300" onClick={() => remove(d.id)}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => setD(null)}>Cancel</button>
              <button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save</button>
            </>
          }
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="ID" htmlFor="fid"><input id="fid" className="input font-mono" disabled={!d.isNew} value={d.id} onChange={(e) => setD({ ...d, id: e.target.value })} /></Field>
            <Field label="Name" htmlFor="fn"><input id="fn" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} /></Field>
            <Field label="Kind" htmlFor="fk">
              <select id="fk" className="input" value={d.kind ?? 'hospital'} onChange={(e) => setD({ ...d, kind: e.target.value })}>
                {KINDS.map(([c, n]) => <option key={c} value={c}>{n}</option>)}
              </select>
            </Field>
            <Field label="Status" htmlFor="fs">
              <select id="fs" className="input" value={d.status} onChange={(e) => setD({ ...d, status: e.target.value as Facility['status'] })}>
                {(['open', 'diversion', 'closed'] as const).map((s) => <option key={s} value={s}>{STATUS[s].label}</option>)}
              </select>
            </Field>
            <div className="sm:col-span-2"><Field label="Phone" htmlFor="fp"><input id="fp" className="input font-mono" value={d.phone ?? ''} onChange={(e) => setD({ ...d, phone: e.target.value })} /></Field></div>
            <Field label="Latitude" htmlFor="flat"><input id="flat" className="input font-mono" value={d.lat} onChange={(e) => setD({ ...d, lat: e.target.value })} /></Field>
            <Field label="Longitude" htmlFor="flon"><input id="flon" className="input font-mono" value={d.lon} onChange={(e) => setD({ ...d, lon: e.target.value })} /></Field>
          </div>
        </Modal>
      )}
    </Page>
  )
}
