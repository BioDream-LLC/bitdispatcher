import { useState } from 'react'
import { MapPin, History } from 'lucide-react'
import { api } from '../api'
import { Modal } from '../ui'
import { cx } from '../util'
import type { ExceptionRow, PlacePolicy } from './types'
import { AllToggle, Busy, Empty, NoteField, Panel, SelectField, Tag, TextField, dateTime, dayOnly, isZeroTime, run, timeOnly, toRFC3339, usePoll } from './common'

// The kinds, shown in CAPITALS and with a plain-word explanation so colour is never the only signal.
const KIND_WORD: Record<string, string> = {
  forced: 'FORCED',
  estimated: 'ESTIMATED',
  map: 'MAP',
  caller: 'CALLER',
  unverified: 'UNVERIFIED',
  verified: 'VERIFIED',
}

function kindLabel(kind: string): string {
  return KIND_WORD[kind] ?? kind.toUpperCase()
}

/** Exceptions is tab 1: the open location exceptions and the supervisor's review. An item stays open
 * until it is acknowledged, both questions are answered and the follow-up is done. */
export default function Exceptions() {
  const [all, setAll] = useState(false)
  const [rows, setRows] = useState<ExceptionRow[]>([])
  const [policy, setPolicy] = useState<PlacePolicy | null>(null)
  const [openId, setOpenId] = useState<string | null>(null)

  const refresh = usePoll(async () => {
    const r = await api.get<{ items: ExceptionRow[] }>(`/exceptions?all=${all ? '1' : ''}`)
    setRows(r.items ?? [])
    if (!policy) setPolicy(await api.get<PlacePolicy>('/places/policy'))
  }, [all])

  const current = rows.find((x) => x.id === openId) ?? null

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-3">
        <p className="text-sm text-slate-400">
          Forced, estimated or otherwise unverified incident locations. Each one stays open until you acknowledge it, answer both questions and finish the follow-up.
        </p>
        <div className="ml-auto"><AllToggle all={all} onChange={setAll} /></div>
      </div>
      <Panel icon={<MapPin size={16} className="text-sky-300" aria-hidden />} title={`Location exceptions (${rows.length})`}>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs uppercase tracking-wider text-slate-500">
                <th className="px-2 py-1">Incident</th>
                <th className="px-2 py-1">Kind</th>
                <th className="px-2 py-1">Entered address</th>
                <th className="px-2 py-1">Accepted location</th>
                <th className="px-2 py-1">Reason</th>
                <th className="px-2 py-1">Call-taker / duty / console</th>
                <th className="px-2 py-1">Time</th>
                <th className="px-2 py-1">Units</th>
                <th className="px-2 py-1">Status</th>
                <th className="px-2 py-1"></th>
              </tr>
            </thead>
            <tbody>
              {rows.length === 0 && (
                <tr><td colSpan={10}><Empty>No location exceptions.</Empty></td></tr>
              )}
              {rows.map((x) => (
                <tr key={x.id} className="border-t border-slate-800 align-top">
                  <td className="px-2 py-1.5 font-mono">{x.incident} <span className="text-xs text-slate-500">{x.id}</span></td>
                  <td className="px-2 py-1.5"><span className="font-mono font-bold">{kindLabel(x.kind)}</span></td>
                  <td className="px-2 py-1.5">{x.entered || '—'}</td>
                  <td className="px-2 py-1.5">{x.location || x.accepted || '—'}</td>
                  <td className="px-2 py-1.5">{x.reason || '—'}</td>
                  <td className="px-2 py-1.5 text-xs text-slate-400">{x.by}{x.duty ? ` · ${x.duty}` : ''}{x.console ? ` · ${x.console}` : ''}</td>
                  <td className="px-2 py-1.5 text-xs text-slate-400">{dateTime(x.at)}</td>
                  <td className="px-2 py-1.5 text-xs">{(x.units ?? []).join(', ') || '—'}</td>
                  <td className="px-2 py-1.5">
                    <div className="flex flex-col gap-0.5">
                      <Tag tone={x.open ? 'warn' : 'good'}>{x.open ? 'open: needs you' : 'reviewed'}</Tag>
                      <span className="text-xs text-slate-500">queue: {x.queue}</span>
                    </div>
                  </td>
                  <td className="px-2 py-1.5">
                    <button type="button" className="btn btn-ghost" onClick={() => setOpenId(x.id)}>Review</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Panel>

      {current && policy && (
        <ReviewDrawer row={current} policy={policy} onClose={() => setOpenId(null)} onSaved={refresh} />
      )}
    </div>
  )
}

function OpenState({ row }: { row: ExceptionRow }) {
  const steps: { label: string; done: boolean }[] = [
    { label: 'Acknowledged', done: !!row.ack },
    { label: 'Question 1 answered (best information used)', done: !!row.located },
    { label: 'Question 2 answered (follow-up set)', done: !!row.followup },
    { label: 'Follow-up done', done: !!row.followup?.done },
  ]
  return (
    <div className="rounded-md border border-slate-800 bg-slate-900/60 p-2 text-xs">
      <p className="mb-1 text-slate-400">This item stays open until every step is finished:</p>
      <ul className="space-y-0.5">
        {steps.map((s) => (
          <li key={s.label} className={cx('flex items-center gap-1.5', s.done ? 'text-emerald-300' : 'text-amber-300')}>
            <span aria-hidden>{s.done ? '✓' : '•'}</span>
            <span>{s.label}{s.done ? ' — done' : ' — still needed'}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}

function ReviewDrawer({ row, policy, onClose, onSaved }: {
  row: ExceptionRow
  policy: PlacePolicy
  onClose: () => void
  onSaved: () => void
}) {
  // Question 1.
  const [answer, setAnswer] = useState<'yes' | 'no' | ''>(row.located?.answer ?? '')
  const [answerNote, setAnswerNote] = useState(row.located?.note ?? '')
  // Question 2.
  const [disposition, setDisposition] = useState(row.followup?.disposition ?? '')
  const [owner, setOwner] = useState(row.followup?.owner ?? '')
  const [due, setDue] = useState(() => (row.followup?.due && !isZeroTime(row.followup.due) ? new Date(row.followup.due).toISOString().slice(0, 10) : ''))
  const [fuNote, setFuNote] = useState('')
  // Address queue.
  const [queue, setQueue] = useState(row.queue)
  const [queueOwner, setQueueOwner] = useState(row.queueOwner ?? '')
  const [queueNote, setQueueNote] = useState('')
  const [master, setMaster] = useState(row.linked?.address ?? '')
  const [lat, setLat] = useState(row.linked?.lat != null ? String(row.linked.lat) : '')
  const [lon, setLon] = useState(row.linked?.lon != null ? String(row.linked.lon) : '')
  // A free note.
  const [note, setNote] = useState('')

  const needsMaster = queue === 'verified' || queue === 'merged'

  const ack = () => run(() => api.post(`/exceptions/${row.id}/review`, { ack: true }), onSaved)

  const saveQ1 = () => {
    if (answer !== 'yes' && answer !== 'no') return
    return run(() => api.post(`/exceptions/${row.id}/review`, { located: { answer, note: answerNote } }), onSaved)
  }
  const saveQ2 = () => {
    if (!disposition) return
    const f: Record<string, unknown> = { disposition, owner, note: fuNote }
    if (due) f.due = toRFC3339(due + 'T00:00:00')
    return run(() => api.post(`/exceptions/${row.id}/review`, { followup: f }), onSaved)
  }
  const markDone = () => run(() => api.post(`/exceptions/${row.id}/review`, { done: true, note }), onSaved)
  const saveNote = () => run(() => api.post(`/exceptions/${row.id}/review`, { note }), () => { setNote(''); onSaved() })

  const saveQueue = () => {
    const body: Record<string, unknown> = { state: queue, owner: queueOwner, note: queueNote }
    if (needsMaster) {
      const link: Record<string, unknown> = { address: master }
      if (lat) link.lat = Number(lat)
      if (lon) link.lon = Number(lon)
      body.link = link
    }
    return run(() => api.post(`/exceptions/${row.id}/queue`, body), onSaved)
  }

  return (
    <Modal open onClose={onClose} title={`Review ${row.id} · ${row.incident}`} icon={<MapPin size={16} aria-hidden />} wide>
      <div className="space-y-4">
        <OpenState row={row} />

        <div className="grid gap-1 rounded-md border border-slate-800 p-2 text-sm sm:grid-cols-2">
          <div><span className="text-slate-500">Kind</span>: <b className="font-mono">{kindLabel(row.kind)}</b></div>
          <div><span className="text-slate-500">Entered</span>: {row.entered || '—'}</div>
          <div><span className="text-slate-500">Accepted</span>: {row.location || row.accepted || '—'}</div>
          <div><span className="text-slate-500">Reason</span>: {row.reason || '—'}{row.reasonNote ? ` (${row.reasonNote})` : ''}</div>
          <div><span className="text-slate-500">Source / confidence</span>: {row.source || '—'} / {row.confidence || '—'}</div>
          <div><span className="text-slate-500">Nearest</span>: {row.nearest || '—'}</div>
        </div>

        <div className="flex items-center gap-2">
          <Busy go disabled={!!row.ack} onClick={ack}>{row.ack ? `Acknowledged by ${row.ack.by}` : 'Acknowledge'}</Busy>
          {row.ack && <span className="text-xs text-slate-500">{dateTime(row.ack.at)}</span>}
        </div>

        <Panel title="Question 1: Was the best available location information used?">
          <div className="space-y-2">
            <div role="radiogroup" aria-label="Was the best available location information used" className="flex gap-4 text-sm">
              <label className="flex items-center gap-1.5"><input type="radio" name={`q1-${row.id}`} checked={answer === 'yes'} onChange={() => setAnswer('yes')} /> Yes</label>
              <label className="flex items-center gap-1.5"><input type="radio" name={`q1-${row.id}`} checked={answer === 'no'} onChange={() => setAnswer('no')} /> No</label>
            </div>
            {answer === 'no' && (
              <NoteField id={`q1note-${row.id}`} label="What better information was there? (required)" value={answerNote} onChange={setAnswerNote} />
            )}
            <Busy onClick={saveQ1} disabled={answer === '' || (answer === 'no' && !answerNote.trim())}>Save answer</Busy>
          </div>
        </Panel>

        <Panel title="Question 2: Does the agency address source or workflow need follow-up?">
          <div className="grid gap-2 sm:grid-cols-2">
            <SelectField id={`disp-${row.id}`} label="Disposition" value={disposition} onChange={setDisposition} options={policy.dispositions} />
            <TextField id={`owner-${row.id}`} label="Owner" value={owner} onChange={setOwner} placeholder="username" />
            <TextField id={`due-${row.id}`} label="Due date" value={due} onChange={setDue} type="date" />
            <div className="sm:col-span-2"><NoteField id={`funote-${row.id}`} label="Note" value={fuNote} onChange={setFuNote} /></div>
          </div>
          <div className="mt-2 flex items-center gap-2">
            <Busy onClick={saveQ2} disabled={!disposition}>Save follow-up</Busy>
            <Busy onClick={markDone} disabled={!row.followup && !disposition}>Follow-up done</Busy>
          </div>
        </Panel>

        <Panel title="Address queue">
          <div className="grid gap-2 sm:grid-cols-2">
            <SelectField id={`queue-${row.id}`} label="State" value={queue} onChange={setQueue} options={policy.queue} placeholder="Choose a state" />
            <TextField id={`qowner-${row.id}`} label="Owner" value={queueOwner} onChange={setQueueOwner} placeholder="username" />
            {needsMaster && (
              <>
                <div className="sm:col-span-2"><TextField id={`master-${row.id}`} label="Master address (required for verified or merged)" value={master} onChange={setMaster} /></div>
                <TextField id={`lat-${row.id}`} label="Latitude (optional)" value={lat} onChange={setLat} />
                <TextField id={`lon-${row.id}`} label="Longitude (optional)" value={lon} onChange={setLon} />
              </>
            )}
            <div className="sm:col-span-2"><NoteField id={`qnote-${row.id}`} label="Note (required when rejected)" value={queueNote} onChange={setQueueNote} /></div>
          </div>
          <div className="mt-2">
            <Busy onClick={saveQueue} disabled={needsMaster && !master.trim()}>Save address queue</Busy>
          </div>
          <p className="mt-1 text-xs text-slate-500">Verifying or merging links the incident to the master address. What was entered and what responders were shown is kept.</p>
        </Panel>

        <Panel title="Add a note">
          <NoteField id={`note-${row.id}`} label="Note" value={note} onChange={setNote} />
          <div className="mt-2"><Busy ghost onClick={saveNote} disabled={!note.trim()}>Add note</Busy></div>
        </Panel>

        <Panel title="History" icon={<History size={14} aria-hidden />}>
          {(row.notes ?? []).length === 0 ? (
            <Empty>No history yet.</Empty>
          ) : (
            <ul className="space-y-1 text-sm">
              {(row.notes ?? []).map((n, i) => (
                <li key={i} className="border-b border-slate-800/60 pb-1">
                  <span className="text-xs text-slate-500">{timeOnly(n.at)} · {dayOnly(n.at)} · {n.by}</span>
                  <div>{n.what}{n.note ? `: ${n.note}` : ''}</div>
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </div>
    </Modal>
  )
}
