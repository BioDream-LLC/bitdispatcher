import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { AlertTriangle, Car, CheckCircle2, Crosshair, FileSearch, Radio, ShieldAlert, Siren, Users } from 'lucide-react'
import { api, errText } from '../api'
import { can, useStore } from '../store'
import { cx, hhmm, locText } from '../util'
import type { Incident, View } from '../types'

const plateKey = (s?: string) => (s ?? '').toUpperCase().replace(/[^A-Z0-9]/g, '')

/** What call entry recorded, for the dispatcher: modifiers, a changed location, the caller elsewhere, the
 * people and vehicles, BOLO matches, the talkgroup and the notifications still to make. */
export default function EntryFacts({ inc, view }: { inc: Incident; view: View }) {
  const me = useStore((s) => s.me)
  const mods = (inc.modifiers ?? []).map((m) => view.entry?.modifiers.find((x) => x.code === m) ?? { code: m, name: m })
  const persons = (inc.persons ?? []).filter((p) => !p.removed)
  const vehicles = (inc.vehicles ?? []).filter((v) => !v.removed)
  const notices = inc.notices ?? []
  const pending = notices.filter((n) => n.status === 'pending')
  const bolo = (view.entry?.bolos ?? []).filter((b) => b.plate && vehicles.some((v) => plateKey(v.plate) === plateKey(b.plate)))
  const changedRecently = inc.changed && Date.now() - Date.parse(inc.changed.at) < 30 * 60_000
  const tgs = view.settings.talkgroups ?? []
  const decide = (accept: boolean) => api.post(`/incidents/${inc.number}/location`, { accept, why: accept ? 'newer caller location (dispatcher)' : 'kept (dispatcher)' }).catch((e) => toast.error(errText(e)))
  const done = (id: string) => api.put(`/incidents/${inc.number}/contacts/${id}`, { status: 'done' }).catch((e) => toast.error(errText(e)))
  const setTG = (tg: string) => api.put(`/incidents/${inc.number}/talkgroup`, { talkgroup: tg }).catch((e) => toast.error(errText(e)))
  const any = mods.length || changedRecently || inc.newLocation || inc.callerAt || persons.length || vehicles.length || pending.length || bolo.length || tgs.length
  return (
    <>
      {any ? (
        <section aria-label="Call entry" className="space-y-1 text-xs">
          {mods.length > 0 && (
            <div className="flex flex-wrap gap-1">
              {mods.map((m) => <span key={m.code} className={cx('chip', 'safety' in m && m.safety ? 'bg-rose-500 text-white' : 'bg-sky-500/20 text-sky-200')}>{'safety' in m && m.safety && <ShieldAlert size={10} aria-hidden />}{m.name}</span>)}
            </div>
          )}
          {inc.newLocation && (
            <div className="flex items-center gap-1.5 rounded border border-amber-700 bg-amber-950/40 px-1.5 py-1 text-amber-100" role="alert">
              <Crosshair size={12} aria-hidden /> Newer caller location ({inc.newLocation.location.source}) {Math.round(inc.newLocation.metres)} m away
              {can.dispatch(me) && <><button className="btn btn-go ml-auto h-5 px-1.5 text-[10px]" onClick={() => decide(true)}>Use it</button><button className="btn h-5 px-1.5 text-[10px]" onClick={() => decide(false)}>Keep</button></>}
            </div>
          )}
          {changedRecently && (
            <div className="flex items-center gap-1.5 rounded border border-amber-600/70 bg-amber-500/10 px-1.5 py-1 font-semibold text-amber-100" role="status">
              <AlertTriangle size={12} aria-hidden /> Location changed {hhmm(inc.changed!.at)} by {inc.changed!.by}: was {locText(inc.changed!.from)}
            </div>
          )}
          {inc.callerAt && <div className="text-slate-400">Caller is at <span className="text-slate-200">{locText(inc.callerAt)}</span></div>}
          {bolo.map((b) => <div key={b.id} className="flex items-center gap-1 font-semibold text-rose-200"><Siren size={12} aria-hidden /> {b.id} matches a vehicle: {b.text}</div>)}
          {persons.length > 0 && (
            <div className="flex flex-wrap items-center gap-1">
              <Users size={11} className="text-slate-500" aria-hidden />
              {persons.map((p) => <span key={p.id} className={cx('chip', p.safety ? 'bg-rose-500/25 text-rose-100' : 'bg-slate-800 text-slate-200')} title={[p.dob, p.clothing, p.notes].filter(Boolean).join(' · ')}>{p.role}: {p.name || p.clothing || '—'}</span>)}
            </div>
          )}
          {vehicles.length > 0 && (
            <div className="flex flex-wrap items-center gap-1">
              <Car size={11} className="text-slate-500" aria-hidden />
              {vehicles.map((v) => <span key={v.id} className="chip bg-slate-800 text-slate-200" title={[v.vin, v.description].filter(Boolean).join(' · ')}>{v.role}: {[v.color, v.make, v.model].filter(Boolean).join(' ')} <span className="font-mono text-amber-200">{v.state} {v.plate}</span>{v.direction ? ` → ${v.direction}` : ''}</span>)}
            </div>
          )}
          {pending.length > 0 && (
            <ul className="space-y-0.5">
              {pending.map((n) => (
                <li key={n.id} className="flex items-center gap-1.5 text-amber-100">
                  <span className="chip bg-amber-500/20 text-amber-200">{n.kind === 'notify' ? 'notify' : n.kind}</span> {n.who} <span className="truncate text-slate-500">{n.detail}</span>
                  {can.dispatch(me) && <button className="btn btn-ghost ml-auto h-5 px-1 text-[10px]" onClick={() => done(n.id)}><CheckCircle2 size={11} aria-hidden /> done</button>}
                </li>
              ))}
            </ul>
          )}
          {tgs.length > 0 && (
            <label className="flex items-center gap-1.5 text-slate-400">
              <Radio size={11} aria-hidden /> Talkgroup
              <select className="input h-6 w-auto py-0 text-xs" value={inc.talkgroup ?? ''} disabled={!can.dispatch(me)} onChange={(e) => setTG(e.target.value)} aria-label="Talkgroup">
                <option value="">—</option>
                {tgs.map((t) => <option key={t} value={t}>{t}</option>)}
              </select>
            </label>
          )}
        </section>
      ) : null}
      <Link to={`/review/${inc.number}`} className="inline-flex w-fit items-center gap-1 text-[11px] text-slate-400 hover:text-slate-200"><FileSearch size={11} aria-hidden /> Review views: as first entered, summary, detailed, audit</Link>
    </>
  )
}

/** NoAck flags a unit that has not acknowledged its assignment in time, with a button to record that it did. */
export function AckCell({ inc, unit, status, dispatched, acked, ackSeconds, now }: { inc: string; unit: string; status: string; dispatched: string; acked?: string; ackSeconds: number; now: number }) {
  const me = useStore((s) => s.me)
  if (acked) return <span className="text-[10px] text-emerald-300" title={`Acknowledged ${hhmm(acked)}`}>✓</span>
  if (status !== 'dispatched') return null
  const late = now - Date.parse(dispatched) > ackSeconds * 1000
  return (
    <button className={cx('chip h-5 cursor-pointer px-1 text-[10px]', late ? 'flash bg-amber-500 text-black' : 'bg-slate-700 text-slate-300')} disabled={!can.dispatch(me)}
      onClick={() => api.post(`/units/${unit}/ack`).catch((e) => toast.error(errText(e)))} title={`${unit} has not acknowledged ${inc}: press when it does (ACK ${unit})`}>
      {late ? 'NO ACK' : 'ack?'}
    </button>
  )
}
