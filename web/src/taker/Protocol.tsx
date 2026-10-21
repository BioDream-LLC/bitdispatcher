import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ListChecks } from 'lucide-react'
import { api, errText } from '../api'
import { cx } from '../util'
import type { Incident, Protocol } from '../types'

/** ProtocolPanel works the agency's call-taking protocol for the call type: one question at a time, the
 * answers on number keys 1–9 while the panel has focus, and at the end the determinant (its code, the
 * priority it sets, what to tell the caller). Each answer is on the incident's timeline. A determinant
 * from protocol software (ProQA and others) shows here too. */
export default function ProtocolPanel({ inc, type }: { inc?: Incident; type: string }) {
  const [list, setList] = useState<Protocol[]>([])
  const [pick, setPick] = useState('')
  useEffect(() => {
    if (!type) return setList([])
    api.get<Protocol[]>(`/protocols?type=${encodeURIComponent(type)}`).then((l) => { setList(l); setPick((p) => p || l[0]?.id || '') }).catch(() => setList([]))
  }, [type])
  const run = inc?.protocol
  const p = list.find((x) => x.id === (run?.protocol || pick))
  if (!list.length && !run) return null
  const current = run && !run.determinant ? run.current : run ? '' : p?.start
  const q = p?.questions.find((x) => x.id === current)
  const answer = async (label: string) => {
    if (!inc || !p || !q) return
    try {
      await api.post(`/incidents/${inc.number}/protocol`, { protocol: p.id, question: q.id, answer: label })
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section className="panel rounded-md" aria-label="Protocol" tabIndex={0}
      onKeyDown={(e) => { const n = Number(e.key); if (q && n >= 1 && n <= q.answers.length) { e.preventDefault(); answer(q.answers[n - 1].label) } }}>
      <div className="panel-head">
        <ListChecks size={12} aria-hidden /> Protocol
        {!run && list.length > 1 && (
          <select className="input ml-2 h-6 w-auto py-0 text-[11px] normal-case tracking-normal" value={pick} onChange={(e) => setPick(e.target.value)} aria-label="Protocol">
            {list.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
          </select>
        )}
        <span className="ml-auto normal-case tracking-normal text-slate-500">{run?.system ? `from ${run.system}` : p ? `${p.name} · v${p.version}` : ''}</span>
      </div>
      <div className="space-y-2 p-2 text-sm">
        {p?.example && <p className="rounded bg-amber-500/15 px-2 py-1 text-[11px] text-amber-200">An example protocol, not for real calls: an agency writes its own under its medical director.</p>}
        {(run?.answers ?? []).map((a, i) => (
          <div key={i} className="flex gap-2 text-xs"><span className="text-slate-400">{a.text}</span><span className="font-semibold text-slate-100">{a.answer}</span></div>
        ))}
        {q && !inc && <p className="text-xs text-slate-500">Submit the call (F4) to start the protocol.</p>}
        {q && inc && (
          <div>
            <div className="text-base font-semibold text-slate-50">{q.text}</div>
            <div className="mt-1.5 flex flex-wrap gap-1.5">
              {q.answers.map((a, i) => (
                <button key={a.label} type="button" className="btn h-9 px-3 text-sm" onClick={() => answer(a.label)}>
                  <span className="kbd mr-1">{i + 1}</span>{a.label}
                </button>
              ))}
            </div>
          </div>
        )}
        {run?.determinant && (
          <div className={cx('rounded-md border p-2', (run.determinant.priority ?? 9) <= 1 ? 'border-rose-600 bg-rose-950/30' : 'border-sky-800 bg-sky-950/20')}>
            <div className="font-mono text-lg font-black text-slate-50">{run.determinant.code} <span className="font-sans text-sm font-semibold text-slate-200">{run.determinant.text}</span></div>
            {run.determinant.priority ? <div className="text-xs text-slate-300">Priority set to P{run.determinant.priority}</div> : null}
            {(run.determinant.instructions ?? []).length > 0 && (
              <ol className="mt-1 list-decimal space-y-0.5 pl-5 text-sm text-slate-100">{run.determinant.instructions!.map((x, i) => <li key={i}>{x}</li>)}</ol>
            )}
          </div>
        )}
      </div>
    </section>
  )
}
