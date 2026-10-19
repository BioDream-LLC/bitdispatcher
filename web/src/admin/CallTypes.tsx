import { useState } from 'react'
import { toast } from 'sonner'
import { ClipboardList, Plus, Trash2, X } from 'lucide-react'
import { api, ApiError, errText } from '../api'
import { Field, Modal, Priority } from '../ui'
import type { Requirement, View } from '../types'
import { Page, Table } from './Admin'

type Draft = { code: string; name: string; priority: number; discipline: string; runCard: Requirement[]; critical?: boolean; drone?: boolean; isNew: boolean }

export default function CallTypes({ view }: { view: View }) {
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
      await api.put(`/admin/calltypes/${encodeURIComponent(d.code.trim().toUpperCase())}`, { name: d.name, priority: d.priority, discipline: d.discipline, runCard: d.runCard, critical: !!d.critical, drone: !!d.drone })
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
      actions={<button className="btn btn-go" onClick={() => open()}><Plus size={14} aria-hidden /> Add call type</button>}
    >
      <Table head={['Code', 'Name', 'Priority', 'Discipline', 'Run card']}>
        {view.callTypes.map((c) => (
          <tr key={c.code} className="cursor-pointer hover:bg-slate-800/50" onClick={() => open(c.code)}>
            <td className="px-3 py-1.5 font-mono font-bold text-slate-100">{c.code}</td>
            <td className="px-3 text-slate-200">{c.name}{c.critical && <span className="chip ml-1 bg-teal-500/20 text-teal-200">critical</span>}{c.drone && <span className="chip ml-1 bg-cyan-500/20 text-cyan-200">drone</span>}</td>
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
            <label className="flex items-start gap-2 text-sm text-slate-300 sm:col-span-2">
              <input type="checkbox" className="mt-1" checked={!!d.critical} onChange={(e) => setD({ ...d, critical: e.target.checked })} />
              <span>Critical call<span className="block text-xs text-slate-500">Hard on the people who work it: they go on the supervisor's care list</span></span>
            </label>
            <label className="flex items-start gap-2 text-sm text-slate-300 sm:col-span-2">
              <input type="checkbox" className="mt-1" checked={!!d.drone} onChange={(e) => setD({ ...d, drone: e.target.checked })} />
              <span>Send a drone<span className="block text-xs text-slate-500">With the drone launch rule on (Integrations), these ask for one by themselves</span></span>
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
          </div>
        </Modal>
      )}
    </Page>
  )
}
