import { History as HistoryIcon, ShieldAlert } from 'lucide-react'
import type { History, Premise } from '../types'
import { cx } from '../util'

/** What is known about the place: hazards to the crews first, in red; then notes, contacts and earlier calls. */
export default function PremiseBox({ premises = [], prior = [], compact }: { premises?: Premise[]; prior?: History[]; compact?: boolean }) {
  if (premises.length === 0 && prior.length === 0) return null
  const today = new Date().toISOString().slice(0, 10)
  return (
    <section className="space-y-1" aria-label="Premise information">
      {premises.map((p) => {
        const expired = !!p.expires && p.expires < today
        return (
          <div key={p.id} className={cx('rounded border px-2 py-1 text-xs', (p.hazards?.length ?? 0) > 0 && !expired ? 'border-rose-600 bg-rose-950/40' : 'border-slate-700 bg-slate-900/60')}>
            <div className="flex items-center gap-1 font-semibold text-slate-100">
              <ShieldAlert size={12} className={(p.hazards?.length ?? 0) > 0 ? 'text-rose-300' : 'text-slate-400'} aria-hidden />
              {p.name || p.address}{p.name && <span className="font-normal text-slate-400">· {p.address}</span>}
              {expired && <span className="chip bg-slate-700 normal-case text-slate-300">expired {p.expires}</span>}
            </div>
            {(p.hazards?.length ?? 0) > 0 && (
              <div className="mt-0.5 flex flex-wrap gap-1">{p.hazards!.map((h) => <span key={h} className={cx('rounded px-1 font-semibold', compact ? 'text-sm' : '', 'bg-rose-600 text-white')}>{h}</span>)}</div>
            )}
            {p.notes && <div className="mt-0.5 whitespace-pre-wrap text-slate-300">{p.notes}</div>}
            {(p.contacts?.length ?? 0) > 0 && <div className="mt-0.5 text-slate-400">{p.contacts!.join(' · ')}</div>}
          </div>
        )
      })}
      {prior.length > 0 && (
        <div className="rounded border border-slate-800 px-2 py-1 text-[11px] text-slate-400">
          <div className="flex items-center gap-1 font-semibold text-slate-300"><HistoryIcon size={11} aria-hidden /> Earlier here (previous system)</div>
          {prior.map((h) => (
            <div key={h.number + h.at} className="flex gap-2"><span className="font-mono">{new Date(h.at).toLocaleDateString()}</span><span className="text-slate-200">{h.type}</span><span className="truncate">{h.disposition}</span></div>
          ))}
        </div>
      )}
    </section>
  )
}
