import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { CheckCircle2, Circle, CircleAlert, CircleHelp, Undo2 } from 'lucide-react'
import { api, errText } from '../api'
import { cx } from '../util'
import type { SetupStep, SetupView } from '../types'
import { Page } from './Admin'

const NEED: Record<SetupStep['need'], string> = { required: 'Required', recommended: 'Recommended', optional: 'Optional' }

/** The setup checklist: everything an agency does before its first live call, in order. bitdispatcher
 * checks most steps itself; training, backups and the parallel run only a person can confirm. */
export default function Setup() {
  const [v, setV] = useState<SetupView | null>(null)
  const [note, setNote] = useState<Record<string, string>>({})
  const [asking, setAsking] = useState<string | null>(null)
  const load = () => api.get<SetupView>('/admin/setup').then(setV).catch((e) => toast.error(errText(e)))
  useEffect(() => {
    load()
  }, [])
  if (!v) return null
  const tick = async (id: string, done: boolean) => {
    try {
      setV(await api.put<SetupView>('/admin/setup/' + id, { done, note: note[id] ?? '' }))
      setAsking(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const pct = Math.round((100 * v.done) / Math.max(1, v.steps.length))
  return (
    <Page title="Setup checklist" intro="Everything to do before the first live call, in order. bitdispatcher checks most steps itself as you work; training, backups and running beside the old system only you can confirm. Every step links to the page that does it and to its chapter of the manual.">
      <section className="panel mb-4 rounded-lg p-4 text-sm" aria-label="Progress">
        <div className="flex items-center gap-3">
          <div className="h-2 flex-1 overflow-hidden rounded bg-slate-800" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100} aria-label="Steps done">
            <div className="h-full bg-emerald-500" style={{ width: pct + '%' }} />
          </div>
          <span className="text-slate-300">{v.done} of {v.steps.length} done</span>
        </div>
        <p className="mt-2 text-xs text-slate-400" data-testid="setup-left">
          {v.left === 0 ? 'Every required step is done.' : `${v.left} required ${v.left === 1 ? 'step is' : 'steps are'} not done yet.`} When they are, run the <Link className="text-sky-300 underline" to="/admin/golive">go-live check</Link>.
        </p>
      </section>
      <ol className="space-y-2">
        {v.steps.map((x, i) => (
          <li key={x.id} className={cx('panel rounded-lg p-3 text-sm', x.status === 'todo' && x.need === 'required' && 'border-l-2 border-amber-500/70')} data-testid={'step-' + x.id}>
            <div className="flex items-start gap-3">
              <span className="mt-0.5" aria-label={x.status === 'done' ? 'Done' : x.status === 'warn' ? 'Done, with something to look at' : 'Not done'}>
                {x.status === 'done' ? <CheckCircle2 size={16} className="text-emerald-400" aria-hidden /> : x.status === 'warn' ? <CircleAlert size={16} className="text-amber-400" aria-hidden /> : <Circle size={16} className="text-slate-500" aria-hidden />}
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-slate-500">{i + 1}.</span>
                  <span className="font-semibold text-slate-100">{x.title}</span>
                  <span className={cx('rounded px-1.5 text-[10px] uppercase tracking-wider', x.need === 'required' ? 'bg-amber-500/15 text-amber-300' : 'bg-slate-800 text-slate-400')}>{NEED[x.need]}</span>
                </div>
                <p className="mt-0.5 text-xs text-slate-400">{x.why}</p>
                {x.detail && <p className="mt-1 text-xs text-slate-300">{x.detail}</p>}
                {x.tick && <p className="mt-1 text-xs text-emerald-300">{x.manual ? 'Confirmed' : 'Not needed'} by {x.tick.by}{x.tick.at ? ' on ' + new Date(x.tick.at).toLocaleDateString() : ''}{x.tick.note ? ': ' + x.tick.note : ''}</p>}
                {asking === x.id && (
                  <div className="mt-2 flex gap-2">
                    <input className="input h-7 flex-1 text-xs" autoFocus value={note[x.id] ?? ''} onChange={(e) => setNote({ ...note, [x.id]: e.target.value })}
                      placeholder={x.manual ? 'A note (optional): who was trained, where the copies go…' : 'Why it is not needed here'} aria-label="Note" />
                    <button className="btn btn-go h-7 py-0 text-xs" onClick={() => tick(x.id, true)}>{x.manual ? 'Confirm' : 'Not needed'}</button>
                    <button className="btn h-7 py-0 text-xs" onClick={() => setAsking(null)}>Cancel</button>
                  </div>
                )}
              </div>
              <div className="flex shrink-0 items-center gap-1.5">
                {x.page && <Link className="btn h-7 py-0 text-xs" to={'/admin/' + x.page}>Open</Link>}
                {x.chapter && <Link className="btn h-7 px-2 py-0 text-xs" to={'/help?a=' + x.chapter} aria-label={'Manual: ' + x.title} title="The manual"><CircleHelp size={12} className="text-sky-300" aria-hidden /></Link>}
                {x.tick ? (
                  <button className="btn h-7 py-0 text-xs" onClick={() => tick(x.id, false)} aria-label={'Undo: ' + x.title}><Undo2 size={12} aria-hidden /> Undo</button>
                ) : x.status === 'todo' && (x.manual || x.need !== 'required') && asking !== x.id && (
                  <button className="btn h-7 py-0 text-xs" onClick={() => setAsking(x.id)}>{x.manual ? 'Mark done' : 'Not needed'}</button>
                )}
              </div>
            </div>
          </li>
        ))}
      </ol>
    </Page>
  )
}
