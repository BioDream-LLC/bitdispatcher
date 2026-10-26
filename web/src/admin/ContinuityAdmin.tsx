import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { ArrowRightLeft, Plus, Save, Trash2, Wifi } from 'lucide-react'
import { api, errText } from '../api'
import { Field } from '../ui'
import type { View } from '../types'
import { Page } from './Admin'
import type { ConnCheck, ContinuityPolicy, PartnerOverflow, PartnerView } from '../continuity/types'

const lines = (s: string) => s.split('\n').map((x) => x.trim()).filter(Boolean)

/** Admin › Continuity: the internet checks, the relocation checklist and contacts, the continuity pack,
 * and the overflow agreed with each neighbouring centre. */
export function ContinuityAdmin({ view }: { view: View }) {
  const cur = view.settings.continuity ?? {}
  const [p, setP] = useState<ContinuityPolicy>(cur)
  const [steps, setSteps] = useState((cur.steps ?? []).join('\n'))
  const [contacts, setContacts] = useState((cur.contacts ?? []).join('\n'))
  const [dirs, setDirs] = useState((cur.packDirs ?? []).join('\n'))
  const [busy, setBusy] = useState(false)
  const checks = p.checks ?? []
  const setCheck = (i: number, c: Partial<ConnCheck>) => setP({ ...p, checks: checks.map((x, j) => (j === i ? { ...x, ...c } : x)) })

  const save = async () => {
    setBusy(true)
    try {
      const continuity: ContinuityPolicy = { ...p, checks: checks.filter((c) => c.name.trim() || c.url.trim()), steps: lines(steps), contacts: lines(contacts), packDirs: lines(dirs) }
      await api.put('/admin/settings', { ...view.settings, continuity })
      toast.success('Continuity saved')
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  const num = (v: string) => (v === '' ? 0 : Math.max(0, Math.round(Number(v))))

  return (
    <Page
      title="Continuity"
      intro="How the centre keeps working through an outage: the checks that show the internet is up, the checklist for moving to the backup site, the continuity pack written for working on paper, and overflow to neighbouring centres. The backup site itself is a second bitdispatcher site (Admin › Sites), in a building the county owns or in the cloud."
      actions={<button className="btn btn-go" disabled={busy} onClick={save}><Save size={14} aria-hidden /> Save</button>}
    >
      <div className="grid gap-4 xl:grid-cols-2">
        <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="ca-checks">
          <h2 id="ca-checks" className="flex items-center gap-2 font-semibold text-slate-100"><Wifi size={15} aria-hidden /> Internet checks</h2>
          <p className="text-xs text-slate-400">Addresses bitdispatcher asks to show the internet works; any one answering is enough. Choose ones the centre relies on, such as the state's message switch portal, the paging provider and a large public site. When none answers, every console shows the internet down and the supervisors get a message. BitVoice, the other sites and overflow neighbours are checked as well, without being listed here.</p>
          {checks.map((c, i) => (
            <div key={i} className="flex gap-2">
              <input className="input w-40 text-sm" placeholder="Name" value={c.name} onChange={(e) => setCheck(i, { name: e.target.value })} aria-label={`Check ${i + 1} name`} />
              <input className="input flex-1 font-mono text-xs" placeholder="https://" value={c.url} onChange={(e) => setCheck(i, { url: e.target.value })} aria-label={`Check ${i + 1} address`} />
              <button className="btn btn-ghost px-1.5" onClick={() => setP({ ...p, checks: checks.filter((_, j) => j !== i) })} aria-label={`Remove check ${i + 1}`}><Trash2 size={13} aria-hidden /></button>
            </div>
          ))}
          <button className="btn text-xs" disabled={checks.length >= 10} onClick={() => setP({ ...p, checks: [...checks, { name: '', url: 'https://' }] })}><Plus size={12} aria-hidden /> Add a check</button>
          <Field label="Check every (seconds)" htmlFor="ca-secs" hint="10 to 600; empty for 30.">
            <input id="ca-secs" type="number" min={10} max={600} className="input w-28" value={p.checkSeconds || ''} onChange={(e) => setP({ ...p, checkSeconds: num(e.target.value) })} />
          </Field>
        </section>

        <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="ca-reloc">
          <h2 id="ca-reloc" className="font-semibold text-slate-100">Moving to the backup site</h2>
          <Field label="Checklist" htmlFor="ca-steps" hint="One step a line, in order. A supervisor ticks them on the Continuity page; each tick is recorded.">
            <textarea id="ca-steps" className="input w-full text-sm" rows={7} value={steps} onChange={(e) => setSteps(e.target.value)}
              placeholder={'Call the 911 authority to reroute 911 to the backup site\nSign in at the backup site consoles\nCheck radio control stations at the backup site\nTell field units by radio'} />
          </Field>
          <Field label="Contacts" htmlFor="ca-contacts" hint="One a line: printed on the continuity pack and shown on the Continuity page.">
            <textarea id="ca-contacts" className="input w-full text-sm" rows={4} value={contacts} onChange={(e) => setContacts(e.target.value)} placeholder="Backup site: 540-555-0199" />
          </Field>
        </section>

        <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="ca-pack">
          <h2 id="ca-pack" className="font-semibold text-slate-100">Continuity pack</h2>
          <p className="text-xs text-slate-400">A page of the open incidents (with callers' names and numbers), the units, the checklist, the contacts and blank log lines. Written to the data directory and to each folder below; a folder on another machine, a shared drive or a USB disk keeps a copy if this server is lost. Files are readable by the server's account only.</p>
          <Field label="Write every (minutes)" htmlFor="ca-pmin" hint="1 to 240; empty for not on a schedule (it can still be opened and printed).">
            <input id="ca-pmin" type="number" min={0} max={240} className="input w-28" value={p.packMinutes || ''} onChange={(e) => setP({ ...p, packMinutes: num(e.target.value) })} />
          </Field>
          <Field label="Folders" htmlFor="ca-dirs" hint="Full paths, one a line, up to 5.">
            <textarea id="ca-dirs" className="input w-full font-mono text-xs" rows={3} value={dirs} onChange={(e) => setDirs(e.target.value)} placeholder="/mnt/county-share/cad" />
          </Field>
        </section>

        <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="ca-ovf">
          <h2 id="ca-ovf" className="flex items-center gap-2 font-semibold text-slate-100"><ArrowRightLeft size={15} aria-hidden /> Overflow</h2>
          <p className="text-xs text-slate-400">A neighbouring centre that runs bitdispatcher takes calls for this one while it is overwhelmed or out of its building, or this one takes theirs. Agree it in writing first, with the 911 authority, which sets up the phone network's alternate routing that sends the 911 calls over. Each call taken becomes an incident in the asking centre's CAD; the helper sees only the calls it took.</p>
          <Field label="An overflow lasts (minutes)" htmlFor="ca-omin" hint="Unless ended sooner; the supervisor asking can choose. Empty for 240.">
            <input id="ca-omin" type="number" min={0} max={10080} className="input w-28" value={p.overflowMinutes || ''} onChange={(e) => setP({ ...p, overflowMinutes: num(e.target.value) })} />
          </Field>
          <PartnersOverflow view={view} />
        </section>
      </div>
    </Page>
  )
}

function PartnersOverflow({ view }: { view: View }) {
  const [ps, setPs] = useState<PartnerView[] | null>(null)
  const load = () => api.get<PartnerView[]>('/admin/partners').then(setPs).catch((e) => toast.error(errText(e)))
  useEffect(() => { load() }, [])
  if (!ps) return <p className="text-xs text-slate-500">Loading the neighbouring agencies…</p>
  if (ps.length === 0) return <p className="text-xs text-slate-400">No neighbouring agency yet. Add one in <Link className="underline" to="/admin/integrations">Admin › Integrations</Link> with its CAD's address (https://their-cad/api/cad/eido/&lt;the ID they gave us&gt;) and a token each way.</p>
  return <div className="space-y-3">{ps.map((p) => <PartnerRow key={p.id} p={p} view={view} onSaved={load} />)}</div>
}

function PartnerRow({ p, view, onSaved }: { p: PartnerView; view: View; onSaved: () => void }) {
  const [o, setO] = useState<PartnerOverflow>({ ...p.overflow })
  const [agencies, setAgencies] = useState((p.overflow.agencies ?? []).join(', '))
  const [busy, setBusy] = useState(false)
  const types = [...view.callTypes].sort((a, b) => a.name.localeCompare(b.name))
  const chosen = new Set(o.callTypes ?? [])
  const save = async () => {
    setBusy(true)
    try {
      await api.put(`/admin/partners/${p.id}/overflow`, { ...o, agencies: agencies.split(',').map((x) => x.trim().toUpperCase()).filter(Boolean) })
      toast.success(`Overflow with ${p.name} saved`)
      onSaved()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  const box = (k: keyof PartnerOverflow, label: string, hint: string) => (
    <label className="flex items-start gap-2 text-sm text-slate-300">
      <input type="checkbox" className="mt-1" checked={!!o[k]} onChange={(e) => setO({ ...o, [k]: e.target.checked })} />
      <span>{label}<span className="block text-xs text-slate-500">{hint}</span></span>
    </label>
  )
  return (
    <div className="rounded border border-slate-800 p-3">
      <div className="flex items-baseline gap-2">
        <span className="font-semibold text-slate-100">{p.name}</span>
        <span className="font-mono text-[11px] text-slate-500">{p.id}</span>
        <button className="btn btn-go ml-auto h-7 text-xs" disabled={busy} onClick={save}><Save size={12} aria-hidden /> Save</button>
      </div>
      {p.overflowProblem && <p className="mt-1 text-xs text-amber-300">{p.overflowProblem}</p>}
      {(!p.inTokenSet || !p.outTokenSet) && <p className="mt-1 text-xs text-amber-300">Set both tokens in Admin › Integrations before turning overflow on.</p>}
      <div className="mt-2 grid gap-2 md:grid-cols-2">
        {box('ask', 'We may ask them to take our calls', 'Our supervisors see "Ask them to take our calls".')}
        {box('dispatch', 'While they take our calls, they may assign our units', 'Otherwise our dispatchers send our units. Assignments they make are recorded under their names.')}
        {box('help', 'We take their calls when they ask', 'Our supervisor accepts each request, unless the next box is ticked.')}
        {box('autoAccept', 'Accept their requests on our own', 'For an agreement that says yes in advance; our supervisors and call takers are told at once.')}
      </div>
      <Field label="Our agencies they may take calls for" htmlFor={`ag-${p.id}`} hint="Codes, separated by commas; empty for all. With one agency, their calls are its incidents; their units are the ones shared for assigning.">
        <input id={`ag-${p.id}`} className="input w-full font-mono text-xs" value={agencies} onChange={(e) => setAgencies(e.target.value)} placeholder="RPD, RFD" />
      </Field>
      <details className="mt-1 text-xs text-slate-400">
        <summary className="cursor-pointer">Call types they may use: {chosen.size ? `${chosen.size} chosen` : 'all'}</summary>
        <div className="mt-1 grid max-h-48 grid-cols-2 gap-x-3 overflow-auto md:grid-cols-3">
          {types.map((t) => (
            <label key={t.code} className="flex items-center gap-1">
              <input type="checkbox" checked={chosen.has(t.code)} onChange={(e) => {
                const n = new Set(chosen)
                if (e.target.checked) n.add(t.code); else n.delete(t.code)
                setO({ ...o, callTypes: [...n] })
              }} /> {t.name} <span className="font-mono text-slate-500">{t.code}</span>
            </label>
          ))}
        </div>
      </details>
    </div>
  )
}
