import { useEffect, useMemo, useState } from 'react'
import BitVoiceRecords from './BitVoiceRecords'
import * as Dialog from '@radix-ui/react-dialog'
import { toast } from 'sonner'
import { BadgeCheck, Download, FileText, Printer, X } from 'lucide-react'
import { api, errText } from '../api'
import { can, store, useStore } from '../store'
import { cx } from '../util'
import type { View } from '../types'
import { Page } from './Admin'
import Chart, { base } from './Chart'

type Count = { key: string; label: string; n: number; events: number[] }
type Durations = { n: number; median: number; p90: number; max: number; events: number[] }
type IncidentLine = { number: string; type: string; typeName: string; priority: number; address?: string; created: string; closed?: string; disposition?: string; units: string[]; seq: number }
type Report = {
  from: string; to: string; key: string; agency: string; generated: string; toSeq: number; hash: string
  totals: Count[]; byType: Count[]; byPriority: Count[]; byHour: Count[]; byDisposition: Count[]
  response: { key: string; label: string; incidents: number; toDispatch: Durations; toEnroute: Durations; toOnScene: Durations; onSceneToClear: Durations }[]
  significant: IncidentLine[]; carriedIn: IncidentLine[]; carriedOut: IncidentLine[]
  timeouts: { unit: string; status: string; incident?: string; started: string; due: string; ended?: string; overSeconds: number; seq: number }[]
  emergencies: { unit: string; alias?: string; raised: string; ackedBy?: string; ackSeconds?: number; where?: string; seq: number }[]
  positions: { actor: string; consoles: string[]; created: number; dispatched: number; statuses: number; notes: number; closed: number; moveUps: number; touched: number; actions: number; events: number[] }[]
  units: { unit: string; type: string; incidents: number; committedSeconds: number; byPhase: Record<string, number>; utilisation: number; oosSeconds: number; markedOutSeconds?: number }[]
  system: { at: string; type: string; by: string; text: string; seq: number }[]
  certifications: { by: string; at: string; hash: string; comment?: string; current: boolean; seq: number }[]
}
type Ev = { seq: number; at: string; type: string; actor: string; console?: string; incident?: string; unit?: string; data?: unknown }

const dur = (s?: number) => {
  if (!s && s !== 0) return '—'
  if (s < 60) return `${Math.round(s)}s`
  if (s < 3600) return `${Math.floor(s / 60)}:${String(Math.round(s % 60)).padStart(2, '0')}`
  return `${(s / 3600).toFixed(1)} h`
}
const t = (s: string) => new Date(s).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
const dt = (s: string) => new Date(s).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
const PHASES = ['dispatched', 'enroute', 'onscene', 'transporting', 'atdestination'] as const
const PHASE_COLOR: Record<string, string> = { dispatched: '#f59e0b', enroute: '#38bdf8', onscene: '#f43f5e', transporting: '#a78bfa', atdestination: '#c084fc' }

// Shift periods from the agency's shift change times (Settings), in this browser's time zone.
function periods(shifts: string[]) {
  const now = new Date()
  const at = (d: Date, hm: string) => {
    const [h, m] = hm.split(':').map(Number)
    const x = new Date(d)
    x.setHours(h || 0, m || 0, 0, 0)
    return x
  }
  const starts: Date[] = []
  for (let back = 2; back >= -1; back--) {
    const d = new Date(now)
    d.setDate(d.getDate() - back)
    for (const s of shifts.length ? shifts : ['00:00']) starts.push(at(d, s))
  }
  starts.sort((a, b) => +a - +b)
  const i = starts.findLastIndex((s) => s <= now)
  const day = at(now, '00:00')
  const yday = new Date(day)
  yday.setDate(day.getDate() - 1)
  const week = new Date(day)
  week.setDate(day.getDate() - 7)
  return [
    { id: 'shift', label: 'This shift', from: starts[i], to: starts[i + 1] },
    { id: 'last', label: 'Last shift', from: starts[i - 1], to: starts[i] },
    { id: 'today', label: 'Today', from: day, to: new Date(+day + 86400000) },
    { id: 'yday', label: 'Yesterday', from: yday, to: day },
    { id: 'week', label: 'Last 7 days', from: week, to: day },
  ]
}

const local = (d: Date) => new Date(+d - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16)

/** The Daily Operations Report and shift debrief, built from the ledger with every total traceable. */
export default function Reports({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const ps = useMemo(() => periods(view.settings.shifts ?? ['07:00', '19:00']), [view.settings.shifts])
  const [pid, setPid] = useState('shift')
  const [from, setFrom] = useState(local(ps[0].from))
  const [to, setTo] = useState(local(ps[0].to))
  const [rep, setRep] = useState<Report | null>(null)
  const [busy, setBusy] = useState(false)
  const [drill, setDrill] = useState<{ title: string; seqs: number[] } | null>(null)
  const [comment, setComment] = useState('')
  const range = { from: new Date(from).toISOString(), to: new Date(to).toISOString() }
  const load = () => {
    setBusy(true)
    api.get<Report>(`/reports/operations?from=${encodeURIComponent(range.from)}&to=${encodeURIComponent(range.to)}`)
      .then(setRep).catch((e) => toast.error(errText(e))).finally(() => setBusy(false))
  }
  useEffect(load, [from, to])
  const pick = (id: string) => {
    const p = ps.find((x) => x.id === id)!
    setPid(id)
    setFrom(local(p.from))
    setTo(local(p.to))
  }
  const ended = new Date(to) <= new Date()
  const certify = async () => {
    if (!rep) return
    try {
      setRep(await api.post<Report>('/reports/certify', { ...range, hash: rep.hash, comment }))
      setComment('')
      toast.success('Report certified')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const total = (k: string) => rep?.totals.find((x) => x.key === k)
  const hours = useMemo(() => {
    const h = Array.from({ length: 24 }, (_, i) => rep?.byHour.find((x) => x.key === String(i).padStart(2, '0')))
    return h
  }, [rep])
  const pri = rep?.response.filter((r) => r.key.startsWith('P')) ?? []
  const current = rep?.certifications.filter((c) => c.current) ?? []
  return (
    <Page
      title="Operations report"
      intro="The Daily Operations Report and shift debrief, built from the ledger: nobody retypes what CAD already holds. Click any number to see the events it is made of. A supervisor certifies the report once its period has ended; the certification is a ledger event and says whether the report has changed since."
      actions={<>
        <button className="btn" onClick={() => window.print()}><Printer size={14} aria-hidden /> Print</button>
        {rep && <button className="btn" onClick={() => {
          const a = document.createElement('a')
          a.href = URL.createObjectURL(new Blob([JSON.stringify(rep, null, 2)], { type: 'application/json' }))
          a.download = `operations-${from}-${to}.json`
          a.click()
        }}><Download size={14} aria-hidden /> JSON</button>}
      </>}
    >
      <div className="mb-3 flex flex-wrap items-end gap-2 print:hidden">
        <div className="flex gap-1" role="group" aria-label="Period">
          {ps.map((p) => <button key={p.id} className={cx('btn text-xs', pid === p.id && 'ring-1 ring-sky-400')} onClick={() => pick(p.id)} disabled={!p.from || !p.to}>{p.label}</button>)}
        </div>
        <label className="text-[11px] text-slate-500">From <input type="datetime-local" className="input h-8 w-auto py-0" value={from} onChange={(e) => { setPid(''); setFrom(e.target.value) }} /></label>
        <label className="text-[11px] text-slate-500">To <input type="datetime-local" className="input h-8 w-auto py-0" value={to} onChange={(e) => { setPid(''); setTo(e.target.value) }} /></label>
        {busy && <span className="text-xs text-slate-500">Reading the ledger…</span>}
      </div>
      {!rep ? <p className="text-sm text-slate-500">Loading…</p> : (
        <div className="space-y-4">
          <div className="panel flex flex-wrap items-center gap-3 rounded-lg p-3 text-xs">
            <FileText size={16} className="text-sky-300" aria-hidden />
            <div>
              <div className="font-semibold text-slate-100">{rep.agency || 'Agency'} · {dt(rep.from)} to {dt(rep.to)}</div>
              <div className="text-slate-500">Ledger to event #{rep.toSeq} · report hash <span className="font-mono">{rep.hash.slice(0, 12)}</span> · generated {t(rep.generated)}</div>
            </div>
            <div className="ml-auto flex flex-wrap items-center gap-2">
              {current.length > 0 ? current.map((c) => (
                <span key={c.seq} className="chip bg-emerald-500/15 text-emerald-300" title={c.comment}><BadgeCheck size={12} aria-hidden /> Certified by {c.by} {dt(c.at)}</span>
              )) : rep.certifications.length > 0 ? <span className="chip bg-amber-500/15 text-amber-300">Changed since certified by {rep.certifications.at(-1)!.by}: certify again</span> : <span className="chip bg-slate-800 text-slate-400">Not certified</span>}
              {can.supervise(me) && ended && (
                <span className="flex items-center gap-1 print:hidden">
                  <input className="input h-7 w-56 py-0 text-xs" placeholder="Supervisor context (optional)" value={comment} onChange={(e) => setComment(e.target.value)} />
                  <button className="btn btn-go h-7 py-0 text-xs" onClick={certify}><BadgeCheck size={13} aria-hidden /> Certify</button>
                </span>
              )}
              {!ended && <span className="text-[10px] text-slate-500">Certify after the period ends</span>}
            </div>
          </div>
          {rep.certifications.some((c) => c.comment) && (
            <div className="panel rounded-lg p-3 text-xs text-slate-300">
              {rep.certifications.filter((c) => c.comment).map((c) => <p key={c.seq}><b className="text-slate-100">{c.by}</b> ({dt(c.at)}{c.current ? '' : ', earlier version'}): {c.comment}</p>)}
            </div>
          )}

          <div className="grid grid-cols-2 gap-2 md:grid-cols-6">
            {[
              ['calls', '911 calls'], ['created', 'Incidents'], ['dispatches', 'Dispatches'], ['closed', 'Closed'], ['emergencies', 'Emergency buttons'],
            ].map(([k, label]) => {
              const c = total(k)
              return (
                <button key={k} className="panel rounded-lg p-2 text-left hover:ring-1 hover:ring-sky-500" onClick={() => c && setDrill({ title: label, seqs: c.events })} disabled={!c}>
                  <div className={cx('font-mono text-2xl font-bold', k === 'emergencies' && c ? 'text-rose-300' : 'text-slate-50')}>{c?.n ?? 0}</div>
                  <div className="text-[10px] uppercase tracking-wider text-slate-500">{label}</div>
                </button>
              )
            })}
            <div className="panel rounded-lg p-2">
              <div className="font-mono text-2xl font-bold text-amber-300">{rep.carriedOut.length}</div>
              <div className="text-[10px] uppercase tracking-wider text-slate-500">Open, to next shift</div>
            </div>
          </div>

          <div className="grid gap-4 lg:grid-cols-2">
            <Section title="Incidents by hour" hint="click a bar for its incidents">
              <Chart height={170} onClick={(i) => hours[i] && setDrill({ title: `Incidents ${hours[i]!.label}`, seqs: hours[i]!.events })} option={{
                ...base,
                xAxis: { type: 'category', data: hours.map((_, i) => String(i).padStart(2, '0')), axisLine: { lineStyle: { color: '#334155' } } },
                yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: '#1e293b' } } },
                series: [{ type: 'bar', data: hours.map((h) => h?.n ?? 0), itemStyle: { color: '#38bdf8', borderRadius: [3, 3, 0, 0] } }],
              }} />
            </Section>
            <Section title="Response by priority" hint="created to first on scene, median and 90th percentile (minutes)">
              <Chart height={170} option={{
                ...base,
                legend: { top: 0, right: 0, textStyle: { color: '#94a3b8', fontSize: 10 } },
                xAxis: { type: 'category', data: pri.map((p) => p.key), axisLine: { lineStyle: { color: '#334155' } } },
                yAxis: { type: 'value', splitLine: { lineStyle: { color: '#1e293b' } } },
                series: [
                  { name: 'median', type: 'bar', data: pri.map((p) => +(p.toOnScene.median / 60).toFixed(1)), itemStyle: { color: '#34d399' } },
                  { name: '90th', type: 'bar', data: pri.map((p) => +(p.toOnScene.p90 / 60).toFixed(1)), itemStyle: { color: '#f59e0b' } },
                ],
              }} />
            </Section>
          </div>

          <Section title="Response times" hint="median / 90th percentile; click for the events that end each interval">
            <table className="w-full text-xs tnum">
              <thead><tr className="text-left text-[10px] uppercase tracking-wider text-slate-500"><th className="py-1">Group</th><th>Incidents</th><th>Call to dispatch</th><th>Dispatch to en route</th><th>Call to on scene</th><th>On scene to clear</th></tr></thead>
              <tbody className="divide-y divide-slate-800">
                {rep.response.map((r) => (
                  <tr key={r.key} className={r.key === 'all' ? 'font-semibold text-slate-100' : 'text-slate-300'}>
                    <td className="py-1">{r.label}</td><td className="font-mono">{r.incidents}</td>
                    {[r.toDispatch, r.toEnroute, r.toOnScene, r.onSceneToClear].map((d, i) => (
                      <td key={i}>{d.n ? <button className="font-mono hover:text-sky-300" onClick={() => setDrill({ title: `${r.label}: ${['call to dispatch', 'dispatch to en route', 'call to on scene', 'on scene to clear'][i]}`, seqs: d.events })}>{dur(d.median)} / {dur(d.p90)} <span className="text-slate-500">({d.n})</span></button> : <span className="text-slate-600">—</span>}</td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </Section>

          <div className="grid gap-4 lg:grid-cols-2">
            <Section title={`Priority 1 incidents (${rep.significant.length})`}><Incidents list={rep.significant} /></Section>
            <Section title={`Open at the end: turnover to the next shift (${rep.carriedOut.length})`}><Incidents list={rep.carriedOut} open /></Section>
            <Section title={`Status timers run out (${rep.timeouts.length})`} hint="once per continuous episode">
              {rep.timeouts.length === 0 ? <Empty>No unit ran past a status timer.</Empty> : (
                <table className="w-full text-xs tnum"><tbody className="divide-y divide-slate-800">
                  {rep.timeouts.map((x) => (
                    <tr key={x.seq + x.unit}><td className="py-0.5 font-mono font-bold text-slate-100">{x.unit}</td><td className="text-slate-400">{x.status}</td>
                      <td><button className="font-mono text-sky-300" onClick={() => x.incident && store.select(x.incident)}>{x.incident}</button></td>
                      <td className="text-slate-400">due {t(x.due)}</td><td className="font-mono text-amber-300">+{dur(x.overSeconds)}{!x.ended && ' (still)'}</td></tr>
                  ))}
                </tbody></table>
              )}
            </Section>
            <Section title={`Emergency activations (${rep.emergencies.length})`}>
              {rep.emergencies.length === 0 ? <Empty>No emergency button was pressed.</Empty> : (
                <table className="w-full text-xs tnum"><tbody className="divide-y divide-slate-800">
                  {rep.emergencies.map((x) => (
                    <tr key={x.seq}><td className="py-0.5 font-mono font-bold text-rose-300">{x.unit}</td><td className="text-slate-400">{x.alias}</td><td>{t(x.raised)}</td>
                      <td className="text-slate-300">{x.ackedBy ? <>acknowledged by {x.ackedBy} in <b className="font-mono">{dur(x.ackSeconds)}</b> ({x.where})</> : <span className="text-rose-300">not acknowledged</span>}</td></tr>
                  ))}
                </tbody></table>
              )}
            </Section>
          </div>

          <Section title="Dispatcher and communications activity" hint="intentional actions only (OPERATIONS.md section 7); never a score">
            {rep.positions.length === 0 ? <Empty>No actions in this period.</Empty> : (
              <table className="w-full text-xs tnum">
                <thead><tr className="text-left text-[10px] uppercase tracking-wider text-slate-500"><th className="py-1">Person</th><th>Positions</th><th>Created</th><th>Dispatch</th><th>Statuses</th><th>Notes</th><th>Closed</th><th>Move-ups</th><th>Incidents touched</th><th>All actions</th></tr></thead>
                <tbody className="divide-y divide-slate-800">
                  {rep.positions.map((p) => (
                    <tr key={p.actor} className="text-slate-300">
                      <td className="py-1 font-semibold text-slate-100">{p.actor}</td><td className="font-mono text-slate-400">{p.consoles.join(' ')}</td>
                      {[p.created, p.dispatched, p.statuses, p.notes, p.closed, p.moveUps, p.touched].map((n, i) => <td key={i} className="font-mono">{n || <span className="text-slate-600">0</span>}</td>)}
                      <td><button className="font-mono text-sky-300" onClick={() => setDrill({ title: `${p.actor}: every action`, seqs: p.events })}>{p.actions}</button></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Section>

          <Section title="Unit demand" hint="committed time by phase in the period; utilisation is committed time over the period">
            {rep.units.length === 0 ? <Empty>No unit was committed.</Empty> : (
              <div className="space-y-1">
                {rep.units.map((u) => (
                  <div key={u.unit} className="flex items-center gap-2 text-xs">
                    <span className="w-14 font-mono font-bold text-slate-100">{u.unit}</span>
                    <span className="w-20 truncate text-slate-500">{u.type}</span>
                    <div className="flex h-3 flex-1 overflow-hidden rounded bg-slate-800" title={PHASES.map((p) => `${p} ${dur(u.byPhase[p])}`).join(' · ')}>
                      {PHASES.map((p) => <div key={p} style={{ width: `${((u.byPhase[p] ?? 0) / Math.max(1, (new Date(rep.to).getTime() - new Date(rep.from).getTime()) / 1000)) * 100}%`, background: PHASE_COLOR[p] }} />)}
                    </div>
                    <span className="w-14 text-right font-mono text-slate-200">{(u.utilisation * 100).toFixed(1)}%</span>
                    <span className="w-16 text-right font-mono text-slate-400">{dur(u.committedSeconds)}</span>
                    <span className="w-16 text-right text-slate-500">{u.incidents} inc.</span>
                    {u.oosSeconds > 0 && <span className="text-[10px] text-slate-500">OOS {dur(u.oosSeconds)}</span>}
                    {(u.markedOutSeconds ?? 0) > 0 && <span className="text-[10px] text-teal-400">marked out {dur(u.markedOutSeconds!)}</span>}
                  </div>
                ))}
                <div className="flex gap-3 pt-1 text-[10px] text-slate-500">{PHASES.map((p) => <span key={p} className="flex items-center gap-1"><i className="inline-block h-2 w-2 rounded-sm" style={{ background: PHASE_COLOR[p] }} />{p}</span>)}</div>
              </div>
            )}
          </Section>

          <div className="grid gap-4 lg:grid-cols-3">
            <Section title="By call type"><Counts list={rep.byType} onOpen={(c) => setDrill({ title: c.label, seqs: c.events })} /></Section>
            <Section title="By disposition"><Counts list={rep.byDisposition} onOpen={(c) => setDrill({ title: c.label, seqs: c.events })} /></Section>
            <Section title={`System and configuration (${rep.system.length})`} hint="interruptions and changes">
              {rep.system.length === 0 ? <Empty>No configuration or system events.</Empty> : (
                <ul className="space-y-0.5 text-xs">
                  {rep.system.map((x) => <li key={x.seq} className="flex gap-2"><span className="font-mono text-slate-500">{t(x.at)}</span><span className="text-slate-300">{x.text}</span><span className="ml-auto text-slate-500">{x.by}</span></li>)}
                </ul>
              )}
            </Section>
          </div>
        </div>
      )}
      <Drill drill={drill} onClose={() => setDrill(null)} />
      <BitVoiceRecords view={view} from={from} to={to} />
    </Page>
  )
}

function Section({ title, hint, children }: { title: string; hint?: string; children: React.ReactNode }) {
  return (
    <section className="panel rounded-lg p-3 break-inside-avoid">
      <h2 className="mb-2 flex items-baseline gap-2 text-[11px] font-bold uppercase tracking-wider text-slate-400">{title}{hint && <span className="text-[10px] font-normal normal-case tracking-normal text-slate-500">{hint}</span>}</h2>
      {children}
    </section>
  )
}

const Empty = ({ children }: { children: React.ReactNode }) => <p className="text-xs text-slate-500">{children}</p>

function Counts({ list, onOpen }: { list: Count[]; onOpen: (c: Count) => void }) {
  if (list.length === 0) return <Empty>None.</Empty>
  const max = Math.max(...list.map((c) => c.n))
  return (
    <ul className="space-y-0.5 text-xs">
      {list.map((c) => (
        <li key={c.key}>
          <button className="flex w-full items-center gap-2 rounded px-1 hover:bg-slate-800" onClick={() => onOpen(c)}>
            <span className="w-40 truncate text-left text-slate-300">{c.label || '—'}</span>
            <span className="h-2 flex-1 rounded bg-slate-800"><span className="block h-2 rounded bg-sky-500" style={{ width: `${(c.n / max) * 100}%` }} /></span>
            <span className="w-8 text-right font-mono text-slate-100">{c.n}</span>
          </button>
        </li>
      ))}
    </ul>
  )
}

function Incidents({ list, open }: { list: IncidentLine[]; open?: boolean }) {
  if (list.length === 0) return <Empty>{open ? 'Nothing left open.' : 'None.'}</Empty>
  return (
    <table className="w-full text-xs tnum"><tbody className="divide-y divide-slate-800">
      {list.map((i) => (
        <tr key={i.number} className="text-slate-300">
          <td className="py-0.5"><span className="font-mono font-bold text-slate-100">P{i.priority}</span></td>
          <td className="font-mono text-sky-300">{i.number}</td>
          <td>{i.typeName}</td>
          <td className="max-w-48 truncate text-slate-400">{i.address}</td>
          <td className="font-mono text-slate-400">{dt(i.created)}</td>
          <td className="text-slate-400">{open ? i.units.join(' ') || <span className="text-amber-300">no units</span> : i.disposition}</td>
        </tr>
      ))}
    </tbody></table>
  )
}

function Drill({ drill, onClose }: { drill: { title: string; seqs: number[] } | null; onClose: () => void }) {
  const [evs, setEvs] = useState<Ev[] | null>(null)
  useEffect(() => {
    setEvs(null)
    if (!drill || drill.seqs.length === 0) return
    api.get<{ items: Ev[] }>(`/ledger?limit=2000&seqs=${drill.seqs.slice(0, 2000).join(',')}`).then((r) => setEvs(r.items)).catch((e) => toast.error(errText(e)))
  }, [drill])
  return (
    <Dialog.Root open={!!drill} onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/60" />
        <Dialog.Content className="fixed left-1/2 top-12 z-50 max-h-[80vh] w-[min(56rem,94vw)] -translate-x-1/2 overflow-y-auto rounded-lg border border-slate-700 bg-slate-900 p-4 shadow-2xl">
          <Dialog.Title className="mb-1 flex items-center gap-2 font-semibold text-slate-100">{drill?.title} <span className="font-mono text-xs text-slate-500">{drill?.seqs.length} events</span>
            <Dialog.Close className="btn ml-auto h-7 w-7 justify-center p-0" aria-label="Close"><X size={14} /></Dialog.Close></Dialog.Title>
          <Dialog.Description className="mb-2 text-xs text-slate-500">The ledger events this number is made of, newest first.</Dialog.Description>
          {!evs ? <p className="text-xs text-slate-500">Loading…</p> : (
            <table className="w-full text-xs"><tbody className="divide-y divide-slate-800">
              {evs.map((e) => (
                <tr key={e.seq} className="align-top text-slate-300">
                  <td className="py-0.5 pr-2 font-mono text-slate-500">#{e.seq}</td>
                  <td className="pr-2 font-mono text-slate-400">{dt(e.at)}</td>
                  <td className="pr-2 text-slate-200">{e.type}</td>
                  <td className="pr-2">{e.actor}{e.console && <span className="text-slate-500"> @{e.console}</span>}</td>
                  <td className="pr-2 font-mono text-sky-300">{e.incident} {e.unit}</td>
                  <td className="max-w-72 truncate font-mono text-[10px] text-slate-500">{e.data ? JSON.stringify(e.data) : ''}</td>
                </tr>
              ))}
            </tbody></table>
          )}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
