import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import { ArrowLeft, CheckCircle2, Download, FilePlus2, Layers, MessageSquarePlus, ShieldCheck } from 'lucide-react'
import { api, ApiError, errText } from '../api'
import type { FOIADecision, FOIAItem, FOIAProposal, FOIARequest } from '../types'
import { Modal } from '../ui'
import { cx } from '../util'

const STATUSES = ['received', 'in-review', 'awaiting-approval', 'denied', 'closed'] as const
const STATUS_CLS: Record<string, string> = {
  received: 'bg-slate-700 text-slate-200',
  'in-review': 'bg-sky-500/20 text-sky-300',
  'awaiting-approval': 'bg-violet-500/20 text-violet-300',
  released: 'bg-emerald-500/20 text-emerald-300',
  denied: 'bg-rose-500/20 text-rose-300',
  closed: 'bg-slate-600/40 text-slate-300',
}

// workingDaysBetween counts weekdays from a to b (negative when b is before a).
function workingDaysBetween(from: number, to: number): number {
  const sign = to < from ? -1 : 1
  let a = new Date(Math.min(from, to))
  const b = new Date(Math.max(from, to))
  a = new Date(a.getFullYear(), a.getMonth(), a.getDate())
  const end = new Date(b.getFullYear(), b.getMonth(), b.getDate())
  let days = 0
  while (a < end) {
    a.setDate(a.getDate() + 1)
    const d = a.getDay()
    if (d !== 0 && d !== 6) days++
  }
  return sign * days
}

// dueBadge: working days left until the due date, coloured by urgency.
function dueBadge(due: string, now: number): { text: string; cls: string } {
  const left = workingDaysBetween(now, new Date(due).getTime())
  if (left < 0) return { text: `${-left}d overdue`, cls: 'bg-rose-500/20 text-rose-300' }
  if (left <= 1) return { text: left === 0 ? 'due today' : 'due tomorrow', cls: 'bg-amber-500/20 text-amber-300' }
  return { text: `${left}d left`, cls: 'bg-slate-700 text-slate-300' }
}

// elapsed: a request's working time so far — from started (or received) to released (or now).
function elapsed(r: FOIARequest, now: number): string {
  const start = new Date(r.started ?? r.received).getTime()
  const end = r.released ? new Date(r.released).getTime() : now
  const hours = Math.max(0, (end - start) / 3_600_000)
  if (hours < 48) return `${Math.round(hours)}h`
  return `${(hours / 24).toFixed(1)}d`
}

/** Records requests (FOIA): the queue, and the request being worked. */
export default function Records() {
  const { id } = useParams()
  const nav = useNavigate()
  const [list, setList] = useState<FOIARequest[]>([])
  const [filter, setFilter] = useState('')
  const [forbidden, setForbidden] = useState(false)
  const [creating, setCreating] = useState(false)

  const load = () =>
    api
      .get<FOIARequest[]>('/foia/requests')
      .then((r) => {
        setList(r)
        setForbidden(false)
      })
      .catch((e) => {
        if (e instanceof ApiError && e.status === 403) setForbidden(true)
        else toast.error(errText(e))
      })
  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const now = Date.now()
  const shown = list.filter((r) => !filter || r.status === filter)

  if (forbidden) {
    return (
      <div className="grid h-full place-items-center bg-slate-950 p-6 text-center">
        <div className="max-w-sm space-y-3">
          <p className="text-sm text-slate-300">Records work needs the records duty, a supervisor or an administrator.</p>
          <Link to="/" className="btn">Back to dispatch</Link>
        </div>
      </div>
    )
  }

  return (
    <div className="flex h-full bg-slate-950">
      <nav className="flex w-96 shrink-0 flex-col border-r border-slate-800 bg-slate-900" aria-label="Records requests">
        <div className="flex items-center gap-2 border-b border-slate-800 p-2">
          <Link to="/" className="flex items-center gap-2 rounded px-2 py-1 text-sm font-semibold text-slate-200 hover:bg-slate-800">
            <ArrowLeft size={14} aria-hidden /> Dispatch
          </Link>
          <span className="text-sm font-semibold text-slate-100">Records requests</span>
          <button className="btn btn-go ml-auto h-7 text-xs" onClick={() => setCreating(true)}>
            <FilePlus2 size={13} aria-hidden /> New request
          </button>
        </div>
        <Stats list={list} now={now} />
        <div className="flex flex-wrap gap-1 px-2 pb-2 text-[11px]">
          {['', ...STATUSES, 'released'].map((f) => (
            <button
              key={f}
              className={cx('rounded px-1.5 py-0.5', filter === f ? 'bg-slate-700 text-slate-50' : 'text-slate-400 hover:bg-slate-800')}
              onClick={() => setFilter(f)}
            >
              {f || 'all'} <span className="font-mono text-slate-500">{f ? list.filter((r) => r.status === f).length : list.length}</span>
            </button>
          ))}
        </div>
        <ul className="min-h-0 flex-1 overflow-y-auto">
          {shown.length === 0 && <li className="px-3 py-2 text-sm text-slate-500">No requests.</li>}
          {shown.map((r) => {
            const b = dueBadge(r.due, now)
            return (
              <li key={r.id}>
                <Link
                  to={`/records/${r.id}`}
                  className={cx('block border-b border-slate-800/60 px-3 py-2 text-sm hover:bg-slate-800/60', r.id === id && 'bg-slate-800')}
                >
                  <div className="flex items-center gap-2">
                    <span className="truncate font-semibold text-slate-100">{r.requester}</span>
                    <span className={cx('chip normal-case', STATUS_CLS[r.status])}>{r.status}</span>
                  </div>
                  <div className="mt-0.5 flex items-center gap-2 text-xs text-slate-400">
                    <span>{r.incidents.length} incident{r.incidents.length === 1 ? '' : 's'}</span>
                    <span>·</span>
                    <span>{elapsed(r, now)} taken</span>
                    {r.status !== 'released' && r.status !== 'closed' && r.status !== 'denied' && (
                      <span className={cx('chip normal-case', b.cls)}>{b.text}</span>
                    )}
                  </div>
                  <div className="text-[10px] text-slate-500">received {new Date(r.received).toLocaleDateString()}</div>
                </Link>
              </li>
            )
          })}
        </ul>
      </nav>
      <main className="min-w-0 flex-1 overflow-y-auto p-4">
        {id ? <Detail key={id} id={id} onChanged={load} /> : <div className="grid h-full place-items-center text-sm text-slate-500">Choose a request, or start a new one.</div>}
      </main>
      {creating && (
        <NewRequest
          onClose={() => setCreating(false)}
          onCreated={async (newId) => {
            setCreating(false)
            await load()
            nav(`/records/${newId}`)
          }}
        />
      )}
    </div>
  )
}

function Stats({ list, now }: { list: FOIARequest[]; now: number }) {
  const weekAgo = now - 7 * 86_400_000
  const thisWeek = list.filter((r) => new Date(r.received).getTime() >= weekAgo).length
  const released = list.filter((r) => r.status === 'released').length
  const overdue = list.filter((r) => r.status !== 'released' && r.status !== 'closed' && r.status !== 'denied' && new Date(r.due).getTime() < now).length
  const done = list.filter((r) => r.released)
  const avgH = done.length
    ? done.reduce((s, r) => s + (new Date(r.released!).getTime() - new Date(r.started ?? r.received).getTime()) / 3_600_000, 0) / done.length
    : 0
  const avg = done.length ? (avgH < 48 ? `${Math.round(avgH)}h` : `${(avgH / 24).toFixed(1)}d`) : '–'
  return (
    <dl className="flex flex-wrap gap-x-4 gap-y-1 border-b border-slate-800 px-3 py-2 text-[11px] text-slate-400">
      <div><dt className="inline">This week </dt><dd className="inline font-mono font-bold text-slate-200">{thisWeek}</dd></div>
      <div><dt className="inline">Released </dt><dd className="inline font-mono font-bold text-emerald-300">{released}</dd></div>
      <div><dt className="inline">Overdue </dt><dd className={cx('inline font-mono font-bold', overdue ? 'text-rose-300' : 'text-slate-200')}>{overdue}</dd></div>
      <div><dt className="inline">Avg time </dt><dd className="inline font-mono font-bold text-slate-200">{avg}</dd></div>
    </dl>
  )
}

function NewRequest({ onClose, onCreated }: { onClose: () => void; onCreated: (id: string) => void }) {
  const [requester, setRequester] = useState('')
  const [contact, setContact] = useState('')
  const [description, setDescription] = useState('')
  const [incidents, setIncidents] = useState('')
  const [view, setView] = useState<'summary' | 'detailed'>('summary')
  const [busy, setBusy] = useState(false)
  const submit = async () => {
    const nums = incidents.split(/[\s,]+/).map((x) => x.trim()).filter(Boolean)
    setBusy(true)
    try {
      const r = await api.post<{ id: string }>('/foia/requests', { requester: requester.trim(), contact: contact.trim(), description: description.trim(), incidents: nums, view })
      toast.success('Request logged')
      onCreated(r.id)
    } catch (e) {
      toast.error(e instanceof ApiError && e.status === 403 ? 'Records work needs the records duty, a supervisor or an administrator.' : errText(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal
      open
      onClose={onClose}
      title="New records request"
      icon={<FilePlus2 size={16} aria-hidden />}
      footer={
        <>
          <button className="btn" onClick={onClose}>Cancel</button>
          <button className="btn btn-go" disabled={busy || !requester.trim()} onClick={submit}>Log request</button>
        </>
      }
    >
      <div className="space-y-3">
        <label className="block"><span className="label">Requester</span><input className="input" value={requester} onChange={(e) => setRequester(e.target.value)} autoFocus /></label>
        <label className="block"><span className="label">Contact</span><input className="input" value={contact} onChange={(e) => setContact(e.target.value)} placeholder="email or phone" /></label>
        <label className="block"><span className="label">Description</span><textarea className="input min-h-20" value={description} onChange={(e) => setDescription(e.target.value)} placeholder="What records are asked for." /></label>
        <label className="block"><span className="label">Incidents</span><input className="input font-mono" value={incidents} onChange={(e) => setIncidents(e.target.value)} placeholder="25-00123 25-00124" /><span className="mt-1 block text-[11px] text-slate-500">Incident numbers, separated by commas or spaces.</span></label>
        <label className="block"><span className="label">View to release</span>
          <select className="input" value={view} onChange={(e) => setView(e.target.value as 'summary' | 'detailed')}>
            <option value="summary">Summary</option>
            <option value="detailed">Detailed</option>
          </select>
        </label>
      </div>
    </Modal>
  )
}

function Detail({ id, onChanged }: { id: string; onChanged: () => void }) {
  const [r, setR] = useState<FOIARequest | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const [approving, setApproving] = useState(false)
  const [leaks, setLeaks] = useState<string[] | null>(null)

  const reload = () =>
    api
      .get<FOIARequest>(`/foia/requests/${id}`)
      .then((x) => {
        setR(x)
        setError(null)
      })
      .catch((e) => {
        setR(null)
        setError(e instanceof ApiError && e.status === 403 ? 'Records work needs the records duty, a supervisor or an administrator.' : errText(e))
      })
  useEffect(() => {
    reload()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id])

  const save = async (patch: Record<string, unknown>) => {
    setBusy(true)
    try {
      await api.put(`/foia/requests/${id}`, patch)
      await reload()
      onChanged()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }

  const assemble = async () => {
    setBusy(true)
    try {
      await api.post(`/foia/requests/${id}/assemble`)
      await reload()
      onChanged()
      toast.success('Assembled a release copy')
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }

  const approve = async () => {
    setBusy(true)
    setLeaks(null)
    try {
      await api.post(`/foia/requests/${id}/approve`)
      setApproving(false)
      await reload()
      onChanged()
      toast.success('Released')
    } catch (e) {
      if (e instanceof ApiError && e.status === 422 && Array.isArray(e.data?.leaks)) {
        setLeaks((e.data!.leaks as string[]).map(String))
      } else {
        toast.error(errText(e))
        setApproving(false)
      }
    } finally {
      setBusy(false)
    }
  }

  if (error) return <p className="rounded border border-rose-800 bg-rose-950/30 p-3 text-sm text-rose-200">{error}</p>
  if (!r) return <p className="text-sm text-slate-500">Loading…</p>

  const released = r.status === 'released'
  const editable = !released
  return (
    <div className="mx-auto max-w-5xl space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-xl font-bold text-slate-50">{r.requester}</h1>
        <span className={cx('chip normal-case', STATUS_CLS[r.status])}>{r.status}</span>
        {r.approvedBy && <span className="text-xs text-slate-500">released by {r.approvedBy}</span>}
        <Link to={`/review/${encodeURIComponent(r.incidents[0] ?? '')}`} className="ml-auto text-xs text-sky-300 underline print:hidden" aria-disabled={!r.incidents.length}>
          Review the record
        </Link>
      </div>

      <section className="panel space-y-3 rounded-lg p-3">
        <h2 className="text-[11px] font-bold uppercase tracking-wider text-slate-400">Request</h2>
        {r.description && <p className="text-sm text-slate-200">{r.description}</p>}
        <div className="grid gap-3 md:grid-cols-4">
          <label className="block"><span className="label">Status</span>
            <select className="input" value={r.status === 'released' ? 'released' : r.status} disabled={released} onChange={(e) => save({ status: e.target.value })}>
              {released && <option value="released">released</option>}
              {STATUSES.map((s) => <option key={s} value={s}>{s}</option>)}
            </select>
          </label>
          <label className="block"><span className="label">Assigned</span><input className="input" defaultValue={r.assigned ?? ''} disabled={!editable} onBlur={(e) => e.target.value !== (r.assigned ?? '') && save({ assigned: e.target.value })} /></label>
          <label className="block"><span className="label">View</span>
            <select className="input" value={r.view} disabled={!editable} onChange={(e) => save({ view: e.target.value })}>
              <option value="summary">summary</option>
              <option value="detailed">detailed</option>
            </select>
          </label>
          <label className="block"><span className="label">Incidents</span><input className="input font-mono" defaultValue={r.incidents.join(' ')} disabled={!editable} onBlur={(e) => {
            const nums = e.target.value.split(/[\s,]+/).map((x) => x.trim()).filter(Boolean)
            if (nums.join(' ') !== r.incidents.join(' ')) save({ incidents: nums })
          }} /></label>
        </div>
        <div className="flex flex-wrap items-center gap-3 text-xs text-slate-400">
          <span>received {new Date(r.received).toLocaleString()}</span>
          <span>due {new Date(r.due).toLocaleDateString()}</span>
          {r.contact && <span>contact {r.contact}</span>}
        </div>
      </section>

      <section className="panel space-y-2 rounded-lg p-3 print:hidden">
        <div className="flex items-center gap-2">
          <h2 className="text-[11px] font-bold uppercase tracking-wider text-slate-400">Release copy</h2>
          {editable && <button className="btn ml-auto h-7 text-xs" disabled={busy} onClick={assemble}><Layers size={13} aria-hidden /> {r.items?.length ? 'Re-assemble' : 'Assemble'}</button>}
        </div>
        <p className="text-xs text-slate-500">
          Builds the release copy from the chosen view and proposes redactions by your agency's disclosure rules. The original record is untouched.
        </p>
      </section>

      {(r.items ?? []).map((item) => (
        <ItemPanel key={item.incident} id={id} item={item} editable={editable} onSaved={async () => { await reload(); onChanged() }} />
      ))}

      {!released && (r.items?.length ?? 0) > 0 && (
        <div className="flex items-center gap-2">
          <button className="btn btn-go" disabled={busy} onClick={() => { setLeaks(null); setApproving(true) }}>
            <ShieldCheck size={14} aria-hidden /> Approve and release
          </button>
          {leaks && (
            <span className="text-xs text-rose-300">A leak check stopped the release — see below.</span>
          )}
        </div>
      )}

      {leaks && (
        <section className="panel space-y-1 rounded-lg border-rose-800 p-3">
          <h2 className="text-[11px] font-bold uppercase tracking-wider text-rose-300">Leak check failed</h2>
          <p className="text-xs text-slate-400">These passages would still be exposed. Mark or accept them, then approve again.</p>
          <ul className="list-disc space-y-0.5 pl-5 text-sm text-rose-200">{leaks.map((l, i) => <li key={i} className="font-mono">{l}</li>)}</ul>
        </section>
      )}

      <Notes r={r} note={note} setNote={setNote} onAdd={() => save({ note }).then(() => setNote(''))} busy={busy} />

      <Modal
        open={approving}
        onClose={() => setApproving(false)}
        title="Approve and release"
        icon={<ShieldCheck size={16} aria-hidden />}
        footer={
          <>
            <button className="btn" onClick={() => setApproving(false)}>Cancel</button>
            <button className="btn btn-go" disabled={busy} onClick={approve}>Authorise release</button>
          </>
        }
      >
        <p className="text-sm text-slate-200">A person authorises the release. Accepted redactions are applied permanently and the copy is checked for leaks.</p>
      </Modal>
    </div>
  )
}

function Notes({ r, note, setNote, onAdd, busy }: { r: FOIARequest; note: string; setNote: (v: string) => void; onAdd: () => void; busy: boolean }) {
  return (
    <section className="panel space-y-2 rounded-lg p-3">
      <h2 className="text-[11px] font-bold uppercase tracking-wider text-slate-400">Notes</h2>
      <ul className="space-y-1 text-sm">
        {(r.notes ?? []).length === 0 && <li className="text-slate-500">No notes yet.</li>}
        {(r.notes ?? []).map((n) => (
          <li key={n.seq} className="flex gap-2">
            <span className="shrink-0 font-mono text-[11px] text-slate-500">{new Date(n.at).toLocaleString()}</span>
            <span className="text-slate-300"><span className="text-slate-400">{n.actor}:</span> {n.text}</span>
          </li>
        ))}
      </ul>
      <div className="flex gap-2 print:hidden">
        <input className="input" placeholder="Add a note" value={note} onChange={(e) => setNote(e.target.value)} onKeyDown={(e) => e.key === 'Enter' && note.trim() && onAdd()} />
        <button className="btn" disabled={busy || !note.trim()} onClick={onAdd}><MessageSquarePlus size={13} aria-hidden /> Add</button>
      </div>
    </section>
  )
}

// Merge overlapping/touching ranges, then apply them as [REDACTED: basis].
function applyRedactions(text: string, decisions: FOIADecision[]): string {
  const ranges = decisions
    .filter((d) => d.accept)
    .map((d) => ({ start: d.start, end: d.end, basis: d.basis }))
    .sort((a, b) => a.start - b.start)
  const merged: { start: number; end: number; basis: string }[] = []
  for (const rg of ranges) {
    const last = merged[merged.length - 1]
    if (last && rg.start <= last.end) {
      last.end = Math.max(last.end, rg.end)
      if (rg.basis && !last.basis.includes(rg.basis)) last.basis = last.basis ? `${last.basis}; ${rg.basis}` : rg.basis
    } else {
      merged.push({ ...rg })
    }
  }
  let out = ''
  let at = 0
  for (const rg of merged) {
    out += text.slice(at, rg.start)
    out += `[REDACTED: ${rg.basis || 'withheld'}]`
    at = rg.end
  }
  out += text.slice(at)
  return out
}

type Draft = { accept: boolean; basis: string; note: string }

function ItemPanel({ id, item, editable, onSaved }: { id: string; item: FOIAItem; editable: boolean; onSaved: () => void }) {
  // Decisions keyed by proposal id. Seed from any saved decisions, else from the proposal defaults.
  const seed = useMemo(() => {
    const m = new Map<string, Draft>()
    for (const p of item.proposals) {
      const prior = item.decisions.find((d) => d.id === p.id)
      m.set(p.id, { accept: prior ? prior.accept : false, basis: prior?.basis ?? p.basis, note: prior?.note ?? '' })
    }
    return m
  }, [item])
  const [drafts, setDrafts] = useState<Map<string, Draft>>(seed)
  // Manual passages the reviewer marked, each its own decision.
  const manualSeed = useMemo(
    () => item.decisions.filter((d) => d.id.startsWith('manual-')).map((d) => ({ id: d.id, start: d.start, end: d.end, text: d.text, basis: d.basis })),
    [item],
  )
  const [manual, setManual] = useState(manualSeed)
  const [saving, setSaving] = useState(false)
  const preRef = useRef<HTMLPreElement>(null)
  const done = !!item.released
  const locked = !editable || done

  useEffect(() => {
    setDrafts(seed)
    setManual(manualSeed)
  }, [seed, manualSeed])

  const setDraft = (pid: string, p: Partial<Draft>) =>
    setDrafts((prev) => {
      const next = new Map(prev)
      next.set(pid, { ...next.get(pid)!, ...p })
      return next
    })

  // Build the list of decisions to preview/save: every proposal (accepted or rejected) plus manual marks.
  const decisions: FOIADecision[] = useMemo(() => {
    const out: FOIADecision[] = item.proposals.map((p) => {
      const d = drafts.get(p.id)!
      return { id: p.id, start: p.start, end: p.end, text: p.text, accept: d.accept, basis: d.basis, note: d.note || undefined }
    })
    for (const m of manual) out.push({ id: m.id, start: m.start, end: m.end, text: m.text, accept: true, basis: m.basis })
    return out
  }, [item.proposals, drafts, manual])

  const preview = useMemo(() => applyRedactions(item.text, decisions), [item.text, decisions])

  // Offset of a selection endpoint relative to item.text, by walking the text nodes inside the <pre>.
  const offsetOf = (root: Node, node: Node, offset: number): number => {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
    let total = 0
    let n: Node | null = walker.nextNode()
    while (n) {
      if (n === node) return total + offset
      total += (n.textContent ?? '').length
      n = walker.nextNode()
    }
    return total
  }

  const markPassage = () => {
    const sel = window.getSelection()
    const pre = preRef.current
    if (!sel || sel.rangeCount === 0 || !pre) return
    const range = sel.getRangeAt(0)
    if (!pre.contains(range.startContainer) || !pre.contains(range.endContainer)) {
      toast.error('Select text inside the record first.')
      return
    }
    let start = offsetOf(pre, range.startContainer, range.startOffset)
    let end = offsetOf(pre, range.endContainer, range.endOffset)
    if (start > end) [start, end] = [end, start]
    if (start === end) {
      toast.error('Select a passage first.')
      return
    }
    const text = item.text.slice(start, end)
    const basis = window.prompt(`Basis for withholding “${text.slice(0, 40)}${text.length > 40 ? '…' : ''}”`)?.trim()
    if (!basis) return
    setManual((prev) => [...prev.filter((m) => !(m.start === start && m.end === end)), { id: `manual-${start}-${end}`, start, end, text, basis }])
    sel.removeAllRanges()
  }

  // Highlight accepted/proposed ranges inline. Build segments across the text.
  const segments = useMemo(() => {
    type Seg = { text: string; mark?: FOIAProposal | 'manual' }
    const marks = [
      ...item.proposals.map((p) => ({ start: p.start, end: p.end, p })),
      ...manual.map((m) => ({ start: m.start, end: m.end, p: 'manual' as const })),
    ].sort((a, b) => a.start - b.start)
    const segs: Seg[] = []
    let at = 0
    for (const m of marks) {
      const s = Math.max(m.start, at)
      if (s > at) segs.push({ text: item.text.slice(at, s) })
      if (m.end > s) segs.push({ text: item.text.slice(s, m.end), mark: m.p })
      at = Math.max(at, m.end)
    }
    if (at < item.text.length) segs.push({ text: item.text.slice(at) })
    return segs
  }, [item.text, item.proposals, manual])

  const missingBasis = item.proposals.some((p) => drafts.get(p.id)!.accept && !drafts.get(p.id)!.basis.trim())

  const saveDecisions = async () => {
    if (missingBasis) {
      toast.error('Accepted redactions need a basis.')
      return
    }
    setSaving(true)
    try {
      await api.post(`/foia/requests/${id}/decide`, { incident: item.incident, decisions })
      toast.success('Decisions saved')
      onSaved()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <section className="panel space-y-3 rounded-lg p-3">
      <div className="flex items-center gap-2">
        <h2 className="font-mono text-sm font-bold text-slate-100">{item.incident}</h2>
        {item.released && <span className="chip bg-emerald-500/20 text-emerald-300 normal-case">released</span>}
        <span className="ml-auto text-[11px] text-slate-500">assembled {new Date(item.assembled).toLocaleString()}</span>
      </div>

      <div className="grid gap-3 lg:grid-cols-[1fr_20rem]">
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <span className="text-[11px] font-bold uppercase tracking-wider text-slate-400">Record</span>
            {!locked && <button className="btn h-6 px-1.5 text-[11px]" onClick={markPassage}>Mark a passage</button>}
          </div>
          <pre
            ref={preRef}
            className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded border border-slate-800 bg-slate-950 p-2 font-mono text-[12px] leading-relaxed text-slate-200"
          >
            {segments.map((s, i) =>
              s.mark ? (
                <mark
                  key={i}
                  className={cx(
                    'rounded px-0.5',
                    s.mark === 'manual'
                      ? 'bg-sky-500/30 text-sky-100'
                      : s.mark.confidence === 'high'
                        ? 'bg-rose-500/30 text-rose-100'
                        : 'bg-amber-500/30 text-amber-100',
                  )}
                  title={s.mark === 'manual' ? 'Marked by a reviewer' : `${s.mark.basis}${s.mark.source === 'model' ? ' (model)' : ''}`}
                >
                  {s.text}
                </mark>
              ) : (
                <span key={i}>{s.text}</span>
              ),
            )}
          </pre>

          <div className="space-y-1">
            <span className="text-[11px] font-bold uppercase tracking-wider text-slate-400">Redacted preview</span>
            <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded border border-slate-800 bg-slate-950 p-2 font-mono text-[12px] leading-relaxed text-slate-300">{preview}</pre>
          </div>

          {item.released && (
            <div className="flex items-center gap-3 text-xs">
              <a className="btn h-7 text-xs" href={`./api/foia/requests/${id}/released/${encodeURIComponent(item.incident)}`} download>
                <Download size={13} aria-hidden /> Download released copy
              </a>
              {item.hash && <span className="font-mono text-slate-500">hash {item.hash}</span>}
            </div>
          )}
        </div>

        <aside className="space-y-2">
          <span className="text-[11px] font-bold uppercase tracking-wider text-slate-400">Proposals</span>
          {item.proposals.length === 0 && manual.length === 0 && <p className="text-xs text-slate-500">No proposals. Mark passages by hand, or re-assemble.</p>}
          <ul className="space-y-2">
            {item.proposals.map((p) => {
              const d = drafts.get(p.id)!
              return (
                <li key={p.id} className="rounded border border-slate-800 p-2">
                  <div className="flex items-start gap-2">
                    <span className={cx('mt-0.5 chip normal-case', p.confidence === 'high' ? 'bg-rose-500/20 text-rose-300' : 'bg-amber-500/20 text-amber-300')}>{p.confidence}</span>
                    {p.source === 'model' && <span className="chip bg-slate-700 text-slate-300 normal-case">model</span>}
                    <span className="ml-auto flex gap-1">
                      <button type="button" aria-pressed={d.accept} disabled={locked} onClick={() => setDraft(p.id, { accept: true })} className={cx('rounded px-1.5 py-0.5 text-[11px]', d.accept ? 'bg-emerald-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700')}>Accept</button>
                      <button type="button" aria-pressed={!d.accept} disabled={locked} onClick={() => setDraft(p.id, { accept: false })} className={cx('rounded px-1.5 py-0.5 text-[11px]', !d.accept ? 'bg-slate-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700')}>Reject</button>
                    </span>
                  </div>
                  <p className="mt-1 font-mono text-[11px] text-slate-400">“{p.text}”</p>
                  <p className="text-[11px] text-slate-500">{p.explain}</p>
                  <input
                    className="input mt-1 text-xs"
                    aria-label="Basis"
                    aria-invalid={d.accept && !d.basis.trim()}
                    placeholder="basis (required when accepted)"
                    value={d.basis}
                    disabled={locked}
                    onChange={(e) => setDraft(p.id, { basis: e.target.value })}
                  />
                  <input className="input mt-1 text-xs" aria-label="Note" placeholder="note" value={d.note} disabled={locked} onChange={(e) => setDraft(p.id, { note: e.target.value })} />
                </li>
              )
            })}
            {manual.map((m) => (
              <li key={m.id} className="rounded border border-sky-800 bg-sky-950/20 p-2">
                <div className="flex items-start gap-2">
                  <span className="chip bg-sky-500/20 text-sky-300 normal-case">manual</span>
                  {!locked && <button className="btn btn-ghost ml-auto px-1.5 text-[11px]" onClick={() => setManual((prev) => prev.filter((x) => x.id !== m.id))}>Remove</button>}
                </div>
                <p className="mt-1 font-mono text-[11px] text-slate-400">“{m.text}”</p>
                <p className="text-[11px] text-slate-500">{m.basis}</p>
              </li>
            ))}
          </ul>
          {!locked && (
            <button className="btn btn-go w-full" disabled={saving || missingBasis} onClick={saveDecisions}>
              <CheckCircle2 size={13} aria-hidden /> Save decisions
            </button>
          )}
        </aside>
      </div>
    </section>
  )
}
