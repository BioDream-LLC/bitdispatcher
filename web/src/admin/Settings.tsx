import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Plus, Save, X } from 'lucide-react'
import { api, errText } from '../api'
import { Field } from '../ui'
import { STATUS } from '../util'
import type { Settings, UnitType, View } from '../types'
import { Page } from './Admin'

const OSM = 'https://tile.openstreetmap.org/{z}/{x}/{y}.png'

// The kinds kept covered unless set otherwise (cad.defaultCover).
const COVER = ['engine', 'ladder', 'medic', 'ambulance', 'patrol']

export default function SettingsPage({ view }: { view: View }) {
  const [s, setS] = useState<Settings>(view.settings)
  const [dirty, setDirty] = useState(false)
  useEffect(() => {
    if (!dirty) setS(view.settings)
  }, [view.settings, dirty])
  const up = (p: Partial<Settings>) => {
    setS({ ...s, ...p })
    setDirty(true)
  }
  const save = async () => {
    try {
      await api.put('/admin/settings', s)
      toast.success('Settings saved')
      setDirty(false)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const upType = (i: number, p: Partial<UnitType>) => up({ unitTypes: s.unitTypes.map((t, j) => (j === i ? { ...t, ...p } : t)) })
  return (
    <Page
      title="Settings"
      intro="How this agency works: names, status timers, how stale data is shown, travel estimates and the map. Changes apply at once to every position and are recorded in the ledger."
      actions={<button className="btn btn-go" disabled={!dirty} onClick={save}><Save size={14} aria-hidden /> Save</button>}
    >
      <div className="grid gap-4 lg:grid-cols-2">
        <section className="panel space-y-3 rounded-lg p-4">
          <h2 className="font-semibold text-slate-100">Agency</h2>
          <Field label="Agency name" htmlFor="ag" hint="Shown on every dispatcher's status bar"><input id="ag" className="input" value={s.agency} onChange={(e) => up({ agency: e.target.value })} /></Field>
          <Field label="Incident number prefix" htmlFor="pf" hint="Numbers are PREFIX + year + sequence, such as F26-000123"><input id="pf" className="input w-32 font-mono" maxLength={4} value={s.incidentPrefix} onChange={(e) => up({ incidentPrefix: e.target.value.toUpperCase() })} /></Field>
          <div>
            <span className="label">Dispositions</span>
            <div className="flex flex-wrap gap-1">
              {s.dispositions.map((d, i) => (
                <span key={i} className="chip h-6 bg-slate-800 text-slate-200">
                  {d}
                  <button aria-label={`Remove ${d}`} onClick={() => up({ dispositions: s.dispositions.filter((_, j) => j !== i) })}><X size={11} aria-hidden /></button>
                </span>
              ))}
              <input
                className="input h-6 w-40 py-0 text-xs"
                placeholder="Add, then Enter"
                aria-label="Add a disposition"
                onKeyDown={(e) => {
                  const v = (e.target as HTMLInputElement).value.trim()
                  if (e.key === 'Enter' && v) {
                    up({ dispositions: [...s.dispositions, v] })
                    ;(e.target as HTMLInputElement).value = ''
                  }
                }}
              />
            </div>
          </div>
        </section>
        <section className="panel space-y-3 rounded-lg p-4">
          <h2 className="font-semibold text-slate-100">Status timers</h2>
          <p className="text-xs text-slate-400">Minutes in a status before the unit's tile and row are marked overdue and listed in the Situation panel. 0 turns a timer off. It prompts a person; it never changes a status.</p>
          <div className="grid grid-cols-6 gap-2">
            {['dispatched', 'enroute', 'onscene', 'transporting', 'atdestination', 'markout'].map((k) => (
              <Field key={k} label={STATUS[k].code} htmlFor={`t-${k}`} hint={STATUS[k].label}>
                <input id={`t-${k}`} type="number" min={0} max={600} className="input font-mono" value={s.timers[k] ?? 0} onChange={(e) => up({ timers: { ...s.timers, [k]: Number(e.target.value) } })} />
              </Field>
            ))}
          </div>
          <div className="grid grid-cols-2 gap-2">
            <Field label="AVL stale after (s)" htmlFor="avl" hint="Older positions are drawn dashed and not used to rank units"><input id="avl" type="number" className="input font-mono" value={s.avlStaleSeconds} onChange={(e) => up({ avlStaleSeconds: Number(e.target.value) })} /></Field>
            <Field label="Caller location stale after (s)" htmlFor="ls" hint="Older caller locations are marked STALE"><input id="ls" type="number" className="input font-mono" value={s.locationStaleSeconds} onChange={(e) => up({ locationStaleSeconds: Number(e.target.value) })} /></Field>
          </div>
          <Field label="Shift changes" htmlFor="shifts" hint="Times the shift changes, for the operations report: 07:00 19:00">
            <input id="shifts" className="input font-mono" defaultValue={(s.shifts ?? []).join(' ')} onBlur={(e) => up({ shifts: e.target.value.split(/[\s,]+/).filter(Boolean) })} />
          </Field>
        </section>
        <section className="panel space-y-3 rounded-lg p-4 lg:col-span-2">
          <div className="flex items-center gap-2">
            <h2 className="font-semibold text-slate-100">Mark-outs</h2>
            <button className="btn ml-auto h-7 text-xs" onClick={() => up({ markOuts: [...(s.markOuts ?? []), { code: '', name: '', minutes: 30 }] })}><Plus size={12} aria-hidden /> Add a mark-out</button>
          </div>
          <p className="text-xs text-slate-400">
            What a unit can be marked out for, with <span className="font-mono">MO 2A12 MEAL 315 Church Ave</span> or from its right-click menu, the police board or the mobile terminal. Each has its own status-check time:
            when it runs out the unit flashes for a status check, and <span className="font-mono">SC 2A12</span> (or OK) restarts it. One that can be pulled is offered by the recommendation when no available unit can go.
          </p>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead className="text-[10px] uppercase tracking-wider text-slate-500">
                <tr><th className="py-1 text-left">Code</th><th className="text-left">Name</th><th className="text-left">Status check after (min)</th><th className="text-left">For</th><th className="text-left">Can be pulled</th><th /></tr>
              </thead>
              <tbody>
                {(s.markOuts ?? []).map((m, i) => {
                  const set = (p: Partial<typeof m>) => up({ markOuts: (s.markOuts ?? []).map((x, j) => (j === i ? { ...x, ...p } : x)) })
                  return (
                    <tr key={i} className="border-t border-slate-800">
                      <td className="py-1 pr-2"><input className="input h-7 w-24 py-0 font-mono uppercase" value={m.code} onChange={(e) => set({ code: e.target.value.toUpperCase().replace(/\s/g, '') })} aria-label="Mark-out code" /></td>
                      <td className="pr-2"><input className="input h-7 py-0" value={m.name} onChange={(e) => set({ name: e.target.value })} aria-label="Mark-out name" /></td>
                      <td className="pr-2"><input type="number" min={0} max={600} className="input h-7 w-24 py-0 font-mono" value={m.minutes} onChange={(e) => set({ minutes: Number(e.target.value) })} aria-label="Mark-out minutes" /></td>
                      <td className="pr-2">
                        <select className="input h-7 w-32 py-0" value={m.discipline ?? ''} onChange={(e) => set({ discipline: e.target.value || undefined })} aria-label="Discipline">
                          <option value="">Any unit</option><option value="fire">Fire</option><option value="ems">EMS</option><option value="police">Police</option><option value="other">Other</option>
                        </select>
                      </td>
                      <td className="pr-2"><input type="checkbox" checked={!!m.interruptible} onChange={(e) => set({ interruptible: e.target.checked })} aria-label="Can be pulled for a call" /></td>
                      <td><button className="btn btn-ghost px-1" onClick={() => up({ markOuts: (s.markOuts ?? []).filter((_, j) => j !== i) })} aria-label={`Remove ${m.code}`}><X size={12} /></button></td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </section>
        <section className="panel space-y-3 rounded-lg p-4">
          <h2 className="font-semibold text-slate-100">Travel estimates</h2>
          <p className="text-xs text-slate-400">Inside the recorded map (Admin › Map) drive times come from the road network. Outside it, or where no road connects, they are the straight line times a road factor at an average speed, and every recommendation says so.</p>
          <div className="grid grid-cols-2 gap-2">
            <Field label="Road factor" htmlFor="rf" hint="Road distance over straight-line distance; 1.3 to 1.5 is typical"><input id="rf" type="number" step="0.05" className="input font-mono" value={s.roadFactor} onChange={(e) => up({ roadFactor: Number(e.target.value) })} /></Field>
            <Field label="Average speed (km/h)" htmlFor="sp" hint="Emergency driving, door to door"><input id="sp" type="number" className="input font-mono" value={s.speedKmh} onChange={(e) => up({ speedKmh: Number(e.target.value) })} /></Field>
          </div>
        </section>
        <section className="panel space-y-3 rounded-lg p-4">
          <h2 className="font-semibold text-slate-100">Roles and sign-in</h2>
          <p className="text-xs text-slate-400">What each duty may do is enforced by the server. A one-time code (any authenticator app) protects access to criminal justice information: confirm your state's CJIS requirements before relying on it.</p>
          <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!s.duties?.callTakersDispatch} onChange={(e) => up({ duties: { ...s.duties, callTakersDispatch: e.target.checked } })} /> Call-takers may also send and change units</label>
          <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!s.duties?.crossDiscipline} onChange={(e) => up({ duties: { ...s.duties, crossDiscipline: e.target.checked } })} /> Police, fire and EMS dispatchers may send any discipline's units</label>
          <div className="grid grid-cols-2 gap-2">
            <Field label="One-time codes at sign-in" htmlFor="mfa">
              <select id="mfa" className="input" value={s.security?.mfa || 'off'} onChange={(e) => up({ security: { ...s.security, mfa: e.target.value as 'off' } })}>
                <option value="off">Off (anyone who set one up still uses it)</option><option value="admins">Supervisors and administrators</option><option value="everyone">Everyone</option>
              </select>
            </Field>
            <Field label="Lock a console after (minutes without input)" htmlFor="lck" hint="0: never. Ctrl+Shift+L locks at once; the same person unlocks with their password"><input id="lck" type="number" min={0} className="input font-mono" value={s.security?.lockMinutes ?? 0} onChange={(e) => up({ security: { ...s.security, lockMinutes: Number(e.target.value) } })} /></Field>
          </div>
          <label className="flex items-start gap-2 text-sm text-slate-300"><input type="checkbox" className="mt-1" checked={!!s.security?.stagedChanges} onChange={(e) => up({ security: { ...s.security, stagedChanges: e.target.checked } })} /><span>Change configuration only through staging<span className="block text-xs text-slate-500">Call types, units, stations, plans, guidance and facilities are then changed on a training board (sign in with Training ticked as an administrator), tried there, and published to the live board with a reason. Settings, users and connections stay here.</span></span></label>
        </section>
        <section className="panel space-y-3 rounded-lg p-4">
          <h2 className="font-semibold text-slate-100">Call entry and dispatch</h2>
          <div className="grid grid-cols-2 gap-2">
            <Field label="Flag a unit not acknowledged after (s)" htmlFor="ack"><input id="ack" type="number" min={10} className="input font-mono" value={s.ackSeconds || 60} onChange={(e) => up({ ackSeconds: Number(e.target.value) })} /></Field>
            <Field label="Offer a newer caller location beyond (m)" htmlFor="mvm"><input id="mvm" type="number" min={5} className="input font-mono" value={s.moveMeters || 50} onChange={(e) => up({ moveMeters: Number(e.target.value) })} /></Field>
            <Field label="Records requests due in (working days)" htmlFor="fd"><input id="fd" type="number" min={1} className="input font-mono" value={s.foiaDays || 5} onChange={(e) => up({ foiaDays: Number(e.target.value) })} /></Field>
          </div>
          <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!s.overrideReasonOff} onChange={(e) => up({ overrideReasonOff: !e.target.checked })} /> Ask why when other units than the recommendation are sent</label>
          <Field label="Talkgroups" htmlFor="tgs" hint="Separated by commas: offered on every incident"><input id="tgs" className="input font-mono" key={'tg' + (s.talkgroups ?? []).join()} defaultValue={(s.talkgroups ?? []).join(', ')} onBlur={(e) => up({ talkgroups: e.target.value.split(',').map((x) => x.trim()).filter(Boolean) })} /></Field>
          <Field label="Incident modifiers" htmlFor="mods" hint="One per line: CODE | name | raises to priority (optional) | notify, separated by ; (optional) | safety. Empty: the built-in list">
            <textarea id="mods" rows={6} className="input font-mono text-xs" key={'mods' + JSON.stringify(s.modifiers ?? [])}
              defaultValue={(s.modifiers ?? []).map((m) => [m.code, m.name, m.priority || '', (m.notify ?? []).join('; '), m.safety ? 'safety' : ''].join(' | ').replace(/( \| )+$/, '')).join('\n')}
              placeholder={(view.entry?.modifiers ?? []).map((m) => [m.code, m.name, m.priority || '', (m.notify ?? []).join('; '), m.safety ? 'safety' : ''].join(' | ').replace(/( \| )+$/, '')).join('\n')}
              onBlur={(e) => up({ modifiers: e.target.value.split('\n').map((l) => l.split('|').map((x) => x.trim())).filter((f) => f[0]).map((f) => ({ code: f[0].toUpperCase(), name: f[1] || f[0], priority: Number(f[2]) || undefined, notify: f[3] ? f[3].split(';').map((x) => x.trim()).filter(Boolean) : undefined, safety: f[4] === 'safety' || undefined })) })} />
          </Field>
        </section>
        <section className="panel space-y-3 rounded-lg p-4">
          <h2 className="font-semibold text-slate-100">Calls: grouping and non-emergency</h2>
          <p className="text-xs text-slate-400">Callers this near each other within this time are shown as one group, attached to one incident in one step. Calls on a non-emergency line, or whose caller says one of the words (and nothing urgent), go to the non-emergency list; a person always decides.</p>
          <div className="grid grid-cols-2 gap-2">
            <Field label="Group callers within (m)" htmlFor="clm"><input id="clm" type="number" min={0} className="input font-mono" value={s.clusterMeters ?? 300} onChange={(e) => up({ clusterMeters: Number(e.target.value) })} /></Field>
            <Field label="…and (minutes)" htmlFor="clt"><input id="clt" type="number" min={0} className="input font-mono" value={s.clusterMinutes ?? 10} onChange={(e) => up({ clusterMinutes: Number(e.target.value) })} /></Field>
          </div>
          <Field label="Non-emergency lines" htmlFor="nel" hint="Numbers or line names, separated by commas: +15408532121"><input id="nel" className="input font-mono" key={'nonEmergencyLines' + (s.nonEmergencyLines ?? []).join()} defaultValue={(s.nonEmergencyLines ?? []).join(', ')} onBlur={(e) => up({ nonEmergencyLines: e.target.value.split(',').map((x) => x.trim()).filter(Boolean) })} /></Field>
          <Field label="Non-emergency words" htmlFor="new" hint="Separated by commas; empty uses the built-in list (noise complaint, parking, power outage…)"><textarea id="new" rows={2} className="input text-xs" key={'nonEmergencyWords' + (s.nonEmergencyWords ?? []).join()} defaultValue={(s.nonEmergencyWords ?? []).join(', ')} onBlur={(e) => up({ nonEmergencyWords: e.target.value.split(',').map((x) => x.trim()).filter(Boolean) })} /></Field>
          <Field label="Reply to non-emergency callers" htmlFor="ner" hint="Offered as a one-press text, such as where to report online"><input id="ner" className="input" value={s.nonEmergencyReply ?? ''} onChange={(e) => up({ nonEmergencyReply: e.target.value })} /></Field>
        </section>
        <section className="panel space-y-3 rounded-lg p-4">
          <h2 className="font-semibold text-slate-100">People: critical calls and breaks</h2>
          <p className="text-xs text-slate-400">An incident of a call type marked critical, or where one of these words is said, puts everyone who worked it on the supervisor's care list, and tells them gently once it closes.</p>
          <Field label="Critical words" htmlFor="crw" hint="Separated by commas; empty uses the built-in list (not breathing, CPR, officer down, infant…)"><textarea id="crw" rows={2} className="input text-xs" key={'criticalWords' + (s.criticalWords ?? []).join()} defaultValue={(s.criticalWords ?? []).join(', ')} onBlur={(e) => up({ criticalWords: e.target.value.split(',').map((x) => x.trim()).filter(Boolean) })} /></Field>
          <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!s.careOff} onChange={(e) => up({ careOff: !e.target.checked })} /> Listen for the critical words (critical call types always count)</label>
          <Field label="Support line" htmlFor="sup" hint="Shown to a person after a critical call: the employee assistance programme, peer support"><input id="sup" className="input" value={s.supportLine ?? ''} onChange={(e) => up({ supportLine: e.target.value })} placeholder="Peer support: 540-555-0100" /></Field>
          <Field label="Suggest a break after (minutes)" htmlFor="brk" hint="Worked this long without one: the supervisor board says so"><input id="brk" type="number" min={30} className="input w-32 font-mono" value={s.breakMinutes || 120} onChange={(e) => up({ breakMinutes: Number(e.target.value) })} /></Field>
        </section>
        <section className="panel space-y-3 rounded-lg p-4">
          <h2 className="font-semibold text-slate-100">Quality review standards</h2>
          <div className="grid grid-cols-3 gap-2">
            <Field label="Answer within (s)" htmlFor="qaa"><input id="qaa" type="number" className="input font-mono" value={s.qa?.answerSeconds || 15} onChange={(e) => up({ qa: { ...s.qa, answerSeconds: Number(e.target.value) } })} /></Field>
            <Field label="Enter within (s)" htmlFor="qae"><input id="qae" type="number" className="input font-mono" value={s.qa?.entrySeconds || 90} onChange={(e) => up({ qa: { ...s.qa, entrySeconds: Number(e.target.value) } })} /></Field>
            <Field label="Dispatch P1–2 within (s)" htmlFor="qad"><input id="qad" type="number" className="input font-mono" value={s.qa?.dispatchSeconds || 60} onChange={(e) => up({ qa: { ...s.qa, dispatchSeconds: Number(e.target.value) } })} /></Field>
          </div>
          {['all', 'ems', 'fire', 'police'].map((d) => (
            <Field key={d} label={`Questions to hear: ${d === 'all' ? 'every call' : d}`} htmlFor={`qq-${d}`} hint="One per line; words separated by | count as the same question. Empty lines: the built-in set">
              <textarea id={`qq-${d}`} rows={2} className="input font-mono text-xs" key={d + (s.qa?.questions?.[d] ?? []).join()} defaultValue={(s.qa?.questions?.[d] ?? []).join('\n')}
                onBlur={(e) => up({ qa: { ...s.qa, questions: { ...(s.qa?.questions ?? {}), [d]: e.target.value.split('\n').map((x) => x.trim()).filter(Boolean) } } })} />
            </Field>
          ))}
        </section>
        <section className="panel space-y-3 rounded-lg p-4">
          <h2 className="font-semibold text-slate-100">Crash data and line attacks</h2>
          <p className="text-xs text-slate-400">Vehicle crash notifications are rated by delta-V, the change of speed in the crash, with rollovers and other signs. A flood of calls, one number calling again and again, or a run of silent short calls raises the denial-of-service warning.</p>
          <div className="grid grid-cols-3 gap-2">
            <Field label="High risk at (km/h)" htmlFor="chi"><input id="chi" type="number" className="input font-mono" value={s.crashHighKmh || 40} onChange={(e) => up({ crashHighKmh: Number(e.target.value) })} /></Field>
            <Field label="Medium at (km/h)" htmlFor="cme"><input id="cme" type="number" className="input font-mono" value={s.crashMediumKmh || 20} onChange={(e) => up({ crashMediumKmh: Number(e.target.value) })} /></Field>
            <Field label="Warn at calls a minute" htmlFor="tdp" hint="0: five times the usual (at least 12)"><input id="tdp" type="number" min={0} className="input font-mono" value={s.tdosPerMinute ?? 0} onChange={(e) => up({ tdosPerMinute: Number(e.target.value) })} /></Field>
          </div>
        </section>
        <section className="panel space-y-3 rounded-lg p-4">
          <h2 className="font-semibold text-slate-100">NCIC and the state message switch</h2>
          <p className="text-xs text-slate-400">Inquiries go through the state's switch under its agreement; bitdispatcher logs each with its purpose and reason. The training simulator answers TEST plates, names and serials and marks every answer SIMULATED.</p>
          <div className="grid grid-cols-3 gap-2">
            <Field label="ORI" htmlFor="ori" hint="The agency's originating agency identifier"><input id="ori" className="input font-mono uppercase" value={s.ori ?? ''} onChange={(e) => up({ ori: e.target.value.toUpperCase().trim() })} placeholder="VA0770000" /></Field>
            <Field label="Switch" htmlFor="ncm">
              <select id="ncm" className="input" value={s.ncicMode ?? 'off'} onChange={(e) => up({ ncicMode: e.target.value })}>
                <option value="off">Off</option><option value="sim">Training simulator</option><option value="line">Switch adapter (TCP)</option>
              </select>
            </Field>
            <Field label="Adapter address" htmlFor="nca" hint="host:port"><input id="nca" className="input font-mono" disabled={s.ncicMode !== 'line'} value={s.ncicAddr ?? ''} onChange={(e) => up({ ncicAddr: e.target.value.trim() })} placeholder="10.0.0.5:7001" /></Field>
          </div>
        </section>
        <section className="panel space-y-3 rounded-lg p-4">
          <h2 className="font-semibold text-slate-100">Map beyond the recorded area</h2>
          <p className="text-xs text-slate-400">The dispatch map is drawn from the map recorded in Admin › Map. Outside that area the console falls back to these online tiles, shown muted with the edge of the recorded map marked. bitdispatcher fetches them itself and keeps each one on disk; browsers never contact a tile server.</p>
          <Field label="Online tile source" htmlFor="mt" hint="https://…/{z}/{x}/{y}.png, or file:/path/to/tiles/{z}/{x}/{y}.png; empty: nothing outside the recorded map">
            <input id="mt" className="input font-mono text-xs" value={s.mapTiles} onChange={(e) => up({ mapTiles: e.target.value.trim() })} placeholder="file:/srv/tiles/{z}/{x}/{y}.png" />
          </Field>
          <div className="flex flex-wrap gap-1">
            <button className="btn text-xs" onClick={() => up({ mapTiles: OSM, mapAttribution: '© OpenStreetMap contributors' })}>OpenStreetMap</button>
            <button className="btn text-xs" onClick={() => up({ mapTiles: '', mapAttribution: '' })}>None</button>
          </div>
          {s.mapTiles === OSM && <p className="text-xs text-amber-300">OpenStreetMap's tile servers allow light use under their tile usage policy. As a fallback beyond the recorded area that is fine; it is not a substitute for recording the area.</p>}
          <Field label="Attribution" htmlFor="ma"><input id="ma" className="input" value={s.mapAttribution} onChange={(e) => up({ mapAttribution: e.target.value })} /></Field>
          <div className="grid grid-cols-3 gap-2">
            <Field label="Centre latitude" htmlFor="mclat"><input id="mclat" className="input font-mono" value={s.mapCenter[0]} onChange={(e) => up({ mapCenter: [Number(e.target.value), s.mapCenter[1]] })} /></Field>
            <Field label="Centre longitude" htmlFor="mclon"><input id="mclon" className="input font-mono" value={s.mapCenter[1]} onChange={(e) => up({ mapCenter: [s.mapCenter[0], Number(e.target.value)] })} /></Field>
            <Field label="Zoom" htmlFor="mz"><input id="mz" type="number" className="input font-mono" value={s.mapZoom} onChange={(e) => up({ mapZoom: Number(e.target.value) })} /></Field>
          </div>
        </section>
        <section className="panel space-y-3 rounded-lg p-4 lg:col-span-2">
          <h2 className="font-semibold text-slate-100">Unit types</h2>
          <p className="text-xs text-slate-400">The kinds of unit and the capabilities each has. Run cards ask for capabilities; a unit can add its own. Kinds kept covered get move-up suggestions when a station has none available.</p>
          <table className="w-full text-sm">
            <thead><tr className="text-left text-[11px] uppercase tracking-wider text-slate-500"><th>Code</th><th>Name</th><th>Discipline</th><th>Capabilities</th><th title="Suggest move-ups when a station has none of this kind available">Keep covered</th><th /></tr></thead>
            <tbody>
              {s.unitTypes.map((t, i) => (
                <tr key={i}>
                  <td className="py-0.5 pr-2"><input aria-label="Code" className="input w-32 font-mono" value={t.code} onChange={(e) => upType(i, { code: e.target.value.trim().toLowerCase() })} /></td>
                  <td className="pr-2"><input aria-label="Name" className="input" value={t.name} onChange={(e) => upType(i, { name: e.target.value })} /></td>
                  <td className="pr-2">
                    <select aria-label="Discipline" className="input" value={t.discipline} onChange={(e) => upType(i, { discipline: e.target.value })}>
                      {['fire', 'ems', 'police', 'other'].map((x) => <option key={x}>{x}</option>)}
                    </select>
                  </td>
                  <td className="pr-2"><input aria-label="Capabilities" className="input font-mono" value={t.capabilities.join(', ')} onChange={(e) => upType(i, { capabilities: e.target.value.split(/[,\s]+/).filter(Boolean) })} /></td>
                  <td className="pr-2 text-center"><input type="checkbox" aria-label={`Keep ${t.name} covered`} checked={t.cover ?? COVER.includes(t.code)} onChange={(e) => upType(i, { cover: e.target.checked })} /></td>
                  <td><button className="btn btn-ghost px-1.5" aria-label={`Remove ${t.name}`} onClick={() => up({ unitTypes: s.unitTypes.filter((_, j) => j !== i) })}><X size={13} aria-hidden /></button></td>
                </tr>
              ))}
            </tbody>
          </table>
          <button className="btn" onClick={() => up({ unitTypes: [...s.unitTypes, { code: '', name: '', discipline: 'fire', capabilities: [] }] })}><Plus size={13} aria-hidden /> Add unit type</button>
        </section>
      </div>
    </Page>
  )
}
