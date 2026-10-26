// Premise and safety flags administration: the agency's flag categories and who may do what with each,
// the distance a possible match is offered near an unverified location, the flag list with its states and
// history, the approve, reject, retire, restore and renew actions, and the form to add a flag against a
// verified address or a person.

import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Ban, CheckCircle2, Filter, FlagTriangleRight, Plus, RotateCcw, Save, Search, Trash2 } from 'lucide-react'
import { api, errText, ApiError } from '../api'
import { Field, Modal } from '../ui'
import { cx } from '../util'
import { useStore } from '../store'
import AddressInput from '../AddressInput'
import type { SearchResult } from '../types'
import { Page, Table } from './Admin'
import {
  DUTIES, FLAG_SCOPES, FLAG_STATES, SEVERITIES,
  asNum, dayISO, parseList, saveSettings, shortDate,
  type Flag, type FlagCategory, type FlagPolicy,
} from './FinalShared'

/** FlagsAdmin edits the flag categories and works the flag list. */
export function FlagsAdmin() {
  const view = useStore((s) => s.view)
  const live = (view?.settings as { flags?: FlagPolicy } | undefined)?.flags

  return (
    <Page
      title="Premise and safety flags"
      intro="The agency's flag categories and who may create, approve and see each, and the live flags with their review. A permanent flag is only ever put on a verified address or a named person."
    >
      <div className="space-y-4">
        <CategoriesSection live={live} />
        <FlagListSection />
      </div>
    </Page>
  )
}

// ---- a) categories ----

function CategoriesSection({ live }: { live?: FlagPolicy }) {
  const [p, setP] = useState<FlagPolicy>(live ?? {})
  const [dirty, setDirty] = useState(false)
  const [loaded, setLoaded] = useState(false)
  useEffect(() => {
    if (!dirty) setP(live ?? {})
  }, [live, dirty])

  // The server fills in the default categories when none are set; load them so the agency can edit them.
  useEffect(() => {
    if ((live?.categories ?? []).length === 0 && !loaded) {
      api.get<{ categories: FlagCategory[] }>('/flags/categories')
        .then((r) => { if (!dirty) setP((cur) => ({ ...cur, categories: r.categories ?? [] })) })
        .catch((e) => toast.error(errText(e)))
        .finally(() => setLoaded(true))
    }
  }, [live, loaded, dirty])

  const cats = p.categories ?? []
  const up = (patch: Partial<FlagPolicy>) => { setP((cur) => ({ ...cur, ...patch })); setDirty(true) }
  const setCat = (i: number, patch: Partial<FlagCategory>) => up({ categories: cats.map((c, j) => (j === i ? { ...c, ...patch } : c)) })
  const toggleDuty = (i: number, key: 'create' | 'approve' | 'view', duty: string) => {
    const cur = cats[i][key] ?? []
    const has = cur.includes(duty)
    setCat(i, { [key]: has ? cur.filter((x) => x !== duty) : [...cur, duty] })
  }

  const save = async () => {
    try {
      await saveSettings({ flags: p })
      toast.success('Flag categories saved')
      setDirty(false)
    } catch (e) {
      toast.error(errText(e))
    }
  }

  return (
    <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="flag-cat-h">
      <div className="flex items-center justify-between">
        <h2 id="flag-cat-h" className="flex items-center gap-2 font-semibold text-slate-100"><FlagTriangleRight size={15} className="text-sky-300" aria-hidden /> Flag categories</h2>
        <button className="btn btn-go" disabled={!dirty} onClick={save}><Save size={14} aria-hidden /> Save categories</button>
      </div>
      <p className="text-xs text-slate-400">Each category says what it is about, how serious it is by default, whether its detail is restricted, and which duties may create it, whose flags are active at once and approve others', and who sees a restricted flag's detail. Empty duty lists use the agency's usual rule.</p>
      <Field label="Possible match distance (m)" htmlFor="flag-near" hint="How near an unverified location a flag is offered as a possible match, never attached (0 uses 80 m).">
        <input id="flag-near" type="number" min={0} className="input w-32 font-mono" value={p.nearMetres ?? ''} onChange={(e) => up({ nearMetres: asNum(e.target.value) })} />
      </Field>
      <div className="space-y-3">
        {cats.map((c, i) => (
          <div key={i} className="rounded-md border border-slate-800 p-3">
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
              <Field label="Code" htmlFor={`fc-code-${i}`}><input id={`fc-code-${i}`} className="input h-7 font-mono text-xs" value={c.code} onChange={(e) => setCat(i, { code: e.target.value })} /></Field>
              <Field label="Name" htmlFor={`fc-name-${i}`}><input id={`fc-name-${i}`} className="input h-7 text-xs" value={c.name} onChange={(e) => setCat(i, { name: e.target.value })} /></Field>
              <Field label="Scope" htmlFor={`fc-scope-${i}`}>
                <select id={`fc-scope-${i}`} className="input h-7 text-xs" value={c.scope ?? 'address'} onChange={(e) => setCat(i, { scope: e.target.value })}>
                  {FLAG_SCOPES.map((s) => <option key={s} value={s}>{s}</option>)}
                </select>
              </Field>
              <Field label="Default severity" htmlFor={`fc-sev-${i}`}>
                <select id={`fc-sev-${i}`} className="input h-7 text-xs" value={c.severity} onChange={(e) => setCat(i, { severity: e.target.value })}>
                  {SEVERITIES.map((s) => <option key={s} value={s}>{s}</option>)}
                </select>
              </Field>
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-4">
              <label className="flex items-center gap-2 text-xs text-slate-300"><input type="checkbox" checked={!!c.restricted} onChange={(e) => setCat(i, { restricted: e.target.checked })} /> Restricted detail</label>
              <label className="flex items-center gap-2 text-xs text-slate-300"><input type="checkbox" checked={!!c.off} onChange={(e) => setCat(i, { off: e.target.checked })} /> Off</label>
              <Field label="Review (days)" htmlFor={`fc-rev-${i}`}><input id={`fc-rev-${i}`} type="number" min={0} className="input h-7 w-24 font-mono text-xs" value={c.reviewDays ?? ''} onChange={(e) => setCat(i, { reviewDays: asNum(e.target.value) })} /></Field>
              <Field label="Expire (days)" htmlFor={`fc-exp-${i}`}><input id={`fc-exp-${i}`} type="number" min={0} className="input h-7 w-24 font-mono text-xs" value={c.expireDays ?? ''} onChange={(e) => setCat(i, { expireDays: asNum(e.target.value) })} /></Field>
            </div>
            <div className="mt-2 grid gap-2 lg:grid-cols-3">
              {(['create', 'approve', 'view'] as const).map((key) => (
                <div key={key}>
                  <span className="label">{key === 'create' ? 'May create' : key === 'approve' ? 'Approve (active at once)' : 'See restricted detail'}</span>
                  <div className="flex flex-wrap gap-2">
                    {DUTIES.map((d) => (
                      <label key={d} className="flex items-center gap-1 text-xs text-slate-300">
                        <input type="checkbox" aria-label={`${c.name || c.code}: ${key} by ${d}`} checked={(c[key] ?? []).includes(d)} onChange={() => toggleDuty(i, key, d)} /> {d}
                      </label>
                    ))}
                  </div>
                </div>
              ))}
            </div>
            <div className="mt-2 flex items-end gap-2">
              <Field label="Extra fields (comma list)" htmlFor={`fc-fields-${i}`} hint="Extra details asked for, like animal, capacity, election date.">
                <input id={`fc-fields-${i}`} className="input h-7 text-xs" value={(c.fields ?? []).join(', ')} onChange={(e) => setCat(i, { fields: parseList(e.target.value) })} />
              </Field>
              <button className="btn btn-ghost text-rose-300" aria-label={`Remove category ${i + 1}`} onClick={() => up({ categories: cats.filter((_, j) => j !== i) })}><Trash2 size={13} aria-hidden /> Remove</button>
            </div>
          </div>
        ))}
      </div>
      <button className="btn" onClick={() => up({ categories: [...cats, { code: '', name: '', scope: 'address', severity: 'medium' }] })}><Plus size={13} aria-hidden /> Add a category</button>
    </section>
  )
}

// ---- b) the flag list ----

const STATE_ORDER: Record<string, number> = { proposed: 0, active: 1, retired: 2, rejected: 3 }

function FlagListSection() {
  const [state, setState] = useState('')
  const [q, setQ] = useState('')
  const [items, setItems] = useState<Flag[]>([])
  const [adding, setAdding] = useState(false)
  const [decide, setDecide] = useState<{ flag: Flag; action: string } | null>(null)

  const load = async () => {
    try {
      const r = await api.get<{ items: Flag[] }>(`/admin/flags?state=${encodeURIComponent(state)}&q=${encodeURIComponent(q)}`)
      const list = (r.items ?? []).slice()
      // Proposed first, then by last change.
      list.sort((a, b) => (STATE_ORDER[a.state] ?? 9) - (STATE_ORDER[b.state] ?? 9) || (b.changed || '').localeCompare(a.changed || ''))
      setItems(list)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  useEffect(() => {
    const t = setTimeout(load, 200)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state, q])

  const actionsFor = (f: Flag): { action: string; label: string; needNote?: boolean }[] => {
    switch (f.state) {
      case 'proposed': return [{ action: 'approve', label: 'Approve' }, { action: 'reject', label: 'Reject', needNote: true }]
      case 'active': return [{ action: 'retire', label: 'Retire', needNote: true }, { action: 'renew', label: 'Renew' }]
      case 'retired': return [{ action: 'restore', label: 'Restore' }, { action: 'renew', label: 'Renew' }]
      default: return []
    }
  }

  return (
    <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="flag-list-h">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="flag-list-h" className="flex items-center gap-2 font-semibold text-slate-100"><FlagTriangleRight size={15} className="text-sky-300" aria-hidden /> Flags</h2>
        <button className="btn btn-go" onClick={() => setAdding(true)}><Plus size={14} aria-hidden /> Add a flag</button>
      </div>
      <div className="flex flex-wrap items-end gap-2">
        <Field label="State" htmlFor="flag-state">
          <select id="flag-state" className="input h-8 text-sm" value={state} onChange={(e) => setState(e.target.value)}>
            <option value="">all states</option>
            {FLAG_STATES.map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
        </Field>
        <Field label="Search" htmlFor="flag-q">
          <div className="relative">
            <Search size={13} className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2 text-slate-500" aria-hidden />
            <input id="flag-q" className="input h-8 pl-7 text-sm" value={q} onChange={(e) => setQ(e.target.value)} placeholder="title, address or category" />
          </div>
        </Field>
        <button className="btn" onClick={load}><Filter size={13} aria-hidden /> Refresh</button>
      </div>
      <Table head={['Category', 'Title', 'Severity', 'Scope', 'State', 'Ver', 'Creator', 'Review', 'Expires', '']} empty={items.length === 0 ? <p className="p-3 text-sm text-slate-500">No flags.</p> : undefined}>
        {items.map((f) => (
          <tr key={f.id} className="align-top">
            <td className="px-3 py-1.5 text-slate-300">{f.category}</td>
            <td className="px-3">
              <div className="text-slate-100">{f.title}</div>
              <div className="text-xs text-slate-500">{f.scope.address || f.scope.personName || ''}</div>
            </td>
            <td className="px-3"><span className={cx('chip', sevClass(f.severity))}>{f.severity}</span></td>
            <td className="px-3 text-xs text-slate-400">{scopeLabel(f)}</td>
            <td className="px-3"><span className={cx('chip', stateClass(f.state))}>{f.state}</span></td>
            <td className="px-3 font-mono text-slate-400">{f.version}</td>
            <td className="px-3 text-xs text-slate-400">
              <div>{f.creator}</div>
              {f.verifier && <div className="text-slate-500">verified by {f.verifier}</div>}
            </td>
            <td className="px-3 text-xs text-slate-400">{shortDate(f.review)}</td>
            <td className="px-3 text-xs text-slate-400">{shortDate(f.expires) || 'none'}</td>
            <td className="px-3">
              <div className="flex flex-wrap gap-1">
                {actionsFor(f).map((a) => (
                  <button key={a.action} className="btn btn-ghost px-1.5 text-xs" onClick={() => setDecide({ flag: f, action: a.action })}>{a.label}</button>
                ))}
              </div>
              {(f.history ?? []).length > 0 && <div className="mt-1 text-[10px] text-slate-500">{(f.history ?? []).length} earlier version(s)</div>}
            </td>
          </tr>
        ))}
      </Table>
      {adding && <AddFlag onClose={() => setAdding(false)} onSaved={() => { setAdding(false); load() }} />}
      {decide && <DecideModal flag={decide.flag} action={decide.action} onClose={() => setDecide(null)} onDone={() => { setDecide(null); load() }} />}
    </section>
  )
}

function scopeLabel(f: Flag): string {
  const s = f.scope
  if (s.person) return `person: ${s.personName || ''}`.trim()
  const parts: string[] = []
  if (s.business) parts.push(s.business)
  if (s.complex) parts.push(s.complex)
  if (s.building) parts.push('Bldg ' + s.building)
  if (s.floor) parts.push('Floor ' + s.floor)
  if (s.unit) parts.push('Unit ' + s.unit)
  return parts.length ? parts.join(', ') : 'whole property'
}

function sevClass(sev: string): string {
  switch (sev) {
    case 'critical': return 'bg-rose-500/20 text-rose-200'
    case 'high': return 'bg-orange-500/20 text-orange-200'
    case 'medium': return 'bg-amber-400/20 text-amber-200'
    default: return 'bg-slate-700 text-slate-300'
  }
}

function stateClass(state: string): string {
  switch (state) {
    case 'active': return 'bg-emerald-500/15 text-emerald-300'
    case 'proposed': return 'bg-sky-500/15 text-sky-300'
    case 'retired': return 'bg-slate-700 text-slate-300'
    case 'rejected': return 'bg-rose-500/15 text-rose-300'
    default: return 'bg-slate-700 text-slate-300'
  }
}

// ---- decide on a flag ----

function DecideModal({ flag, action, onClose, onDone }: { flag: Flag; action: string; onClose: () => void; onDone: () => void }) {
  const [note, setNote] = useState('')
  const [expires, setExpires] = useState('')
  const [busy, setBusy] = useState(false)
  const needNote = action === 'reject' || action === 'retire'
  const canExpire = action === 'renew'
  const icon = action === 'reject' || action === 'retire' ? <Ban size={16} className="text-rose-300" aria-hidden /> : <CheckCircle2 size={16} className="text-emerald-300" aria-hidden />

  const go = async () => {
    if (needNote && !note.trim()) { toast.error('Say why'); return }
    setBusy(true)
    try {
      const body: Record<string, unknown> = { action, note: note.trim() }
      if (canExpire && expires) body.expires = dayISO(expires)
      await api.post(`/flags/${encodeURIComponent(flag.id)}/decide`, body)
      toast.success(`Flag ${action}d`)
      onDone()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      title={`${action[0].toUpperCase()}${action.slice(1)} flag ${flag.id}`}
      icon={icon}
      footer={<><button className="btn btn-ghost" onClick={onClose}>Cancel</button><button className="btn btn-go" disabled={busy} onClick={go}>{action[0].toUpperCase()}{action.slice(1)}</button></>}
    >
      <div className="space-y-3">
        <p className="text-sm text-slate-300">{flag.title}</p>
        <Field label={`Note ${needNote ? '(required)' : '(optional)'}`} htmlFor="decide-note">
          <textarea id="decide-note" className="input min-h-[3rem]" value={note} onChange={(e) => setNote(e.target.value)} />
        </Field>
        {canExpire && (
          <Field label="New expiry date (optional)" htmlFor="decide-expires">
            <input id="decide-expires" type="date" className="input" value={expires} onChange={(e) => setExpires(e.target.value)} />
          </Field>
        )}
      </div>
    </Modal>
  )
}

// ---- add a flag ----

type Draft = {
  category: string
  title: string
  warning: string
  instruction: string
  severity: string
  source: string
  reliability: string
  addressText: string
  verified: boolean
  lat?: number
  lon?: number
  business: string
  complex: string
  building: string
  floor: string
  unit: string
  person: string
  personName: string
  expires: string
  details: Record<string, string>
}

const blank = (): Draft => ({
  category: '', title: '', warning: '', instruction: '', severity: '', source: '', reliability: 'reported',
  addressText: '', verified: false, business: '', complex: '', building: '', floor: '', unit: '',
  person: '', personName: '', expires: '', details: {},
})

function AddFlag({ onClose, onSaved }: { onClose: () => void; onSaved: () => void }) {
  const view = useStore((s) => s.view)
  const live = (view?.settings as { flags?: FlagPolicy } | undefined)?.flags
  const [cats, setCats] = useState<FlagCategory[]>(live?.categories ?? [])
  const [d, setD] = useState<Draft>(blank())
  const [busy, setBusy] = useState(false)
  const [conflict, setConflict] = useState<string | null>(null)

  useEffect(() => {
    if ((live?.categories ?? []).length === 0) {
      api.get<{ categories: FlagCategory[] }>('/flags/categories').then((r) => setCats(r.categories ?? [])).catch(() => {})
    }
  }, [live])

  const set = (p: Partial<Draft>) => setD((cur) => ({ ...cur, ...p }))
  const cat = cats.find((c) => c.code === d.category)
  const isPerson = cat?.scope === 'person' || (cat?.scope === 'both' && !!d.person)

  const pick = (r: SearchResult) => set({ addressText: r.label, verified: true, lat: r.lat, lon: r.lon })
  const onAddrChange = (v: string) => set({ addressText: v, verified: false, lat: undefined, lon: undefined })

  const save = async (conflictOk: boolean) => {
    if (!d.category) { toast.error('Choose a category'); return }
    if (!d.title.trim()) { toast.error('Give the flag a title'); return }
    if (!d.warning.trim()) { toast.error('Say what the concern is and why'); return }
    if (!d.instruction.trim()) { toast.error('Say what responders should do'); return }
    if (!isPerson && !d.verified) {
      toast.error('A permanent flag needs a verified address: pick one from the address search')
      return
    }
    if (isPerson && !d.personName.trim()) { toast.error('Name the person the flag is about'); return }
    setBusy(true)
    setConflict(null)
    try {
      const scope: Record<string, unknown> = isPerson
        ? { person: d.person || undefined, personName: d.personName.trim() }
        : { address: d.addressText.trim(), verified: true, lat: d.lat, lon: d.lon, business: d.business || undefined, complex: d.complex || undefined, building: d.building || undefined, floor: d.floor || undefined, unit: d.unit || undefined }
      const body: Record<string, unknown> = {
        category: d.category,
        title: d.title.trim(),
        warning: d.warning.trim(),
        instruction: d.instruction.trim(),
        severity: d.severity || undefined,
        source: d.source || undefined,
        reliability: d.reliability || undefined,
        scope,
        details: Object.keys(d.details).length ? d.details : undefined,
        expires: d.expires ? dayISO(d.expires) : undefined,
        conflictOk,
      }
      await api.put('/flags', body)
      toast.success('Flag saved')
      onSaved()
    } catch (e) {
      if (e instanceof ApiError && e.status === 422 && e.field === 'conflict') {
        setConflict(e.message)
      } else {
        toast.error(errText(e))
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      wide
      title="Add a flag"
      icon={<FlagTriangleRight size={16} className="text-sky-300" aria-hidden />}
      footer={<>
        <button className="btn btn-ghost" onClick={onClose}>Cancel</button>
        <button className="btn btn-go" disabled={busy} onClick={() => save(false)}><Save size={14} aria-hidden /> Save flag</button>
      </>}
    >
      <div className="space-y-3">
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Category" htmlFor="af-cat">
            <select id="af-cat" className="input" value={d.category} onChange={(e) => set({ category: e.target.value, details: {} })}>
              <option value="">choose a category</option>
              {cats.filter((c) => !c.off).map((c) => <option key={c.code} value={c.code}>{c.name}</option>)}
            </select>
          </Field>
          <Field label="Severity" htmlFor="af-sev" hint={cat ? `Default for ${cat.name}: ${cat.severity}` : undefined}>
            <select id="af-sev" className="input" value={d.severity} onChange={(e) => set({ severity: e.target.value })}>
              <option value="">use the category default</option>
              {SEVERITIES.map((s) => <option key={s} value={s}>{s}</option>)}
            </select>
          </Field>
        </div>
        <Field label="Title" htmlFor="af-title"><input id="af-title" className="input" value={d.title} onChange={(e) => set({ title: e.target.value })} /></Field>
        <Field label="Warning (what the concern is and why it applies)" htmlFor="af-warn"><textarea id="af-warn" className="input min-h-[3rem]" value={d.warning} onChange={(e) => set({ warning: e.target.value })} /></Field>
        <Field label="Instruction (what responders should do)" htmlFor="af-inst"><textarea id="af-inst" className="input min-h-[3rem]" value={d.instruction} onChange={(e) => set({ instruction: e.target.value })} /></Field>
        <div className="grid gap-3 sm:grid-cols-2">
          <Field label="Source (how it was reported)" htmlFor="af-src"><input id="af-src" className="input" value={d.source} onChange={(e) => set({ source: e.target.value })} /></Field>
          <Field label="Reliability" htmlFor="af-rel">
            <select id="af-rel" className="input" value={d.reliability} onChange={(e) => set({ reliability: e.target.value })}>
              <option value="verified">verified</option>
              <option value="reported">reported</option>
              <option value="unconfirmed">unconfirmed</option>
            </select>
          </Field>
        </div>

        {cat?.scope === 'both' && (
          <p className="text-xs text-slate-400">This category can be about a place or a person. Fill the person's name to make it a person flag, or leave it blank and choose an address.</p>
        )}

        {!isPerson && (
          <div className="space-y-2 rounded-md border border-slate-800 p-3">
            <Field label="Address" htmlFor="af-addr" hint="Pick an address from the agency's search. Only a picked address counts as verified." error={!d.verified && d.addressText.trim() ? 'A permanent flag needs a verified address: pick one from the results.' : undefined}>
              <AddressInput id="af-addr" value={d.addressText} onChange={onAddrChange} onPick={pick} placeholder="Start typing an address" />
            </Field>
            {d.verified && <p className="text-xs text-emerald-300">Verified address chosen.</p>}
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              <Field label="Business or complex" htmlFor="af-biz"><input id="af-biz" className="input h-8 text-sm" value={d.business} onChange={(e) => set({ business: e.target.value })} /></Field>
              <Field label="Complex or campus" htmlFor="af-cx"><input id="af-cx" className="input h-8 text-sm" value={d.complex} onChange={(e) => set({ complex: e.target.value })} /></Field>
              <Field label="Building" htmlFor="af-bld"><input id="af-bld" className="input h-8 text-sm" value={d.building} onChange={(e) => set({ building: e.target.value })} /></Field>
              <Field label="Floor" htmlFor="af-flr"><input id="af-flr" className="input h-8 text-sm" value={d.floor} onChange={(e) => set({ floor: e.target.value })} /></Field>
              <Field label="Unit" htmlFor="af-unit"><input id="af-unit" className="input h-8 text-sm" value={d.unit} onChange={(e) => set({ unit: e.target.value })} /></Field>
            </div>
          </div>
        )}

        {cat && (cat.scope === 'person' || cat.scope === 'both') && (
          <div className="grid gap-2 sm:grid-cols-2">
            <Field label="Person's name" htmlFor="af-pn"><input id="af-pn" className="input h-8 text-sm" value={d.personName} onChange={(e) => set({ personName: e.target.value })} placeholder="LAST, FIRST" /></Field>
            <Field label="Person key (optional)" htmlFor="af-pk" hint="Leave blank for a new person."><input id="af-pk" className="input h-8 font-mono text-sm" value={d.person} onChange={(e) => set({ person: e.target.value })} /></Field>
          </div>
        )}

        {(cat?.fields ?? []).length > 0 && (
          <div className="space-y-2 rounded-md border border-slate-800 p-3">
            <span className="label">{cat?.name} details</span>
            <div className="grid gap-2 sm:grid-cols-2">
              {(cat?.fields ?? []).map((f) => (
                <Field key={f} label={f} htmlFor={`af-d-${f}`}>
                  <input id={`af-d-${f}`} className="input h-8 text-sm" value={d.details[f] ?? ''} onChange={(e) => set({ details: { ...d.details, [f]: e.target.value } })} />
                </Field>
              ))}
            </div>
          </div>
        )}

        <Field label="Expiry date (optional)" htmlFor="af-exp" hint="Leave blank to use the category's rule."><input id="af-exp" type="date" className="input w-48" value={d.expires} onChange={(e) => set({ expires: e.target.value })} /></Field>

        {conflict && (
          <div className="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-200">
            <p>{conflict}</p>
            <button className="btn mt-2" disabled={busy} onClick={() => save(true)}><RotateCcw size={13} aria-hidden /> Save as a separate flag</button>
          </div>
        )}
      </div>
    </Modal>
  )
}
