import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Coffee, HeartHandshake, Users } from 'lucide-react'
import { api, errText } from '../api'
import { cx, useNow, ago } from '../util'
import { minutes } from '../dream'
import type { View, Workload } from '../types'

// The supervisor's view of the people: who has worked how long without a break, who has had critical calls,
// and the care list after them (a break offered, a debrief arranged).

const LEVEL: Record<string, string> = { ok: 'bg-emerald-500/15 text-emerald-300', watch: 'bg-amber-500/25 text-amber-200', relieve: 'bg-rose-500 text-white' }

export default function PeoplePanel({ view }: { view: View }) {
  const now = useNow()
  const [items, setItems] = useState<Workload[]>([])
  const [relief, setRelief] = useState<string[]>([])
  const minute = Math.floor(now / 30000)
  useEffect(() => {
    api.get<{ items: Workload[]; relief: string[] }>('/workload').then((r) => { setItems(r.items); setRelief(r.relief ?? []) }).catch(() => {})
  }, [minute, view.seq])
  const care = (view.care ?? []).filter((c) => c.status !== 'closed')
  const act = async (incident: string, action: string, user = '', note = '') => {
    try {
      await api.post(`/care/${incident}`, { action, user, note })
      toast.success(action === 'break' ? `Break offered to ${user}` : action === 'debrief' ? 'Debrief arranged' : 'Done')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section className="flex max-h-[48%] min-h-0 flex-col overflow-hidden rounded-xl border border-slate-800 bg-slate-900/70" aria-label="People">
      <div className="flex items-center gap-2 border-b border-slate-800 px-2.5 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-slate-400">
        <Users size={12} aria-hidden /> People
        <span className="ml-auto normal-case tracking-normal text-slate-500">{items.length} signed in{relief.length ? ` · free to relieve: ${relief.slice(0, 3).join(', ')}` : ''}</span>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {care.length > 0 && (
          <ul className="border-b border-slate-800 bg-teal-950/30" aria-label="Critical calls: care">
            {care.map((c) => (
              <li key={c.incident} className="px-2.5 py-1.5 text-[12px]">
                <div className="flex items-center gap-1.5">
                  <HeartHandshake size={13} className="text-teal-300" aria-hidden />
                  <span className="font-mono font-bold text-slate-100">{c.incident}</span>
                  <span className="truncate text-slate-300">{c.reason}</span>
                  <span className={cx('chip ml-auto', c.status === 'debrief' ? 'bg-sky-500/20 text-sky-200' : 'bg-teal-500/20 text-teal-200')}>{c.status === 'debrief' ? 'debrief arranged' : 'open'}</span>
                </div>
                <div className="mt-1 flex flex-wrap items-center gap-1">
                  {c.people.map((p) => (
                    <button key={p} className={cx('btn h-6 px-1.5 py-0 text-[11px]', c.offered?.includes(p) && 'border-teal-600 text-teal-200')} disabled={c.offered?.includes(p)}
                      onClick={() => act(c.incident, 'break', p)} title={c.offered?.includes(p) ? 'A break was offered' : `Offer ${p} a break`}>
                      <Coffee size={11} aria-hidden /> {p}{c.offered?.includes(p) ? ' ✓' : ''}
                    </button>
                  ))}
                  {(c.takers ?? []).filter((t) => !c.people.includes(t)).map((t) => <span key={t} className="chip bg-slate-800 text-slate-300" title="Call-taker on the phone system">{t}</span>)}
                  {c.status === 'open' && <button className="btn h-6 px-1.5 py-0 text-[11px]" onClick={() => { const n = prompt('When and where is the debrief?'); if (n !== null) act(c.incident, 'debrief', '', n) }}>Arrange debrief</button>}
                  <button className="btn btn-ghost h-6 px-1.5 py-0 text-[11px]" onClick={() => act(c.incident, 'close')}>Done</button>
                  <span className="ml-auto text-[10px] text-slate-500">{ago(c.at, now)} ago</span>
                </div>
              </li>
            ))}
          </ul>
        )}
        {items.length === 0 ? <p className="p-3 text-sm text-slate-500">No one signed in.</p> : (
          <table className="w-full text-[12px]">
            <thead className="text-[10px] uppercase tracking-wider text-slate-500">
              <tr><th className="px-2 py-1 text-left">Position</th><th className="px-1 text-right">Calls 1 h</th><th className="px-1 text-right">Actions</th><th className="px-1 text-right">Since break</th><th className="px-2 text-left">State</th></tr>
            </thead>
            <tbody>
              {items.map((w) => (
                <tr key={w.username} className="border-t border-slate-800/70 align-top">
                  <td className="px-2 py-1"><div className="font-semibold text-slate-100">{w.console || '—'} <span className="font-normal text-slate-400">{w.name || w.username}</span></div>
                    {w.critical.length > 0 && <div className="text-[10px] text-teal-300">critical: {w.critical.join(', ')}</div>}</td>
                  <td className="px-1 text-right font-mono tnum">{w.calls}</td>
                  <td className="px-1 text-right font-mono tnum">{w.actions}</td>
                  <td className="px-1 text-right font-mono tnum">{w.onBreak ? <span className="text-teal-300">on break</span> : minutes(w.sinceBreak)}</td>
                  <td className="px-2 py-1"><span className={cx('chip uppercase', LEVEL[w.level])}>{w.level}</span>
                    {w.reasons.length > 0 && <div className="mt-0.5 text-[10px] text-slate-400">{w.reasons.join('; ')}</div>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </section>
  )
}
