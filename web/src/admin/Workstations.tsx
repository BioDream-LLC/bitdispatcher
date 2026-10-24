import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { KeyRound, MonitorSmartphone, Plus, Save, Trash2, X } from 'lucide-react'
import { api, errText } from '../api'
import { Field, Modal } from '../ui'
import { useAgencies } from '../console/Coverage'
import { ago, cx, useNow } from '../util'
import type { Workstation } from '../types'
import { Page } from './Admin'

// The duties a workstation may offer, in the order the agency speaks them, with plain names.
const DUTIES: [string, string][] = [
  ['calltaker', 'Call-taker'],
  ['police', 'Police dispatcher'],
  ['fire', 'Fire dispatcher'],
  ['ems', 'EMS dispatcher'],
  ['dispatcher', 'Dispatcher (all disciplines)'],
  ['supervisor', 'Supervisor'],
  ['admin', 'Administrator'],
  ['records', 'Records'],
]

/** Workstations: the agency's console positions. A browser enrolled as one signs in there, and nobody
 * is asked which console they are at. */
export default function Workstations() {
  const now = useNow()
  const agencies = useAgencies()?.agencies ?? []
  const [items, setItems] = useState<Workstation[]>([])
  const [d, setD] = useState<(Workstation & { isNew: boolean }) | null>(null)
  const [newId, setNewId] = useState('')
  const [enrolLabel, setEnrolLabel] = useState('')
  const [code, setCode] = useState<{ code: string; expires: string } | null>(null)
  const here = localStorage.getItem('bd.ws.name') ?? ''
  const load = () => api.get<Workstation[]>('/admin/workstations').then(setItems).catch((e) => toast.error(errText(e)))
  useEffect(() => {
    load()
  }, [])
  const open = (w?: Workstation) => setD(w ? { ...w, isNew: false } : { id: '', duties: [], enrolled: [], isNew: true })
  const save = async () => {
    if (!d) return
    try {
      await api.put(`/admin/workstations/${encodeURIComponent(d.id.trim())}`, { name: d.name ?? '', duties: d.duties ?? [], agencies: d.agencies ?? [], monitor: d.monitor ?? [] })
      toast.success(`Workstation ${d.id} saved`)
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const remove = async (id: string) => {
    if (!confirm(`Remove workstation ${id}?`)) return
    try {
      await api.del(`/admin/workstations/${encodeURIComponent(id)}`)
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const toggle = (duty: string) =>
    d && setD({ ...d, duties: d.duties?.includes(duty) ? d.duties.filter((x) => x !== duty) : [...(d.duties ?? []), duty] })
  // Enrolling THIS browser as the workstation: the token is kept here so every sign-in names the console.
  const enrol = async (id: string) => {
    try {
      const r = await api.post<{ token: string; workstation: string }>(`/admin/workstations/${encodeURIComponent(id)}/enrol`, { label: enrolLabel.trim() })
      localStorage.setItem('bd.ws', r.token)
      localStorage.setItem('bd.ws.name', r.workstation)
      toast.success(`This browser is now ${r.workstation}`)
      setEnrolLabel('')
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  // A one-time code enrols another computer from its own sign-in page.
  const getCode = async (id: string) => {
    try {
      setCode(await api.post<{ code: string; expires: string }>(`/admin/workstations/${encodeURIComponent(id)}/code`))
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const revoke = async (id: string, hash: string) => {
    try {
      await api.del(`/admin/workstations/${encodeURIComponent(id)}/enrolled/${encodeURIComponent(hash)}`)
      load()
      setD((x) => (x && x.id === id ? { ...x, enrolled: (x.enrolled ?? []).filter((e) => e.hash !== hash) } : x))
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Page
      title="Workstations"
      intro="The agency's console positions. A browser enrolled as a workstation signs in there, and nobody is asked which console they are at. Enrol this browser, or hand another computer a one-time code from its own sign-in page."
      actions={
        <form className="flex items-center gap-2" onSubmit={(e) => { e.preventDefault(); if (newId.trim()) { open({ id: newId.trim().toUpperCase(), duties: [], enrolled: [] }); setNewId('') } }}>
          <input className="input w-36 font-mono uppercase" placeholder="New id, e.g. CT1" value={newId} onChange={(e) => setNewId(e.target.value)} aria-label="New workstation id" />
          <button className="btn btn-go" type="submit"><Plus size={14} aria-hidden /> Add</button>
        </form>
      }
    >
      <div className="grid gap-3 lg:grid-cols-2">
        {items.length === 0 && <p className="text-sm text-slate-500">No workstations yet. Add one above.</p>}
        {items.map((w) => (
          <section key={w.id} className={cx('panel space-y-2 rounded-lg p-3', here === w.id && 'ring-1 ring-emerald-500/50')}>
            <header className="flex items-center gap-2">
              <MonitorSmartphone size={16} className="text-sky-300" aria-hidden />
              <span className="font-mono font-bold text-slate-100">{w.id}</span>
              {w.name && <span className="text-slate-300">{w.name}</span>}
              {here === w.id && <span className="chip bg-emerald-500/20 text-emerald-200">this browser</span>}
              <button className="btn btn-ghost ml-auto px-1.5" onClick={() => open(w)} aria-label={`Edit ${w.id}`}><Save size={13} aria-hidden /></button>
              <button className="btn btn-ghost px-1.5 text-rose-300" onClick={() => remove(w.id)} aria-label={`Remove ${w.id}`}><Trash2 size={13} aria-hidden /></button>
            </header>
            <div>
              <div className="label">Duties offered</div>
              <div className="text-xs text-slate-300">
                {(w.duties ?? []).length
                  ? DUTIES.filter(([c]) => w.duties?.includes(c)).map(([, n]) => n).join(' · ')
                  : <span className="text-slate-500">any duty</span>}
              </div>
            </div>
            <div>
              <div className="label">Enrolled browsers</div>
              {(w.enrolled ?? []).length === 0 ? (
                <p className="text-xs text-slate-500">None yet.</p>
              ) : (
                <ul className="space-y-1 text-xs">
                  {w.enrolled!.map((e) => (
                    <li key={e.hash} className="flex items-center gap-2">
                      <span className="text-slate-200">{e.label || 'a browser'}</span>
                      <span className="text-slate-500">{ago(e.at, now)} ago{e.by && ` · by ${e.by}`}</span>
                      <button className="btn btn-ghost ml-auto px-1.5 text-rose-300" onClick={() => revoke(w.id, e.hash)} aria-label={`Revoke ${e.label || 'browser'}`}><X size={12} aria-hidden /> Revoke</button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
            <div className="flex flex-wrap items-center gap-2 border-t border-slate-800 pt-2">
              <input className="input h-8 w-40 py-0 text-xs" placeholder="Label for this browser" value={enrolLabel} onChange={(e) => setEnrolLabel(e.target.value)} aria-label={`Label for enrolling ${w.id}`} />
              <button className="btn text-xs" onClick={() => enrol(w.id)}><MonitorSmartphone size={13} aria-hidden /> Enrol this browser</button>
              <button className="btn text-xs" onClick={() => getCode(w.id)}><KeyRound size={13} aria-hidden /> Get a code for another computer</button>
            </div>
          </section>
        ))}
      </div>

      {d && (
        <Modal
          open
          onClose={() => setD(null)}
          title={d.isNew ? 'New workstation' : `Workstation ${d.id}`}
          icon={<MonitorSmartphone size={16} className="text-sky-300" aria-hidden />}
          footer={
            <>
              {!d.isNew && <button className="btn mr-auto text-rose-300" onClick={() => remove(d.id)}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => setD(null)}>Cancel</button>
              <button className="btn btn-go" onClick={save}>Save</button>
            </>
          }
        >
          <div className="space-y-3">
            <Field label="ID" htmlFor="wid" hint="short, e.g. CT1 or DISP2"><input id="wid" className="input font-mono uppercase" disabled={!d.isNew} value={d.id} onChange={(e) => setD({ ...d, id: e.target.value })} /></Field>
            <Field label="Name" htmlFor="wn" hint="optional: where it sits, in words"><input id="wn" className="input" value={d.name ?? ''} onChange={(e) => setD({ ...d, name: e.target.value })} placeholder="Call-taker 1, north wall" /></Field>
            <div>
              <div className="label">Duties offered</div>
              <p className="mb-1 text-xs text-slate-500">Pick none to offer every duty.</p>
              <div className="flex flex-wrap gap-1">
                {DUTIES.map(([code, name]) => (
                  <button key={code} type="button" aria-pressed={d.duties?.includes(code)} onClick={() => toggle(code)}
                    className={cx('rounded px-1.5 py-0.5 text-[11px]', d.duties?.includes(code) ? 'bg-sky-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700')}>{name}</button>
                ))}
              </div>
            </div>
            {agencies.length > 0 && (
              <div>
                <div className="label">Agencies covered here</div>
                <div className="flex flex-wrap gap-2">
                  {agencies.map((a) => (
                    <label key={a.code} className="flex items-center gap-1 text-sm text-slate-300">
                      <input type="checkbox" checked={(d.agencies ?? []).includes(a.code)} onChange={(e) => setD({ ...d, agencies: e.target.checked ? [...(d.agencies ?? []), a.code] : (d.agencies ?? []).filter((x) => x !== a.code) })} aria-label={`Cover ${a.code} — ${a.name}`} />
                      {a.code}
                    </label>
                  ))}
                </div>
              </div>
            )}
            {agencies.length > 0 && (
              <div>
                <div className="label">Agencies watched here</div>
                <div className="flex flex-wrap gap-2">
                  {agencies.map((a) => (
                    <label key={a.code} className="flex items-center gap-1 text-sm text-slate-300">
                      <input type="checkbox" checked={(d.monitor ?? []).includes(a.code)} onChange={(e) => setD({ ...d, monitor: e.target.checked ? [...(d.monitor ?? []), a.code] : (d.monitor ?? []).filter((x) => x !== a.code) })} aria-label={`Watch ${a.code} — ${a.name}`} />
                      {a.code}
                    </label>
                  ))}
                </div>
              </div>
            )}
          </div>
        </Modal>
      )}

      {code && (
        <Modal open onClose={() => setCode(null)} title="Enrolment code" icon={<KeyRound size={16} className="text-amber-300" aria-hidden />}>
          <div className="space-y-3 text-center">
            <p className="text-sm text-slate-300">On the other computer, open the sign-in page, choose “Enrol this computer” and type this code. It works once, for 24 hours.</p>
            <div className="select-all rounded-lg border border-amber-700 bg-amber-950/30 py-6 font-mono text-5xl font-bold tracking-[0.3em] text-amber-200" aria-label="Enrolment code">{code.code}</div>
            <p className="text-xs text-slate-500">Expires {new Date(code.expires).toLocaleString()}</p>
          </div>
        </Modal>
      )}
    </Page>
  )
}
