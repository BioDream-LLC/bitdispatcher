import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Copy, KeyRound, Plus, Save, Siren, Trash2, Upload } from 'lucide-react'
import { api, errText } from '../api'
import { Field, Modal } from '../ui'
import type { View } from '../types'
import { Page } from './Admin'

type Company = { id: string; name: string; phone?: string; answerUrl?: string; answerTokenSet?: boolean; off?: boolean; address: string }
type Draft = { id: string; name: string; phone: string; answerUrl: string; answerToken: string; off: boolean; token?: string }

/** Alarm companies and permits: each central station gets its own address and token, the permit register
 * is checked on every alarm, and repeat false alarms are flagged. */
export default function AlarmsAdmin({ view }: { view: View }) {
  const [list, setList] = useState<Company[]>([])
  const [permits, setPermits] = useState(0)
  const [d, setD] = useState<Draft | null>(null)
  const [shown, setShown] = useState<{ name: string; token: string; address: string } | null>(null)
  const file = useRef<HTMLInputElement>(null)
  const pol = view.settings.next?.alarms ?? {}
  const load = () => api.get<{ companies: Company[]; permits: number }>('/admin/alarms').then((r) => { setList(r.companies); setPermits(r.permits) }).catch((e) => toast.error(errText(e)))
  useEffect(() => {
    load()
  }, [])
  const save = async (x: Draft) => {
    try {
      const r = await api.put<{ id: string; token: string; address: string }>('/admin/alarms', x)
      if (r.token) setShown({ name: x.name, token: r.token, address: location.origin + location.pathname.replace(/[^/]*$/, '') + r.address.replace(/^\//, '') })
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const upload = async (f: File) => {
    try {
      const res = await fetch('./api/admin/alarms/permits', { method: 'PUT', headers: { 'X-BitDispatcher': '1', 'Content-Type': 'text/csv' }, body: await f.text(), credentials: 'same-origin' })
      const b = await res.json()
      if (!res.ok) throw new Error(b.message ?? res.statusText)
      toast.success(`${b.permits} permits on the register`)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const savePolicy = async (falseLimit: number, falseWords: string[]) => {
    try {
      await api.put('/admin/settings', { ...view.settings, next: { ...view.settings.next, alarms: { falseLimit, falseWords } } })
      toast.success('Saved')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Page title="Alarm companies & permits" intro="Alarms arrive as calls in the queue, data only, for a dispatcher to make an incident of. Each company the agency works with gets its own address and token, so its alarms arrive named and its answers (accepted, dispatched, on scene, closed) go back to it alone. ASAP over Nlets, the national standard, still needs the standard's schema and the state's message switch."
      actions={<button className="btn" onClick={() => setD({ id: '', name: '', phone: '', answerUrl: '', answerToken: '', off: false })}><Plus size={13} aria-hidden /> Add a company</button>}>
      <section className="panel mb-4 rounded-lg p-4">
        <h2 className="mb-2 flex items-center gap-2 font-semibold text-slate-100"><Siren size={15} className="text-amber-300" aria-hidden /> Companies</h2>
        {!list.length ? <p className="text-sm text-slate-500">None yet. Companies not on the list can still send with the shared hook token (Admin › Integrations); a call-taker types a phoned alarm with Phoned alarm, above the calls.</p> : (
          <table className="w-full text-sm">
            <thead className="text-left text-[11px] uppercase text-slate-500"><tr><th>Company</th><th>Operators</th><th>Its address here</th><th>Answers to</th><th /></tr></thead>
            <tbody>
              {list.map((c) => (
                <tr key={c.id} className="border-t border-slate-800">
                  <td className="py-1.5 text-slate-100">{c.name}{c.off && <span className="ml-1 text-[10px] text-amber-300">off</span>}</td>
                  <td className="font-mono text-xs text-slate-400">{c.phone}</td>
                  <td className="font-mono text-xs text-slate-400">{c.address}</td>
                  <td className="font-mono text-xs text-slate-400">{c.answerUrl || 'no answers'}</td>
                  <td className="whitespace-nowrap text-right">
                    <button className="btn btn-ghost h-6 px-1.5 text-[11px]" onClick={() => setD({ id: c.id, name: c.name, phone: c.phone ?? '', answerUrl: c.answerUrl ?? '', answerToken: '', off: !!c.off })}>Edit</button>
                    <button className="btn btn-ghost h-6 px-1.5 text-[11px]" title="Make a new token (the old one stops working)" onClick={() => confirm(`New token for ${c.name}? The old one stops working.`) && save({ id: c.id, name: c.name, phone: c.phone ?? '', answerUrl: c.answerUrl ?? '', answerToken: '', off: !!c.off, token: 'new' })}><KeyRound size={12} aria-hidden /></button>
                    <button className="btn btn-ghost h-6 px-1.5" aria-label={`Remove ${c.name}`} onClick={async () => { if (confirm(`Remove ${c.name}?`)) { try { await api.del(`/admin/alarms/${c.id}`); load() } catch (e) { toast.error(errText(e)) } } }}><Trash2 size={12} aria-hidden /></button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
      <section className="panel space-y-3 rounded-lg p-4 text-sm">
        <h2 className="font-semibold text-slate-100">Permits and false alarms</h2>
        <p className="text-xs text-slate-400">Each alarm is checked against the register (by permit number, else by address): valid, expired, suspended, or none on file. Its address's false alarms in the last year are counted from the incidents' dispositions.</p>
        <div className="flex items-center gap-3">
          <input ref={file} type="file" accept=".csv,text/csv" className="hidden" onChange={(e) => e.target.files?.[0] && upload(e.target.files[0])} />
          <button className="btn" onClick={() => file.current?.click()}><Upload size={13} aria-hidden /> Load the register (CSV)</button>
          <span className="text-slate-400">{permits} permits now</span>
        </div>
        <p className="text-[11px] text-slate-500">Columns, with a heading row: number, address, holder, company, expires (YYYY-MM-DD), status (active, suspended, revoked). Loading replaces the register.</p>
        <div className="grid grid-cols-[10rem_1fr] gap-3">
          <Field label="Flag at (false alarms a year)" htmlFor="fl"><input id="fl" type="number" min={1} max={100} className="input font-mono" defaultValue={pol.falseLimit || 3} onBlur={(e) => savePolicy(Number(e.target.value), pol.falseWords ?? [])} /></Field>
          <Field label="Dispositions that are false alarms" htmlFor="fw" hint="Separated by commas; empty: any with “false” in it"><input id="fw" className="input" defaultValue={(pol.falseWords ?? []).join(', ')} onBlur={(e) => savePolicy(pol.falseLimit || 3, e.target.value.split(',').map((x) => x.trim()).filter(Boolean))} /></Field>
        </div>
      </section>
      <Modal open={!!d} onClose={() => setD(null)} title={d?.id ? 'Edit company' : 'Add an alarm company'} footer={<><button className="btn" onClick={() => setD(null)}>Cancel</button><button className="btn btn-go" disabled={!d?.name.trim()} onClick={() => d && save(d)}><Save size={13} aria-hidden /> Save</button></>}>
        {d && (
          <div className="space-y-2 text-sm">
            <Field label="Name" htmlFor="an"><input id="an" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} placeholder="Acme Monitoring" /></Field>
            <Field label="Its operators' line" htmlFor="ap"><input id="ap" className="input font-mono" value={d.phone} onChange={(e) => setD({ ...d, phone: e.target.value })} /></Field>
            <Field label="Where to send what became of its alarms (optional)" htmlFor="au"><input id="au" className="input font-mono text-xs" value={d.answerUrl} onChange={(e) => setD({ ...d, answerUrl: e.target.value })} placeholder="https://cad-link.acme.example/answers" /></Field>
            <Field label="Token for that address (optional; - to clear)" htmlFor="at"><input id="at" type="password" autoComplete="off" className="input" value={d.answerToken} onChange={(e) => setD({ ...d, answerToken: e.target.value })} /></Field>
            <label className="flex items-center gap-2 text-slate-300"><input type="checkbox" checked={d.off} onChange={(e) => setD({ ...d, off: e.target.checked })} /> Not taking its alarms</label>
          </div>
        )}
      </Modal>
      <Modal open={!!shown} onClose={() => setShown(null)} title={`Give these to ${shown?.name ?? ''}`} icon={<KeyRound size={16} aria-hidden />}>
        {shown && (
          <div className="space-y-2 text-sm">
            <p className="text-xs text-amber-300">The token is shown this once. Send it to the company by a safe route, not in the same message as the address.</p>
            <div><div className="text-[11px] text-slate-500">Address (POST, JSON; see docs/ALERTS.md)</div><code className="break-all text-xs">{shown.address}</code></div>
            <div><div className="text-[11px] text-slate-500">Token (Authorization: Bearer)</div><code className="break-all text-xs">{shown.token}</code> <button aria-label="Copy the token" onClick={() => navigator.clipboard?.writeText(shown.token)}><Copy size={11} aria-hidden /></button></div>
          </div>
        )}
      </Modal>
    </Page>
  )
}
