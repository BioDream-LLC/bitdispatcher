import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { AlertTriangle, ArrowLeft, CheckCircle2, ClipboardCheck, Pencil, Plus, Save, Trash2, UserMinus, UserPlus, Users } from 'lucide-react'
import { api, errText } from '../api'
import { can, useStore } from '../store'
import { Field, Logo, Modal } from '../ui'
import { cx } from '../util'
import type { CrewSeat, CrewTemplate, PersonBrief, QualDef, StaffingCoverage, Unit, UnitType } from '../types'

const AVAILABILITY = ['available', 'assigned', 'unavailable', 'training', 'court', 'meal', 'station-duty', 'admin']
const DUTY_KINDS = ['regular', 'overtime', 'holdover', 'detail', 'callback']
// Positions offered in the datalist, beyond those a unit type names.
const COMMON_POSITIONS = ['officer', 'driver', 'firefighter', 'paramedic', 'emt', 'partner', 'team leader']

const toISO = (local: string): string | undefined => (local ? new Date(local).toISOString() : undefined)
const nowLocal = (): string => {
  const d = new Date()
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset())
  return d.toISOString().slice(0, 16)
}

/** Staffing: who is on duty, the crews aboard each unit, sign-ons waiting, saved crews and the shift change,
 * coverage against the minimums, and crew-record corrections. Dispatchers see who is on and ride what;
 * supervisors approve sign-ons, run shift changes, see coverage and correct the record. */
export default function Staffing() {
  const view = useStore((s) => s.view)
  const me = useStore((s) => s.me)
  const supervisor = can.supervise(me)
  const [quals, setQuals] = useState<QualDef[]>([])
  useEffect(() => {
    api.get<{ items: QualDef[] }>('/quals').then((r) => setQuals(r.items ?? [])).catch(() => {})
  }, [])

  return (
    <div className="flex h-full flex-col bg-slate-950">
      <header className="flex items-center gap-3 border-b border-slate-800 bg-slate-900 px-3 py-2">
        <Link to="/" className="btn btn-ghost"><ArrowLeft size={14} aria-hidden /> <Logo size={18} /> Dispatch</Link>
        <h1 className="flex items-center gap-2 text-lg font-bold text-slate-50"><Users size={18} className="text-sky-300" aria-hidden /> Staffing</h1>
      </header>
      <main className="min-h-0 flex-1 overflow-y-auto p-4">
        {!view ? (
          <p className="text-sm text-slate-500">Loading…</p>
        ) : (
          <div className="mx-auto max-w-6xl space-y-8">
            <Roster quals={quals} supervisor={supervisor} />
            <UnitsAndCrews supervisor={supervisor} />
            {supervisor && <SignOnsWaiting />}
            {supervisor && <SavedCrews />}
            {supervisor && <Coverage />}
          </div>
        )}
      </main>
    </div>
  )
}

// ---- a) On duty / roster ----

function Roster({ quals, supervisor }: { quals: QualDef[]; supervisor: boolean }) {
  const view = useStore((s) => s.view)
  const [onduty, setOnduty] = useState(true)
  const [q, setQ] = useState('')
  const [qual, setQual] = useState('')
  const [agency, setAgency] = useState('')
  const [station, setStation] = useState('')
  const [unit, setUnit] = useState('')
  const [availability, setAvailability] = useState('')
  const [items, setItems] = useState<PersonBrief[] | null>(null)
  const [signOn, setSignOn] = useState<PersonBrief | null>(null)
  const [signOff, setSignOff] = useState<PersonBrief | null>(null)

  const load = () => {
    const p = new URLSearchParams()
    if (onduty) p.set('onduty', '1')
    if (q) p.set('q', q)
    if (qual) p.set('qual', qual)
    if (agency) p.set('agency', agency)
    if (station) p.set('station', station)
    if (unit) p.set('unit', unit)
    if (availability) p.set('availability', availability)
    api.get<{ items: PersonBrief[] }>(`/staffing/people?${p.toString()}`).then((r) => setItems(r.items ?? [])).catch((e) => toast.error(errText(e)))
  }
  useEffect(() => {
    const t = setTimeout(load, 200)
    return () => clearTimeout(t)
  }, [onduty, q, qual, agency, station, unit, availability, view?.seq]) // eslint-disable-line react-hooks/exhaustive-deps

  const setAvail = async (id: string, value: string) => {
    try {
      await api.put(`/members/${encodeURIComponent(id)}/availability`, { availability: value, note: '' })
      toast.success('Availability set')
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  return (
    <section aria-label="On duty and roster" className="space-y-3">
      <h2 className="text-base font-semibold text-slate-100">On duty</h2>
      <div className="flex flex-wrap items-end gap-2">
        <label className="flex items-center gap-2 text-sm text-slate-300">
          <input type="checkbox" checked={!onduty} onChange={(e) => setOnduty(!e.target.checked)} aria-label="Show everyone, not only those on duty" /> Show everyone
        </label>
        <div className="w-44"><Field label="Qualification" htmlFor="rf-qual"><select id="rf-qual" className="input" value={qual} onChange={(e) => setQual(e.target.value)}><option value="">any</option>{quals.map((x) => <option key={x.code} value={x.code}>{x.name}</option>)}</select></Field></div>
        <div className="w-28"><Field label="Agency" htmlFor="rf-ag"><input id="rf-ag" className="input" value={agency} onChange={(e) => setAgency(e.target.value)} /></Field></div>
        <div className="w-28"><Field label="Station" htmlFor="rf-st"><input id="rf-st" className="input" value={station} onChange={(e) => setStation(e.target.value)} /></Field></div>
        <div className="w-28"><Field label="Unit" htmlFor="rf-u"><input id="rf-u" className="input font-mono" value={unit} onChange={(e) => setUnit(e.target.value)} /></Field></div>
        <div className="w-40"><Field label="Availability" htmlFor="rf-av"><select id="rf-av" className="input" value={availability} onChange={(e) => setAvailability(e.target.value)}><option value="">any</option>{AVAILABILITY.map((x) => <option key={x} value={x}>{x}</option>)}</select></Field></div>
        <input aria-label="Search people" className="input ml-auto w-64" placeholder="name, number, radio…" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      <div className="panel overflow-x-auto rounded-lg">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-slate-800 text-left text-[11px] uppercase tracking-wider text-slate-500">
              <th className="px-3 py-2 font-semibold">Name</th><th className="px-3 font-semibold">Number</th><th className="px-3 font-semibold">Agency</th><th className="px-3 font-semibold">Station</th>
              <th className="px-3 font-semibold">Riding</th><th className="px-3 font-semibold">Duty</th><th className="px-3 font-semibold">Availability</th><th className="px-3 font-semibold">Qualifications</th><th className="px-3 font-semibold">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-800">
            {(items ?? []).map((p) => (
              <tr key={p.id} className="align-top hover:bg-slate-800/50">
                <td className="px-3 py-1.5 text-slate-100">{p.name}{p.rank && <span className="ml-1 text-xs text-slate-500">{p.rank}</span>}{p.credProblems && p.credProblems.length > 0 && (
                  <div className="mt-1 flex items-start gap-1 text-xs text-amber-300"><AlertTriangle size={12} aria-hidden className="mt-0.5 shrink-0" /><span>{p.credProblems.join('; ')}</span></div>
                )}</td>
                <td className="px-3 font-mono text-slate-400">{p.id}</td>
                <td className="px-3 text-slate-400">{p.agency || '—'}</td>
                <td className="px-3 text-slate-400">{p.station || '—'}</td>
                <td className="px-3 font-mono text-slate-300">{(p.units ?? []).join(', ') || '—'}</td>
                <td className="px-3 text-slate-300">{p.duty ? <span>{p.duty.kind || 'regular'}{p.duty.start && <span className="block text-[10px] text-slate-500">since {new Date(p.duty.start).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}</span>}</span> : <span className="text-slate-500">off</span>}</td>
                <td className="px-3">
                  {p.onDuty ? (
                    <select aria-label={`Availability for ${p.name}`} className="input h-7 py-0 text-xs" value={p.duty?.availability ?? ''} onChange={(e) => setAvail(p.id, e.target.value)}>
                      <option value="">—</option>
                      {AVAILABILITY.map((x) => <option key={x} value={x}>{x}</option>)}
                    </select>
                  ) : <span className="text-slate-500">—</span>}
                </td>
                <td className="px-3">
                  <div className="flex max-w-[16rem] flex-wrap gap-1">
                    {(p.quals ?? []).length ? (p.quals ?? []).map((x) => <span key={x} className="chip bg-slate-800 text-slate-300">{x}</span>) : <span className="text-slate-500">—</span>}
                  </div>
                </td>
                <td className="px-3">
                  {p.onDuty
                    ? <button className="btn h-7" onClick={() => setSignOff(p)}><UserMinus size={12} aria-hidden /> Sign off</button>
                    : <button className="btn btn-go h-7" onClick={() => setSignOn(p)}><UserPlus size={12} aria-hidden /> Sign on</button>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {items?.length === 0 && <p className="p-3 text-sm text-slate-500">No one found.</p>}
        {items === null && <p className="p-3 text-sm text-slate-500">Loading…</p>}
      </div>
      {signOn && <SignOnModal person={signOn} onClose={() => setSignOn(null)} onDone={() => { setSignOn(null); load() }} />}
      {signOff && <SignOffModal person={signOff} supervisor={supervisor} onClose={() => setSignOff(null)} onDone={() => { setSignOff(null); load() }} />}
    </section>
  )
}

function SignOnModal({ person, onClose, onDone }: { person: PersonBrief; onClose: () => void; onDone: () => void }) {
  const view = useStore((s) => s.view)
  const shifts = view?.settings.shifts ?? []
  const [kind, setKind] = useState('regular')
  const [shift, setShift] = useState('')
  const [until, setUntil] = useState('')
  const [availability, setAvailability] = useState('available')
  const [detail, setDetail] = useState('')
  const [note, setNote] = useState('')
  const save = async () => {
    if (kind === 'detail' && !detail.trim()) { toast.error('A detail needs a note saying what it is'); return }
    try {
      await api.post(`/members/${encodeURIComponent(person.id)}/on`, {
        kind, shift: shift || undefined, until: toISO(until), availability, detail: detail || undefined, note: note || undefined,
      })
      toast.success(`${person.name} signed on`)
      onDone()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Modal open onClose={onClose} title={`Sign on ${person.name}`} icon={<UserPlus size={16} className="text-emerald-300" aria-hidden />}
      footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Sign on</button></>}>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Kind" htmlFor="so-kind"><select id="so-kind" className="input" value={kind} onChange={(e) => setKind(e.target.value)}>{DUTY_KINDS.map((k) => <option key={k} value={k}>{k}</option>)}</select></Field>
        <Field label="Shift" htmlFor="so-shift"><select id="so-shift" className="input" value={shift} onChange={(e) => setShift(e.target.value)}><option value="">—</option>{shifts.map((x) => <option key={x} value={x}>{x}</option>)}</select></Field>
        <Field label="Availability" htmlFor="so-av"><select id="so-av" className="input" value={availability} onChange={(e) => setAvailability(e.target.value)}>{AVAILABILITY.map((x) => <option key={x} value={x}>{x}</option>)}</select></Field>
        <Field label="Until" htmlFor="so-until" hint="When this duty period ends; leave blank for open-ended."><input id="so-until" type="datetime-local" className="input" value={until} onChange={(e) => setUntil(e.target.value)} /></Field>
        {kind === 'detail' && <div className="sm:col-span-2"><Field label="Detail" htmlFor="so-detail" hint="Required: what the detail is for."><input id="so-detail" className="input" value={detail} onChange={(e) => setDetail(e.target.value)} /></Field></div>}
        <div className="sm:col-span-2"><Field label="Note" htmlFor="so-note"><input id="so-note" className="input" value={note} onChange={(e) => setNote(e.target.value)} /></Field></div>
      </div>
    </Modal>
  )
}

function SignOffModal({ person, supervisor, onClose, onDone }: { person: PersonBrief; supervisor: boolean; onClose: () => void; onDone: () => void }) {
  const [reason, setReason] = useState('')
  const [override, setOverride] = useState('')
  const [effective, setEffective] = useState('')
  const save = async () => {
    try {
      await api.post(`/members/${encodeURIComponent(person.id)}/off`, {
        reason: reason || undefined, override: override || undefined, effective: toISO(effective),
      })
      toast.success(`${person.name} signed off`)
      onDone()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Modal open onClose={onClose} title={`Sign off ${person.name}`} icon={<UserMinus size={16} className="text-amber-300" aria-hidden />}
      footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Sign off</button></>}>
      <div className="space-y-3">
        <p className="text-sm text-slate-400">A unit left short of its minimum crew becomes limited or out of service, per agency policy — unless a supervisor overrides it.</p>
        <Field label="Reason" htmlFor="sf-reason"><input id="sf-reason" className="input" value={reason} onChange={(e) => setReason(e.target.value)} /></Field>
        <Field label="Effective" htmlFor="sf-eff" hint="When the sign-off takes effect; leave blank for now."><input id="sf-eff" type="datetime-local" className="input" value={effective} onChange={(e) => setEffective(e.target.value)} /></Field>
        {supervisor && <Field label="Override" htmlFor="sf-ov" hint="A supervisor may keep the unit in service despite the short crew; say why."><input id="sf-ov" className="input" value={override} onChange={(e) => setOverride(e.target.value)} placeholder="reason to keep in service" /></Field>}
      </div>
    </Modal>
  )
}

// ---- b) Units and crews ----

const CAP_WORD: Record<string, string> = { full: 'Full', limited: 'Limited', unavailable: 'Unavailable' }

function CapabilityBadge({ state, reasons }: { state: 'full' | 'limited' | 'unavailable'; reasons?: string[] }) {
  const cls = state === 'full' ? 'bg-emerald-500/20 text-emerald-200' : state === 'limited' ? 'bg-amber-500/20 text-amber-200' : 'bg-rose-500/20 text-rose-200'
  const Icon = state === 'full' ? CheckCircle2 : AlertTriangle
  return (
    <span className="inline-flex items-center gap-1">
      <span className={cx('chip', cls)}><Icon size={12} aria-hidden /> {CAP_WORD[state]}</span>
      {reasons && reasons.length > 0 && <span className="text-xs text-slate-400">{reasons.join('; ')}</span>}
    </span>
  )
}

function UnitsAndCrews({ supervisor }: { supervisor: boolean }) {
  const view = useStore((s) => s.view)
  const units = view?.units ?? []
  const [edit, setEdit] = useState<Unit | null>(null)
  const [correct, setCorrect] = useState<Unit | null>(null)
  const [tplFor, setTplFor] = useState<Unit | null>(null)

  return (
    <section aria-label="Units and crews" className="space-y-3">
      <h2 className="text-base font-semibold text-slate-100">Units and crews</h2>
      <div className="grid gap-3 md:grid-cols-2">
        {units.map((u) => (
          <div key={u.id} className="panel rounded-lg p-3">
            <div className="flex items-center gap-2">
              <span className="font-mono font-semibold text-slate-100">{u.id}</span>
              {u.capability && <CapabilityBadge state={u.capability.state} reasons={u.capability.reasons} />}
              <div className="ml-auto flex gap-1">
                <button className="btn h-7" onClick={() => setEdit(u)}><Pencil size={12} aria-hidden /> Edit crew</button>
                {supervisor && <button className="btn h-7" onClick={() => setTplFor(u)} title="Save this crew as a template"><Save size={12} aria-hidden /> Save crew</button>}
                {supervisor && <button className="btn h-7" onClick={() => setCorrect(u)} title="Correct a past crew record"><ClipboardCheck size={12} aria-hidden /> Correct</button>}
              </div>
            </div>
            <table className="mt-2 w-full text-sm">
              <thead><tr className="text-left text-[10px] uppercase tracking-wider text-slate-500"><th className="font-semibold">Member</th><th className="font-semibold">Position</th></tr></thead>
              <tbody>
                {(u.crew ?? []).map((c, i) => (
                  <tr key={i}><td className="py-0.5 text-slate-200">{c.name || c.member}{c.secondary && <span className="ml-1 text-xs text-slate-500">(secondary)</span>}</td><td className="text-slate-400">{c.role || '—'}</td></tr>
                ))}
                {(u.crew ?? []).length === 0 && <tr><td colSpan={2} className="py-0.5 text-slate-500">No crew aboard.</td></tr>}
              </tbody>
            </table>
          </div>
        ))}
        {units.length === 0 && <p className="text-sm text-slate-500">No units.</p>}
      </div>
      {edit && <EditCrewModal unit={edit} supervisor={supervisor} onClose={() => setEdit(null)} onDone={() => setEdit(null)} />}
      {correct && <CorrectCrewModal unit={correct} onClose={() => setCorrect(null)} onDone={() => setCorrect(null)} />}
      {tplFor && <SaveTemplateModal unit={tplFor} onClose={() => setTplFor(null)} onDone={() => setTplFor(null)} />}
    </section>
  )
}

type SeatDraft = { member: string; role: string; secondary?: boolean; move?: boolean; reason?: string }

function positionsFor(unitTypes: UnitType[], typeCode: string): string[] {
  const t = unitTypes.find((x) => x.code === typeCode)
  return [...new Set([...(t?.positions ?? []), ...COMMON_POSITIONS])]
}

function EditCrewModal({ unit, supervisor, onClose, onDone }: { unit: Unit; supervisor: boolean; onClose: () => void; onDone: () => void }) {
  const view = useStore((s) => s.view)
  const members = view?.roadmap?.members ?? []
  const positions = useMemo(() => positionsFor(view?.settings.unitTypes ?? [], unit.type), [view, unit.type])
  const [rows, setRows] = useState<SeatDraft[]>(() => (unit.crew ?? []).map((c) => ({ member: c.member, role: c.role ?? '', secondary: c.secondary, move: c.move, reason: c.reason })))
  const [reason, setReason] = useState('')
  const [effective, setEffective] = useState(nowLocal())
  const [override, setOverride] = useState('')
  const listId = `pos-${unit.id}`

  const ridingElsewhere = (memberId: string): string | undefined => {
    const m = members.find((x) => x.id === memberId)
    return m?.riding && m.riding !== unit.id ? m.riding : undefined
  }
  const setRow = (i: number, patch: Partial<SeatDraft>) => setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  const removeRow = (i: number) => setRows((rs) => rs.filter((_, j) => j !== i))
  const addRow = () => setRows((rs) => [...rs, { member: '', role: '' }])

  const save = async () => {
    for (const r of rows) {
      if (r.member && ridingElsewhere(r.member) && r.secondary && !r.reason?.trim()) { toast.error('A secondary assignment needs a reason'); return }
    }
    const crew: Partial<CrewSeat>[] = rows.filter((r) => r.member).map((r) => ({
      member: r.member, role: r.role || undefined, secondary: r.secondary || undefined, move: r.move || undefined, reason: r.reason || undefined,
    }))
    try {
      await api.put(`/units/${encodeURIComponent(unit.id)}/crew`, { crew, reason: reason || undefined, effective: toISO(effective), override: override || undefined })
      toast.success(`${unit.id} crew saved`)
      onDone()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  return (
    <Modal open onClose={onClose} wide title={`Edit crew — ${unit.id}`} icon={<Users size={16} className="text-sky-300" aria-hidden />}
      footer={<><button className="btn mr-auto" onClick={addRow}><Plus size={13} aria-hidden /> Add a seat</button><button className="btn" onClick={onClose}>Cancel</button><button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save crew</button></>}>
      <datalist id={listId}>{positions.map((p) => <option key={p} value={p} />)}</datalist>
      <div className="space-y-3">
        {rows.map((r, i) => {
          const elsewhere = r.member ? ridingElsewhere(r.member) : undefined
          return (
            <div key={i} className="rounded border border-slate-800 p-2">
              <div className="grid gap-2 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
                <Field label="Member" htmlFor={`cr-m-${i}`}>
                  <select id={`cr-m-${i}`} className="input" value={r.member} onChange={(e) => setRow(i, { member: e.target.value, secondary: false, move: false, reason: '' })}>
                    <option value="">— choose —</option>
                    {members.map((m) => <option key={m.id} value={m.id}>{m.name}{m.riding ? ` (riding ${m.riding})` : ''}</option>)}
                  </select>
                </Field>
                <Field label="Position" htmlFor={`cr-p-${i}`}><input id={`cr-p-${i}`} className="input" list={listId} value={r.role} onChange={(e) => setRow(i, { role: e.target.value })} /></Field>
                <button className="btn h-9 text-rose-300" onClick={() => removeRow(i)} aria-label={`Remove seat ${i + 1}`}><Trash2 size={13} aria-hidden /></button>
              </div>
              {elsewhere && (
                <div className="mt-2 space-y-2 rounded bg-slate-900 p-2 text-sm">
                  <p className="text-amber-300">This member is riding {elsewhere}.</p>
                  <div className="flex flex-wrap gap-3">
                    <label className="flex items-center gap-2 text-slate-300"><input type="radio" name={`seat-${i}`} checked={!!r.move} onChange={() => setRow(i, { move: true, secondary: false })} /> Move them here (relief or swap)</label>
                    <label className="flex items-center gap-2 text-slate-300"><input type="radio" name={`seat-${i}`} checked={!!r.secondary} onChange={() => setRow(i, { secondary: true, move: false })} /> Secondary assignment</label>
                  </div>
                  {r.secondary && <Field label="Reason" htmlFor={`cr-r-${i}`} hint="Required for a secondary assignment."><input id={`cr-r-${i}`} className="input" value={r.reason ?? ''} onChange={(e) => setRow(i, { reason: e.target.value })} /></Field>}
                </div>
              )}
            </div>
          )
        })}
        {rows.length === 0 && <p className="text-sm text-slate-500">No seats. Add one below.</p>}
        <div className="grid gap-3 border-t border-slate-800 pt-3 sm:grid-cols-2">
          <Field label="Reason for the change" htmlFor="cc-reason"><input id="cc-reason" className="input" value={reason} onChange={(e) => setReason(e.target.value)} /></Field>
          <Field label="Effective" htmlFor="cc-eff"><input id="cc-eff" type="datetime-local" className="input" value={effective} onChange={(e) => setEffective(e.target.value)} /></Field>
          {supervisor && <div className="sm:col-span-2"><Field label="Override" htmlFor="cc-ov" hint="A supervisor may force a change that policy would otherwise refuse; say why."><input id="cc-ov" className="input" value={override} onChange={(e) => setOverride(e.target.value)} /></Field></div>}
        </div>
      </div>
    </Modal>
  )
}

function SaveTemplateModal({ unit, onClose, onDone }: { unit: Unit; onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState('')
  const save = async () => {
    if (!name.trim()) { toast.error('A name is needed'); return }
    try {
      await api.put('/crewtemplates', { name: name.trim(), unit: unit.id, crew: unit.crew ?? [] })
      toast.success('Crew saved')
      onDone()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Modal open onClose={onClose} title={`Save ${unit.id}'s crew`} icon={<Save size={16} className="text-sky-300" aria-hidden />}
      footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save</button></>}>
      <Field label="Name" htmlFor="tpl-name" hint={`The ${(unit.crew ?? []).length} seat(s) aboard ${unit.id} now are kept as a saved crew.`}><input id="tpl-name" className="input" value={name} onChange={(e) => setName(e.target.value)} /></Field>
    </Modal>
  )
}

function CorrectCrewModal({ unit, onClose, onDone }: { unit: Unit; onClose: () => void; onDone: () => void }) {
  const view = useStore((s) => s.view)
  const members = view?.roadmap?.members ?? []
  const positions = useMemo(() => positionsFor(view?.settings.unitTypes ?? [], unit.type), [view, unit.type])
  const [incident, setIncident] = useState('')
  const [rows, setRows] = useState<SeatDraft[]>(() => (unit.crew ?? []).map((c) => ({ member: c.member, role: c.role ?? '' })))
  const [effective, setEffective] = useState(nowLocal())
  const [reason, setReason] = useState('')
  const listId = `cpos-${unit.id}`
  const setRow = (i: number, patch: Partial<SeatDraft>) => setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  const removeRow = (i: number) => setRows((rs) => rs.filter((_, j) => j !== i))
  const addRow = () => setRows((rs) => [...rs, { member: '', role: '' }])
  const save = async () => {
    if (!reason.trim()) { toast.error('A correction needs a reason'); return }
    const crew: Partial<CrewSeat>[] = rows.filter((r) => r.member).map((r) => ({ member: r.member, role: r.role || undefined, since: new Date(effective).toISOString() }))
    try {
      await api.post(`/units/${encodeURIComponent(unit.id)}/crew/correct`, { incident: incident || undefined, crew, effective: toISO(effective), reason })
      toast.success('Correction recorded')
      onDone()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Modal open onClose={onClose} wide title={`Correct a crew record — ${unit.id}`} icon={<ClipboardCheck size={16} className="text-amber-300" aria-hidden />}
      footer={<><button className="btn mr-auto" onClick={addRow}><Plus size={13} aria-hidden /> Add a seat</button><button className="btn" onClick={onClose}>Cancel</button><button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Record correction</button></>}>
      <datalist id={listId}>{positions.map((p) => <option key={p} value={p} />)}</datalist>
      <div className="space-y-3">
        <p className="text-sm text-slate-400">This records who was really aboard. The original record is kept; the correction sits alongside it.</p>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Incident number" htmlFor="cor-inc" hint="Optional: the incident this crew was on."><input id="cor-inc" className="input font-mono" value={incident} onChange={(e) => setIncident(e.target.value)} /></Field>
          <Field label="Effective" htmlFor="cor-eff"><input id="cor-eff" type="datetime-local" className="input" value={effective} onChange={(e) => setEffective(e.target.value)} /></Field>
        </div>
        {rows.map((r, i) => (
          <div key={i} className="grid gap-2 sm:grid-cols-[1fr_1fr_auto] sm:items-end rounded border border-slate-800 p-2">
            <Field label="Member" htmlFor={`cor-m-${i}`}>
              <select id={`cor-m-${i}`} className="input" value={r.member} onChange={(e) => setRow(i, { member: e.target.value })}>
                <option value="">— choose —</option>
                {members.map((m) => <option key={m.id} value={m.id}>{m.name}{m.riding ? ` (riding ${m.riding})` : ''}</option>)}
              </select>
            </Field>
            <Field label="Position" htmlFor={`cor-p-${i}`}><input id={`cor-p-${i}`} className="input" list={listId} value={r.role} onChange={(e) => setRow(i, { role: e.target.value })} /></Field>
            <button className="btn h-9 text-rose-300" onClick={() => removeRow(i)} aria-label={`Remove seat ${i + 1}`}><Trash2 size={13} aria-hidden /></button>
          </div>
        ))}
        <Field label="Reason" htmlFor="cor-reason" hint="Required: why the record is being corrected."><input id="cor-reason" className="input" value={reason} onChange={(e) => setReason(e.target.value)} /></Field>
      </div>
    </Modal>
  )
}

// ---- c) Sign-ons waiting (supervisors) ----

function SignOnsWaiting() {
  const view = useStore((s) => s.view)
  const [cov, setCov] = useState<StaffingCoverage | null>(null)
  const load = () => api.get<StaffingCoverage>('/staffing/coverage').then(setCov).catch((e) => toast.error(errText(e)))
  useEffect(() => { load() }, [view?.seq]) // eslint-disable-line react-hooks/exhaustive-deps
  const decide = async (id: string, approve: boolean) => {
    let why = ''
    if (!approve) {
      const r = prompt('Why is this sign-on refused?')
      if (r === null) return
      why = r
    }
    try {
      await api.post(`/crewrequests/${encodeURIComponent(id)}`, { approve, why })
      toast.success(approve ? 'Approved' : 'Refused')
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const reqs = cov?.requests ?? []
  return (
    <section aria-label="Sign-ons waiting" className="space-y-3">
      <h2 className="text-base font-semibold text-slate-100">Sign-ons waiting</h2>
      {reqs.length === 0 ? (
        <p className="text-sm text-slate-500">None waiting.</p>
      ) : (
        <div className="panel overflow-x-auto rounded-lg">
          <table className="w-full text-sm">
            <thead><tr className="border-b border-slate-800 text-left text-[11px] uppercase tracking-wider text-slate-500"><th className="px-3 py-2 font-semibold">Unit</th><th className="px-3 font-semibold">Crew change</th><th className="px-3 font-semibold">By</th><th className="px-3 font-semibold">Actions</th></tr></thead>
            <tbody className="divide-y divide-slate-800">
              {reqs.map((req) => (
                <tr key={req.id}>
                  <td className="px-3 py-1.5 font-mono text-slate-100">{req.unit}</td>
                  <td className="px-3 text-slate-300">{(req.change.crew ?? []).map((c) => `${c.member}${c.role ? ` (${c.role})` : ''}`).join(', ') || '—'}{req.change.reason && <span className="block text-xs text-slate-500">{req.change.reason}</span>}</td>
                  <td className="px-3 text-slate-400">{req.by}</td>
                  <td className="px-3">
                    <div className="flex gap-1">
                      <button className="btn btn-go h-7" onClick={() => decide(req.id, true)}>Approve</button>
                      <button className="btn h-7 text-rose-300" onClick={() => decide(req.id, false)}>Refuse</button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}

// ---- d) Saved crews and shift change (supervisors) ----

function SavedCrews() {
  const [items, setItems] = useState<CrewTemplate[]>([])
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [reason, setReason] = useState('')
  const [effective, setEffective] = useState(nowLocal())
  const load = () => api.get<{ items: CrewTemplate[] }>('/crewtemplates').then((r) => setItems(r.items ?? [])).catch((e) => toast.error(errText(e)))
  useEffect(() => { load() }, [])
  const toggle = (id: string) => setPicked((p) => { const n = new Set(p); n.has(id) ? n.delete(id) : n.add(id); return n })
  const remove = async (id: string) => {
    if (!confirm('Delete this saved crew?')) return
    try {
      await api.del(`/crewtemplates/${encodeURIComponent(id)}`)
      toast.success('Deleted')
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const runShift = async () => {
    if (picked.size === 0) { toast.error('Tick at least one saved crew'); return }
    try {
      const r = await api.post<{ units: number }>('/staffing/shift', { templates: [...picked], reason: reason || undefined, effective: toISO(effective) })
      toast.success(`Shift change applied to ${r.units} unit(s)`)
      setPicked(new Set())
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section aria-label="Saved crews and shift change" className="space-y-3">
      <h2 className="text-base font-semibold text-slate-100">Saved crews and shift change</h2>
      {items.length === 0 ? (
        <p className="text-sm text-slate-500">No saved crews yet. Save one from a unit above.</p>
      ) : (
        <div className="panel overflow-x-auto rounded-lg">
          <table className="w-full text-sm">
            <thead><tr className="border-b border-slate-800 text-left text-[11px] uppercase tracking-wider text-slate-500"><th className="px-3 py-2 font-semibold">Use</th><th className="px-3 font-semibold">Name</th><th className="px-3 font-semibold">Unit</th><th className="px-3 font-semibold">Crew</th><th className="px-3 font-semibold">Actions</th></tr></thead>
            <tbody className="divide-y divide-slate-800">
              {items.map((t) => (
                <tr key={t.id}>
                  <td className="px-3 py-1.5"><input type="checkbox" checked={picked.has(t.id)} onChange={() => toggle(t.id)} aria-label={`Use saved crew ${t.name}`} /></td>
                  <td className="px-3 text-slate-100">{t.name}</td>
                  <td className="px-3 font-mono text-slate-300">{t.unit}</td>
                  <td className="px-3 text-slate-400">{(t.crew ?? []).map((c) => `${c.name || c.member}${c.role ? ` (${c.role})` : ''}`).join(', ') || '—'}</td>
                  <td className="px-3"><button className="btn h-7 text-rose-300" onClick={() => remove(t.id)}><Trash2 size={12} aria-hidden /> Delete</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <div className="flex flex-wrap items-end gap-2">
        <Field label="Reason" htmlFor="sh-reason"><input id="sh-reason" className="input w-56" value={reason} onChange={(e) => setReason(e.target.value)} /></Field>
        <Field label="Effective" htmlFor="sh-eff"><input id="sh-eff" type="datetime-local" className="input" value={effective} onChange={(e) => setEffective(e.target.value)} /></Field>
        <button className="btn btn-go" onClick={runShift}>Apply shift change</button>
      </div>
    </section>
  )
}

// ---- e) Coverage (supervisors) ----

function Coverage() {
  const view = useStore((s) => s.view)
  const [cov, setCov] = useState<StaffingCoverage | null>(null)
  const [hidden, setHidden] = useState(false)
  const load = () => api.get<StaffingCoverage>('/staffing/coverage').then(setCov).catch((e) => {
    // The agency may not grant the coverage view: hide the section on a 403.
    const msg = errText(e)
    if (msg.toLowerCase().includes('forbid') || msg.includes('403')) setHidden(true)
    else toast.error(msg)
  })
  useEffect(() => { load() }, [view?.seq]) // eslint-disable-line react-hooks/exhaustive-deps
  if (hidden || !cov) return null
  const gaps = cov.gaps ?? []
  const expiring = cov.expiring ?? []
  const units = cov.units ?? []
  return (
    <section aria-label="Coverage" className="space-y-4">
      <h2 className="text-base font-semibold text-slate-100">Coverage</h2>
      <div>
        <h3 className="mb-1 text-xs font-semibold uppercase tracking-wider text-slate-500">Minimum complements</h3>
        {gaps.length === 0 ? <p className="text-sm text-slate-500">All minimums met.</p> : (
          <div className="panel overflow-x-auto rounded-lg">
            <table className="w-full text-sm">
              <thead><tr className="border-b border-slate-800 text-left text-[11px] uppercase tracking-wider text-slate-500"><th className="px-3 py-2 font-semibold">Qualification</th><th className="px-3 font-semibold">Wanted</th><th className="px-3 font-semibold">Have</th><th className="px-3 font-semibold">People</th></tr></thead>
              <tbody className="divide-y divide-slate-800">
                {gaps.map((g, i) => {
                  const short = g.count - g.have
                  return (
                    <tr key={i}>
                      <td className="px-3 py-1.5 text-slate-100">{g.qual}{g.agency && <span className="ml-1 text-xs text-slate-500">{g.agency}</span>}</td>
                      <td className="px-3 font-mono text-slate-300">{g.count}</td>
                      <td className="px-3 font-mono text-slate-300">{g.have}</td>
                      <td className="px-3 text-slate-400">{(g.names ?? []).join(', ') || '—'}{short > 0 && <span className="ml-2 chip bg-rose-500/20 text-rose-200">short by {short}</span>}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
      <div>
        <h3 className="mb-1 text-xs font-semibold uppercase tracking-wider text-slate-500">Expiring credentials</h3>
        {expiring.length === 0 ? <p className="text-sm text-slate-500">None expiring.</p> : (
          <div className="panel overflow-x-auto rounded-lg">
            <table className="w-full text-sm">
              <thead><tr className="border-b border-slate-800 text-left text-[11px] uppercase tracking-wider text-slate-500"><th className="px-3 py-2 font-semibold">Name</th><th className="px-3 font-semibold">Qualification</th><th className="px-3 font-semibold">Expires</th></tr></thead>
              <tbody className="divide-y divide-slate-800">
                {expiring.map((x, i) => (
                  <tr key={i}>
                    <td className="px-3 py-1.5 text-slate-100">{x.name}</td>
                    <td className="px-3 text-slate-300">{x.qual}</td>
                    <td className="px-3 text-slate-400">{x.expires?.slice(0, 10)}{x.expired && <span className="ml-2 chip bg-rose-500/20 text-rose-200">EXPIRED</span>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
      <div>
        <h3 className="mb-1 text-xs font-semibold uppercase tracking-wider text-slate-500">Each unit's capability</h3>
        <div className="panel overflow-x-auto rounded-lg">
          <table className="w-full text-sm">
            <thead><tr className="border-b border-slate-800 text-left text-[11px] uppercase tracking-wider text-slate-500"><th className="px-3 py-2 font-semibold">Unit</th><th className="px-3 font-semibold">Status</th><th className="px-3 font-semibold">Capability</th><th className="px-3 font-semibold">Crew</th></tr></thead>
            <tbody className="divide-y divide-slate-800">
              {units.map((u) => (
                <tr key={u.unit} className="align-top">
                  <td className="px-3 py-1.5 font-mono text-slate-100">{u.unit}</td>
                  <td className="px-3 text-slate-400">{u.status}</td>
                  <td className="px-3"><CapabilityBadge state={u.capability.state} reasons={u.capability.reasons} /></td>
                  <td className="px-3 text-slate-400">{(u.crew ?? []).map((c) => c.name || c.member).join(', ') || '—'}</td>
                </tr>
              ))}
              {units.length === 0 && <tr><td colSpan={4} className="px-3 py-1.5 text-slate-500">No units.</td></tr>}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  )
}
