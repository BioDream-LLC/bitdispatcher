import { useState } from 'react'
import { ShieldAlert, FileText, Download, RefreshCw, Flag } from 'lucide-react'
import { api } from '../api'
import { Modal } from '../ui'
import { cx } from '../util'
import type { CriticalCase, CriticalFeed, CriticalReport, DerivedView, QARow, ReportResponse } from './types'
import { AllToggle, Busy, Empty, NoteField, Panel, SelectField, TextField, Tag, datetimeLocalNow, dateTime, run, seconds, timeOnly, toRFC3339, usePoll } from './common'

/** Critical is tab 2: the critical incident reviews, the manual designation forms, and the assembled
 * full Critical Incident Review. */
export default function Critical() {
  const [all, setAll] = useState(false)
  const [cases, setCases] = useState<CriticalCase[]>([])
  const [results, setResults] = useState<string[]>([])
  const [openId, setOpenId] = useState<string | null>(null)

  const refresh = usePoll(async () => {
    const r = await api.get<CriticalFeed>(`/critical?all=${all ? '1' : ''}`)
    setCases(r.items ?? [])
    setResults(r.results ?? [])
  }, [all])

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-3">
        <p className="text-sm text-slate-400">Each critical incident opens one review. A trigger alerts the supervisor; the full review is assembled only when you ask for it.</p>
        <div className="ml-auto"><AllToggle all={all} onChange={setAll} /></div>
      </div>

      <div className="grid gap-3 lg:grid-cols-2">
        <FlagForm on onSaved={refresh} />
        <FlagForm on={false} onSaved={refresh} />
      </div>

      <Panel icon={<ShieldAlert size={16} className="text-sky-300" aria-hidden />} title={`Critical incidents (${cases.length})`}>
        {cases.length === 0 ? (
          <Empty>No critical incidents.</Empty>
        ) : (
          <ul className="space-y-2">
            {cases.map((c) => (
              <li key={c.id} className="rounded-md border border-slate-800 bg-slate-900/50 p-3">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-semibold text-slate-100">{c.id} · {c.incident}</span>
                  {!c.designated && <Tag tone="warn">designation removed</Tag>}
                  <Tag tone={c.ack ? 'good' : 'warn'}>{c.ack ? `acknowledged by ${c.ack.by}` : 'not acknowledged'}</Tag>
                  <Tag tone={c.review === 'complete' ? 'good' : 'info'}>review: {c.review}</Tag>
                </div>
                <div className="mt-1 text-sm text-slate-300">{c.summary}</div>
                <div className="mt-1 text-xs text-slate-400">Triggers: {(c.triggers ?? []).map((t) => t.name).join('; ') || '—'}</div>
                <div className="mt-2 flex flex-wrap gap-2">
                  <Busy disabled={!!c.ack} onClick={() => run(() => api.post(`/critical/${c.id}/ack`), refresh)}>{c.ack ? 'Acknowledged' : 'Acknowledge'}</Busy>
                  <button type="button" className="btn btn-go" onClick={() => setOpenId(c.id)}>Open full review</button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </Panel>

      {openId && (
        <FullReview id={openId} results={results} onClose={() => setOpenId(null)} onSaved={refresh} />
      )}
    </div>
  )
}

function FlagForm({ on, onSaved }: { on: boolean; onSaved: () => void }) {
  const [incident, setIncident] = useState('')
  const [reason, setReason] = useState('')
  const go = () =>
    run(() => api.post('/critical/flag', { incident, on, reason }), () => { setIncident(''); setReason(''); onSaved() })
  return (
    <Panel icon={<Flag size={16} aria-hidden />} title={on ? 'Mark an incident critical' : 'Remove critical designation'}>
      <div className="grid gap-2 sm:grid-cols-2">
        <TextField id={`flag-inc-${on}`} label="Incident number" value={incident} onChange={setIncident} />
        <TextField id={`flag-reason-${on}`} label="Reason" value={reason} onChange={setReason} />
      </div>
      <div className="mt-2"><Busy go={on} onClick={go} disabled={!incident.trim() || !reason.trim()}>{on ? 'Mark critical' : 'Remove designation'}</Busy></div>
      {!on && <p className="mt-1 text-xs text-slate-500">The original triggers stay on record.</p>}
    </Panel>
  )
}

function FullReview({ id, results, onClose, onSaved }: { id: string; results: string[]; onClose: () => void; onSaved: () => void }) {
  const [report, setReport] = useState<CriticalReport | null>(null)
  const [cutoff, setCutoff] = useState(datetimeLocalNow())
  const [narrative, setNarrative] = useState('')
  const [findings, setFindings] = useState('')
  const [derived, setDerived] = useState<{ view: string; data: DerivedView } | null>(null)

  const load = async () => {
    const cq = cutoff ? `?cutoff=${encodeURIComponent(toRFC3339(cutoff))}` : ''
    const r = await api.get<ReportResponse>(`/critical/${id}/report${cq}`)
    setReport(r.report)
    setNarrative((n) => n || r.report.case.narrative || '')
    setFindings((f) => f || r.report.case.findings || '')
  }
  usePoll(load, [id])

  const c = report?.case
  const downloadHref = `./api/critical/${id}/report?format=text${cutoff ? `&cutoff=${encodeURIComponent(toRFC3339(cutoff))}` : ''}`

  return (
    <Modal open onClose={onClose} title={`Critical incident review ${id}`} icon={<ShieldAlert size={16} aria-hidden />} xl>
      {!report || !c ? (
        <p className="text-sm text-slate-400">Assembling the review…</p>
      ) : (
        <div className="space-y-4 text-sm">
          {/* cover */}
          <section className="rounded-md border border-slate-800 p-3">
            <div className="text-base font-semibold text-slate-100">{report.incident.number} · {report.incident.type} · P{report.incident.priority}</div>
            <div className="text-slate-300">{report.incident.location?.address || '—'}</div>
            <div className="mt-1 grid gap-x-6 gap-y-0.5 text-xs text-slate-400 sm:grid-cols-3">
              <div>Version it will be: {report.version}</div>
              <div>Evidence cutoff: {dateTime(report.cutoff)}</div>
              <div>Generated: {dateTime(report.generated)} ({report.timeZone})</div>
              <div>Designated critical: {c.designated ? 'yes' : 'no'}</div>
              <div>Acknowledged: {c.ack ? `${c.ack.by} ${dateTime(c.ack.at)}` : 'not yet'}</div>
              <div>Review state: {c.review}</div>
            </div>
            <div className="mt-2 flex flex-wrap items-end gap-2">
              <div>
                <label className="label" htmlFor={`cutoff-${id}`}>Evidence cutoff</label>
                <input id={`cutoff-${id}`} type="datetime-local" className="input" value={cutoff} onChange={(e) => setCutoff(e.target.value)} />
              </div>
              <Busy ghost onClick={load} title="Reassemble with this cutoff"><span className="flex items-center gap-1"><RefreshCw size={14} aria-hidden /> Refresh</span></Busy>
            </div>
          </section>

          {/* source inventory */}
          <Panel title="Source inventory">
            <ul className="space-y-0.5">
              {report.sources.map((s, i) => (
                <li key={i} className="flex items-start gap-2">
                  <span className={cx('chip shrink-0', s.present ? 'bg-emerald-500/20 text-emerald-200' : 'bg-slate-600/30 text-slate-300')}>{s.present ? 'present' : 'absent'}</span>
                  <span><b>{s.name}</b>{s.note ? <span className="text-slate-400"> — {s.note}</span> : null}</span>
                </li>
              ))}
            </ul>
          </Panel>

          {/* milestones */}
          <Panel title="Call-handling milestones">
            <table className="w-full text-xs">
              <thead><tr className="text-left text-slate-500"><th className="px-2 py-1">Milestone</th><th className="px-2 py-1">Time</th><th className="px-2 py-1">Source</th><th className="px-2 py-1">Note</th></tr></thead>
              <tbody>
                {report.milestones.map((m, i) => (
                  <tr key={i} className="border-t border-slate-800">
                    <td className="px-2 py-1 font-mono">{m.name}</td>
                    <td className="px-2 py-1">{timeOnly(m.at)}</td>
                    <td className="px-2 py-1">{m.source}{m.seq ? ` #${m.seq}` : ''}</td>
                    <td className="px-2 py-1 text-slate-400">{m.note || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </Panel>

          {/* units */}
          <Panel title="Units">
            {(report.units ?? []).length === 0 ? <Empty>No units.</Empty> : (
              <table className="w-full text-xs">
                <thead><tr className="text-left text-slate-500"><th className="px-2 py-1">Unit</th><th className="px-2 py-1">Status times</th></tr></thead>
                <tbody>
                  {(report.units ?? []).map((u) => (
                    <tr key={u.unit} className="border-t border-slate-800">
                      <td className="px-2 py-1 font-mono">{u.unit}{u.recommended && <span className="text-slate-500"> (recommended)</span>}</td>
                      <td className="px-2 py-1">{Object.entries(u.times).map(([k, t]) => `${k} ${timeOnly(t)}`).join(', ') || '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Panel>

          {/* tagged radio */}
          <Panel title="Tagged radio traffic">
            <p className="mb-1 text-xs text-slate-500">Machine tags are aids to review, not findings. A negated tag is shown so it is not mistaken for the real thing.</p>
            {(report.radio ?? []).length === 0 ? <Empty>No tagged radio traffic.</Empty> : (
              <ul className="space-y-0.5 text-xs">
                {(report.radio ?? []).map((t, i) => (
                  <li key={i}>
                    <span className="text-slate-500">{timeOnly(t.at)}</span> <b>{t.unit || '—'}</b> [{t.name}{t.negated ? ' NEGATED' : ''}] &ldquo;{t.text}&rdquo;
                  </li>
                ))}
              </ul>
            )}
          </Panel>

          {/* requests */}
          <Panel title="Requests and inquiries (metadata only)">
            {(report.requests ?? []).length === 0 ? <Empty>No requests.</Empty> : (
              <ul className="space-y-0.5 text-xs">
                {(report.requests ?? []).map((q) => (
                  <li key={q.id}><span className="font-mono">{q.id}</span> {q.type} {q.category} — requested {dateTime(q.requestedAt)}, owner {q.owner || '—'}, {q.status}</li>
                ))}
              </ul>
            )}
          </Panel>

          {/* QA matrix */}
          <Panel title={`QA matrix — ${c.template.name}${c.template.policy ? ` (${c.template.policy})` : ''}`}>
            <div className="space-y-3">
              {report.qa.map((row) => (
                <QAMatrixRow key={row.measure.id} caseId={id} row={row} results={results} onSaved={() => { void load(); onSaved() }} />
              ))}
            </div>
          </Panel>

          {/* limitations */}
          <Panel title="Evidence limitations">
            {(report.limitations ?? []).length === 0 ? <Empty>None noted.</Empty> : (
              <ul className="list-disc space-y-0.5 pl-5 text-xs text-slate-300">
                {(report.limitations ?? []).map((l, i) => <li key={i}>{l}</li>)}
              </ul>
            )}
          </Panel>

          {/* narrative and findings */}
          <Panel title="Supervisor narrative and findings">
            <div className="space-y-2">
              <NoteField id={`narr-${id}`} label="Supervisor narrative (observation)" value={narrative} onChange={setNarrative} rows={3} />
              <NoteField id={`find-${id}`} label="Findings (human)" value={findings} onChange={setFindings} rows={3} />
              <Busy go onClick={() => run(() => api.post(`/critical/${id}`, { narrative, findings }), () => { void load(); onSaved() })}>Save narrative and findings</Busy>
            </div>
          </Panel>

          {/* actions */}
          <Actions caseId={id} caseData={c} onSaved={() => { void load(); onSaved() }} />

          {/* issue and copies */}
          <IssueAndCopies id={id} cutoff={cutoff} downloadHref={downloadHref} reports={c.reports ?? []} onSaved={() => { void load(); onSaved() }} onDerived={(view, data) => setDerived({ view, data })} />
        </div>
      )}

      {derived && (
        <Modal open onClose={() => setDerived(null)} title={derived.view === 'foia' ? 'Records-request copy' : 'Accreditation copy'} icon={<FileText size={16} aria-hidden />} wide>
          <div className="rounded-md border border-amber-700/50 bg-amber-950/30 p-2 text-sm text-amber-200">
            The source report is not changed. A person verifies this derived copy before it is released.
          </div>
          <pre className="mt-2 max-h-[40vh] overflow-auto whitespace-pre-wrap rounded-md border border-slate-800 bg-slate-950/60 p-2 text-xs">{derived.data.text}</pre>
          <h4 className="mt-3 text-sm font-semibold text-slate-200">Redaction log</h4>
          {(derived.data.redactions ?? []).length === 0 ? (
            <Empty>No redactions were proposed.</Empty>
          ) : (
            <ul className="list-disc space-y-0.5 pl-5 text-xs text-slate-300">
              {(derived.data.redactions ?? []).map((l, i) => <li key={i}>{l}</li>)}
            </ul>
          )}
          <p className="mt-2 text-xs text-slate-500">Source report hash: <span className="font-mono">{derived.data.source}</span></p>
        </Modal>
      )}
    </Modal>
  )
}

function QAMatrixRow({ caseId, row, results, onSaved }: { caseId: string; row: QARow; results: string[]; onSaved: () => void }) {
  const m = row.measure
  const [result, setResult] = useState(row.answer?.result ?? '')
  const [comment, setComment] = useState(row.answer?.comment ?? '')
  const [citation, setCitation] = useState(row.answer?.citation ?? '')
  const [reason, setReason] = useState(row.answer?.reason ?? '')

  const save = () => {
    const body: Record<string, unknown> = { measure: m.id, result, comment, citation }
    if (result === 'na') body.reason = reason
    return run(() => api.post(`/critical/${caseId}/qa`, body), onSaved)
  }

  return (
    <div className="rounded-md border border-slate-800 p-2">
      <div className="flex flex-wrap items-center gap-2">
        <b className="text-slate-100">{m.name}</b>
        {m.scored && <Tag>scored</Tag>}
        {m.specialist && <Tag tone="warn">QA specialist or supervisor</Tag>}
      </div>
      {row.seconds !== undefined && (
        <div className="text-xs text-slate-400">
          Calculated {seconds(row.seconds)} from #{row.fromSeq} to #{row.toSeq}
          {row.within !== undefined && (
            <span className={cx('ml-1', row.within ? 'text-emerald-300' : 'text-rose-300')}>
              ({row.within ? 'within' : 'over'} the {m.standardSec} s standard)
            </span>
          )}
        </div>
      )}
      {(row.evidence ?? []).length > 0 && (
        <ul className="mt-1 space-y-0.5 text-xs text-slate-400">
          {(row.evidence ?? []).map((e, i) => <li key={i}>evidence: {e}</li>)}
        </ul>
      )}
      {row.prompt && <div className="mt-1 text-xs text-amber-300">prompt: {row.prompt}</div>}

      <div className="mt-2 grid items-end gap-2 sm:grid-cols-2">
        <SelectField id={`qa-res-${caseId}-${m.id}`} label="Result" value={result} onChange={setResult} options={results} />
        {result === 'na' && (m.naReasons ?? []).length > 0 && (
          <SelectField id={`qa-reason-${caseId}-${m.id}`} label="Not-applicable reason" value={reason} onChange={setReason} options={m.naReasons ?? []} />
        )}
        <TextField id={`qa-cite-${caseId}-${m.id}`} label="Citation (required unless n/a)" value={citation} onChange={setCitation} placeholder="event, audio time or transcript segment" />
        <div className="sm:col-span-2"><NoteField id={`qa-com-${caseId}-${m.id}`} label="Comment (required unless met)" value={comment} onChange={setComment} rows={1} /></div>
      </div>
      <div className="mt-2">
        <Busy onClick={save} disabled={!result || (result !== 'na' && !citation.trim()) || (result !== 'met' && !comment.trim())}>Save answer</Busy>
        {row.answer && <span className="ml-2 text-xs text-slate-500">last answer: {row.answer.result} by {row.answer.by}</span>}
      </div>
    </div>
  )
}

function Actions({ caseId, caseData, onSaved }: { caseId: string; caseData: CriticalCase; onSaved: () => void }) {
  const [action, setAction] = useState('')
  const [support, setSupport] = useState('')
  const [after, setAfter] = useState('')
  const noClinical = 'No clinical or wellness detail goes in CAD. This records only that an operational action was taken.'
  return (
    <Panel title="Actions">
      {(caseData.actions ?? []).length > 0 && (
        <ul className="mb-2 space-y-0.5 text-xs text-slate-400">
          {(caseData.actions ?? []).map((a, i) => <li key={i}>{dateTime(a.at)} · {a.by} · {a.what}: {a.note}</li>)}
        </ul>
      )}
      <div className="space-y-3">
        <div>
          <NoteField id={`act-${caseId}`} label="Add a corrective or follow-up action" value={action} onChange={setAction} rows={1} />
          <div className="mt-1"><Busy onClick={() => run(() => api.post(`/critical/${caseId}`, { action }), () => { setAction(''); onSaved() })} disabled={!action.trim()}>Add action</Busy></div>
        </div>
        <div>
          <NoteField id={`sup-${caseId}`} label="Offer support (RESET): a short operational note" value={support} onChange={setSupport} rows={1} />
          <p className="mt-1 text-xs text-slate-500">{noClinical}</p>
          <div className="mt-1"><Busy onClick={() => run(() => api.post(`/critical/${caseId}`, { support }), () => { setSupport(''); onSaved() })} disabled={!support.trim()}>Offer support</Busy></div>
        </div>
        <div>
          <NoteField id={`aa-${caseId}`} label="Open after-action (ARC): a short operational note" value={after} onChange={setAfter} rows={1} />
          <p className="mt-1 text-xs text-slate-500">{noClinical}</p>
          <div className="mt-1"><Busy onClick={() => run(() => api.post(`/critical/${caseId}`, { afterAction: after }), () => { setAfter(''); onSaved() })} disabled={!after.trim()}>Open after-action</Busy></div>
        </div>
      </div>
    </Panel>
  )
}

function IssueAndCopies({ id, cutoff, downloadHref, reports, onSaved, onDerived }: {
  id: string
  cutoff: string
  downloadHref: string
  reports: CriticalReport['case']['reports']
  onSaved: () => void
  onDerived: (view: string, data: DerivedView) => void
}) {
  const [note, setNote] = useState('')
  const cutoffQ = cutoff ? `&cutoff=${encodeURIComponent(toRFC3339(cutoff))}` : ''

  const issue = (kind: 'full' | 'foia' | 'accreditation') =>
    run(() => api.post(`/critical/${id}/issue`, { cutoff: cutoff ? toRFC3339(cutoff) : '', note, kind }), () => { setNote(''); onSaved() })

  const openDerived = (view: 'foia' | 'accreditation') => async () => {
    const data = await api.get<DerivedView>(`/critical/${id}/report?view=${view}${cutoffQ}`)
    onDerived(view, data)
  }

  return (
    <Panel title="Issue and copies">
      <div className="flex flex-wrap items-end gap-2">
        <div className="min-w-[14rem] flex-1">
          <NoteField id={`issue-note-${id}`} label="Note for this issued version" value={note} onChange={setNote} rows={1} />
        </div>
        <Busy go onClick={() => issue('full')}>Issue version</Busy>
      </div>
      <div className="mt-2 flex flex-wrap gap-2">
        <Busy ghost onClick={openDerived('foia')}>Records-request copy</Busy>
        <Busy ghost onClick={openDerived('accreditation')}>Accreditation copy</Busy>
        <a className="btn btn-ghost flex items-center gap-1" href={downloadHref} target="_blank" rel="noreferrer"><Download size={14} aria-hidden /> Download text</a>
      </div>
      <h4 className="mt-3 text-sm font-semibold text-slate-200">Issued versions</h4>
      {(reports ?? []).length === 0 ? (
        <Empty>No versions issued yet.</Empty>
      ) : (
        <ul className="space-y-1 text-xs">
          {(reports ?? []).map((v) => (
            <li key={v.version} className="border-b border-slate-800/60 pb-1">
              <b>v{v.version}</b> {v.kind} by {v.by} · {dateTime(v.at)} · cutoff {dateTime(v.cutoff)}{v.note ? ` · ${v.note}` : ''}
              <div className="font-mono text-slate-500">sha256 {v.hash}</div>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  )
}
