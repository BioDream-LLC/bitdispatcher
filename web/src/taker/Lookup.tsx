import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { AlertTriangle, FileClock, Search, ShieldAlert, Siren } from 'lucide-react'
import { api } from '../api'
import { cx, hhmm } from '../util'
import type { Hit, Incident, Premise, PrevMatch, View } from '../types'
import { location, type Draft } from './draft'

export type Alerts = {
  count: number
  critical: string[]
  premises: Premise[]
  bolos: { id: string; text: string; why: string }[]
  safety: string[]
  prev: PrevMatch[]
}

const plateKey = (s?: string) => (s ?? '').toUpperCase().replace(/[^A-Z0-9]/g, '')

function useDebounced<T>(v: T, ms = 400): T {
  const [x, setX] = useState(v)
  useEffect(() => {
    const t = setTimeout(() => setX(v), ms)
    return () => clearTimeout(t)
  }, [v, ms])
  return x
}

/** Everything that may be dangerous about this call: premises hazards at and near the place, officer-safety
 * modifiers and people, BOLOs that match a vehicle, and earlier incidents with a safety note. The critical
 * ones show as one line on every tab. */
export function useAlerts(view: View | null, d: Draft, inc?: Incident): Alerts {
  const loc = inc?.location ?? location(d)
  const lat = useDebounced(loc?.lat)
  const lon = useDebounced(loc?.lon)
  const [premises, setPremises] = useState<Premise[]>([])
  const [prev, setPrev] = useState<PrevMatch[]>([])
  useEffect(() => {
    if (lat == null || lon == null) return setPremises([])
    api.get<Premise[]>(`/premises/near?lat=${lat}&lon=${lon}`).then(setPremises).catch(() => setPremises([]))
  }, [lat, lon])
  const persons = inc?.persons ?? d.persons
  const vehicles = inc?.vehicles ?? d.vehicles
  const prevKey = useDebounced(JSON.stringify([inc?.number, loc?.address, lat, lon, d.callback, persons.map((p) => p.name), vehicles.map((v) => v.plate)]), 600)
  useEffect(() => {
    const q = new URLSearchParams()
    if (inc) q.set('number', inc.number)
    if (loc?.address) q.set('address', loc.address)
    if (lat != null && lon != null) { q.set('lat', String(lat)); q.set('lon', String(lon)) }
    if (d.callback) q.append('phone', d.callback)
    persons.forEach((p) => p.name && q.append('name', p.name))
    vehicles.forEach((v) => { if (v.plate) q.append('plate', v.plate); if (v.vin) q.append('vin', v.vin) })
    if ([...q.keys()].length === 0) return setPrev([])
    api.get<PrevMatch[]>(`/previous?${q}`).then(setPrev).catch(() => setPrev([]))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [prevKey])
  return useMemo(() => {
    const mods = view?.entry?.modifiers ?? []
    const safety: string[] = []
    for (const m of inc?.modifiers ?? d.modifiers) {
      const md = mods.find((x) => x.code === m)
      if (md?.safety) safety.push(md.name)
    }
    for (const p of persons) if (p.safety && !p.removed) safety.push(`Officer safety: ${p.name ?? p.role}`)
    const bolos: Alerts['bolos'] = []
    for (const b of view?.entry?.bolos ?? []) {
      for (const v of vehicles) {
        if (b.plate && v.plate && plateKey(b.plate) === plateKey(v.plate)) bolos.push({ id: b.id, text: b.text, why: `plate ${v.plate}` })
      }
    }
    const prevSafety = prev.filter((p) => p.safety)
    const hazardous = premises.filter((p) => (p.hazards ?? []).length > 0)
    const critical = [
      ...safety,
      ...bolos.map((b) => `${b.id} matches ${b.why}`),
      ...hazardous.slice(0, 2).map((p) => `${p.name || p.address}: ${(p.hazards ?? []).join(', ')}`),
      ...prevSafety.slice(0, 1).map((p) => `${p.incident}: officer-safety note`),
    ]
    return { count: safety.length + bolos.length + hazardous.length + prevSafety.length, critical, premises, bolos, safety, prev }
  }, [view, inc, d.modifiers, persons, vehicles, premises, prev])
}

export function HazardsTab({ alerts }: { alerts: Alerts }) {
  const { premises, bolos, safety, prev } = alerts
  const none = !premises.length && !bolos.length && !safety.length && !prev.some((p) => p.safety)
  return (
    <div className="space-y-3">
      <h2 className="flex items-center gap-1.5 text-sm font-semibold text-slate-100"><ShieldAlert size={14} aria-hidden /> Hazards</h2>
      {none && <p className="text-xs text-slate-500">Nothing known at or near this place, on these people or vehicles. Hazards appear here as the address, people and vehicles are entered.</p>}
      {safety.length > 0 && (
        <section>
          <h3 className="label">Officer safety</h3>
          <ul className="space-y-0.5">{safety.map((s) => <li key={s} className="flex items-center gap-1.5 text-sm font-semibold text-rose-200"><AlertTriangle size={13} aria-hidden /> {s}</li>)}</ul>
        </section>
      )}
      {bolos.length > 0 && (
        <section>
          <h3 className="label">BOLO matches</h3>
          <ul className="space-y-0.5">{bolos.map((b) => <li key={b.id + b.why} className="flex items-center gap-1.5 text-sm text-amber-100"><Siren size={13} aria-hidden /> <b>{b.id}</b> ({b.why}): {b.text}</li>)}</ul>
        </section>
      )}
      {premises.length > 0 && (
        <section>
          <h3 className="label">Premises at and near the place</h3>
          <ul className="space-y-1">
            {premises.map((p) => (
              <li key={p.id} className="panel rounded-md px-2 py-1.5 text-xs">
                <div className="font-semibold text-slate-100">{p.name || p.address} <span className="font-normal text-slate-500">{p.name ? p.address : ''}</span></div>
                {(p.hazards ?? []).length > 0 && <div className="mt-0.5 flex flex-wrap gap-1">{p.hazards!.map((h) => <span key={h} className="chip bg-rose-500/20 text-rose-200">{h}</span>)}</div>}
                {p.notes && <div className="mt-0.5 text-slate-300">{p.notes}</div>}
                {(p.contacts ?? []).length > 0 && <div className="mt-0.5 text-slate-400">Contacts: {p.contacts!.join('; ')}</div>}
              </li>
            ))}
          </ul>
        </section>
      )}
      {prev.some((p) => p.safety) && (
        <section>
          <h3 className="label">Earlier incidents with a safety note</h3>
          <PrevList list={prev.filter((p) => p.safety)} />
        </section>
      )}
    </div>
  )
}

function PrevList({ list }: { list: PrevMatch[] }) {
  return (
    <ul className="divide-y divide-slate-800 rounded-md border border-slate-800">
      {list.map((p) => (
        <li key={p.incident + p.source} className="flex items-start gap-2 px-2 py-1.5 text-xs">
          <span className={cx('chip shrink-0 uppercase', p.confidence === 'confirmed' ? 'bg-emerald-500/20 text-emerald-200' : 'bg-slate-800 text-slate-300')} title={p.confidence === 'confirmed' ? 'An exact identifier: the same address, number, plate or VIN' : 'Near, or a similar name: check it'}>{p.confidence}</span>
          <div className="min-w-0 flex-1">
            <div className="text-slate-100">
              {p.source === 'bitdispatcher' ? <Link className="font-mono font-bold hover:underline" to={`/review/${p.incident}`}>{p.incident}</Link> : <span className="font-mono font-bold">{p.incident}</span>} {p.typeName}
              <span className="ml-1 text-slate-500">{new Date(p.at).toLocaleDateString()} {hhmm(p.at)}{p.disposition ? ` · ${p.disposition}` : p.status ? ` · ${p.status}` : ''}</span>
            </div>
            <div className="text-slate-400">{p.address} · by {p.by.join(', ')}: {p.why.join('; ')}{p.source !== 'bitdispatcher' ? ` · from ${p.source}` : ''}</div>
          </div>
          {p.safety && <ShieldAlert size={13} className="shrink-0 text-rose-300" aria-label="officer-safety note" />}
        </li>
      ))}
    </ul>
  )
}

/** Previous: earlier incidents linked by the address, the phone, the people and the vehicles. */
export function PreviousList({ alerts }: { alerts: Alerts }) {
  const confirmed = alerts.prev.filter((p) => p.confidence === 'confirmed')
  const possible = alerts.prev.filter((p) => p.confidence !== 'confirmed')
  return (
    <div className="space-y-3">
      <h2 className="flex items-center gap-1.5 text-sm font-semibold text-slate-100"><FileClock size={14} aria-hidden /> Previous</h2>
      {!alerts.prev.length && <p className="text-xs text-slate-500">No earlier incident linked yet. They appear as the address, callback, names, plates and VINs are entered.</p>}
      {confirmed.length > 0 && <section><h3 className="label">Confirmed: the same identifier</h3><PrevList list={confirmed} /></section>}
      {possible.length > 0 && <section><h3 className="label">Possible: near, or a similar name — check before relying on it</h3><PrevList list={possible} /></section>}
    </div>
  )
}

/** CAD Search: names, phones, addresses, plates, VINs and incident numbers across the record. */
export function SearchTab() {
  const [q, setQ] = useState('')
  const [hits, setHits] = useState<Hit[] | null>(null)
  const dq = useDebounced(q, 300)
  useEffect(() => {
    if (dq.trim().length < 2) return setHits(null)
    api.get<Hit[]>(`/search?q=${encodeURIComponent(dq)}`).then(setHits).catch(() => setHits([]))
  }, [dq])
  return (
    <div className="space-y-2">
      <label className="label" htmlFor="cad-search">CAD search <span className="normal-case tracking-normal text-slate-500">— a name, phone, address, plate, VIN or incident number (F3)</span></label>
      <div className="relative">
        <Search size={13} className="absolute left-2 top-2 text-slate-500" aria-hidden />
        <input id="cad-search" className="input pl-7" autoFocus value={q} onChange={(e) => setQ(e.target.value)} placeholder="DOE JOHN · 540 555 2348 · KTR4471 · 1519 Rosewalk" />
      </div>
      {hits && !hits.length && <p className="text-xs text-slate-500">Nothing found.</p>}
      {hits && hits.length > 0 && (
        <ul className="divide-y divide-slate-800 rounded-md border border-slate-800">
          {hits.map((h, i) => (
            <li key={i} className="flex items-start gap-2 px-2 py-1.5 text-xs">
              <span className="chip shrink-0 bg-slate-800 uppercase text-slate-300">{h.kind}</span>
              <div className="min-w-0 flex-1">
                <div className="text-slate-100">{h.kind === 'incident' || h.kind === 'person' || h.kind === 'vehicle' ? <Link className="hover:underline" to={`/review/${h.ref}`}>{h.title}</Link> : h.title}</div>
                <div className="truncate text-slate-400">{h.detail}{h.at ? ` · ${new Date(h.at).toLocaleDateString()}` : ''} · {h.source}</div>
              </div>
              <span className={cx('chip shrink-0', h.match === 'exact' ? 'bg-emerald-500/20 text-emerald-200' : 'bg-slate-800 text-slate-400')} title="How well it matches">{h.match}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
