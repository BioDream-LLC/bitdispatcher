import { ArrowRightLeft, House, Siren } from 'lucide-react'
import { can, useStore } from '../store'
import { activity, clock, cx, mins, ms, statusMeta, useNow } from '../util'
import type { Gap, Station, Unit, View } from '../types'
import { Tile, run } from './UnitBoard'
import { useCoverage } from './Situation'

type House = { s: Station; homed: Unit[]; guests: Unit[]; inQuarters: Unit[]; gaps: Gap[] }

// at is where a unit is posted: the station it covers, or its own.
const at = (u: Unit) => u.covering || u.station || ''

/** The bays of a house: one door per unit that lives there, shut and green when it is in, open in its status colour when it is out. */
function Bays({ h }: { h: House }) {
  const n = Math.max(1, h.homed.length + h.guests.length)
  const W = 100 / n
  return (
    <svg viewBox="0 0 100 26" preserveAspectRatio="none" className="h-7 w-full" aria-hidden>
      <path d="M0 8 L50 0.6 L100 8" fill="none" stroke="#475569" strokeWidth="0.8" vectorEffect="non-scaling-stroke" />
      {[...h.homed, ...h.guests].map((u, i) => {
        const inside = at(u) === h.s.id && u.status === 'available'
        const c = statusMeta(u.status).dot
        const x = i * W + 1.2, w = W - 2.4
        return (
          <g key={u.id}>
            <rect x={x} y={9} width={w} height={16} rx="0.8" fill={inside ? '#064e3b' : '#020617'} stroke={inside ? '#10b981' : c} strokeWidth="0.9" vectorEffect="non-scaling-stroke" />
            {inside
              ? [12, 15, 18, 21].map((y) => <line key={y} x1={x + 0.8} x2={x + w - 0.8} y1={y} y2={y} stroke="#10b981" strokeOpacity="0.45" strokeWidth="0.6" vectorEffect="non-scaling-stroke" />)
              : <rect x={x} y={9} width={w} height={3} fill={c} fillOpacity="0.9" />}
          </g>
        )
      })}
    </svg>
  )
}

function HouseCard({ h, view, now, canAct }: { h: House; view: View; now: number; canAct: boolean }) {
  const empty = h.inQuarters.length === 0
  const covered = h.guests.some((u) => u.status === 'available')
  const allIn = !empty && h.homed.every((u) => at(u) === h.s.id && u.status === 'available')
  const tone = empty ? 'rose' : covered ? 'sky' : allIn ? 'emerald' : 'amber'
  const stripe = { rose: 'bg-rose-500', sky: 'bg-sky-500', emerald: 'bg-emerald-500', amber: 'bg-amber-400' }[tone]
  const label = empty ? 'EMPTY' : covered ? 'COVERED' : allIn ? 'IN QUARTERS' : 'PARTIAL'
  const since = empty ? Math.max(...[...h.homed, ...h.guests].map((u) => ms(u.statusAt)), 0) : 0
  return (
    <article className={cx('relative flex flex-col overflow-hidden rounded-lg border bg-slate-900/70', empty ? 'border-rose-700/70 shadow-[0_0_0_1px_rgba(244,63,94,0.25)]' : 'border-slate-800')} aria-label={`${h.s.name}: ${label.toLowerCase()}`}>
      <div className={cx('h-1', stripe, empty && 'live-dot')} />
      <header className="flex items-start gap-2 px-2.5 pt-2">
        <span className="rounded bg-slate-800 px-1.5 py-0.5 font-mono text-[15px] font-black text-slate-50">{h.s.id}</span>
        <div className="min-w-0 flex-1 leading-tight">
          <div className="truncate text-[13px] font-semibold text-slate-100">{h.s.name}</div>
          <div className="truncate text-[10px] text-slate-500">{h.s.address}</div>
        </div>
        <div className="text-right leading-tight">
          <span className={cx('chip h-5 px-1.5 text-[10px] font-black tracking-wider', {
            rose: 'bg-rose-500/20 text-rose-200', sky: 'bg-sky-500/15 text-sky-200', emerald: 'bg-emerald-500/15 text-emerald-300', amber: 'bg-amber-500/15 text-amber-200',
          }[tone])}>{label}</span>
          <div className="mt-0.5 font-mono text-[10px] text-slate-400 tnum">{h.inQuarters.length}/{h.homed.length + h.guests.length} in{empty && since ? ` · ${clock(since, now)}` : ''}</div>
        </div>
      </header>
      <div className="px-2.5"><Bays h={h} /></div>
      <div className="flex flex-wrap gap-1 px-2.5 pb-2 pt-1">
        {h.homed.map((u) => <Tile key={u.id} u={u} view={view} now={now} size="lg" />)}
        {h.guests.map((u) => (
          <div key={u.id} className="relative">
            <Tile u={u} view={view} now={now} size="lg" />
            <span className="pointer-events-none absolute -bottom-1 left-1 rounded bg-sky-600 px-1 text-[8px] font-bold uppercase text-white">from {u.station}</span>
          </div>
        ))}
      </div>
      {h.gaps.length > 0 && (
        <footer className="mt-auto space-y-1 border-t border-slate-800 bg-slate-950/50 px-2.5 py-1.5">
          {h.gaps.map((g) => (
            <div key={g.type} className="flex flex-wrap items-center gap-x-1.5 text-[11px]">
              <Siren size={11} className="shrink-0 text-rose-300" aria-hidden />
              <span className="text-rose-200">No {g.typeName.toLowerCase()} in quarters</span>
              {g.move ? (
                <>
                  <span className="text-slate-400">· move up <span className="font-mono text-slate-200">{g.move.unit}</span> from {g.move.from}{g.move.seconds ? ` (${mins(g.move.seconds)})` : ''}</span>
                  {canAct && <button className="btn ml-auto h-5 px-1.5 py-0 text-[10px] text-sky-300" onClick={() => run(`MU ${g.move!.unit} ${h.s.id}`)}><ArrowRightLeft size={10} aria-hidden /> Move up</button>}
                </>
              ) : <span className="text-slate-500">· nothing free to move up</span>}
            </div>
          ))}
        </footer>
      )}
    </article>
  )
}

/** Unit types across the houses: how many of each are in quarters, with every status in a bar. */
function TypeBars({ view, units }: { view: View; units: Unit[] }) {
  const types = view.settings.unitTypes.filter((t) => units.some((u) => u.type === t.code))
  return (
    <div className="flex flex-wrap gap-x-4 gap-y-1.5">
      {types.map((t) => {
        const us = units.filter((u) => u.type === t.code)
        const av = us.filter((u) => u.status === 'available').length
        return (
          <div key={t.code} className="min-w-[8.5rem]">
            <div className="flex items-baseline gap-1 text-[11px]">
              <span className="font-semibold text-slate-300">{t.name}</span>
              <span className={cx('ml-auto font-mono font-bold tnum', av === 0 ? 'text-rose-300' : av === 1 ? 'text-amber-300' : 'text-emerald-300')}>{av}</span>
              <span className="font-mono text-slate-500">/{us.length}</span>
            </div>
            <div className="mt-0.5 flex h-1.5 overflow-hidden rounded-full bg-slate-800">
              {view.statuses.map((s) => {
                const n = us.filter((u) => u.status === s).length
                return n ? <span key={s} style={{ width: `${(n / us.length) * 100}%`, background: statusMeta(s).dot }} title={`${n} ${statusMeta(s).label}`} /> : null
              })}
            </div>
          </div>
        )
      })}
    </div>
  )
}

/** Fire houses: every station, its apparatus and their status, who covers whom, the empty houses and their move-ups. */
export default function StationBoard({ view }: { view: View }) {
  const now = useNow()
  const me = useStore((s) => s.me)
  const cov = useCoverage(view.seq)
  const houses: House[] = view.stations
    .map((s) => {
      const homed = view.units.filter((u) => u.station === s.id)
      const guests = view.units.filter((u) => u.covering === s.id && u.station !== s.id)
      const inQuarters = [...homed, ...guests].filter((u) => at(u) === s.id && u.status === 'available')
      return { s, homed, guests, inQuarters, gaps: (cov?.gaps ?? []).filter((g) => g.station === s.id && view.units.some((u) => u.type === g.type)) }
    })
    .filter((h) => h.homed.length + h.guests.length > 0)
    .sort((a, b) => a.s.id.localeCompare(b.s.id, undefined, { numeric: true }))
  const empty = houses.filter((h) => !h.inQuarters.length).length
  const units = houses.flatMap((h) => h.homed)
  const away = units.filter((u) => u.status !== 'available' || at(u) !== u.station)
  return (
    <section className="panel flex min-h-0 flex-1 flex-col" aria-label="Fire houses">
      <div className="flex flex-wrap items-center gap-x-6 gap-y-2 border-b border-slate-800 px-3 py-2">
        <div className="flex items-center gap-2">
          <House size={18} className="text-rose-300" aria-hidden />
          <div className="leading-tight">
            <div className="font-mono text-2xl font-black text-slate-50 tnum">{houses.length - empty}<span className="text-base text-slate-500">/{houses.length}</span></div>
            <div className="text-[10px] uppercase tracking-wider text-slate-400">houses with a unit in</div>
          </div>
        </div>
        <div className={cx('leading-tight', empty ? 'text-rose-300' : 'text-slate-500')}>
          <div className="font-mono text-2xl font-black tnum">{empty}</div>
          <div className="text-[10px] uppercase tracking-wider">empty</div>
        </div>
        <div className="leading-tight text-slate-300">
          <div className="font-mono text-2xl font-black tnum">{away.length}</div>
          <div className="text-[10px] uppercase tracking-wider text-slate-400">units out</div>
        </div>
        <div className="min-w-0 flex-1"><TypeBars view={view} units={units} /></div>
      </div>
      {houses.length === 0 ? (
        <p className="p-4 text-sm text-slate-500">No station has units on this screen. Stations and their units are set in Admin › Stations and Admin › Units; the screen’s disciplines are chosen in Screens.</p>
      ) : (
        <div className="grid min-h-0 flex-1 auto-rows-min grid-cols-[repeat(auto-fill,minmax(19rem,1fr))] gap-2 overflow-y-auto p-2">
          {houses.map((h) => (
            <HouseCard key={h.s.id} h={h} view={view} now={now} canAct={can.dispatch(me)} />
          ))}
        </div>
      )}
      <p className="sr-only">{houses.map((h) => `${h.s.name}: ${h.inQuarters.length} in quarters; ${h.homed.filter((u) => u.status !== 'available').map((u) => `${u.id} ${activity(view, u) || statusMeta(u.status).label}`).join(', ')}`).join('. ')}</p>
    </section>
  )
}
