import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { ArrowRightLeft, Check, Hand, Inbox, ListChecks, RefreshCw, Send, UserCheck } from 'lucide-react'
import { api, errText } from '../api'
import { Modal } from '../ui'
import { cx, hhmm } from '../util'
import { useNow } from '../util'
import { CATEGORY_LABELS, TYPE_LABELS, dueState, requestClosed, type Request, type RequestList } from './types'

type Me = { username?: string }

async function fetchMe(): Promise<Me> {
  try {
    const r = await api.get<{ signedIn: boolean; session?: { username: string } }>('/auth/me')
    return { username: r.session?.username }
  } catch {
    return {}
  }
}

/** Queue is the request table, used full-page and compact per-incident. It shows metadata only, never a
 * return's text. */
export function Queue({ incident, compact }: { incident?: string; compact?: boolean }) {
  const [data, setData] = useState<RequestList | null>(null)
  const [me, setMe] = useState<Me>({})
  const [blockers, setBlockers] = useState<Request[]>([])
  const now = useNow()

  // filters
  const [status, setStatus] = useState('open')
  const [mine, setMine] = useState(false)
  const [type, setType] = useState('')
  const [category, setCategory] = useState('')
  const [text, setText] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')

  const load = useCallback(async () => {
    const q = new URLSearchParams()
    if (status) q.set('status', status)
    if (mine) q.set('mine', '1')
    if (incident) q.set('incident', incident)
    if (type) q.set('type', type)
    if (category) q.set('category', category)
    if (text.trim()) q.set('q', text.trim())
    if (from) q.set('from', new Date(from).toISOString())
    if (to) q.set('to', new Date(to).toISOString())
    try {
      const r = await api.get<RequestList>(`/requests?${q}`)
      setData(r)
    } catch (e) {
      toast.error(errText(e))
    }
  }, [status, mine, incident, type, category, text, from, to])

  useEffect(() => {
    load()
  }, [load])
  useEffect(() => {
    fetchMe().then(setMe)
    api.get<{ items: Request[] }>('/me/open-requests').then((r) => setBlockers(r.items)).catch(() => setBlockers([]))
  }, [])

  const warnMinutes = data?.warnMinutes ?? 3
  const label = (s: string) => data?.labels[s] ?? s

  return (
    <div className="space-y-3">
      {!compact && (
        <div className="flex items-center gap-2">
          <a href="#/" className="btn btn-ghost h-6 px-1.5 text-xs">← Dispatch</a>
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-slate-100">
            <ListChecks size={14} aria-hidden /> NCIC and state requests
          </h2>
          <button className="btn btn-ghost ml-auto h-7 text-xs" onClick={load} aria-label="Refresh">
            <RefreshCw size={12} aria-hidden /> Refresh
          </button>
        </div>
      )}

      {!compact && (
        <div className={cx('rounded-md border px-3 py-2 text-xs', blockers.length ? 'border-amber-800 bg-amber-950/30 text-amber-100' : 'border-slate-800 bg-slate-900/50 text-slate-300')}>
          <div className="flex items-center gap-1.5 font-semibold">
            <Inbox size={12} aria-hidden /> My open requests: {blockers.length}
          </div>
          {blockers.length > 0 && (
            <p className="mt-0.5 text-[11px]">You cannot sign off while you own open requests. Finish them or hand them over: {blockers.map((b) => b.id).join(', ')}.</p>
          )}
        </div>
      )}

      <div className={cx('flex flex-wrap items-end gap-2', compact && 'text-xs')}>
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
          Status
          <select className="input h-7 text-xs" value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value="open">Open</option>
            <option value="closed">Closed</option>
            <option value="">All</option>
            {data?.statuses.map((s) => (
              <option key={s} value={s}>
                {label(s)}
              </option>
            ))}
          </select>
        </label>
        <label className="flex items-center gap-1.5 text-[11px] text-slate-400">
          <input type="checkbox" checked={mine} onChange={(e) => setMine(e.target.checked)} /> Mine
        </label>
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
          Type
          <select className="input h-7 text-xs" value={type} onChange={(e) => setType(e.target.value)}>
            <option value="">Any type</option>
            {(data?.types ?? []).map((t) => (
              <option key={t} value={t}>
                {TYPE_LABELS[t] ?? t}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
          Category
          <select className="input h-7 text-xs" value={category} onChange={(e) => setCategory(e.target.value)}>
            <option value="">Any category</option>
            {(data?.categories ?? []).map((c) => (
              <option key={c} value={c}>
                {CATEGORY_LABELS[c] ?? c}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
          Text
          <input className="input h-7 text-xs" value={text} onChange={(e) => setText(e.target.value)} placeholder="requester, subject, ORI" />
        </label>
        {!compact && (
          <>
            <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
              From
              <input type="datetime-local" className="input h-7 text-xs" value={from} onChange={(e) => setFrom(e.target.value)} />
            </label>
            <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
              To
              <input type="datetime-local" className="input h-7 text-xs" value={to} onChange={(e) => setTo(e.target.value)} />
            </label>
          </>
        )}
      </div>

      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-left text-xs">
          <thead>
            <tr className="border-b border-slate-800 text-[10px] uppercase tracking-wider text-slate-500">
              <th className="px-1.5 py-1">ID</th>
              <th className="px-1.5 py-1">Type</th>
              <th className="px-1.5 py-1">Category</th>
              {!incident && <th className="px-1.5 py-1">Incident</th>}
              <th className="px-1.5 py-1">Subject</th>
              <th className="px-1.5 py-1">Requester</th>
              <th className="px-1.5 py-1">Requested</th>
              <th className="px-1.5 py-1">Owner</th>
              <th className="px-1.5 py-1">Sent</th>
              <th className="px-1.5 py-1">Destination</th>
              <th className="px-1.5 py-1">Received</th>
              <th className="px-1.5 py-1">Status</th>
              <th className="px-1.5 py-1">Disposition</th>
              <th className="px-1.5 py-1">Due</th>
              <th className="px-1.5 py-1">Actions</th>
            </tr>
          </thead>
          <tbody>
            {(data?.items ?? []).map((r) => (
              <Row key={r.id} r={r} me={me} nowMs={now} warnMinutes={warnMinutes} label={label} incidentScoped={!!incident} queues={data?.queues ?? []} statuses={data?.statuses ?? []} outcomes={data?.outcomes ?? []} onChanged={load} />
            ))}
            {data && data.items.length === 0 && (
              <tr>
                <td colSpan={15} className="px-1.5 py-3 text-center text-slate-500">
                  No requests match. The queue keeps metadata only, never return text.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function Row({ r, me, nowMs, warnMinutes, label, incidentScoped, queues, statuses, outcomes, onChanged }: {
  r: Request
  me: Me
  nowMs: number
  warnMinutes: number
  label: (s: string) => string
  incidentScoped: boolean
  queues: string[]
  statuses: string[]
  outcomes: string[]
  onChanged: () => void
}) {
  const [open, setOpen] = useState<'' | 'answer' | 'handoff' | 'note' | 'attempt'>('')
  const due = dueState(r, nowMs, warnMinutes)
  const handedToMe = r.handoff && r.handoff.to === me.username

  const quick = async (body: Record<string, unknown>, ok: string) => {
    try {
      await api.post(`/requests/${r.id}`, body)
      toast.success(ok)
      onChanged()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const accept = async () => {
    try {
      await api.post(`/requests/${r.id}/accept`)
      toast.success(`Accepted ${r.id}`)
      onChanged()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  return (
    <>
      <tr className={cx('border-b border-slate-900 align-top', due === 'overdue' && 'bg-rose-950/30', due === 'nearly-due' && 'bg-amber-950/20')}>
        <td className="px-1.5 py-1 font-mono text-slate-200">{r.id}</td>
        <td className="px-1.5 py-1">{TYPE_LABELS[r.type] ?? r.type}</td>
        <td className="px-1.5 py-1">{CATEGORY_LABELS[r.category] ?? r.category}</td>
        {!incidentScoped && (
          <td className="px-1.5 py-1">
            {r.incident ? (
              <Link className="text-sky-300 hover:underline" to={`/review/${r.incident}`}>
                {r.incident}
              </Link>
            ) : (
              '—'
            )}
          </td>
        )}
        <td className="px-1.5 py-1 text-slate-300">{r.subject || '—'}</td>
        <td className="px-1.5 py-1">{r.requester || '—'}</td>
        <td className="px-1.5 py-1 font-mono text-slate-400">{hhmm(r.requestedAt)}</td>
        <td className="px-1.5 py-1">
          {r.owner}
          {r.handoff && <span className="ml-1 chip h-4 bg-slate-800 px-1 text-[9px] text-slate-300">handed to {r.handoff.to.replace('@queue:', 'queue ')}</span>}
        </td>
        <td className="px-1.5 py-1 font-mono text-slate-400">{r.sentAt ? `${hhmm(r.sentAt)}${r.sentBy ? ' ' + r.sentBy : ''}` : '—'}</td>
        <td className="px-1.5 py-1">{r.destination || '—'}</td>
        <td className="px-1.5 py-1 font-mono text-slate-400">{r.responseAt ? `${hhmm(r.responseAt)}${r.receivedBy ? ' ' + r.receivedBy : ''}` : '—'}</td>
        <td className="px-1.5 py-1">
          <span className="font-semibold text-slate-200">{label(r.status)}</span>
        </td>
        <td className="px-1.5 py-1 text-slate-400">{r.disposition || '—'}</td>
        <td className="px-1.5 py-1 font-mono">
          {r.due ? (
            <span className={cx(due === 'overdue' ? 'font-bold text-rose-300' : due === 'nearly-due' ? 'text-amber-300' : 'text-slate-400')}>
              {hhmm(r.due)}
              {due === 'overdue' ? ' OVERDUE' : due === 'nearly-due' ? ' nearly due' : ''}
            </span>
          ) : (
            '—'
          )}
        </td>
        <td className="px-1.5 py-1">
          <div className="flex flex-wrap gap-0.5">
            {!r.sentAt && !requestClosed(r.status) && (
              <button className="btn btn-ghost h-6 px-1 text-[10px]" title="Mark sent" onClick={() => quick({ sent: true }, `${r.id} marked sent`)}>
                <Send size={11} aria-hidden /> Sent
              </button>
            )}
            {!requestClosed(r.status) && (
              <button className="btn btn-ghost h-6 px-1 text-[10px]" title="Record answer / outcome" onClick={() => setOpen('answer')}>
                <Check size={11} aria-hidden /> Answer
              </button>
            )}
            <button className="btn btn-ghost h-6 px-1 text-[10px]" title="Add follow-up attempt" onClick={() => setOpen('attempt')}>
              Attempt
            </button>
            <button className="btn btn-ghost h-6 px-1 text-[10px]" title="Add a note" onClick={() => setOpen('note')}>
              Note
            </button>
            {!requestClosed(r.status) && (
              <button className="btn btn-ghost h-6 px-1 text-[10px]" title="Hand over" onClick={() => setOpen('handoff')}>
                <Hand size={11} aria-hidden /> Hand over
              </button>
            )}
            {handedToMe && (
              <button className="btn btn-ghost h-6 px-1 text-[10px]" title="Accept this request" onClick={accept}>
                <UserCheck size={11} aria-hidden /> Accept
              </button>
            )}
          </div>
        </td>
      </tr>

      {open === 'answer' && <AnswerDialog r={r} statuses={statuses} outcomes={outcomes} label={label} onClose={() => setOpen('')} onChanged={onChanged} />}
      {open === 'handoff' && <HandoffDialog r={r} queues={queues} onClose={() => setOpen('')} onChanged={onChanged} />}
      {open === 'note' && <TextDialog r={r} title="Add a note" field="note" onClose={() => setOpen('')} onChanged={onChanged} />}
      {open === 'attempt' && <TextDialog r={r} title="Add a follow-up attempt" field="attempt" onClose={() => setOpen('')} onChanged={onChanged} />}
    </>
  )
}

function AnswerDialog({ r, statuses, outcomes, label, onClose, onChanged }: {
  r: Request
  statuses: string[]
  outcomes: string[]
  label: (s: string) => string
  onClose: () => void
  onChanged: () => void
}) {
  const isHit = r.type === 'hit-confirmation'
  const choices = isHit ? outcomes : statuses
  const [status, setStatus] = useState(choices[0] ?? '')
  const [responder, setResponder] = useState('')
  const [disposition, setDisposition] = useState('')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const needNote = ['will-not-extradite', 'extradition-limited', 'pending-information'].includes(status)
  const save = async () => {
    if (needNote && !note.trim()) {
      toast.error('Write the extradition limits, instructions or what is pending')
      return
    }
    setBusy(true)
    try {
      await api.post(`/requests/${r.id}`, {
        status,
        responder: responder.trim() || undefined,
        disposition: disposition.trim() || undefined,
        note: note.trim() || undefined,
        received: true,
      })
      toast.success(`${r.id}: ${label(status)}`)
      onChanged()
      onClose()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal open onClose={onClose} title={`Record answer: ${r.id}`}>
      <div className="space-y-2 text-sm">
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
          {isHit ? 'Outcome' : 'Status'}
          <select className="input h-8 text-sm" value={status} onChange={(e) => setStatus(e.target.value)}>
            {choices.map((s) => (
              <option key={s} value={s}>
                {label(s)}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
          Responder (who answered at the other agency)
          <input className="input h-8 text-sm" value={responder} onChange={(e) => setResponder(e.target.value)} />
        </label>
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
          Disposition
          <input className="input h-8 text-sm" value={disposition} onChange={(e) => setDisposition(e.target.value)} />
        </label>
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
          Note{needNote ? ' (required for this outcome)' : ''}
          <textarea className="input min-h-[52px] text-sm" value={note} onChange={(e) => setNote(e.target.value)} />
        </label>
        <div className="flex justify-end">
          <button className="btn btn-go h-8 text-sm" onClick={save} disabled={busy}>
            Record answer
          </button>
        </div>
      </div>
    </Modal>
  )
}

function HandoffDialog({ r, queues, onClose, onChanged }: { r: Request; queues: string[]; onClose: () => void; onChanged: () => void }) {
  const [toKind, setToKind] = useState<'user' | 'queue'>('user')
  const [user, setUser] = useState('')
  const [queue, setQueue] = useState(queues[0] ?? '')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const save = async () => {
    const to = toKind === 'queue' ? `@queue:${queue}` : user.trim()
    if (!to) {
      toast.error('Name the dispatcher or queue taking it')
      return
    }
    if (!note.trim()) {
      toast.error('Write the handoff note: what is outstanding')
      return
    }
    setBusy(true)
    try {
      await api.post(`/requests/${r.id}/handoff`, { to, note: note.trim() })
      toast.success(`${r.id} handed to ${to.replace('@queue:', 'queue ')}`)
      onChanged()
      onClose()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal open onClose={onClose} title={`Hand over ${r.id}`} icon={<ArrowRightLeft size={16} aria-hidden />}>
      <div className="space-y-2 text-sm">
        <div className="flex gap-1" role="radiogroup" aria-label="Hand to">
          {([['user', 'A dispatcher'], ['queue', 'A queue']] as const).map(([v, lbl]) => (
            <button key={v} type="button" role="radio" aria-checked={toKind === v} className={cx('chip h-7 cursor-pointer px-2 text-xs', toKind === v ? 'bg-sky-500 text-white' : 'bg-slate-800 text-slate-300')} onClick={() => setToKind(v)}>
              {lbl}
            </button>
          ))}
        </div>
        {toKind === 'user' ? (
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
            Username
            <input className="input h-8 text-sm" value={user} onChange={(e) => setUser(e.target.value)} />
          </label>
        ) : (
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
            Queue
            <select className="input h-8 text-sm" value={queue} onChange={(e) => setQueue(e.target.value)}>
              {queues.map((q) => (
                <option key={q} value={q}>
                  {q}
                </option>
              ))}
            </select>
          </label>
        )}
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
          Handoff note (required): what is outstanding
          <textarea className="input min-h-[52px] text-sm" value={note} onChange={(e) => setNote(e.target.value)} />
        </label>
        <div className="flex justify-end">
          <button className="btn btn-go h-8 text-sm" onClick={save} disabled={busy}>
            Hand over
          </button>
        </div>
      </div>
    </Modal>
  )
}

function TextDialog({ r, title, field, onClose, onChanged }: { r: Request; title: string; field: 'note' | 'attempt'; onClose: () => void; onChanged: () => void }) {
  const [v, setV] = useState('')
  const [busy, setBusy] = useState(false)
  const save = async () => {
    if (!v.trim()) return
    setBusy(true)
    try {
      await api.post(`/requests/${r.id}`, { [field]: v.trim() })
      toast.success(`${title} saved`)
      onChanged()
      onClose()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal open onClose={onClose} title={`${title}: ${r.id}`}>
      <div className="space-y-2 text-sm">
        <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
          {field === 'attempt' ? 'What was done' : 'Note'}
          <textarea className="input min-h-[60px] text-sm" value={v} onChange={(e) => setV(e.target.value)} autoFocus />
        </label>
        <div className="flex justify-end">
          <button className="btn btn-go h-8 text-sm" onClick={save} disabled={busy}>
            Save
          </button>
        </div>
      </div>
    </Modal>
  )
}

export default Queue
