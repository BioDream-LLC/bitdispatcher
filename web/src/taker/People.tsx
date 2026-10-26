import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { AlertTriangle, Car, Copy, Link2, Pencil, Plus, Search, Send, ShieldAlert, Trash2, Truck, Users } from 'lucide-react'
import { api, ApiError, errText } from '../api'
import { cx } from '../util'
import type { Call, Incident, View } from '../types'
import type { Draft } from './draft'
import { CodeSelect, listOf, codeLabel, useCodes, useDebounced } from '../requests/codes'
import HitConfirm from '../requests/HitConfirm'
import { TowDialog } from '../requests/Tow'
import type { InqReturn, Person, PersonHit, PersonPhone, PersonQuery, SafetyConcern, Vehicle, VehicleLink } from '../requests/types'

type Props = { view: View; d: Draft; inc?: Incident; call?: Call; update: (p: Partial<Draft> | ((d: Draft) => Draft)) => void }

// The draft keeps people/vehicles in the shared (older) shape; the new optional fields ride along as the
// same JSON the Go side reads. We treat the draft lists as the local extended shape here.
const draftPersons = (d: Draft) => (d.persons as unknown as Person[]) ?? []
const draftVehicles = (d: Draft) => (d.vehicles as unknown as Vehicle[]) ?? []

const PHONE_TYPES = [
  ['mobile', 'Mobile'],
  ['home', 'Home'],
  ['work', 'Work'],
  ['other', 'Other'],
] as const
const PHONE_SOURCES = ['caller', 'ani', 'ali', 'records', 'other']
const RELIABILITY = [
  ['verified', 'Verified'],
  ['reported', 'Reported'],
  ['unconfirmed', 'Unconfirmed'],
] as const

/** fieldToInput maps an ApiError.field to the id of the input to focus. */
function focusField(field?: string) {
  if (!field) return
  const map: Record<string, string> = {
    first: 'pf-first',
    last: 'pf-last',
    dob: 'pf-dob',
    ssn: 'pf-ssn',
    phones: 'pf-phone-0',
    'safety.title': 'pf-safety-title',
    'safety.reason': 'pf-safety-reason',
    'safety.source': 'pf-safety-source',
    'safety.instruction': 'pf-safety-instruction',
    'safety.scope': 'pf-safety-scope',
    plate: 'vf-plate',
    state: 'vf-state',
  }
  const id = map[field] ?? field
  const el = document.getElementById(id)
  if (el) {
    el.focus()
    el.setAttribute('aria-invalid', 'true')
  }
}

function ageFromDob(dob?: string): number | null {
  if (!dob) return null
  const t = Date.parse(dob)
  if (isNaN(t)) return null
  const now = new Date()
  const b = new Date(dob)
  let age = now.getFullYear() - b.getFullYear()
  const m = now.getMonth() - b.getMonth()
  if (m < 0 || (m === 0 && now.getDate() < b.getDate())) age--
  return age >= 0 && age < 130 ? age : null
}

// ======================================================================= Persons

export function PersonsTab({ d, inc, update }: Props) {
  const codes = useCodes()
  const list = (inc ? ((inc.persons as unknown as Person[]) ?? []) : draftPersons(d)).filter((p) => !p.removed)
  const [edit, setEdit] = useState<Person | null>(null)
  const personRoles = codes?.personRoles ?? ['caller', 'victim', 'suspect', 'witness', 'complainant', 'subject', 'patient', 'other']

  const save = async (p: Person): Promise<boolean> => {
    // The same checks the server makes, before the call is sent too: an officer-safety concern says what,
    // why, from whom and what to do.
    if (p.safety && !p.removed) {
      const n = p.safetyNote
      const miss: [string, string][] = [['safety.title', 'Give the officer-safety alert a short title'], ['safety.reason', 'Say what the concern is and why it applies'],
        ['safety.source', 'Say how it was reported'], ['safety.instruction', 'Say what responders should do']]
      const got = { 'safety.title': n?.title, 'safety.reason': n?.reason, 'safety.source': n?.source, 'safety.instruction': n?.instruction } as Record<string, string | undefined>
      const m = miss.find(([f]) => !got[f]?.trim())
      if (m) {
        toast.error(m[1])
        focusField(m[0])
        return false
      }
    }
    try {
      if (inc) {
        if (p.id && !p.id.startsWith('draft-')) await api.put(`/incidents/${inc.number}/persons/${p.id}`, p)
        else await api.post(`/incidents/${inc.number}/persons`, { ...p, id: '' })
      } else {
        update((cur) => {
          const ps = draftPersons(cur)
          const next = p.id ? ps.map((x) => (x.id === p.id ? p : x)) : [...ps, { ...p, id: `draft-${ps.length + 1}` }]
          return { ...cur, persons: next as unknown as Draft['persons'] }
        })
      }
      setEdit(null)
      return true
    } catch (e) {
      toast.error(errText(e))
      if (e instanceof ApiError) focusField(e.field)
      return false
    }
  }

  const remove = (p: Person) => {
    if (inc) void save({ ...p, removed: true })
    else update((cur) => ({ ...cur, persons: draftPersons(cur).filter((x) => x.id !== p.id) as unknown as Draft['persons'] }))
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-1.5 text-sm font-semibold text-slate-100">
          <Users size={14} aria-hidden /> Persons
        </h2>
        <button className="btn btn-go ml-auto h-7 text-xs" onClick={() => setEdit({ id: '', role: list.length ? 'other' : 'caller', roles: [list.length ? 'other' : 'caller'], locKind: 'incident' })}>
          <Plus size={12} aria-hidden /> Add a person
        </button>
      </div>

      {list.length === 0 && !edit && <p className="text-xs text-slate-500">No one added. Add the caller, victims, suspects and witnesses as the call goes.</p>}

      <ul className="space-y-1.5">
        {list.map((p) => (
          <PersonCard key={p.id} p={p} inc={inc} codes={codes} onEdit={() => setEdit(p)} onRemove={() => remove(p)} />
        ))}
      </ul>

      {edit && <PersonForm value={edit} roles={personRoles} codes={codes} onCancel={() => setEdit(null)} onSave={save} />}

      <PeopleSearch codes={codes} />
    </div>
  )
}

function PersonCard({ p, inc, codes, onEdit, onRemove }: { p: Person; inc?: Incident; codes: ReturnType<typeof useCodes>; onEdit: () => void; onRemove: () => void }) {
  const [expand, setExpand] = useState(false)
  const n = p.safetyNote
  const roles = p.roles?.length ? p.roles : [p.role]
  const age = ageFromDob(p.dob)
  const desc = [
    p.sex && codeLabel(listOf(codes, 'sex'), p.sex),
    p.race && codeLabel(listOf(codes, 'race'), p.race),
    p.ethnicity && codeLabel(listOf(codes, 'ethnicity'), p.ethnicity),
    p.height,
    p.weight && `${p.weight} lb`,
    p.build,
    p.hair && `hair ${codeLabel(listOf(codes, 'hair'), p.hair)}`,
    p.eyes && `eyes ${codeLabel(listOf(codes, 'eyes'), p.eyes)}`,
    p.clothing,
    p.smt,
  ]
    .filter(Boolean)
    .join(' · ')
  return (
    <li className={cx('panel rounded-md px-2 py-1.5 text-xs', p.safety && 'border-rose-700')}>
      <div className="flex items-start gap-2">
        <div className="flex flex-col gap-0.5">
          {roles.map((r, i) => (
            <span key={r + i} className={cx('chip uppercase', r === 'suspect' ? 'bg-rose-500/20 text-rose-200' : 'bg-slate-800 text-slate-300')}>
              {r}
              {i === 0 && roles.length > 1 ? ' (primary)' : ''}
            </span>
          ))}
        </div>
        <div className="min-w-0 flex-1">
          <div className="font-semibold text-slate-100">{p.name || 'Unnamed'}</div>
          {desc && <div className="text-slate-400">{desc}</div>}
          <div className="text-slate-400">
            {[
              p.dob && `DOB ${p.dob}${age != null ? ` (age ${age})` : ''}`,
              !p.dob && p.approxAge && `about ${p.approxAge}`,
              p.ssn && `SSN ${p.ssn}`,
              p.dl && `DL ${p.dl}${p.dlState ? ' ' + p.dlState : ''}`,
              p.idNumber && `${p.idType ?? 'ID'} ${p.idNumber}${p.idState ? ' ' + p.idState : ''}`,
              p.phone,
              locText(p),
            ]
              .filter(Boolean)
              .join(' · ')}
          </div>
          {p.aliases?.length ? <div className="text-slate-500">aka {p.aliases.join(', ')}</div> : null}
          {p.notes && <div className="text-slate-300">{p.notes}</div>}
        </div>
        <button className="btn btn-ghost h-6 px-1" aria-label={`Change ${p.name || p.role}`} onClick={onEdit}>
          <Pencil size={12} aria-hidden />
        </button>
        <button className="btn btn-ghost h-6 px-1" aria-label={`Remove ${p.name || p.role}`} onClick={onRemove}>
          <Trash2 size={12} aria-hidden />
        </button>
      </div>

      {p.safety && n && (
        <div className="mt-1.5 rounded border border-rose-800 bg-rose-950/40 px-2 py-1 text-rose-100">
          <button className="flex w-full items-center gap-1 text-left font-bold" onClick={() => setExpand((x) => !x)} aria-expanded={expand}>
            <ShieldAlert size={12} aria-hidden /> OFFICER SAFETY: {n.title}
            <span className="ml-auto text-[10px] font-normal text-rose-300">{expand ? 'less' : 'more'}</span>
          </button>
          {expand && (
            <dl className="mt-1 space-y-0.5 text-[11px]">
              <div>
                <dt className="inline font-semibold">Reason: </dt>
                <dd className="inline">{n.reason}</dd>
              </div>
              {n.weapons && (
                <div>
                  <dt className="inline font-semibold">Weapons or threat: </dt>
                  <dd className="inline">{n.weapons}</dd>
                </div>
              )}
              <div>
                <dt className="inline font-semibold">Responders: </dt>
                <dd className="inline">{n.instruction}</dd>
              </div>
              <div>
                <dt className="inline font-semibold">Source: </dt>
                <dd className="inline">
                  {n.source}
                  {n.reliability ? ` (${n.reliability})` : ''}
                </dd>
              </div>
              <div>
                <dt className="inline font-semibold">Scope: </dt>
                <dd className="inline">{n.scope === 'permanent' ? 'proposed as a permanent warning (supervisor review)' : 'this incident only'}</dd>
              </div>
            </dl>
          )}
        </div>
      )}

      {inc && <PersonInquiries incident={inc.number} record={p.id} requester="" />}
    </li>
  )
}

function locText(p: Person): string {
  switch (p.locKind) {
    case 'other':
      return ['elsewhere', p.address, p.building && `bldg ${p.building}`, p.floor && `flr ${p.floor}`, p.unit && `unit ${p.unit}`].filter(Boolean).join(' ')
    case 'unknown':
      return 'location unknown'
    case 'transient':
      return 'transient'
    default:
      return p.address || ''
  }
}

function PersonForm({ value, roles, codes, onCancel, onSave }: {
  value: Person
  roles: string[]
  codes: ReturnType<typeof useCodes>
  onCancel: () => void
  onSave: (p: Person) => Promise<boolean>
}) {
  const [p, setP] = useState<Person>({ ...value, roles: value.roles?.length ? value.roles : [value.role] })
  const [matches, setMatches] = useState<PersonHit[]>([])
  const set = (patch: Partial<Person>) => setP((cur) => ({ ...cur, ...patch }))
  const age = ageFromDob(p.dob)

  // Possible matches while typing a name (debounced), never blocking saving.
  const matchKey = useDebounced(JSON.stringify([p.first, p.last, p.dob]), 500)
  useEffect(() => {
    if (!(p.first || p.last)) {
      setMatches([])
      return
    }
    api.post<{ items: PersonHit[] }>('/persons/matches', p).then((r) => setMatches(r.items)).catch(() => setMatches([]))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [matchKey])

  const toggleRole = (r: string) => {
    const has = p.roles?.includes(r)
    const next = has ? (p.roles ?? []).filter((x) => x !== r) : [...(p.roles ?? []), r]
    set({ roles: next, role: next[0] ?? r })
  }

  const phones = p.phones ?? []
  const setPhone = (i: number, patch: Partial<PersonPhone>) => set({ phones: phones.map((x, j) => (j === i ? { ...x, ...patch } : x)) })
  const addPhone = () => set({ phones: [...phones, { number: '', type: 'mobile', source: 'caller' }] })
  const removePhone = (i: number) => set({ phones: phones.filter((_, j) => j !== i) })

  const setSafety = (patch: Partial<SafetyConcern>) => {
    const base: SafetyConcern = p.safetyNote ?? { title: '', reason: '', instruction: '', source: '', scope: 'incident' }
    set({ safetyNote: { ...base, ...patch } })
  }

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    await onSave(p)
  }

  return (
    <form className="panel space-y-3 rounded-lg p-3" onSubmit={submit} aria-label={p.id ? 'Change person' : 'New person'}>
      <h3 className="text-xs font-bold text-slate-100">{p.id ? 'Change person' : 'New person'}</h3>

      {/* roles */}
      <fieldset>
        <legend className="label">Roles (first is primary)</legend>
        <div className="flex flex-wrap gap-1" role="group" aria-label="Roles">
          {roles.map((r) => (
            <button
              key={r}
              type="button"
              aria-pressed={p.roles?.includes(r)}
              className={cx('chip h-6 cursor-pointer px-1.5 text-[10px] uppercase', p.roles?.includes(r) ? 'bg-sky-500 text-white' : 'bg-slate-800 text-slate-300')}
              onClick={() => toggleRole(r)}
            >
              {r}
            </button>
          ))}
        </div>
      </fieldset>

      {/* name */}
      <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400" htmlFor="pf-first">
          First name
          <input id="pf-first" className="input h-7 text-xs" value={p.first ?? ''} onChange={(e) => set({ first: e.target.value })} autoFocus />
        </label>
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400" htmlFor="pf-middle">
          Middle name
          <input id="pf-middle" className="input h-7 text-xs" value={p.middle ?? ''} onChange={(e) => set({ middle: e.target.value })} />
        </label>
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400" htmlFor="pf-last">
          Last name
          <input id="pf-last" className="input h-7 text-xs" value={p.last ?? ''} onChange={(e) => set({ last: e.target.value })} placeholder="DOE" />
        </label>
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400" htmlFor="pf-suffix">
          Suffix
          <input id="pf-suffix" className="input h-7 text-xs" value={p.suffix ?? ''} onChange={(e) => set({ suffix: e.target.value })} placeholder="JR, SR, III" />
        </label>
        <label className="col-span-2 flex flex-col gap-0.5 text-[11px] text-slate-400" htmlFor="pf-aliases">
          Aliases (comma separated)
          <input id="pf-aliases" className="input h-7 text-xs" value={(p.aliases ?? []).join(', ')} onChange={(e) => set({ aliases: e.target.value.split(',').map((x) => x.trim()).filter(Boolean) })} />
        </label>
      </div>

      <Matches matches={matches} onCopy={(h) => set({ first: h.first ?? p.first, middle: h.middle ?? p.middle, last: h.last ?? p.last, dob: h.dob || p.dob, sex: h.sex || p.sex, race: h.race || p.race, address: h.address || p.address, phone: h.phone || p.phone })} />

      {/* location */}
      <fieldset>
        <legend className="label">Where the person is</legend>
        <div className="flex flex-wrap gap-1" role="radiogroup" aria-label="Where the person is">
          {([['incident', 'Same as incident'], ['other', 'Somewhere else'], ['unknown', 'Unknown'], ['transient', 'Transient']] as const).map(([v, lbl]) => (
            <button key={v} type="button" role="radio" aria-checked={(p.locKind ?? 'incident') === v} className={cx('chip h-6 cursor-pointer px-1.5 text-[10px]', (p.locKind ?? 'incident') === v ? 'bg-sky-500 text-white' : 'bg-slate-800 text-slate-300')} onClick={() => set({ locKind: v })}>
              {lbl}
            </button>
          ))}
        </div>
        {p.locKind === 'other' && (
          <div className="mt-1 grid grid-cols-2 gap-2 lg:grid-cols-4">
            <label className="col-span-2 flex flex-col gap-0.5 text-[11px] text-slate-400">
              Address
              <input className="input h-7 text-xs" value={p.address ?? ''} onChange={(e) => set({ address: e.target.value })} />
            </label>
            <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
              Building
              <input className="input h-7 text-xs" value={p.building ?? ''} onChange={(e) => set({ building: e.target.value })} />
            </label>
            <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
              Floor
              <input className="input h-7 text-xs" value={p.floor ?? ''} onChange={(e) => set({ floor: e.target.value })} />
            </label>
            <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
              Unit
              <input className="input h-7 text-xs" value={p.unit ?? ''} onChange={(e) => set({ unit: e.target.value })} />
            </label>
          </div>
        )}
      </fieldset>

      {/* telephones */}
      <fieldset>
        <legend className="label">Telephone numbers</legend>
        <div className="space-y-1">
          {phones.map((ph, i) => (
            <div key={i} className="grid grid-cols-2 gap-1 lg:grid-cols-6">
              <label className="flex flex-col gap-0.5 text-[10px] text-slate-500" htmlFor={`pf-phone-${i}`}>
                Number
                <input id={`pf-phone-${i}`} className="input h-7 text-xs" value={ph.number} onChange={(e) => setPhone(i, { number: e.target.value })} />
              </label>
              <label className="flex flex-col gap-0.5 text-[10px] text-slate-500">
                Type
                <select className="input h-7 text-xs" value={ph.type ?? 'mobile'} onChange={(e) => setPhone(i, { type: e.target.value })}>
                  {PHONE_TYPES.map(([v, l]) => (
                    <option key={v} value={v}>
                      {l}
                    </option>
                  ))}
                </select>
              </label>
              <label className="flex flex-col gap-0.5 text-[10px] text-slate-500">
                Ext
                <input className="input h-7 text-xs" value={ph.ext ?? ''} onChange={(e) => setPhone(i, { ext: e.target.value })} />
              </label>
              <label className="flex flex-col gap-0.5 text-[10px] text-slate-500">
                Source
                <select className="input h-7 text-xs" value={ph.source ?? 'caller'} onChange={(e) => setPhone(i, { source: e.target.value })}>
                  {PHONE_SOURCES.map((s) => (
                    <option key={s} value={s}>
                      {s}
                    </option>
                  ))}
                </select>
              </label>
              <label className="flex items-end gap-1 text-[11px] text-slate-400">
                <input type="checkbox" checked={!!ph.verified} onChange={(e) => setPhone(i, { verified: e.target.checked })} /> Verified
              </label>
              <button type="button" className="btn btn-ghost h-7 self-end px-1 text-[10px]" onClick={() => removePhone(i)} aria-label={`Remove telephone ${i + 1}`}>
                <Trash2 size={11} aria-hidden /> Remove
              </button>
            </div>
          ))}
          <button type="button" className="btn h-7 text-xs" onClick={addPhone}>
            <Plus size={11} aria-hidden /> Add a telephone number
          </button>
        </div>
      </fieldset>

      {/* physical description */}
      <fieldset>
        <legend className="label">Physical description</legend>
        <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
          <CodeSelect id="pf-sex" label="Sex" list={listOf(codes, 'sex')} value={p.sex} onChange={(v) => set({ sex: v })} />
          <CodeSelect id="pf-race" label="Race" list={listOf(codes, 'race')} value={p.race} onChange={(v) => set({ race: v })} />
          <CodeSelect id="pf-ethnicity" label="Ethnicity" list={listOf(codes, 'ethnicity')} value={p.ethnicity} onChange={(v) => set({ ethnicity: v })} />
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
            Height
            <input className="input h-7 text-xs" value={p.height ?? ''} onChange={(e) => set({ height: e.target.value })} placeholder={`5'10"`} />
          </label>
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
            Weight
            <input className="input h-7 text-xs" value={p.weight ?? ''} onChange={(e) => set({ weight: e.target.value })} placeholder="180" />
          </label>
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
            Build
            <input className="input h-7 text-xs" value={p.build ?? ''} onChange={(e) => set({ build: e.target.value })} />
          </label>
          <CodeSelect id="pf-hair" label="Hair" list={listOf(codes, 'hair')} value={p.hair} onChange={(v) => set({ hair: v })} />
          <CodeSelect id="pf-eyes" label="Eyes" list={listOf(codes, 'eyes')} value={p.eyes} onChange={(v) => set({ eyes: v })} />
          <label className="col-span-2 flex flex-col gap-0.5 text-[11px] text-slate-400">
            Clothing
            <input className="input h-7 text-xs" value={p.clothing ?? ''} onChange={(e) => set({ clothing: e.target.value })} placeholder="black hoodie, jeans" />
          </label>
          <label className="col-span-2 flex flex-col gap-0.5 text-[11px] text-slate-400">
            Scars, marks and tattoos
            <input className="input h-7 text-xs" value={p.smt ?? ''} onChange={(e) => set({ smt: e.target.value })} />
          </label>
        </div>
      </fieldset>

      {/* identifiers */}
      <fieldset>
        <legend className="label">Identifiers</legend>
        <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
          <Identifiers p={p} set={set} />
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400" htmlFor="pf-dl">
            Operator licence number
            <input id="pf-dl" className="input h-7 text-xs" value={p.dl ?? ''} onChange={(e) => set({ dl: e.target.value })} />
          </label>
          <CodeSelect id="pf-dlstate" label="Licence state" list={listOf(codes, 'state')} value={p.dlState} onChange={(v) => set({ dlState: v })} />
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
            Other ID type
            <input className="input h-7 text-xs" value={p.idType ?? ''} onChange={(e) => set({ idType: e.target.value })} placeholder="passport, state ID" />
          </label>
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
            Other ID number
            <input className="input h-7 text-xs" value={p.idNumber ?? ''} onChange={(e) => set({ idNumber: e.target.value })} />
          </label>
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400" htmlFor="pf-dob">
            Date of birth
            <input id="pf-dob" type="date" className="input h-7 text-xs" value={p.dob && /^\d{4}-\d{2}-\d{2}$/.test(p.dob) ? p.dob : ''} onChange={(e) => set({ dob: e.target.value })} />
          </label>
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
            Approximate age {age != null ? <span className="text-slate-500">(from DOB: {age})</span> : null}
            <input className="input h-7 text-xs" value={p.approxAge ?? ''} onChange={(e) => set({ approxAge: e.target.value })} placeholder="about 30" />
          </label>
        </div>
      </fieldset>

      {/* officer safety */}
      <div>
        <label className="flex items-center gap-1.5 text-xs text-rose-200">
          <input type="checkbox" checked={!!p.safety} onChange={(e) => set({ safety: e.target.checked, safetyNote: e.target.checked ? p.safetyNote ?? { title: '', reason: '', instruction: '', source: '', scope: 'incident' } : undefined })} />
          <ShieldAlert size={12} aria-hidden /> Officer-safety concern about this person
        </label>
        {p.safety && (
          <div className="mt-1 grid grid-cols-1 gap-2 rounded border border-rose-800 bg-rose-950/20 p-2 lg:grid-cols-2">
            <label className="flex flex-col gap-0.5 text-[11px] text-rose-200" htmlFor="pf-safety-title">
              Alert title
              <input id="pf-safety-title" className="input h-7 text-xs" value={p.safetyNote?.title ?? ''} onChange={(e) => setSafety({ title: e.target.value })} />
            </label>
            <label className="flex flex-col gap-0.5 text-[11px] text-rose-200" htmlFor="pf-safety-source">
              Source (how it was reported)
              <input id="pf-safety-source" className="input h-7 text-xs" value={p.safetyNote?.source ?? ''} onChange={(e) => setSafety({ source: e.target.value })} />
            </label>
            <label className="col-span-1 flex flex-col gap-0.5 text-[11px] text-rose-200 lg:col-span-2" htmlFor="pf-safety-reason">
              Reason (what happened and why it is a concern)
              <textarea id="pf-safety-reason" className="input min-h-[44px] text-xs" value={p.safetyNote?.reason ?? ''} onChange={(e) => setSafety({ reason: e.target.value })} />
            </label>
            <label className="flex flex-col gap-0.5 text-[11px] text-rose-200">
              Weapons or threat
              <input className="input h-7 text-xs" value={p.safetyNote?.weapons ?? ''} onChange={(e) => setSafety({ weapons: e.target.value })} />
            </label>
            <label className="flex flex-col gap-0.5 text-[11px] text-rose-200" htmlFor="pf-safety-instruction">
              Responder instruction
              <input id="pf-safety-instruction" className="input h-7 text-xs" value={p.safetyNote?.instruction ?? ''} onChange={(e) => setSafety({ instruction: e.target.value })} />
            </label>
            <label className="flex flex-col gap-0.5 text-[11px] text-rose-200">
              Reliability
              <select className="input h-7 text-xs" value={p.safetyNote?.reliability ?? ''} onChange={(e) => setSafety({ reliability: e.target.value })}>
                <option value="">Choose reliability</option>
                {RELIABILITY.map(([v, l]) => (
                  <option key={v} value={v}>
                    {l}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-0.5 text-[11px] text-rose-200" htmlFor="pf-safety-scope">
              Scope
              <select id="pf-safety-scope" className="input h-7 text-xs" value={p.safetyNote?.scope ?? 'incident'} onChange={(e) => setSafety({ scope: e.target.value })}>
                <option value="incident">This incident only</option>
                <option value="permanent">Propose a permanent warning (supervisor review)</option>
              </select>
            </label>
          </div>
        )}
      </div>

      <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
        Notes
        <textarea className="input min-h-[44px] text-xs" value={p.notes ?? ''} onChange={(e) => set({ notes: e.target.value })} />
      </label>

      <div className="flex justify-end gap-1.5">
        <button type="button" className="btn h-7 text-xs" onClick={onCancel}>
          Cancel
        </button>
        <button className="btn btn-go h-7 text-xs">Save</button>
      </div>
    </form>
  )
}

// The Social Security number: masked once saved, with an 'Open (recorded)' button for those allowed.
function Identifiers({ p, set }: { p: Person; set: (patch: Partial<Person>) => void }) {
  // When saved (not a draft id) and the value looks masked, offer to open it; otherwise edit freely.
  const masked = /\*/.test(p.ssn ?? '')
  return (
    <label className="flex flex-col gap-0.5 text-[11px] text-slate-400" htmlFor="pf-ssn">
      Social Security number
      <input id="pf-ssn" className="input h-7 text-xs" value={p.ssn ?? ''} onChange={(e) => set({ ssn: e.target.value })} placeholder="123-45-6789" readOnly={masked} aria-readonly={masked} />
      {masked && <span className="text-[10px] text-slate-500">Shown masked. Open it from the saved record below.</span>}
    </label>
  )
}

function Matches({ matches, onCopy }: { matches: PersonHit[]; onCopy: (h: PersonHit) => void }) {
  if (matches.length === 0) return null
  return (
    <div className="rounded border border-sky-900 bg-sky-950/30 p-2">
      <div className="mb-1 flex items-center gap-1 text-[11px] font-semibold text-sky-200">
        <Link2 size={12} aria-hidden /> Possible same person ({matches.length}) — linking or copying does not block saving
      </div>
      <ul className="space-y-1">
        {matches.map((h) => (
          <li key={h.key} className="flex items-start gap-2 text-[11px]">
            <div className="min-w-0 flex-1">
              <span className="font-semibold text-slate-100">{h.name}</span>
              <span className="text-slate-400"> {[h.dob && `DOB ${h.dob}`, h.sex, h.race, h.address].filter(Boolean).join(' · ')}</span>
              {h.flags?.length ? <span className="ml-1 text-rose-300">⚠ {h.flags.join(', ')}</span> : null}
              <span className="text-slate-500"> — {h.incidents.length} incident(s); matched {h.why.join(', ')}</span>
            </div>
            <button type="button" className="btn btn-ghost h-6 px-1 text-[10px]" onClick={() => onCopy(h)}>
              <Copy size={11} aria-hidden /> Copy in
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}

/** PersonInquiries shows the person's returns from the protected endpoint and a Run (QW) button. */
function PersonInquiries({ incident, record, requester }: { incident: string; record: string; requester: string }) {
  const [returns, setReturns] = useState<InqReturn[] | null>(null)
  const [allowed, setAllowed] = useState(true)
  const [reqUnit, setReqUnit] = useState(requester)
  const [running, setRunning] = useState(false)
  const [hit, setHit] = useState<InqReturn | null>(null)
  const draft = record.startsWith('draft-')

  const load = () => {
    if (draft) return
    api
      .get<{ returns: InqReturn[]; discrepancies: string[] }>(`/incidents/${incident}/records/${record}/returns`)
      .then((r) => {
        setReturns(r.returns)
        setAllowed(true)
      })
      .catch((e) => {
        if (e instanceof ApiError && e.status === 403) setAllowed(false)
        setReturns([])
      })
  }
  useEffect(load, [incident, record, draft])

  if (draft) {
    return <p className="mt-1 text-[10px] text-amber-300">Send the call first: inquiries run once the incident exists.</p>
  }

  const run = async () => {
    setRunning(true)
    try {
      await api.post(`/incidents/${incident}/records/${record}/run`, { key: 'QW', requester: reqUnit.trim() || undefined })
      toast.success('Ran QW: the return is attached to this record')
      load()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setRunning(false)
    }
  }

  return (
    <div className="mt-1.5 border-t border-slate-800 pt-1.5">
      <div className="flex items-center gap-1">
        <input className="input h-6 w-24 text-[11px]" value={reqUnit} onChange={(e) => setReqUnit(e.target.value)} placeholder="requester" aria-label="Requester unit" />
        <button className="btn h-6 text-[11px]" onClick={run} disabled={running}>
          <Send size={11} aria-hidden /> Run (QW)
        </button>
      </div>
      {!allowed && <p className="mt-1 text-[10px] text-slate-500">Your duty cannot open inquiry returns.</p>}
      {allowed && returns && <ReturnsList returns={returns} onHit={setHit} />}
      {hit && <HitConfirm incident={incident} record={record} ret={hit} requester={reqUnit} open onClose={() => setHit(null)} onAnswered={load} />}
    </div>
  )
}

// ======================================================================= People search panel

function PeopleSearch({ codes }: { codes: ReturnType<typeof useCodes> }) {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState<PersonQuery>({ phonetic: false, partial: false })
  const [hits, setHits] = useState<PersonHit[] | null>(null)
  const [refused, setRefused] = useState(false)
  const set = (patch: Partial<PersonQuery>) => setQ((cur) => ({ ...cur, ...patch }))

  const search = async () => {
    const body: PersonQuery = { ...q }
    // Only send identifiers when entered.
    if (!body.ssn?.trim()) delete body.ssn
    if (!body.dl?.trim()) delete body.dl
    if (!body.age) delete body.age
    try {
      const r = await api.post<{ items: PersonHit[] }>('/persons/search', body)
      setHits(r.items)
      setRefused(false)
    } catch (e) {
      if (e instanceof ApiError && e.status === 403) {
        setRefused(true)
        setHits([])
      }
      toast.error(errText(e))
    }
  }

  return (
    <div className="rounded-md border border-slate-800">
      <button className="panel-head flex w-full items-center gap-1.5" onClick={() => setOpen((x) => !x)} aria-expanded={open}>
        <Search size={12} aria-hidden /> Search people
        <span className="ml-auto text-[10px] text-slate-500">{open ? 'hide' : 'show'}</span>
      </button>
      {open && (
        <div className="space-y-2 p-2">
          <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
            <L label="First">
              <input className="input h-7 text-xs" value={q.first ?? ''} onChange={(e) => set({ first: e.target.value })} />
            </L>
            <L label="Middle">
              <input className="input h-7 text-xs" value={q.middle ?? ''} onChange={(e) => set({ middle: e.target.value })} />
            </L>
            <L label="Last">
              <input className="input h-7 text-xs" value={q.last ?? ''} onChange={(e) => set({ last: e.target.value })} />
            </L>
            <L label="Date of birth">
              <input type="date" className="input h-7 text-xs" value={q.dob ?? ''} onChange={(e) => set({ dob: e.target.value })} />
            </L>
            <L label="Approximate age">
              <input type="number" className="input h-7 text-xs" value={q.age ?? ''} onChange={(e) => set({ age: e.target.value ? Number(e.target.value) : undefined })} />
            </L>
            <L label="Phone">
              <input className="input h-7 text-xs" value={q.phone ?? ''} onChange={(e) => set({ phone: e.target.value })} />
            </L>
            <L label="Address">
              <input className="input h-7 text-xs" value={q.address ?? ''} onChange={(e) => set({ address: e.target.value })} />
            </L>
            <L label="Licence number">
              <input className="input h-7 text-xs" value={q.dl ?? ''} onChange={(e) => set({ dl: e.target.value })} />
            </L>
            <L label="Social Security number">
              <input className="input h-7 text-xs" value={q.ssn ?? ''} onChange={(e) => set({ ssn: e.target.value })} />
            </L>
            <CodeSelect id="ps-sex" label="Sex" list={listOf(codes, 'sex')} value={q.sex} onChange={(v) => set({ sex: v })} />
            <CodeSelect id="ps-race" label="Race" list={listOf(codes, 'race')} value={q.race} onChange={(v) => set({ race: v })} />
          </div>
          <div className="flex items-center gap-3">
            <label className="flex items-center gap-1 text-[11px] text-slate-400">
              <input type="checkbox" checked={!!q.phonetic} onChange={(e) => set({ phonetic: e.target.checked })} /> Phonetic (sounds like)
            </label>
            <label className="flex items-center gap-1 text-[11px] text-slate-400">
              <input type="checkbox" checked={!!q.partial} onChange={(e) => set({ partial: e.target.checked })} /> Partial (matches the start)
            </label>
            <button className="btn btn-go ml-auto h-7 text-xs" onClick={search}>
              <Search size={12} aria-hidden /> Search
            </button>
          </div>
          {refused && <p className="text-[11px] text-rose-300">Your duty cannot search by Social Security or licence number.</p>}
          {hits && hits.length === 0 && !refused && <p className="text-[11px] text-slate-500">No one found.</p>}
          {hits && hits.length > 0 && (
            <ul className="space-y-1">
              {hits.map((h) => (
                <li key={h.key} className="rounded border border-slate-800 px-2 py-1 text-[11px]">
                  <span className="font-semibold text-slate-100">{h.name}</span>
                  <span className="text-slate-400"> {[h.dob && `DOB ${h.dob}`, h.sex, h.race, h.address, h.phone, h.ssn && `SSN ${h.ssn}`, h.dl && `DL ${h.dl}`].filter(Boolean).join(' · ')}</span>
                  {h.flags?.length ? <span className="ml-1 text-rose-300">⚠ {h.flags.join(', ')}</span> : null}
                  <span className="text-slate-500"> — matched {h.why.join(', ')}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  )
}

function L({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
      {label}
      {children}
    </label>
  )
}

// ======================================================================= returns list (shared)

function ReturnsList({ returns, onHit }: {
  returns: InqReturn[]
  onHit: (r: InqReturn) => void
}) {
  if (returns.length === 0) return <p className="mt-1 text-[10px] text-slate-500">No returns yet.</p>
  return (
    <ul className="mt-1 space-y-1">
      {returns.map((r) => (
        <li key={r.id} className={cx('rounded border px-2 py-1 text-[11px]', r.superseded ? 'border-slate-800 text-slate-500' : 'border-slate-700 text-slate-300')}>
          <div className="flex items-center gap-1">
            <span className="font-mono">{r.id}</span>
            <span className="chip h-4 bg-slate-800 px-1 text-[9px] uppercase">{r.source}</span>
            {r.simulated && <span className="chip h-4 bg-amber-500/20 px-1 text-[9px] text-amber-200">simulated</span>}
            {r.superseded && <span className="chip h-4 bg-slate-800 px-1 text-[9px]">superseded</span>}
            {r.hit && (
              <span className={cx('chip h-4 px-1 text-[9px] font-bold', r.confirmed === 'confirmed-valid' ? 'bg-rose-500 text-white' : 'bg-amber-500/30 text-amber-100')}>
                {r.confirmed === 'confirmed-valid' ? `CONFIRMED HIT: ${r.hit}` : `POSSIBLE HIT: not confirmed (${r.hit})`}
              </span>
            )}
          </div>
          {r.fields && Object.keys(r.fields).length > 0 && (
            <div className="mt-0.5 text-slate-400">
              {Object.entries(r.fields)
                .filter(([k]) => k !== 'noRecord')
                .map(([k, v]) => `${k}: ${v}`)
                .join(' · ')}
            </div>
          )}
          {r.hit && !r.superseded && (
            <button className="btn btn-ghost mt-0.5 h-5 px-1 text-[10px]" onClick={() => onHit(r)}>
              <ShieldAlert size={10} aria-hidden /> Hit confirmation
            </button>
          )}
        </li>
      ))}
    </ul>
  )
}

// ======================================================================= Vehicles

export function VehiclesTab({ d, inc, update }: Props) {
  const codes = useCodes()
  const list = (inc ? ((inc.vehicles as unknown as Vehicle[]) ?? []) : draftVehicles(d)).filter((v) => !v.removed)
  const persons = (inc ? ((inc.persons as unknown as Person[]) ?? []) : draftPersons(d)).filter((p) => !p.removed)
  const [edit, setEdit] = useState<Vehicle | null>(null)
  const [tow, setTow] = useState<string | null>(null)
  const vehicleRoles = codes?.vehicleRoles ?? ['suspect', 'victim', 'involved', 'abandoned', 'other']

  const save = async (v: Vehicle): Promise<boolean> => {
    try {
      if (inc) {
        if (v.id && !v.id.startsWith('draft-')) await api.put(`/incidents/${inc.number}/vehicles/${v.id}`, v)
        else await api.post(`/incidents/${inc.number}/vehicles`, { ...v, id: '' })
      } else {
        update((cur) => {
          const vs = draftVehicles(cur)
          const next = v.id ? vs.map((x) => (x.id === v.id ? v : x)) : [...vs, { ...v, id: `draft-${vs.length + 1}` }]
          return { ...cur, vehicles: next as unknown as Draft['vehicles'] }
        })
      }
      setEdit(null)
      return true
    } catch (e) {
      toast.error(errText(e))
      if (e instanceof ApiError) focusField(e.field)
      return false
    }
  }
  const remove = (v: Vehicle) => {
    if (inc) void save({ ...v, removed: true })
    else update((cur) => ({ ...cur, vehicles: draftVehicles(cur).filter((x) => x.id !== v.id) as unknown as Draft['vehicles'] }))
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-1.5 text-sm font-semibold text-slate-100">
          <Car size={14} aria-hidden /> Vehicles
        </h2>
        <button className="btn btn-go ml-auto h-7 text-xs" onClick={() => setEdit({ id: '', role: 'involved', roles: ['involved'], state: codes?.defaultState })}>
          <Plus size={12} aria-hidden /> Add a vehicle
        </button>
      </div>

      {list.length === 0 && !edit && <p className="text-xs text-slate-500">No vehicle added.</p>}

      <ul className="space-y-1.5">
        {list.map((v) => (
          <VehicleCard key={v.id} v={v} inc={inc} persons={persons} codes={codes} onEdit={() => setEdit(v)} onRemove={() => remove(v)} onTow={() => setTow(v.id)} />
        ))}
      </ul>

      {edit && <VehicleForm value={edit} roles={vehicleRoles} persons={persons} codes={codes} inc={inc} onCancel={() => setEdit(null)} onSave={save} />}

      {tow && inc && <TowDialog incident={inc.number} vehicle={tow} open onClose={() => setTow(null)} />}
    </div>
  )
}

function VehicleCard({ v, inc, persons, codes, onEdit, onRemove, onTow }: {
  v: Vehicle
  inc?: Incident
  persons: Person[]
  codes: ReturnType<typeof useCodes>
  onEdit: () => void
  onRemove: () => void
  onTow: () => void
}) {
  const draft = !inc || v.id.startsWith('draft-')
  const roles = v.roles?.length ? v.roles : [v.role]
  const title = [codeLabel(listOf(codes, 'color'), v.color), v.year, codeLabel(listOf(codes, 'make'), v.make), v.model, codeLabel(listOf(codes, 'body'), v.body)].filter(Boolean).join(' ') || v.description || 'Vehicle'
  const linkedPeople = (v.links ?? []).map((l) => {
    const p = persons.find((x) => x.id === l.person)
    return `${l.relation} ${p?.name || l.person}`
  })
  return (
    <li className="panel rounded-md px-2 py-1.5 text-xs">
      <div className="flex items-start gap-2">
        <div className="flex flex-col gap-0.5">
          {roles.map((r, i) => (
            <span key={r + i} className={cx('chip uppercase', r === 'suspect' ? 'bg-rose-500/20 text-rose-200' : 'bg-slate-800 text-slate-300')}>
              {r}
              {i === 0 && roles.length > 1 ? ' (primary)' : ''}
            </span>
          ))}
        </div>
        <div className="min-w-0 flex-1">
          <div className="font-semibold text-slate-100">
            {title}
            {v.plate && (
              <span className="ml-1.5 font-mono text-amber-200">
                {v.state} {v.plate}
              </span>
            )}
            {v.plateType && <span className="ml-1 text-slate-500">{codeLabel(listOf(codes, 'plateType'), v.plateType)}</span>}
          </div>
          <div className="text-slate-400">
            {[v.vin && `VIN ${v.vin}`, v.vinNote, v.direction && `going ${v.direction}`, v.damage, v.description, linkedPeople.join(', ')].filter(Boolean).join(' · ')}
          </div>
          {v.notes && <div className="text-slate-300">{v.notes}</div>}
        </div>
        <button className="btn btn-ghost h-6 px-1" aria-label="Tow this vehicle" onClick={onTow} disabled={draft} title={draft ? 'Send the call first' : 'Tow this vehicle'}>
          <Truck size={12} aria-hidden />
        </button>
        <button className="btn btn-ghost h-6 px-1" aria-label="Change vehicle" onClick={onEdit}>
          <Pencil size={12} aria-hidden />
        </button>
        <button className="btn btn-ghost h-6 px-1" aria-label="Remove vehicle" onClick={onRemove}>
          <Trash2 size={12} aria-hidden />
        </button>
      </div>
      {inc && !draft && <VehicleInquiries incident={inc.number} record={v.id} />}
    </li>
  )
}

function VehicleForm({ value, roles, persons, codes, inc, onCancel, onSave }: {
  value: Vehicle
  roles: string[]
  persons: Person[]
  codes: ReturnType<typeof useCodes>
  inc?: Incident
  onCancel: () => void
  onSave: (v: Vehicle) => Promise<boolean>
}) {
  const [v, setV] = useState<Vehicle>({ ...value, roles: value.roles?.length ? value.roles : [value.role] })
  const [vinCheck, setVinCheck] = useState<string>('')
  const [dups, setDups] = useState<string[]>([])
  const set = (patch: Partial<Vehicle>) => setV((cur) => ({ ...cur, ...patch }))
  const draft = !inc || v.id.startsWith('draft-')

  const toggleRole = (r: string) => {
    const has = v.roles?.includes(r)
    const next = has ? (v.roles ?? []).filter((x) => x !== r) : [...(v.roles ?? []), r]
    set({ roles: next, role: next[0] ?? r })
  }

  // Live VIN check (debounced), never blocking a partial VIN.
  const vin = useDebounced(v.vin ?? '', 500)
  useEffect(() => {
    if (!vin.trim()) {
      setVinCheck('')
      return
    }
    api
      .post<{ complete: boolean; valid: boolean; year?: string; problems?: string[] }>('/vin/check', { vin })
      .then((c) => setVinCheck(c.valid ? `valid${c.year ? ', model year ' + c.year : ''}` : (c.problems ?? []).join('; ') || 'checked'))
      .catch(() => setVinCheck(''))
  }, [vin])

  // Duplicates after plate entry (non-blocking notice).
  const plateKey = useDebounced(JSON.stringify([v.plate, v.state, v.vin]), 500)
  useEffect(() => {
    if (draft || (!v.plate && !v.vin)) {
      setDups([])
      return
    }
    api.post<{ items: string[] }>(`/incidents/${inc!.number}/vehicles/duplicates`, v).then((r) => setDups(r.items)).catch(() => setDups([]))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [plateKey])

  const links = v.links ?? []
  const setLink = (i: number, patch: Partial<VehicleLink>) => set({ links: links.map((x, j) => (j === i ? { ...x, ...patch } : x)) })
  const addLink = () => set({ links: [...links, { person: persons[0]?.id ?? '', relation: 'driver' }] })
  const removeLink = (i: number) => set({ links: links.filter((_, j) => j !== i) })

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    await onSave(v)
  }

  return (
    <form className="panel space-y-3 rounded-lg p-3" onSubmit={submit} aria-label={v.id ? 'Change vehicle' : 'New vehicle'}>
      <h3 className="text-xs font-bold text-slate-100">{v.id ? 'Change vehicle' : 'New vehicle'}</h3>

      <fieldset>
        <legend className="label">Roles (first is primary)</legend>
        <div className="flex flex-wrap gap-1" role="group" aria-label="Roles">
          {roles.map((r) => (
            <button key={r} type="button" aria-pressed={v.roles?.includes(r)} className={cx('chip h-6 cursor-pointer px-1.5 text-[10px] uppercase', v.roles?.includes(r) ? 'bg-sky-500 text-white' : 'bg-slate-800 text-slate-300')} onClick={() => toggleRole(r)}>
              {r}
            </button>
          ))}
        </div>
      </fieldset>

      {/* plate and VIN */}
      <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400" htmlFor="vf-plate">
          Plate
          <input id="vf-plate" className="input h-7 text-xs" value={v.plate ?? ''} onChange={(e) => set({ plate: e.target.value.toUpperCase() })} placeholder="KTR4471" />
        </label>
        <CodeSelect id="vf-state" label="Plate state" list={listOf(codes, 'state')} value={v.state} onChange={(x) => set({ state: x })} />
        <CodeSelect id="vf-platetype" label="Plate type" list={listOf(codes, 'plateType')} value={v.plateType} onChange={(x) => set({ plateType: x })} />
        <CodeSelect id="vf-platecond" label="Plate condition" list={listOf(codes, 'plateCondition')} value={v.plateCondition} onChange={(x) => set({ plateCondition: x })} />
        <label className="col-span-2 flex flex-col gap-0.5 text-[11px] text-slate-400" htmlFor="vf-vin">
          VIN
          <input id="vf-vin" className="input h-7 text-xs" value={v.vin ?? ''} onChange={(e) => set({ vin: e.target.value.toUpperCase() })} />
          {vinCheck && <span className={cx('text-[10px]', /valid/.test(vinCheck) ? 'text-emerald-300' : 'text-amber-300')}>VIN check: {vinCheck}</span>}
        </label>
      </div>

      {dups.length > 0 && (
        <div className="rounded border border-amber-800 bg-amber-950/30 px-2 py-1 text-[11px] text-amber-100">
          <AlertTriangle size={11} className="mr-1 inline" aria-hidden /> Possible duplicate (a notice, not a block): {dups.join('; ')}
        </div>
      )}

      <RunButtons v={v} inc={inc} />

      {/* reported vs official */}
      <fieldset>
        <legend className="label">Reported description (official return shown beside it below)</legend>
        <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
            Year
            <input className="input h-7 text-xs" value={v.year ?? ''} onChange={(e) => set({ year: e.target.value })} />
          </label>
          <CodeSelect id="vf-make" label="Make" list={listOf(codes, 'make')} value={v.make} onChange={(x) => set({ make: x })} />
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
            Model
            <input className="input h-7 text-xs" value={v.model ?? ''} onChange={(e) => set({ model: e.target.value })} />
          </label>
          <CodeSelect id="vf-body" label="Body" list={listOf(codes, 'body')} value={v.body} onChange={(x) => set({ body: x })} />
          <CodeSelect id="vf-color" label="Color" list={listOf(codes, 'color')} value={v.color} onChange={(x) => set({ color: x })} />
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
            Owner (reported)
            <input className="input h-7 text-xs" value={v.owner ?? ''} onChange={(e) => set({ owner: e.target.value })} />
          </label>
        </div>
      </fieldset>

      {/* related people */}
      <fieldset>
        <legend className="label">Related people on this incident</legend>
        {persons.length === 0 && <p className="text-[11px] text-slate-500">Add people in the Persons tab first to link a driver or owner.</p>}
        <div className="space-y-1">
          {links.map((l, i) => (
            <div key={i} className="flex gap-1">
              <select className="input h-7 flex-1 text-xs" value={l.person} onChange={(e) => setLink(i, { person: e.target.value })} aria-label="Person">
                <option value="">Choose a person</option>
                {persons.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.id} {p.name || p.role}
                  </option>
                ))}
              </select>
              <select className="input h-7 w-28 text-xs" value={l.relation} onChange={(e) => setLink(i, { relation: e.target.value })} aria-label="Relation">
                {(codes?.relations ?? ['driver', 'owner', 'lessee', 'passenger', 'occupant', 'other']).map((r) => (
                  <option key={r} value={r}>
                    {r}
                  </option>
                ))}
              </select>
              <button type="button" className="btn btn-ghost h-7 px-1 text-[10px]" onClick={() => removeLink(i)} aria-label={`Remove link ${i + 1}`}>
                <Trash2 size={11} aria-hidden />
              </button>
            </div>
          ))}
          {persons.length > 0 && (
            <button type="button" className="btn h-7 text-xs" onClick={addLink}>
              <Plus size={11} aria-hidden /> Link a person
            </button>
          )}
        </div>
      </fieldset>

      {/* other */}
      <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
        <label className="col-span-2 flex flex-col gap-0.5 text-[11px] text-slate-400">
          Direction of travel
          <input className="input h-7 text-xs" value={v.direction ?? ''} onChange={(e) => set({ direction: e.target.value })} placeholder="north on Williamson Rd" />
        </label>
        <label className="col-span-2 flex flex-col gap-0.5 text-[11px] text-slate-400">
          Damage
          <input className="input h-7 text-xs" value={v.damage ?? ''} onChange={(e) => set({ damage: e.target.value })} />
        </label>
        <label className="col-span-2 flex flex-col gap-0.5 text-[11px] text-slate-400 lg:col-span-4">
          Description
          <input className="input h-7 text-xs" value={v.description ?? ''} onChange={(e) => set({ description: e.target.value })} />
        </label>
        <label className="col-span-2 flex flex-col gap-0.5 text-[11px] text-slate-400 lg:col-span-4">
          Notes
          <textarea className="input min-h-[44px] text-xs" value={v.notes ?? ''} onChange={(e) => set({ notes: e.target.value })} />
        </label>
      </div>

      <div className="flex justify-end gap-1.5">
        <button type="button" className="btn h-7 text-xs" onClick={onCancel}>
          Cancel
        </button>
        <button className="btn btn-go h-7 text-xs">Save</button>
      </div>
    </form>
  )
}

// Run DMV (RQ) and Run NCIC (QV), enabled only when plate+state or VIN are complete; draft explains first.
function RunButtons({ v, inc }: { v: Vehicle; inc?: Incident }) {
  const [requester, setRequester] = useState('')
  const [running, setRunning] = useState('')
  const draft = !inc || v.id.startsWith('draft-')
  const plateReady = !!(v.plate && v.state)
  const vinReady = (v.vin ?? '').length === 17
  const ready = plateReady || vinReady

  const run = async (key: 'RQ' | 'QV') => {
    if (draft || !ready) return
    setRunning(key)
    try {
      await api.post(`/incidents/${inc!.number}/records/${v.id}/run`, { key, requester: requester.trim() || undefined })
      toast.success(`Ran ${key}: the return is attached to this vehicle`)
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setRunning('')
    }
  }

  if (draft) {
    return <p className="text-[11px] text-amber-300">Send the call first: DMV and NCIC inquiries and tows run once the incident exists.</p>
  }
  return (
    <div className="flex flex-wrap items-center gap-1">
      <input className="input h-7 w-28 text-xs" value={requester} onChange={(e) => setRequester(e.target.value)} placeholder="requester unit" aria-label="Requester unit" />
      <button type="button" className="btn h-7 text-xs" onClick={() => run('RQ')} disabled={!ready || !!running} title={ready ? 'Run DMV' : 'Enter plate and state, or a complete VIN'}>
        <Send size={11} aria-hidden /> Run DMV (RQ)
      </button>
      <button type="button" className="btn h-7 text-xs" onClick={() => run('QV')} disabled={!ready || !!running} title={ready ? 'Run NCIC' : 'Enter plate and state, or a complete VIN'}>
        <Send size={11} aria-hidden /> Run NCIC (QV)
      </button>
      {!ready && <span className="text-[10px] text-slate-500">Enter plate and state, or a complete 17-character VIN.</span>}
    </div>
  )
}

function VehicleInquiries({ incident, record }: { incident: string; record: string }) {
  const [returns, setReturns] = useState<InqReturn[] | null>(null)
  const [disc, setDisc] = useState<string[]>([])
  const [allowed, setAllowed] = useState(true)
  const [hit, setHit] = useState<InqReturn | null>(null)

  const load = () => {
    api
      .get<{ returns: InqReturn[]; discrepancies: string[] }>(`/incidents/${incident}/records/${record}/returns`)
      .then((r) => {
        setReturns(r.returns)
        setDisc(r.discrepancies)
        setAllowed(true)
      })
      .catch((e) => {
        if (e instanceof ApiError && e.status === 403) setAllowed(false)
        setReturns([])
      })
  }
  useEffect(load, [incident, record])

  if (!allowed) return <p className="mt-1 text-[10px] text-slate-500">Your duty cannot open inquiry returns.</p>
  if (!returns || returns.length === 0) return null
  const current = [...returns].reverse().find((r) => !r.superseded)
  return (
    <div className="mt-1.5 border-t border-slate-800 pt-1.5">
      {disc.length > 0 && (
        <div className="mb-1 rounded border border-amber-800 bg-amber-950/30 px-2 py-1 text-[11px] text-amber-100">
          Differences from what was reported: {disc.join('; ')}
        </div>
      )}
      {current?.fields && (
        <div className="mb-1 text-[11px] text-slate-300">
          <span className="font-semibold text-slate-200">Official return ({(current.source ?? '').toUpperCase()}):</span>{' '}
          {[['year', 'year'], ['make', 'make'], ['model', 'model'], ['body', 'body'], ['color', 'color'], ['owner', 'owner']]
            .map(([k, lbl]) => (current.fields![k] ? `${lbl} ${current.fields![k]}` : ''))
            .filter(Boolean)
            .join(' · ')}
        </div>
      )}
      <ReturnsList returns={returns} onHit={setHit} />
      {hit && <HitConfirm incident={incident} record={record} ret={hit} open onClose={() => setHit(null)} onAnswered={load} />}
    </div>
  )
}
