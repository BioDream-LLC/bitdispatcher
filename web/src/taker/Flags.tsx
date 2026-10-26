import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { ChevronDown, ChevronRight, Flag as FlagIcon, Plus, ShieldAlert } from 'lucide-react'
import { api, errText } from '../api'
import { cx } from '../util'
import type { Flag, FlagShown, Incident } from '../types'
import type { Draft } from './draft'

export type FlagSet = { flags: Flag[]; possible: Flag[]; shown: FlagShown[] }
const EMPTY: FlagSet = { flags: [], possible: [], shown: [] }

/** placeVerify is how the draft's location is settled. */
export function placeVerify(d: Draft): string {
  return d.placeVerify || (d.verified ? 'verified' : '')
}

/** useFlags are the flags that apply to the call being entered: at its verified address and sublocation
 * (shown before the incident is sent), or, for an unverified location, only possible matches nearby. */
export function useFlags(d: Draft, inc?: Incident): FlagSet {
  const [set, setSet] = useState<FlagSet>(EMPTY)
  const verified = placeVerify(d) === 'verified'
  const q = useMemo(() => {
    if (inc) return `/incidents/${inc.number}/flags`
    if (!d.address && !d.lat) return ''
    const p = new URLSearchParams({ address: d.address, verified: verified ? '1' : '0', name: d.placeName ?? '', complex: d.complex ?? '', building: d.building ?? '', floor: d.floor ?? '', unit: d.unit ?? '' })
    if (d.lat && d.lon) { p.set('lat', d.lat); p.set('lon', d.lon) }
    return `/flags?${p}`
  }, [inc?.number, inc?.updated, d.address, d.lat, d.lon, verified, d.placeName, d.complex, d.building, d.floor, d.unit]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!q) { setSet(EMPTY); return }
    let live = true
    const get = () => api.get<Partial<FlagSet>>(q).then((r) => { if (live) setSet({ flags: r.flags ?? [], possible: r.possible ?? [], shown: r.shown ?? [] }) }).catch(() => {})
    const t = window.setTimeout(get, 300)
    const iv = window.setInterval(get, 30000)
    return () => { live = false; window.clearTimeout(t); window.clearInterval(iv) }
  }, [q])
  return set
}

const SEV: Record<string, string> = { critical: 'border-rose-600 bg-rose-950/60', high: 'border-amber-600 bg-amber-950/40', medium: 'border-sky-700 bg-sky-950/30', low: 'border-slate-700 bg-slate-900' }

export function scopeLabel(f: Flag): string {
  const s = f.scope
  if (s.person) return 'person: ' + (s.personName || 'on this call')
  const parts = [s.business, s.complex, s.building && 'Bldg ' + s.building, s.floor && 'Floor ' + s.floor, s.unit && 'Unit ' + s.unit].filter(Boolean)
  return parts.length ? parts.join(', ') : 'the whole property'
}

/** One flag: severity and category in words, the title and what to do; the reason and source open when
 * the person may see them (restricted categories say so). */
export function FlagCard({ f, possible }: { f: Flag; possible?: boolean }) {
  const [open, setOpen] = useState(false)
  const [full, setFull] = useState<Flag | null>(null)
  const [why, setWhy] = useState('')
  const toggle = async () => {
    setOpen(!open)
    if (!open && !full && !f.warning) {
      try { setFull(await api.get<Flag>(`/flags/${f.id}`)) } catch (e) { setWhy(errText(e)) }
    }
  }
  const x = full ?? f
  return (
    <li className={cx('rounded border px-2 py-1 text-xs', SEV[f.severity] ?? SEV.low, possible && 'border-dashed opacity-90')}>
      <button type="button" className="flex w-full items-start gap-1.5 text-left" onClick={toggle} aria-expanded={open}>
        {open ? <ChevronDown size={12} className="mt-0.5 shrink-0" aria-hidden /> : <ChevronRight size={12} className="mt-0.5 shrink-0" aria-hidden />}
        <span className="min-w-0">
          <span className="font-bold uppercase">{f.severity}</span> <span className="uppercase text-slate-300">{f.category.replace(/-/g, ' ')}</span>{possible && <span className="ml-1 chip h-4 bg-slate-700 px-1 text-[9px]">POSSIBLE MATCH NEARBY</span>}
          <span className="block font-semibold text-slate-50">{f.title}</span>
          <span className="block text-slate-200">Responders: {f.instruction}</span>
          <span className="block text-[10px] text-slate-400">{scopeLabel(f)} · version {f.version}{f.expires && !f.expires.startsWith('0001') ? ` · until ${new Date(f.expires).toLocaleDateString()}` : ''}</span>
        </span>
      </button>
      {open && (
        <div className="ml-5 mt-1 space-y-0.5 border-t border-slate-700/60 pt-1 text-[11px] text-slate-200">
          {x.warning ? <p><b>Why:</b> {x.warning}</p> : <p className="text-slate-400">{why || f.note || 'The detail of this category is restricted.'}</p>}
          {x.source && <p><b>Source:</b> {x.source}{x.reliability ? ` (${x.reliability})` : ''}</p>}
          {x.details && Object.entries(x.details).map(([k, v]) => <p key={k}><b>{k}:</b> {v}</p>)}
          {possible && <p className="text-amber-200">Not attached to this call: the location is not verified. Confirm the location before relying on it.</p>}
        </div>
      )}
    </li>
  )
}

/** FlagsBox lists the call's flags and, for an unverified location, the possible matches nearby. */
export function FlagsBox({ set }: { set: FlagSet }) {
  if (!set.flags.length && !set.possible.length) return null
  return (
    <section aria-label="Premise and safety flags" className="space-y-1">
      {set.flags.length > 0 && (
        <>
          <h3 className="label flex items-center gap-1"><ShieldAlert size={12} aria-hidden /> Flags at this place ({set.flags.length})</h3>
          <ul className="space-y-1">{set.flags.map((f) => <FlagCard key={f.id} f={f} />)}</ul>
        </>
      )}
      {set.possible.length > 0 && (
        <>
          <h3 className="label">Possible matches near an unverified location</h3>
          <ul className="space-y-1">{set.possible.map((f) => <FlagCard key={f.id} f={f} possible />)}</ul>
        </>
      )}
    </section>
  )
}

/** AddFlag adds a flag at the verified place of the call, or keeps an incident-only note instead. Those
 * who may not approve the category propose it for review; nothing is put on an unverified location. */
export function AddFlag({ d, inc }: { d: Draft; inc?: Incident }) {
  const [open, setOpen] = useState(false)
  const [cats, setCats] = useState<{ code: string; name: string; off?: boolean }[]>([])
  const [f, setF] = useState({ category: 'hazard', title: '', warning: '', instruction: '', source: 'caller', scope: 'place' as 'property' | 'place', incidentOnly: false })
  const verified = inc ? inc.place?.verify === 'verified' || (!inc.place && inc.locVerified) : placeVerify(d) === 'verified'
  const lat = inc?.location?.lat ?? (d.lat ? +d.lat : undefined)
  const lon = inc?.location?.lon ?? (d.lon ? +d.lon : undefined)
  const address = inc?.location?.address ?? d.address
  useEffect(() => { if (open && !cats.length) api.get<{ categories: typeof cats }>('/flags/categories').then((r) => setCats(r.categories.filter((c) => !c.off))).catch(() => {}) }, [open, cats.length])
  const save = async () => {
    try {
      if (f.incidentOnly) {
        if (!inc) return toast.error('Send the call first; an incident-only note goes on the incident')
        await api.post(`/incidents/${inc.number}/notes`, { text: `${f.category.toUpperCase()}: ${f.title}. ${f.warning}. Responders: ${f.instruction} (source: ${f.source}; incident only)`, priority: true })
        toast.success('Kept as a priority note on this incident only')
      } else {
        const p = inc?.place
        const sc = f.scope === 'place'
          ? { business: p?.name ?? d.placeName, complex: p?.complex ?? d.complex, building: p?.building ?? d.building, floor: p?.floor ?? d.floor, unit: p?.unit ?? d.unit }
          : { complex: p?.complex ?? d.complex }
        const r = await api.put<{ id: string; state: string }>('/flags', { category: f.category, title: f.title, warning: f.warning, instruction: f.instruction, source: f.source, incident: inc?.number,
          scope: { address, lat, lon, verified: true, ...sc } })
        toast.success(r.state === 'active' ? `Flag ${r.id} added` : `Flag ${r.id} proposed: a supervisor reviews it`)
      }
      setOpen(false)
      setF({ ...f, title: '', warning: '', instruction: '' })
    } catch (e) {
      toast.error(errText(e))
    }
  }
  if (!open) return <button type="button" className="btn h-7 text-xs" onClick={() => setOpen(true)}><Plus size={12} aria-hidden /> Add a flag or a note for this place</button>
  return (
    <div className="space-y-1.5 rounded border border-slate-700 bg-slate-900 p-2 text-xs">
      <div className="flex items-center gap-1 font-semibold text-slate-200"><FlagIcon size={12} aria-hidden /> Add a flag</div>
      {!verified && <p className="text-amber-200">The location is not verified: a permanent flag cannot go on it. Keep an incident-only note, or verify the address first.</p>}
      <div className="grid grid-cols-2 gap-1.5">
        <label className="label">Category<select className="input" value={f.category} onChange={(e) => setF({ ...f, category: e.target.value })}>{cats.map((c) => <option key={c.code} value={c.code}>{c.name}</option>)}</select></label>
        <label className="label">Source<input className="input" value={f.source} onChange={(e) => setF({ ...f, source: e.target.value })} /></label>
      </div>
      <label className="label">Title<input className="input" value={f.title} onChange={(e) => setF({ ...f, title: e.target.value })} placeholder="Two large dogs loose in the yard" /></label>
      <label className="label">What it is and why it applies<textarea className="input min-h-[40px]" value={f.warning} onChange={(e) => setF({ ...f, warning: e.target.value })} /></label>
      <label className="label">What responders should do<input className="input" value={f.instruction} onChange={(e) => setF({ ...f, instruction: e.target.value })} /></label>
      <fieldset className="flex flex-wrap gap-3">
        <legend className="label">Scope</legend>
        <label className="flex items-center gap-1"><input type="radio" checked={!f.incidentOnly && f.scope === 'place'} onChange={() => setF({ ...f, incidentOnly: false, scope: 'place' })} disabled={!verified} /> This building, floor and unit</label>
        <label className="flex items-center gap-1"><input type="radio" checked={!f.incidentOnly && f.scope === 'property'} onChange={() => setF({ ...f, incidentOnly: false, scope: 'property' })} disabled={!verified} /> The whole property or complex</label>
        <label className="flex items-center gap-1"><input type="radio" checked={f.incidentOnly} onChange={() => setF({ ...f, incidentOnly: true })} /> This incident only (a note)</label>
      </fieldset>
      <div className="flex gap-1.5">
        <button type="button" className="btn btn-go h-7 text-xs" onClick={save} disabled={!f.title || !f.instruction || (!f.incidentOnly && !verified)}>Save</button>
        <button type="button" className="btn h-7 text-xs" onClick={() => setOpen(false)}>Cancel</button>
      </div>
    </div>
  )
}
