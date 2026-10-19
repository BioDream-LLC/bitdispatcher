import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Check, ChevronDown, ChevronRight, HelpCircle, Lightbulb, X } from 'lucide-react'
import { api, errText } from '../api'
import { can, store, useStore } from '../store'
import { cx } from '../util'
import type { AdvisoryStatus, Suggestion } from '../types'

type List = { status: AdvisoryStatus; suggestions: Suggestion[] }

// One fetch for every panel that shows suggestions, again whenever the CAD changes.
let cache: List | null = null
const subs = new Set<(l: List) => void>()
let timer = 0
function refresh() {
  clearTimeout(timer)
  timer = window.setTimeout(() => api.get<List>('/advisory').then((l) => { cache = l; subs.forEach((f) => f(l)) }).catch(() => {}), 200)
}

export function useAdvisory(seq: number): List | null {
  const [l, setL] = useState<List | null>(cache)
  useEffect(() => {
    subs.add(setL)
    return () => { subs.delete(setL) }
  }, [])
  useEffect(refresh, [seq])
  useEffect(() => {
    const t = setInterval(refresh, 10000)
    return () => clearInterval(t)
  }, [])
  return l
}

const SRC: Record<string, string> = { unit: 'bg-emerald-500/15 text-emerald-300', caller: 'bg-sky-500/15 text-sky-300', dispatcher: 'bg-slate-700 text-slate-200', machine: 'bg-violet-500/15 text-violet-300' }
const REASONS = ['Not the same', 'Already handled', 'Wrong', 'Not now']

/** Suggestions from the advisory panel: why, and accept or reject in one click. It never acts by itself. */
export function AdvisoryCards({ list, incident, status }: { list: Suggestion[]; incident?: string; status: AdvisoryStatus }) {
  const me = useStore((s) => s.me)
  const [open, setOpen] = useState<string | null>(null)
  const items = incident ? list.filter((s) => s.incident === incident || s.other === incident) : list
  if (!items.length) return null
  const decide = async (s: Suggestion, accept: boolean, reason = '') => {
    try {
      const r = await api.post<{ result?: { ok: boolean; message: string; incident?: string } }>(`/advisory/${s.id}/decide`, { accept, run: accept && !!s.action, reason })
      if (r.result?.message) toast.success(r.result.message)
      else toast.success(accept ? 'Noted' : 'Rejected: it will not be shown again')
      refresh()
    } catch (e) {
      toast.error(errText(e))
      refresh()
    }
  }
  return (
    <section className={cx('rounded-md border p-1.5', status.mode === 'shadow' ? 'border-dashed border-violet-700 bg-violet-950/20' : 'border-amber-700/60 bg-amber-950/15')} aria-label="Advisory">
      <h3 className="mb-1 flex items-center gap-1 px-0.5 text-[11px] font-bold uppercase tracking-wider text-amber-300">
        <Lightbulb size={12} aria-hidden /> Advisory <span className="font-mono text-slate-300">{items.length}</span>
        {status.mode === 'shadow' && <span className="ml-1 rounded bg-violet-600/30 px-1 text-[9px] text-violet-200" title="Dispatchers do not see this yet">SHADOW · supervisors only</span>}
        <span className="ml-auto text-[9px] font-normal normal-case tracking-normal text-slate-500">advises only · codebook v{status.version}</span>
      </h3>
      <ul className="space-y-1">
        {items.map((s) => (
          <li key={s.id} className={cx('rounded border px-1.5 py-1 text-xs', s.severity === 'urgent' ? 'border-amber-600/60 bg-amber-500/10' : 'border-slate-700 bg-slate-900/60')}>
            <div className="flex items-start gap-1">
              <button className="mt-0.5 text-slate-400" onClick={() => setOpen(open === s.id ? null : s.id)} aria-expanded={open === s.id} aria-label="Why">
                {open === s.id ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
              </button>
              <div className="min-w-0 flex-1">
                <div className="font-semibold text-slate-100">
                  {s.verdict === 'cannot-determine' && <HelpCircle size={11} className="mr-0.5 inline text-sky-300" aria-label="Cannot determine" />}
                  {s.title}
                </div>
                <div className="text-slate-400">{s.detail}</div>
              </div>
              {!incident && s.incident && <button className="font-mono text-[10px] text-sky-300 underline" onClick={() => store.select(s.incident!)}>{s.incident}</button>}
            </div>
            {open === s.id && (
              <div className="ml-4 mt-1 space-y-1 border-l border-slate-700 pl-2 text-[11px]">
                <div className="text-slate-300">{s.explain}</div>
                <div className="text-[10px] uppercase tracking-wider text-slate-500">Relied on</div>
                {s.relied.map((e, i) => <div key={i} className="flex gap-1"><span className={cx('rounded px-1 text-[9px] uppercase', SRC[e.source] ?? 'bg-slate-700')}>{e.source}</span><span className="text-slate-300">{e.text}</span></div>)}
                {s.assumedAbsent.length > 0 && <><div className="text-[10px] uppercase tracking-wider text-slate-500">Assumed absent</div>{s.assumedAbsent.map((a, i) => <div key={i} className="text-slate-400">{a}</div>)}</>}
                <div className="text-[10px] text-slate-500">Rule {s.ruleName} ({s.rule})</div>
              </div>
            )}
            {can.dispatch(me) && (
              <div className="mt-1 flex flex-wrap items-center gap-1">
                <button className="btn h-6 px-1.5 py-0 text-[11px]" onClick={() => decide(s, true)}><Check size={11} aria-hidden /> {s.action ? s.action.label : 'Noted'}</button>
                {REASONS.map((r) => <button key={r} className="rounded px-1 text-[10px] text-slate-400 hover:bg-slate-800 hover:text-slate-100" onClick={() => decide(s, false, r)}><X size={9} className="inline" aria-hidden /> {r}</button>)}
              </div>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}
