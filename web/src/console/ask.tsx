import { useEffect, useState } from 'react'
import { AlertTriangle, HelpCircle } from 'lucide-react'
import { toast } from 'sonner'
import { api, ApiError, errText } from '../api'
import { Modal } from '../ui'

// A question asked before sending units: why other units than recommended, or confirm the recommendation's
// warnings (an unverified or uncertain location). One host on the page answers every caller.

type Q = { kind: 'reason' | 'confirm' | 'override'; text: string; warnings?: string[]; resolve: (v: string | null) => void }
let push: ((q: Q) => void) | null = null

function ask(q: Omit<Q, 'resolve'>): Promise<string | null> {
  return new Promise((resolve) => (push ? push({ ...q, resolve }) : resolve(null)))
}

const REASONS = ['Closer by radio or AVL', 'Crew asked for it', 'Supervisor decision', 'Recommended unit has a problem', 'Special skill or equipment']

/** dispatchUnits sends units, asking why when they are not the recommendation and asking to confirm the
 * recommendation's warnings, as the server requires. */
export async function dispatchUnits(number: string, units: string[], recommended: string[]): Promise<boolean> {
  let reason = ''
  let confirm = false
  let override = ''
  for (let i = 0; i < 4; i++) {
    try {
      await api.post(`/incidents/${number}/dispatch`, { units, recommended, reason, confirm, override })
      toast.success(`Dispatched ${units.join(' ')} to ${number}`)
      return true
    } catch (e) {
      if (e instanceof ApiError && e.field === 'reason') {
        const r = await ask({ kind: 'reason', text: e.message })
        if (!r) return false
        reason = r
        continue
      }
      if (e instanceof ApiError && e.field === 'override') {
        // A short-staffed unit: the agency wants a supervisor's override and reason.
        const r = await ask({ kind: 'override', text: e.message })
        if (!r) return false
        override = r
        continue
      }
      if (e instanceof ApiError && e.field === 'confirm') {
        const r = await ask({ kind: 'confirm', text: 'Check before sending', warnings: (e.data?.warnings as string[]) ?? [e.message] })
        if (!r) return false
        confirm = true
        continue
      }
      toast.error(errText(e))
      return false
    }
  }
  return false
}

export function AskHost() {
  const [q, setQ] = useState<Q | null>(null)
  const [text, setText] = useState('')
  useEffect(() => {
    push = (x) => { setText(''); setQ(x) }
    return () => { push = null }
  }, [])
  const done = (v: string | null) => {
    q?.resolve(v)
    setQ(null)
  }
  if (!q) return null
  return (
    <Modal open onClose={() => done(null)} title={q.kind === 'reason' ? 'Why these units?' : q.kind === 'override' ? 'Send short-staffed?' : 'Confirm before sending'}
      icon={q.kind === 'reason' ? <HelpCircle size={16} className="text-sky-300" aria-hidden /> : <AlertTriangle size={16} className="text-amber-300" aria-hidden />}
      footer={<>
        <button className="btn" onClick={() => done(null)}>Cancel</button>
        <button className="btn btn-go" disabled={q.kind !== 'confirm' && !text.trim()} onClick={() => done(q.kind !== 'confirm' ? text.trim() : 'yes')}>{q.kind === 'reason' ? 'Send with this reason' : q.kind === 'override' ? 'Override and send' : 'Confirmed: send'}</button>
      </>}>
      {q.kind === 'override' ? (
        <div className="space-y-2">
          <p className="text-sm text-amber-100">{q.text}</p>
          <p className="text-xs text-slate-400">Only a supervisor can override; the reason is kept with the dispatch.</p>
          <input className="input" autoFocus placeholder="Why send it short-staffed" value={text} onChange={(e) => setText(e.target.value)} aria-label="Override reason" />
        </div>
      ) : q.kind === 'reason' ? (
        <div className="space-y-2">
          <p className="text-sm text-slate-300">{q.text}. The reason is kept with the dispatch in the ledger.</p>
          <div className="flex flex-wrap gap-1">{REASONS.map((r) => <button key={r} className="btn h-7 text-xs" onClick={() => done(r)}>{r}</button>)}</div>
          <input className="input" autoFocus placeholder="Or say why" value={text} onChange={(e) => setText(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && text.trim() && done(text.trim())} aria-label="Reason" />
        </div>
      ) : (
        <ul className="space-y-1 text-sm text-amber-100">{(q.warnings ?? []).map((w) => <li key={w} className="flex gap-1.5"><AlertTriangle size={14} className="mt-0.5 shrink-0 text-amber-300" aria-hidden /> {w}</li>)}</ul>
      )}
    </Modal>
  )
}
