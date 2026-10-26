import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Send, ShieldAlert } from 'lucide-react'
import { api, errText } from '../api'
import { Modal } from '../ui'
import { cx } from '../util'
import { OUTCOME_LABELS, type InqReturn, type Request } from './types'

// What is being confirmed, read from the return: a person's warrant, or a plate/VIN/record.
function subjectLine(record: string, ret: InqReturn): string {
  const f = ret.fields ?? {}
  const parts: string[] = []
  if (ret.kind === 'person') {
    parts.push(f.name || 'the person on ' + record)
    if (f.dob) parts.push('DOB ' + f.dob)
  } else {
    const plate = [f.state, f.plate].filter(Boolean).join(' ')
    if (plate) parts.push('plate ' + plate)
    if (f.vin) parts.push('VIN ' + f.vin)
    if (!plate && !f.vin) parts.push('vehicle on ' + record)
  }
  const hit = ret.hit ? ret.hit.toUpperCase() : 'POSSIBLE HIT'
  return `${hit}${ret.hitDetail ? ' (' + ret.hitDetail + ')' : ''}: ${parts.join(', ')}`
}

/** HitConfirm sends a hit confirmation to the entering agency for one possible-hit return, then records
 * the answer. It shows exactly which person/warrant or plate/VIN/record is being confirmed. */
export default function HitConfirm({ incident, record, ret, requester, open, onClose, onSent, onAnswered }: {
  incident: string
  record: string // P1, V2
  ret: InqReturn
  requester?: string
  open: boolean
  onClose: () => void
  onSent?: (requestId: string) => void
  onAnswered?: () => void
}) {
  // When the return already has a confirmation sent (has a request id on it through the queue), the second
  // panel records the answer. We keep the request id locally after sending.
  const [requestId, setRequestId] = useState<string>(ret.request ?? '')
  const [sending, setSending] = useState(false)
  const [reqUnit, setReqUnit] = useState(requester ?? ret.requester ?? '')
  const [dest, setDest] = useState(ret.hitOri ?? '')
  const [method, setMethod] = useState('message switch')
  const [priority, setPriority] = useState<'urgent' | 'routine'>('urgent')
  const [note, setNote] = useState('')

  const [outcome, setOutcome] = useState('confirmed-valid')
  const [responder, setResponder] = useState('')
  const [answerNote, setAnswerNote] = useState('')
  const [attempt, setAttempt] = useState('')
  const [recording, setRecording] = useState(false)

  useEffect(() => {
    if (open) {
      setRequestId(ret.request ?? '')
      setReqUnit(requester ?? ret.requester ?? '')
      setDest(ret.hitOri ?? '')
    }
  }, [open, ret, requester])

  const send = async () => {
    if (sending) return
    setSending(true)
    try {
      const r = await api.post<{ id: string }>(`/incidents/${incident}/records/${record}/hit-confirmation`, {
        returnId: ret.id,
        destination: dest.trim() || undefined,
        method,
        priority,
        requester: reqUnit.trim() || undefined,
        note: note.trim() || undefined,
      })
      setRequestId(r.id)
      toast.success(`Hit confirmation ${r.id} sent to ${dest || 'the entering agency'}`)
      onSent?.(r.id)
    } catch (e) {
      const m = errText(e)
      toast.error(m)
    } finally {
      setSending(false)
    }
  }

  const recordAnswer = async () => {
    if (!requestId) return
    setRecording(true)
    try {
      const needNote = ['will-not-extradite', 'extradition-limited', 'pending-information'].includes(outcome)
      if (needNote && !answerNote.trim()) {
        toast.error('Write the extradition limits, instructions or what is pending')
        setRecording(false)
        return
      }
      await api.post(`/requests/${requestId}`, {
        status: outcome,
        responder: responder.trim() || undefined,
        note: answerNote.trim() || undefined,
      } as Partial<Request>)
      toast.success(`Answer recorded: ${OUTCOME_LABELS[outcome] ?? outcome}`)
      onAnswered?.()
      onClose()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setRecording(false)
    }
  }

  const addAttempt = async () => {
    if (!requestId || !attempt.trim()) return
    try {
      await api.post(`/requests/${requestId}`, { attempt: attempt.trim() })
      toast.success('Follow-up attempt recorded')
      setAttempt('')
      onAnswered?.()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  return (
    <Modal open={open} onClose={onClose} wide title="Hit confirmation" icon={<ShieldAlert size={16} className="text-rose-300" aria-hidden />}>
      <div className="space-y-4 text-sm">
        <div className="rounded-md border border-rose-800 bg-rose-950/40 px-3 py-2 text-rose-100">
          <div className="text-[10px] font-bold uppercase tracking-wider text-rose-300">Confirming</div>
          <div className="font-semibold">{subjectLine(record, ret)}</div>
          <div className="text-[11px] text-rose-200/80">
            Record {record} · return {ret.id} · {(ret.source ?? '').toUpperCase()}
            {ret.confirmed ? ` · already answered: ${OUTCOME_LABELS[ret.confirmed] ?? ret.confirmed}` : ''}
          </div>
        </div>

        {!requestId && (
          <section className="space-y-2" aria-label="Send the hit confirmation">
            <div className="grid grid-cols-2 gap-2">
              <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
                Requester unit
                <input className="input h-8 text-sm" value={reqUnit} onChange={(e) => setReqUnit(e.target.value)} placeholder="unit or officer" />
              </label>
              <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
                Destination ORI (entering agency)
                <input className="input h-8 text-sm" value={dest} onChange={(e) => setDest(e.target.value)} placeholder="VA0590000" />
              </label>
              <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
                Method
                <select className="input h-8 text-sm" value={method} onChange={(e) => setMethod(e.target.value)}>
                  <option value="message switch">Message switch</option>
                  <option value="telephone">Telephone</option>
                  <option value="teletype">Teletype</option>
                </select>
              </label>
              <fieldset className="flex flex-col gap-0.5 text-[11px] text-slate-400">
                <legend className="mb-0.5">Priority</legend>
                <div className="flex gap-1" role="radiogroup" aria-label="Priority">
                  {([['urgent', 'Urgent (10 minutes)'], ['routine', 'Routine (1 hour)']] as const).map(([v, lbl]) => (
                    <button
                      key={v}
                      type="button"
                      role="radio"
                      aria-checked={priority === v}
                      className={cx('chip h-7 flex-1 cursor-pointer px-2 text-xs', priority === v ? 'bg-sky-500 text-white' : 'bg-slate-800 text-slate-300')}
                      onClick={() => setPriority(v)}
                    >
                      {lbl}
                    </button>
                  ))}
                </div>
              </fieldset>
            </div>
            <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
              Notes
              <textarea className="input min-h-[52px] text-sm" value={note} onChange={(e) => setNote(e.target.value)} placeholder="what to ask the entering agency" />
            </label>
            <div className="flex justify-end">
              <button className="btn btn-go h-8 text-sm" onClick={send} disabled={sending}>
                <Send size={13} aria-hidden /> Send hit confirmation
              </button>
            </div>
          </section>
        )}

        {requestId && (
          <section className="space-y-3" aria-label="Record the answer">
            <div className="text-[11px] text-slate-400">
              Sent as request <span className="font-mono text-slate-200">{requestId}</span>. Record the answer when the entering agency replies.
            </div>
            <div className="grid grid-cols-2 gap-2">
              <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
                Outcome
                <select className="input h-8 text-sm" value={outcome} onChange={(e) => setOutcome(e.target.value)}>
                  {Object.entries(OUTCOME_LABELS).map(([v, lbl]) => (
                    <option key={v} value={v}>
                      {lbl}
                    </option>
                  ))}
                </select>
              </label>
              <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
                Responder (who answered at the other agency)
                <input className="input h-8 text-sm" value={responder} onChange={(e) => setResponder(e.target.value)} placeholder="name or ID" />
              </label>
            </div>
            <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
              Notes (extradition limits, pickup radius, caution, reference numbers)
              <textarea className="input min-h-[52px] text-sm" value={answerNote} onChange={(e) => setAnswerNote(e.target.value)} />
            </label>
            <div className="flex justify-end">
              <button className="btn btn-go h-8 text-sm" onClick={recordAnswer} disabled={recording}>
                Record answer
              </button>
            </div>
            <div className="border-t border-slate-800 pt-2">
              <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
                Follow-up attempt (what was done)
                <div className="flex gap-1">
                  <input className="input h-8 flex-1 text-sm" value={attempt} onChange={(e) => setAttempt(e.target.value)} placeholder="called again, left message..." />
                  <button className="btn h-8 text-sm" onClick={addAttempt} disabled={!attempt.trim()}>
                    Add attempt
                  </button>
                </div>
              </label>
            </div>
          </section>
        )}
      </div>
    </Modal>
  )
}
