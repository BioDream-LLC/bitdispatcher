import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Building2, History, Plus, Save, Trash2, Undo2, Users } from 'lucide-react'
import { api, errText } from '../api'
import { Field, Modal } from '../ui'
import { hhmm } from '../util'
import type { Agency, AgencyView, ConfigVersion, Staff, View } from '../types'
import { Page, Table } from './Admin'

// A person as the staff list returns them: the public user plus their agency record.
type PublicUser = { username: string; name: string; role: string; duties?: string[] }
type StaffRow = PublicUser & { staff: Staff }

// The disciplines an agency can be configured for.
const DISCIPLINES: { code: string; label: string }[] = [
  { code: 'dispatch', label: 'dispatch' },
  { code: 'police', label: 'police' },
  { code: 'fire', label: 'fire' },
  { code: 'ems', label: 'ems' },
  { code: 'other', label: 'other' },
]

// A blank agency for the editor, with the code taken from the path on save.
const blank = (): Agency => ({ code: '', name: '', disciplines: [] })

type Draft = Agency & { isNew: boolean; timerText: Record<string, string> }

/** Agencies is who the centre dispatches for: the organisations, the people who work them, and the record
 * of every published configuration change. Police, fire and EMS are disciplines; agencies are the
 * organisations. A centre with none configured works as one agency. */
export function Agencies({ view }: { view: View }) {
  const [data, setData] = useState<AgencyView | null>(null)
  const [d, setD] = useState<Draft | null>(null)

  const timerKeys = Object.keys(view.settings.timers ?? {})

  const load = async () => {
    const r = await api.get<AgencyView>('/agencies')
    setData(r)
  }
  useEffect(() => {
    load().catch((e) => toast.error(errText(e)))
  }, [])

  const open = (a?: Agency) => {
    const base = a ? { ...a } : blank()
    const timerText: Record<string, string> = {}
    for (const k of timerKeys) {
      const v = base.timers?.[k]
      timerText[k] = v === undefined ? '' : String(v)
    }
    setD({ ...base, disciplines: [...(base.disciplines ?? [])], isNew: !a, timerText })
  }

  const toggleDiscipline = (code: string) => {
    if (!d) return
    const has = d.disciplines.includes(code)
    setD({ ...d, disciplines: has ? d.disciplines.filter((x) => x !== code) : [...d.disciplines, code] })
  }

  const save = async () => {
    if (!d) return
    const code = d.code.trim()
    if (!code) { toast.error('A code is needed'); return }
    if (d.ori && d.ori.length !== 9) { toast.error('An ORI is 9 characters'); return }
    // Only keep timers that were given a number; a blank means "use the centre's".
    const timers: Record<string, number> = {}
    for (const k of timerKeys) {
      const t = (d.timerText[k] ?? '').trim()
      if (t !== '') timers[k] = Number(t)
    }
    const body: Agency = {
      code,
      name: d.name.trim(),
      disciplines: d.disciplines,
      inactive: d.inactive,
      ori: d.ori?.trim() || undefined,
      neris: d.neris?.trim() || undefined,
      incidentPrefix: d.incidentPrefix?.trim() || undefined,
      dispositions: (d.dispositions ?? []),
      timers: Object.keys(timers).length ? timers : undefined,
      talkgroups: (d.talkgroups ?? []),
      records: d.records?.trim() || undefined,
      note: d.note?.trim() || undefined,
    }
    try {
      await api.put(`/admin/agencies/${encodeURIComponent(code)}`, body)
      toast.success(`${body.name || code} saved`)
      setD(null)
      await load()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const remove = async (code: string) => {
    if (!confirm('Remove this agency?')) return
    try {
      await api.del(`/admin/agencies/${encodeURIComponent(code)}`)
      setD(null)
      await load()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  // Counts of what each agency owns, from the state document.
  const counts = (code: string) => ({
    units: view.units.filter((u) => u.agency === code).length,
    stations: view.stations.filter((s) => s.agency === code).length,
    callTypes: view.callTypes.filter((c) => (c.agencies ?? []).includes(code)).length,
  })

  // A comma list in and out of a string field on the draft.
  const commaList = (xs?: string[]) => (xs ?? []).join(', ')
  const parseList = (s: string) => s.split(',').map((x) => x.trim()).filter(Boolean)

  const agencies = data?.agencies ?? []

  return (
    <Page
      title="Agencies"
      intro="Police, fire and EMS are disciplines; agencies are the organisations the centre dispatches for. A centre with no agencies configured works as a single agency. Here you set up the agencies, say which agencies each person may work, and see the record of every published configuration change."
      actions={<button className="btn btn-go" onClick={() => open()}><Plus size={14} aria-hidden /> Add an agency</button>}
    >
      {/* 1. Agencies */}
      <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="ag-h">
        <h2 id="ag-h" className="flex items-center gap-2 font-semibold text-slate-100"><Building2 size={15} className="text-sky-300" aria-hidden /> Agencies</h2>
        <Table head={['Code', 'Name', 'Disciplines', 'Units', 'Stations', 'Call types', 'State']} empty={agencies.length === 0 ? <p className="p-3 text-sm text-slate-500">No agencies yet; the centre works as one agency.</p> : undefined}>
          {agencies.map((a) => {
            const c = counts(a.code)
            return (
              <tr key={a.code} className="cursor-pointer hover:bg-slate-800/50" onClick={() => open(a)}>
                <td className="px-3 py-1.5 font-mono text-slate-100">{a.code}</td>
                <td className="px-3 text-slate-100">{a.name || '—'}</td>
                <td className="px-3 text-slate-400">{(a.disciplines ?? []).length ? (a.disciplines ?? []).join(', ') : '—'}</td>
                <td className="px-3 font-mono text-slate-400">{c.units}</td>
                <td className="px-3 font-mono text-slate-400">{c.stations}</td>
                <td className="px-3 font-mono text-slate-400">{c.callTypes}</td>
                <td className="px-3 text-xs">{a.inactive ? <span className="text-amber-300">inactive</span> : <span className="text-emerald-300">active</span>}</td>
              </tr>
            )
          })}
        </Table>
      </section>

      {/* 2. People's agencies */}
      <StaffSection view={view} />

      {/* 3. Configuration versions */}
      <Versions />

      {/* Agency editor */}
      {d && (
        <Modal
          open
          wide
          onClose={() => setD(null)}
          title={d.isNew ? 'New agency' : d.name || d.code}
          icon={<Building2 size={16} className="text-sky-300" aria-hidden />}
          footer={
            <>
              {!d.isNew && <button className="btn mr-auto text-rose-300" onClick={() => remove(d.code)}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => setD(null)}>Cancel</button>
              <button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save</button>
            </>
          }
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Code" htmlFor="ag-code" hint="A short code, like PD or FIRE"><input id="ag-code" className="input font-mono" disabled={!d.isNew} value={d.code} onChange={(e) => setD({ ...d, code: e.target.value })} /></Field>
            <Field label="Name" htmlFor="ag-name"><input id="ag-name" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} /></Field>
            <div className="sm:col-span-2">
              <span className="label">Disciplines</span>
              <div className="flex flex-wrap gap-3">
                {DISCIPLINES.map((x) => (
                  <label key={x.code} className="flex items-center gap-1.5 text-sm text-slate-300">
                    <input type="checkbox" checked={d.disciplines.includes(x.code)} onChange={() => toggleDiscipline(x.code)} /> {x.label}
                  </label>
                ))}
              </div>
            </div>
            <Field label="ORI" htmlFor="ag-ori" hint="The agency's ORI, 9 characters"><input id="ag-ori" className="input font-mono" maxLength={9} value={d.ori ?? ''} onChange={(e) => setD({ ...d, ori: e.target.value })} /></Field>
            <Field label="NERIS id" htmlFor="ag-neris"><input id="ag-neris" className="input font-mono" value={d.neris ?? ''} onChange={(e) => setD({ ...d, neris: e.target.value })} /></Field>
            <Field label="Incident number prefix" htmlFor="ag-prefix" hint="Blank uses the centre's"><input id="ag-prefix" className="input w-32 font-mono" maxLength={4} value={d.incidentPrefix ?? ''} onChange={(e) => setD({ ...d, incidentPrefix: e.target.value.toUpperCase() })} /></Field>
            <Field label="Dispositions" htmlFor="ag-disp" hint="A comma list; blank uses the centre's"><input id="ag-disp" className="input" value={commaList(d.dispositions)} onChange={(e) => setD({ ...d, dispositions: parseList(e.target.value) })} /></Field>
            <Field label="Talkgroups" htmlFor="ag-tg" hint="A comma list"><input id="ag-tg" className="input font-mono" value={commaList(d.talkgroups)} onChange={(e) => setD({ ...d, talkgroups: parseList(e.target.value) })} /></Field>
            <div className="sm:col-span-2">
              <span className="label">Status timers (minutes)</span>
              <p className="mb-1 text-xs text-slate-500">Blank uses the centre's timer for that status.</p>
              {timerKeys.length ? (
                <div className="grid grid-cols-3 gap-2 sm:grid-cols-6">
                  {timerKeys.map((k) => (
                    <Field key={k} label={k} htmlFor={`ag-timer-${k}`}>
                      <input id={`ag-timer-${k}`} type="number" min={0} max={600} className="input font-mono" value={d.timerText[k] ?? ''} onChange={(e) => setD({ ...d, timerText: { ...d.timerText, [k]: e.target.value } })} />
                    </Field>
                  ))}
                </div>
              ) : <p className="text-xs text-slate-500">No status timers configured on the centre.</p>}
            </div>
            <div className="sm:col-span-2">
              <Field label="Records rules" htmlFor="ag-records" hint="How this agency's records are kept and released">
                <textarea id="ag-records" className="input min-h-24" value={d.records ?? ''} onChange={(e) => setD({ ...d, records: e.target.value })} />
              </Field>
            </div>
            <div className="sm:col-span-2">
              <Field label="Note" htmlFor="ag-note"><input id="ag-note" className="input" value={d.note ?? ''} onChange={(e) => setD({ ...d, note: e.target.value })} /></Field>
            </div>
            <label className="flex items-center gap-2 text-sm text-slate-300 sm:col-span-2">
              <input type="checkbox" checked={!!d.inactive} onChange={(e) => setD({ ...d, inactive: e.target.checked })} /> Inactive (kept, but not dispatched for)
            </label>
          </div>
        </Modal>
      )}
    </Page>
  )
}

/** StaffSection sets which agencies each person may work, their usual and covered agencies, who they
 * watch, whether they are a trainee, and which roster member they are. */
function StaffSection({ view }: { view: View }) {
  const [rows, setRows] = useState<StaffRow[]>([])
  const members = view.roadmap?.members ?? []

  // The agency codes to offer, from the loaded agencies.
  const [codes, setCodes] = useState<string[]>([])

  const load = async () => {
    const [staff, av] = await Promise.all([
      api.get<{ items: StaffRow[] }>('/admin/staff'),
      api.get<AgencyView>('/agencies'),
    ])
    setRows(staff.items ?? [])
    setCodes((av.agencies ?? []).map((a) => a.code))
  }
  useEffect(() => {
    load().catch((e) => toast.error(errText(e)))
  }, [])

  const set = (username: string, p: Partial<Staff>) =>
    setRows((rs) => rs.map((r) => (r.username === username ? { ...r, staff: { ...r.staff, ...p } } : r)))

  const toggleMay = (username: string, code: string) =>
    setRows((rs) => rs.map((r) => {
      if (r.username !== username) return r
      const cur = r.staff.agencies ?? []
      const has = cur.includes(code)
      return { ...r, staff: { ...r.staff, agencies: has ? cur.filter((x) => x !== code) : [...cur, code] } }
    }))

  const save = async (row: StaffRow) => {
    const s = row.staff
    const body: Staff = {
      username: row.username,
      agencies: (s.agencies ?? []).length ? s.agencies : undefined,
      home: s.home || undefined,
      covered: (s.covered ?? []).length ? s.covered : undefined,
      monitor: (s.monitor ?? []).length ? s.monitor : undefined,
      trainee: s.trainee,
      member: s.member || undefined,
      note: s.note?.trim() || undefined,
    }
    try {
      await api.put(`/admin/staff/${encodeURIComponent(row.username)}`, body)
      toast.success(`${row.name || row.username} saved`)
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const parseList = (s: string) => s.split(',').map((x) => x.trim()).filter(Boolean)

  return (
    <section className="panel mt-4 space-y-3 rounded-lg p-4" aria-labelledby="staff-h">
      <h2 id="staff-h" className="flex items-center gap-2 font-semibold text-slate-100"><Users size={15} className="text-sky-300" aria-hidden /> People's agencies</h2>
      <p className="text-xs text-slate-400">Which agencies each person may work (none ticked means every agency), their usual agency, the ones they also cover by default, those they watch, and the roster member they are.</p>
      <Table head={['Person', 'May work', 'Usual', 'Covers', 'Watches', 'Trainee', 'Roster member', 'Note', '']} empty={rows.length === 0 ? <p className="p-3 text-sm text-slate-500">No people yet.</p> : undefined}>
        {rows.map((r) => {
          const s = r.staff
          return (
            <tr key={r.username} className="align-top">
              <td className="px-3 py-1.5">
                <div className="text-slate-100">{r.name || r.username}</div>
                <div className="font-mono text-xs text-slate-500">{r.role}</div>
              </td>
              <td className="px-3">
                <div className="flex max-w-56 flex-wrap gap-1">
                  {codes.length === 0 && <span className="text-xs text-slate-500">no agencies</span>}
                  {codes.map((code) => {
                    const on = (s.agencies ?? []).includes(code)
                    return (
                      <label key={code} className="flex items-center gap-1 text-xs text-slate-300">
                        <input type="checkbox" aria-label={`${r.name || r.username} may work ${code}`} checked={on} onChange={() => toggleMay(r.username, code)} />
                        <span className="font-mono">{code}</span>
                      </label>
                    )
                  })}
                </div>
              </td>
              <td className="px-3">
                <select className="input h-7 text-xs" aria-label={`Usual agency for ${r.name || r.username}`} value={s.home ?? ''} onChange={(e) => set(r.username, { home: e.target.value || undefined })}>
                  <option value="">—</option>
                  {codes.map((code) => <option key={code} value={code}>{code}</option>)}
                </select>
              </td>
              <td className="px-3">
                <input className="input h-7 w-28 font-mono text-xs" aria-label={`Agencies ${r.name || r.username} covers`} value={(s.covered ?? []).join(', ')} onChange={(e) => set(r.username, { covered: parseList(e.target.value) })} placeholder="codes" />
              </td>
              <td className="px-3">
                <input className="input h-7 w-28 font-mono text-xs" aria-label={`Agencies ${r.name || r.username} watches`} value={(s.monitor ?? []).join(', ')} onChange={(e) => set(r.username, { monitor: parseList(e.target.value) })} placeholder="codes" />
              </td>
              <td className="px-3 text-center">
                <input type="checkbox" aria-label={`${r.name || r.username} is a trainee`} checked={!!s.trainee} onChange={(e) => set(r.username, { trainee: e.target.checked })} />
              </td>
              <td className="px-3">
                <select className="input h-7 text-xs" aria-label={`Roster member for ${r.name || r.username}`} value={s.member ?? ''} onChange={(e) => set(r.username, { member: e.target.value || undefined })}>
                  <option value="">—</option>
                  {members.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
                </select>
              </td>
              <td className="px-3">
                <input className="input h-7 w-32 text-xs" aria-label={`Note for ${r.name || r.username}`} value={s.note ?? ''} onChange={(e) => set(r.username, { note: e.target.value })} />
              </td>
              <td className="px-3">
                <button className="btn" onClick={() => save(r)}><Save size={12} aria-hidden /> Save</button>
              </td>
            </tr>
          )
        })}
      </Table>
    </section>
  )
}

/** Versions shows every published configuration change, newest first, and lets an administrator roll one
 * back. Changes are drafted on the training board (staging), published with a reason — optionally for one
 * agency only; a rollback puts the earlier approved version back and is itself recorded. */
function Versions() {
  const [items, setItems] = useState<ConfigVersion[]>([])
  const [back, setBack] = useState<ConfigVersion | null>(null)
  const [why, setWhy] = useState('')

  const load = async () => {
    const r = await api.get<{ items: ConfigVersion[] }>('/admin/config/versions')
    setItems(r.items ?? [])
  }
  useEffect(() => {
    load().catch((e) => toast.error(errText(e)))
  }, [])

  const rollback = async () => {
    if (!back) return
    try {
      await api.post(`/admin/config/versions/${back.seq}/rollback`, { why: why.trim() })
      toast.success(`Rolled back to before #${back.seq}`)
      setBack(null)
      setWhy('')
      await load()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const partsText = (parts: Record<string, string[]>) =>
    Object.entries(parts ?? {}).map(([k, v]) => `${k}${v?.length ? ` (${v.join(', ')})` : ''}`).join('; ')

  return (
    <section className="panel mt-4 space-y-3 rounded-lg p-4" aria-labelledby="ver-h">
      <h2 id="ver-h" className="flex items-center gap-2 font-semibold text-slate-100"><History size={15} className="text-sky-300" aria-hidden /> Configuration versions</h2>
      <p className="text-xs text-slate-400">Changes are drafted on the training board (staging), then published with a reason — optionally for one agency only. A rollback puts the earlier approved version back, and is itself recorded.</p>
      <Table head={['#', 'When', 'By', 'Reason', 'Agency', 'Changes', 'Parts', '']} empty={items.length === 0 ? <p className="p-3 text-sm text-slate-500">No published changes yet.</p> : undefined}>
        {items.map((v) => {
          const isRollback = v.rollbackOf !== undefined
          const undone = v.undoneBy !== undefined
          return (
            <tr key={v.seq} className="align-top hover:bg-slate-800/50">
              <td className="px-3 py-1.5 font-mono text-slate-300">{v.seq}</td>
              <td className="px-3 font-mono text-xs text-slate-400">{hhmm(v.at)}</td>
              <td className="px-3 text-slate-300">{v.by}</td>
              <td className="px-3 text-slate-300">
                {v.why || '—'}
                {isRollback && <span className="ml-1 chip bg-slate-800 text-slate-400">rollback of #{v.rollbackOf}</span>}
                {undone && <span className="ml-1 chip bg-amber-500/20 text-amber-200">undone by #{v.undoneBy}</span>}
              </td>
              <td className="px-3 font-mono text-slate-400">{v.agency || 'all'}</td>
              <td className="px-3 font-mono text-slate-400">{v.changes}</td>
              <td className="px-3 text-xs text-slate-500">{partsText(v.parts) || '—'}</td>
              <td className="px-3">
                {!isRollback && !undone && (
                  <button className="btn" onClick={() => { setBack(v); setWhy('') }}><Undo2 size={12} aria-hidden /> Roll back</button>
                )}
              </td>
            </tr>
          )
        })}
      </Table>

      {back && (
        <Modal
          open
          onClose={() => setBack(null)}
          title={`Roll back change #${back.seq}`}
          icon={<Undo2 size={16} className="text-amber-300" aria-hidden />}
          footer={
            <>
              <button className="btn" onClick={() => setBack(null)}>Cancel</button>
              <button className="btn btn-go" onClick={rollback} disabled={!why.trim()}><Undo2 size={13} aria-hidden /> Roll back</button>
            </>
          }
        >
          <div className="space-y-3">
            <p className="text-sm text-slate-400">This puts the approved version from before #{back.seq} back. The rollback is recorded as its own change.</p>
            <Field label="Reason" htmlFor="rb-why" hint="Why this is being rolled back">
              <textarea id="rb-why" className="input min-h-24" value={why} onChange={(e) => setWhy(e.target.value)} />
            </Field>
          </div>
        </Modal>
      )}
    </section>
  )
}
