import { useState } from 'react'
import { Users } from 'lucide-react'
import { api } from '../api'
import type { PeopleResponse, Period, PersonShift } from './types'
import { Busy, Empty, NoteField, Panel, SelectField, run, seconds, usePoll } from './common'

/** People is tab 5: each person's shift record for the period, for coaching and awareness. */
export default function People({ periodId, from, to }: { periodId: string; from?: string; to?: string }) {
  const [people, setPeople] = useState<PersonShift[]>([])
  const [period, setPeriod] = useState<Period | null>(null)
  const [noteFor, setNoteFor] = useState<string | null>(null)

  const query = from && to ? `from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}` : `period=${encodeURIComponent(periodId)}`

  const refresh = usePoll(async () => {
    const r = await api.get<PeopleResponse>(`/shift/people?${query}`)
    setPeople((r.people ?? []).slice().sort((a, b) => (a.name ?? a.user).localeCompare(b.name ?? b.user)))
    setPeriod(r.period)
  }, [query])

  return (
    <div className="space-y-3">
      <div className="rounded-md border border-sky-800/50 bg-sky-950/30 p-2 text-sm text-sky-100">
        This is for coaching and awareness. A screen lock is not a break. Nothing here ranks or disciplines anyone.
      </div>
      <Panel icon={<Users size={16} className="text-sky-300" aria-hidden />} title={`People this period${period ? ` · ${period.name}` : ''} (${people.length})`}>
        {people.length === 0 ? (
          <Empty>No shift records for this period yet.</Empty>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-xs uppercase tracking-wider text-slate-500">
                  <th className="px-2 py-1">User</th>
                  <th className="px-2 py-1">Name</th>
                  <th className="px-2 py-1">Duties</th>
                  <th className="px-2 py-1">Positions</th>
                  <th className="px-2 py-1">Sign-ons</th>
                  <th className="px-2 py-1">Calls</th>
                  <th className="px-2 py-1">Mean answer</th>
                  <th className="px-2 py-1">Created</th>
                  <th className="px-2 py-1">Mean answer to incident</th>
                  <th className="px-2 py-1">Dispatches</th>
                  <th className="px-2 py-1">Statuses</th>
                  <th className="px-2 py-1">Queries</th>
                  <th className="px-2 py-1">Critical roles</th>
                  <th className="px-2 py-1">Breaks</th>
                  <th className="px-2 py-1">Console locks</th>
                  <th className="px-2 py-1"></th>
                </tr>
              </thead>
              <tbody>
                {people.map((p) => (
                  <tr key={p.user} className="border-t border-slate-800 align-top">
                    <td className="px-2 py-1.5 font-mono">{p.user}{p.trainee && <span className="chip ml-1 bg-sky-500/20 text-sky-200">trainee</span>}</td>
                    <td className="px-2 py-1.5">{p.name || '—'}</td>
                    <td className="px-2 py-1.5 text-xs">{(p.duties ?? []).join(', ') || '—'}</td>
                    <td className="px-2 py-1.5 text-xs">{(p.consoles ?? []).join(', ') || '—'}</td>
                    <td className="px-2 py-1.5">{(p.signOns ?? []).length}</td>
                    <td className="px-2 py-1.5">{p.calls}</td>
                    <td className="px-2 py-1.5">{seconds(p.answerSecs)}</td>
                    <td className="px-2 py-1.5">{p.created}</td>
                    <td className="px-2 py-1.5">{seconds(p.createSecs)}</td>
                    <td className="px-2 py-1.5">{p.dispatches}</td>
                    <td className="px-2 py-1.5">{p.statuses}</td>
                    <td className="px-2 py-1.5">{p.queries}</td>
                    <td className="px-2 py-1.5 text-xs">{(p.critical ?? []).length === 0 ? '—' : (p.critical ?? []).join(', ')}</td>
                    <td className="px-2 py-1.5">{(p.breaks ?? []).length} ({Math.round(p.breakMins)} min)</td>
                    <td className="px-2 py-1.5">{(p.locks ?? []).length} <span className="text-xs text-slate-500">(not a break)</span></td>
                    <td className="px-2 py-1.5">
                      <button type="button" className="btn btn-ghost" onClick={() => setNoteFor(noteFor === p.user ? null : p.user)}>Note</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {people.some((p) => p.prompt) && (
          <div className="mt-3 space-y-2">
            {people.filter((p) => p.prompt).map((p) => (
              <div key={p.user} className="rounded-md border border-amber-700/50 bg-amber-950/30 p-2 text-sm text-amber-200">
                <b>{p.name || p.user}</b> review prompt: {p.prompt}
              </div>
            ))}
          </div>
        )}
      </Panel>

      {noteFor && period && (
        <NoteForm person={noteFor} periodId={period.id} onDone={() => { setNoteFor(null); refresh() }} />
      )}
    </div>
  )
}

function NoteForm({ person, periodId, onDone }: { person: string; periodId: string; onDone: () => void }) {
  const [kind, setKind] = useState('context')
  const [text, setText] = useState('')
  const save = () => run(() => api.post('/shift/notes', { person, period: periodId, text, kind }), onDone)
  return (
    <Panel title={`Note on ${person}`}>
      <div className="grid gap-2 sm:grid-cols-[12rem_1fr]">
        <SelectField id="note-kind" label="Kind" value={kind} onChange={setKind} options={['context', 'correction']} placeholder="context" />
        <NoteField id="note-text" label="Note" value={text} onChange={setText} />
      </div>
      <p className="mt-1 text-xs text-slate-500">A note is kept with the record and never changes the events themselves.</p>
      <div className="mt-2"><Busy go onClick={save} disabled={!text.trim()}>Save note</Busy></div>
    </Panel>
  )
}
