import { useState } from 'react'
import { BellRing, Check, MapPin, Radio } from 'lucide-react'
import { can, store, useStore } from '../store'
import { StatusChip, Priority } from '../ui'
import { activity, ago, avl, checkDue, clock, cx, markOutType, ms, statusMeta, timerMinutes, typeName, useNow } from '../util'
import type { SearchResult, Unit, View } from '../types'
import { api } from '../api'
import { UnitMenu, run } from './UnitBoard'
import { markOutIcon, openMarkOut } from './MarkOut'

const ORDER = ['onscene', 'enroute', 'dispatched', 'transporting', 'atdestination', 'markout', 'available', 'oos']
type Filter = 'all' | 'calls' | 'markout' | 'due'

/** The status-check ring: how much of the unit's time has gone, and how long is left (or how long it is past). */
export function CheckRing({ view, u, now, size = 30 }: { view: View; u: Unit; now: number; size?: number }) {
  const mins = timerMinutes(view, u)
  if (!mins) return <span className="text-[11px] text-slate-600">no timer</span>
  const due = checkDue(view, u)
  const total = mins * 60000
  const left = due - now
  const frac = Math.min(1, Math.max(0, 1 - left / total))
  const over = left < 0
  const soon = !over && left < Math.min(5 * 60000, total * 0.25)
  const r = size / 2 - 3, c = 2 * Math.PI * r
  const col = over ? '#f43f5e' : soon ? '#fbbf24' : '#2dd4bf'
  return (
    <span className={cx('inline-flex items-center gap-1.5', over && 'overdue rounded px-0.5')} title={over ? `Status check due ${clock(due, now)} ago` : `Status check in ${clock(now, due)} (${mins} min timer)`}>
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} aria-hidden>
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="#1e293b" strokeWidth="3.5" />
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke={col} strokeWidth="3.5" strokeLinecap="round" strokeDasharray={`${c * frac} ${c}`} transform={`rotate(-90 ${size / 2} ${size / 2})`} />
        {over && <circle cx={size / 2} cy={size / 2} r={r - 4} fill="#f43f5e" className="live-dot" />}
      </svg>
      <span className={cx('font-mono text-[12px] font-bold tnum', over ? 'text-rose-200' : soon ? 'text-amber-300' : 'text-slate-300')}>
        {over ? `+${clock(due, now)}` : clock(now, due)}
      </span>
    </span>
  )
}

// Nearest addresses for AVL positions, rounded to about 100 m so a moving car asks rarely.
const near = new Map<string, string | null>()
const asked = new Set<string>()
function useNear(lat?: number, lon?: number): string | undefined {
  const [, bump] = useState(0)
  if (lat === undefined || lon === undefined) return undefined
  const k = `${lat.toFixed(3)},${lon.toFixed(3)}`
  if (!asked.has(k)) {
    asked.add(k)
    api.get<SearchResult>(`/map/reverse?lat=${lat.toFixed(5)}&lon=${lon.toFixed(5)}`)
      .then((r) => near.set(k, r.label)).catch(() => near.set(k, null)).finally(() => bump((n) => n + 1))
  }
  return near.get(k) ?? undefined
}

function Where({ view, u, now }: { view: View; u: Unit; now: number }) {
  const n = useNear(u.position?.lat, u.position?.lon)
  const w = where(view, u, now)
  // The reverse lookup may already say "Near …" (a road with no address): say it once.
  const text = w.src.startsWith('AVL') && n ? (/^near\b/i.test(n) ? n : `near ${n}`) : w.text
  return (
    <>
      <div className="flex items-center gap-1"><MapPin size={11} className="shrink-0 text-slate-500" aria-hidden /><span className="truncate text-slate-200">{text}</span></div>
      {w.src && <div className="pl-4 text-[10px] text-slate-500">{w.src}</div>}
    </>
  )
}

function where(view: View, u: Unit, now: number): { text: string; src: string } {
  if (u.status === 'markout' && u.markOut?.location?.address) return { text: u.markOut.location.address, src: 'given' }
  const inc = u.incident ? view.incidents.find((i) => i.number === u.incident) : undefined
  if (inc?.location?.address) return { text: inc.location.address, src: 'incident' }
  const a = avl(view, u, now)
  if (u.position) return { text: `AVL ${u.position.lat.toFixed(4)}, ${u.position.lon.toFixed(4)}`, src: a === 'fresh' ? `AVL ${ago(u.position.at, now)}` : `AVL stale ${ago(u.position.at, now)}` }
  if (u.status === 'markout' && u.markOut?.location) return { text: 'Its position when marked out', src: 'AVL' }
  const st = view.stations.find((s) => s.id === (u.covering || u.station))
  return { text: st ? st.name : 'Unknown', src: st ? 'station' : '' }
}

/** Police status and mark-outs: every car, what it is doing, where, for how long, and when it is due a status check. */
export default function PoliceBoard({ view }: { view: View }) {
  const now = useNow()
  const me = useStore((s) => s.me)
  const selUnit = useStore((s) => s.selectedUnit)
  const [filter, setFilter] = useState<Filter>('all')
  const canAct = can.dispatch(me)
  const units = view.units
  const due = (u: Unit) => checkDue(view, u)
  const overdueUnits = units.filter((u) => due(u) && now > due(u))
  const soonUnits = units.filter((u) => due(u) && now <= due(u) && due(u) - now < 5 * 60000)
  const committed = units.filter((u) => u.incident)
  const marked = units.filter((u) => u.status === 'markout')
  const avail = units.filter((u) => u.status === 'available')
  const oos = units.filter((u) => u.status === 'oos')
  const byKind = new Map<string, number>()
  for (const u of marked) byKind.set(u.markOut?.code ?? '?', (byKind.get(u.markOut?.code ?? '?') ?? 0) + 1)
  const rows = units
    .filter((u) => filter === 'all' || (filter === 'calls' ? !!u.incident : filter === 'markout' ? u.status === 'markout' : !!due(u) && (now > due(u) || due(u) - now < 5 * 60000)))
    .sort((a, b) => {
      const oa = due(a) && now > due(a) ? now - due(a) : -1, ob = due(b) && now > due(b) ? now - due(b) : -1
      if (oa !== ob) return ob - oa
      const da = due(a) || Infinity, db = due(b) || Infinity
      if (da !== db) return da - db
      return ORDER.indexOf(a.status) - ORDER.indexOf(b.status) || a.id.localeCompare(b.id, undefined, { numeric: true })
    })
  const kpi = (n: number, label: string, cls: string, f?: Filter, extra?: React.ReactNode) => (
    <button type="button" onClick={() => f && setFilter(filter === f ? 'all' : f)} aria-pressed={!!f && filter === f}
      className={cx('flex min-w-[6.5rem] flex-col rounded-lg border px-3 py-1.5 text-left transition', f && filter === f ? 'border-sky-500 bg-sky-500/10' : 'border-slate-800 bg-slate-900/60 hover:border-slate-600', !f && 'cursor-default')}>
      <span className={cx('font-mono text-[26px] font-black leading-none tnum', cls)}>{n}</span>
      <span className="mt-0.5 text-[10px] font-semibold uppercase tracking-wider text-slate-400">{label}</span>
      {extra}
    </button>
  )
  return (
    <section className="panel flex min-h-0 flex-1 flex-col" aria-label="Police status and mark-outs">
      <div className="flex flex-wrap items-stretch gap-2 border-b border-slate-800 p-2">
        {kpi(avail.length, 'available', avail.length === 0 ? 'text-rose-300' : 'text-emerald-300')}
        {kpi(committed.length, 'on calls', 'text-orange-300', 'calls')}
        {kpi(marked.length, 'marked out', 'text-teal-300', 'markout',
          byKind.size > 0 && (
            <span className="mt-1 flex flex-wrap gap-1">
              {[...byKind].map(([k, n]) => {
                const Icon = markOutIcon(k)
                return <span key={k} className="inline-flex items-center gap-0.5 rounded bg-teal-500/10 px-1 font-mono text-[10px] text-teal-200" title={markOutType(view, k)?.name ?? k}><Icon size={10} aria-hidden />{k} {n}</span>
              })}
            </span>
          ))}
        {kpi(oos.length, 'out of service', 'text-slate-400')}
        {kpi(overdueUnits.length, 'status checks due', overdueUnits.length ? 'text-rose-300' : 'text-slate-500', 'due',
          soonUnits.length > 0 && <span className="mt-0.5 text-[10px] text-amber-300">{soonUnits.length} more within 5 min</span>)}
        {overdueUnits.length > 0 && (
          <div className="flash ml-auto flex items-center gap-2 self-center rounded-lg px-3 py-2 text-sm font-bold text-white">
            <BellRing size={16} aria-hidden /> Check {overdueUnits.map((u) => u.id).join(', ')}
          </div>
        )}
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        <table className="w-full border-separate border-spacing-0 text-xs">
          <thead className="sticky top-0 z-10 bg-slate-950 text-[10px] uppercase tracking-wider text-slate-500">
            <tr>
              {['Unit', 'Status', 'Doing', 'Where', 'Time', 'Status check', 'Last check', 'Radio', ''].map((h) => <th key={h} className="border-b border-slate-800 px-2 py-1.5 text-left font-semibold">{h}</th>)}
            </tr>
          </thead>
          <tbody>
            {rows.map((u) => {
              const d = due(u)
              const isOver = !!d && now > d
              const inc = u.incident ? view.incidents.find((i) => i.number === u.incident) : undefined
              const Icon = markOutIcon(u.markOut?.code)
              const meta = statusMeta(u.status)
              return (
                <UnitMenu key={u.id} u={u} view={view}>
                  <tr onClick={() => store.selectUnit(selUnit === u.id ? null : u.id)}
                    className={cx('cursor-pointer align-middle transition-colors', isOver ? 'bg-rose-500/10 hover:bg-rose-500/15' : selUnit === u.id ? 'bg-sky-500/10' : 'hover:bg-slate-800/50')}>
                    <td className="border-b border-slate-800/70 px-2 py-1.5" style={{ boxShadow: `inset 3px 0 0 ${meta.dot}` }}>
                      <div className="font-mono text-[15px] font-black text-slate-50">{u.id}</div>
                      <div className="text-[10px] text-slate-500">{u.radioAlias || view.settings.unitTypes.find((t) => t.code === u.type)?.name}</div>
                    </td>
                    <td className="border-b border-slate-800/70 px-2"><StatusChip status={u.status} big /><div className="mt-0.5 text-[10px] text-slate-500">{meta.label}</div></td>
                    <td className="max-w-[16rem] border-b border-slate-800/70 px-2">
                      {inc ? (
                        <button className="flex items-center gap-1.5 text-left" onClick={(e) => { e.stopPropagation(); store.select(inc.number) }}>
                          <Priority p={inc.priority} />
                          <span className="min-w-0"><span className="block font-mono text-sky-300 underline">{inc.number}</span><span className="block truncate text-slate-300">{typeName(view, inc.type)}</span></span>
                        </button>
                      ) : u.status === 'markout' ? (
                        <span className="flex items-center gap-1.5">
                          <span className="grid h-7 w-7 shrink-0 place-items-center rounded-md bg-teal-500/15 text-teal-300"><Icon size={15} aria-hidden /></span>
                          <span className="min-w-0"><span className="block truncate font-semibold text-teal-200">{activity(view, u)}</span>{u.markOut?.note && <span className="block truncate text-[10px] text-slate-400">{u.markOut.note}</span>}</span>
                        </span>
                      ) : <span className="text-slate-500">{u.status === 'available' ? 'Patrol' : u.status === 'oos' ? 'Out of service' : ''}</span>}
                    </td>
                    <td className="max-w-[18rem] border-b border-slate-800/70 px-2"><Where view={view} u={u} now={now} /></td>
                    <td className="border-b border-slate-800/70 px-2 font-mono text-[13px] text-slate-200 tnum">{u.status === 'available' || u.status === 'oos' ? <span className="text-slate-600">{ago(u.statusAt, now)}</span> : clock(u.statusAt, now)}</td>
                    <td className="border-b border-slate-800/70 px-2"><CheckRing view={view} u={u} now={now} /></td>
                    <td className="border-b border-slate-800/70 px-2 text-[11px] text-slate-400">{u.checked && ms(u.checked) > ms(u.statusAt) ? <>{ago(u.checked, now)} ago<div className="text-[10px] text-slate-500">{u.checkedBy}</div></> : <span className="text-slate-600">since the status</span>}</td>
                    <td className="border-b border-slate-800/70 px-2 text-[11px]">
                      {u.lastHeard ? <span className={cx('inline-flex items-center gap-1', now - ms(u.lastHeard) < 30000 ? 'text-emerald-300' : 'text-slate-400')}><Radio size={11} aria-hidden />{ago(u.lastHeard, now)}</span> : <span className="text-slate-600">not heard</span>}
                    </td>
                    <td className="border-b border-slate-800/70 px-2 text-right">
                      {canAct && (
                        <span className="inline-flex gap-1" onClick={(e) => e.stopPropagation()}>
                          {d > 0 && <button className={cx('btn h-7 px-2 text-[11px]', isOver ? 'btn-go' : 'text-emerald-300')} onClick={() => run(`SC ${u.id}`)} title={`Status check: ${u.id} is all right`}><Check size={12} aria-hidden /> OK</button>}
                          <button className="btn h-7 px-2 text-[11px] text-teal-300" onClick={() => openMarkOut(u.id)} title="Mark out: meal, court, fuel…">MO</button>
                          {u.status !== 'available' && !u.incident && <button className="btn h-7 px-2 font-mono text-[11px] text-emerald-300" onClick={() => run(`AV ${u.id}`)} title="Available">AV</button>}
                        </span>
                      )}
                    </td>
                  </tr>
                </UnitMenu>
              )
            })}
          </tbody>
        </table>
        {rows.length === 0 && <p className="p-4 text-sm text-slate-500">{units.length ? 'Nothing matches the filter.' : 'No police units on this screen: add units of a police type in Admin › Units, or choose the screen’s disciplines in Screens.'}</p>}
      </div>
    </section>
  )
}
