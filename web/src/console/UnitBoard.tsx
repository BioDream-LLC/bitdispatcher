import { Radio } from 'lucide-react'
import * as CM from '@radix-ui/react-context-menu'
import { useDraggable } from '@dnd-kit/core'
import { toast } from 'sonner'
import { api, errText } from '../api'
import { can, store, useStore } from '../store'
import { activity, avl, clock, cx, discipline, markOutOver, ms, overdue, statusMeta, timerMinutes, useNow } from '../util'
import { openMarkOut } from './MarkOut'
import type { Unit, View } from '../types'

/** Runs a command line as the dispatcher, with the result as a toast. */
export const run = (line: string, incident = '') => api.post<{ ok: boolean; message: string }>('/command', { line, incident })
  .then((r) => (r.ok ? toast.success(r.message) : toast.error(r.message))).catch((e) => toast.error(errText(e)))

const STATUS_CMDS: [string, string][] = [['ER', 'En route'], ['OS', 'On scene'], ['TR', 'Transporting'], ['AD', 'At destination'], ['AV', 'Available'], ['OOS', 'Out of service']]
const item = 'flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-xs text-slate-200 outline-none data-[highlighted]:bg-slate-700'

/** Right-click on a unit: its statuses, sending it, moving it up, in one click each. */
export function UnitMenu({ u, view, children }: { u: Unit; view: View; children: React.ReactNode }) {
  const selected = useStore((s) => s.selected)
  const me = useStore((s) => s.me)
  if (!can.dispatch(me)) return <>{children}</>
  return (
    <CM.Root>
      <CM.Trigger asChild>{children}</CM.Trigger>
      <CM.Portal>
        <CM.Content className="z-50 min-w-48 rounded-md border border-slate-700 bg-slate-900 p-1 shadow-2xl">
          <CM.Label className="px-2 py-1 font-mono text-[11px] font-bold text-slate-400">{u.id} · {statusMeta(u.status).label}{u.incident ? ` · ${u.incident}` : ''}</CM.Label>
          {selected && u.incident !== selected && <CM.Item className={item} onSelect={() => run(`D ${u.id} #${selected}`)}>Send to {selected}</CM.Item>}
          <CM.Separator className="my-1 h-px bg-slate-800" />
          {STATUS_CMDS.filter(([, l]) => l.toLowerCase().replace(/ /g, '') !== u.status).map(([c, l]) => (
            <CM.Item key={c} className={item} onSelect={() => run(`${c} ${u.id}`)}>{l}<span className="ml-auto font-mono text-[10px] text-slate-500">{c} {u.id}</span></CM.Item>
          ))}
          <CM.Item className={item} onSelect={() => openMarkOut(u.id)}>Mark out…<span className="ml-auto font-mono text-[10px] text-slate-500">MO {u.id}</span></CM.Item>
          {timerMinutes(view, u) > 0 && <CM.Item className={item} onSelect={() => run(`SC ${u.id}`)}>Status check: all right<span className="ml-auto font-mono text-[10px] text-slate-500">SC {u.id}</span></CM.Item>}
          <CM.Separator className="my-1 h-px bg-slate-800" />
          <CM.Sub>
            <CM.SubTrigger className={item}>Move up to…</CM.SubTrigger>
            <CM.Portal>
              <CM.SubContent className="z-50 min-w-44 rounded-md border border-slate-700 bg-slate-900 p-1 shadow-2xl">
                {view.stations.filter((s) => s.id !== (u.covering || u.station)).map((s) => (
                  <CM.Item key={s.id} className={item} onSelect={() => run(`MU ${u.id} ${s.id}`)}><span className="font-mono">{s.id}</span> {s.name}</CM.Item>
                ))}
              </CM.SubContent>
            </CM.Portal>
          </CM.Sub>
          {u.covering && <CM.Item className={item} onSelect={() => run(`RET ${u.id}`)}>Home to {u.station}</CM.Item>}
          <CM.Item className={item} onSelect={() => store.selectUnit(u.id)}>Details</CM.Item>
        </CM.Content>
      </CM.Portal>
    </CM.Root>
  )
}

const ORDER = ['fire', 'ems', 'police', 'other']

export function Tile({ u, view, now, size = 'md' }: { u: Unit; view: View; now: number; size?: 'md' | 'lg' }) {
  const me = useStore((s) => s.me)
  const selected = useStore((s) => s.selected)
  const selUnit = useStore((s) => s.selectedUnit)
  const pick = useStore((s) => s.pick)
  const meta = statusMeta(u.status)
  const over = overdue(view, u, now)
  const a = avl(view, u, now)
  const picked = pick.includes(u.id)
  const heard = u.lastHeard && now - ms(u.lastHeard) < 30000
  const pickable = !!selected && can.dispatch(me) && u.incident !== selected
  const mo = u.status === 'markout'
  // Drag a unit onto an incident in the queue to send it there.
  const drag = useDraggable({ id: 'unit:' + u.id, disabled: !can.dispatch(me) })
  return (
    <UnitMenu u={u} view={view}>
    <button
      ref={drag.setNodeRef}
      {...drag.listeners}
      {...drag.attributes}
      style={drag.transform ? { transform: `translate(${drag.transform.x}px, ${drag.transform.y}px)`, zIndex: 50 } : undefined}
      type="button"
      onClick={() => (pickable ? store.togglePick(u.id) : store.selectUnit(selUnit === u.id ? null : u.id))}
      onDoubleClick={() => store.selectUnit(u.id)}
      aria-pressed={picked}
      title={`${u.id} ${meta.label}${u.incident ? ' on ' + u.incident : ''}${mo ? ': ' + activity(view, u) + (u.markOut?.location?.address ? ' at ' + u.markOut.location.address : '') : ''} · AVL ${a}${pickable ? ' · click to add to the dispatch' : ''}`}
      className={cx(
        'relative flex shrink-0 flex-col justify-between rounded border text-left',
        size === 'lg' ? 'h-[62px] w-[112px] px-2 py-1.5' : 'h-[46px] w-[84px] px-1.5 py-1',
        meta.bg, u.emergency ? 'flash border-white text-white' : 'border-slate-800',
        over > 0 && 'overdue border-amber-400',
        picked && 'ring-2 ring-rose-400', selUnit === u.id && 'ring-2 ring-sky-300',
        u.incident && u.incident === selected && 'border-sky-500',
      )}
    >
      <div className="flex items-center gap-1">
        <span className={cx('font-mono font-black leading-none text-slate-50', size === 'lg' ? 'text-[17px]' : 'text-[13px]')}>{u.id}</span>
        <span className={cx('ml-auto font-mono text-[10px] font-bold', u.emergency ? 'text-white' : meta.fg)}>{meta.code}</span>
      </div>
      {size === 'lg' && <div className="truncate text-[10px] leading-none text-slate-400">{activity(view, u) || (u.covering ? `covering ${u.covering}` : u.station ?? '')}</div>}
      <div className={cx('flex items-center gap-1 font-mono leading-none', size === 'lg' ? 'text-[11px]' : 'text-[10px]')}>
        <span className={cx('tnum', over > 0 ? 'font-bold text-amber-300' : 'text-slate-300')}>{u.status === 'available' || u.status === 'oos' ? '' : clock(Math.max(ms(u.statusAt), ms(u.checked)), now)}</span>
        {u.incident && <span className="truncate text-sky-300">{u.incident.slice(-4)}</span>}
        {mo && <span className={cx('truncate font-bold', markOutOver(view, u, now) ? 'text-amber-300' : 'text-teal-200')}>{u.markOut?.code}{markOutOver(view, u, now) ? ' OVER' : ''}</span>}
        <span className="ml-auto flex items-center gap-0.5">
          {heard && <Radio size={9} className="text-emerald-300 live-dot" aria-label="talking on the radio" />}
          <span className={cx('h-1.5 w-1.5 rounded-full', a === 'fresh' ? 'bg-emerald-400' : a === 'stale' ? 'bg-amber-400' : 'bg-slate-600')} aria-label={`AVL ${a}`} />
        </span>
      </div>
      {u.covering && <span className="absolute -top-1 right-0 rounded bg-sky-600 px-0.5 font-mono text-[8px] text-white" title={`covering ${u.covering}`}>↗{u.covering}</span>}
    </button>
    </UnitMenu>
  )
}

export default function UnitBoard({ view, size = 'md' }: { view: View; size?: 'md' | 'lg' }) {
  const now = useNow()
  const selected = useStore((s) => s.selected)
  const pick = useStore((s) => s.pick)
  const groups = new Map<string, Unit[]>()
  for (const u of view.units) {
    const d = discipline(view, u)
    if (!groups.has(d)) groups.set(d, [])
    groups.get(d)!.push(u)
  }
  const keys = [...groups.keys()].sort((a, b) => (ORDER.indexOf(a) + 1 || 9) - (ORDER.indexOf(b) + 1 || 9))
  return (
    <section className="panel flex min-h-0 flex-col" aria-label="Unit status board">
      <div className="panel-head">
        Units <span className="font-mono text-slate-300">{view.units.length}</span>
        {view.statuses.map((s) => {
          const n = view.units.filter((u) => u.status === s).length
          return n ? <span key={s} className={cx('font-mono normal-case', statusMeta(s).fg)}>{statusMeta(s).code} {n}</span> : null
        })}
        <span className="ml-auto normal-case tracking-normal text-slate-500">
          {selected ? (pick.length ? `${pick.length} chosen for ${selected}: Ctrl+Enter dispatches` : `click units to choose them for ${selected}`) : 'drag a unit onto an incident to send it · right-click for statuses and move-ups · dot: AVL'}
        </span>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-1.5">
        {view.units.length === 0 ? (
          <p className="p-2 text-xs text-slate-500">No units yet. Add stations and units in Admin, or start with <span className="font-mono">-demo</span> for demonstration units.</p>
        ) : (
          <div className="flex flex-col gap-1">
            {keys.map((k) => (
              <div key={k} className="flex items-start gap-1.5">
                <span className={cx('shrink-0 pt-1 font-bold uppercase tracking-wider text-slate-500', size === 'lg' ? 'w-14 text-[11px]' : 'w-10 text-[9px]')}>{k}</span>
                <div className="flex flex-wrap gap-1">
                  {groups.get(k)!.map((u) => <Tile key={u.id} u={u} view={view} now={now} size={size} />)}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </section>
  )
}
