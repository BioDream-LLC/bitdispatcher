import { useDroppable } from '@dnd-kit/core'
import { toast } from 'sonner'
import { MapPin, MessageSquareText, Phone, PhoneIncoming, Plus, Video, Link2, AlertTriangle, Image as ImageIcon } from 'lucide-react'
import { api, errText } from '../api'
import { can, store, useStore } from '../store'
import { Priority, StatusChip, Tip } from '../ui'
import { ago, clock, cx, hasPoint, locText, locationStale, ms, phone, prio, typeName, uncertainty, useNow, overdue } from '../util'
import type { Call, Incident, View } from '../types'

function CallCard({ c, view, now, onCreate }: { c: Call; view: View; now: number; onCreate: (c: Call) => void }) {
  const me = useStore((s) => s.me)
  const selected = useStore((s) => s.selected)
  const selCall = useStore((s) => s.selectedCall)
  const ringing = c.state === 'ringing'
  const stale = locationStale(view, c.location, now)
  const photos = (c.media ?? []).filter((m) => m.purpose === 'caller-photo').length
  return (
    <li>
      <div
        role="button"
        tabIndex={0}
        aria-pressed={selCall === c.id}
        onClick={() => store.selectCall(selCall === c.id ? null : c.id)}
        onKeyDown={(e) => e.key === 'Enter' && onCreate(c)}
        className={cx(
          'group relative cursor-pointer border-l-4 px-2.5 py-1.5 text-xs outline-none hover:bg-slate-800/60 focus-visible:bg-slate-800/60',
          ringing ? 'border-rose-500 bg-rose-500/10' : c.state === 'ended' ? 'border-slate-700 opacity-60' : 'border-sky-500 bg-sky-500/5',
          selCall === c.id && 'bg-slate-800',
        )}
      >
        <div className="flex items-center gap-1.5">
          {ringing ? <PhoneIncoming size={13} className="text-rose-300 live-dot" aria-hidden /> : <Phone size={13} className="text-sky-300" aria-hidden />}
          <span className="font-mono text-[13px] font-bold text-slate-50">{phone(c.callback) || 'No callback'}</span>
          {c.class && <span className="chip bg-slate-800 font-mono text-slate-300">{c.class}</span>}
          <span className={cx('chip uppercase', ringing ? 'bg-rose-500 text-white' : c.state === 'ended' ? 'bg-slate-700 text-slate-300' : 'bg-sky-500/20 text-sky-300')}>{c.state}</span>
          <span className="ml-auto font-mono font-semibold text-slate-300 tnum">{clock(c.start, now)}</span>
        </div>
        <div className="mt-0.5 flex items-start gap-1">
          <MapPin size={12} className={cx('mt-px shrink-0', c.location ? (stale ? 'text-amber-300' : 'text-emerald-300') : 'text-rose-300')} aria-hidden />
          <span className={cx('min-w-0 flex-1 truncate', c.location ? 'text-slate-200' : 'text-rose-300')}>{locText(c.location)}</span>
        </div>
        <div className="mt-0.5 flex flex-wrap items-center gap-1 text-[10px] text-slate-400">
          {c.location && <span className="font-mono">{uncertainty(c.location) || 'no uncertainty given'}</span>}
          {stale && <span className="chip bg-amber-500/20 text-amber-300">STALE {ago(c.location?.at, now)}</span>}
          {c.locations > 1 && <span className="chip bg-slate-800 text-slate-300">{c.locations} fixes</span>}
          {c.callerName && <span className="truncate text-slate-300">{c.callerName}</span>}
          {c.text?.length ? <span className="chip bg-violet-500/20 text-violet-300"><MessageSquareText size={10} aria-hidden /> {c.text.length}</span> : null}
          {c.textOnly && <span className="chip bg-violet-500 text-white">TEXT ONLY</span>}
          {c.video && <span className={cx('chip', c.video.live ? 'bg-rose-500/25 text-rose-200' : 'bg-slate-800 text-slate-300')}><Video size={10} aria-hidden /> {c.video.live ? 'LIVE' : 'video'}</span>}
          {photos > 0 && <span className="chip bg-slate-800 text-slate-300"><ImageIcon size={10} aria-hidden /> {photos}</span>}
          {(c.problems ?? []).length > 0 && (
            <Tip text={c.problems!.join('; ')}>
              <span className="chip bg-amber-500/20 text-amber-300"><AlertTriangle size={10} aria-hidden /> {c.problems!.length}</span>
            </Tip>
          )}
          <span className="ml-auto font-mono text-slate-500">{c.sources.join('+')}{c.agent ? ` · ${c.agent}` : ''}</span>
        </div>
        {can.dispatch(me) && c.state !== 'ended' && !c.incident && (
          <div className="mt-1 flex gap-1">
            <button className="btn h-6 px-2 py-0 text-[11px]" onClick={(e) => { e.stopPropagation(); onCreate(c) }}>
              <Plus size={12} aria-hidden /> Incident <span className="kbd">↵</span>
            </button>
            {selected && (
              <button
                className="btn h-6 px-2 py-0 text-[11px]"
                onClick={async (e) => {
                  e.stopPropagation()
                  try {
                    await api.post(`/calls/${encodeURIComponent(c.id)}/attach`, { incident: selected })
                    toast.success(`Call attached to ${selected}`)
                  } catch (err) {
                    toast.error(errText(err))
                  }
                }}
              >
                <Link2 size={12} aria-hidden /> to {selected}
              </button>
            )}
          </div>
        )}
      </div>
    </li>
  )
}

function IncidentRow({ i, view, now }: { i: Incident; view: View; now: number }) {
  const selected = useStore((s) => s.selected)
  const p = prio(i.priority)
  const live = i.units.filter((a) => !a.cleared)
  const anyOver = live.some((a) => {
    const u = view.units.find((x) => x.id === a.unit)
    return u && overdue(view, u, now) > 0
  })
  const pendingFor = now - ms(i.created)
  const late = i.status === 'pending' && pendingFor > (i.priority <= 1 ? 60000 : i.priority === 2 ? 180000 : 600000)
  // A unit tile dropped here is dispatched to this incident.
  const drop = useDroppable({ id: 'inc:' + i.number, disabled: i.status === 'closed' })
  return (
    <li ref={drop.setNodeRef} className={cx(drop.isOver && 'outline outline-2 -outline-offset-2 outline-rose-400 bg-rose-500/10')}>
      <button
        type="button"
        onClick={() => store.select(selected === i.number ? null : i.number)}
        aria-pressed={selected === i.number}
        className={cx('block w-full border-l-4 px-2.5 py-1.5 text-left text-xs hover:bg-slate-800/60', p.border, selected === i.number ? 'bg-slate-800' : '', i.status === 'closed' && 'opacity-50')}
      >
        <div className="flex items-center gap-1.5">
          <Priority p={i.priority} />
          <span className="font-mono font-bold text-slate-100">{i.number}</span>
          <span className="truncate font-semibold text-slate-200">{typeName(view, i.type)}</span>
          <span className={cx('ml-auto font-mono tnum', late ? 'font-bold text-rose-300' : 'text-slate-400')}>{clock(i.created, now)}</span>
        </div>
        <div className="mt-0.5 flex items-center gap-1 text-slate-400">
          <MapPin size={11} className={hasPoint(i.location) ? 'text-slate-400' : 'text-amber-300'} aria-hidden />
          <span className="truncate">{locText(i.location)}</span>
          {i.calls.length > 0 && <span className="ml-auto shrink-0 font-mono text-[10px] text-sky-300"><Phone size={10} className="inline" aria-hidden /> {i.calls.length}</span>}
        </div>
        {(live.length > 0 || i.status === 'pending') && (
          <div className={cx('mt-1 flex flex-wrap items-center gap-1', anyOver && 'rounded')}>
            {i.status === 'pending' && <span className={cx('chip', late ? 'bg-rose-500 text-white' : 'bg-amber-500/20 text-amber-300')}>NO UNITS {late ? '· LATE' : ''}</span>}
            {live.map((a) => {
              const u = view.units.find((x) => x.id === a.unit)
              const over = u ? overdue(view, u, now) : 0
              return (
                <span key={a.unit} className={cx('inline-flex items-center gap-0.5', over > 0 && 'overdue rounded')}>
                  <span className="font-mono text-[11px] font-bold text-slate-200">{a.unit}</span>
                  <StatusChip status={u?.status ?? a.status} />
                  {u?.emergency && <span className="chip flash text-white">EMER</span>}
                </span>
              )
            })}
          </div>
        )}
        {i.status === 'closed' && <div className="mt-0.5 text-[10px] text-slate-500">Closed {ago(i.closed, now)} ago · {i.disposition}</div>}
      </button>
    </li>
  )
}

export default function Queue({ view, onCreateFromCall, onNew }: { view: View; onCreateFromCall: (c: Call) => void; onNew: () => void }) {
  const now = useNow()
  const me = useStore((s) => s.me)
  const calls = view.calls
    .filter((c) => !c.incident && (c.state !== 'ended' || now - ms(c.end) < 10 * 60000))
    .sort((a, b) => (a.state === 'ringing' ? 0 : 1) - (b.state === 'ringing' ? 0 : 1) || ms(a.start) - ms(b.start))
  const open = view.incidents
    .filter((i) => i.status !== 'closed')
    .sort((a, b) => (a.status === 'pending' ? 0 : 1) - (b.status === 'pending' ? 0 : 1) || a.priority - b.priority || ms(a.created) - ms(b.created))
  const closed = view.incidents.filter((i) => i.status === 'closed').slice(0, 8)
  const avail = view.units.filter((u) => u.status === 'available').length
  return (
    <aside className="panel flex min-h-0 flex-1 flex-col" aria-label="Calls and incidents">
      <div className="panel-head">
        <Phone size={12} aria-hidden /> 911 calls
        <span className="font-mono text-slate-300">{calls.filter((c) => c.state !== 'ended').length}</span>
        <span className="ml-auto normal-case tracking-normal text-slate-500">{view.health.bitvoice.connected ? 'from BitVoice' : view.health.ali.listening ? 'from ALI' : 'no call feed'}</span>
      </div>
      <ul className="max-h-[42%] shrink-0 overflow-y-auto divide-y divide-slate-800/70" aria-label="911 calls">
        {calls.length === 0 ? (
          <li className="px-3 py-2 text-[11px] text-slate-500">
            No 911 call waiting.{' '}
            {view.health.bitvoice.configured
              ? view.health.bitvoice.connected
                ? 'Watching BitVoice: ringing calls appear here before they are answered.'
                : 'BitVoice is configured but not connected.'
              : 'Connect call handling in Admin › Integrations.'}
          </li>
        ) : (
          calls.map((c) => <CallCard key={c.id} c={c} view={view} now={now} onCreate={onCreateFromCall} />)
        )}
      </ul>
      <div className="panel-head border-t">
        Incidents <span className="font-mono text-slate-300">{open.length}</span>
        {can.dispatch(me) && (
          <button className="btn ml-auto h-5 px-1.5 py-0 text-[10px] normal-case tracking-normal" onClick={onNew}>
            <Plus size={11} aria-hidden /> New <span className="kbd h-4">F2</span>
          </button>
        )}
      </div>
      <ul className="min-h-0 flex-1 overflow-y-auto divide-y divide-slate-800/70" aria-label="Open incidents">
        {open.length === 0 && (
          <li className="px-3 py-2 text-[11px] text-slate-500">
            No open incident. {avail} unit{avail === 1 ? '' : 's'} available. Create one with F2 or <span className="font-mono">NEW type address</span>.
          </li>
        )}
        {open.map((i) => (
          <IncidentRow key={i.number} i={i} view={view} now={now} />
        ))}
        {closed.length > 0 && <li className="bg-slate-950/50 px-2.5 py-1 text-[10px] font-bold uppercase tracking-wider text-slate-500">Recently closed</li>}
        {closed.map((i) => (
          <IncidentRow key={i.number} i={i} view={view} now={now} />
        ))}
      </ul>
    </aside>
  )
}
