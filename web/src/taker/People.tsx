import { useState } from 'react'
import { toast } from 'sonner'
import { Car, Pencil, Plus, ShieldAlert, Sparkles, Trash2, Users } from 'lucide-react'
import { api, errText } from '../api'
import { cx } from '../util'
import type { Call, Incident, Person, Vehicle, View } from '../types'
import type { Draft } from './draft'

type Props = { view: View; d: Draft; inc?: Incident; call?: Call; update: (p: Partial<Draft> | ((d: Draft) => Draft)) => void }

const PERSON_ROLES = ['caller', 'victim', 'suspect', 'witness', 'complainant', 'subject', 'patient', 'other']
const VEHICLE_ROLES = ['suspect', 'victim', 'involved', 'witness', 'abandoned', 'other']

const PERSON_FIELDS: [keyof Person, string, string?][] = [
  ['name', 'Name', 'LAST, FIRST MIDDLE'], ['dob', 'Date of birth or age', '1988-03-14, or about 30'], ['sex', 'Sex'], ['race', 'Race'],
  ['height', 'Height', `5'10"`], ['weight', 'Weight', '180'], ['hair', 'Hair'], ['eyes', 'Eyes'], ['clothing', 'Clothing', 'black hoodie, jeans'],
  ['address', 'Address'], ['phone', 'Phone'], ['idType', 'ID type', 'DL, state ID'], ['idNumber', 'ID number'], ['idState', 'ID state'], ['notes', 'Notes'],
]
const VEHICLE_FIELDS: [keyof Vehicle, string, string?][] = [
  ['plate', 'Plate', 'KTR4471'], ['state', 'State', 'VA'], ['vin', 'VIN'], ['year', 'Year'], ['make', 'Make'], ['model', 'Model'], ['color', 'Colour'],
  ['body', 'Body', 'sedan, pickup'], ['direction', 'Direction of travel', 'north on Williamson Rd'], ['description', 'Description'],
  ['driver', 'Driver'], ['owner', 'Owner'], ['contact', 'Contact'], ['notes', 'Notes'],
]

/** What the call's words say about people and vehicles, offered to fill the forms. */
function fromCall(call: Call | undefined, kinds: string[]) {
  return (call?.details ?? []).filter((x) => kinds.includes(x.kind))
}

export function PersonsTab({ d, inc, call, update }: Props) {
  const list = (inc?.persons ?? d.persons).filter((p) => !p.removed)
  const [edit, setEdit] = useState<Person | null>(null)
  const save = async (p: Person) => {
    try {
      if (inc) {
        if (p.id) await api.put(`/incidents/${inc.number}/persons/${p.id}`, p)
        else await api.post(`/incidents/${inc.number}/persons`, p)
      } else {
        update((cur) => ({ ...cur, persons: p.id ? cur.persons.map((x) => (x.id === p.id ? p : x)) : [...cur.persons, { ...p, id: `draft-${cur.persons.length + 1}` }] }))
      }
      setEdit(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const hints = fromCall(call, ['name', 'count'])
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-1.5 text-sm font-semibold text-slate-100"><Users size={14} aria-hidden /> Persons</h2>
        <button className="btn btn-go ml-auto h-7 text-xs" onClick={() => setEdit({ id: '', role: list.length ? 'other' : 'caller' })}><Plus size={12} aria-hidden /> Add a person</button>
      </div>
      {hints.length > 0 && (
        <div className="flex flex-wrap items-center gap-1 text-[11px] text-slate-400">
          <Sparkles size={11} className="text-sky-300" aria-hidden /> Heard on the call:
          {hints.map((h, i) => (
            <button key={i} className="chip h-5 bg-slate-800 text-slate-200 hover:bg-slate-700" onClick={() => setEdit({ id: '', role: /suspect/i.test(h.text) ? 'suspect' : 'other', name: h.kind === 'name' ? h.text : undefined, notes: h.kind !== 'name' ? h.text : undefined })}>{h.text}</button>
          ))}
        </div>
      )}
      {list.length === 0 && !edit && <p className="text-xs text-slate-500">No one added. Add the caller, victims, suspects and witnesses as the call goes.</p>}
      <ul className="space-y-1">
        {list.map((p) => (
          <li key={p.id} className={cx('panel flex items-start gap-2 rounded-md px-2 py-1.5 text-xs', p.safety && 'border-rose-700')}>
            <span className={cx('chip uppercase', p.role === 'suspect' ? 'bg-rose-500/20 text-rose-200' : 'bg-slate-800 text-slate-300')}>{p.role}</span>
            <div className="min-w-0 flex-1">
              <div className="font-semibold text-slate-100">{p.name || 'Unnamed'} {p.safety && <span className="chip bg-rose-500 text-white"><ShieldAlert size={10} aria-hidden /> officer safety</span>}</div>
              <div className="text-slate-400">{[p.dob, p.sex, p.race, p.height, p.weight, p.hair && `hair ${p.hair}`, p.eyes && `eyes ${p.eyes}`, p.clothing, p.address, p.phone, p.idNumber && `${p.idType ?? 'ID'} ${p.idNumber} ${p.idState ?? ''}`].filter(Boolean).join(' · ')}</div>
              {p.aliases?.length ? <div className="text-slate-500">aka {p.aliases.join(', ')}</div> : null}
              {p.notes && <div className="text-slate-300">{p.notes}</div>}
            </div>
            <button className="btn btn-ghost h-6 px-1" aria-label={`Change ${p.name ?? p.role}`} onClick={() => setEdit(p)}><Pencil size={12} /></button>
            <button className="btn btn-ghost h-6 px-1" aria-label={`Remove ${p.name ?? p.role}`} onClick={() => (inc ? save({ ...p, removed: true }) : update((cur) => ({ ...cur, persons: cur.persons.filter((x) => x.id !== p.id) })))}><Trash2 size={12} /></button>
          </li>
        ))}
      </ul>
      {edit && (
        <Form title={edit.id ? 'Change person' : 'New person'} roles={PERSON_ROLES} value={edit} fields={PERSON_FIELDS} onCancel={() => setEdit(null)} onSave={(v) => save(v as Person)}
          extra={(v, set) => (
            <>
              <label className="col-span-2 flex flex-col gap-0.5 text-[11px] text-slate-400">Aliases (comma separated)
                <input className="input h-7 text-xs" value={(v.aliases as string[] | undefined)?.join(', ') ?? ''} onChange={(e) => set({ aliases: e.target.value.split(',').map((x) => x.trim()).filter(Boolean) })} />
              </label>
              <label className="col-span-2 flex items-center gap-1.5 text-xs text-rose-200"><input type="checkbox" checked={!!v.safety} onChange={(e) => set({ safety: e.target.checked })} /> Officer-safety concern about this person</label>
            </>
          )} />
      )}
    </div>
  )
}

export function VehiclesTab({ d, inc, call, update }: Props) {
  const list = (inc?.vehicles ?? d.vehicles).filter((v) => !v.removed)
  const [edit, setEdit] = useState<Vehicle | null>(null)
  const save = async (v: Vehicle) => {
    try {
      if (inc) {
        if (v.id) await api.put(`/incidents/${inc.number}/vehicles/${v.id}`, v)
        else await api.post(`/incidents/${inc.number}/vehicles`, v)
      } else {
        update((cur) => ({ ...cur, vehicles: v.id ? cur.vehicles.map((x) => (x.id === v.id ? v : x)) : [...cur.vehicles, { ...v, id: `draft-${cur.vehicles.length + 1}` }] }))
      }
      setEdit(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  // What the call said: a vehicle, a plate and a direction become one vehicle.
  const heard = fromCall(call, ['vehicle', 'plate', 'direction'])
  const prefill = (): Vehicle => {
    const v: Vehicle = { id: '', role: 'suspect' }
    for (const h of heard) {
      if (h.kind === 'plate') {
        const m = h.text.match(/^(?:([A-Z]{2})\s+)?([A-Z0-9-]+)$/i)
        v.plate = (m?.[2] ?? h.text).replace(/-/g, '').toUpperCase()
        if (m?.[1]) v.state = m[1].toUpperCase()
      } else if (h.kind === 'direction') v.direction = h.text
      else if (h.kind === 'vehicle') v.description = h.text
    }
    return v
  }
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-1.5 text-sm font-semibold text-slate-100"><Car size={14} aria-hidden /> Vehicles</h2>
        {heard.length > 0 && <button className="btn h-7 text-xs" onClick={() => setEdit(prefill())}><Sparkles size={12} className="text-sky-300" aria-hidden /> From the call: {heard.map((h) => h.text).join(' · ')}</button>}
        <button className="btn btn-go ml-auto h-7 text-xs" onClick={() => setEdit({ id: '', role: 'involved' })}><Plus size={12} aria-hidden /> Add a vehicle</button>
      </div>
      {list.length === 0 && !edit && <p className="text-xs text-slate-500">No vehicle added.</p>}
      <ul className="space-y-1">
        {list.map((v) => (
          <li key={v.id} className="panel flex items-start gap-2 rounded-md px-2 py-1.5 text-xs">
            <span className={cx('chip uppercase', v.role === 'suspect' ? 'bg-rose-500/20 text-rose-200' : 'bg-slate-800 text-slate-300')}>{v.role}</span>
            <div className="min-w-0 flex-1">
              <div className="font-semibold text-slate-100">{[v.color, v.year, v.make, v.model, v.body].filter(Boolean).join(' ') || v.description || 'Vehicle'}{v.plate && <span className="ml-1.5 font-mono text-amber-200">{v.state} {v.plate}</span>}</div>
              <div className="text-slate-400">{[v.vin && `VIN ${v.vin}`, v.direction && `going ${v.direction}`, v.driver && `driver ${v.driver}`, v.owner && `owner ${v.owner}`, v.contact, v.description].filter(Boolean).join(' · ')}</div>
            </div>
            <button className="btn btn-ghost h-6 px-1" aria-label="Change vehicle" onClick={() => setEdit(v)}><Pencil size={12} /></button>
            <button className="btn btn-ghost h-6 px-1" aria-label="Remove vehicle" onClick={() => (inc ? save({ ...v, removed: true }) : update((cur) => ({ ...cur, vehicles: cur.vehicles.filter((x) => x.id !== v.id) })))}><Trash2 size={12} /></button>
          </li>
        ))}
      </ul>
      {edit && <Form title={edit.id ? 'Change vehicle' : 'New vehicle'} roles={VEHICLE_ROLES} value={edit} fields={VEHICLE_FIELDS} onCancel={() => setEdit(null)} onSave={(v) => save(v as Vehicle)} />}
    </div>
  )
}

type Any = Record<string, unknown> & { role: string }

function Form<T extends Any>({ title, roles, value, fields, onSave, onCancel, extra }: {
  title: string; roles: string[]; value: T; fields: [string, string, string?][]; onSave: (v: T) => void; onCancel: () => void
  extra?: (v: T, set: (p: Partial<T>) => void) => React.ReactNode
}) {
  const [v, setV] = useState<T>(value)
  const set = (p: Partial<T>) => setV((cur) => ({ ...cur, ...p }))
  return (
    <form className="panel rounded-lg p-2" onSubmit={(e) => { e.preventDefault(); onSave(v) }} aria-label={title}>
      <div className="mb-1.5 flex items-center gap-2">
        <h3 className="text-xs font-bold text-slate-100">{title}</h3>
        <div className="flex flex-wrap gap-1" role="radiogroup" aria-label="Role">
          {roles.map((r) => (
            <button key={r} type="button" role="radio" aria-checked={v.role === r} className={cx('chip h-5 cursor-pointer px-1.5 text-[10px] uppercase', v.role === r ? 'bg-sky-500 text-white' : 'bg-slate-800 text-slate-300')} onClick={() => set({ role: r } as Partial<T>)}>{r}</button>
          ))}
        </div>
      </div>
      <div className="grid grid-cols-2 gap-x-2 gap-y-1 lg:grid-cols-4">
        {fields.map(([k, label, ph]) => (
          <label key={k} className={cx('flex flex-col gap-0.5 text-[11px] text-slate-400', (k === 'notes' || k === 'description' || k === 'address' || k === 'clothing') && 'col-span-2')}>
            {label}
            <input className="input h-7 text-xs" value={(v[k] as string | undefined) ?? ''} placeholder={ph} onChange={(e) => set({ [k]: e.target.value } as Partial<T>)} autoFocus={k === fields[0][0]} />
          </label>
        ))}
        {extra?.(v, set)}
      </div>
      <div className="mt-2 flex justify-end gap-1.5">
        <button type="button" className="btn h-7 text-xs" onClick={onCancel}>Cancel</button>
        <button className="btn btn-go h-7 text-xs">Save</button>
      </div>
    </form>
  )
}
