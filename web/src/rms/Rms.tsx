import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { ArrowLeft, BadgeCheck, CheckCircle2, CircleAlert, FilePlus2, Plus, Send, Sparkles, Trash2, Undo2 } from 'lucide-react'
import { api, errText } from '../api'
import { can, store, useStore } from '../store'
import type { Session } from '../types'
import { Logo } from '../ui'
import { cx } from '../util'
import NcicPanel from './NcicPanel'
import StateFile from './StateFile'

export type Offense = {
  code: string; attempted?: boolean; weapons?: string[]; bias?: string; using?: string[]; activity?: string[]; entry?: string; premises?: number
  security?: string[]; entryPts?: string[]; exitPts?: string[]; leftBy?: string; lpr?: boolean
}
export type Person = {
  role: string; name: string; dob?: string; age?: string; sex?: string; race?: string; ethnicity?: string; address?: string; phone?: string; injury?: string; relation?: string
  victimType?: string; resident?: string; injuries?: string[]; circumstances?: string[]; relations?: { suspect: number; code: string }[]
  officerAct?: string; officerAssign?: string; officerOri?: string; assaultStatus?: string
  arrestNo?: string; arrestDate?: string; arrestType?: string; arrestOffense?: string; armed?: string[]; juvenile?: string; arrestDrugs?: string[]
}
export type Property = { loss: string; type: string; description?: string; value?: number; quantity?: number; recoveredOn?: string; serial?: string; drug?: string; drugQty?: number; drugUnit?: string }
export type Vehicle = { role?: string; plate?: string; state?: string; year?: string; make?: string; model?: string; color?: string; vin?: string }
export type Report = {
  number: string; incident?: string; status: 'draft' | 'submitted' | 'approved' | 'returned'; author: string; created: string; updated: string
  approver?: string; decided?: string; note?: string; occurred?: string; hourUnknown?: boolean; location?: string; locationType?: string; narrative?: string
  offenses: Offense[]; people: Person[]; property: Property[]; vehicles: Vehicle[]; problems?: number; cleared?: string; clearedOn?: string
}
type Codes = Record<'using' | 'bias' | 'activity' | 'weapons' | 'loss' | 'property' | 'drugs' | 'drugUnits' | 'victimTypes' | 'officer' | 'assignment' | 'races' | 'ethnicities'
  | 'residents' | 'circumstances' | 'injuries' | 'relations' | 'arrestTypes' | 'armed' | 'juvenile' | 'clearance' | 'security' | 'points' | 'leftBy', Record<string, string>>
type Check = { field: string; message: string }
export type Meta = {
  offenses: { code: string; name: string; against: string; group: string }[]; locations: Record<string, string>; codes: Codes; needs: Record<string, string[]>
  ncic: { mode: string; ori: string; kinds: { key: string; name: string; fields: string[]; need: string[][] }[]; fields: Record<string, string>; purposes: Record<string, string> }
}

const STATUS_CLS: Record<string, string> = { draft: 'bg-slate-700 text-slate-200', submitted: 'bg-sky-500/20 text-sky-300', approved: 'bg-emerald-500/20 text-emerald-300', returned: 'bg-amber-500/20 text-amber-300' }

/** Incident reports (RMS): the list, and the report being written. */
export default function Rms() {
  const { number } = useParams()
  const me = useStore((s) => s.me)
  const [list, setList] = useState<Report[]>([])
  const [meta, setMeta] = useState<Meta | null>(null)
  const [filter, setFilter] = useState('')
  const [ncic, setNcic] = useState(false)
  const nav = useNavigate()
  const load = () => api.get<Report[]>('/rms/reports').then(setList).catch((e) => toast.error(errText(e)))
  useEffect(() => {
    load()
    api.get<Meta>('/rms/meta').then(setMeta).catch(() => {})
  }, [number])
  const shown = list.filter((r) => !filter || r.status === filter)
  return (
    <div className="flex h-full bg-slate-950">
      <nav className="flex w-80 shrink-0 flex-col border-r border-slate-800 bg-slate-900" aria-label="Reports">
        <div className="flex items-center gap-2 border-b border-slate-800 p-2">
          <Link to="/" className="flex items-center gap-2 rounded px-2 py-1 text-sm font-semibold text-slate-200 hover:bg-slate-800"><ArrowLeft size={14} aria-hidden /> <Logo size={18} /> Dispatch</Link>
          <button className="btn ml-auto h-7 text-xs" onClick={() => setNcic(true)} disabled={!meta || meta.ncic.mode === 'off'} title={meta?.ncic.mode === 'off' ? 'NCIC is off in Settings' : 'NCIC inquiry (F9)'}>NCIC</button>
          <button className="btn btn-go h-7 text-xs" onClick={async () => {
            try {
              const r = await api.post<{ report: Report }>('/rms/reports', { offenses: [], people: [], property: [], vehicles: [] })
              nav(`/rms/${r.report.number}`)
            } catch (e) { toast.error(errText(e)) }
          }}><FilePlus2 size={13} aria-hidden /> New</button>
        </div>
        <div className="flex gap-1 p-2 text-[11px]">
          {['', 'draft', 'submitted', 'returned', 'approved'].map((f) => (
            <button key={f} className={cx('rounded px-1.5 py-0.5', filter === f ? 'bg-slate-700 text-slate-50' : 'text-slate-400 hover:bg-slate-800')} onClick={() => setFilter(f)}>
              {f || 'all'} <span className="font-mono text-slate-500">{f ? list.filter((r) => r.status === f).length : list.length}</span>
            </button>
          ))}
        </div>
        <ul className="min-h-0 flex-1 overflow-y-auto">
          {shown.length === 0 && <li className="px-3 py-2 text-sm text-slate-500">No reports. Write one from an incident (Report) or with New.</li>}
          {shown.map((r) => (
            <li key={r.number}>
              <Link to={`/rms/${r.number}`} className={cx('block border-b border-slate-800/60 px-3 py-2 text-sm hover:bg-slate-800/60', r.number === number && 'bg-slate-800')}>
                <div className="flex items-center gap-2">
                  <span className="font-mono font-bold text-slate-100">{r.number}</span>
                  <span className={cx('chip normal-case', STATUS_CLS[r.status])}>{r.status}</span>
                  {r.status !== 'approved' && (r.problems ?? 0) > 0 && <span className="chip bg-amber-500/10 text-amber-300">{r.problems} to fix</span>}
                </div>
                <div className="truncate text-xs text-slate-400">{r.offenses.map((o) => o.code).join(' ') || 'no offence yet'} · {r.location || 'no location'}</div>
                <div className="text-[10px] text-slate-500">{r.author}{r.incident && ` · ${r.incident}`} · {new Date(r.updated).toLocaleString()}</div>
              </Link>
            </li>
          ))}
        </ul>
        {can.supervise(me) && <StateFile />}
      </nav>
      <main className="min-w-0 flex-1 overflow-y-auto p-4">
        {number && meta ? <Editor key={number} number={number} meta={meta} me={me} onSaved={load} /> : (
          <div className="grid h-full place-items-center text-sm text-slate-500">Choose a report, or write a new one.</div>
        )}
      </main>
      {meta && <NcicPanel open={ncic} onOpenChange={setNcic} meta={meta} reason={number ?? ""} />}
    </div>
  )
}

function Editor({ number, meta, me, onSaved }: { number: string; meta: Meta; me: Session | null; onSaved: () => void }) {
  const [r, setR] = useState<Report | null>(null)
  const [checks, setChecks] = useState<Check[]>([])
  const [agency, setAgency] = useState<Check[]>([])
  const [saving, setSaving] = useState(false)
  const [note, setNote] = useState('')
  const dirty = useRef(false)
  useEffect(() => {
    api.get<{ report: Report; checks: Check[]; agency?: Check[] }>(`/rms/reports/${number}`).then((x) => { setR(x.report); setChecks(x.checks); setAgency(x.agency ?? []) }).catch((e) => toast.error(errText(e)))
  }, [number])
  const editable = !!r && (r.status === 'draft' || r.status === 'returned') && (r.author === me?.username || can.supervise(me))
  // Drafts save themselves a moment after typing stops: no Save button to forget.
  useEffect(() => {
    if (!r || !dirty.current || !editable) return
    const t = setTimeout(async () => {
      setSaving(true)
      try {
        const x = await api.post<{ report: Report; checks: Check[]; agency?: Check[] }>('/rms/reports', r)
        setChecks(x.checks)
        setAgency(x.agency ?? [])
        dirty.current = false
        onSaved()
      } catch (e) {
        toast.error(errText(e))
      } finally {
        setSaving(false)
      }
    }, 900)
    return () => clearTimeout(t)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [r])
  const offenseName = useMemo(() => Object.fromEntries(meta.offenses.map((o) => [o.code, o])), [meta])
  const c = meta.codes
  if (!r) return <p className="text-sm text-slate-500">Loading…</p>
  const up = (p: Partial<Report>) => { dirty.current = true; setR({ ...r, ...p }) }
  const status = async (s: string) => {
    try {
      if (dirty.current) await api.post('/rms/reports', r)
      await api.post(`/rms/reports/${r.number}/status`, { status: s, note })
      const x = await api.get<{ report: Report; checks: Check[]; agency?: Check[] }>(`/rms/reports/${r.number}`)
      setR(x.report)
      setChecks(x.checks)
      setAgency(x.agency ?? [])
      setNote('')
      onSaved()
      toast.success(s === 'submitted' ? 'Submitted for approval' : s === 'approved' ? 'Approved' : 'Returned to the author')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const list = <T,>(k: 'offenses' | 'people' | 'property' | 'vehicles', i: number, p: Partial<T>) => up({ [k]: (r[k] as T[]).map((x, j) => (j === i ? { ...x, ...p } : x)) } as Partial<Report>)
  const del = (k: 'offenses' | 'people' | 'property' | 'vehicles', i: number) => up({ [k]: (r[k] as unknown[]).filter((_, j) => j !== i) } as Partial<Report>)
  const sup = can.supervise(me) && r.author !== me?.username
  const dis = !editable
  return (
    <div className="mx-auto grid max-w-6xl gap-4 xl:grid-cols-[1fr_20rem]">
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <h1 className="font-mono text-xl font-bold text-slate-50">{r.number}</h1>
          <span className={cx('chip normal-case', STATUS_CLS[r.status])}>{r.status}</span>
          {r.incident && <Link to="/" onClick={() => store.select(r.incident!)} className="font-mono text-sm text-sky-300 underline">{r.incident}</Link>}
          <span className="text-xs text-slate-500">by {r.author}{r.approver && ` · ${r.status === 'approved' ? 'approved' : 'returned'} by ${r.approver}`}</span>
          <span className="ml-auto text-xs text-slate-500">{saving ? 'Saving…' : editable ? 'Saved as you type' : ''}</span>
        </div>
        {r.status === 'returned' && r.note && <p className="rounded border border-amber-700 bg-amber-950/30 p-2 text-sm text-amber-100">Returned: {r.note}</p>}
        <Box title="When and where">
          <div className="grid gap-2 md:grid-cols-4">
            <label className="text-xs text-slate-400">Occurred<input type="datetime-local" className="input mt-0.5" disabled={dis} value={r.occurred ? new Date(+new Date(r.occurred) - new Date().getTimezoneOffset() * 60000).toISOString().slice(0, 16) : ''} onChange={(e) => up({ occurred: e.target.value ? new Date(e.target.value).toISOString() : undefined })} /></label>
            <label className="flex items-end gap-1 pb-2 text-xs text-slate-400"><input type="checkbox" disabled={dis} checked={!!r.hourUnknown} onChange={(e) => up({ hourUnknown: e.target.checked })} /> hour unknown</label>
            <label className="text-xs text-slate-400 md:col-span-2">Location<input className="input mt-0.5" disabled={dis} value={r.location ?? ''} onChange={(e) => up({ location: e.target.value })} /></label>
            <label className="text-xs text-slate-400 md:col-span-2">Location type
              <Pick dis={dis} value={r.locationType} codes={meta.locations} onChange={(v) => up({ locationType: v })} label="Location type" sort />
            </label>
            <label className="text-xs text-slate-400">Cleared
              <Pick dis={dis} value={r.cleared} codes={c.clearance} onChange={(v) => up({ cleared: v })} label="Cleared" none="Not cleared" />
            </label>
            {r.cleared && r.cleared !== 'arrest' && <label className="text-xs text-slate-400">Cleared on<input type="date" className="input mt-0.5" disabled={dis} value={r.clearedOn ?? ''} onChange={(e) => up({ clearedOn: e.target.value })} /></label>}
          </div>
        </Box>
        <Box title="Offences" add={dis ? undefined : () => up({ offenses: [...r.offenses, { code: '', using: ['N'], bias: '88' }] })}>
          {r.offenses.map((o, i) => {
            const set = (p: Partial<Offense>) => list<Offense>('offenses', i, p)
            const info = offenseName[o.code]
            const groupA = info?.group === 'A'
            const needs = (k: string) => meta.needs[k]?.includes(o.code)
            return (
              <div key={i} className="space-y-1 rounded border border-slate-800 p-1.5">
                <div className="flex flex-wrap items-center gap-2">
                  <select className="input w-80" disabled={dis} value={o.code} onChange={(e) => set({ code: e.target.value })} aria-label="Offence">
                    <option value="">Choose the offence</option>
                    {['person', 'property', 'society'].map((g) => (
                      <optgroup key={g} label={`Crimes against ${g}`}>
                        {meta.offenses.filter((x) => x.against === g).map((x) => <option key={x.code} value={x.code}>{x.code} {x.name}{x.group === 'B' ? ' (arrest only)' : ''}</option>)}
                      </optgroup>
                    ))}
                  </select>
                  {groupA && <label className="flex items-center gap-1 text-xs text-slate-400"><input type="checkbox" disabled={dis} checked={!!o.attempted} onChange={(e) => set({ attempted: e.target.checked })} /> attempted</label>}
                  {groupA && <span className="w-44"><Pick dis={dis} value={o.bias} codes={c.bias} onChange={(v) => set({ bias: v })} label="Bias motivation" /></span>}
                  {info && <span className="text-[10px] text-slate-500">against {info.against}{info.group === 'B' ? ' · arrest only' : ''}</span>}
                  {!dis && <button className="btn btn-ghost ml-auto px-1.5" onClick={() => del('offenses', i)} aria-label="Remove offence"><Trash2 size={12} /></button>}
                </div>
                {groupA && (
                  <>
                    <Chips label="Offender using" dis={dis} codes={c.using} value={o.using} max={3} exclusive="N" onChange={(v) => set({ using: v })} />
                    {(needs('weapon') || (o.weapons?.length ?? 0) > 0) && <Chips label="Weapon or force" dis={dis} codes={c.weapons} value={o.weapons} max={3} exclusive="99" onChange={(v) => set({ weapons: v })} />}
                    {needs('activity') && <Chips label="Criminal activity" dis={dis} codes={pick(c.activity, 'BCDEOPTU')} value={o.activity} max={3} onChange={(v) => set({ activity: v })} />}
                    {needs('gang') && <Chips label="Gang" dis={dis} codes={pick(c.activity, 'JGN')} value={o.activity} max={2} exclusive="N" onChange={(v) => set({ activity: v })} />}
                    {o.code === '720' && <Chips label="Animal cruelty" dis={dis} codes={pick(c.activity, 'AFIS')} value={o.activity} max={3} onChange={(v) => set({ activity: v })} />}
                    {o.code === '220' && (
                      <>
                        <div className="flex flex-wrap items-center gap-2 text-xs text-slate-400">
                          Entry
                          {[['F', 'Force'], ['N', 'No force']].map(([k, l]) => <button key={k} disabled={dis} className={cx('chip', o.entry === k ? 'bg-sky-600 text-white' : 'bg-slate-800 text-slate-300')} onClick={() => set({ entry: k })}>{l}</button>)}
                          {(r.locationType === '14' || r.locationType === '19') && <label className="flex items-center gap-1">premises entered<input type="number" min={1} className="input w-16" disabled={dis} value={o.premises ?? ''} onChange={(e) => set({ premises: Number(e.target.value) })} /></label>}
                          <span className="w-36"><Pick dis={dis} value={o.leftBy} codes={c.leftBy} onChange={(v) => set({ leftBy: v })} label="Left the scene by" none="left by (VA)" /></span>
                        </div>
                        <Chips label="Entry point (VA)" dis={dis} codes={c.points} value={o.entryPts} max={2} onChange={(v) => set({ entryPts: v })} />
                        <Chips label="Security (VA)" dis={dis} codes={c.security} value={o.security} max={2} onChange={(v) => set({ security: v })} />
                      </>
                    )}
                    <label className="flex items-center gap-1 text-[11px] text-slate-400"><input type="checkbox" disabled={dis} checked={!!o.lpr} onChange={(e) => set({ lpr: e.target.checked })} /> a licence plate reader was involved (VA)</label>
                  </>
                )}
              </div>
            )
          })}
        </Box>
        <Box title="People" add={dis ? undefined : () => up({ people: [...r.people, { role: 'victim', name: '', victimType: 'I' }] })}>
          {r.people.map((p, i) => {
            const set = (x: Partial<Person>) => list<Person>('people', i, x)
            const codes = r.offenses.map((o) => o.code)
            const any = (k: string) => codes.some((x) => meta.needs[k]?.includes(x))
            const suspects = r.people.filter((x) => x.role === 'suspect')
            const individual = p.role !== 'victim' || !p.victimType || p.victimType === 'I' || p.victimType === 'L'
            const ageNow = p.dob && r.occurred ? Math.floor((+new Date(r.occurred) - +new Date(p.dob)) / 31557600000) : Number(p.age)
            return (
              <div key={i} className="space-y-1 rounded border border-slate-800 p-1.5">
                <div className="grid grid-cols-2 gap-1 md:grid-cols-8">
                  <select className="input" disabled={dis} value={p.role} onChange={(e) => set({ role: e.target.value })} aria-label="Role">
                    {['victim', 'suspect', 'arrestee', 'witness', 'reporting', 'other'].map((x) => <option key={x}>{x}</option>)}
                  </select>
                  <input className="input md:col-span-2" disabled={dis} placeholder={individual ? 'LAST, FIRST MIDDLE' : 'name of the business or owner'} value={p.name} onChange={(e) => set({ name: e.target.value })} aria-label="Name" />
                  {individual ? (
                    <>
                      <input className="input" disabled={dis} type="date" value={p.dob ?? ''} onChange={(e) => set({ dob: e.target.value })} aria-label="Date of birth" />
                      <input className="input" disabled={dis} placeholder="age" value={p.age ?? ''} onChange={(e) => set({ age: e.target.value })} aria-label="Age" />
                      <select className="input" disabled={dis} value={p.sex ?? ''} onChange={(e) => set({ sex: e.target.value })} aria-label="Sex"><option value="">sex</option><option>M</option><option>F</option>{p.role !== 'arrestee' && <option>U</option>}</select>
                      <Pick dis={dis} value={p.race} codes={c.races} onChange={(v) => set({ race: v })} label="Race" none="race" />
                    </>
                  ) : <span className="md:col-span-4" />}
                  <div className="flex gap-1">
                    <input className="input" disabled={dis} placeholder="phone" value={p.phone ?? ''} onChange={(e) => set({ phone: e.target.value })} aria-label="Phone" />
                    {!dis && <button className="btn btn-ghost px-1.5" onClick={() => del('people', i)} aria-label="Remove person"><Trash2 size={12} /></button>}
                  </div>
                  <input className="input col-span-2 md:col-span-4" disabled={dis} placeholder="address" value={p.address ?? ''} onChange={(e) => set({ address: e.target.value })} aria-label="Address" />
                  {individual && ['victim', 'suspect', 'arrestee'].includes(p.role) && <Pick dis={dis} value={p.ethnicity} codes={c.ethnicities} onChange={(v) => set({ ethnicity: v })} label="Ethnicity" none="ethnicity" />}
                  {individual && (p.role === 'victim' || p.role === 'arrestee') && <Pick dis={dis} value={p.resident} codes={c.residents} onChange={(v) => set({ resident: v })} label="Resident status" none="resident?" />}
                  {p.role === 'victim' && <span className="md:col-span-2"><Pick dis={dis} value={p.victimType || 'I'} codes={c.victimTypes} onChange={(v) => set({ victimType: v })} label="Type of victim" /></span>}
                </div>
                {p.role === 'victim' && individual && (
                  <>
                    {any('injury') && <Chips label="Injury" dis={dis} codes={c.injuries} value={p.injuries} max={5} exclusive="N" onChange={(v) => set({ injuries: v })} />}
                    {any('circumstances') && <Chips label="Circumstances" dis={dis} codes={c.circumstances} value={p.circumstances} max={2} onChange={(v) => set({ circumstances: v })} />}
                    {any('relation') && suspects.map((sx, k) => {
                      const cur = p.relations?.find((x) => x.suspect === k + 1)?.code ?? (k === 0 ? p.relation : '') ?? ''
                      return (
                        <div key={k} className="flex items-center gap-2 text-xs text-slate-400">to suspect {k + 1} {sx.name && `(${sx.name})`}
                          <span className="w-56"><Pick dis={dis} value={cur} codes={c.relations} onChange={(v) => set({ relations: [...(p.relations ?? []).filter((x) => x.suspect !== k + 1), { suspect: k + 1, code: v }] })} label={`Relationship to suspect ${k + 1}`} none="relationship" /></span>
                        </div>
                      )
                    })}
                    {p.victimType === 'L' && (
                      <div className="grid gap-1 md:grid-cols-4">
                        <Pick dis={dis} value={p.officerAct} codes={c.officer} onChange={(v) => set({ officerAct: v })} label="Officer activity" none="activity" />
                        <Pick dis={dis} value={p.officerAssign} codes={c.assignment} onChange={(v) => set({ officerAssign: v })} label="Officer assignment" none="assignment" />
                        <input className="input font-mono" disabled={dis} placeholder="other agency ORI" value={p.officerOri ?? ''} onChange={(e) => set({ officerOri: e.target.value.toUpperCase() })} aria-label="Officer's ORI" />
                        <Pick dis={dis} value={p.assaultStatus} codes={{ C: 'Cleared', P: 'Pending' }} onChange={(v) => set({ assaultStatus: v })} label="Assault status (VA)" none="assault status (VA)" />
                      </div>
                    )}
                  </>
                )}
                {p.role === 'arrestee' && (
                  <div className="grid gap-1 md:grid-cols-4">
                    <label className="text-[11px] text-slate-400">Arrested<input type="date" className="input" disabled={dis} value={p.arrestDate ?? ''} onChange={(e) => set({ arrestDate: e.target.value })} /></label>
                    <label className="text-[11px] text-slate-400">Type<Pick dis={dis} value={p.arrestType} codes={c.arrestTypes} onChange={(v) => set({ arrestType: v })} label="Type of arrest" none="type of arrest" /></label>
                    <label className="text-[11px] text-slate-400">For<select className="input" disabled={dis} value={p.arrestOffense ?? ''} onChange={(e) => set({ arrestOffense: e.target.value })} aria-label="Arrest offence">
                      <option value="">{r.offenses[0]?.code ? `${r.offenses[0].code} (the first offence)` : '—'}</option>
                      {meta.offenses.map((x) => <option key={x.code} value={x.code}>{x.code} {x.name}</option>)}
                    </select></label>
                    <label className="text-[11px] text-slate-400">Arrest number<input className="input font-mono" disabled={dis} placeholder={r.number} value={p.arrestNo ?? ''} onChange={(e) => set({ arrestNo: e.target.value.toUpperCase() })} /></label>
                    <div className="md:col-span-4"><Chips label="Armed with" dis={dis} codes={c.armed} value={p.armed} max={2} exclusive="01" onChange={(v) => set({ armed: v })} /></div>
                    {ageNow > 0 && ageNow < 18 && <span className="md:col-span-2"><Pick dis={dis} value={p.juvenile} codes={c.juvenile} onChange={(v) => set({ juvenile: v })} label="Under 18: handled" none="under 18: how handled" /></span>}
                    {(p.arrestOffense || r.offenses[0]?.code) === '35A' && <div className="md:col-span-4"><Chips label="Drug (VA)" dis={dis} codes={c.drugs} value={p.arrestDrugs} max={3} onChange={(v) => set({ arrestDrugs: v })} /></div>}
                  </div>
                )}
              </div>
            )
          })}
        </Box>
        <Box title="Property" add={dis ? undefined : () => up({ property: [...r.property, { loss: 'stolen', type: '', quantity: 1 }] })}>
          {r.property.map((p, i) => {
            const set = (x: Partial<Property>) => list<Property>('property', i, x)
            return (
              <div key={i} className="flex flex-wrap items-center gap-1">
                <select className="input w-32" disabled={dis} value={p.loss} onChange={(e) => set({ loss: e.target.value })} aria-label="Loss">
                  {[...Object.keys(c.loss), 'evidence'].map((x) => <option key={x}>{x}</option>)}
                </select>
                <span className="w-56"><Pick dis={dis} value={p.type} codes={c.property} onChange={(v) => set({ type: v })} label="Property type" none="type" sort /></span>
                <input className="input w-16" disabled={dis} type="number" min={0} placeholder="qty" value={p.quantity ?? ''} onChange={(e) => set({ quantity: Number(e.target.value) })} aria-label="Quantity" />
                <input className="input min-w-32 flex-1" disabled={dis} placeholder="description" value={p.description ?? ''} onChange={(e) => set({ description: e.target.value })} aria-label="Description" />
                <input className="input w-24" disabled={dis} type="number" placeholder="value $" value={p.value ?? ''} onChange={(e) => set({ value: Number(e.target.value) })} aria-label="Value" />
                <input className="input w-32" disabled={dis} placeholder="serial" value={p.serial ?? ''} onChange={(e) => set({ serial: e.target.value })} aria-label="Serial" />
                {p.loss === 'recovered' && <input className="input w-36" disabled={dis} type="date" value={p.recoveredOn ?? ''} onChange={(e) => set({ recoveredOn: e.target.value })} aria-label="Recovered on" />}
                {p.loss === 'seized' && p.type === '10' && (
                  <>
                    <span className="w-48"><Pick dis={dis} value={p.drug} codes={c.drugs} onChange={(v) => set({ drug: v })} label="Drug" none="drug" sort /></span>
                    <input className="input w-20" disabled={dis} type="number" step="0.001" placeholder="amount" value={p.drugQty ?? ''} onChange={(e) => set({ drugQty: Number(e.target.value) })} aria-label="Drug amount" />
                    <span className="w-32"><Pick dis={dis} value={p.drugUnit} codes={c.drugUnits} onChange={(v) => set({ drugUnit: v })} label="Measurement" none="unit" /></span>
                  </>
                )}
                {!dis && <button className="btn btn-ghost px-1.5" onClick={() => del('property', i)} aria-label="Remove property"><Trash2 size={12} /></button>}
              </div>
            )
          })}
        </Box>
        <Box title="Vehicles" add={dis ? undefined : () => up({ vehicles: [...r.vehicles, { role: 'involved' }] })}>
          {r.vehicles.map((v, i) => (
            <div key={i} className="flex flex-wrap items-center gap-1">
              <select className="input w-28" disabled={dis} value={v.role ?? ''} onChange={(e) => list<Vehicle>('vehicles', i, { role: e.target.value })} aria-label="Role">
                {['involved', 'stolen', 'suspect', 'victim', 'recovered'].map((x) => <option key={x}>{x}</option>)}
              </select>
              {(['plate', 'state', 'year', 'make', 'model', 'color', 'vin'] as const).map((f) => (
                <input key={f} className={cx('input font-mono', f === 'vin' ? 'w-52' : f === 'state' || f === 'year' ? 'w-16' : 'w-28')} disabled={dis} placeholder={f} value={v[f] ?? ''} onChange={(e) => list<Vehicle>('vehicles', i, { [f]: e.target.value.toUpperCase() })} aria-label={f} />
              ))}
              {!dis && <button className="btn btn-ghost px-1.5" onClick={() => del('vehicles', i)} aria-label="Remove vehicle"><Trash2 size={12} /></button>}
            </div>
          ))}
        </Box>
        <Box title="Narrative">
          {!dis && r.incident && <DraftButton incident={r.incident} has={!!r.narrative?.trim()} onDraft={(t) => up({ narrative: t })} />}
          <textarea className="input min-h-64 font-[inherit] text-sm leading-relaxed" disabled={dis} value={r.narrative ?? ''} onChange={(e) => up({ narrative: e.target.value })} aria-label="Narrative" placeholder="What happened, in your words." />
        </Box>
      </div>
      <aside className="space-y-3">
        <div className="panel sticky top-0 space-y-2 rounded-lg p-3">
          <h2 className="text-[11px] font-bold uppercase tracking-wider text-slate-400">Before it is submitted</h2>
          {checks.length === 0 ? <p className="flex items-center gap-1 text-sm text-emerald-300"><CheckCircle2 size={14} aria-hidden /> Complete for NIBRS</p> : (
            <ul className="space-y-1 text-xs">
              {checks.map((c, i) => <li key={i} className="flex gap-1 text-amber-200"><CircleAlert size={12} className="mt-0.5 shrink-0" aria-hidden /> {c.message}</li>)}
            </ul>
          )}
          <p className="text-[10px] text-slate-500">The FBI NIBRS data elements with Virginia's additions, checked as the state repository checks them.</p>
          {agency.length > 0 && (
            <div className="rounded border border-sky-800 bg-sky-950/30 p-1.5">
              <div className="text-[10px] font-bold uppercase tracking-wider text-sky-300">Agency checks (advice)</div>
              <ul className="space-y-0.5 text-xs text-sky-100">{agency.map((c, i) => <li key={i}>{c.message}</li>)}</ul>
            </div>
          )}
          {editable && <button className="btn btn-go w-full" disabled={checks.length > 0} onClick={() => status('submitted')}><Send size={13} aria-hidden /> Submit for approval</button>}
          {r.status === 'submitted' && sup && (
            <>
              <button className="btn btn-go w-full" onClick={() => status('approved')}><BadgeCheck size={13} aria-hidden /> Approve</button>
              <input className="input text-xs" placeholder="What to fix (to return it)" value={note} onChange={(e) => setNote(e.target.value)} />
              <button className="btn w-full" disabled={!note.trim()} onClick={() => status('returned')}><Undo2 size={13} aria-hidden /> Return to {r.author}</button>
            </>
          )}
          {r.status === 'submitted' && !sup && <p className="text-xs text-slate-400">Waiting for a supervisor.</p>}
          <button className="btn w-full text-xs" onClick={() => window.print()}>Print</button>
        </div>
      </aside>
    </div>
  )
}

const pick = (codes: Record<string, string>, keys: string) => Object.fromEntries(Object.entries(codes).filter(([k]) => keys.includes(k)))

/** A code list as a select, showing the words and keeping the code. */
function Pick({ value, codes, onChange, label, none, dis, sort }: { value?: string; codes: Record<string, string>; onChange: (v: string) => void; label: string; none?: string; dis?: boolean; sort?: boolean }) {
  // Codes in code order: JavaScript would put "10" before "01".
  const entries = Object.entries(codes).sort((a, b) => a[0].localeCompare(b[0]))
  if (sort) entries.sort((a, b) => a[1].localeCompare(b[1]))
  return (
    <select className="input mt-0.5" disabled={dis} value={value ?? ''} onChange={(e) => onChange(e.target.value)} aria-label={label}>
      {none !== undefined || !value ? <option value="">{none ?? '—'}</option> : null}
      {entries.map(([k, n]) => <option key={k} value={k}>{n}</option>)}
    </select>
  )
}

/** A short code list as one-click chips: up to max chosen; the exclusive code ("none") clears the rest. */
function Chips({ label, codes, value = [], onChange, max, exclusive, dis }: { label: string; codes: Record<string, string>; value?: string[]; onChange: (v: string[]) => void; max: number; exclusive?: string; dis?: boolean }) {
  const toggle = (k: string) => {
    if (value.includes(k)) return onChange(value.filter((x) => x !== k))
    if (k === exclusive) return onChange([k])
    const rest = value.filter((x) => x !== exclusive)
    onChange(rest.length >= max ? [...rest.slice(1), k] : [...rest, k])
  }
  return (
    <div className="flex flex-wrap items-center gap-1" role="group" aria-label={label}>
      <span className="w-28 shrink-0 text-[11px] text-slate-400">{label}</span>
      {Object.entries(codes).sort((a, b) => a[0].localeCompare(b[0])).map(([k, n]) => (
        <button key={k} type="button" disabled={dis} aria-pressed={value.includes(k)} onClick={() => toggle(k)}
          className={cx('rounded px-1.5 py-0.5 text-[11px]', value.includes(k) ? 'bg-sky-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700', dis && 'opacity-60')}>{n}</button>
      ))}
    </div>
  )
}

function Box({ title, add, children }: { title: string; add?: () => void; children: React.ReactNode }) {
  return (
    <section className="panel space-y-2 rounded-lg p-3">
      <h2 className="flex items-center text-[11px] font-bold uppercase tracking-wider text-slate-400">{title}
        {add && <button className="btn ml-auto h-6 px-1.5 py-0 text-[11px] normal-case tracking-normal" onClick={add}><Plus size={11} aria-hidden /> Add</button>}</h2>
      {children}
    </section>
  )
}

/** DraftButton fills the narrative with a first draft from the incident's record (or the language model,
 * when the agency turned it on, from the same facts). It is a proposal: read it, correct it, finish it. */
function DraftButton({ incident, has, onDraft }: { incident: string; has: boolean; onDraft: (t: string) => void }) {
  const [busy, setBusy] = useState(false)
  const [by, setBy] = useState('')
  return (
    <div className="mb-1.5 flex items-center gap-2 text-xs">
      <button className="btn h-7 text-xs" disabled={busy} onClick={async () => {
        if (has && !confirm('Replace the narrative with a draft from the incident?')) return
        setBusy(true)
        try {
          const d = await api.post<{ text: string; by: string }>(`/incidents/${encodeURIComponent(incident)}/draft-narrative`)
          onDraft(d.text)
          setBy(d.by)
        } catch (e) {
          toast.error(errText(e))
        } finally {
          setBusy(false)
        }
      }}><Sparkles size={12} aria-hidden /> {busy ? 'Drafting…' : 'Draft from the incident'}</button>
      {by && <span className="rounded bg-amber-500/15 px-2 py-0.5 text-amber-200">A draft {by === 'model' ? 'written by the language model' : 'from the record'}: check every line and finish it in your words.</span>}
    </div>
  )
}
