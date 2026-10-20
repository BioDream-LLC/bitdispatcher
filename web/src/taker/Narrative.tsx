import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { CheckCircle2, History as HistoryIcon, MessageSquare, PhoneOutgoing, Radio } from 'lucide-react'
import { api, errText } from '../api'
import { cx, hhmm, statusMeta } from '../util'
import type { Contact, Incident, Recommendation, View } from '../types'
import type { Draft } from './draft'

const KINDS: { kind: Contact['kind']; label: string; who: string; detail: string }[] = [
  { kind: 'mutualaid', label: 'Mutual aid', who: 'Agency asked', detail: 'What was asked for, and the answer' },
  { kind: 'resource', label: 'Resource request', who: 'Asked of (tow, utility, Red Cross…)', detail: 'What, and when they expect to arrive' },
  { kind: 'transfer', label: 'Transfer', who: 'To (agency or number)', detail: 'Warm or cold, why' },
  { kind: 'referral', label: 'Referral', who: 'Referred to', detail: 'Why, what the caller was told' },
  { kind: 'outbound', label: 'Outbound call', who: 'Called (name or number)', detail: 'Why, and what was said' },
  { kind: 'instructions', label: 'Instructions given', who: 'Which (CPR, bleeding control, stay inside…)', detail: 'What the caller did' },
  { kind: 'notify', label: 'Notification', who: 'Who was notified', detail: 'How' },
]

/** Comments: every note and change in order, with who and when; new notes go in, nothing is overwritten.
 * Transfers, referrals, outbound calls and instructions given are logged here too. */
export function CommentsTab({ d, inc, update }: { view: View; d: Draft; inc?: Incident; update: (p: Partial<Draft> | ((d: Draft) => Draft)) => void }) {
  const [kind, setKind] = useState<Contact['kind'] | ''>('')
  const [who, setWho] = useState('')
  const [detail, setDetail] = useState('')
  const lines = inc
    ? inc.timeline.filter((t) => ['note', 'priority-note', 'change', 'created', 'notice', 'person', 'vehicle', 'location'].includes(t.kind))
    : d.notes.map((n, i) => ({ seq: i, at: n.at, actor: 'you', kind: 'note', text: n.text + ' (not yet submitted)' }))
  const contacts = inc?.notices ?? d.contacts
  const log = async () => {
    if (!kind || !who.trim()) return
    const c = { id: '', kind, who: who.trim(), detail: detail.trim(), status: kind === 'notify' ? 'pending' : 'done', at: new Date().toISOString(), by: 'you' } as Contact
    try {
      if (inc) await api.post(`/incidents/${inc.number}/contacts`, c)
      else update((cur) => ({ ...cur, contacts: [...cur.contacts, c] }))
      setKind('')
      setWho('')
      setDetail('')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const done = (c: Contact) => inc && api.put(`/incidents/${inc.number}/contacts/${c.id}`, { status: 'done' }).catch((e) => toast.error(errText(e)))
  const k = KINDS.find((x) => x.kind === kind)
  return (
    <div className="space-y-3">
      <h2 className="flex items-center gap-1.5 text-sm font-semibold text-slate-100"><MessageSquare size={14} aria-hidden /> Comments <span className="text-xs font-normal text-slate-500">the narrative field is always on the right (F8)</span></h2>
      <ul className="space-y-0.5 text-xs">
        {lines.length === 0 && <li className="text-slate-500">Nothing written yet.</li>}
        {lines.map((t) => (
          <li key={t.seq + t.at} className={cx('grid grid-cols-[4.5rem_6rem_1fr] gap-2', t.kind === 'priority-note' && 'font-semibold text-amber-200', t.kind !== 'note' && t.kind !== 'priority-note' && 'text-slate-400')}>
            <span className="font-mono text-[10px] text-slate-500">{new Date(t.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}</span>
            <span className="truncate text-slate-500">{t.actor}</span>
            <span className="text-slate-200">{t.text}</span>
          </li>
        ))}
      </ul>
      <section className="panel rounded-lg p-2">
        <h3 className="label flex items-center gap-1"><PhoneOutgoing size={11} aria-hidden /> Log a contact</h3>
        <div className="flex flex-wrap gap-1">
          {KINDS.map((x) => <button key={x.kind} type="button" aria-pressed={kind === x.kind} className={cx('btn h-6 px-2 text-[11px]', kind === x.kind && 'ring-1 ring-sky-400')} onClick={() => setKind(x.kind)}>{x.label}</button>)}
        </div>
        {k && (
          <form className="mt-1.5 grid grid-cols-[1fr_1fr_auto] gap-1.5" onSubmit={(e) => { e.preventDefault(); log() }}>
            <input className="input h-7 text-xs" aria-label={k.who} placeholder={k.who} value={who} onChange={(e) => setWho(e.target.value)} autoFocus />
            <input className="input h-7 text-xs" aria-label={k.detail} placeholder={k.detail} value={detail} onChange={(e) => setDetail(e.target.value)} />
            <button className="btn btn-go h-7 text-xs" disabled={!who.trim()}>Log it</button>
          </form>
        )}
        {contacts.length > 0 && (
          <ul className="mt-2 space-y-0.5 text-xs">
            {contacts.map((c) => (
              <li key={c.id + c.at} className="flex items-center gap-1.5">
                <span className={cx('chip', c.status === 'done' ? 'bg-emerald-500/20 text-emerald-200' : c.status === 'cancelled' ? 'bg-slate-700 text-slate-400' : 'bg-amber-500/20 text-amber-200')}>{c.status}</span>
                <span className="text-slate-300">{KINDS.find((x) => x.kind === c.kind)?.label ?? c.kind}</span> <b className="text-slate-100">{c.who}</b>
                <span className="truncate text-slate-500">{c.detail}</span>
                {c.status === 'pending' && inc && <button className="btn btn-ghost ml-auto h-5 px-1 text-[10px]" onClick={() => done(c)}><CheckCircle2 size={11} aria-hidden /> done</button>}
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}

/** History: what dispatch did with it, without leaving the call: units, their times, radio, and the response
 * recommended (read-only for the call-taker). */
export function HistoryTab({ view, inc }: { view: View; inc?: Incident }) {
  const [rec, setRec] = useState<Recommendation | null>(null)
  const seq = view.seq
  useEffect(() => {
    if (!inc) return
    api.get<Recommendation>(`/incidents/${inc.number}/recommend`).then(setRec).catch(() => setRec(null))
  }, [inc?.number, seq])
  if (!inc) return <p className="text-xs text-slate-500">Submit the incident (F4) to follow its dispatch here.</p>
  const events = inc.timeline.filter((t) => ['dispatch', 'status', 'radio', 'drone', 'call', 'location', 'bolo', 'close', 'reopen', 'priority-note'].includes(t.kind))
  return (
    <div className="space-y-3">
      <h2 className="flex items-center gap-1.5 text-sm font-semibold text-slate-100"><HistoryIcon size={14} aria-hidden /> History</h2>
      <table className="w-full text-xs">
        <thead><tr className="text-left text-[10px] uppercase tracking-wider text-slate-500"><th className="py-1">Unit</th><th>Status</th><th>Sent</th><th>Ack</th><th>En route</th><th>On scene</th><th>Clear</th></tr></thead>
        <tbody className="divide-y divide-slate-800">
          {inc.units.length === 0 && <tr><td colSpan={7} className="py-1 text-slate-500">No unit sent yet.</td></tr>}
          {inc.units.map((a) => (
            <tr key={a.unit + a.dispatched}>
              <td className="py-1 font-mono font-bold text-slate-100">{a.unit}</td>
              <td><span className={cx('chip font-mono', statusMeta(a.status).bg, statusMeta(a.status).fg)}>{statusMeta(a.status).code}</span></td>
              <td className="font-mono">{hhmm(a.dispatched)}</td><td className="font-mono">{a.acked ? hhmm(a.acked) : '—'}</td>
              <td className="font-mono">{a.enroute ? hhmm(a.enroute) : '—'}</td><td className="font-mono">{a.onScene ? hhmm(a.onScene) : '—'}</td><td className="font-mono">{a.cleared ? hhmm(a.cleared) : '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {rec && (rec.units.length > 0 || rec.plan) && (
        <p className="text-xs text-slate-400">Recommended response: <span className="font-mono text-slate-200">{rec.units.join(' ') || '—'}</span>{rec.plan ? ` · plan ${rec.plan}` : ''}</p>
      )}
      <ul className="space-y-0.5 text-xs">
        {events.map((t) => (
          <li key={t.seq} className="grid grid-cols-[4.5rem_1fr] gap-2">
            <span className="font-mono text-[10px] text-slate-500">{new Date(t.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}</span>
            <span className={cx(t.kind === 'radio' ? 'text-teal-200' : 'text-slate-300')}>{t.kind === 'radio' && <Radio size={10} className="mr-1 inline" aria-hidden />}{t.text}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}
