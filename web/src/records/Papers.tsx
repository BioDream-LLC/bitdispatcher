import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { ArrowLeft, Gavel, Plus, Save } from 'lucide-react'
import { api, errText } from '../api'
import { useStore } from '../store'
import { Field, Logo, Modal } from '../ui'
import { cx, hhmm } from '../util'
import type { Paper } from '../types'

const KIND: Record<string, string> = {
  arrest: 'Arrest warrant', bench: 'Bench warrant', 'protective-order': 'Protective order', eviction: 'Eviction', summons: 'Summons', subpoena: 'Subpoena', 'civil-other': 'Other civil paper',
}
const STATUS_CLS: Record<string, string> = { active: 'bg-rose-500/20 text-rose-200', served: 'bg-emerald-500/20 text-emerald-200', recalled: 'bg-slate-700 text-slate-300', returned: 'bg-amber-500/20 text-amber-200' }
const RESULTS = ['no answer', 'not there', 'moved', 'refused', 'served']

type Draft = { id: string; kind: string; number: string; court: string; name: string; dob: string; sex: string; description: string; address: string; charge: string; issued: string; expires: string; caution: string; notes: string }
const blank = (): Draft => ({ id: '', kind: 'arrest', number: '', court: '', name: '', dob: '', sex: '', description: '', address: '', charge: '', issued: '', expires: '', caution: '', notes: '' })

/** Warrants and civil process: the papers to serve, who they name, where, and every attempt. An incident at
 * the address of an active one says so on the console. Police dispatch, records and supervisors. */
export default function Papers() {
  const view = useStore((s) => s.view)
  const [status, setStatus] = useState('active')
  const [q, setQ] = useState('')
  const [items, setItems] = useState<Paper[] | null>(null)
  const [d, setD] = useState<Draft | null>(null)
  const [open, setOpen] = useState<Paper | null>(null)
  const [denied, setDenied] = useState('')
  const load = () => api.get<{ items: Paper[] }>(`/papers?status=${status}&q=${encodeURIComponent(q)}`).then((r) => { setItems(r.items); setOpen((o) => (o ? r.items.find((x) => x.id === o.id) ?? null : null)) }).catch((e) => setDenied(errText(e)))
  useEffect(() => {
    const t = setTimeout(load, 200)
    return () => clearTimeout(t)
  }, [status, q, view?.seq]) // eslint-disable-line react-hooks/exhaustive-deps
  const edit = (p?: Paper) => setD(p ? { id: p.id, kind: p.kind, number: p.number ?? '', court: p.court ?? '', name: p.subject.name, dob: p.subject.dob ?? '', sex: p.subject.sex ?? '', description: p.subject.description ?? '', address: p.location?.address ?? '', charge: p.charge ?? '', issued: p.issued?.slice(0, 10) ?? '', expires: p.expires?.slice(0, 10) ?? '', caution: p.caution ?? '', notes: p.notes ?? '' } : blank())
  const save = async () => {
    if (!d) return
    const old = items?.find((x) => x.id === d.id)
    try {
      await api.put('/papers', {
        id: d.id || undefined, kind: d.kind, number: d.number, court: d.court, subject: { name: d.name, dob: d.dob || undefined, sex: d.sex || undefined, description: d.description || undefined },
        location: d.address ? { ...(old?.location?.address === d.address ? old.location : {}), address: d.address } : undefined, charge: d.charge, caution: d.caution, notes: d.notes,
        issued: d.issued ? new Date(d.issued + 'T12:00:00').toISOString() : undefined, expires: d.expires ? new Date(d.expires + 'T12:00:00').toISOString() : undefined,
      })
      toast.success('Saved')
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <div className="flex h-full flex-col bg-slate-950">
      <header className="flex items-center gap-3 border-b border-slate-800 bg-slate-900 px-3 py-2">
        <Link to="/" className="btn btn-ghost"><ArrowLeft size={14} aria-hidden /> <Logo size={18} /> Dispatch</Link>
        <h1 className="flex items-center gap-2 text-lg font-bold text-slate-50"><Gavel size={18} className="text-rose-300" aria-hidden /> Warrants and papers to serve</h1>
        <button className="btn btn-go ml-auto" onClick={() => edit()}><Plus size={14} aria-hidden /> Enter a paper</button>
      </header>
      <main className="min-h-0 flex-1 overflow-y-auto p-4">
        {denied ? <p className="text-sm text-amber-300">{denied}</p> : (
          <div className="mx-auto max-w-6xl space-y-3">
            <p className="text-sm text-slate-400">Papers held to serve, with the person, the address and every attempt. An incident near the address of an active paper shows it on the console with its caution. Confirm every warrant with the issuing agency before acting on it; every look is recorded.</p>
            <div className="flex flex-wrap items-center gap-2">
              <div className="flex gap-1" role="radiogroup" aria-label="Status">
                {['active', 'served', 'recalled', 'returned', ''].map((s) => <button key={s} role="radio" aria-checked={status === s} className={cx('btn h-7', status === s && 'border-sky-400 bg-sky-500/15 text-sky-100')} onClick={() => setStatus(s)}>{s || 'all'}</button>)}
              </div>
              <input aria-label="Search papers" className="input ml-auto w-72" placeholder="name, number, address…" value={q} onChange={(e) => setQ(e.target.value)} />
            </div>
            <div className="panel overflow-x-auto rounded-lg">
              <table className="w-full text-sm">
                <thead><tr className="border-b border-slate-800 text-left text-[11px] uppercase tracking-wider text-slate-500"><th className="px-3 py-2">Paper</th><th>Kind</th><th>Person</th><th>Address</th><th>Charge or matter</th><th>Attempts</th><th>Status</th></tr></thead>
                <tbody className="divide-y divide-slate-800">
                  {(items ?? []).map((p) => (
                    <tr key={p.id} className="cursor-pointer hover:bg-slate-800/50" onClick={() => setOpen(p)}>
                      <td className="px-3 py-1.5 font-mono text-xs text-slate-300">{p.id}<div className="text-slate-500">{p.number}</div></td>
                      <td className="text-xs text-slate-300">{KIND[p.kind] ?? p.kind}</td>
                      <td className="text-slate-100">{p.subject.name}{p.caution && <span className="chip ml-1 bg-rose-600 text-white">{p.caution}</span>}<div className="font-mono text-[10px] text-slate-500">{p.subject.dob}</div></td>
                      <td className="text-xs text-slate-300">{p.location?.address}</td>
                      <td className="text-xs text-slate-400">{p.charge}</td>
                      <td className="font-mono text-xs">{p.attempts.length}</td>
                      <td><span className={cx('chip', STATUS_CLS[p.status])}>{p.status}</span></td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {items?.length === 0 && <p className="p-3 text-sm text-slate-500">None.</p>}
            </div>
          </div>
        )}
      </main>
      {open && <PaperDialog p={open} onClose={() => setOpen(null)} onEdit={() => { edit(open); setOpen(null) }} onChange={load} />}
      {d && (
        <Modal open onClose={() => setD(null)} wide title={d.id ? `${d.id}: ${d.name}` : 'Enter a warrant or paper'} icon={<Gavel size={16} className="text-rose-300" aria-hidden />}
          footer={<><button className="btn" onClick={() => setD(null)}>Cancel</button><button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save</button></>}>
          <div className="grid gap-3 sm:grid-cols-3">
            <Field label="Kind" htmlFor="pk"><select id="pk" className="input" value={d.kind} onChange={(e) => setD({ ...d, kind: e.target.value })}>{Object.entries(KIND).map(([k, n]) => <option key={k} value={k}>{n}</option>)}</select></Field>
            <Field label="Number" htmlFor="pn"><input id="pn" className="input font-mono" value={d.number} onChange={(e) => setD({ ...d, number: e.target.value })} /></Field>
            <Field label="Court" htmlFor="pc"><input id="pc" className="input" value={d.court} onChange={(e) => setD({ ...d, court: e.target.value })} /></Field>
            <Field label="Person named" htmlFor="pname"><input id="pname" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} /></Field>
            <Field label="Date of birth" htmlFor="pdob"><input id="pdob" type="date" className="input" value={d.dob} onChange={(e) => setD({ ...d, dob: e.target.value })} /></Field>
            <Field label="Sex" htmlFor="psex"><select id="psex" className="input" value={d.sex} onChange={(e) => setD({ ...d, sex: e.target.value })}><option value="" /> <option value="F">F</option><option value="M">M</option><option value="X">X</option><option value="U">unknown</option></select></Field>
            <div className="sm:col-span-3"><Field label="Description" htmlFor="pdesc"><input id="pdesc" className="input" value={d.description} onChange={(e) => setD({ ...d, description: e.target.value })} placeholder="height, build, hair, tattoos, vehicle" /></Field></div>
            <div className="sm:col-span-2"><Field label="Where to serve it" htmlFor="paddr" hint="Placed on the map from the recorded addresses"><input id="paddr" className="input" value={d.address} onChange={(e) => setD({ ...d, address: e.target.value })} /></Field></div>
            <Field label="Caution" htmlFor="pcau" hint="Shown to everyone sent there"><input id="pcau" className="input" value={d.caution} onChange={(e) => setD({ ...d, caution: e.target.value })} placeholder="armed, violent, dog" /></Field>
            <div className="sm:col-span-3"><Field label="Charge or matter" htmlFor="pch"><input id="pch" className="input" value={d.charge} onChange={(e) => setD({ ...d, charge: e.target.value })} /></Field></div>
            <Field label="Issued" htmlFor="piss"><input id="piss" type="date" className="input" value={d.issued} onChange={(e) => setD({ ...d, issued: e.target.value })} /></Field>
            <Field label="Serve by / court date" htmlFor="pexp"><input id="pexp" type="date" className="input" value={d.expires} onChange={(e) => setD({ ...d, expires: e.target.value })} /></Field>
            <div className="sm:col-span-3"><Field label="Notes" htmlFor="pnotes"><textarea id="pnotes" rows={2} className="input" value={d.notes} onChange={(e) => setD({ ...d, notes: e.target.value })} /></Field></div>
          </div>
        </Modal>
      )}
    </div>
  )
}

function PaperDialog({ p, onClose, onEdit, onChange }: { p: Paper; onClose: () => void; onEdit: () => void; onChange: () => void }) {
  const view = useStore((s) => s.view)
  const [unit, setUnit] = useState('')
  const [result, setResult] = useState(RESULTS[0])
  const [note, setNote] = useState('')
  const police = (view?.units ?? []).filter((u) => view?.settings.unitTypes.find((t) => t.code === u.type)?.discipline === 'police')
  const attempt = async () => {
    try {
      await api.post(`/papers/${p.id}/attempt`, { unit, result, note })
      toast.success(result === 'served' ? `${p.id} served` : 'Attempt recorded')
      setNote('')
      onChange()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const setStatus = async (s: string) => {
    const why = s === 'served' ? '' : prompt(`Why is ${p.id} ${s === 'active' ? 'active again' : s}?`)
    if (why === null) return
    try {
      await api.post(`/papers/${p.id}/status`, { status: s, note: why })
      onChange()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Modal open onClose={onClose} wide title={`${p.id}: ${KIND[p.kind] ?? p.kind}`} icon={<Gavel size={16} className="text-rose-300" aria-hidden />}
      footer={<><button className="btn mr-auto" onClick={onEdit}>Edit</button>{p.status === 'active' ? <><button className="btn" onClick={() => setStatus('recalled')}>Recalled</button><button className="btn" onClick={() => setStatus('returned')}>Returned unserved</button></> : <button className="btn" onClick={() => setStatus('active')}>Make active again</button>}<button className="btn" onClick={onClose}>Close</button></>}>
      <div className="space-y-3 text-sm">
        <div className="flex flex-wrap items-center gap-2"><b className="text-lg text-slate-50">{p.subject.name}</b>{p.subject.dob && <span className="font-mono text-slate-400">DOB {p.subject.dob}</span>}{p.subject.sex && <span className="text-slate-400">{p.subject.sex}</span>}<span className={cx('chip', STATUS_CLS[p.status])}>{p.status}</span>{p.caution && <span className="chip bg-rose-600 text-white">CAUTION: {p.caution}</span>}</div>
        {p.subject.description && <p className="text-slate-300">{p.subject.description}</p>}
        <dl className="grid grid-cols-[8rem_1fr] gap-x-3 gap-y-0.5 text-xs">
          <dt className="text-slate-500">Where</dt><dd className="text-slate-200">{p.location?.address}{p.location?.lat == null && <span className="ml-1 text-amber-300">(not on the map)</span>}</dd>
          <dt className="text-slate-500">Number, court</dt><dd className="text-slate-200">{p.number} {p.court}</dd>
          <dt className="text-slate-500">Charge or matter</dt><dd className="text-slate-200">{p.charge}</dd>
          <dt className="text-slate-500">Issued, serve by</dt><dd className="text-slate-200">{p.issued?.slice(0, 10)} {p.expires && `· by ${p.expires.slice(0, 10)}`}</dd>
          <dt className="text-slate-500">Entered</dt><dd className="text-slate-200">{new Date(p.entered).toLocaleString()} by {p.by}</dd>
        </dl>
        {p.notes && <pre className="whitespace-pre-wrap rounded bg-slate-900 p-2 text-xs text-slate-300">{p.notes}</pre>}
        <div>
          <span className="label">Attempts</span>
          <ul className="text-xs">
            {p.attempts.map((a, i) => <li key={i}><span className="font-mono text-slate-400">{new Date(a.at).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })}</span> {a.unit && <b className="font-mono">{a.unit}</b>} {a.result}{a.note ? `: ${a.note}` : ''} <span className="text-slate-500">· {a.by}{a.incident ? ` · ${a.incident}` : ''}</span></li>)}
            {p.attempts.length === 0 && <li className="text-slate-500">None yet.</li>}
          </ul>
        </div>
        {p.status === 'active' && (
          <div className="flex flex-wrap items-end gap-2 rounded border border-slate-800 p-2">
            <Field label="Unit" htmlFor="au"><select id="au" className="input w-28" value={unit} onChange={(e) => setUnit(e.target.value)}><option value="" />{police.map((u) => <option key={u.id}>{u.id}</option>)}</select></Field>
            <Field label="Result" htmlFor="ar"><select id="ar" className="input w-36" value={result} onChange={(e) => setResult(e.target.value)}>{RESULTS.map((r) => <option key={r}>{r}</option>)}</select></Field>
            <div className="min-w-48 flex-1"><Field label="Note" htmlFor="an"><input id="an" className="input" value={note} onChange={(e) => setNote(e.target.value)} /></Field></div>
            <button className="btn btn-go" onClick={attempt}>Record the attempt</button>
          </div>
        )}
        <p className="text-[10px] text-slate-500">Last change {p.statusAt ? `${hhmm(p.statusAt)} by ${p.statusBy}` : 'none'}.</p>
      </div>
    </Modal>
  )
}
