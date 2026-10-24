import { useEffect, useState } from 'react'
import { Landmark } from 'lucide-react'
import { toast } from 'sonner'
import { api, errText } from '../api'
import { useStore } from '../store'
import { Field, Modal } from '../ui'
import { cx } from '../util'
import type { AgencyView, Incident, View } from '../types'

// The agency a person is working for (whose rules apply) is always shown, with the agencies covered and
// watched; their usual coverage loads by itself at sign-in. A change here is recorded.

let cached: AgencyView | null = null
const listeners = new Set<(v: AgencyView) => void>()

export function useAgencies(): AgencyView | null {
  const me = useStore((s) => s.me)
  const [v, setV] = useState<AgencyView | null>(cached)
  useEffect(() => {
    if (!me) return
    const f = (x: AgencyView) => setV(x)
    listeners.add(f)
    api.get<AgencyView>('/agencies').then((x) => { cached = x; listeners.forEach((l) => l(x)) }).catch(() => {})
    return () => { listeners.delete(f) }
  }, [me?.username, me?.duty, me?.console])
  return v
}

function publish(v: AgencyView) {
  cached = v
  listeners.forEach((l) => l(v))
}

export function WorkingAgency() {
  const v = useAgencies()
  const [open, setOpen] = useState(false)
  if (!v || v.agencies.length === 0) return null
  const c = v.coverage
  const name = (code: string) => v.agencies.find((g) => g.code === code)?.name ?? code
  const label = c.working ? `Working for ${name(c.working)}` : 'No working agency'
  return (
    <>
      <button type="button" onClick={() => setOpen(true)} className={cx('chip h-6 shrink-0 gap-1 whitespace-nowrap px-2 text-[11px] font-bold', c.working ? 'bg-indigo-500/20 text-indigo-100' : 'bg-amber-500/20 text-amber-200')}
        aria-label={`${label}. Covering ${c.covered.join(', ') || 'none'}; watching ${c.monitored.join(', ') || 'none'}. Change`} title={`${label}: its rules apply. Covering ${c.covered.join(', ') || '—'}; watching ${c.monitored.join(', ') || '—'}`}>
        <Landmark size={12} aria-hidden />{c.working || 'agency?'}
        {c.covered.length > 1 && <span className="font-normal opacity-80">+{c.covered.length - 1}</span>}
        {c.monitored.length > 0 && <span className="font-normal opacity-70">· watch {c.monitored.length}</span>}
      </button>
      {open && <CoverageDialog v={v} onClose={() => setOpen(false)} />}
    </>
  )
}

function CoverageDialog({ v, onClose }: { v: AgencyView; onClose: () => void }) {
  const [working, setWorking] = useState(v.coverage.working)
  const [covered, setCovered] = useState<string[]>(v.coverage.covered)
  const [watched, setWatched] = useState<string[]>(v.coverage.monitored)
  const [busy, setBusy] = useState(false)
  const toggle = (list: string[], set: (x: string[]) => void, code: string) => set(list.includes(code) ? list.filter((x) => x !== code) : [...list, code])
  const save = async () => {
    setBusy(true)
    try {
      const c = await api.put<AgencyView['coverage']>('/me/coverage', { working, covered, monitored: watched })
      publish({ ...v, coverage: c })
      toast.success(`Working for ${c.working}`)
      onClose()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal open onClose={onClose} title="Agencies at this position" icon={<Landmark size={16} aria-hidden />}
      footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn btn-go" disabled={busy || !working} onClick={save}>Save</button></>}>
      <p className="mb-3 text-xs text-slate-400">The working agency's rules apply to what you do (its incident numbers, timers, call types and plans). Covered agencies are those you dispatch for here; watched ones are shown for awareness only. Your usual coverage loads by itself at sign-in.</p>
      <Field label="Working agency" htmlFor="cov-working">
        <select id="cov-working" className="input" value={working} onChange={(e) => { setWorking(e.target.value); if (!covered.includes(e.target.value)) setCovered([...covered, e.target.value]) }}>
          <option value="">Choose…</option>
          {v.may.map((code) => <option key={code} value={code}>{code} — {v.agencies.find((g) => g.code === code)?.name}</option>)}
        </select>
      </Field>
      <fieldset className="mt-3">
        <legend className="label">Covered</legend>
        <div className="flex flex-wrap gap-2">
          {v.may.map((code) => (
            <label key={code} className="flex items-center gap-1 text-sm"><input type="checkbox" checked={covered.includes(code)} onChange={() => toggle(covered, setCovered, code)} /> {code}</label>
          ))}
        </div>
      </fieldset>
      <fieldset className="mt-3">
        <legend className="label">Watched</legend>
        <div className="flex flex-wrap gap-2">
          {v.agencies.filter((g) => !covered.includes(g.code)).map((g) => (
            <label key={g.code} className="flex items-center gap-1 text-sm"><input type="checkbox" checked={watched.includes(g.code)} onChange={() => toggle(watched, setWatched, g.code)} /> {g.code}</label>
          ))}
        </div>
      </fieldset>
    </Modal>
  )
}

/** An incident's primary and assisting agencies; every unit keeps its own home agency. */
export function IncidentAgencies({ view, inc }: { view: View; inc: Incident }) {
  const v = useAgencies()
  const [edit, setEdit] = useState(false)
  const [primary, setPrimary] = useState(inc.agency ?? '')
  const [assisting, setAssisting] = useState<string[]>(inc.assisting ?? [])
  const [why, setWhy] = useState('')
  if (!v || v.agencies.length === 0) return null
  const fromUnits = [...new Set(inc.units.map((a) => view.units.find((u) => u.id === a.unit)?.agency).filter((x): x is string => !!x && x !== inc.agency))]
  const all = [...new Set([...(inc.assisting ?? []), ...fromUnits])]
  const save = async () => {
    try {
      await api.put(`/incidents/${inc.number}/agency`, { agency: primary, assisting, reason: why })
      toast.success('Agencies changed')
      setEdit(false)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <div className="flex flex-wrap items-center gap-1 text-[11px]" aria-label="Agencies on this incident">
      <span className="chip bg-indigo-500/20 font-bold text-indigo-100" title="Primary agency">{inc.agency || 'no agency'}</span>
      {all.map((a) => <span key={a} className="chip bg-slate-700/60 text-slate-200" title="Assisting">assisting {a}</span>)}
      <button type="button" className="btn btn-ghost h-6 px-1.5 text-[11px]" onClick={() => setEdit(true)}>Agencies…</button>
      {edit && (
        <Modal open onClose={() => setEdit(false)} title={`Agencies on ${inc.number}`} icon={<Landmark size={16} aria-hidden />}
          footer={<><button className="btn" onClick={() => setEdit(false)}>Cancel</button><button className="btn btn-go" onClick={save}>Save</button></>}>
          <Field label="Primary agency" htmlFor="ia-primary">
            <select id="ia-primary" className="input" value={primary} onChange={(e) => setPrimary(e.target.value)}>
              <option value="">Choose…</option>
              {v.agencies.map((g) => <option key={g.code} value={g.code}>{g.code} — {g.name}</option>)}
            </select>
          </Field>
          <fieldset className="mt-3">
            <legend className="label">Assisting</legend>
            <div className="flex flex-wrap gap-2">
              {v.agencies.filter((g) => g.code !== primary).map((g) => (
                <label key={g.code} className="flex items-center gap-1 text-sm"><input type="checkbox" checked={assisting.includes(g.code)} onChange={() => setAssisting(assisting.includes(g.code) ? assisting.filter((x) => x !== g.code) : [...assisting, g.code])} /> {g.code}</label>
              ))}
            </div>
          </fieldset>
          <p className="mt-2 text-xs text-slate-400">Units from other agencies show as assisting by themselves and keep their home agency.</p>
          <Field label="Reason" htmlFor="ia-why" hint="Needed when the primary agency changes">
            <input id="ia-why" className="input" value={why} onChange={(e) => setWhy(e.target.value)} />
          </Field>
        </Modal>
      )}
    </div>
  )
}
