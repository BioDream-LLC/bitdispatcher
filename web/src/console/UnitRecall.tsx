// Unit recall: everything one unit did over a stretch of time, as one plain timeline a front-line
// dispatcher can read — status changes, status checks, crew changes, move-ups, messages, emergencies and
// where it was. It reads the record; it changes nothing. internal/cad/history.go builds the history.
import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { AlertTriangle, ArrowRightLeft, Clock, Download, MapPin, MessageSquare, Radio, ShieldCheck, Users } from 'lucide-react'
import { api, errText } from '../api'
import { store } from '../store'
import { Field, Modal } from '../ui'
import { cx, hhmm, mins, statusMeta } from '../util'
import type { View } from '../types'

// The unit history JSON, richer than the brief in types.ts: each status span can carry its mark-out and
// incident, there are marks, crew spans, locations and per-incident information. (Defined here so types.ts
// is left untouched.)
export type HistorySpan = {
  from: string
  to: string
  status: string
  incident?: string
  markOut?: string
  reason?: string
  by?: string
  seconds: number
  open?: boolean
}
export type HistoryMark = {
  at: string
  kind: 'check' | 'crew' | 'cover' | 'emergency' | 'message'
  text: string
  by?: string
}
export type HistoryCrew = {
  member: string
  name: string
  role?: string
  from: string
  to: string
  open?: boolean
}
export type HistoryLocation = {
  at: string
  lat: number
  lon: number
  source?: string
}
export type IncInfo = { type?: string; address?: string }
export type UnitHistory = {
  unit: string
  from: string
  to: string
  spans: HistorySpan[]
  marks: HistoryMark[]
  totals: Record<string, number>
  incidents: string[]
  committed: number
  known: number
  crew: HistoryCrew[]
  locations: HistoryLocation[]
  incInfo: Record<string, IncInfo>
}

// A line on the merged timeline. Everything sorts by `at`; each kind renders its own way.
type Row =
  | { at: string; kind: 'span'; span: HistorySpan }
  | { at: string; kind: 'mark'; mark: HistoryMark }
  | { at: string; kind: 'crew'; crew: HistoryCrew; leaving: boolean }
  | { at: string; kind: 'location'; loc: HistoryLocation }

const markIcon: Record<HistoryMark['kind'], typeof Radio> = {
  check: ShieldCheck,
  crew: Users,
  cover: ArrowRightLeft,
  emergency: AlertTriangle,
  message: MessageSquare,
}
const markLabel: Record<HistoryMark['kind'], string> = {
  check: 'Status check',
  crew: 'Crew change',
  cover: 'Move-up',
  emergency: 'Emergency',
  message: 'Message',
}

// A datetime-local value (local time, no timezone) for an instant.
function localValue(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
}
// An ISO instant from a datetime-local value.
function isoFromLocal(v: string): string | undefined {
  if (!v) return undefined
  const t = new Date(v)
  return isNaN(t.getTime()) ? undefined : t.toISOString()
}

/** Unit recall: pick a unit and a span, and read one timeline of everything it did. */
export function UnitRecall({ view, unit, onClose }: { view: View; unit?: string; onClose: () => void }) {
  const [unitId, setUnitId] = useState(unit ?? view.units[0]?.id ?? '')
  const now = useMemo(() => new Date(), [])
  const [from, setFrom] = useState(() => localValue(new Date(now.getTime() - 12 * 3600000)))
  const [to, setTo] = useState(() => localValue(now))
  const [responder, setResponder] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [incidentFilter, setIncidentFilter] = useState('')
  const [history, setHistory] = useState<UnitHistory | null>(null)
  const [busy, setBusy] = useState(false)

  const fromIso = isoFromLocal(from)
  const toIso = isoFromLocal(to)

  const load = async () => {
    if (!unitId) {
      toast.error('Choose a unit.')
      return
    }
    if (!fromIso || !toIso) {
      toast.error('Choose a start and end time.')
      return
    }
    setBusy(true)
    try {
      const r = await api.get<UnitHistory>(`/units/${encodeURIComponent(unitId)}/history?from=${encodeURIComponent(fromIso)}&to=${encodeURIComponent(toIso)}`)
      setHistory(r)
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }

  // The statuses seen in this history, for the status filter.
  const statusesSeen = useMemo(() => {
    const set = new Set<string>()
    history?.spans.forEach((s) => set.add(s.status))
    return [...set]
  }, [history])

  // Who staffed the unit over this span, by member id, with the name to show.
  const whoStaffed = useMemo(() => {
    const m = new Map<string, string>()
    history?.crew.forEach((c) => m.set(c.member, c.name))
    return m
  }, [history])

  // When a named responder was aboard, as [from,to) windows, for filtering other rows to their time on board.
  const responderWindows = useMemo(() => {
    if (!responder.trim() || !history) return null
    const q = responder.trim().toLowerCase()
    return history.crew
      .filter((c) => c.member.toLowerCase().includes(q) || c.name.toLowerCase().includes(q))
      .map((c) => ({ from: new Date(c.from).getTime(), to: c.open ? Infinity : new Date(c.to).getTime() }))
  }, [responder, history])

  const inResponderWindow = (at: string) => {
    if (!responderWindows) return true
    const t = new Date(at).getTime()
    return responderWindows.some((w) => t >= w.from && t <= w.to)
  }

  // Build one chronological timeline from spans, marks, crew spans and locations, then apply the
  // client-side filters.
  const rows = useMemo<Row[]>(() => {
    if (!history) return []
    const out: Row[] = []
    for (const span of history.spans) {
      if (statusFilter && span.status !== statusFilter) continue
      if (incidentFilter && !(span.incident ?? '').toLowerCase().includes(incidentFilter.trim().toLowerCase())) continue
      if (!inResponderWindow(span.from)) continue
      out.push({ at: span.from, kind: 'span', span })
    }
    for (const mark of history.marks) {
      if (statusFilter) continue
      if (incidentFilter) continue
      if (!inResponderWindow(mark.at)) continue
      out.push({ at: mark.at, kind: 'mark', mark })
    }
    for (const crew of history.crew) {
      if (statusFilter || incidentFilter) continue
      if (responderWindows) {
        const q = responder.trim().toLowerCase()
        if (!(crew.member.toLowerCase().includes(q) || crew.name.toLowerCase().includes(q))) continue
      }
      out.push({ at: crew.from, kind: 'crew', crew, leaving: false })
      if (!crew.open) out.push({ at: crew.to, kind: 'crew', crew, leaving: true })
    }
    for (const loc of history.locations) {
      if (statusFilter || incidentFilter) continue
      if (!inResponderWindow(loc.at)) continue
      out.push({ at: loc.at, kind: 'location', loc })
    }
    out.sort((a, b) => new Date(a.at).getTime() - new Date(b.at).getTime())
    return out
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [history, statusFilter, incidentFilter, responder, responderWindows])

  const csvHref = fromIso && toIso
    ? `./api/units/${encodeURIComponent(unitId)}/history?from=${encodeURIComponent(fromIso)}&to=${encodeURIComponent(toIso)}&format=csv`
    : undefined

  return (
    <Modal open onClose={onClose} xl title="Unit recall" icon={<Clock size={16} className="text-sky-300" aria-hidden />}
      footer={<button className="btn" onClick={onClose}>Close</button>}>
      <div className="grid gap-4 lg:grid-cols-[20rem_1fr]">
        {/* What to recall. */}
        <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); load() }}>
          <Field label="Unit" htmlFor="recallunit">
            <select id="recallunit" className="input" value={unitId} onChange={(e) => setUnitId(e.target.value)}>
              {view.units.map((u) => <option key={u.id} value={u.id}>{u.id}{u.name ? ` · ${u.name}` : ''}</option>)}
            </select>
          </Field>
          <div className="grid grid-cols-1 gap-2">
            <Field label="From" htmlFor="recallfrom"><input id="recallfrom" type="datetime-local" className="input" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
            <Field label="To" htmlFor="recallto"><input id="recallto" type="datetime-local" className="input" value={to} onChange={(e) => setTo(e.target.value)} /></Field>
          </div>
          <button className="btn btn-go w-full" type="submit" disabled={busy}>
            {busy ? 'Loading…' : 'Recall'}
          </button>

          <fieldset className="space-y-2 rounded border border-slate-800 p-2">
            <legend className="px-1 text-[11px] uppercase tracking-wider text-slate-500">Narrow it down</legend>
            <Field label="Responder" htmlFor="recallresp" hint="Member id or name who was aboard">
              <input id="recallresp" className="input" value={responder} onChange={(e) => setResponder(e.target.value)} placeholder="e.g. 4417 or Garcia" />
            </Field>
            <Field label="Status" htmlFor="recallstatus">
              <select id="recallstatus" className="input" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
                <option value="">Any status</option>
                {statusesSeen.map((s) => <option key={s} value={s}>{statusMeta(s).label}</option>)}
              </select>
            </Field>
            <Field label="Incident" htmlFor="recallinc">
              <input id="recallinc" className="input" value={incidentFilter} onChange={(e) => setIncidentFilter(e.target.value)} placeholder="Incident number" />
            </Field>
          </fieldset>

          {csvHref && (
            <a className="btn btn-ghost w-full justify-center" href={csvHref} target="_blank" rel="noreferrer" aria-label="Download this history as CSV">
              <Download size={14} aria-hidden /> Download CSV
            </a>
          )}
        </form>

        {/* The timeline and the summary. */}
        <div>
          {!history ? (
            <p className="text-sm text-slate-500">Pick a unit and a span, then Recall.</p>
          ) : (
            <>
              <Summary history={history} whoStaffed={whoStaffed} />
              <h3 className="mb-2 mt-3 text-[11px] font-bold uppercase tracking-wider text-slate-300">Timeline</h3>
              {rows.length === 0 ? (
                <p className="text-sm text-slate-500">Nothing in this span with these filters.</p>
              ) : (
                <ol className="space-y-1" aria-label={`Timeline for ${history.unit}`}>
                  {rows.map((r, i) => <Line key={i} row={r} history={history} onIncident={(n) => { store.select(n); onClose() }} />)}
                </ol>
              )}
            </>
          )}
        </div>
      </div>
    </Modal>
  )
}

function Summary({ history, whoStaffed }: { history: UnitHistory; whoStaffed: Map<string, string> }) {
  const totals = Object.entries(history.totals).filter(([, sec]) => sec > 0).sort((a, b) => b[1] - a[1])
  return (
    <section className="rounded border border-slate-800 bg-slate-900/40 p-2 text-sm" aria-label="Summary">
      <div className="mb-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-slate-300">
        <span><span className="text-slate-500">Incidents:</span> {history.incidents.length}</span>
        <span><span className="text-slate-500">Staffed by:</span> {whoStaffed.size || '—'}</span>
      </div>
      <div className="flex flex-wrap gap-1.5">
        {totals.length === 0 ? (
          <span className="text-xs text-slate-500">No time recorded.</span>
        ) : (
          totals.map(([st, sec]) => {
            const m = statusMeta(st)
            return <span key={st} className={cx('chip', m.bg, m.fg)} title={m.label}>{m.label}: {mins(sec)}</span>
          })
        )}
      </div>
      {whoStaffed.size > 0 && (
        <p className="mt-1 text-xs text-slate-400">
          <span className="text-slate-500">People:</span> {[...whoStaffed.values()].join(', ')}
        </p>
      )}
    </section>
  )
}

function Line({ row, history, onIncident }: { row: Row; history: UnitHistory; onIncident: (n: string) => void }) {
  const time = <span className="shrink-0 font-mono text-[11px] text-slate-500">{hhmm(row.at)}</span>

  if (row.kind === 'span') {
    const s = row.span
    const m = statusMeta(s.status)
    const inc = s.incident ? history.incInfo[s.incident] : undefined
    return (
      <li className="flex items-start gap-2">
        {time}
        <span className={cx('mt-0.5 chip font-mono', m.bg, m.fg)} title={m.label}>{m.code}</span>
        <div className="min-w-0 text-sm">
          <span className="text-slate-200">{m.label}</span>
          {s.markOut && <span className="ml-1 text-slate-400">({s.markOut}{s.reason ? ` — ${s.reason}` : ''})</span>}
          {s.incident && (
            <>
              {' '}
              <button type="button" className="font-mono text-sky-300 underline" onClick={() => onIncident(s.incident!)}>{s.incident}</button>
              {inc && <span className="text-slate-400"> {inc.type ?? ''}{inc.address ? ` · ${inc.address}` : ''}</span>}
            </>
          )}
          {s.seconds > 0 && <span className="ml-1 text-[11px] text-slate-500">· {mins(s.seconds)}</span>}
          {s.open && <span className="ml-1 text-[11px] text-emerald-300">· now</span>}
        </div>
      </li>
    )
  }

  if (row.kind === 'mark') {
    const Icon = markIcon[row.mark.kind] ?? Radio
    const emergency = row.mark.kind === 'emergency'
    return (
      <li className="flex items-start gap-2">
        {time}
        <Icon size={14} className={cx('mt-0.5 shrink-0', emergency ? 'text-rose-300' : 'text-slate-400')} aria-hidden />
        <div className="min-w-0 text-sm">
          <span className={cx(emergency ? 'font-semibold text-rose-300' : 'text-slate-300')}>{markLabel[row.mark.kind]}</span>
          {row.mark.text && <span className="text-slate-400"> — {row.mark.text}</span>}
          {row.mark.by && <span className="ml-1 text-[11px] text-slate-500">by {row.mark.by}</span>}
        </div>
      </li>
    )
  }

  if (row.kind === 'crew') {
    return (
      <li className="flex items-start gap-2">
        {time}
        <Users size={14} className="mt-0.5 shrink-0 text-slate-400" aria-hidden />
        <div className="min-w-0 text-sm text-slate-300">
          {row.leaving ? 'Left: ' : 'Joined: '}
          <span className="text-slate-200">{row.crew.name}</span>
          {row.crew.role && <span className="text-slate-400"> ({row.crew.role})</span>}
        </div>
      </li>
    )
  }

  // location
  return (
    <li className="flex items-start gap-2">
      {time}
      <MapPin size={14} className="mt-0.5 shrink-0 text-slate-400" aria-hidden />
      <div className="min-w-0 text-sm text-slate-400">
        <span className="font-mono">{row.loc.lat.toFixed(5)}, {row.loc.lon.toFixed(5)}</span>
        {row.loc.source && <span className="ml-1 text-[11px] text-slate-500">{row.loc.source}</span>}
      </div>
    </li>
  )
}

/** Listens for 'bd:recall' (detail = unit id) and opens Unit recall for that unit, so any screen can open
 * it with: window.dispatchEvent(new CustomEvent('bd:recall', { detail: unitId })). */
export function UnitRecallHost({ view }: { view: View }) {
  const [unit, setUnit] = useState<string | null>(null)
  const [openFor, setOpenFor] = useState<string | undefined>(undefined)
  useEffect(() => {
    const on = (e: Event) => {
      setOpenFor((e as CustomEvent<string>).detail || undefined)
      setUnit((e as CustomEvent<string>).detail || '')
    }
    window.addEventListener('bd:recall', on)
    return () => window.removeEventListener('bd:recall', on)
  }, [])
  if (unit === null) return null
  return <UnitRecall view={view} unit={openFor} onClose={() => setUnit(null)} />
}
