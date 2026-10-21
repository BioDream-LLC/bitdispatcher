import { lazy, Suspense, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { DndContext, closestCenter, PointerSensor, useSensor, useSensors, useDraggable, useDroppable, type DragEndEvent } from '@dnd-kit/core'
import { FlaskConical, GripVertical, History, Lightbulb, Plus, RotateCcw, Save, Trash2 } from 'lucide-react'
import { api, errText } from '../api'
import { can, useStore } from '../store'
import { cx } from '../util'
import type { AdvisoryStatus, Suggestion } from '../types'
import { Page } from './Admin'
import { AdvisoryCards } from '../console/Advisory'

const ScriptEditor = lazy(() => import('./ScriptEditor'))

type Rule = {
  id: string; kind: string; name: string; enabled: boolean; severity: 'urgent' | 'info'; explain: string
  metres?: number; minutes?: number; days?: number; count?: number; priority?: number; words?: string[]; means?: string; fits?: string[]; suggest?: string
  lang?: string; hook?: string; source?: string; timeoutMs?: number
}
type Codebook = { rules: Rule[]; precedence: string[] }
type Version = { version: number; mode: string; note?: string; by: string; at: string; seq: number; rules?: Codebook }
type Decision = { id: string; rule: string; accepted: boolean; reason?: string; by: string; at: string; ran?: string }
type AdminView = { codebook: Codebook; mode: 'off' | 'shadow' | 'live'; version: number; history: Version[] | null; default: Codebook; decisions: Decision[]; status: AdvisoryStatus; shadow: Suggestion[] }
type TestResult = { events: number; evaluations: number; byRule: Record<string, number>; cannotDetermine: number; sample: Suggestion[]; millis: number; faults?: { rule: string; name: string; message: string }[] }

const KIND: Record<string, string> = {
  duplicate: 'Two open incidents close together', 'call-near': 'An unattached call near an incident', words: 'Words that do not fit the call type',
  repeat: 'Repeat address', pending: 'High priority without a unit', 'unit-quiet': 'Unit on scene not heard', script: 'Agency script',
}
const HOOKS: Record<string, string> = { incident: 'each open incident: suggest', recommend: 'unit recommendation: prefer units, add notes', report: 'incident reports: agency checks' }
// Starting points for a new script, one per hook and language.
const TEMPLATE: Record<string, Record<string, string>> = {
  javascript: {
    incident: `// Called for each open incident. Return nothing, or {title, detail, severity, action: {label, command}}.
function advise(i) {
  if (i.type === "FALL" && i.notes.some((n) => /not breathing|unconscious/i.test(n)))
    return { title: i.number + ": fall, not breathing", detail: "A note says not breathing.", action: { label: "Cardiac arrest", command: "T CARD" } }
}`,
    recommend: `// Called with the recommendation. Return {prefer: [unit ids], notes: [text]} or nothing.
function recommend(r) {
  if (r.incident.hazards.length) return { notes: ["Hazards at this address: " + r.incident.hazards.join(", ")] }
}`,
    report: `// Called with a report. Return a list of {field, message}: shown as the agency's own checks.
function check(r) {
  if ((r.narrative || "").length < 40) return [{ field: "narrative", message: "Our policy: at least two sentences in the narrative" }]
}`,
  },
  lua: {
    incident: `-- Called for each open incident. Return nil, or {title=, detail=, severity=, action={label=, command=}}.
function advise(i)
  if i.priority == 1 and #i.units == 0 and i.minutes > 2 then
    return {title = i.number .. ": P1 waiting " .. math.floor(i.minutes) .. " min", severity = "urgent"}
  end
end`,
    recommend: `-- Called with the recommendation. Return {prefer={...}, notes={...}} or nil.
function recommend(r)
  return nil
end`,
    report: `-- Called with a report. Return a list of {field=, message=}.
function check(r)
  return nil
end`,
  },
}
const MODES = [
  { m: 'off', label: 'Off', hint: 'Nothing is evaluated' },
  { m: 'shadow', label: 'Shadow', hint: 'Evaluated and kept in the ledger; supervisors see it, dispatchers do not' },
  { m: 'live', label: 'Live', hint: 'Dispatchers see suggestions beside the incident' },
] as const

/** The advisory codebook: rules in plain words, edited, versioned, tested against the recorded ledger. */
export default function AdvisoryAdmin() {
  const me = useStore((s) => s.me)
  const [v, setV] = useState<AdminView | null>(null)
  const [cb, setCb] = useState<Codebook | null>(null)
  const [mode, setMode] = useState<AdminView['mode']>('shadow')
  const [note, setNote] = useState('')
  const [test, setTest] = useState<TestResult | null>(null)
  const [days, setDays] = useState(7)
  const load = () => api.get<AdminView>('/admin/advisory').then((x) => { setV(x); setCb(x.codebook); setMode(x.mode) }).catch((e) => toast.error(errText(e)))
  useEffect(() => { load() }, [])
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))
  if (!v || !cb) return <p className="text-sm text-slate-500">Loading…</p>
  const admin = can.admin(me)
  const up = (i: number, p: Partial<Rule>) => setCb({ ...cb, rules: cb.rules.map((r, j) => (j === i ? { ...r, ...p } : r)) })
  const dirty = JSON.stringify(cb) !== JSON.stringify(v.codebook) || mode !== v.mode
  const save = async (c = cb, m = mode, n = note) => {
    try {
      await api.put('/admin/advisory', { codebook: c, mode: m, note: n })
      toast.success('Codebook saved as a new version')
      setNote('')
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const runTest = async () => {
    try {
      setTest(await api.post<TestResult>('/admin/advisory/test', { codebook: cb, days }))
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const onDrag = (e: DragEndEvent) => {
    if (!e.over || e.active.id === e.over.id) return
    const list = cb.precedence.filter((x) => x !== e.active.id)
    list.splice(list.indexOf(String(e.over.id)), 0, String(e.active.id))
    setCb({ ...cb, precedence: list })
  }
  const decided = v.decisions.length
  const accepted = v.decisions.filter((d) => d.accepted).length
  return (
    <Page
      title="Advisory panel"
      intro="Deterministic evidence rules from a written codebook, and the agency's own scripts in JavaScript or Lua (sandboxed and time-limited). The panel proposes and explains, a person decides, and it never acts on its own. Start in shadow mode, test the codebook against recorded incidents, then go live. Every saved codebook is a version in the ledger and can be restored."
      actions={admin && <button className="btn btn-go" disabled={!dirty} onClick={() => save()}><Save size={14} aria-hidden /> Save version</button>}
    >
      <div className="grid gap-4 lg:grid-cols-[1fr_22rem]">
        <div className="space-y-4">
          <section className="panel rounded-lg p-3">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-[11px] font-bold uppercase tracking-wider text-slate-400">Mode</span>
              {MODES.map((x) => (
                <button key={x.m} disabled={!admin} title={x.hint} onClick={() => setMode(x.m)}
                  className={cx('btn text-xs', mode === x.m && (x.m === 'live' ? 'ring-1 ring-emerald-400 text-emerald-200' : x.m === 'shadow' ? 'ring-1 ring-violet-400 text-violet-200' : 'ring-1 ring-slate-400'))}>{x.label}</button>
              ))}
              <span className="text-xs text-slate-500">{MODES.find((x) => x.m === mode)!.hint}</span>
              <span className="ml-auto text-xs text-slate-500">version {v.version || 'default'} · {v.status.current} current{v.status.degraded && <b className="ml-1 text-amber-300">degraded</b>}</span>
            </div>
            {admin && dirty && <input className="input mt-2 text-xs" placeholder="What changed and why (kept with the version)" value={note} onChange={(e) => setNote(e.target.value)} />}
            {admin && (() => {
              // Rules supplied since this codebook was saved: offered, never added without a person.
              const fresh = v.default.rules.filter((d) => !cb.rules.some((r) => r.id === d.id))
              if (fresh.length === 0) return null
              return (
                <div className="mt-2 flex flex-wrap items-center gap-2 rounded border border-sky-500/40 bg-sky-500/10 px-2 py-1.5 text-xs text-sky-100" role="status">
                  <Lightbulb size={12} aria-hidden />
                  <span>New {fresh.length === 1 ? 'rule' : 'rules'} supplied since this version: {fresh.map((r) => r.name).join(', ')}.</span>
                  <button className="btn h-6 px-2 py-0 text-[11px]" onClick={() => { setCb({ ...cb, rules: [...cb.rules, ...fresh] }); setNote(`added the supplied ${fresh.map((r) => r.name).join(', ')}`) }}>
                    <Plus size={12} aria-hidden /> Add {fresh.length === 1 ? 'it' : 'them'}
                  </button>
                  <span className="text-sky-200/70">then Save version</span>
                </div>
              )
            })()}
          </section>

          <section className="panel rounded-lg p-3">
            <h2 className="mb-2 text-[11px] font-bold uppercase tracking-wider text-slate-400">Source precedence <span className="font-normal normal-case tracking-normal text-slate-500">strongest first; drag to reorder. A stronger source outweighs a weaker one; equal sources that disagree are "cannot determine".</span></h2>
            <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDrag}>
              <div className="flex flex-wrap items-center gap-1">
                {cb.precedence.map((p, i) => <Source key={p} id={p} n={i + 1} disabled={!admin} />)}
              </div>
            </DndContext>
          </section>

          <section className="panel space-y-2 rounded-lg p-3">
            <h2 className="text-[11px] font-bold uppercase tracking-wider text-slate-400">Rules</h2>
            {cb.rules.map((r, i) => (
              <div key={r.id} className={cx('rounded border p-2', r.enabled ? 'border-slate-700' : 'border-slate-800 opacity-60')}>
                <div className="flex flex-wrap items-center gap-2">
                  <input type="checkbox" checked={r.enabled} disabled={!admin} onChange={(e) => up(i, { enabled: e.target.checked })} aria-label={`Rule ${r.name} on`} />
                  <input className="input h-7 w-56 py-0 text-sm font-semibold" value={r.name} disabled={!admin} onChange={(e) => up(i, { name: e.target.value })} aria-label="Rule name" />
                  <span className="text-[11px] text-slate-500">{KIND[r.kind] ?? r.kind}</span>
                  <select className="input h-7 w-24 py-0 text-xs" value={r.severity} disabled={!admin} onChange={(e) => up(i, { severity: e.target.value as Rule['severity'] })} aria-label="Severity">
                    <option value="urgent">urgent</option><option value="info">info</option>
                  </select>
                  {test && <span className="ml-auto font-mono text-xs text-sky-300">{test.byRule[r.id] ?? 0} in test</span>}
                  {admin && r.kind === 'script' && <button className={cx('btn btn-ghost px-1', !test && 'ml-auto')} onClick={() => setCb({ ...cb, rules: cb.rules.filter((_, j) => j !== i) })} aria-label={`Remove ${r.name}`}><Trash2 size={12} /></button>}
                </div>
                <div className="mt-1 flex flex-wrap gap-2 text-xs">
                  {r.metres !== undefined && <Num label="metres" v={r.metres} on={(x) => up(i, { metres: x })} dis={!admin} />}
                  {r.minutes !== undefined && <Num label="minutes" v={r.minutes} on={(x) => up(i, { minutes: x })} dis={!admin} />}
                  {r.days !== undefined && <Num label="days" v={r.days} on={(x) => up(i, { days: x })} dis={!admin} />}
                  {r.count !== undefined && <Num label="count" v={r.count} on={(x) => up(i, { count: x })} dis={!admin} />}
                  {r.priority !== undefined && <Num label="up to priority" v={r.priority} on={(x) => up(i, { priority: x })} dis={!admin} />}
                  {r.kind === 'script' && (
                    <>
                      <select className="input h-7 w-32 py-0 text-xs" value={r.lang} disabled={!admin} onChange={(e) => up(i, { lang: e.target.value, ...(r.source === TEMPLATE[r.lang ?? 'javascript']?.[r.hook ?? 'incident'] ? { source: TEMPLATE[e.target.value][r.hook ?? 'incident'] } : {}) })} aria-label="Language"><option value="javascript">JavaScript</option><option value="lua">Lua</option></select>
                      <select className="input h-7 w-80 py-0 text-xs" value={r.hook} disabled={!admin} onChange={(e) => up(i, { hook: e.target.value, ...(r.source === TEMPLATE[r.lang ?? 'javascript']?.[r.hook ?? 'incident'] ? { source: TEMPLATE[r.lang ?? 'javascript'][e.target.value] } : {}) })} aria-label="Hook">{Object.entries(HOOKS).map(([k, n]) => <option key={k} value={k}>{n}</option>)}</select>
                      <Num label="time limit ms" v={r.timeoutMs ?? 50} on={(x) => up(i, { timeoutMs: x })} dis={!admin} />
                    </>
                  )}
                  {r.kind === 'words' && (
                    <>
                      <label className="flex min-w-72 flex-1 items-center gap-1 text-slate-500">words <input className="input h-7 py-0 font-mono text-xs" value={(r.words ?? []).join(', ')} disabled={!admin} onChange={(e) => up(i, { words: e.target.value.split(',').map((w) => w.trim()).filter(Boolean) })} /></label>
                      <label className="flex items-center gap-1 text-slate-500">fit <input className="input h-7 w-40 py-0 font-mono text-xs" value={(r.fits ?? []).join(' ')} disabled={!admin} onChange={(e) => up(i, { fits: e.target.value.toUpperCase().split(/[\s,]+/).filter(Boolean) })} /></label>
                      <label className="flex items-center gap-1 text-slate-500">suggest <input className="input h-7 w-20 py-0 font-mono text-xs" value={r.suggest ?? ''} disabled={!admin} onChange={(e) => up(i, { suggest: e.target.value.toUpperCase().trim() })} /></label>
                    </>
                  )}
                </div>
                {r.kind === 'script' && <Suspense fallback={<p className="text-xs text-slate-500">Loading the editor…</p>}><ScriptEditor rule={r} disabled={!admin} onChange={(src) => up(i, { source: src })} /></Suspense>}
                {v.status.faults?.filter((f) => f.rule === r.id).map((f) => <p key={f.message} className="mt-1 text-xs text-rose-300">Failed on the last run: {f.message}</p>)}
                <textarea className="input mt-1 h-auto min-h-9 py-1 text-xs text-slate-300" rows={2} value={r.explain} disabled={!admin} onChange={(e) => up(i, { explain: e.target.value })} aria-label="The rule in plain words" />
              </div>
            ))}
            {admin && (
              <div className="flex flex-wrap gap-2">
                {(['javascript', 'lua'] as const).map((lang) => (
                  <button key={lang} className="btn text-xs" onClick={() => {
                    const id = `script-${cb.rules.filter((x) => x.kind === 'script').length + 1}`
                    setCb({ ...cb, rules: [...cb.rules, { id, kind: 'script', name: lang === 'lua' ? 'Lua script' : 'JavaScript script', enabled: true, severity: 'info', explain: 'Our own rule: say what it looks for.', lang, hook: 'incident', source: TEMPLATE[lang].incident, timeoutMs: 50 }] })
                  }}><Plus size={12} aria-hidden /> {lang === 'lua' ? 'Lua' : 'JavaScript'} script</button>
                ))}
                <button className="btn text-xs" onClick={() => setCb(v.default)}><RotateCcw size={12} aria-hidden /> Start from the supplied codebook</button>
              </div>
            )}
          </section>
        </div>

        <div className="space-y-4">
          <section className="panel space-y-2 rounded-lg p-3">
            <h2 className="flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400"><FlaskConical size={12} aria-hidden /> Test against recorded incidents</h2>
            <p className="text-xs text-slate-400">Replays the ledger and applies these rules (saved or not) as if they had been live: each suggestion counted once.</p>
            <div className="flex items-center gap-2">
              <select className="input h-8 w-32 py-0 text-xs" value={days} onChange={(e) => setDays(Number(e.target.value))}>{[1, 7, 30, 90].map((d) => <option key={d} value={d}>last {d} day{d > 1 ? 's' : ''}</option>)}</select>
              <button className="btn text-xs" onClick={runTest}>Run test</button>
            </div>
            {test && (
              <div className="text-xs text-slate-300">
                <b className="text-slate-100">{test.sample.length ? Object.values(test.byRule).reduce((a, b) => a + b, 0) : 0}</b> suggestions over {test.events.toLocaleString()} events ({test.evaluations.toLocaleString()} evaluations, {test.millis} ms), {test.cannotDetermine} cannot determine.
                {test.faults?.map((f, k) => <p key={k} className="text-rose-300">Script {f.name}: {f.message}</p>)}
                <ul className="mt-1 max-h-60 space-y-0.5 overflow-y-auto">
                  {test.sample.slice(0, 40).map((s) => <li key={s.id} className="truncate"><span className="font-mono text-slate-500">{s.rule}</span> {s.title}</li>)}
                </ul>
              </div>
            )}
          </section>
          {v.shadow.length > 0 && <AdvisoryCards list={v.shadow} status={v.status} />}
          <section className="panel rounded-lg p-3">
            <h2 className="flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400"><Lightbulb size={12} aria-hidden /> Decisions <span className="font-normal normal-case tracking-normal text-slate-500">{decided ? `${accepted} of ${decided} accepted; rejecting is never an error` : 'none yet'}</span></h2>
            <ul className="mt-1 max-h-64 space-y-0.5 overflow-y-auto text-xs">
              {v.decisions.map((d) => <li key={d.id + d.at} className="flex gap-2"><span className={d.accepted ? 'text-emerald-300' : 'text-slate-400'}>{d.accepted ? 'accepted' : 'rejected'}</span><span className="font-mono text-slate-500">{d.rule}</span><span className="truncate text-slate-300">{d.reason || d.ran}</span><span className="ml-auto text-slate-500">{d.by}</span></li>)}
            </ul>
          </section>
          <section className="panel rounded-lg p-3">
            <h2 className="flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400"><History size={12} aria-hidden /> Versions</h2>
            <ul className="mt-1 space-y-1 text-xs">
              {(v.history ?? []).slice().reverse().map((h) => (
                <li key={h.seq} className="flex items-center gap-2">
                  <span className="font-mono font-bold text-slate-100">v{h.version}</span><span className="text-slate-400">{h.mode}</span>
                  <span className="truncate text-slate-300">{h.note}</span><span className="ml-auto text-slate-500">{h.by} {new Date(h.at).toLocaleDateString()}</span>
                  {admin && h.version !== v.version && h.rules && <button className="btn h-6 px-1.5 py-0 text-[11px]" onClick={() => save(h.rules!, h.mode as AdminView['mode'], `restore v${h.version}`)}>Restore</button>}
                </li>
              ))}
              {!v.history?.length && <li className="text-slate-500">The supplied codebook, in shadow mode, until the first save.</li>}
            </ul>
          </section>
        </div>
      </div>
    </Page>
  )
}

function Num({ label, v, on, dis }: { label: string; v: number; on: (x: number) => void; dis: boolean }) {
  return <label className="flex items-center gap-1 text-slate-500">{label} <input type="number" className="input h-7 w-20 py-0 font-mono text-xs" value={v} disabled={dis} onChange={(e) => on(Number(e.target.value))} /></label>
}

function Source({ id, n, disabled }: { id: string; n: number; disabled: boolean }) {
  const drag = useDraggable({ id, disabled })
  const drop = useDroppable({ id })
  const style = drag.transform ? { transform: `translate(${drag.transform.x}px, ${drag.transform.y}px)` } : undefined
  return (
    <span ref={(el) => { drag.setNodeRef(el); drop.setNodeRef(el) }} style={style} {...drag.attributes} {...drag.listeners}
      className={cx('flex cursor-grab items-center gap-1 rounded border px-2 py-1 text-sm', drop.isOver ? 'border-sky-400' : 'border-slate-700', drag.isDragging && 'z-10 bg-slate-800 shadow-lg')}>
      <GripVertical size={12} className="text-slate-500" aria-hidden /><span className="font-mono text-xs text-slate-500">{n}</span> {id}{n > 0 && ''}
    </span>
  )
}
