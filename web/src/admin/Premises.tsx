import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Plus, Save, ShieldAlert, Trash2, X } from 'lucide-react'
import { api, errText } from '../api'
import AddressInput from '../AddressInput'
import { Field } from '../ui'
import { cx } from '../util'
import type { Premise } from '../types'
import { Page } from './Admin'

type List = { total: number; matched: number; premises: Premise[] }
const blank = (): Premise => ({ id: '', address: '', lat: 0, lon: 0, hazards: [], contacts: [], updated: '', by: '' })
const COMMON = ['Dog', 'Oxygen in use', 'Officer safety', 'Armed occupant', 'Knox box', 'Hazardous materials', 'Bariatric patient', 'Hearing impaired', 'Sprinklered', 'Vacant structure']

/** Premise information: hazards, notes and contacts at a place, shown with every incident there. */
export default function Premises() {
  const [q, setQ] = useState('')
  const [list, setList] = useState<List | null>(null)
  const [d, setD] = useState<Premise | null>(null)
  const [hz, setHz] = useState('')
  const load = () => api.get<List>(`/premises?q=${encodeURIComponent(q)}`).then(setList).catch((e) => toast.error(errText(e)))
  useEffect(() => {
    const t = setTimeout(load, 200)
    return () => clearTimeout(t)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q])
  const save = async () => {
    if (!d) return
    try {
      await api.put('/premises', { ...d, hazards: [...(d.hazards ?? []), ...(hz.trim() ? [hz.trim()] : [])] })
      toast.success('Saved')
      setD(null)
      setHz('')
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const remove = async (p: Premise) => {
    try {
      await api.del(`/premises/${encodeURIComponent(p.id)}`)
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const toggle = (h: string) => d && setD({ ...d, hazards: d.hazards?.includes(h) ? d.hazards.filter((x) => x !== h) : [...(d.hazards ?? []), h] })
  return (
    <Page title="Premises" intro="What the centre knows about a place before anyone calls: hazards to the crews, notes and contacts. A premise is shown with every incident within 40 m of it, on the console and the mobile terminal, hazards first. Bring premises over from an earlier system in Migration."
      actions={<button className="btn btn-go" onClick={() => setD(blank())}><Plus size={14} aria-hidden /> Add a premise</button>}>
      <div className="grid gap-4 lg:grid-cols-[1fr_24rem]">
        <section className="panel rounded-lg">
          <div className="flex items-center gap-2 border-b border-slate-800 p-2">
            <input className="input" placeholder="Search address, name, hazard or id" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Search premises" />
            <span className="shrink-0 text-xs text-slate-500">{list ? `${list.matched.toLocaleString()} of ${list.total.toLocaleString()}` : ''}</span>
          </div>
          <ul className="max-h-[70vh] overflow-y-auto text-sm">
            {list?.premises.length === 0 && <li className="p-3 text-slate-500">No premises{q && ' match'}.</li>}
            {list?.premises.map((p) => (
              <li key={p.id}>
                <button className={cx('block w-full border-b border-slate-800/60 px-3 py-1.5 text-left hover:bg-slate-800/50', d?.id === p.id && 'bg-slate-800')} onClick={() => { setD({ ...p }); setHz('') }}>
                  <div className="flex items-center gap-2">
                    {(p.hazards?.length ?? 0) > 0 && <ShieldAlert size={13} className="text-rose-300" aria-label="has hazards" />}
                    <span className="text-slate-100">{p.address}</span>{p.name && <span className="text-slate-400">· {p.name}</span>}
                    <span className="ml-auto font-mono text-[10px] text-slate-500">{p.id}</span>
                  </div>
                  {(p.hazards?.length ?? 0) > 0 && <div className="text-xs text-rose-200">{p.hazards!.join(' · ')}</div>}
                </button>
              </li>
            ))}
          </ul>
        </section>
        {d && (
          <section className="panel space-y-2 rounded-lg p-3">
            <h2 className="flex items-center text-sm font-semibold text-slate-100">{d.id ? `Premise ${d.id}` : 'New premise'}
              <button className="btn btn-ghost ml-auto px-1" onClick={() => setD(null)} aria-label="Close"><X size={13} /></button></h2>
            <Field label="Address" htmlFor="pa" hint={d.lat ? `${d.lat.toFixed(5)}, ${d.lon.toFixed(5)}` : 'pick it from the suggestions to place it'}>
              <AddressInput id="pa" value={d.address} onChange={(v) => setD({ ...d, address: v })} onPick={(r) => setD({ ...d, address: r.label, lat: r.lat, lon: r.lon })} />
            </Field>
            <Field label="Name" htmlFor="pn"><input id="pn" className="input" value={d.name ?? ''} onChange={(e) => setD({ ...d, name: e.target.value })} placeholder="business or common name" /></Field>
            <div>
              <div className="label">Hazards</div>
              <div className="flex flex-wrap gap-1">
                {[...new Set([...COMMON, ...(d.hazards ?? [])])].map((h) => (
                  <button key={h} type="button" aria-pressed={d.hazards?.includes(h)} onClick={() => toggle(h)}
                    className={cx('rounded px-1.5 py-0.5 text-[11px]', d.hazards?.includes(h) ? 'bg-rose-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700')}>{h}</button>
                ))}
              </div>
              <input className="input mt-1 text-xs" placeholder="another hazard, then Enter" value={hz} onChange={(e) => setHz(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter' && hz.trim()) { setD({ ...d, hazards: [...(d.hazards ?? []), hz.trim()] }); setHz('') } }} />
            </div>
            <Field label="Notes" htmlFor="pno"><textarea id="pno" className="input min-h-20" value={d.notes ?? ''} onChange={(e) => setD({ ...d, notes: e.target.value })} /></Field>
            <Field label="Contacts" htmlFor="pc" hint="one per line: key holder, owner, alarm company"><textarea id="pc" className="input min-h-14" value={(d.contacts ?? []).join('\n')} onChange={(e) => setD({ ...d, contacts: e.target.value.split('\n') })} /></Field>
            <Field label="Expires" htmlFor="pe" hint="optional: a temporary hazard"><input id="pe" type="date" className="input" value={d.expires ?? ''} onChange={(e) => setD({ ...d, expires: e.target.value })} /></Field>
            {d.source && <p className="text-[11px] text-slate-500">From {d.source}</p>}
            <div className="flex gap-2">
              <button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save</button>
              {d.id && <button className="btn ml-auto" onClick={() => remove(d)}><Trash2 size={13} aria-hidden /> Remove</button>}
            </div>
          </section>
        )}
      </div>
    </Page>
  )
}
