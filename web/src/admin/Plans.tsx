import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ArrowDown, ArrowUp, Plus, Route, Trash2, X } from 'lucide-react'
import { api, errText } from '../api'
import { Field, Modal } from '../ui'
import { cx } from '../util'
import type { ResponsePlan, View } from '../types'
import { Page, Table } from './Admin'

const blank = (): ResponsePlan => ({ id: '', name: '', callTypes: [], modifiers: [], area: '', addresses: [], from: '', to: '', order: [], note: '' })

/** Response plans: run orders. The recommendation follows a plan's due order when one applies. */
export default function Plans({ view }: { view: View }) {
  const [items, setItems] = useState<ResponsePlan[]>([])
  const [d, setD] = useState<(ResponsePlan & { isNew: boolean }) | null>(null)
  const [addUnit, setAddUnit] = useState('')
  const modifiers = view.entry?.modifiers ?? view.settings.modifiers ?? []
  const load = () => api.get<ResponsePlan[]>('/admin/plans').then(setItems).catch((e) => toast.error(errText(e)))
  useEffect(() => {
    load()
  }, [])
  const open = (p?: ResponsePlan) => {
    setAddUnit('')
    setD(p ? { ...p, isNew: false } : { ...blank(), isNew: true })
  }
  const save = async () => {
    if (!d) return
    const body: Omit<ResponsePlan, 'updated'> = {
      id: d.id, name: d.name, callTypes: d.callTypes, modifiers: d.modifiers, area: d.area,
      addresses: (d.addresses ?? []).map((a) => a.trim()).filter(Boolean), from: d.from, to: d.to, order: d.order, note: d.note,
    }
    try {
      await api.put<{ id: string }>(d.isNew ? '/admin/plans/new' : `/admin/plans/${encodeURIComponent(d.id)}`, body)
      toast.success(`Plan ${d.name || d.id} saved`)
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const remove = async (id: string) => {
    if (!confirm('Remove this plan?')) return
    try {
      await api.del(`/admin/plans/${encodeURIComponent(id)}`)
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const toggle = (key: 'callTypes' | 'modifiers', code: string) =>
    d && setD({ ...d, [key]: d[key]?.includes(code) ? d[key]!.filter((x) => x !== code) : [...(d[key] ?? []), code] })
  const moveUnit = (i: number, dir: -1 | 1) => {
    if (!d) return
    const order = [...d.order]
    const j = i + dir
    if (j < 0 || j >= order.length) return
    ;[order[i], order[j]] = [order[j], order[i]]
    setD({ ...d, order })
  }
  const stationName = (id?: string) => (id ? view.stations.find((s) => s.id === id)?.name ?? id : 'Anywhere')
  return (
    <Page
      title="Response plans"
      intro="Run orders. The recommendation follows a plan's due order when one applies; if a planned unit cannot go it says why and who goes instead. A person always decides."
      actions={<button className="btn btn-go" onClick={() => open()}><Plus size={14} aria-hidden /> Add a plan</button>}
    >
      <Table head={['Name', 'Call types', 'Area', 'Due order']} empty={items.length === 0 ? <p className="p-3 text-sm text-slate-500">No plans yet.</p> : undefined}>
        {items.map((p) => (
          <tr key={p.id} className="cursor-pointer hover:bg-slate-800/50" onClick={() => open(p)}>
            <td className="px-3 py-1.5 text-slate-100">{p.name || <span className="text-slate-500">unnamed</span>}</td>
            <td className="px-3 text-xs text-slate-300">{(p.callTypes ?? []).length ? p.callTypes!.join(' ') : <span className="text-slate-500">every type</span>}</td>
            <td className="px-3 text-slate-400">{stationName(p.area)}</td>
            <td className="px-3 font-mono text-xs text-slate-300">{p.order.join(' → ') || <span className="text-slate-500">none</span>}</td>
          </tr>
        ))}
      </Table>

      {d && (
        <Modal
          open
          wide
          onClose={() => setD(null)}
          title={d.isNew ? 'New response plan' : d.name || 'Response plan'}
          icon={<Route size={16} className="text-indigo-300" aria-hidden />}
          footer={
            <>
              {!d.isNew && <button className="btn mr-auto text-rose-300" onClick={() => remove(d.id)}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => setD(null)}>Cancel</button>
              <button className="btn btn-go" onClick={save}>Save</button>
            </>
          }
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="sm:col-span-2"><Field label="Name" htmlFor="pln"><input id="pln" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} placeholder="Structure fire, downtown" /></Field></div>
            <div className="sm:col-span-2">
              <div className="label">Call types</div>
              <p className="mb-1 text-xs text-slate-500">Empty means every type.</p>
              <div className="flex max-h-28 flex-wrap gap-1 overflow-y-auto">
                {view.callTypes.map((c) => (
                  <button key={c.code} type="button" aria-pressed={d.callTypes?.includes(c.code)} onClick={() => toggle('callTypes', c.code)}
                    className={cx('chip cursor-pointer', d.callTypes?.includes(c.code) ? 'bg-indigo-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700')} title={c.name}>{c.code}</button>
                ))}
              </div>
            </div>
            {modifiers.length > 0 && (
              <div className="sm:col-span-2">
                <div className="label">Modifiers</div>
                <div className="flex flex-wrap gap-1">
                  {modifiers.map((m) => (
                    <button key={m.code} type="button" aria-pressed={d.modifiers?.includes(m.code)} onClick={() => toggle('modifiers', m.code)}
                      className={cx('chip cursor-pointer', d.modifiers?.includes(m.code) ? 'bg-amber-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700')} title={m.name}>{m.name}</button>
                  ))}
                </div>
              </div>
            )}
            <Field label="First-due station area" htmlFor="pla">
              <select id="pla" className="input" value={d.area ?? ''} onChange={(e) => setD({ ...d, area: e.target.value })}>
                <option value="">Anywhere</option>
                {view.stations.map((s) => <option key={s.id} value={s.id}>{s.id} · {s.name}</option>)}
              </select>
            </Field>
            <div className="grid grid-cols-2 gap-2">
              <Field label="From" htmlFor="plf" hint="HH:MM, optional"><input id="plf" type="time" className="input" value={d.from ?? ''} onChange={(e) => setD({ ...d, from: e.target.value })} /></Field>
              <Field label="To" htmlFor="plt" hint="HH:MM, optional"><input id="plt" type="time" className="input" value={d.to ?? ''} onChange={(e) => setD({ ...d, to: e.target.value })} /></Field>
            </div>
            <div className="sm:col-span-2"><Field label="Addresses" htmlFor="plad" hint="one per line, address-specific: the incident address starts with…"><textarea id="plad" className="input min-h-16 font-mono text-xs" value={(d.addresses ?? []).join('\n')} onChange={(e) => setD({ ...d, addresses: e.target.value.split('\n') })} placeholder="100 Main St&#10;Oak Ridge Mall" /></Field></div>
            <div className="sm:col-span-2">
              <div className="label">Due order</div>
              <ul className="space-y-1">
                {d.order.map((u, i) => (
                  <li key={u + i} className="flex items-center gap-2">
                    <span className="w-6 font-mono text-xs text-slate-500">{i + 1}.</span>
                    <span className="font-mono text-slate-100">{u}</span>
                    <span className="text-xs text-slate-500">{view.units.find((x) => x.id === u)?.name ?? ''}</span>
                    <span className="ml-auto flex gap-1">
                      <button className="btn btn-ghost px-1.5" aria-label={`Move ${u} up`} disabled={i === 0} onClick={() => moveUnit(i, -1)}><ArrowUp size={13} aria-hidden /></button>
                      <button className="btn btn-ghost px-1.5" aria-label={`Move ${u} down`} disabled={i === d.order.length - 1} onClick={() => moveUnit(i, 1)}><ArrowDown size={13} aria-hidden /></button>
                      <button className="btn btn-ghost px-1.5 text-rose-300" aria-label={`Remove ${u}`} onClick={() => setD({ ...d, order: d.order.filter((_, j) => j !== i) })}><X size={13} aria-hidden /></button>
                    </span>
                  </li>
                ))}
                {d.order.length === 0 && <li className="text-xs text-slate-500">No units yet; add one below.</li>}
              </ul>
              <div className="mt-2 flex items-center gap-2">
                <select className="input w-56" value={addUnit} onChange={(e) => setAddUnit(e.target.value)} aria-label="Add a unit to the due order">
                  <option value="">Add a unit…</option>
                  {view.units.filter((u) => !d.order.includes(u.id)).map((u) => <option key={u.id} value={u.id}>{u.id} {u.name ?? ''}</option>)}
                </select>
                <button className="btn" disabled={!addUnit} onClick={() => { if (addUnit) { setD({ ...d, order: [...d.order, addUnit] }); setAddUnit('') } }}><Plus size={13} aria-hidden /> Add</button>
              </div>
            </div>
            <div className="sm:col-span-2"><Field label="Note" htmlFor="pln2"><textarea id="pln2" className="input min-h-14" value={d.note ?? ''} onChange={(e) => setD({ ...d, note: e.target.value })} /></Field></div>
          </div>
        </Modal>
      )}
    </Page>
  )
}
