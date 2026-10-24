import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { Copy, Eye, KeyRound, MessageSquare, Plus, Save, ShieldCheck, Terminal, Trash2, Users, Volume2 } from 'lucide-react'
import { api, errText } from '../api'
import { Field } from '../ui'
import type {
  CommKind, CommandRules, KeyBinding, KeyProfile, MultiCall, Preset, PriorityRule,
  Settings, StaffMinimum, StaffingRules, StatusCode, View, ViewDef, ViewRule,
} from '../types'
import { Page, Table } from './Admin'

// The duties the console serves, for per-duty settings (max working calls, key profiles).
const DUTIES = ['calltaker', 'police', 'fire', 'ems', 'dispatcher', 'supervisor'] as const

// The working-call defaults when a per-duty max is blank.
const DEFAULT_MAX: Record<string, number> = { calltaker: 3, supervisor: 4, police: 2, fire: 2, ems: 2, dispatcher: 2 }

// The priority levels communications use, in order.
const COMM_LEVELS: PriorityRule['level'][] = ['urgent', 'high', 'medium', 'low']
const TONES: PriorityRule['tone'][] = ['alarm', 'alert', 'chime', 'tick', 'none']

// The communication kinds offered when none are configured yet (noExpiry everywhere).
const DEFAULT_COMM_KINDS: CommKind[] = [
  { code: 'message', name: 'Message', priority: 'medium', send: ['field', 'dispatcher', 'supervisor', 'admin'], hours: 24, noExpiry: true },
  { code: 'bolo', name: 'BOLO', priority: 'high', hours: 72, noExpiry: true, waits: true },
  { code: 'officer-safety', name: 'Officer safety', priority: 'urgent', hours: 168, noExpiry: true, waits: true },
  { code: 'admin', name: 'Administrative', priority: 'low', send: ['supervisor', 'admin'], hours: 336, noExpiry: true, waits: true },
]

// The priority-rule defaults.
const DEFAULT_PRIORITY_RULES: PriorityRule[] = [
  { level: 'urgent', tone: 'alarm', repeat: 10, ack: true, escalate: 2 },
  { level: 'high', tone: 'alert', repeat: 60, ack: true, escalate: 10 },
  { level: 'medium', tone: 'chime', silenceable: true },
  { level: 'low', tone: 'tick', silenceable: true },
]

const SHORT_HELP: Record<string, string> = {
  warn: 'warn — note the shortage but allow it',
  limited: 'limited — mark the unit limited',
  oos: 'out of service — hold the unit out until crewed',
}
const DISPATCH_HELP: Record<string, string> = {
  warn: 'warn — note it on dispatch',
  override: 'override — allow it with a reason',
  block: 'block — refuse the dispatch',
}

// A small WebAudio beep so an administrator can hear a tone. Kept inline; nothing is loaded.
function playTone(tone: PriorityRule['tone']) {
  if (tone === 'none') return
  type WithWebkit = typeof window & { webkitAudioContext?: typeof AudioContext }
  const Ctx = window.AudioContext ?? (window as WithWebkit).webkitAudioContext
  if (!Ctx) return
  const ctx = new Ctx()
  const shape: Record<string, { freq: number; dur: number; type: OscillatorType; beeps: number }> = {
    alarm: { freq: 880, dur: 0.18, type: 'square', beeps: 3 },
    alert: { freq: 660, dur: 0.16, type: 'sawtooth', beeps: 2 },
    chime: { freq: 520, dur: 0.22, type: 'sine', beeps: 1 },
    tick: { freq: 420, dur: 0.06, type: 'sine', beeps: 1 },
  }
  const s = shape[tone] ?? shape.chime
  let t = ctx.currentTime
  for (let i = 0; i < s.beeps; i++) {
    const osc = ctx.createOscillator()
    const gain = ctx.createGain()
    osc.type = s.type
    osc.frequency.value = s.freq
    gain.gain.setValueAtTime(0.0001, t)
    gain.gain.exponentialRampToValueAtTime(0.25, t + 0.01)
    gain.gain.exponentialRampToValueAtTime(0.0001, t + s.dur)
    osc.connect(gain).connect(ctx.destination)
    osc.start(t)
    osc.stop(t + s.dur)
    t += s.dur + 0.08
  }
  window.setTimeout(() => ctx.close().catch(() => {}), (t - ctx.currentTime) * 1000 + 200)
}

const parseList = (s: string) => s.split(',').map((x) => x.trim()).filter(Boolean)
const asNum = (s: string): number | undefined => (s.trim() === '' ? undefined : Number(s))

/** OperationsRules edits the parts of settings that shape how the console runs: which supervisory views
 * exist, the assigned duty presets, working calls, communications, the staffing policy, and the command
 * line and key profiles. The whole settings object is saved each time. */
export function OperationsRules({ view }: { view: View }) {
  const [s, setS] = useState<Settings>(view.settings)
  const [dirty, setDirty] = useState(false)
  useEffect(() => {
    if (!dirty) setS(view.settings)
  }, [view.settings, dirty])

  const up = (p: Partial<Settings>) => {
    setS((cur) => ({ ...cur, ...p }))
    setDirty(true)
  }

  const save = async () => {
    try {
      await api.put('/admin/settings', { ...view.settings, ...s })
      toast.success('Operations rules saved')
      setDirty(false)
    } catch (e) {
      toast.error(errText(e))
    }
  }

  return (
    <Page
      title="Operations rules"
      intro="How the console runs: the supervisory views, the assigned duty presets, working calls, communications, the staffing policy, and the command line and keys. Changes are saved together and recorded in the ledger."
      actions={<button className="btn btn-go" disabled={!dirty} onClick={save}><Save size={14} aria-hidden /> Save</button>}
    >
      <div className="space-y-4">
        <ViewsSection s={s} up={up} />
        <PresetsSection />
        <MultiCallSection s={s} up={up} />
        <CommsSection s={s} up={up} />
        <StaffingSection s={s} up={up} view={view} />
        <CommandsSection s={s} up={up} view={view} />
      </div>
    </Page>
  )
}

// ---- a) Supervisory views ----

type ViewRow = { id: string; name: string; route: string; off: boolean; roles: string[]; users: string; agencies: string }

function ViewsSection({ s, up }: { s: Settings; up: (p: Partial<Settings>) => void }) {
  const [defs, setDefs] = useState<ViewDef[]>([])
  useEffect(() => {
    api.get<{ items: ViewDef[]; all: ViewDef[] }>('/me/views')
      .then((r) => setDefs(r.all ?? []))
      .catch((e) => toast.error(errText(e)))
  }, [])

  // The rows, seeded from any rule already set.
  const rules = s.views ?? []
  const rows: ViewRow[] = defs.map((v) => {
    const rule = rules.find((r) => r.id === v.id)
    return {
      id: v.id,
      name: v.name,
      route: v.route,
      off: !!rule?.off,
      roles: rule?.roles ?? [],
      users: (rule?.users ?? []).join(', '),
      agencies: (rule?.agencies ?? []).join(', '),
    }
  })

  // Turn the rows back into rules, omitting rows left at default (no rule).
  const commit = (next: ViewRow[]) => {
    const out: ViewRule[] = []
    for (const r of next) {
      const users = parseList(r.users)
      const agencies = parseList(r.agencies)
      if (!r.off && r.roles.length === 0 && users.length === 0 && agencies.length === 0) continue
      out.push({ id: r.id, off: r.off || undefined, roles: r.roles.length ? r.roles : undefined, users: users.length ? users : undefined, agencies: agencies.length ? agencies : undefined })
    }
    up({ views: out })
  }

  const setRow = (id: string, p: Partial<ViewRow>) => commit(rows.map((r) => (r.id === id ? { ...r, ...p } : r)))
  const toggleRole = (id: string, role: string) => {
    const r = rows.find((x) => x.id === id)
    if (!r) return
    const has = r.roles.includes(role)
    setRow(id, { roles: has ? r.roles.filter((x) => x !== role) : [...r.roles, role] })
  }

  return (
    <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="views-h">
      <h2 id="views-h" className="flex items-center gap-2 font-semibold text-slate-100"><Eye size={15} className="text-sky-300" aria-hidden /> Supervisory views</h2>
      <p className="text-xs text-slate-400">Administrators always see every view. A view with no rule is for supervisors. The server refuses a direct link without the right.</p>
      <Table head={['View', 'Off', 'Roles', 'Named users', 'Agencies']} empty={rows.length === 0 ? <p className="p-3 text-sm text-slate-500">No views.</p> : undefined}>
        {rows.map((r) => (
          <tr key={r.id} className="align-top">
            <td className="px-3 py-1.5">
              <div className="text-slate-100">{r.name}</div>
              <div className="font-mono text-xs text-slate-500">{r.route}</div>
            </td>
            <td className="px-3 text-center">
              <input type="checkbox" aria-label={`Turn off ${r.name}`} checked={r.off} onChange={(e) => setRow(r.id, { off: e.target.checked })} />
            </td>
            <td className="px-3">
              <div className="flex flex-wrap gap-2">
                {['supervisor', 'dispatcher', 'viewer'].map((role) => (
                  <label key={role} className="flex items-center gap-1 text-xs text-slate-300">
                    <input type="checkbox" aria-label={`${r.name} for ${role}`} checked={r.roles.includes(role)} onChange={() => toggleRole(r.id, role)} /> {role}
                  </label>
                ))}
              </div>
            </td>
            <td className="px-3">
              <input className="input h-7 text-xs" aria-label={`Named users for ${r.name}`} value={r.users} onChange={(e) => setRow(r.id, { users: e.target.value })} placeholder="usernames" />
            </td>
            <td className="px-3">
              <input className="input h-7 font-mono text-xs" aria-label={`Agencies for ${r.name}`} value={r.agencies} onChange={(e) => setRow(r.id, { agencies: e.target.value })} placeholder="codes" />
            </td>
          </tr>
        ))}
      </Table>
    </section>
  )
}

// ---- b) Assigned duty presets ----

function PresetsSection() {
  const [items, setItems] = useState<Preset[]>([])
  const load = async () => {
    const r = await api.get<{ items: Preset[] }>('/admin/presets')
    setItems(r.items ?? [])
  }
  useEffect(() => {
    load().catch((e) => toast.error(errText(e)))
  }, [])

  const toggleProtect = async (p: Preset) => {
    try {
      await api.put(`/admin/presets/${encodeURIComponent(p.user)}/${encodeURIComponent(p.duty)}`, { protected: !p.protected })
      toast.success(`${p.protected ? 'Unprotected' : 'Protected'} ${p.user} / ${p.duty}`)
      await load()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  return (
    <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="presets-h">
      <h2 id="presets-h" className="flex items-center gap-2 font-semibold text-slate-100"><Users size={15} className="text-sky-300" aria-hidden /> Assigned duty presets</h2>
      <p className="text-xs text-slate-400">The preset marked <span className="font-mono">@assigned</span> is the agency's preset for a duty; the rest are people's own. An administrator makes the assigned preset by setting up the console as wanted and choosing "Save as the agency's preset" on the console.</p>
      <Table head={['User', 'Duty', 'Version', 'Updated', 'By', 'Protected', '']} empty={items.length === 0 ? <p className="p-3 text-sm text-slate-500">No presets yet.</p> : undefined}>
        {items.map((p) => {
          const assigned = p.user === '@assigned'
          return (
            <tr key={`${p.user}/${p.duty}`} className="hover:bg-slate-800/50">
              <td className="px-3 py-1.5 font-mono text-slate-100">{p.user}</td>
              <td className="px-3 text-slate-300">{p.duty}</td>
              <td className="px-3 font-mono text-slate-400">{p.version}</td>
              <td className="px-3 text-xs text-slate-400">{p.updated}</td>
              <td className="px-3 text-slate-400">{p.by}</td>
              <td className="px-3 text-xs">{p.protected ? <span className="text-emerald-300">protected</span> : <span className="text-slate-500">—</span>}</td>
              <td className="px-3">
                {!assigned && (
                  <button className="btn" onClick={() => toggleProtect(p)}>
                    <ShieldCheck size={12} aria-hidden /> {p.protected ? 'Unprotect' : 'Protect'}
                  </button>
                )}
              </td>
            </tr>
          )
        })}
      </Table>
    </section>
  )
}

// ---- c) Working calls ----

function MultiCallSection({ s, up }: { s: Settings; up: (p: Partial<Settings>) => void }) {
  const mc = s.multiCall ?? {}
  const setMc = (p: Partial<MultiCall>) => up({ multiCall: { ...mc, ...p } })
  const setMax = (duty: string, v: string) => {
    const max = { ...(mc.max ?? {}) }
    if (v.trim() === '') delete max[duty]
    else max[duty] = Number(v)
    setMc({ max })
  }
  return (
    <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="mc-h">
      <h2 id="mc-h" className="flex items-center gap-2 font-semibold text-slate-100"><MessageSquare size={15} className="text-sky-300" aria-hidden /> Working calls</h2>
      <p className="text-xs text-slate-400">How many calls a person may have open at once. Blank uses the default: call-taker 3, supervisor 4, others 2.</p>
      <label className="flex items-center gap-2 text-sm text-slate-300">
        <input type="checkbox" checked={!!mc.off} onChange={(e) => setMc({ off: e.target.checked })} /> Off (only one working call each)
      </label>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-6">
        {DUTIES.map((duty) => (
          <Field key={duty} label={duty} htmlFor={`mc-${duty}`} hint={`default ${DEFAULT_MAX[duty]}`}>
            <input id={`mc-${duty}`} type="number" min={1} max={9} className="input font-mono" value={mc.max?.[duty] ?? ''} onChange={(e) => setMax(duty, e.target.value)} />
          </Field>
        ))}
      </div>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Field label="Trainee max" htmlFor="mc-trainee" hint="How many a trainee may have">
          <input id="mc-trainee" type="number" min={1} max={9} className="input font-mono" value={mc.traineeMax ?? ''} onChange={(e) => setMc({ traineeMax: asNum(e.target.value) })} />
        </Field>
        <Field label="Abandon after (minutes)" htmlFor="mc-abandon" hint="An untouched working call is dropped">
          <input id="mc-abandon" type="number" min={1} className="input font-mono" value={mc.abandonMinutes ?? ''} onChange={(e) => setMc({ abandonMinutes: asNum(e.target.value) })} />
        </Field>
      </div>
    </section>
  )
}

// ---- d) Communications ----

function CommsSection({ s, up }: { s: Settings; up: (p: Partial<Settings>) => void }) {
  const kinds = s.commKinds ?? []
  const setKinds = (next: CommKind[]) => up({ commKinds: next })
  const setKind = (i: number, p: Partial<CommKind>) => setKinds(kinds.map((k, j) => (j === i ? { ...k, ...p } : k)))

  const rules = s.priorityRules ?? DEFAULT_PRIORITY_RULES
  const ruleFor = (level: PriorityRule['level']) => rules.find((r) => r.level === level) ?? { level, tone: 'chime' as const }
  const setRule = (level: PriorityRule['level'], p: Partial<PriorityRule>) => {
    const next = COMM_LEVELS.map((lv) => {
      const cur = ruleFor(lv)
      return lv === level ? { ...cur, ...p } : cur
    })
    up({ priorityRules: next })
  }

  const fileTypes = (s.commFileTypes ?? []).join(', ')

  return (
    <section className="panel space-y-4 rounded-lg p-4" aria-labelledby="comms-h">
      <h2 id="comms-h" className="flex items-center gap-2 font-semibold text-slate-100"><MessageSquare size={15} className="text-sky-300" aria-hidden /> Communications</h2>

      {/* Kinds */}
      <div className="space-y-2">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500">Kinds</h3>
        {kinds.length === 0 ? (
          <div className="rounded-md border border-slate-800 p-3 text-sm text-slate-400">
            <p>No communication kinds are set. Start from the usual defaults, then edit.</p>
            <button className="btn mt-2" onClick={() => setKinds(DEFAULT_COMM_KINDS.map((k) => ({ ...k, send: k.send ? [...k.send] : undefined })))}><Plus size={13} aria-hidden /> Start from defaults</button>
          </div>
        ) : (
          <Table head={['Code', 'Name', 'Priority', 'Sends to', 'Hours', 'No expiry', 'Waits', '']}>
            {kinds.map((k, i) => (
              <tr key={i} className="align-top">
                <td className="px-3 py-1"><input className="input h-7 font-mono text-xs" aria-label={`Code, row ${i + 1}`} value={k.code} onChange={(e) => setKind(i, { code: e.target.value })} /></td>
                <td className="px-3"><input className="input h-7 text-xs" aria-label={`Name, row ${i + 1}`} value={k.name} onChange={(e) => setKind(i, { name: e.target.value })} /></td>
                <td className="px-3">
                  <select className="input h-7 text-xs" aria-label={`Priority, row ${i + 1}`} value={k.priority ?? 'medium'} onChange={(e) => setKind(i, { priority: e.target.value as CommKind['priority'] })}>
                    {COMM_LEVELS.map((lv) => <option key={lv} value={lv}>{lv}</option>)}
                  </select>
                </td>
                <td className="px-3"><input className="input h-7 text-xs" aria-label={`Sends to, row ${i + 1}`} value={(k.send ?? []).join(', ')} onChange={(e) => setKind(i, { send: parseList(e.target.value) })} placeholder="field, dispatcher…" /></td>
                <td className="px-3"><input type="number" min={0} className="input h-7 w-16 font-mono text-xs" aria-label={`Hours, row ${i + 1}`} value={k.hours ?? ''} onChange={(e) => setKind(i, { hours: asNum(e.target.value) })} /></td>
                <td className="px-3 text-center"><input type="checkbox" aria-label={`No expiry, row ${i + 1}`} checked={!!k.noExpiry} onChange={(e) => setKind(i, { noExpiry: e.target.checked })} /></td>
                <td className="px-3 text-center"><input type="checkbox" aria-label={`Waits, row ${i + 1}`} checked={!!k.waits} onChange={(e) => setKind(i, { waits: e.target.checked })} /></td>
                <td className="px-3"><button className="btn btn-ghost px-1.5" aria-label={`Remove row ${i + 1}`} onClick={() => setKinds(kinds.filter((_, j) => j !== i))}><Trash2 size={13} aria-hidden /></button></td>
              </tr>
            ))}
          </Table>
        )}
        {kinds.length > 0 && <button className="btn" onClick={() => setKinds([...kinds, { code: '', name: '', priority: 'medium', noExpiry: true }])}><Plus size={13} aria-hidden /> Add a kind</button>}
      </div>

      {/* Priority rules */}
      <div className="space-y-2">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500">Priority rules</h3>
        <Table head={['Level', 'Tone', '', 'Repeat (s)', 'Ack', 'Escalate (m)', 'Silenceable']}>
          {COMM_LEVELS.map((level) => {
            const r = ruleFor(level)
            return (
              <tr key={level} className="align-top">
                <td className="px-3 py-1.5 text-slate-200">{level}</td>
                <td className="px-3">
                  <select className="input h-7 text-xs" aria-label={`Tone for ${level}`} value={r.tone} onChange={(e) => setRule(level, { tone: e.target.value as PriorityRule['tone'] })}>
                    {TONES.map((t) => <option key={t} value={t}>{t}</option>)}
                  </select>
                </td>
                <td className="px-3"><button className="btn" aria-label={`Play the ${r.tone} tone for ${level}`} onClick={() => playTone(r.tone)}><Volume2 size={12} aria-hidden /> Play</button></td>
                <td className="px-3"><input type="number" min={0} className="input h-7 w-20 font-mono text-xs" aria-label={`Repeat seconds for ${level}`} value={r.repeat ?? ''} onChange={(e) => setRule(level, { repeat: asNum(e.target.value) })} /></td>
                <td className="px-3 text-center"><input type="checkbox" aria-label={`Acknowledge ${level}`} checked={!!r.ack} onChange={(e) => setRule(level, { ack: e.target.checked })} /></td>
                <td className="px-3"><input type="number" min={0} className="input h-7 w-20 font-mono text-xs" aria-label={`Escalate minutes for ${level}`} value={r.escalate ?? ''} onChange={(e) => setRule(level, { escalate: asNum(e.target.value) })} /></td>
                <td className="px-3 text-center"><input type="checkbox" aria-label={`Silenceable ${level}`} checked={!!r.silenceable} onChange={(e) => setRule(level, { silenceable: e.target.checked })} /></td>
              </tr>
            )
          })}
        </Table>
      </div>

      {/* Attachments */}
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Attachment types" htmlFor="comm-filetypes" hint="A comma list of MIME types; blank allows pictures, PDF, text, audio and video">
          <input id="comm-filetypes" className="input font-mono" value={fileTypes} onChange={(e) => up({ commFileTypes: parseList(e.target.value) })} placeholder="image/*, application/pdf" />
        </Field>
        <Field label="Largest attachment (MB)" htmlFor="comm-filemb">
          <input id="comm-filemb" type="number" min={1} className="input font-mono" value={s.commFileMB ?? ''} onChange={(e) => up({ commFileMB: asNum(e.target.value) })} />
        </Field>
      </div>
    </section>
  )
}

// ---- e) Staffing policy ----

function StaffingSection({ s, up, view }: { s: Settings; up: (p: Partial<Settings>) => void; view: View }) {
  const st = s.staffing ?? {}
  const setSt = (p: Partial<StaffingRules>) => up({ staffing: { ...st, ...p } })
  const mins = st.minimums ?? []
  const setMin = (i: number, p: Partial<StaffMinimum>) => setSt({ minimums: mins.map((m, j) => (j === i ? { ...m, ...p } : m)) })
  const quals = view.settings.qualifications ?? []

  return (
    <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="staffing-h">
      <h2 id="staffing-h" className="flex items-center gap-2 font-semibold text-slate-100"><Users size={15} className="text-sky-300" aria-hidden /> Staffing policy</h2>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="When a unit is short-crewed" htmlFor="st-short" hint={SHORT_HELP[st.short ?? ''] ?? 'Choose what happens when a unit is below its minimum crew'}>
          <select id="st-short" className="input" value={st.short ?? ''} onChange={(e) => setSt({ short: e.target.value as StaffingRules['short'] })}>
            <option value="">no policy</option>
            <option value="warn">warn</option>
            <option value="limited">limited</option>
            <option value="oos">out of service</option>
          </select>
        </Field>
        <Field label="Dispatching a short unit" htmlFor="st-dispatch" hint={DISPATCH_HELP[st.dispatch ?? ''] ?? 'Choose what happens when a short unit is dispatched'}>
          <select id="st-dispatch" className="input" value={st.dispatch ?? ''} onChange={(e) => setSt({ dispatch: e.target.value as StaffingRules['dispatch'] })}>
            <option value="">no policy</option>
            <option value="warn">warn</option>
            <option value="override">override</option>
            <option value="block">block</option>
          </select>
        </Field>
      </div>

      <div className="space-y-2">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500">Minimums on duty</h3>
        <Table head={['Qualification', 'Count', 'Agency', 'Note', '']} empty={mins.length === 0 ? <p className="p-3 text-sm text-slate-500">No minimums set.</p> : undefined}>
          {mins.map((m, i) => (
            <tr key={i} className="align-top">
              <td className="px-3 py-1">
                <input className="input h-7 font-mono text-xs" list="st-quals" aria-label={`Qualification, row ${i + 1}`} value={m.qual} onChange={(e) => setMin(i, { qual: e.target.value })} />
              </td>
              <td className="px-3"><input type="number" min={0} className="input h-7 w-16 font-mono text-xs" aria-label={`Count, row ${i + 1}`} value={m.count} onChange={(e) => setMin(i, { count: Number(e.target.value) })} /></td>
              <td className="px-3"><input className="input h-7 w-24 font-mono text-xs" aria-label={`Agency, row ${i + 1}`} value={m.agency ?? ''} onChange={(e) => setMin(i, { agency: e.target.value || undefined })} placeholder="any" /></td>
              <td className="px-3"><input className="input h-7 text-xs" aria-label={`Note, row ${i + 1}`} value={m.note ?? ''} onChange={(e) => setMin(i, { note: e.target.value || undefined })} /></td>
              <td className="px-3"><button className="btn btn-ghost px-1.5" aria-label={`Remove row ${i + 1}`} onClick={() => setSt({ minimums: mins.filter((_, j) => j !== i) })}><Trash2 size={13} aria-hidden /></button></td>
            </tr>
          ))}
        </Table>
        <datalist id="st-quals">
          {quals.map((q) => <option key={q} value={q} />)}
        </datalist>
        <button className="btn" onClick={() => setSt({ minimums: [...mins, { qual: '', count: 1 }] })}><Plus size={13} aria-hidden /> Add a minimum</button>
      </div>
    </section>
  )
}

// ---- f) Command line and keys ----

function CommandsSection({ s, up, view }: { s: Settings; up: (p: Partial<Settings>) => void; view: View }) {
  const c = s.commands ?? {}
  const setC = (p: Partial<CommandRules>) => up({ commands: { ...c, ...p } })

  const statusCodes = c.statusCodes ?? []
  const setStatusCode = (i: number, p: Partial<StatusCode>) => setC({ statusCodes: statusCodes.map((x, j) => (j === i ? { ...x, ...p } : x)) })

  // Key profiles.
  const [stock, setStock] = useState<KeyProfile[]>([])
  const [actions, setActions] = useState<Record<string, string>>({})
  const [checking, setChecking] = useState(false)
  const [problems, setProblems] = useState<string[] | null>(null)
  const [printId, setPrintId] = useState('')

  useEffect(() => {
    api.get<{ stock: KeyProfile[]; actions: Record<string, string>; statusCodes: StatusCode[] }>('/admin/keys')
      .then((r) => { setStock(r.stock ?? []); setActions(r.actions ?? {}) })
      .catch((e) => toast.error(errText(e)))
  }, [])

  const profiles = c.keyProfiles ?? []
  const allProfiles = useMemo(() => [...stock, ...profiles], [stock, profiles])
  const actionList = Object.keys(actions)

  const copyStock = (p: KeyProfile) => {
    const id = `${p.id}-copy`
    if (profiles.some((x) => x.id === id)) { toast.error('Already copied'); return }
    setC({ keyProfiles: [...profiles, { id, name: `${p.name} (copy)`, bindings: p.bindings.map((b) => ({ ...b })) }] })
    toast.success(`Copied ${p.name}`)
  }
  const setProfile = (pi: number, p: Partial<KeyProfile>) => setC({ keyProfiles: profiles.map((x, j) => (j === pi ? { ...x, ...p } : x)) })
  const setBinding = (pi: number, bi: number, p: Partial<KeyBinding>) =>
    setProfile(pi, { bindings: profiles[pi].bindings.map((b, j) => (j === bi ? { ...b, ...p } : b)) })
  const addBinding = (pi: number) => setProfile(pi, { bindings: [...profiles[pi].bindings, { key: '', action: '' }] })
  const removeBinding = (pi: number, bi: number) => setProfile(pi, { bindings: profiles[pi].bindings.filter((_, j) => j !== bi) })
  const removeProfile = (pi: number) => setC({ keyProfiles: profiles.filter((_, j) => j !== pi) })

  const setDutyKey = (duty: string, profileId: string) => {
    const keys = { ...(c.keys ?? {}) }
    if (!profileId) delete keys[duty]
    else keys[duty] = profileId
    setC({ keys })
  }

  const check = async (p: KeyProfile) => {
    setChecking(true)
    setProblems(null)
    try {
      const r = await api.post<{ problems: string[] }>('/admin/keys/check', { profile: p })
      setProblems(r.problems ?? [])
      if ((r.problems ?? []).length === 0) toast.success('No problems found')
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setChecking(false)
    }
  }

  const printProfile = allProfiles.find((p) => p.id === printId) ?? allProfiles[0]

  return (
    <section className="panel space-y-4 rounded-lg p-4" aria-labelledby="cmd-h">
      <h2 id="cmd-h" className="flex items-center gap-2 font-semibold text-slate-100"><Terminal size={15} className="text-sky-300" aria-hidden /> Command line and keys</h2>

      {/* Command grammar */}
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Delimiter" htmlFor="cmd-delim" hint="What separates fields on the command line"><input id="cmd-delim" className="input font-mono" value={c.delim ?? ''} onChange={(e) => setC({ delim: e.target.value })} placeholder="space" /></Field>
        <Field label="Field-initiate type" htmlFor="cmd-fitype" hint="The default call type for a field-initiated incident (default TS)"><input id="cmd-fitype" className="input font-mono" value={c.fiType ?? ''} onChange={(e) => setC({ fiType: e.target.value })} placeholder="TS" /></Field>
        <Field label="Field-initiate order" htmlFor="cmd-fiorder" hint="Default unit, location, type, plate, state, comment, platetype, status"><input id="cmd-fiorder" className="input font-mono" value={(c.fieldInitiate ?? []).join(', ')} onChange={(e) => setC({ fieldInitiate: parseList(e.target.value) })} placeholder="unit, location, type, …" /></Field>
        <Field label="Unit-status order" htmlFor="cmd-usorder" hint="Default unit, status, disposition, location, type, comment"><input id="cmd-usorder" className="input font-mono" value={(c.unitStatus ?? []).join(', ')} onChange={(e) => setC({ unitStatus: parseList(e.target.value) })} placeholder="unit, status, disposition, …" /></Field>
        <Field label="Field-initiate mark-out" htmlFor="cmd-fimarkout" hint="The mark-out a field-initiate uses, if any"><input id="cmd-fimarkout" className="input font-mono" value={c.fiMarkOut ?? ''} onChange={(e) => setC({ fiMarkOut: e.target.value })} /></Field>
      </div>
      <div className="flex flex-wrap gap-4">
        <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!c.twoPass} onChange={(e) => setC({ twoPass: e.target.checked })} /> Two-pass entry</label>
        <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!c.autoQuery} onChange={(e) => setC({ autoQuery: e.target.checked })} /> Auto-query plates and people</label>
        <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!c.multiUnit} onChange={(e) => setC({ multiUnit: e.target.checked })} /> Allow many units on one command</label>
      </div>

      {/* Status codes */}
      <div className="space-y-2">
        <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500">Status codes</h3>
        <Table head={['Code', 'Status', 'Name', 'Retired', '']} empty={statusCodes.length === 0 ? <p className="p-3 text-sm text-slate-500">No status codes set.</p> : undefined}>
          {statusCodes.map((sc, i) => (
            <tr key={i} className="align-top">
              <td className="px-3 py-1"><input className="input h-7 w-20 font-mono text-xs" aria-label={`Code, row ${i + 1}`} value={sc.code} onChange={(e) => setStatusCode(i, { code: e.target.value })} /></td>
              <td className="px-3">
                <select className="input h-7 text-xs" aria-label={`Status, row ${i + 1}`} value={sc.status} onChange={(e) => setStatusCode(i, { status: e.target.value })}>
                  <option value="">—</option>
                  {view.statuses.map((st) => <option key={st} value={st}>{st}</option>)}
                </select>
              </td>
              <td className="px-3"><input className="input h-7 text-xs" aria-label={`Name, row ${i + 1}`} value={sc.name ?? ''} onChange={(e) => setStatusCode(i, { name: e.target.value || undefined })} /></td>
              <td className="px-3 text-center"><input type="checkbox" aria-label={`Retired, row ${i + 1}`} checked={!!sc.retired} onChange={(e) => setStatusCode(i, { retired: e.target.checked })} /></td>
              <td className="px-3"><button className="btn btn-ghost px-1.5" aria-label={`Remove row ${i + 1}`} onClick={() => setC({ statusCodes: statusCodes.filter((_, j) => j !== i) })}><Trash2 size={13} aria-hidden /></button></td>
            </tr>
          ))}
        </Table>
        <button className="btn" onClick={() => setC({ statusCodes: [...statusCodes, { code: '', status: '' }] })}><Plus size={13} aria-hidden /> Add a status code</button>
      </div>

      {/* Key profiles */}
      <div className="space-y-3">
        <h3 className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-slate-500"><KeyRound size={13} aria-hidden /> Key profiles</h3>

        {/* Per-duty choice */}
        <div>
          <span className="label">Which profile each duty uses</span>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 md:grid-cols-4">
            {(['*', ...DUTIES] as string[]).map((duty) => (
              <Field key={duty} label={duty === '*' ? 'everyone' : duty} htmlFor={`key-duty-${duty}`}>
                <select id={`key-duty-${duty}`} className="input h-7 text-xs" value={c.keys?.[duty] ?? ''} onChange={(e) => setDutyKey(duty, e.target.value)}>
                  <option value="">—</option>
                  {allProfiles.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
                </select>
              </Field>
            ))}
          </div>
        </div>

        {/* Stock profiles to copy */}
        <div className="flex flex-wrap gap-2">
          {stock.map((p) => (
            <button key={p.id} className="btn" onClick={() => copyStock(p)}><Copy size={12} aria-hidden /> Copy {p.name}</button>
          ))}
          {stock.length === 0 && <span className="text-xs text-slate-500">No stock profiles loaded.</span>}
        </div>

        {/* Editable copied profiles */}
        {profiles.map((p, pi) => (
          <div key={p.id} className="rounded-md border border-slate-800 p-3">
            <div className="mb-2 flex flex-wrap items-end gap-2">
              <Field label="Profile id" htmlFor={`kp-id-${pi}`}><input id={`kp-id-${pi}`} className="input h-7 w-32 font-mono text-xs" value={p.id} onChange={(e) => setProfile(pi, { id: e.target.value })} /></Field>
              <Field label="Name" htmlFor={`kp-name-${pi}`}><input id={`kp-name-${pi}`} className="input h-7 text-xs" value={p.name} onChange={(e) => setProfile(pi, { name: e.target.value })} /></Field>
              <button className="btn" disabled={checking} onClick={() => check(p)}><ShieldCheck size={12} aria-hidden /> Check</button>
              <button className="btn text-rose-300" onClick={() => removeProfile(pi)}><Trash2 size={12} aria-hidden /> Remove profile</button>
            </div>
            <Table head={['Key', 'Screen', 'Action', '']}>
              {p.bindings.map((b, bi) => (
                <tr key={bi} className="align-top">
                  <td className="px-3 py-1"><input className="input h-7 w-28 font-mono text-xs" aria-label={`Key, binding ${bi + 1}`} value={b.key} onChange={(e) => setBinding(pi, bi, { key: e.target.value })} placeholder="F7 or Shift+F11" /></td>
                  <td className="px-3">
                    <select className="input h-7 text-xs" aria-label={`Screen, binding ${bi + 1}`} value={b.screen ?? ''} onChange={(e) => setBinding(pi, bi, { screen: e.target.value as KeyBinding['screen'] })}>
                      <option value="">any</option>
                      <option value="console">console</option>
                      <option value="taker">taker</option>
                      <option value="mdt">mdt</option>
                    </select>
                  </td>
                  <td className="px-3">
                    <select className="input h-7 text-xs" aria-label={`Action, binding ${bi + 1}`} value={b.action} onChange={(e) => setBinding(pi, bi, { action: e.target.value })}>
                      <option value="">—</option>
                      {actionList.map((a) => <option key={a} value={a}>{a}</option>)}
                    </select>
                  </td>
                  <td className="px-3"><button className="btn btn-ghost px-1.5" aria-label={`Remove binding ${bi + 1}`} onClick={() => removeBinding(pi, bi)}><Trash2 size={13} aria-hidden /></button></td>
                </tr>
              ))}
            </Table>
            <button className="btn mt-2" onClick={() => addBinding(pi)}><Plus size={13} aria-hidden /> Add a key</button>
          </div>
        ))}

        {problems && (
          <div className={problems.length ? 'rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-200' : 'rounded-md border border-emerald-500/40 bg-emerald-500/10 p-3 text-sm text-emerald-200'}>
            {problems.length ? (
              <>
                <p className="font-semibold">The server would refuse this profile:</p>
                <ul className="mt-1 list-disc space-y-0.5 pl-5">{problems.map((x, i) => <li key={i}>{x}</li>)}</ul>
              </>
            ) : <p>No problems found.</p>}
          </div>
        )}

        {/* Printable key map */}
        <div className="space-y-2">
          <div className="flex flex-wrap items-end gap-2">
            <Field label="Show the key map for" htmlFor="key-print">
              <select id="key-print" className="input h-7 text-xs" value={printProfile?.id ?? ''} onChange={(e) => setPrintId(e.target.value)}>
                {allProfiles.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
              </select>
            </Field>
          </div>
          {printProfile && (
            <Table head={['Key', 'Screen', 'Action']} empty={printProfile.bindings.length === 0 ? <p className="p-3 text-sm text-slate-500">No keys in this profile.</p> : undefined}>
              {printProfile.bindings.map((b, i) => (
                <tr key={i}>
                  <td className="px-3 py-1.5"><span className="kbd">{b.key}</span></td>
                  <td className="px-3 text-slate-400">{b.screen || 'any'}</td>
                  <td className="px-3 text-slate-200">{actions[b.action] ?? b.action}</td>
                </tr>
              ))}
            </Table>
          )}
        </div>
      </div>
    </section>
  )
}
