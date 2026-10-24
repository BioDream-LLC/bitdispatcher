import { useState } from 'react'
import { toast } from 'sonner'
import { Plus, Trash2, Truck } from 'lucide-react'
import { api, ApiError, errText } from '../api'
import { Field, Modal, StatusChip } from '../ui'
import type { UnitType, View } from '../types'
import { Page, Table } from './Admin'

type Draft = { id: string; name: string; type: string; station: string; capabilities: string; radioId: string; radioAlias: string; agency: string; area: string; minStaff: string; isNew: boolean }

export default function Units({ view }: { view: View }) {
  const [d, setD] = useState<Draft | null>(null)
  const [err, setErr] = useState<ApiError | null>(null)
  const open = (id?: string) => {
    const u = view.units.find((x) => x.id === id)
    setErr(null)
    setD(u
      ? { id: u.id, name: u.name ?? '', type: u.type, station: u.station ?? '', capabilities: (u.capabilities ?? []).join(', '), radioId: u.radioId ? String(u.radioId) : '', radioAlias: u.radioAlias ?? '', agency: u.agency ?? '', area: u.area ?? '', minStaff: u.minStaff ? String(u.minStaff) : '', isNew: false }
      : { id: '', name: '', type: view.settings.unitTypes[0]?.code ?? '', station: view.stations[0]?.id ?? '', capabilities: '', radioId: '', radioAlias: '', agency: '', area: '', minStaff: '', isNew: true })
  }
  const save = async () => {
    if (!d) return
    try {
      await api.put(`/admin/units/${encodeURIComponent(d.id.trim().toUpperCase())}`, {
        name: d.name, type: d.type, station: d.station, agency: d.agency, radioAlias: d.radioAlias,
        radioId: d.radioId ? Number(d.radioId) : 0, area: d.area || undefined, minStaff: d.minStaff ? Number(d.minStaff) : 0,
        capabilities: d.capabilities.split(/[,\s]+/).map((c) => c.trim().toLowerCase()).filter(Boolean),
      })
      if (d.isNew) await api.post(`/units/${encodeURIComponent(d.id.trim().toUpperCase())}/status`, { status: 'available', reason: 'new unit' }).catch(() => {})
      toast.success(`Unit ${d.id.toUpperCase()} saved`)
      setD(null)
    } catch (e) {
      setErr(e instanceof ApiError ? e : new ApiError(0, errText(e)))
    }
  }
  const remove = async () => {
    if (!d || !confirm(`Remove unit ${d.id}?`)) return
    try {
      await api.del(`/admin/units/${encodeURIComponent(d.id)}`)
      toast.success(`Unit ${d.id} removed`)
      setD(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const typeOf = (c: string): UnitType | undefined => view.settings.unitTypes.find((t) => t.code === c)
  return (
    <Page
      title="Units"
      intro="Every unit a dispatcher can send. A unit's capabilities come from its type plus any you add; run cards ask for capabilities, not unit names. The radio ID ties BitVoice's radio events (emergency button, GPS, last heard) to the unit."
      actions={<button className="btn btn-go" onClick={() => open()}><Plus size={14} aria-hidden /> Add unit</button>}
    >
      <Table head={['Call sign', 'Type', 'Station', 'Capabilities', 'Radio', 'Status']} empty={view.units.length === 0 && <p className="p-4 text-sm text-slate-500">No units yet.</p>}>
        {view.units.map((u) => (
          <tr key={u.id} className="cursor-pointer hover:bg-slate-800/50" onClick={() => open(u.id)}>
            <td className="px-3 py-1.5 font-mono font-bold text-slate-100">{u.id}{u.name && <span className="ml-2 font-sans text-xs font-normal text-slate-400">{u.name}</span>}</td>
            <td className="px-3 text-slate-300">{typeOf(u.type)?.name ?? u.type}</td>
            <td className="px-3 text-slate-300">{view.stations.find((s) => s.id === u.station)?.name ?? <span className="text-amber-300">none</span>}</td>
            <td className="px-3 text-xs text-slate-400">{u.caps.join(', ')}</td>
            <td className="px-3 font-mono text-xs text-slate-400">{u.radioId || ''}{u.radioAlias ? ` ${u.radioAlias}` : ''}</td>
            <td className="px-3"><StatusChip status={u.status} /></td>
          </tr>
        ))}
      </Table>
      {d && (
        <Modal
          open
          onClose={() => setD(null)}
          title={d.isNew ? 'New unit' : `Unit ${d.id}`}
          icon={<Truck size={16} className="text-orange-300" aria-hidden />}
          footer={
            <>
              {!d.isNew && <button className="btn mr-auto text-rose-300" onClick={remove}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => setD(null)}>Cancel</button>
              <button className="btn btn-go" onClick={save}>Save</button>
            </>
          }
        >
          <div className="grid gap-3 sm:grid-cols-2">
            {err && <p className="sm:col-span-2 rounded bg-rose-950/50 px-2 py-1 text-sm text-rose-200">{err.message}</p>}
            <Field label="Call sign" htmlFor="uid" error={err?.field === 'id' ? err.message : undefined}>
              <input id="uid" className="input font-mono uppercase" disabled={!d.isNew} value={d.id} onChange={(e) => setD({ ...d, id: e.target.value })} autoFocus={d.isNew} />
            </Field>
            <Field label="Name" htmlFor="uname" hint="Optional, such as Medic 1 or a crew name">
              <input id="uname" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} />
            </Field>
            <Field label="Type" htmlFor="utype" error={err?.field === 'type' ? err.message : undefined} hint={typeOf(d.type)?.capabilities.join(', ')}>
              <select id="utype" className="input" value={d.type} onChange={(e) => setD({ ...d, type: e.target.value })}>
                {view.settings.unitTypes.map((t) => <option key={t.code} value={t.code}>{t.name}</option>)}
              </select>
            </Field>
            <Field label="Station" htmlFor="ust" hint="Where it is placed on the map when it has no AVL">
              <select id="ust" className="input" value={d.station} onChange={(e) => setD({ ...d, station: e.target.value })}>
                <option value="">None</option>
                {view.stations.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
              </select>
            </Field>
            <Field label="Extra capabilities" htmlFor="ucap" hint="Comma-separated, such as hazmat, water-rescue">
              <input id="ucap" className="input font-mono" value={d.capabilities} onChange={(e) => setD({ ...d, capabilities: e.target.value })} />
            </Field>
            <Field label="Agency" htmlFor="uag" hint="For shared and mutual-aid systems">
              <input id="uag" className="input" value={d.agency} onChange={(e) => setD({ ...d, agency: e.target.value })} />
            </Field>
            <Field label="Radio unit ID" htmlFor="urid" error={err?.field === 'radioId' ? err.message : undefined} hint="The P25 unit ID (decimal) BitVoice reports">
              <input id="urid" className="input font-mono" inputMode="numeric" value={d.radioId} onChange={(e) => setD({ ...d, radioId: e.target.value.replace(/\D/g, '') })} />
            </Field>
            <Field label="Radio alias" htmlFor="ural" hint="Matched when the radio sends no ID">
              <input id="ural" className="input" value={d.radioAlias} onChange={(e) => setD({ ...d, radioAlias: e.target.value })} />
            </Field>
            <Field label="Beat or area" htmlFor="uarea" hint="For police backup and response plans">
              <input id="uarea" className="input" value={d.area} onChange={(e) => setD({ ...d, area: e.target.value })} />
            </Field>
            <Field label="Minimum aboard" htmlFor="umin" hint="Empty: the unit type's (Settings)">
              <input id="umin" className="input font-mono" inputMode="numeric" value={d.minStaff} onChange={(e) => setD({ ...d, minStaff: e.target.value.replace(/\D/g, '') })} />
            </Field>
          </div>
        </Modal>
      )}
    </Page>
  )
}
