import { useState } from 'react'
import { Radio } from 'lucide-react'
import { api } from '../api'
import type { CriticalFeed, PhraseAlert } from './types'
import { Busy, Empty, NoteField, Panel, SelectField, Tag, dateTime, run, usePoll } from './common'

/** Phrases is tab 3: open phrase alerts the supervisor disposes of. A negated or uncertain phrase is
 * shown for review and never treated as the real thing; a machine tag is an aid, not a finding. */
export default function Phrases() {
  const [alerts, setAlerts] = useState<PhraseAlert[]>([])
  const [dispositions, setDispositions] = useState<string[]>([])

  const refresh = usePoll(async () => {
    const r = await api.get<CriticalFeed>('/critical?all=')
    setAlerts(r.phrases ?? [])
    setDispositions(r.dispositions ?? [])
  }, [])

  return (
    <div className="space-y-3">
      <p className="text-sm text-slate-400">
        Phrases heard in call and radio transcripts and narrative. Labels below are plain words: a phrase shown as NEGATED (for example &ldquo;no shots fired&rdquo;) is not treated as the real thing, UNCERTAIN needs a person to check it, and TESTING phrases do not alert.
      </p>
      <Panel icon={<Radio size={16} className="text-sky-300" aria-hidden />} title={`Open phrase alerts (${alerts.length})`}>
        {alerts.length === 0 ? (
          <Empty>No open phrase alerts.</Empty>
        ) : (
          <ul className="space-y-2">
            {alerts.map((a) => (
              <PhraseRow key={a.id} alert={a} dispositions={dispositions} onSaved={refresh} />
            ))}
          </ul>
        )}
      </Panel>
    </div>
  )
}

function PhraseRow({ alert, dispositions, onSaved }: { alert: PhraseAlert; dispositions: string[]; onSaved: () => void }) {
  const [disposition, setDisposition] = useState('')
  const [note, setNote] = useState('')
  const dispose = () => run(() => api.post(`/phrases/${alert.id}/dispose`, { disposition, note }), onSaved)
  return (
    <li className="rounded-md border border-slate-800 bg-slate-900/50 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-semibold text-slate-100">{alert.name}</span>
        <span className="text-xs text-slate-500">{alert.id}</span>
        <Tag>severity: {alert.severity}</Tag>
        {alert.negated && <Tag tone="warn" title={alert.negation ? `negation: ${alert.negation}` : undefined}>NEGATED</Tag>}
        {alert.uncertain && <Tag tone="warn">UNCERTAIN</Tag>}
        {alert.testing && <Tag>TESTING</Tag>}
        {alert.count > 1 && <Tag>heard {alert.count} times</Tag>}
      </div>
      <div className="mt-1 grid gap-x-6 gap-y-0.5 text-sm sm:grid-cols-2">
        <div><span className="text-slate-500">Found</span>: &ldquo;{alert.found}&rdquo;</div>
        <div><span className="text-slate-500">Source</span>: {alert.source}{alert.channel ? ` · ${alert.channel}` : ''}</div>
        <div className="sm:col-span-2"><span className="text-slate-500">Context</span>: {alert.context || '—'}</div>
        <div><span className="text-slate-500">Speaker</span>: {alert.speaker || '—'}</div>
        <div><span className="text-slate-500">Incident</span>: {alert.incident || '—'}</div>
        <div><span className="text-slate-500">Time</span>: {dateTime(alert.at)}</div>
      </div>
      <div className="mt-2 grid items-end gap-2 sm:grid-cols-[1fr_1fr_auto]">
        <SelectField id={`pd-${alert.id}`} label="Disposition" value={disposition} onChange={setDisposition} options={dispositions} />
        <NoteField id={`pn-${alert.id}`} label="Note" value={note} onChange={setNote} rows={1} />
        <Busy go onClick={dispose} disabled={!disposition}>Dispose</Busy>
      </div>
    </li>
  )
}
