import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { Award, Plus, Save, Trash2 } from 'lucide-react'
import { api, errText } from '../api'
import { Field, Modal } from '../ui'
import { cx } from '../util'
import type { Agency, CapNeed, Credential, Equivalency, PersonBrief, QualDef, View } from '../types'
import { Page, Table } from './Admin'

type QualsResponse = { items: QualDef[]; equivalencies: Equivalency[]; categories: string[]; states: string[] }

const CRED_STATES = ['trainee', 'qualified', 'instructor', 'evaluator', 'restricted', 'unavailable', 'suspended', 'expired']
const CATEGORIES: QualDef['category'][] = ['team', 'specialty', 'training', 'credential']

const toISO = (date: string): string | undefined => (date ? new Date(date).toISOString() : undefined)
const dateInput = (iso?: string): string => (iso ? iso.slice(0, 10) : '')

/** Qualifications and teams: the definitions (what each qualification means, how long it lasts, what it needs
 * first), mutual-aid equivalencies, and each person's skills file of credentials. */
export function Qualifications({ view }: { view: View }) {
  const [data, setData] = useState<QualsResponse | null>(null)
  const [agencies, setAgencies] = useState<Agency[]>([])
  const load = () => api.get<QualsResponse>('/quals').then((r) => setData({ items: r.items ?? [], equivalencies: r.equivalencies ?? [], categories: r.categories ?? CATEGORIES, states: r.states ?? CRED_STATES })).catch((e) => toast.error(errText(e)))
  useEffect(() => {
    load()
    api.get<{ agencies: Agency[] }>('/agencies').then((r) => setAgencies(r.agencies ?? [])).catch(() => {})
  }, [])
  const quals = data?.items ?? []
  return (
    <Page
      title="Qualifications and teams"
      intro="What each qualification means, how long it stays valid and what it needs first; the equivalencies that honour another agency's qualifications; and each person's skills file. A unit's live capability is read from the valid credentials of the people aboard."
    >
      <Definitions quals={quals} agencies={agencies} onChange={load} />
      <Equivalencies equivalencies={data?.equivalencies ?? []} quals={quals} agencies={agencies} onChange={load} />
      <SkillsFile view={view} quals={quals} />
      <CapabilityNote view={view} />
    </Page>
  )
}

// ---- a) Definitions ----

type QualDraft = QualDef & { isNew: boolean; levelsText: string }
const blankQual = (): QualDraft => ({ code: '', name: '', category: 'credential', levels: [], prereqs: [], isNew: true, levelsText: '' })

function Definitions({ quals, agencies, onChange }: { quals: QualDef[]; agencies: Agency[]; onChange: () => void }) {
  const [d, setD] = useState<QualDraft | null>(null)
  const open = (q?: QualDef) => setD(q
    ? { ...q, levels: q.levels ?? [], prereqs: q.prereqs ?? [], isNew: false, levelsText: (q.levels ?? []).join(', ') }
    : blankQual())
  const togglePrereq = (code: string) => {
    if (!d) return
    const has = (d.prereqs ?? []).includes(code)
    setD({ ...d, prereqs: has ? (d.prereqs ?? []).filter((x) => x !== code) : [...(d.prereqs ?? []), code] })
  }
  const save = async () => {
    if (!d) return
    const code = d.code.trim().toLowerCase()
    if (!code) { toast.error('A code is needed'); return }
    const levels = d.levelsText.split(',').map((x) => x.trim()).filter(Boolean)
    try {
      await api.put(`/admin/quals/${encodeURIComponent(code)}`, {
        name: d.name, category: d.category, agency: d.agency || undefined, levels, months: d.months || undefined, renewDays: d.renewDays || undefined,
        prereqs: d.prereqs ?? [], verify: !!d.verify, selfAvailable: !!d.selfAvailable, documents: d.documents || undefined, note: d.note || undefined,
      })
      toast.success('Saved')
      setD(null)
      onChange()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const remove = async (code: string) => {
    if (!confirm(`Delete qualification ${code}?`)) return
    try {
      await api.del(`/admin/quals/${encodeURIComponent(code)}`)
      setD(null)
      onChange()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section aria-label="Qualification definitions" className="mb-8">
      <div className="mb-2 flex items-center gap-2">
        <h2 className="font-semibold text-slate-100">Definitions</h2>
        <button className="btn btn-go ml-auto" onClick={() => open()}><Plus size={14} aria-hidden /> Add a qualification</button>
      </div>
      <Table head={['Code', 'Name', 'Category', 'Agency', 'Months valid', 'Prerequisites']} empty={quals.length === 0 ? <p className="p-3 text-sm text-slate-500">No qualifications defined yet.</p> : undefined}>
        {quals.map((q) => (
          <tr key={q.code} className="cursor-pointer hover:bg-slate-800/50" onClick={() => open(q)}>
            <td className="px-3 py-1.5 font-mono text-slate-300">{q.code}</td>
            <td className="px-3 text-slate-100">{q.name}</td>
            <td className="px-3 text-slate-400">{q.category}</td>
            <td className="px-3 text-slate-400">{q.agency || <span className="text-slate-500">centre</span>}</td>
            <td className="px-3 font-mono text-slate-400">{q.months || '—'}</td>
            <td className="px-3 text-slate-400">{(q.prereqs ?? []).join(', ') || '—'}</td>
          </tr>
        ))}
      </Table>
      {d && (
        <Modal open wide onClose={() => setD(null)} title={d.isNew ? 'New qualification' : d.name || d.code} icon={<Award size={16} className="text-sky-300" aria-hidden />}
          footer={<>{!d.isNew && <button className="btn mr-auto text-rose-300" onClick={() => remove(d.code)}><Trash2 size={13} aria-hidden /> Delete</button>}<button className="btn" onClick={() => setD(null)}>Cancel</button><button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save</button></>}>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Code" htmlFor="q-code" hint="Lower case with dashes, e.g. swift-water."><input id="q-code" className="input font-mono" disabled={!d.isNew} value={d.code} onChange={(e) => setD({ ...d, code: e.target.value })} /></Field>
            <Field label="Name" htmlFor="q-name"><input id="q-name" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} /></Field>
            <Field label="Category" htmlFor="q-cat"><select id="q-cat" className="input" value={d.category} onChange={(e) => setD({ ...d, category: e.target.value as QualDef['category'] })}>{CATEGORIES.map((c) => <option key={c} value={c}>{c}</option>)}</select></Field>
            <Field label="Agency" htmlFor="q-ag" hint="Blank for the centre's own."><select id="q-ag" className="input" value={d.agency ?? ''} onChange={(e) => setD({ ...d, agency: e.target.value || undefined })}><option value="">centre</option>{agencies.map((a) => <option key={a.code} value={a.code}>{a.code} — {a.name}</option>)}</select></Field>
            <div className="sm:col-span-2"><Field label="Levels" htmlFor="q-lv" hint="A comma list, e.g. basic, advanced, lead."><input id="q-lv" className="input" value={d.levelsText} onChange={(e) => setD({ ...d, levelsText: e.target.value })} /></Field></div>
            <Field label="Months valid" htmlFor="q-mo" hint="How long a credential lasts; 0 for no expiry."><input id="q-mo" type="number" min={0} className="input" value={d.months ?? ''} onChange={(e) => setD({ ...d, months: e.target.value ? Number(e.target.value) : undefined })} /></Field>
            <Field label="Renew days" htmlFor="q-rd" hint="Days before expiry to start reminding."><input id="q-rd" type="number" min={0} className="input" value={d.renewDays ?? ''} onChange={(e) => setD({ ...d, renewDays: e.target.value ? Number(e.target.value) : undefined })} /></Field>
            <div className="sm:col-span-2">
              <span className="label">Prerequisites</span>
              <div className="flex flex-wrap gap-1.5">
                {quals.filter((q) => q.code !== d.code).map((q) => {
                  const on = (d.prereqs ?? []).includes(q.code)
                  return <button key={q.code} type="button" className={cx('chip cursor-pointer', on ? 'bg-sky-500/20 text-sky-200' : 'bg-slate-800 text-slate-400 hover:bg-slate-700')} aria-pressed={on} onClick={() => togglePrereq(q.code)}>{q.code}</button>
                })}
                {quals.filter((q) => q.code !== d.code).length === 0 && <span className="text-xs text-slate-500">No other qualifications to require.</span>}
              </div>
            </div>
            <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!d.verify} onChange={(e) => setD({ ...d, verify: e.target.checked })} /> Counts only once a supervisor verifies it</label>
            <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!d.selfAvailable} onChange={(e) => setD({ ...d, selfAvailable: e.target.checked })} /> A person may enter this for themselves</label>
            <div className="sm:col-span-2"><Field label="Documents" htmlFor="q-doc" hint="What proof to keep, e.g. a card scan."><input id="q-doc" className="input" value={d.documents ?? ''} onChange={(e) => setD({ ...d, documents: e.target.value || undefined })} /></Field></div>
            <div className="sm:col-span-2"><Field label="Note" htmlFor="q-note"><input id="q-note" className="input" value={d.note ?? ''} onChange={(e) => setD({ ...d, note: e.target.value || undefined })} /></Field></div>
          </div>
        </Modal>
      )}
    </section>
  )
}

// ---- b) Equivalencies (mutual aid) ----

function Equivalencies({ equivalencies, quals, agencies, onChange }: { equivalencies: Equivalency[]; quals: QualDef[]; agencies: Agency[]; onChange: () => void }) {
  const [fromAgency, setFromAgency] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [agency, setAgency] = useState('')
  const [scope, setScope] = useState('')
  const [expires, setExpires] = useState('')
  const add = async () => {
    if (!from.trim() || !to.trim()) { toast.error('A from and a to qualification are needed'); return }
    try {
      await api.put('/admin/equivalencies', { fromAgency: fromAgency || undefined, from: from.trim(), to: to.trim(), agency: agency || undefined, scope: scope || undefined, expires: toISO(expires) })
      toast.success('Equivalency added')
      setFromAgency(''); setFrom(''); setTo(''); setAgency(''); setScope(''); setExpires('')
      onChange()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const remove = async (id: string) => {
    if (!confirm('Remove this equivalency?')) return
    try {
      await api.del(`/admin/equivalencies/${encodeURIComponent(id)}`)
      onChange()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section aria-label="Equivalencies" className="mb-8">
      <h2 className="mb-2 font-semibold text-slate-100">Equivalencies (mutual aid)</h2>
      <p className="mb-2 text-sm text-slate-400">Honour another agency's qualification as one of ours, for a scope and until a date.</p>
      <Table head={['From agency', 'Their qualification', 'Counts as (ours)', 'Accepting agency', 'Scope', 'Expires', '']} empty={equivalencies.length === 0 ? <p className="p-3 text-sm text-slate-500">No equivalencies set.</p> : undefined}>
        {equivalencies.map((eq) => (
          <tr key={eq.id}>
            <td className="px-3 py-1.5 text-slate-300">{eq.fromAgency || '—'}</td>
            <td className="px-3 text-slate-100">{eq.from}</td>
            <td className="px-3 text-slate-100">{eq.to}</td>
            <td className="px-3 text-slate-400">{eq.agency || <span className="text-slate-500">centre</span>}</td>
            <td className="px-3 text-slate-400">{eq.scope || '—'}</td>
            <td className="px-3 text-slate-400">{eq.expires?.slice(0, 10) || '—'}</td>
            <td className="px-3"><button className="btn h-7 text-rose-300" onClick={() => remove(eq.id)} aria-label={`Remove equivalency ${eq.from} to ${eq.to}`}><Trash2 size={12} aria-hidden /></button></td>
          </tr>
        ))}
      </Table>
      <div className="panel mt-3 grid gap-3 rounded-lg p-3 sm:grid-cols-3">
        <Field label="From agency" htmlFor="eq-fa"><input id="eq-fa" className="input" value={fromAgency} onChange={(e) => setFromAgency(e.target.value)} placeholder="their agency" /></Field>
        <Field label="Their qualification" htmlFor="eq-from"><input id="eq-from" className="input" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
        <Field label="Counts as (ours)" htmlFor="eq-to"><select id="eq-to" className="input" value={to} onChange={(e) => setTo(e.target.value)}><option value="">— choose —</option>{quals.map((q) => <option key={q.code} value={q.code}>{q.name} ({q.code})</option>)}</select></Field>
        <Field label="Accepting agency" htmlFor="eq-ag" hint="Blank for the centre."><select id="eq-ag" className="input" value={agency} onChange={(e) => setAgency(e.target.value)}><option value="">centre</option>{agencies.map((a) => <option key={a.code} value={a.code}>{a.code} — {a.name}</option>)}</select></Field>
        <Field label="Scope" htmlFor="eq-scope"><input id="eq-scope" className="input" value={scope} onChange={(e) => setScope(e.target.value)} placeholder="e.g. this incident, this event" /></Field>
        <Field label="Expires" htmlFor="eq-exp"><input id="eq-exp" type="date" className="input" value={expires} onChange={(e) => setExpires(e.target.value)} /></Field>
        <div className="sm:col-span-3"><button className="btn btn-go" onClick={add}><Plus size={13} aria-hidden /> Add equivalency</button></div>
      </div>
    </section>
  )
}

// ---- c) A person's skills file ----

function SkillsFile({ view, quals }: { view: View; quals: QualDef[] }) {
  const members = view.roadmap?.members ?? []
  const [memberId, setMemberId] = useState('')
  const [file, setFile] = useState<PersonBrief | null>(null)
  const [editing, setEditing] = useState<Credential | null>(null)
  const [adding, setAdding] = useState(false)
  const load = (id: string) => {
    if (!id) { setFile(null); return }
    api.get<PersonBrief>(`/members/${encodeURIComponent(id)}/file`).then(setFile).catch((e) => toast.error(errText(e)))
  }
  useEffect(() => { load(memberId) }, [memberId]) // eslint-disable-line react-hooks/exhaustive-deps
  const creds = file?.creds ?? []
  const problems = useMemo(() => {
    const m: Record<string, string[]> = {}
    for (const p of file?.credProblems ?? []) {
      const [code, ...rest] = p.split(': ')
      ;(m[code] ??= []).push(rest.join(': '))
    }
    return m
  }, [file])
  const endCred = async (qual: string) => {
    const reason = prompt(`Why is ${qual} ended?`)
    if (reason === null) return
    try {
      await api.del(`/members/${encodeURIComponent(memberId)}/credentials/${encodeURIComponent(qual)}?reason=${encodeURIComponent(reason)}`)
      toast.success('Credential ended')
      load(memberId)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section aria-label="Person skills file" className="mb-8">
      <h2 className="mb-2 font-semibold text-slate-100">A person's skills file</h2>
      <div className="flex flex-wrap items-end gap-2">
        <div className="w-72"><Field label="Member" htmlFor="sf-member"><select id="sf-member" className="input" value={memberId} onChange={(e) => setMemberId(e.target.value)}><option value="">— choose a person —</option>{members.map((m) => <option key={m.id} value={m.id}>{m.name} ({m.id})</option>)}</select></Field></div>
        {file && <button className="btn btn-go" onClick={() => setAdding(true)}><Plus size={13} aria-hidden /> Add a credential</button>}
      </div>
      {file && (
        <div className="mt-3">
          <Table head={['Qualification', 'Level', 'State', 'Issuer', 'Issued', 'Expires', 'Verified', 'Restriction', 'Actions']} empty={creds.length === 0 ? <p className="p-3 text-sm text-slate-500">No credentials recorded.</p> : undefined}>
            {creds.map((c) => {
              const def = quals.find((q) => q.code === c.qual)
              const probs = problems[c.qual] ?? []
              return (
                <tr key={c.qual} className="align-top">
                  <td className="px-3 py-1.5 text-slate-100">{def?.name ?? c.qual}
                    {probs.length > 0 && <div className="mt-0.5 text-xs text-amber-300">{probs.join('; ')}</div>}
                  </td>
                  <td className="px-3 text-slate-400">{c.level || '—'}</td>
                  <td className="px-3 text-slate-300">{c.state}</td>
                  <td className="px-3 text-slate-400">{c.issuer || '—'}</td>
                  <td className="px-3 text-slate-400">{c.issued?.slice(0, 10) || '—'}</td>
                  <td className="px-3 text-slate-400">{c.expires?.slice(0, 10) || '—'}</td>
                  <td className="px-3 text-slate-400">{c.verifiedBy ? `${c.verifiedBy}${c.verifiedAt ? ` · ${c.verifiedAt.slice(0, 10)}` : ''}` : '—'}</td>
                  <td className="px-3 text-slate-400">{c.restriction || '—'}</td>
                  <td className="px-3">
                    <div className="flex gap-1">
                      <button className="btn h-7" onClick={() => setEditing(c)}>Edit</button>
                      <button className="btn h-7 text-rose-300" onClick={() => endCred(c.qual)}>End</button>
                    </div>
                  </td>
                </tr>
              )
            })}
          </Table>
        </div>
      )}
      {(adding || editing) && (
        <CredentialModal memberId={memberId} quals={quals} existing={editing} onClose={() => { setAdding(false); setEditing(null) }} onDone={() => { setAdding(false); setEditing(null); load(memberId) }} />
      )}
    </section>
  )
}

function CredentialModal({ memberId, quals, existing, onClose, onDone }: { memberId: string; quals: QualDef[]; existing: Credential | null; onClose: () => void; onDone: () => void }) {
  const [qual, setQual] = useState(existing?.qual ?? '')
  const [level, setLevel] = useState(existing?.level ?? '')
  const [state, setState] = useState(existing?.state ?? 'qualified')
  const [issuer, setIssuer] = useState(existing?.issuer ?? '')
  const [issued, setIssued] = useState(dateInput(existing?.issued))
  const [expires, setExpires] = useState(dateInput(existing?.expires))
  const [restriction, setRestriction] = useState(existing?.restriction ?? '')
  const [reason, setReason] = useState(existing?.reason ?? '')
  const [note, setNote] = useState(existing?.note ?? '')
  const [verify, setVerify] = useState(false)
  const def = quals.find((q) => q.code === qual)
  const needsReason = state === 'suspended' || state === 'restricted'
  const save = async () => {
    if (!qual) { toast.error('Choose a qualification'); return }
    if (needsReason && !reason.trim()) { toast.error('A suspended or restricted credential needs a reason'); return }
    const body: Credential = {
      qual, level: level || undefined, state, issuer: issuer || undefined, issued: toISO(issued), expires: toISO(expires),
      restriction: restriction || undefined, reason: reason || undefined, note: note || undefined, verify: verify || undefined,
    }
    try {
      await api.put(`/members/${encodeURIComponent(memberId)}/credentials/${encodeURIComponent(qual)}`, body)
      toast.success('Credential saved')
      onDone()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Modal open wide onClose={onClose} title={existing ? `Edit ${def?.name ?? existing.qual}` : 'Add a credential'} icon={<Award size={16} className="text-sky-300" aria-hidden />}
      footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save</button></>}>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Qualification" htmlFor="cm-qual"><select id="cm-qual" className="input" disabled={!!existing} value={qual} onChange={(e) => { setQual(e.target.value); setLevel('') }}><option value="">— choose —</option>{quals.map((q) => <option key={q.code} value={q.code}>{q.name} ({q.code})</option>)}</select></Field>
        <Field label="Level" htmlFor="cm-level"><select id="cm-level" className="input" value={level} onChange={(e) => setLevel(e.target.value)}><option value="">—</option>{(def?.levels ?? []).map((l) => <option key={l} value={l}>{l}</option>)}</select></Field>
        <Field label="State" htmlFor="cm-state"><select id="cm-state" className="input" value={state} onChange={(e) => setState(e.target.value)}>{CRED_STATES.map((s) => <option key={s} value={s}>{s}</option>)}</select></Field>
        <Field label="Issuer" htmlFor="cm-issuer"><input id="cm-issuer" className="input" value={issuer} onChange={(e) => setIssuer(e.target.value)} /></Field>
        <Field label="Issued" htmlFor="cm-issued"><input id="cm-issued" type="date" className="input" value={issued} onChange={(e) => setIssued(e.target.value)} /></Field>
        <Field label="Expires" htmlFor="cm-expires" hint="Blank to compute it from the qualification's months valid."><input id="cm-expires" type="date" className="input" value={expires} onChange={(e) => setExpires(e.target.value)} /></Field>
        <div className="sm:col-span-2"><Field label="Restriction" htmlFor="cm-restr"><input id="cm-restr" className="input" value={restriction} onChange={(e) => setRestriction(e.target.value)} /></Field></div>
        {needsReason && <div className="sm:col-span-2"><Field label="Reason" htmlFor="cm-reason" hint="Required when suspended or restricted."><input id="cm-reason" className="input" value={reason} onChange={(e) => setReason(e.target.value)} /></Field></div>}
        <div className="sm:col-span-2"><Field label="Note" htmlFor="cm-note"><input id="cm-note" className="input" value={note} onChange={(e) => setNote(e.target.value)} /></Field></div>
        <label className="flex items-center gap-2 text-sm text-slate-300 sm:col-span-2"><input type="checkbox" checked={verify} onChange={(e) => setVerify(e.target.checked)} /> Mark verified</label>
      </div>
    </Modal>
  )
}

// ---- d) A note on capability ----

function CapabilityNote({ view }: { view: View }) {
  const needs = view.settings.capNeeds ?? []
  const byCap = useMemo(() => {
    const m: Record<string, CapNeed[]> = {}
    for (const n of needs) (m[n.capability] ??= []).push(n)
    return m
  }, [needs])
  return (
    <section aria-label="Capability note" className="panel rounded-lg p-4">
      <h2 className="font-semibold text-slate-100">A note on capability</h2>
      <p className="mt-1 max-w-3xl text-sm text-slate-400">A unit's live capability comes from the valid credentials of the people aboard, measured against the capability needs set in Settings. Edit these needs in Admin › Personnel.</p>
      {needs.length === 0 ? (
        <p className="mt-3 text-sm text-slate-500">No capability needs set. Any recorded crew will do.</p>
      ) : (
        <ul className="mt-3 space-y-1 text-sm text-slate-300">
          {Object.entries(byCap).map(([cap, list]) => (
            <li key={cap}>
              <span className="font-mono text-slate-100">{cap}</span> needs {list.map((n) => `${n.count} × ${n.qualification}`).join(', ')}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
