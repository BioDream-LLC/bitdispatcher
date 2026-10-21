import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Activity, AlertTriangle, ArrowRightLeft, Building2, Clock3, Home, Plus, ShieldAlert } from 'lucide-react'
import { api, errText } from '../api'
import { AdvisoryCards, useAdvisory } from './Advisory'
import { can, store, useStore } from '../store'
import { StatusChip } from '../ui'
import { activity as doing, ago, clock, cx, ms, overdue, statusMeta, typeName, useNow } from '../util'
import type { Call, Coverage, View } from '../types'
import { Passdown, BOLOs, Facilities, Messages } from './Ops'
import { CallBox, UnitDetail } from './IncidentPanel'

const LABEL: Record<string, string> = {
  'incident.created': 'created', 'incident.dispatch': 'dispatched to', 'incident.note': 'noted on', 'incident.changed': 'changed',
  'incident.closed': 'closed', 'incident.reopened': 'reopened', 'unit.status': 'status', 'call.received': '911 call', 'call.updated': 'call update',
  'call.ended': 'call ended', 'call.attached': 'call attached to', 'call.action': 'caller action', 'radio.emergency': 'EMERGENCY',
  'radio.emergency.ack': 'emergency acknowledged', 'session.signin': 'signed in', 'session.signout': 'signed out', 'unit.saved': 'unit configured',
  'unit.cover': 'move-up', 'settings.changed': 'settings changed', 'integration.changed': 'integration changed', 'calltype.saved': 'call type saved', 'station.saved': 'station saved',
}

// Situation is what the right-hand panel shows when no incident is selected: what needs attention next.
export default function Situation({ view, onCreateFromCall }: { view: View; onCreateFromCall: (c: Call) => void }) {
  const now = useNow()
  const me = useStore((s) => s.me)
  const activity = useStore((s) => s.activity)
  const selCall = useStore((s) => s.selectedCall)
  const selUnit = useStore((s) => s.selectedUnit)
  const call = view.calls.find((c) => c.id === selCall)
  const over = view.units.map((u) => ({ u, by: overdue(view, u, now) })).filter((x) => x.by > 0).sort((a, b) => b.by - a.by)
  const pending = view.incidents.filter((i) => i.status === 'pending').sort((a, b) => ms(a.created) - ms(b.created))
  const cov = useCoverage(view.seq)
  const adv = useAdvisory(view.seq)
  const coverage = view.stations
    .map((s) => {
      const here = view.units.filter((u) => (u.covering || u.station) === s.id)
      return { s, all: here.length, av: here.filter((u) => u.status === 'available').length, units: here }
    })
    .filter((c) => c.all > 0)
  const failures = view.health.bitvoice.failures.filter((f) => now - ms(f.at) < 3600000)
  return (
    <aside className="panel flex min-h-0 flex-1 flex-col" aria-label="Situation">
      <div className="panel-head">
        <Activity size={12} aria-hidden /> Situation
        <span className="ml-auto normal-case tracking-normal text-slate-500">select an incident to work it</span>
      </div>
      <div className="min-h-0 flex-1 space-y-2 overflow-y-auto p-2">
        {call && (
          <div className="space-y-1">
            <CallBox call={call} view={view} now={now} />
            {can.dispatch(me) && !call.incident && call.state !== 'ended' && (
              <button className="btn btn-go w-full" onClick={() => onCreateFromCall(call)}>
                <Plus size={13} aria-hidden /> Create incident from this call
              </button>
            )}
          </div>
        )}
        {selUnit && <UnitDetail view={view} id={selUnit} />}
        <Messages view={view} />
        <Passdown view={view} />
        <BOLOs view={view} />
        <Facilities view={view} />
        {adv && <AdvisoryCards list={adv.suggestions} status={adv.status} />}
        {failures.length > 0 && (
          <section className="rounded-md border border-amber-800/60 bg-amber-950/20 p-2 text-xs">
            <div className="mb-1 flex items-center gap-1 font-semibold text-amber-300"><AlertTriangle size={12} aria-hidden /> BitVoice could not deliver</div>
            {failures.slice(0, 5).map((f, i) => (
              <div key={i} className="text-amber-200/90">{ago(f.at, now)} ago · {f.what}{f.call ? ` (call ${f.call})` : ''}: {f.error}</div>
            ))}
            <div className="mt-1 text-[10px] text-slate-400">bitdispatcher fills what it can from BitVoice’s event stream.</div>
          </section>
        )}
        <section>
          <h3 className="mb-1 flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400"><Clock3 size={12} aria-hidden /> Waiting for units <span className="font-mono text-slate-300">{pending.length}</span></h3>
          {pending.length === 0 ? (
            <p className="text-[11px] text-slate-500">Nothing waiting. Every open incident has a unit.</p>
          ) : (
            <ul className="space-y-0.5">
              {pending.map((i) => (
                <li key={i.number}>
                  <button className="flex w-full items-center gap-1.5 rounded px-1 py-0.5 text-left text-xs hover:bg-slate-800" onClick={() => store.select(i.number)}>
                    <span className="font-mono font-bold text-slate-200">P{i.priority}</span>
                    <span className="font-mono text-slate-400">{i.number}</span>
                    <span className="truncate text-slate-300">{typeName(view, i.type)}</span>
                    <span className="ml-auto font-mono font-semibold text-amber-300 tnum">{clock(i.created, now)}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>
        <section>
          <h3 className="mb-1 flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400"><ShieldAlert size={12} aria-hidden /> Status timers past due <span className="font-mono text-slate-300">{over.length}</span></h3>
          {over.length === 0 ? (
            <p className="text-[11px] text-slate-500">No unit is past its status timer.</p>
          ) : (
            <ul className="space-y-0.5">
              {over.map(({ u, by }) => (
                <li key={u.id} className="flex items-center gap-1.5 rounded bg-amber-500/10 px-1 py-0.5 text-xs">
                  <span className="font-mono font-bold text-slate-100">{u.id}</span>
                  <StatusChip status={u.status} />
                  {u.incident ? <button className="font-mono text-sky-300 underline" onClick={() => store.select(u.incident!)}>{u.incident}</button> : <span className="truncate text-teal-300">{doing(view, u)}</span>}
                  <span className="ml-auto font-mono text-amber-300 tnum">+{clock(now - by, now)}</span>
                  <span className="text-[10px] text-slate-500">{u.lastHeard ? `radio ${ago(u.lastHeard, now)}` : 'not heard'}</span>
                  {can.dispatch(me) && <button className="btn h-5 px-1 py-0 text-[10px] text-emerald-300" onClick={() => run(`SC ${u.id}`)} title={`Status check: ${u.id} is all right; its timer starts again`}>OK</button>}
                </li>
              ))}
            </ul>
          )}
        </section>
        {cov && (cov.gaps.length > 0 || cov.returns.length > 0) && <MoveUps cov={cov} view={view} now={now} canAct={can.dispatch(me)} />}
        <section>
          <h3 className="mb-1 flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400"><Building2 size={12} aria-hidden /> Coverage by station</h3>
          {coverage.length === 0 ? (
            <p className="text-[11px] text-slate-500">No stations yet: add them in Admin › Stations so units without AVL can be placed.</p>
          ) : (
            <ul className="grid grid-cols-2 gap-1">
              {coverage.map(({ s, all, av, units }) => (
                <li key={s.id} className={cx('rounded border px-1.5 py-1 text-[11px]', av === 0 ? 'border-rose-800 bg-rose-950/30' : 'border-slate-800')}>
                  <div className="flex items-center gap-1">
                    <span className="font-mono font-bold text-slate-200">{s.id}</span>
                    <span className="truncate text-slate-400">{s.name}</span>
                    <span className={cx('ml-auto font-mono', av === 0 ? 'text-rose-300' : 'text-emerald-300')}>{av}/{all}</span>
                  </div>
                  <div className="mt-0.5 flex flex-wrap gap-0.5">
                    {units.map((u) => (
                      <span key={u.id} className={cx('font-mono text-[10px]', statusMeta(u.status).fg)} title={u.covering ? `${u.id} covering from ${u.station}` : undefined}>{u.id}{u.covering && '↗'}</span>
                    ))}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </section>
        <section>
          <h3 className="mb-1 text-[11px] font-bold uppercase tracking-wider text-slate-400">Recent activity</h3>
          {activity.length === 0 ? (
            <p className="text-[11px] text-slate-500">Changes by every position appear here as they happen.</p>
          ) : (
            <ul className="space-y-px font-mono text-[10.5px]">
              {activity.slice(0, 30).map((a) => (
                <li key={a.seq + a.type}>
                  <button className="flex w-full gap-1.5 rounded px-1 text-left hover:bg-slate-800" onClick={() => a.incident && store.select(a.incident)}>
                    <span className="text-slate-500">{ago(a.at, now)}</span>
                    <span className="text-slate-400">{a.actor}</span>
                    <span className={cx(a.type.startsWith('radio.emergency') ? 'font-bold text-rose-300' : 'text-slate-200')}>{LABEL[a.type] ?? a.type}</span>
                    <span className="truncate text-sky-300">{a.incident ?? ''} {a.unit ?? ''}</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </aside>
  )
}

// useCoverage follows the move-up picture, asked again when anything changes.
export function useCoverage(seq: number): Coverage | null {
  const [cov, setCov] = useState<Coverage | null>(null)
  useEffect(() => {
    const t = setTimeout(() => api.get<Coverage>('/coverage').then(setCov).catch(() => {}), 250)
    return () => clearTimeout(t)
  }, [seq])
  return cov
}

const run = (line: string) => api.post<{ ok: boolean; message: string }>('/command', { line, incident: '' })
  .then((r) => (r.ok ? toast.success(r.message) : toast.error(r.message))).catch((e) => toast.error(errText(e)))

/** Stations left without a kind of unit, with the move-up that fills each in one click. */
export function MoveUps({ cov, view, now, canAct }: { cov: Coverage; view: View; now: number; canAct: boolean }) {
  const min = (s?: number) => (s ? `${Math.max(1, Math.round(s / 60))} min` : '')
  return (
    <section className="rounded-md border border-rose-800/60 bg-rose-950/20 p-2">
      <h3 className="mb-1 flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-rose-300"><ArrowRightLeft size={12} aria-hidden /> Move-up <span className="font-mono text-slate-300">{cov.gaps.length}</span>
        <span className="ml-auto text-[9px] font-normal normal-case tracking-normal text-slate-500" title={cov.estimate}>{cov.estimate.startsWith('Straight') ? 'straight-line times' : 'drive times by road'}</span></h3>
      <ul className="space-y-1">
        {cov.gaps.map((g) => (
          <li key={g.station + g.type} className="text-xs">
            <div className="flex items-center gap-1.5">
              <span className="font-mono font-bold text-slate-100">{g.station}</span>
              <span className="text-rose-200">no {g.typeName.toLowerCase()}</span>
              {g.since && <span className="text-[10px] text-slate-500">for {ago(g.since, now)}</span>}
              {g.move ? (
                canAct ? (
                  <button className="btn ml-auto h-6 px-1.5 py-0 text-[11px]" title={g.move.reason} onClick={() => run(`MU ${g.move!.unit} ${g.station}`)}>
                    Move <b className="font-mono">{g.move.unit}</b> from {g.move.from || '—'} <span className="text-slate-400">{min(g.move.seconds)}</span>
                    {g.move.opens && <span className="text-amber-300">· empties {g.move.from}</span>}
                  </button>
                ) : <span className="ml-auto text-slate-400">{g.move.unit} from {g.move.from}</span>
              ) : <span className="ml-auto text-[10px] text-amber-300">no {g.typeName.toLowerCase()} to spare: mutual aid</span>}
            </div>
            {canAct && g.others.length > 0 && (
              <div className="mt-0.5 flex flex-wrap justify-end gap-1">
                {g.others.map((m) => (
                  <button key={m.unit} className="rounded px-1 font-mono text-[10px] text-slate-400 hover:bg-slate-800 hover:text-slate-100" title={m.reason} onClick={() => run(`MU ${m.unit} ${g.station}`)}>
                    {m.unit} {min(m.seconds)}{m.opens && <span className="text-amber-400">*</span>}
                  </button>
                ))}
              </div>
            )}
          </li>
        ))}
        {cov.returns.map((r) => (
          <li key={r.unit} className="flex items-center gap-1.5 text-xs">
            <Home size={11} className="text-emerald-300" aria-hidden />
            <span className="font-mono font-bold text-slate-100">{r.unit}</span>
            <span className="truncate text-slate-400">{r.reason}</span>
            {canAct && <button className="btn ml-auto h-6 px-1.5 py-0 text-[11px]" onClick={() => run(`RET ${r.unit}`)}>Home to {r.home}</button>}
          </li>
        ))}
      </ul>
      {view.stations.length === 0 && <p className="text-[10px] text-slate-500">Add stations to see coverage.</p>}
    </section>
  )
}
