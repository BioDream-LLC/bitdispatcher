import { useState } from 'react'
import { ClipboardList, UserCheck } from 'lucide-react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import { cx } from '../util'
import type { PeriodsResponse, ReportItem, ShiftReport, ShiftReportResponse } from './types'
import { Busy, Empty, NoteField, Panel, Tag, dateTime, run, usePoll } from './common'

type Mode = { kind: 'period'; id: string } | { kind: 'range'; from: string; to: string }

/** ShiftIntel is tab 4: the period's report assembled from the records, with the supervisor's summary
 * and handoff kept apart from the facts, finalize and addendum, and the incoming acknowledgement. */
export default function ShiftIntel({ mode, periods, setMode }: {
  mode: Mode
  periods: PeriodsResponse | null
  setMode: (m: Mode) => void
}) {
  const [draft, setDraft] = useState<ShiftReport | null>(null)
  const [issued, setIssued] = useState<ShiftReport[]>([])

  const query = mode.kind === 'range' ? `from=${encodeURIComponent(mode.from)}&to=${encodeURIComponent(mode.to)}` : `period=${encodeURIComponent(mode.id)}`

  const refresh = usePoll(async () => {
    const r = await api.get<ShiftReportResponse>(`/shift/report?${query}`)
    setDraft(r.draft)
    setIssued(r.issued ?? [])
  }, [query])

  const names = periods?.names ?? {}
  const sections = periods?.sections ?? []
  const items = draft?.items ?? []
  const bySection = sections.map((s) => ({ section: s, name: names[s] ?? s, items: items.filter((it) => it.section === s) })).filter((g) => g.items.length > 0)
  const periodId = draft?.period.id ?? (mode.kind === 'period' ? mode.id : '')
  const isPeriod = mode.kind === 'period' && !periodId.startsWith('custom:')

  return (
    <div className="space-y-3">
      <PeriodPicker mode={mode} periods={periods} setMode={setMode} />

      <Panel icon={<ClipboardList size={16} className="text-sky-300" aria-hidden />} title={`Draft${draft ? ` · ${draft.period.name}` : ''}`}>
        {bySection.length === 0 ? (
          <Empty>Nothing to report for this period yet.</Empty>
        ) : (
          <div className="space-y-4">
            {bySection.map((g) => (
              <div key={g.section}>
                <h4 className="mb-1 text-xs font-bold uppercase tracking-wider text-slate-400">{g.name}</h4>
                <ul className="space-y-1">
                  {g.items.map((it) => <Item key={it.section + it.key} it={it} />)}
                </ul>
              </div>
            ))}
          </div>
        )}
      </Panel>

      {isPeriod && <Finalize periodId={periodId} issued={issued} onSaved={refresh} />}

      <Panel title="Issued versions">
        {issued.length === 0 ? (
          <Empty>Not finalized yet.</Empty>
        ) : (
          <ul className="space-y-2">
            {issued.map((r) => (
              <li key={r.version} className="rounded-md border border-slate-800 bg-slate-900/50 p-2 text-sm">
                <div className="flex flex-wrap items-center gap-2">
                  <b>Version {r.version}</b>
                  {r.addendum && <Tag tone="warn">addendum</Tag>}
                  <span className="text-xs text-slate-500">by {r.by} · {dateTime(r.at)}</span>
                </div>
                {r.note && <div className="text-xs text-slate-400">Why: {r.note}</div>}
                {r.summary && <div className="mt-1"><span className="text-slate-500">Summary (observation):</span> {r.summary}</div>}
                {r.handoff && <div><span className="text-slate-500">Handoff (observation):</span> {r.handoff}</div>}
                {(r.acks ?? []).length > 0 && (
                  <div className="mt-1 text-xs text-slate-400">
                    Acknowledged by {(r.acks ?? []).map((a) => `${a.by} (${dateTime(a.at)})`).join(', ')}
                  </div>
                )}
              </li>
            ))}
          </ul>
        )}
      </Panel>

      {isPeriod && issued.length > 0 && <IncomingAck periodId={periodId} items={items} onSaved={refresh} />}
    </div>
  )
}

function Item({ it }: { it: ReportItem }) {
  return (
    <li className="rounded-md border border-slate-800 bg-slate-900/40 p-2 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-slate-100">{it.title}</span>
        {it.restricted && <Tag tone="warn">RESTRICTED</Tag>}
        <span className="text-xs text-slate-500">{dateTime(it.at)}</span>
        {it.incident && <Link to={`/review/${it.incident}`} className="ml-auto text-xs text-sky-300 hover:underline">open incident {it.incident}</Link>}
      </div>
      <div className="mt-0.5 text-xs text-slate-400">
        {it.significance && <span>significance: {it.significance} · </span>}
        status: {it.status}
        {it.owner && <span> · owner: {it.owner}</span>}
        {it.next && <span> · next: {it.next}</span>}
      </div>
    </li>
  )
}

function PeriodPicker({ mode, periods, setMode }: { mode: Mode; periods: PeriodsResponse | null; setMode: (m: Mode) => void }) {
  const [from, setFrom] = useState(mode.kind === 'range' ? mode.from.slice(0, 16) : '')
  const [to, setTo] = useState(mode.kind === 'range' ? mode.to.slice(0, 16) : '')
  const issued = periods?.issued ?? {}
  return (
    <Panel title="Period">
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <label className="label" htmlFor="period-select">Operational period</label>
          <select
            id="period-select"
            className="input"
            value={mode.kind === 'period' ? mode.id : ''}
            onChange={(e) => e.target.value && setMode({ kind: 'period', id: e.target.value })}
          >
            <option value="">Choose a period…</option>
            {(periods?.periods ?? []).map((p, i) => (
              <option key={p.id} value={p.id}>
                {p.name} · {new Date(p.from).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false })}{i === 0 ? ' (current)' : ''}{issued[p.id] ? ` · issued v${issued[p.id]}` : ''}
              </option>
            ))}
          </select>
        </div>
        <span className="pb-2 text-xs text-slate-500">or a custom range</span>
        <div>
          <label className="label" htmlFor="range-from">From</label>
          <input id="range-from" type="datetime-local" className="input" value={from} onChange={(e) => setFrom(e.target.value)} />
        </div>
        <div>
          <label className="label" htmlFor="range-to">To</label>
          <input id="range-to" type="datetime-local" className="input" value={to} onChange={(e) => setTo(e.target.value)} />
        </div>
        <button
          type="button"
          className="btn"
          disabled={!from || !to}
          onClick={() => setMode({ kind: 'range', from: new Date(from).toISOString(), to: new Date(to).toISOString() })}
        >
          Use range
        </button>
      </div>
    </Panel>
  )
}

function Finalize({ periodId, issued, onSaved }: { periodId: string; issued: ShiftReport[]; onSaved: () => void }) {
  const [summary, setSummary] = useState('')
  const [handoff, setHandoff] = useState('')
  const [note, setNote] = useState('')
  const addendum = issued.length > 0
  const go = () =>
    run(() => api.post('/shift/report/finalize', { period: periodId, summary, handoff, note }), () => { setSummary(''); setHandoff(''); setNote(''); onSaved() })
  return (
    <Panel title={addendum ? 'Add an addendum' : 'Finalize this period'}>
      <div className="space-y-2">
        <NoteField id="sh-summary" label="Supervisor summary (observations, kept apart from the facts)" value={summary} onChange={setSummary} />
        <NoteField id="sh-handoff" label="Handoff notes (observations)" value={handoff} onChange={setHandoff} />
        {addendum && <NoteField id="sh-note" label="Why this addendum (required)" value={note} onChange={setNote} rows={1} />}
        <Busy go onClick={go} disabled={addendum && !note.trim()}>{addendum ? 'Add addendum' : 'Finalize'}</Busy>
      </div>
    </Panel>
  )
}

function IncomingAck({ periodId, items, onSaved }: { periodId: string; items: ReportItem[]; onSaved: () => void }) {
  // The open follow-up items of this report, keyed by follow-up ID.
  const fus = items.filter((it) => it.section === 'followups' && it.next)
  const [accept, setAccept] = useState<Record<string, boolean>>({})
  const [reassign, setReassign] = useState<Record<string, string>>({})
  const [note, setNote] = useState('')

  const fuId = (key: string) => key.replace(/^followup:/, '')

  const go = () => {
    const acceptIds = fus.map((it) => fuId(it.key)).filter((id) => accept[id])
    const reassignMap: Record<string, string> = {}
    for (const it of fus) {
      const id = fuId(it.key)
      if (reassign[id]?.trim()) reassignMap[id] = reassign[id].trim()
    }
    return run(() => api.post('/shift/report/ack', { period: periodId, accept: acceptIds, reassign: reassignMap, note }), () => { setNote(''); onSaved() })
  }

  return (
    <Panel icon={<UserCheck size={16} className="text-sky-300" aria-hidden />} title="Incoming supervisor acknowledgement">
      {fus.length === 0 ? (
        <p className="text-sm text-slate-400">No open follow-up to take. You can still acknowledge the report.</p>
      ) : (
        <ul className="space-y-2">
          {fus.map((it) => {
            const id = fuId(it.key)
            return (
              <li key={id} className={cx('rounded-md border border-slate-800 p-2 text-sm')}>
                <label className="flex items-center gap-2">
                  <input type="checkbox" checked={!!accept[id]} onChange={(e) => setAccept({ ...accept, [id]: e.target.checked })} />
                  <span>Accept {id}: {it.title}</span>
                </label>
                <div className="mt-1">
                  <label className="label" htmlFor={`re-${id}`}>Or reassign to username</label>
                  <input id={`re-${id}`} className="input" value={reassign[id] ?? ''} onChange={(e) => setReassign({ ...reassign, [id]: e.target.value })} placeholder="username" />
                </div>
              </li>
            )
          })}
        </ul>
      )}
      <div className="mt-2">
        <NoteField id="ack-note" label="Note" value={note} onChange={setNote} rows={1} />
      </div>
      <div className="mt-2"><Busy go onClick={go}>Acknowledge</Busy></div>
    </Panel>
  )
}
