import { useState } from 'react'
import { toast } from 'sonner'
import { ClipboardList, Plus, Trash2, X } from 'lucide-react'
import { api, ApiError, errText } from '../api'
import { Field, Modal, Priority } from '../ui'
import { useAgencies } from '../console/Coverage'
import type { AlarmLevel, Requirement, View } from '../types'
import { Page, Table } from './Admin'

type Draft = { code: string; name: string; priority: number; discipline: string; runCard: Requirement[]; critical?: boolean; drone?: boolean; autoDispatch?: boolean; notify?: string[]; group?: string; alarms?: AlarmLevel[]; par?: boolean; parMinutes?: number; nemsis?: string; agencies?: string[]; checkMinutes?: number; isNew: boolean }

// importCSV reads call types from a spreadsheet: code, name, priority, discipline, group, run card
// ("2 engine; 1 ladder"), notify ("Fire marshal; Animal control"). A header row is skipped.
export function parseCallTypes(text: string) {
  const rows = text.split(/\r?\n/).map((l) => l.split(/,(?=(?:[^"]*"[^"]*")*[^"]*$)/).map((x) => x.trim().replace(/^"|"$/g, ''))).filter((r) => r[0])
  if (rows[0] && /^code$/i.test(rows[0][0])) rows.shift()
  return rows.map((r) => ({
    code: r[0].toUpperCase(), name: r[1] || r[0], priority: Math.min(5, Math.max(1, Number(r[2]) || 3)), discipline: (r[3] || 'other').toLowerCase(), group: r[4] || undefined,
    runCard: (r[5] || '').split(';').map((x) => x.trim().match(/^(\d+)\s*[x×]?\s*(.+)$/i)).filter(Boolean).map((m) => ({ count: Number(m![1]), capability: m![2].trim().toLowerCase() })),
    notify: r[6] ? r[6].split(';').map((x) => x.trim()).filter(Boolean) : undefined,
  }))
}

export default function CallTypes({ view }: { view: View }) {
  const agencies = useAgencies()?.agencies ?? []
  const [d, setD] = useState<Draft | null>(null)
  const [err, setErr] = useState<ApiError | null>(null)
  const caps = [...new Set(view.settings.unitTypes.flatMap((t) => t.capabilities).concat(view.units.flatMap((u) => u.caps)))].sort()
  const open = (code?: string) => {
    const c = view.callTypes.find((x) => x.code === code)
    setErr(null)
    setD(c ? { ...c, runCard: c.runCard ?? [], isNew: false } : { code: '', name: '', priority: 3, discipline: 'fire', runCard: [], isNew: true })
  }
  const save = async () => {
    if (!d) return
    try {
      await api.put(`/admin/calltypes/${encodeURIComponent(d.code.trim().toUpperCase())}`, { name: d.name, priority: d.priority, discipline: d.discipline, runCard: d.runCard, critical: !!d.critical, drone: !!d.drone, autoDispatch: !!d.autoDispatch && d.priority >= 3, notify: d.notify, group: d.group, alarms: (d.alarms ?? []).filter((a) => a.runCard.length), par: !!d.par, parMinutes: d.parMinutes || 0, nemsis: d.nemsis || undefined, agencies: (d.agencies ?? []).length ? d.agencies : undefined, checkMinutes: d.checkMinutes || 0 })
      toast.success(`Call type ${d.code.toUpperCase()} saved`)
      setD(null)
    } catch (e) {
      setErr(e instanceof ApiError ? e : new ApiError(0, errText(e)))
    }
  }
  const avail = (cap: string) => view.units.filter((u) => u.caps.includes(cap)).length
  return (
    <Page
      title="Call types and run cards"
      intro="Each call type has a default priority and a run card: how many units of each capability to recommend. The recommendation fills scarce capabilities first, counts units already on the incident, and flags lines it cannot fill."
      actions={<>
        <label className="btn cursor-pointer" title="A CSV: code, name, priority, discipline, group, run card (2 engine; 1 ladder), notify (Fire marshal; Animal control)">
          Import CSV
          <input type="file" accept=".csv,text/csv" className="sr-only" onChange={async (e) => {
            const f = e.target.files?.[0]
            if (!f) return
            const list = parseCallTypes(await f.text())
            if (!confirm(`Import ${list.length} call types? Existing codes are replaced.`)) return
            let n = 0
            for (const c of list) { try { await api.put(`/admin/calltypes/${encodeURIComponent(c.code)}`, c); n++ } catch (err) { toast.error(`${c.code}: ${errText(err)}`) } }
            toast.success(`${n} call types imported`)
            e.target.value = ''
          }} />
        </label>
        <button className="btn btn-go" onClick={() => open()}><Plus size={14} aria-hidden /> Add call type</button>
      </>}
    >
      <Table head={['Code', 'Name', 'Priority', 'Discipline', 'Run card']}>
        {view.callTypes.map((c) => (
          <tr key={c.code} className="cursor-pointer hover:bg-slate-800/50" onClick={() => open(c.code)}>
            <td className="px-3 py-1.5 font-mono font-bold text-slate-100">{c.code}</td>
            <td className="px-3 text-slate-200">{c.name}{c.critical && <span className="chip ml-1 bg-teal-500/20 text-teal-200">critical</span>}{c.drone && <span className="chip ml-1 bg-cyan-500/20 text-cyan-200">drone</span>}{c.autoDispatch && <span className="chip ml-1 bg-amber-500/20 text-amber-200">auto-dispatch</span>}{(c.alarms?.length ?? 0) > 0 && <span className="chip ml-1 bg-orange-500/20 text-orange-200">{c.alarms!.length + 1} alarms</span>}{c.par && <span className="chip ml-1 bg-rose-500/20 text-rose-200">PAR</span>}</td>
            <td className="px-3"><Priority p={c.priority} /></td>
            <td className="px-3 text-slate-400">{c.discipline}</td>
            <td className="px-3 font-mono text-xs text-slate-300">{(c.runCard ?? []).map((r) => `${r.count}×${r.capability}`).join('  ') || <span className="text-slate-500">none: chosen by hand</span>}</td>
          </tr>
        ))}
      </Table>
      {d && (
        <Modal
          open
          wide
          onClose={() => setD(null)}
          title={d.isNew ? 'New call type' : `${d.code} ${d.name}`}
          icon={<ClipboardList size={16} className="text-lime-300" aria-hidden />}
          footer={
            <>
              {!d.isNew && (
                <button className="btn mr-auto text-rose-300" onClick={async () => { if (!confirm(`Remove ${d.code}?`)) return; try { await api.del(`/admin/calltypes/${d.code}`); setD(null) } catch (e) { toast.error(errText(e)) } }}>
                  <Trash2 size={13} aria-hidden /> Remove
                </button>
              )}
              <button className="btn" onClick={() => setD(null)}>Cancel</button>
              <button className="btn btn-go" onClick={save}>Save</button>
            </>
          }
        >
          <div className="grid gap-3 sm:grid-cols-4">
            {err && <p className="sm:col-span-4 rounded bg-rose-950/50 px-2 py-1 text-sm text-rose-200">{err.message}</p>}
            <Field label="Code" htmlFor="cc"><input id="cc" className="input font-mono uppercase" disabled={!d.isNew} value={d.code} onChange={(e) => setD({ ...d, code: e.target.value })} /></Field>
            <div className="sm:col-span-3"><Field label="Name" htmlFor="cn"><input id="cn" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} /></Field></div>
            <Field label="Priority" htmlFor="cp">
              <select id="cp" className="input" value={d.priority} onChange={(e) => setD({ ...d, priority: Number(e.target.value) })}>
                {[1, 2, 3, 4, 5].map((p) => <option key={p} value={p}>P{p}</option>)}
              </select>
            </Field>
            <Field label="Discipline" htmlFor="cd">
              <select id="cd" className="input" value={d.discipline} onChange={(e) => setD({ ...d, discipline: e.target.value })}>
                {['fire', 'ems', 'police', 'other'].map((x) => <option key={x}>{x}</option>)}
              </select>
            </Field>
            <Field label="Group" htmlFor="cg"><input id="cg" className="input" value={d.group ?? ''} placeholder="Medical, Traffic…" onChange={(e) => setD({ ...d, group: e.target.value || undefined })} /></Field>
            <div className="sm:col-span-3"><Field label="Notify" htmlFor="cno" hint="People or agencies to tell for this type, separated by semicolons: a checklist on every incident of it"><input id="cno" className="input" value={(d.notify ?? []).join('; ')} onChange={(e) => setD({ ...d, notify: e.target.value.split(';').map((x) => x.trim()).filter(Boolean) })} /></Field></div>
            <label className="flex items-start gap-2 text-sm text-slate-300 sm:col-span-2">
              <input type="checkbox" className="mt-1" checked={!!d.critical} onChange={(e) => setD({ ...d, critical: e.target.checked })} />
              <span>Critical call<span className="block text-xs text-slate-500">Hard on the people who work it: they go on the supervisor's care list</span></span>
            </label>
            <label className="flex items-start gap-2 text-sm text-slate-300 sm:col-span-2">
              <input type="checkbox" className="mt-1" checked={!!d.drone} onChange={(e) => setD({ ...d, drone: e.target.checked })} />
              <span>Send a drone<span className="block text-xs text-slate-500">With the drone launch rule on (Integrations), these ask for one by themselves</span></span>
            </label>
            <label className="flex items-start gap-2 text-sm text-slate-300 sm:col-span-2">
              <input type="checkbox" className="mt-1" disabled={d.priority < 3} checked={!!d.autoDispatch && d.priority >= 3} onChange={(e) => setD({ ...d, autoDispatch: e.target.checked })} />
              <span>Auto-dispatch<span className="block text-xs text-slate-500">{d.priority < 3 ? 'Only priority 3 to 5 types' : 'When no dispatcher has sent units in time (Settings), the recommended units go by themselves; only a complete recommendation without warnings'}</span></span>
            </label>
            <div className="sm:col-span-4">
              <span className="label">Run card</span>
              <ul className="space-y-1">
                {d.runCard.map((r, i) => (
                  <li key={i} className="flex items-center gap-2">
                    <input aria-label={`Line ${i + 1} count`} type="number" min={1} max={20} className="input w-20 font-mono" value={r.count} onChange={(e) => setD({ ...d, runCard: d.runCard.map((x, j) => (j === i ? { ...x, count: Number(e.target.value) } : x)) })} />
                    <span className="text-slate-500">×</span>
                    <input aria-label={`Line ${i + 1} capability`} list="caps" className="input font-mono" value={r.capability} onChange={(e) => setD({ ...d, runCard: d.runCard.map((x, j) => (j === i ? { ...x, capability: e.target.value.trim().toLowerCase() } : x)) })} />
                    <span className={avail(r.capability) < r.count ? 'w-32 text-xs text-amber-300' : 'w-32 text-xs text-slate-500'}>{avail(r.capability)} unit{avail(r.capability) === 1 ? '' : 's'} can</span>
                    <button className="btn btn-ghost px-1.5" aria-label={`Remove line ${i + 1}`} onClick={() => setD({ ...d, runCard: d.runCard.filter((_, j) => j !== i) })}><X size={13} aria-hidden /></button>
                  </li>
                ))}
              </ul>
              <datalist id="caps">{caps.map((c) => <option key={c} value={c} />)}</datalist>
              <button className="btn mt-2" onClick={() => setD({ ...d, runCard: [...d.runCard, { capability: '', count: 1 }] })}><Plus size={13} aria-hidden /> Add line</button>
            </div>
            <div className="sm:col-span-4">
              <span className="label">Further alarms</span>
              <p className="mb-1 text-xs text-slate-500">Each further alarm adds units to the run card. ALARM on the command line (or the incident's Upgrade button) moves an incident up one, and the recommendation fills the bigger card.</p>
              {(d.alarms ?? []).map((al, ai) => (
                <div key={ai} className="mb-2 rounded border border-slate-800 p-2">
                  <div className="flex items-center gap-2">
                    <input aria-label={`Alarm ${ai + 2} name`} className="input w-40" value={al.name} onChange={(e) => setD({ ...d, alarms: (d.alarms ?? []).map((x, j) => (j === ai ? { ...x, name: e.target.value } : x)) })} />
                    <span className="text-xs text-slate-500">adds</span>
                    <button className="btn btn-ghost ml-auto px-1.5" aria-label={`Remove the ${al.name}`} onClick={() => setD({ ...d, alarms: (d.alarms ?? []).filter((_, j) => j !== ai) })}><X size={13} aria-hidden /></button>
                  </div>
                  <ul className="mt-1 space-y-1">
                    {al.runCard.map((r, i) => (
                      <li key={i} className="flex items-center gap-2">
                        <input aria-label={`${al.name} line ${i + 1} count`} type="number" min={1} max={20} className="input w-20 font-mono" value={r.count} onChange={(e) => setD({ ...d, alarms: (d.alarms ?? []).map((x, j) => (j === ai ? { ...x, runCard: x.runCard.map((y, k) => (k === i ? { ...y, count: Number(e.target.value) } : y)) } : x)) })} />
                        <span className="text-slate-500">×</span>
                        <input aria-label={`${al.name} line ${i + 1} capability`} list="caps" className="input font-mono" value={r.capability} onChange={(e) => setD({ ...d, alarms: (d.alarms ?? []).map((x, j) => (j === ai ? { ...x, runCard: x.runCard.map((y, k) => (k === i ? { ...y, capability: e.target.value.trim().toLowerCase() } : y)) } : x)) })} />
                        <button className="btn btn-ghost px-1.5" aria-label={`Remove ${al.name} line ${i + 1}`} onClick={() => setD({ ...d, alarms: (d.alarms ?? []).map((x, j) => (j === ai ? { ...x, runCard: x.runCard.filter((_, k) => k !== i) } : x)) })}><X size={13} aria-hidden /></button>
                      </li>
                    ))}
                  </ul>
                  <button className="btn mt-1 h-7" onClick={() => setD({ ...d, alarms: (d.alarms ?? []).map((x, j) => (j === ai ? { ...x, runCard: [...x.runCard, { capability: '', count: 1 }] } : x)) })}><Plus size={12} aria-hidden /> Add line</button>
                </div>
              ))}
              <button className="btn" onClick={() => setD({ ...d, alarms: [...(d.alarms ?? []), { name: ordinal((d.alarms?.length ?? 0) + 2) + ' alarm', runCard: [{ capability: 'engine', count: 2 }] }] })}><Plus size={13} aria-hidden /> Add the {ordinal((d.alarms?.length ?? 0) + 2)} alarm</button>
            </div>
            <label className="flex items-start gap-2 text-sm text-slate-300 sm:col-span-2">
              <input type="checkbox" className="mt-1" checked={!!d.par} onChange={(e) => setD({ ...d, par: e.target.checked })} />
              <span>Fire-ground accountability<span className="block text-xs text-slate-500">The roll-call (PAR) clock starts when the first unit arrives</span></span>
            </label>
            <Field label="Roll call every (minutes)" htmlFor="cpm" hint="0: the setting (20 unless changed)"><input id="cpm" type="number" min={0} max={60} className="input w-24 font-mono" value={d.parMinutes ?? 0} onChange={(e) => setD({ ...d, parMinutes: Number(e.target.value) })} /></Field>
            <Field label="NEMSIS dispatch reason" htmlFor="cnem" hint="For EMS patient-care reports (eDispatch.01); empty: suggested from the name"><input id="cnem" className="input font-mono" placeholder="2301…" value={d.nemsis ?? ''} onChange={(e) => setD({ ...d, nemsis: e.target.value.trim() })} /></Field>
            <Field label="Safety timer (minutes)" htmlFor="cck" hint="Status check for units on scene of this type, e.g. a traffic stop"><input id="cck" type="number" min={0} max={600} className="input w-24 font-mono" value={d.checkMinutes ?? 0} onChange={(e) => setD({ ...d, checkMinutes: Number(e.target.value) })} /></Field>
            {agencies.length > 0 && (
              <div className="sm:col-span-4">
                <span className="label">Agencies</span>
                <p className="mb-1 text-xs text-slate-500">None ticked means every agency.</p>
                <div className="flex flex-wrap gap-2">
                  {agencies.map((a) => (
                    <label key={a.code} className="flex items-center gap-1 text-sm text-slate-300">
                      <input type="checkbox" checked={(d.agencies ?? []).includes(a.code)} onChange={(e) => setD({ ...d, agencies: e.target.checked ? [...(d.agencies ?? []), a.code] : (d.agencies ?? []).filter((x) => x !== a.code) })} aria-label={`${a.code} — ${a.name}`} />
                      {a.code}
                    </label>
                  ))}
                </div>
              </div>
            )}
          </div>
        </Modal>
      )}
    </Page>
  )
}

function ordinal(n: number) {
  return n === 2 ? '2nd' : n === 3 ? '3rd' : `${n}th`
}
