import { useState } from 'react'
import { ListChecks, Plus } from 'lucide-react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { FollowupsResponse, TrackedFollowup } from './types'
import { AllToggle, Busy, Empty, NoteField, Panel, TextField, dateTime, dayOnly, isZeroTime, run, timeOnly, usePoll } from './common'

/** Followups is tab 6: the open follow-up still owed, one item per source, carried forward until done. */
export default function Followups() {
  const [all, setAll] = useState(false)
  const [items, setItems] = useState<TrackedFollowup[]>([])

  const refresh = usePoll(async () => {
    const r = await api.get<FollowupsResponse>(`/followups?all=${all ? '1' : ''}`)
    setItems(r.items ?? [])
  }, [all])

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-3">
        <p className="text-sm text-slate-400">Work owed after the period: from location exceptions, critical reviews, phrase alerts, NCIC requests, or added by hand. It carries forward until it is done.</p>
        <div className="ml-auto"><AllToggle all={all} onChange={setAll} /></div>
      </div>

      <AddFollowup onSaved={refresh} />

      <Panel icon={<ListChecks size={16} className="text-sky-300" aria-hidden />} title={`Follow-up (${items.length})`}>
        {items.length === 0 ? (
          <Empty>No follow-up owed.</Empty>
        ) : (
          <ul className="space-y-2">
            {items.map((f) => <FollowupRow key={f.id} fu={f} onSaved={refresh} />)}
          </ul>
        )}
      </Panel>
    </div>
  )
}

function AddFollowup({ onSaved }: { onSaved: () => void }) {
  const [title, setTitle] = useState('')
  const [incident, setIncident] = useState('')
  const [owner, setOwner] = useState('')
  const [due, setDue] = useState('')
  const [note, setNote] = useState('')
  const add = () =>
    run(
      () => api.post('/followups', { title, incident, owner, due: due ? new Date(due + 'T00:00:00').toISOString() : undefined, note }),
      () => { setTitle(''); setIncident(''); setOwner(''); setDue(''); setNote(''); onSaved() },
    )
  return (
    <Panel icon={<Plus size={16} aria-hidden />} title="Add a manual follow-up">
      <div className="grid gap-2 sm:grid-cols-2">
        <TextField id="fu-title" label="What is owed" value={title} onChange={setTitle} />
        <TextField id="fu-incident" label="Incident (optional)" value={incident} onChange={setIncident} />
        <TextField id="fu-owner" label="Owner" value={owner} onChange={setOwner} placeholder="username" />
        <TextField id="fu-due" label="Due date" value={due} onChange={setDue} type="date" />
        <div className="sm:col-span-2"><NoteField id="fu-note" label="Note" value={note} onChange={setNote} rows={1} /></div>
      </div>
      <div className="mt-2"><Busy go onClick={add} disabled={!title.trim()}>Add follow-up</Busy></div>
    </Panel>
  )
}

function FollowupRow({ fu, onSaved }: { fu: TrackedFollowup; onSaved: () => void }) {
  const [owner, setOwner] = useState(fu.owner ?? '')
  const [due, setDue] = useState(() => (fu.due && !isZeroTime(fu.due) ? new Date(fu.due).toISOString().slice(0, 10) : ''))
  const [note, setNote] = useState('')

  const reassign = () =>
    run(() => api.post('/followups', { key: fu.key, owner, due: due ? new Date(due + 'T00:00:00').toISOString() : undefined, note }), () => { setNote(''); onSaved() })
  const markDone = () => run(() => api.post('/followups', { key: fu.key, done: true, note }), onSaved)

  return (
    <li className="rounded-md border border-slate-800 bg-slate-900/50 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-semibold text-slate-100">{fu.id} {fu.title}</span>
        {fu.done ? <span className="chip bg-emerald-500/20 text-emerald-200">done</span> : <span className="chip bg-amber-500/20 text-amber-200">open</span>}
        <span className="text-xs text-slate-500">from {fu.key}</span>
        {fu.incident && <Link to={`/review/${fu.incident}`} className="text-xs text-sky-300 hover:underline">incident {fu.incident}</Link>}
      </div>
      <div className="mt-1 text-xs text-slate-400">Owner: {fu.owner || '—'} · Due: {isZeroTime(fu.due) ? '—' : dayOnly(fu.due)} · Opened: {dateTime(fu.opened)} in {fu.period}{fu.done ? ` · Done ${dateTime(fu.doneAt)}` : ''}</div>

      {!fu.done && (
        <div className="mt-2 grid items-end gap-2 sm:grid-cols-[1fr_1fr_1fr_auto_auto]">
          <TextField id={`fuo-${fu.id}`} label="Reassign owner" value={owner} onChange={setOwner} placeholder="username" />
          <TextField id={`fud-${fu.id}`} label="Due" value={due} onChange={setDue} type="date" />
          <NoteField id={`fun-${fu.id}`} label="Note" value={note} onChange={setNote} rows={1} />
          <Busy onClick={reassign}>Save</Busy>
          <Busy go onClick={markDone}>Mark done</Busy>
        </div>
      )}

      {(fu.history ?? []).length > 0 && (
        <details className="mt-2 text-sm">
          <summary className="cursor-pointer text-xs text-slate-400">History ({(fu.history ?? []).length})</summary>
          <ul className="mt-1 space-y-0.5">
            {(fu.history ?? []).map((h, i) => (
              <li key={i} className="text-xs text-slate-400">{timeOnly(h.at)} · {dayOnly(h.at)} · {h.by}: {h.what}{h.note ? ` — ${h.note}` : ''}</li>
            ))}
          </ul>
        </details>
      )}
    </li>
  )
}
