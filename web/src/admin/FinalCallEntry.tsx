// Call entry rules: how unverified locations are handled and reviewed, who may open people's protected
// identifiers, the agency's vehicle defaults and code additions, the NCIC and state request queue, and the
// operational periods and the Shift Intelligence report. Each section edits one sub-object of the settings
// and saves the whole settings object with only that sub-object changed.

import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { toast } from 'sonner'
import { Car, ClipboardList, FileText, MapPin, Plus, Save, Trash2, UserCheck } from 'lucide-react'
import { api, errText } from '../api'
import { Field } from '../ui'
import { useStore } from '../store'
import { Page, Table } from './Admin'
import {
  DUTIES, REPORT_SECTIONS, REQUEST_STATUSES,
  asNum, fromLines, parseList, saveSettings, toLines,
  type CodeList, type CodesResponse, type FinalSettings, type PersonPolicy, type PlacePolicy,
  type RequestPolicy, type ShiftPeriod, type ShiftPolicy, type VehCode, type VehiclePolicy,
} from './FinalShared'

/** CallEntryRules edits the parts of settings that shape call entry and the request queue. */
export function CallEntryRules() {
  const settings = useStore((s) => s.view?.settings) as FinalSettings | undefined

  return (
    <Page
      title="Call entry rules"
      intro="How unverified locations are forced and reviewed, who may open protected identifiers, the vehicle defaults and agency codes, the NCIC and state request queue, and the operational periods and shift report."
    >
      <div className="space-y-4">
        <UnverifiedSection live={settings?.places} />
        <IdentifiersSection live={settings?.persons} />
        <VehiclesSection live={settings?.vehicles} />
        <RequestsSection live={settings?.requests} />
        <PeriodsSection live={settings?.shift} />
      </div>
    </Page>
  )
}

// A section wrapper with its own save button.
function Section({ id, title, icon, intro, dirty, onSave, children }: {
  id: string; title: string; icon: ReactNode; intro: string; dirty: boolean; onSave: () => void; children: ReactNode
}) {
  return (
    <section className="panel space-y-3 rounded-lg p-4" aria-labelledby={`${id}-h`}>
      <div className="flex items-center justify-between gap-2">
        <h2 id={`${id}-h`} className="flex items-center gap-2 font-semibold text-slate-100">{icon} {title}</h2>
        <button className="btn btn-go" disabled={!dirty} onClick={onSave}><Save size={14} aria-hidden /> Save</button>
      </div>
      <p className="text-xs text-slate-400">{intro}</p>
      {children}
    </section>
  )
}

function useDraft<T extends object>(live: T | undefined): [T, (p: Partial<T>) => void, boolean, () => void] {
  const [d, setD] = useState<T>(live ?? ({} as T))
  const [dirty, setDirty] = useState(false)
  useEffect(() => { if (!dirty) setD(live ?? ({} as T)) }, [live, dirty])
  const up = (p: Partial<T>) => { setD((cur) => ({ ...cur, ...p })); setDirty(true) }
  const clean = () => setDirty(false)
  return [d, up, dirty, clean]
}

// ---- 1) unverified locations ----

function UnverifiedSection({ live }: { live?: PlacePolicy }) {
  const [d, up, dirty, clean] = useDraft<PlacePolicy>(live)
  const toggleDuty = (duty: string) => {
    const cur = d.forceDuties ?? []
    up({ forceDuties: cur.includes(duty) ? cur.filter((x) => x !== duty) : [...cur, duty] })
  }
  const save = async () => {
    try { await saveSettings({ places: d }); toast.success('Unverified location rules saved'); clean() } catch (e) { toast.error(errText(e)) }
  }
  return (
    <Section id="unverified" title="Unverified locations" icon={<MapPin size={15} className="text-sky-300" aria-hidden />} dirty={dirty} onSave={save}
      intro="A call is never held up for an address. These rules say who may force a location, the reasons offered, and the review and follow-up that every forced or unverified location opens.">
      <div>
        <span className="label">Duties that may force a location</span>
        <div className="flex flex-wrap gap-3">
          {DUTIES.map((duty) => (
            <label key={duty} className="flex items-center gap-1 text-xs text-slate-300">
              <input type="checkbox" aria-label={`Force a location: ${duty}`} checked={(d.forceDuties ?? []).includes(duty)} onChange={() => toggleDuty(duty)} /> {duty}
            </label>
          ))}
        </div>
        <p className="mt-1 text-xs text-slate-500">Empty uses the usual rule: call-takers, dispatchers and supervisors.</p>
      </div>
      <div className="flex flex-wrap gap-4">
        <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!d.traineesForce} onChange={(e) => up({ traineesForce: e.target.checked })} /> Trainees may force a location</label>
        <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!d.forceNoteAlways} onChange={(e) => up({ forceNoteAlways: e.target.checked })} /> A written note with every forced location</label>
        <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!d.supervisorApprove} onChange={(e) => up({ supervisorApprove: e.target.checked })} /> A supervisor is alerted to acknowledge it (never holds up dispatch)</label>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Reasons for forcing (one per line)" htmlFor="pl-reasons" hint="Empty uses the defaults. A reason ending in (explain) asks for a note.">
          <textarea id="pl-reasons" className="input min-h-[4rem] text-xs" value={toLines(d.forceReasons)} onChange={(e) => up({ forceReasons: fromLines(e.target.value) })} />
        </Field>
        <Field label="Review dispositions (one per line)" htmlFor="pl-disp" hint="How a reviewer closes a location exception; empty uses the defaults.">
          <textarea id="pl-disp" className="input min-h-[4rem] text-xs" value={toLines(d.dispositions)} onChange={(e) => up({ dispositions: fromLines(e.target.value) })} />
        </Field>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Follow-up due in (days)" htmlFor="pl-fu" hint="The default due date of a follow-up (0 uses 7).">
          <input id="pl-fu" type="number" min={0} className="input w-32 font-mono" value={d.followupDays ?? ''} onChange={(e) => up({ followupDays: asNum(e.target.value) })} />
        </Field>
        <Field label="Sublocation kinds" htmlFor="pl-kinds" hint="Comma list; empty uses apartment, suite, room, unit, lot, space.">
          <input id="pl-kinds" className="input" value={(d.unitKinds ?? []).join(', ')} onChange={(e) => up({ unitKinds: parseList(e.target.value) })} />
        </Field>
      </div>
    </Section>
  )
}

// ---- 2) protected identifiers ----

function IdentifiersSection({ live }: { live?: PersonPolicy }) {
  const [d, up, dirty, clean] = useDraft<PersonPolicy>(live)
  const toggleDuty = (duty: string) => {
    const cur = d.identDuties ?? []
    up({ identDuties: cur.includes(duty) ? cur.filter((x) => x !== duty) : [...cur, duty] })
  }
  const save = async () => {
    try { await saveSettings({ persons: d }); toast.success('Identifier rules saved'); clean() } catch (e) { toast.error(errText(e)) }
  }
  return (
    <Section id="ident" title="Protected identifiers" icon={<UserCheck size={15} className="text-sky-300" aria-hidden />} dirty={dirty} onSave={save}
      intro="A Social Security or driver's licence number is masked on the board. These duties may open the full number, and every opening is recorded with a reason.">
      <div>
        <span className="label">Duties that may open full identifiers</span>
        <div className="flex flex-wrap gap-3">
          {DUTIES.map((duty) => (
            <label key={duty} className="flex items-center gap-1 text-xs text-slate-300">
              <input type="checkbox" aria-label={`Open identifiers: ${duty}`} checked={(d.identDuties ?? []).includes(duty)} onChange={() => toggleDuty(duty)} /> {duty}
            </label>
          ))}
        </div>
        <p className="mt-1 text-xs text-slate-500">Empty uses the usual rule: police, dispatchers, supervisors and records.</p>
      </div>
    </Section>
  )
}

// ---- 3) vehicles ----

function VehiclesSection({ live }: { live?: VehiclePolicy }) {
  const [d, up, dirty, clean] = useDraft<VehiclePolicy>(live)
  const [lists, setLists] = useState<CodeList[]>([])
  useEffect(() => {
    api.get<CodesResponse>('/codes').then((r) => setLists(r.lists ?? [])).catch((e) => toast.error(errText(e)))
  }, [])

  const stateList = lists.find((l) => l.id === 'state')
  const codes = d.codes ?? {}
  const setRows = (listId: string, rows: VehCode[]) => up({ codes: { ...codes, [listId]: rows } })
  const [addList, setAddList] = useState('state')

  const save = async () => {
    try { await saveSettings({ vehicles: d }); toast.success('Vehicle rules saved'); clean() } catch (e) { toast.error(errText(e)) }
  }

  return (
    <Section id="vehicles" title="Vehicles" icon={<Car size={15} className="text-sky-300" aria-hidden />} dirty={dirty} onSave={save}
      intro="The plate state offered first, and the agency's additions to the controlled code lists (state, plate type, colour, body, make and so on). Additions sit beside the standard codes.">
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Default plate state" htmlFor="vh-state" hint="Offered first on a new plate. Empty uses the agency's ORI or VA.">
          <select id="vh-state" className="input" value={d.defaultState ?? ''} onChange={(e) => up({ defaultState: e.target.value || undefined })}>
            <option value="">use the default</option>
            {(stateList?.codes ?? []).map((c) => <option key={c.code} value={c.code}>{c.code} · {c.label}</option>)}
          </select>
        </Field>
      </div>

      <div className="space-y-3">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500">Agency code additions</h3>
        {Object.keys(codes).length === 0 && <p className="text-sm text-slate-500">No additions yet.</p>}
        {Object.entries(codes).map(([listId, rows]) => {
          const list = lists.find((l) => l.id === listId)
          return (
            <div key={listId} className="rounded-md border border-slate-800 p-3">
              <div className="mb-1 flex items-center justify-between">
                <span className="text-sm font-semibold text-slate-100">{list?.name ?? listId} <span className="font-mono text-xs text-slate-500">({listId})</span></span>
                <button className="btn btn-ghost px-1.5" aria-label={`Remove all ${listId} additions`} onClick={() => { const n = { ...codes }; delete n[listId]; up({ codes: n }) }}><Trash2 size={13} aria-hidden /></button>
              </div>
              <Table head={['Code', 'Label', '']}>
                {rows.map((row, i) => (
                  <tr key={i} className="align-top">
                    <td className="px-2 py-1"><input className="input h-7 w-24 font-mono text-xs" aria-label={`${listId} code ${i + 1}`} value={row.code} onChange={(e) => setRows(listId, rows.map((x, j) => (j === i ? { ...x, code: e.target.value } : x)))} /></td>
                    <td className="px-2"><input className="input h-7 text-xs" aria-label={`${listId} label ${i + 1}`} value={row.label} onChange={(e) => setRows(listId, rows.map((x, j) => (j === i ? { ...x, label: e.target.value } : x)))} /></td>
                    <td className="px-2"><button className="btn btn-ghost px-1.5" aria-label={`Remove ${listId} row ${i + 1}`} onClick={() => setRows(listId, rows.filter((_, j) => j !== i))}><Trash2 size={13} aria-hidden /></button></td>
                  </tr>
                ))}
              </Table>
              <button className="btn mt-2" onClick={() => setRows(listId, [...rows, { code: '', label: '' }])}><Plus size={13} aria-hidden /> Add a {listId} code</button>
            </div>
          )
        })}
        <div className="flex items-end gap-2">
          <Field label="Add codes to a list" htmlFor="vh-addlist">
            <select id="vh-addlist" className="input h-8 text-sm" value={addList} onChange={(e) => setAddList(e.target.value)}>
              {lists.map((l) => <option key={l.id} value={l.id}>{l.name}</option>)}
            </select>
          </Field>
          <button className="btn" onClick={() => { if (!codes[addList]) setRows(addList, [{ code: '', label: '' }]) }}><Plus size={13} aria-hidden /> Start additions</button>
        </div>
      </div>
    </Section>
  )
}

// ---- 4) NCIC requests ----

function RequestsSection({ live }: { live?: RequestPolicy }) {
  const [d, up, dirty, clean] = useDraft<RequestPolicy>(live)
  const labels = d.labels ?? {}
  const setLabel = (status: string, v: string) => {
    const n = { ...labels }
    if (v.trim() === '') delete n[status]
    else n[status] = v
    up({ labels: n })
  }
  const save = async () => {
    try { await saveSettings({ requests: d }); toast.success('Request queue rules saved'); clean() } catch (e) { toast.error(errText(e)) }
  }
  return (
    <Section id="requests" title="NCIC requests" icon={<ClipboardList size={15} className="text-sky-300" aria-hidden />} dirty={dirty} onSave={save}
      intro="The queue that accounts for every DMV, VCIC, NCIC, Nlets and other-state transaction and every hit confirmation. Rename the statuses, set the handoff queues, and set how long a request or a hit confirmation has before it is overdue.">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Field label="Urgent hit confirmation (minutes)" htmlFor="rq-urgent"><input id="rq-urgent" type="number" min={0} className="input font-mono" value={d.urgentMinutes ?? ''} onChange={(e) => up({ urgentMinutes: asNum(e.target.value) })} /></Field>
        <Field label="Routine hit confirmation (minutes)" htmlFor="rq-routine"><input id="rq-routine" type="number" min={0} className="input font-mono" value={d.routineMinutes ?? ''} onChange={(e) => up({ routineMinutes: asNum(e.target.value) })} /></Field>
        <Field label="Ordinary request due (minutes)" htmlFor="rq-due" hint="How long an ordinary request may stay open (0 uses 15)."><input id="rq-due" type="number" min={0} className="input font-mono" value={d.dueMinutes ?? ''} onChange={(e) => up({ dueMinutes: asNum(e.target.value) })} /></Field>
        <Field label="Warn before due (minutes)" htmlFor="rq-warn" hint="Shown as nearly due this long before (0 uses 3)."><input id="rq-warn" type="number" min={0} className="input font-mono" value={d.warnMinutes ?? ''} onChange={(e) => up({ warnMinutes: asNum(e.target.value) })} /></Field>
      </div>
      <p className="rounded bg-slate-800/60 px-2 py-1 text-xs text-slate-300">NCIC policy: 10 minutes urgent, 1 hour routine. Blank fields use those.</p>
      <Field label="Handoff queues (one per line)" htmlFor="rq-queues" hint="Where a request can be handed when no one accepts it directly; empty uses Records desk and Supervisor.">
        <textarea id="rq-queues" className="input min-h-[3rem] text-xs" value={toLines(d.queues)} onChange={(e) => up({ queues: fromLines(e.target.value) })} />
      </Field>
      <label className="flex items-start gap-2 text-sm text-slate-300">
        <input type="checkbox" className="mt-1" checked={!!d.signOutCheckOff} onChange={(e) => up({ signOutCheckOff: e.target.checked })} />
        <span>Let people sign off with open requests<span className="block text-xs text-amber-200">On by default the sign-off check stops someone leaving an open request behind. Turning it off removes that safeguard.</span></span>
      </label>
      <div>
        <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500">Status names</h3>
        <Table head={['Status', 'Agency name']}>
          {REQUEST_STATUSES.map((s) => (
            <tr key={s.code}>
              <td className="px-3 py-1 text-sm text-slate-400">{s.label} <span className="font-mono text-xs text-slate-600">{s.code}</span></td>
              <td className="px-3"><input className="input h-7 text-xs" aria-label={`Agency name for ${s.label}`} value={labels[s.code] ?? ''} onChange={(e) => setLabel(s.code, e.target.value)} placeholder={s.label} /></td>
            </tr>
          ))}
        </Table>
      </div>
    </Section>
  )
}

// ---- 5) operational periods and the shift report ----

function PeriodsSection({ live }: { live?: ShiftPolicy }) {
  const [d, up, dirty, clean] = useDraft<ShiftPolicy>(live)
  const periods = d.periods ?? []
  const setPeriod = (i: number, patch: Partial<ShiftPeriod>) => up({ periods: periods.map((p, j) => (j === i ? { ...p, ...patch } : p)) })
  const sections = d.sections ?? []
  const toggleSection = (code: string) => up({ sections: sections.includes(code) ? sections.filter((x) => x !== code) : [...sections, code] })
  const toggleRecipient = (duty: string) => {
    const cur = d.recipients ?? []
    up({ recipients: cur.includes(duty) ? cur.filter((x) => x !== duty) : [...cur, duty] })
  }
  const save = async () => {
    try { await saveSettings({ shift: d }); toast.success('Operational period rules saved'); clean() } catch (e) { toast.error(errText(e)) }
  }

  // The sections to show, in the chosen order, then any not yet chosen.
  const orderedCodes = [...sections.filter((c) => REPORT_SECTIONS.some((s) => s.code === c)), ...REPORT_SECTIONS.map((s) => s.code).filter((c) => !sections.includes(c))]

  return (
    <Section id="periods" title="Operational periods" icon={<FileText size={15} className="text-sky-300" aria-hidden />} dirty={dirty} onSave={save}
      intro="The operational periods by name and start time, and what the Shift Intelligence report shows at a period's end. The report is built from records, not typed again.">
      <div>
        <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500">Periods</h3>
        <Table head={['Name', 'Start (HH:MM)', '']} empty={periods.length === 0 ? <p className="p-3 text-sm text-slate-500">Empty uses Day at 07:00 and Night at 19:00.</p> : undefined}>
          {periods.map((p, i) => (
            <tr key={i} className="align-top">
              <td className="px-3 py-1"><input className="input h-7 text-xs" aria-label={`Period name ${i + 1}`} value={p.name} onChange={(e) => setPeriod(i, { name: e.target.value })} placeholder="Day" /></td>
              <td className="px-3"><input type="time" className="input h-7 w-28 font-mono text-xs" aria-label={`Period start ${i + 1}`} value={p.start} onChange={(e) => setPeriod(i, { start: e.target.value })} /></td>
              <td className="px-3"><button className="btn btn-ghost px-1.5" aria-label={`Remove period ${i + 1}`} onClick={() => up({ periods: periods.filter((_, j) => j !== i) })}><Trash2 size={13} aria-hidden /></button></td>
            </tr>
          ))}
        </Table>
        <button className="btn mt-2" onClick={() => up({ periods: [...periods, { name: '', start: '07:00' }] })}><Plus size={13} aria-hidden /> Add a period</button>
      </div>
      <Field label="Time zone" htmlFor="sh-tz" hint="The agency's time zone for periods, like America/New_York; empty uses the server's.">
        <input id="sh-tz" className="input w-64 font-mono" value={d.timeZone ?? ''} onChange={(e) => up({ timeZone: e.target.value })} placeholder="America/New_York" />
      </Field>
      <div>
        <span className="label">Report sections to show</span>
        <div className="flex flex-wrap gap-3">
          {orderedCodes.map((code) => {
            const s = REPORT_SECTIONS.find((x) => x.code === code)!
            return (
              <label key={code} className="flex items-center gap-1 text-xs text-slate-300">
                <input type="checkbox" aria-label={`Show ${s.label}`} checked={sections.length === 0 || sections.includes(code)} onChange={() => toggleSection(code)} /> {s.label}
              </label>
            )
          })}
        </div>
        <p className="mt-1 text-xs text-slate-500">None ticked shows every section. Ticking sets the order they appear.</p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <Field label="Major priority (this or more urgent)" htmlFor="sh-major" hint="Incidents at this priority or more urgent are listed as major (0 uses 2).">
          <input id="sh-major" type="number" min={1} max={5} className="input font-mono" value={d.majorPriority ?? ''} onChange={(e) => up({ majorPriority: asNum(e.target.value) })} />
        </Field>
        <Field label="Workload prompt ratio" htmlFor="sh-ratio" hint="A person handling this many times their duty's median is a review prompt (0 uses 2).">
          <input id="sh-ratio" type="number" min={0} step={0.5} className="input font-mono" value={d.workloadRatio ?? ''} onChange={(e) => up({ workloadRatio: asNum(e.target.value) })} />
        </Field>
        <div className="flex items-end">
          <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!d.requireSummary} onChange={(e) => up({ requireSummary: e.target.checked })} /> Ask the supervisor for a summary before finalizing</label>
        </div>
      </div>
      <div>
        <span className="label">Who may read the report</span>
        <div className="flex flex-wrap gap-3">
          {DUTIES.map((duty) => (
            <label key={duty} className="flex items-center gap-1 text-xs text-slate-300">
              <input type="checkbox" aria-label={`Read the report: ${duty}`} checked={(d.recipients ?? []).includes(duty)} onChange={() => toggleRecipient(duty)} /> {duty}
            </label>
          ))}
        </div>
        <p className="mt-1 text-xs text-slate-500">Empty means supervisors.</p>
      </div>
    </Section>
  )
}
