// Critical incidents administration: which incidents become a critical case, the alert phrases and radio
// tags checked in transcripts and narrative, the QA templates by call type, and the dispositions of a
// phrase alert. Everything is part of the settings object; saving bumps the policy version, and a case
// keeps the version it opened under.

import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { toast } from 'sonner'
import { AlertTriangle, FlaskConical, ListChecks, Play, Plus, RadioTower, Save, Trash2 } from 'lucide-react'
import { api, errText } from '../api'
import { Field } from '../ui'
import { useStore } from '../store'
import { Page, Table } from './Admin'
import {
  CRITICAL_KINDS, PHRASE_SEVERITIES, PHRASE_SOURCES, QA_EVENTS,
  asNum, fromLines, newId, parseList, saveSettings, toLines,
  type CriticalPolicy, type CriticalRule, type PhraseMatch, type PhraseRule, type QAMeasure, type QATemplate,
} from './FinalShared'

const SOURCE_LABEL: Record<string, string> = { call: 'call', radio: 'radio', text: 'text', narrative: 'narrative' }

/** CriticalAdmin edits the critical-incident policy. */
export function CriticalAdmin() {
  const view = useStore((s) => s.view)
  const live = (view?.settings as { critical?: CriticalPolicy } | undefined)?.critical
  const [p, setP] = useState<CriticalPolicy>(live ?? {})
  const [dirty, setDirty] = useState(false)
  useEffect(() => {
    if (!dirty) setP(live ?? {})
  }, [live, dirty])

  const callTypes = view?.callTypes ?? []
  const modifiers = view?.settings.modifiers ?? []

  const up = (patch: Partial<CriticalPolicy>) => {
    setP((cur) => ({ ...cur, ...patch }))
    setDirty(true)
  }

  const save = async () => {
    try {
      const next: CriticalPolicy = { ...p, version: (p.version ?? 0) + 1 }
      await saveSettings({ critical: next })
      toast.success('Critical incident policy saved')
      setDirty(false)
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const phraseRuleIds = (p.phrases ?? []).map((r) => ({ id: r.id, name: r.name }))

  return (
    <Page
      title="Critical incidents"
      intro="Which incidents open a critical case for the supervisor's care and quality review, the alert phrases and radio tags read in call and radio transcripts and in narrative, and the quality templates by call type. Saving raises the policy version; open cases keep the version they opened under."
      actions={<button className="btn btn-go" disabled={!dirty} onClick={save}><Save size={14} aria-hidden /> Save</button>}
    >
      <div className="space-y-4">
        <p className="text-xs text-slate-500">Policy version <span className="font-mono text-slate-300">{p.version ?? 0}</span>. A case keeps the version in force when it opened, so changing the policy never rewrites an open review.</p>
        <RulesSection rules={p.rules ?? []} up={up} callTypes={callTypes} modifiers={modifiers} phraseRuleIds={phraseRuleIds} />
        <PhrasesSection
          title="Alert phrases"
          icon={<AlertTriangle size={15} className="text-sky-300" aria-hidden />}
          intro="Words and short phrases that raise a critical alert when heard on a call or the radio, or seen in narrative. A negation nearby before the phrase cancels it. Testing marks matches so they are recorded but not alerted live."
          rules={p.phrases ?? []}
          onChange={(rs) => up({ phrases: rs })}
          starterKind="phrases"
          callTypes={callTypes}
          allRules={p.phrases ?? []}
        />
        <PhrasesSection
          title="Radio tags"
          icon={<RadioTower size={15} className="text-sky-300" aria-hidden />}
          intro="Operational facts marked in radio traffic for the review: descriptions, weapons, direction of travel, threats, use of force, CPR, hazards, requests, transports. Tags are never alerts."
          rules={p.tags ?? []}
          onChange={(rs) => up({ tags: rs })}
          starterKind="tags"
          callTypes={callTypes}
          allRules={p.tags ?? []}
          noSeverity
        />
        <QASection templates={p.qa ?? []} up={up} callTypes={callTypes} allTags={p.tags ?? []} />
        <DispositionsSection list={p.phraseDispositions ?? []} up={up} />
      </div>
    </Page>
  )
}

// ---- a) the rules that open a critical case ----

function RulesSection({ rules, up, callTypes, modifiers, phraseRuleIds }: {
  rules: CriticalRule[]
  up: (p: Partial<CriticalPolicy>) => void
  callTypes: { code: string; name: string }[]
  modifiers: { code: string; name: string }[]
  phraseRuleIds: { id: string; name: string }[]
}) {
  const setRules = (next: CriticalRule[]) => up({ rules: next })
  const setRule = (i: number, patch: Partial<CriticalRule>) => setRules(rules.map((r, j) => (j === i ? { ...r, ...patch } : r)))

  const matchCell = (r: CriticalRule, i: number) => {
    const id = `cr-match-${i}`
    const label = `Match for rule ${i + 1}`
    switch (r.kind) {
      case 'calltype':
        return (
          <select id={id} className="input h-7 text-xs" aria-label={label} value={r.match} onChange={(e) => setRule(i, { match: e.target.value })}>
            <option value="">choose a call type</option>
            {callTypes.map((c) => <option key={c.code} value={c.code}>{c.code} · {c.name}</option>)}
          </select>
        )
      case 'modifier':
        return (
          <select id={id} className="input h-7 text-xs" aria-label={label} value={r.match} onChange={(e) => setRule(i, { match: e.target.value })}>
            <option value="">choose a modifier</option>
            {modifiers.map((m) => <option key={m.code} value={m.code}>{m.code} · {m.name}</option>)}
          </select>
        )
      case 'priority':
        return (
          <input id={id} type="number" min={1} max={5} className="input h-7 w-20 font-mono text-xs" aria-label={`${label}: this priority or more urgent`} value={r.match} onChange={(e) => setRule(i, { match: e.target.value })} placeholder="1" />
        )
      case 'event':
        return (
          <select id={id} className="input h-7 text-xs" aria-label={label} value={r.match || 'unit-emergency'} onChange={(e) => setRule(i, { match: e.target.value })}>
            <option value="unit-emergency">unit emergency</option>
          </select>
        )
      case 'phrase':
        return (
          <select id={id} className="input h-7 text-xs" aria-label={label} value={r.match} onChange={(e) => setRule(i, { match: e.target.value })}>
            <option value="">choose a phrase rule</option>
            {phraseRuleIds.map((x) => <option key={x.id} value={x.id}>{x.name || x.id}</option>)}
          </select>
        )
      default:
        return <input id={id} className="input h-7 text-xs" aria-label={label} value={r.match} onChange={(e) => setRule(i, { match: e.target.value })} />
    }
  }

  return (
    <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="crit-rules-h">
      <h2 id="crit-rules-h" className="flex items-center gap-2 font-semibold text-slate-100"><AlertTriangle size={15} className="text-sky-300" aria-hidden /> What opens a critical case</h2>
      <p className="text-xs text-slate-400">A rule opens a critical case when an incident is of a call type, carries a modifier, is at a priority or more urgent, when a unit raises an emergency, or when an alert phrase is heard. The priority match means this priority number or more urgent (a lower number).</p>
      <Table head={['Name', 'Kind', 'Match', 'Off', '']} empty={rules.length === 0 ? <p className="p-3 text-sm text-slate-500">No rules yet. Critical call types still open a case on their own.</p> : undefined}>
        {rules.map((r, i) => (
          <tr key={i} className="align-top">
            <td className="px-3 py-1"><input className="input h-7 text-xs" aria-label={`Name for rule ${i + 1}`} value={r.name} onChange={(e) => setRule(i, { name: e.target.value })} placeholder="Shooting" /></td>
            <td className="px-3">
              <select className="input h-7 text-xs" aria-label={`Kind for rule ${i + 1}`} value={r.kind} onChange={(e) => setRule(i, { kind: e.target.value, match: '' })}>
                {CRITICAL_KINDS.map((k) => <option key={k} value={k}>{k}</option>)}
              </select>
            </td>
            <td className="px-3">{matchCell(r, i)}</td>
            <td className="px-3 text-center"><input type="checkbox" aria-label={`Turn off rule ${i + 1}`} checked={!!r.off} onChange={(e) => setRule(i, { off: e.target.checked })} /></td>
            <td className="px-3"><button className="btn btn-ghost px-1.5" aria-label={`Remove rule ${i + 1}`} onClick={() => setRules(rules.filter((_, j) => j !== i))}><Trash2 size={13} aria-hidden /></button></td>
          </tr>
        ))}
      </Table>
      <button className="btn" onClick={() => setRules([...rules, { id: newId('CR'), name: '', kind: 'calltype', match: '' }])}><Plus size={13} aria-hidden /> Add a rule</button>
    </section>
  )
}

// ---- b/c) alert phrases and radio tags (same shape) ----

function PhrasesSection({ title, icon, intro, rules, onChange, starterKind, callTypes, allRules, noSeverity }: {
  title: string
  icon: ReactNode
  intro: string
  rules: PhraseRule[]
  onChange: (rs: PhraseRule[]) => void
  starterKind: 'phrases' | 'tags'
  callTypes: { code: string; name: string }[]
  allRules: PhraseRule[]
  noSeverity?: boolean
}) {
  const hid = title.toLowerCase().replace(/\s+/g, '-')
  const setRule = (i: number, patch: Partial<PhraseRule>) => onChange(rules.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  const toggleSource = (i: number, src: string) => {
    const cur = rules[i].sources ?? []
    const has = cur.includes(src)
    setRule(i, { sources: has ? cur.filter((x) => x !== src) : [...cur, src] })
  }

  const loadStarters = async () => {
    try {
      const r = await api.get<{ phrases: PhraseRule[]; tags: PhraseRule[] }>('/admin/phrases/examples')
      const starters = (starterKind === 'phrases' ? r.phrases : r.tags) ?? []
      const have = new Set(rules.map((x) => x.id))
      const add = starters.filter((x) => !have.has(x.id)).map((x) => ({ ...x, testing: true }))
      if (add.length === 0) { toast('The starters are already loaded'); return }
      onChange([...rules, ...add])
      toast.success(`Loaded ${add.length} starter ${starterKind === 'phrases' ? 'phrases' : 'tags'}, marked testing`)
    } catch (e) {
      toast.error(errText(e))
    }
  }

  return (
    <section className="panel space-y-3 rounded-lg p-4" aria-labelledby={`${hid}-h`}>
      <h2 id={`${hid}-h`} className="flex items-center gap-2 font-semibold text-slate-100">{icon} {title}</h2>
      <p className="text-xs text-slate-400">{intro}</p>
      <div className="space-y-3">
        {rules.map((r, i) => (
          <div key={i} className="rounded-md border border-slate-800 p-3">
            <div className="grid gap-2 sm:grid-cols-2">
              <Field label="Id" htmlFor={`${hid}-id-${i}`}><input id={`${hid}-id-${i}`} className="input h-7 font-mono text-xs" value={r.id} onChange={(e) => setRule(i, { id: e.target.value })} /></Field>
              <Field label="Name" htmlFor={`${hid}-name-${i}`}><input id={`${hid}-name-${i}`} className="input h-7 text-xs" value={r.name} onChange={(e) => setRule(i, { name: e.target.value })} placeholder="Shots fired" /></Field>
            </div>
            <Field label="Phrases (one per line)" htmlFor={`${hid}-ph-${i}`} hint="A star at the end of a word matches its start, like shoot* for shooting.">
              <textarea id={`${hid}-ph-${i}`} className="input min-h-[3.5rem] text-xs" value={toLines(r.phrases)} onChange={(e) => setRule(i, { phrases: fromLines(e.target.value) })} placeholder={'shots fired\nshot fired\ngun shots'} />
            </Field>
            <div className="mt-2 grid gap-2 sm:grid-cols-2">
              <Field label="Negations (one per line)" htmlFor={`${hid}-neg-${i}`} hint="Empty uses the usual no, not, never, negative, false, cancel.">
                <textarea id={`${hid}-neg-${i}`} className="input min-h-[3.5rem] text-xs" value={toLines(r.negations)} onChange={(e) => setRule(i, { negations: fromLines(e.target.value) })} />
              </Field>
              <div className="space-y-2">
                <Field label="Negation window (words)" htmlFor={`${hid}-nw-${i}`} hint="How many words before the phrase a negation reaches (0 uses 3).">
                  <input id={`${hid}-nw-${i}`} type="number" min={0} className="input h-7 w-24 font-mono text-xs" value={r.negationWindow ?? ''} onChange={(e) => setRule(i, { negationWindow: asNum(e.target.value) })} />
                </Field>
                <div>
                  <span className="label">Sources</span>
                  <div className="flex flex-wrap gap-3">
                    {PHRASE_SOURCES.map((src) => (
                      <label key={src} className="flex items-center gap-1 text-xs text-slate-300">
                        <input type="checkbox" aria-label={`${r.name || r.id || 'rule'} on ${SOURCE_LABEL[src]}`} checked={(r.sources ?? []).includes(src)} onChange={() => toggleSource(i, src)} /> {SOURCE_LABEL[src]}
                      </label>
                    ))}
                  </div>
                  <p className="mt-1 text-xs text-slate-500">None ticked means every source.</p>
                </div>
              </div>
            </div>
            <div className="mt-2 grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
              <Field label="Call types" htmlFor={`${hid}-ct-${i}`} hint="Comma list of codes; empty means all.">
                <input id={`${hid}-ct-${i}`} className="input h-7 font-mono text-xs" list={`${hid}-ct-list`} value={(r.callTypes ?? []).join(', ')} onChange={(e) => setRule(i, { callTypes: parseList(e.target.value) })} />
              </Field>
              <Field label="Least confidence (0 to 1)" htmlFor={`${hid}-mc-${i}`} hint="A match below this is reported as uncertain, not dropped.">
                <input id={`${hid}-mc-${i}`} type="number" min={0} max={1} step={0.05} className="input h-7 w-24 font-mono text-xs" value={r.minConfidence ?? ''} onChange={(e) => setRule(i, { minConfidence: asNum(e.target.value) })} />
              </Field>
              {!noSeverity && (
                <Field label="Severity" htmlFor={`${hid}-sv-${i}`}>
                  <select id={`${hid}-sv-${i}`} className="input h-7 text-xs" value={r.severity ?? 'high'} onChange={(e) => setRule(i, { severity: e.target.value })}>
                    {PHRASE_SEVERITIES.map((sv) => <option key={sv} value={sv}>{sv}</option>)}
                  </select>
                </Field>
              )}
              <div className="flex items-end gap-4">
                <label className="flex items-center gap-2 text-xs text-slate-300"><input type="checkbox" checked={!!r.testing} onChange={(e) => setRule(i, { testing: e.target.checked })} /> Testing</label>
                <label className="flex items-center gap-2 text-xs text-slate-300"><input type="checkbox" checked={!!r.off} onChange={(e) => setRule(i, { off: e.target.checked })} /> Off</label>
              </div>
            </div>
            <div className="mt-2 flex justify-end">
              <button className="btn btn-ghost text-rose-300" aria-label={`Remove ${title} rule ${i + 1}`} onClick={() => onChange(rules.filter((_, j) => j !== i))}><Trash2 size={12} aria-hidden /> Remove</button>
            </div>
          </div>
        ))}
      </div>
      <datalist id={`${hid}-ct-list`}>{callTypes.map((c) => <option key={c.code} value={c.code}>{c.name}</option>)}</datalist>
      <div className="flex flex-wrap gap-2">
        <button className="btn" onClick={() => onChange([...rules, { id: newId(starterKind === 'phrases' ? 'PH' : 'TG'), name: '', phrases: [] }])}><Plus size={13} aria-hidden /> Add {starterKind === 'phrases' ? 'a phrase' : 'a tag'}</button>
        <button className="btn" onClick={loadStarters}><FlaskConical size={13} aria-hidden /> Load the starter {starterKind === 'phrases' ? 'phrases' : 'tags'}</button>
      </div>
      <TryBox rules={allRules} />
    </section>
  )
}

// ---- Try it box ----

function TryBox({ rules }: { rules: PhraseRule[] }) {
  const [text, setText] = useState('')
  const [source, setSource] = useState('call')
  const [matches, setMatches] = useState<PhraseMatch[] | null>(null)
  const [problems, setProblems] = useState<Record<string, string[]>>({})
  const [busy, setBusy] = useState(false)

  const run = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ matches: PhraseMatch[]; problems: Record<string, string[]> }>('/admin/phrases/try', { rules, text, source })
      setMatches(r.matches ?? [])
      setProblems(r.problems ?? {})
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }

  const tags = (m: PhraseMatch) => {
    const t: { text: string; cls: string }[] = []
    if (m.negated) t.push({ text: 'NEGATED', cls: 'bg-slate-700 text-slate-200' })
    if (m.uncertain) t.push({ text: 'UNCERTAIN', cls: 'bg-amber-500/20 text-amber-200' })
    if (m.testing) t.push({ text: 'TESTING', cls: 'bg-sky-500/20 text-sky-200' })
    return t
  }

  const problemList = Object.entries(problems).filter(([, v]) => v.length > 0)

  return (
    <div className="rounded-md border border-slate-800 p-3">
      <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500">Try it</h3>
      <div className="mt-2 grid gap-2 sm:grid-cols-[1fr_auto]">
        <Field label="Text" htmlFor="try-text">
          <textarea id="try-text" className="input min-h-[3rem] text-xs" value={text} onChange={(e) => setText(e.target.value)} placeholder="Type what a caller or a unit said." />
        </Field>
        <div className="space-y-2">
          <Field label="Source" htmlFor="try-source">
            <select id="try-source" className="input h-7 text-xs" value={source} onChange={(e) => setSource(e.target.value)}>
              {PHRASE_SOURCES.map((s) => <option key={s} value={s}>{s}</option>)}
            </select>
          </Field>
          <button className="btn btn-go" disabled={busy || !text.trim()} onClick={run}><Play size={13} aria-hidden /> Try it</button>
        </div>
      </div>
      {matches && (
        <div className="mt-2 space-y-1">
          {matches.length === 0 ? <p className="text-xs text-slate-500">No matches.</p> : (
            <ul className="space-y-1">
              {matches.map((m, i) => (
                <li key={i} className="rounded border border-slate-800 px-2 py-1 text-xs">
                  <span className="font-semibold text-slate-100">{m.name || m.rule || m.ruleId}</span>
                  {m.found && <span className="ml-2 font-mono text-emerald-300">{m.found}</span>}
                  {tags(m).map((t) => <span key={t.text} className={`chip ml-2 ${t.cls}`}>{t.text}</span>)}
                  {m.context && <div className="mt-0.5 text-slate-400">…{m.context}…</div>}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
      {problemList.length > 0 && (
        <div className="mt-2 rounded-md border border-amber-500/40 bg-amber-500/10 p-2 text-xs text-amber-200">
          <p className="font-semibold">Validation problems:</p>
          <ul className="mt-1 list-disc space-y-0.5 pl-5">
            {problemList.map(([id, ps]) => ps.map((pp, j) => <li key={id + j}><span className="font-mono">{id}</span>: {pp}</li>))}
          </ul>
        </div>
      )}
    </div>
  )
}

// ---- d) QA templates ----

function QASection({ templates, up, callTypes, allTags }: {
  templates: QATemplate[]
  up: (p: Partial<CriticalPolicy>) => void
  callTypes: { code: string; name: string }[]
  allTags: PhraseRule[]
}) {
  const setTemplates = (next: QATemplate[]) => up({ qa: next })
  const setT = (i: number, patch: Partial<QATemplate>) => setTemplates(templates.map((t, j) => (j === i ? { ...t, ...patch } : t)))
  const setMeasure = (ti: number, mi: number, patch: Partial<QAMeasure>) =>
    setT(ti, { measures: templates[ti].measures.map((m, j) => (j === mi ? { ...m, ...patch } : m)) })

  const loadStarters = async () => {
    try {
      const r = await api.get<{ qa: QATemplate[] }>('/admin/phrases/examples')
      const have = new Set(templates.map((x) => x.id))
      const add = (r.qa ?? []).filter((x) => !have.has(x.id))
      if (add.length === 0) { toast('The starter templates are already loaded'); return }
      setTemplates([...templates, ...add])
      toast.success(`Loaded ${add.length} starter templates`)
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const tagOptions = useMemo(() => allTags.map((t) => t.id), [allTags])

  return (
    <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="qa-h">
      <h2 id="qa-h" className="flex items-center gap-2 font-semibold text-slate-100"><ListChecks size={15} className="text-sky-300" aria-hidden /> Quality templates</h2>
      <p className="text-xs text-slate-400">Each template is the quality review for its call types: timed intervals against a standard, and checks a reviewer judges. Leave call types empty to use a template for any critical incident.</p>
      <div className="space-y-3">
        {templates.map((t, ti) => (
          <div key={ti} className="rounded-md border border-slate-800 p-3">
            <div className="grid gap-2 sm:grid-cols-2">
              <Field label="Id" htmlFor={`qa-id-${ti}`}><input id={`qa-id-${ti}`} className="input h-7 font-mono text-xs" value={t.id} onChange={(e) => setT(ti, { id: e.target.value })} /></Field>
              <Field label="Name" htmlFor={`qa-name-${ti}`}><input id={`qa-name-${ti}`} className="input h-7 text-xs" value={t.name} onChange={(e) => setT(ti, { name: e.target.value })} placeholder="High-risk call review" /></Field>
            </div>
            <div className="mt-2 grid gap-2 sm:grid-cols-2">
              <Field label="Call types" htmlFor={`qa-ct-${ti}`} hint="Comma list of codes; empty means any critical incident.">
                <input id={`qa-ct-${ti}`} className="input h-7 font-mono text-xs" list="qa-ct-list" value={(t.callTypes ?? []).join(', ')} onChange={(e) => setT(ti, { callTypes: parseList(e.target.value) })} />
              </Field>
              <Field label="Policy or standard" htmlFor={`qa-pol-${ti}`} hint="The policy and its version this template measures against.">
                <input id={`qa-pol-${ti}`} className="input h-7 text-xs" value={t.policy ?? ''} onChange={(e) => setT(ti, { policy: e.target.value })} />
              </Field>
            </div>
            <h4 className="mt-3 text-xs font-semibold uppercase tracking-wider text-slate-500">Measures</h4>
            <Table head={['Name', 'Kind', 'From', 'To', 'Standard (s)', 'Scored', 'N/A reasons', 'Specialist', 'Expect tag', '']}>
              {(t.measures ?? []).map((m, mi) => (
                <tr key={mi} className="align-top">
                  <td className="px-2 py-1"><input className="input h-7 text-xs" aria-label={`Measure name ${mi + 1}`} value={m.name} onChange={(e) => setMeasure(ti, mi, { name: e.target.value })} /></td>
                  <td className="px-2">
                    <select className="input h-7 text-xs" aria-label={`Measure kind ${mi + 1}`} value={m.kind} onChange={(e) => setMeasure(ti, mi, { kind: e.target.value })}>
                      <option value="interval">interval</option>
                      <option value="check">check</option>
                    </select>
                  </td>
                  <td className="px-2">
                    <select className="input h-7 text-xs" aria-label={`From event ${mi + 1}`} value={m.from ?? ''} disabled={m.kind !== 'interval'} onChange={(e) => setMeasure(ti, mi, { from: e.target.value })}>
                      <option value="">none</option>
                      {QA_EVENTS.map((ev) => <option key={ev} value={ev}>{ev}</option>)}
                    </select>
                  </td>
                  <td className="px-2">
                    <select className="input h-7 text-xs" aria-label={`To event ${mi + 1}`} value={m.to ?? ''} disabled={m.kind !== 'interval'} onChange={(e) => setMeasure(ti, mi, { to: e.target.value })}>
                      <option value="">none</option>
                      {QA_EVENTS.map((ev) => <option key={ev} value={ev}>{ev}</option>)}
                    </select>
                  </td>
                  <td className="px-2"><input type="number" min={0} className="input h-7 w-20 font-mono text-xs" aria-label={`Standard seconds ${mi + 1}`} value={m.standardSec ?? ''} disabled={m.kind !== 'interval'} onChange={(e) => setMeasure(ti, mi, { standardSec: asNum(e.target.value) })} /></td>
                  <td className="px-2 text-center"><input type="checkbox" aria-label={`Scored ${mi + 1}`} checked={!!m.scored} onChange={(e) => setMeasure(ti, mi, { scored: e.target.checked })} /></td>
                  <td className="px-2"><input className="input h-7 text-xs" aria-label={`N/A reasons ${mi + 1}`} value={(m.naReasons ?? []).join(', ')} onChange={(e) => setMeasure(ti, mi, { naReasons: parseList(e.target.value) })} placeholder="comma list" /></td>
                  <td className="px-2 text-center"><input type="checkbox" aria-label={`Specialist review ${mi + 1}`} checked={!!m.specialist} onChange={(e) => setMeasure(ti, mi, { specialist: e.target.checked })} /></td>
                  <td className="px-2"><input className="input h-7 text-xs" aria-label={`Expect tag ${mi + 1}`} list="qa-tag-list" value={m.expect ?? ''} onChange={(e) => setMeasure(ti, mi, { expect: e.target.value })} /></td>
                  <td className="px-2"><button className="btn btn-ghost px-1.5" aria-label={`Remove measure ${mi + 1}`} onClick={() => setT(ti, { measures: t.measures.filter((_, j) => j !== mi) })}><Trash2 size={13} aria-hidden /></button></td>
                </tr>
              ))}
            </Table>
            <div className="mt-2 flex justify-between">
              <button className="btn" onClick={() => setT(ti, { measures: [...(t.measures ?? []), { id: newId('M'), name: '', kind: 'interval' }] })}><Plus size={13} aria-hidden /> Add a measure</button>
              <button className="btn btn-ghost text-rose-300" aria-label={`Remove template ${ti + 1}`} onClick={() => setTemplates(templates.filter((_, j) => j !== ti))}><Trash2 size={12} aria-hidden /> Remove template</button>
            </div>
          </div>
        ))}
      </div>
      <datalist id="qa-ct-list">{callTypes.map((c) => <option key={c.code} value={c.code}>{c.name}</option>)}</datalist>
      <datalist id="qa-tag-list">{tagOptions.map((x) => <option key={x} value={x} />)}</datalist>
      <div className="flex flex-wrap gap-2">
        <button className="btn" onClick={() => setTemplates([...templates, { id: newId('QA'), name: '', callTypes: [], measures: [] }])}><Plus size={13} aria-hidden /> Add a template</button>
        <button className="btn" onClick={loadStarters}><FlaskConical size={13} aria-hidden /> Load the starter templates</button>
      </div>
    </section>
  )
}

// ---- e) phrase dispositions ----

function DispositionsSection({ list, up }: { list: string[]; up: (p: Partial<CriticalPolicy>) => void }) {
  return (
    <section className="panel space-y-2 rounded-lg p-4" aria-labelledby="disp-h">
      <h2 id="disp-h" className="font-semibold text-slate-100">Phrase alert dispositions</h2>
      <p className="text-xs text-slate-400">How a person closes out a phrase alert (acted on it, no action needed, a false match). One per line; empty uses the defaults.</p>
      <label className="label" htmlFor="phrase-disp">Dispositions</label>
      <textarea id="phrase-disp" className="input min-h-[4rem] text-xs" value={toLines(list)} onChange={(e) => up({ phraseDispositions: fromLines(e.target.value) })} placeholder={'Acted on\nNo action needed\nFalse match'} />
    </section>
  )
}
