import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Award } from 'lucide-react'
import { api, errText } from '../api'
import { Modal } from '../ui'
import { cx, hhmm } from '../util'
import { Checks } from '../console/Dream'
import type { Review } from '../types'
import { Page, Table } from './Admin'

type Quality = { from: string; to: string; reviews: Review[]; takers: { taker: string; incidents: number; average: number }[]; checks: Record<string, [number, number]> }

const day = (d: Date) => d.toISOString().slice(0, 10)

/** Quality is the quality review: every incident checked against the agency's standards. */
export default function Quality() {
  const [from, setFrom] = useState(day(new Date(Date.now() - 86400000)))
  const [to, setTo] = useState(day(new Date(Date.now() + 86400000)))
  const [q, setQ] = useState<Quality | null>(null)
  const [open, setOpen] = useState<Review | null>(null)
  useEffect(() => {
    api.get<Quality>(`/reports/quality?from=${new Date(from).toISOString()}&to=${new Date(to).toISOString()}`).then(setQ).catch((e) => toast.error(errText(e)))
  }, [from, to])
  const tone = (n: number) => (n >= 90 ? 'text-emerald-300' : n >= 70 ? 'text-amber-300' : 'text-rose-300')
  return (
    <Page title="Quality review" intro="Every incident checked against the centre's standards: answer, entry and dispatch times, the location placed on the map, the callback, the disposition, and the questions the call-taker was heard to ask (from the transcript). Set the standards and the questions in Settings.">
      <div className="mb-3 flex items-end gap-2 text-sm">
        <label className="flex flex-col gap-1"><span className="label">From</span><input type="date" className="input" value={from} onChange={(e) => setFrom(e.target.value)} /></label>
        <label className="flex flex-col gap-1"><span className="label">To</span><input type="date" className="input" value={to} onChange={(e) => setTo(e.target.value)} /></label>
        {q && <span className="ml-auto text-slate-400">{q.reviews.length} incident{q.reviews.length === 1 ? '' : 's'}</span>}
      </div>
      {q && (
        <div className="grid gap-4 lg:grid-cols-[1fr_20rem]">
          <Table head={['Incident', 'Type', 'Taker', 'Score', 'Missed']} empty={q.reviews.length === 0 ? 'No incident in this period.' : undefined}>
            {q.reviews.map((r) => (
              <tr key={r.incident} className="cursor-pointer border-t border-slate-800 hover:bg-slate-800/50" onClick={() => setOpen(r)}>
                <td className="px-3 py-1.5 font-mono">{r.incident} <span className="text-xs text-slate-500">{hhmm(r.created)}</span></td>
                <td className="px-3 py-1.5">{r.type} <span className="text-xs text-slate-500">P{r.priority}</span>{r.critical && <span className="chip ml-1 bg-teal-500/20 text-teal-200">critical</span>}</td>
                <td className="px-3 py-1.5">{r.taker ?? '—'}</td>
                <td className={cx('px-3 py-1.5 font-mono font-bold', tone(r.score))}>{r.score}%</td>
                <td className="px-3 py-1.5 text-xs text-slate-400">{r.checks.filter((c) => !c.ok && !c.skip).map((c) => c.name).join(', ') || '—'}</td>
              </tr>
            ))}
          </Table>
          <div className="space-y-4">
            <section className="panel rounded-lg p-3">
              <h2 className="mb-2 text-sm font-semibold text-slate-200">By call-taker</h2>
              <ul className="space-y-1 text-sm">{q.takers.map((t) => <li key={t.taker} className="flex"><span>{t.taker}</span><span className="ml-auto text-xs text-slate-500">{t.incidents} incident{t.incidents === 1 ? "" : "s"}</span><b className={cx('ml-1 font-mono', tone(t.average))}>{t.average}%</b></li>)}</ul>
            </section>
            <section className="panel rounded-lg p-3">
              <h2 className="mb-2 text-sm font-semibold text-slate-200">By check</h2>
              <ul className="space-y-1 text-sm">{Object.entries(q.checks).sort().map(([k, [ok, n]]) => <li key={k} className="flex gap-2"><span className="truncate">{k}</span><span className={cx('ml-auto font-mono', tone((ok * 100) / n))}>{ok}/{n}</span></li>)}</ul>
            </section>
          </div>
        </div>
      )}
      <Modal open={!!open} onClose={() => setOpen(null)} title={open ? `${open.incident} · ${open.type}: ${open.score}%` : ''} icon={<Award size={16} className="text-amber-300" aria-hidden />} wide>
        {open && <Checks checks={open.checks} />}
        <p className="mt-3 text-xs text-slate-500">A review is a coaching aid: it reads the record, and a missed question may simply not be on the transcript.</p>
      </Modal>
    </Page>
  )
}
