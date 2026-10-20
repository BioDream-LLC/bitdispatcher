import { Link } from 'react-router-dom'
import { AlertOctagon } from 'lucide-react'
import { store } from '../store'
import { overdue, typeName, useNow } from '../util'
import type { View } from '../types'

type Ex = { key: string; level: 'urgent' | 'watch'; text: string; incident?: string }

/** What needs a supervisor now, across the centre: emergencies, units that have not acknowledged, P1s
 * waiting, overdue status checks, notifications not made and newer caller locations not decided. */
export default function Exceptions({ view }: { view: View }) {
  const now = useNow()
  const ack = (view.settings.ackSeconds || 60) * 1000
  const out: Ex[] = []
  for (const u of view.units) if (u.emergency) out.push({ key: 'em' + u.id, level: 'urgent', text: `${u.id} emergency button`, incident: u.incident })
  for (const i of view.incidents) {
    if (i.status === 'closed') continue
    const name = `${i.number} ${typeName(view, i.type)}`
    if (i.status === 'pending' && i.priority <= 1 && now - Date.parse(i.created) > 60_000) out.push({ key: 'p' + i.number, level: 'urgent', text: `${name}: P1 with no unit for ${Math.round((now - Date.parse(i.created)) / 60000)} min`, incident: i.number })
    for (const a of i.units) {
      if (!a.cleared && a.status === 'dispatched' && !a.acked && now - Date.parse(a.dispatched) > ack) out.push({ key: 'a' + i.number + a.unit, level: 'urgent', text: `${a.unit} has not acknowledged ${i.number}`, incident: i.number })
    }
    const notify = (i.notices ?? []).filter((n) => n.status === 'pending' && now - Date.parse(n.at) > 5 * 60_000)
    if (notify.length) out.push({ key: 'n' + i.number, level: 'watch', text: `${name}: ${notify.map((n) => n.who).join(', ')} not yet notified`, incident: i.number })
    if (i.newLocation && now - Date.parse(i.newLocation.at) > 60_000) out.push({ key: 'l' + i.number, level: 'watch', text: `${name}: a newer caller location waits for a decision`, incident: i.number })
  }
  for (const u of view.units) {
    const by = overdue(view, u, now)
    if (by > 0) out.push({ key: 'o' + u.id, level: by > 5 * 60_000 ? 'urgent' : 'watch', text: `${u.id} status check overdue ${Math.round(by / 60000)} min`, incident: u.incident })
  }
  return (
    <section className="max-h-[38%] shrink-0 overflow-y-auto rounded-xl border border-slate-800 bg-slate-900/70" aria-label="Exceptions">
      <div className="flex items-center gap-1.5 px-2 py-1 text-[11px] font-bold uppercase tracking-wider text-slate-400">
        <AlertOctagon size={12} aria-hidden /> Exceptions <span className="font-mono text-slate-300">{out.length}</span>
        <span className="ml-auto normal-case tracking-normal text-slate-500">every action is recorded as you</span>
      </div>
      <ul className="divide-y divide-slate-800 text-xs">
        {out.length === 0 && <li className="px-2 py-1 text-slate-500">Nothing needs a supervisor.</li>}
        {out.map((x) => (
          <li key={x.key} className="flex items-center gap-1.5 px-2 py-1">
            <span className={x.level === 'urgent' ? 'h-2 w-2 shrink-0 rounded-full bg-rose-500' : 'h-2 w-2 shrink-0 rounded-full bg-amber-400'} aria-label={x.level} />
            <span className="text-slate-200">{x.text}</span>
            {x.incident && <Link to="/console" onClick={() => store.select(x.incident!)} className="ml-auto shrink-0 text-[10px] text-sky-300 hover:underline">open</Link>}
          </li>
        ))}
      </ul>
    </section>
  )
}
