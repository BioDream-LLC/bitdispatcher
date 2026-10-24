import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Plus, Save, Trash2, Truck } from 'lucide-react'
import { api, errText } from '../api'
import { Field, Modal } from '../ui'
import { cx, hhmm } from '../util'
import type { Tow, TowCompany, TowTurn, View } from '../types'
import { Page, Table } from './Admin'

type Draft = TowCompany & { isNew: boolean }

const blank = (): TowCompany => ({ id: '', name: '', phone: '', classes: [], zones: [] })

const KIND: Record<Tow['kind'], string> = {
  rotation: 'rotation',
  owner: "owner's choice",
  police: "officer's choice",
}

/** Tow rotation: the companies on call, whose turn is next by class and zone, and the recent tows. */
export default function Tows({ view }: { view: View }) {
  const companies = view.roadmap?.towCompanies ?? []
  const towClasses = view.roadmap?.towClasses ?? []
  const zones = view.settings.towZones ?? []
  const [d, setD] = useState<Draft | null>(null)

  const open = (c?: TowCompany) => {
    setD(c ? { ...c, classes: c.classes ?? [], zones: c.zones ?? [], isNew: false } : { ...blank(), isNew: true })
  }
  const toggle = (field: 'classes' | 'zones', v: string) => {
    if (!d) return
    const cur = d[field] ?? []
    const has = cur.includes(v)
    setD({ ...d, [field]: has ? cur.filter((x) => x !== v) : [...cur, v] })
  }
  const save = async () => {
    if (!d) return
    if (!d.name.trim()) { toast.error('A name is needed'); return }
    try {
      await api.put<{ id: string }>('/admin/tows', { ...d, name: d.name.trim() })
      toast.success(`${d.name.trim()} saved`)
      setD(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const remove = async (id: string) => {
    if (!confirm('Remove this tow company?')) return
    try {
      await api.del(`/admin/tows/${encodeURIComponent(id)}`)
      setD(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }

  return (
    <Page
      title="Tow rotation"
      intro="The tow companies on call, and whose turn comes next by class and zone. A company with no zones is on call for every zone. Suspend a company to hold it out of the rotation without removing it."
      actions={<button className="btn btn-go" onClick={() => open()}><Plus size={14} aria-hidden /> Add a company</button>}
    >
      <Table head={['Name', 'Phone', 'Classes', 'Zones', 'State']} empty={companies.length === 0 ? <p className="p-3 text-sm text-slate-500">No tow companies yet.</p> : undefined}>
        {companies.map((c) => (
          <tr key={c.id} className="cursor-pointer hover:bg-slate-800/50" onClick={() => open(c)}>
            <td className="px-3 py-1.5 text-slate-100">{c.name}</td>
            <td className="px-3 font-mono text-slate-400">{c.phone || '—'}</td>
            <td className="px-3">
              <div className="flex flex-wrap gap-1">
                {(c.classes ?? []).length ? (c.classes ?? []).map((x) => <span key={x} className="chip bg-slate-800 text-slate-300">{x}</span>) : <span className="text-slate-500">—</span>}
              </div>
            </td>
            <td className="px-3 text-slate-400">{(c.zones ?? []).length ? (c.zones ?? []).join(', ') : <span className="text-slate-500">every zone</span>}</td>
            <td className="px-3 text-xs">{c.off ? <span className="text-amber-300">suspended</span> : <span className="text-emerald-300">on call</span>}</td>
          </tr>
        ))}
      </Table>

      <Rotation towClasses={towClasses} zones={zones} />
      <RecentTows />

      {d && (
        <Modal
          open
          wide
          onClose={() => setD(null)}
          title={d.isNew ? 'New tow company' : d.name}
          icon={<Truck size={16} className="text-amber-300" aria-hidden />}
          footer={
            <>
              {!d.isNew && <button className="btn mr-auto text-rose-300" onClick={() => remove(d.id)}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => setD(null)}>Cancel</button>
              <button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save</button>
            </>
          }
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Name" htmlFor="tn"><input id="tn" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} /></Field>
            <Field label="Phone" htmlFor="tp"><input id="tp" className="input font-mono" value={d.phone} onChange={(e) => setD({ ...d, phone: e.target.value })} /></Field>
            <div className="sm:col-span-2">
              <Field label="Address" htmlFor="ta"><input id="ta" className="input" value={d.address ?? ''} onChange={(e) => setD({ ...d, address: e.target.value || undefined })} /></Field>
            </div>
            <div className="sm:col-span-2">
              <span className="label">Classes it handles</span>
              <div className="flex flex-wrap gap-1.5">
                {towClasses.map((x) => {
                  const on = (d.classes ?? []).includes(x)
                  return <button key={x} type="button" className={cx('chip cursor-pointer', on ? 'bg-amber-500/20 text-amber-200' : 'bg-slate-800 text-slate-400 hover:bg-slate-700')} aria-pressed={on} onClick={() => toggle('classes', x)}>{x}</button>
                })}
                {towClasses.length === 0 && <span className="text-xs text-slate-500">No tow classes set. Add them in Admin › Settings.</span>}
              </div>
            </div>
            <div className="sm:col-span-2">
              <span className="label">Zones</span>
              {zones.length ? (
                <div className="flex flex-wrap gap-1.5">
                  {zones.map((z) => {
                    const on = (d.zones ?? []).includes(z)
                    return <button key={z} type="button" className={cx('chip cursor-pointer', on ? 'bg-amber-500/20 text-amber-200' : 'bg-slate-800 text-slate-400 hover:bg-slate-700')} aria-pressed={on} onClick={() => toggle('zones', z)}>{z}</button>
                  })}
                </div>
              ) : (
                <p className="text-xs text-slate-500">One zone for the whole area: add zones in Admin › Settings.</p>
              )}
              {zones.length > 0 && <p className="mt-1 text-xs text-slate-500">Leave all off for every zone.</p>}
            </div>
            <div className="sm:col-span-2">
              <Field label="Note" htmlFor="tnote"><input id="tnote" className="input" value={d.note ?? ''} onChange={(e) => setD({ ...d, note: e.target.value || undefined })} /></Field>
            </div>
            <label className="flex items-center gap-2 text-sm text-slate-300 sm:col-span-2">
              <input type="checkbox" checked={!!d.off} onChange={(e) => setD({ ...d, off: e.target.checked })} /> Suspended (held out of the rotation)
            </label>
          </div>
        </Modal>
      )}
    </Page>
  )
}

/** Rotation shows whose turn is next for a chosen class and zone, and who was passed and why. */
function Rotation({ towClasses, zones }: { towClasses: string[]; zones: string[] }) {
  const [cls, setCls] = useState('')
  const [zone, setZone] = useState('')
  const [data, setData] = useState<{ eligible: TowTurn[]; passed: TowTurn[] } | null>(null)
  useEffect(() => {
    if (towClasses.length && !cls) setCls(towClasses[0])
  }, [towClasses, cls])
  useEffect(() => {
    if (!cls) { setData(null); return }
    const q = `/tows/rotation?class=${encodeURIComponent(cls)}&zone=${encodeURIComponent(zone)}`
    api.get<{ eligible: TowTurn[]; passed: TowTurn[] }>(q)
      .then((r) => setData({ eligible: r.eligible ?? [], passed: r.passed ?? [] }))
      .catch((e) => { setData(null); toast.error(errText(e)) })
  }, [cls, zone])

  return (
    <div className="panel mt-6 rounded-lg p-4">
      <h2 className="font-semibold text-slate-100">Rotation</h2>
      <p className="mt-1 text-sm text-slate-400">Whose turn comes next for this class and zone. The one marked next is first to call.</p>
      <div className="mt-3 flex flex-wrap items-end gap-3">
        <Field label="Class" htmlFor="rcls">
          <select id="rcls" className="input" value={cls} onChange={(e) => setCls(e.target.value)}>
            {towClasses.length === 0 && <option value="">no classes set</option>}
            {towClasses.map((x) => <option key={x} value={x}>{x}</option>)}
          </select>
        </Field>
        <Field label="Zone" htmlFor="rzone">
          <select id="rzone" className="input" value={zone} onChange={(e) => setZone(e.target.value)}>
            <option value="">{zones.length ? 'every zone' : 'the whole area'}</option>
            {zones.map((z) => <option key={z} value={z}>{z}</option>)}
          </select>
        </Field>
      </div>
      {data && (
        <div className="mt-4 space-y-4">
          <div>
            <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500">In turn</h3>
            {data.eligible.length ? (
              <ul className="mt-1 space-y-1 text-sm">
                {data.eligible.map((t, i) => (
                  <li key={t.company.id} className="flex items-center gap-2 text-slate-200">
                    {i === 0 ? <span className="chip bg-emerald-500/20 text-emerald-200">1 next</span> : <span className="chip bg-slate-800 text-slate-400">{i + 1}</span>}
                    <span>{t.company.name}</span>
                    <span className="text-xs text-slate-500">{t.lastTurn ? `last turn ${hhmm(t.lastTurn)}` : 'no turn yet'}</span>
                  </li>
                ))}
              </ul>
            ) : <p className="mt-1 text-sm text-slate-500">No company is in turn for this class and zone.</p>}
          </div>
          {data.passed.length > 0 && (
            <div>
              <h3 className="text-xs font-semibold uppercase tracking-wider text-slate-500">Passed</h3>
              <ul className="mt-1 space-y-1 text-sm text-slate-500">
                {data.passed.map((t) => (
                  <li key={t.company.id} className="flex items-center gap-2">
                    <span>{t.company.name}</span>
                    {t.why && <span className="text-xs">— {t.why}</span>}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

/** RecentTows lists the latest tows: how the company was picked, and how it went. */
function RecentTows() {
  const [items, setItems] = useState<Tow[]>([])
  useEffect(() => {
    api.get<{ items: Tow[] }>('/tows')
      .then((r) => setItems(r.items ?? []))
      .catch((e) => toast.error(errText(e)))
  }, [])
  return (
    <div className="mt-6">
      <h2 className="mb-2 font-semibold text-slate-100">Recent tows</h2>
      <Table head={['Time', 'Incident', 'Company', 'Class', 'Zone', 'Picked', 'Result', 'Skipped']} empty={items.length === 0 ? <p className="p-3 text-sm text-slate-500">No tows recorded yet.</p> : undefined}>
        {items.map((t) => (
          <tr key={t.id} className="hover:bg-slate-800/50">
            <td className="px-3 py-1.5 font-mono text-xs text-slate-400">{hhmm(t.at)}</td>
            <td className="px-3 font-mono text-slate-300">{t.incident || '—'}</td>
            <td className="px-3 text-slate-100">{t.name || t.company}</td>
            <td className="px-3 text-slate-400">{t.class || '—'}</td>
            <td className="px-3 text-slate-400">{t.zone || '—'}</td>
            <td className="px-3 text-slate-400">{KIND[t.kind] ?? t.kind}</td>
            <td className="px-3 text-slate-300">{t.result}</td>
            <td className="px-3 text-xs text-slate-500">{(t.skipped ?? []).length ? (t.skipped ?? []).join(', ') : '—'}</td>
          </tr>
        ))}
      </Table>
    </div>
  )
}
