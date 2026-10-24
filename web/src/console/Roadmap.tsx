// The roadmap features on the console: alarm upgrades, the fire-ground accountability clock, paging and
// who answered, tows, the call-out list, area cautions and warrants at the address, patient-care reports,
// a unit's crew and status history, the contacts directory and the schedule.
import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import {
  AlertTriangle, BellRing, BookUser, CalendarClock, ClipboardCheck, Copy, Download, FileHeart, Flame, Gavel, HardHat, History as HistoryIcon,
  Hexagon, Phone as PhoneIcon, Plus, Send, Siren, Trash2, Truck, Users,
} from 'lucide-react'
import { api, errText } from '../api'
import { can, useStore } from '../store'
import { Field, Modal } from '../ui'
import { ago, clock, cx, hhmm, statusMeta, typeName, useNow } from '../util'
import type { CallOut, CrewSeat, DirEntry, Incident, Paper, Scheduled, UnitHistory, View } from '../types'

const ms = (s?: string) => (s ? Date.parse(s) : 0)

// ---- 1. alarm levels ----

/** The incident's alarm and the button that upgrades it to the next. */
export function AlarmBar({ inc, view, reload }: { inc: Incident; view: View; reload: () => void }) {
  const me = useStore((s) => s.me)
  const a = view.roadmap?.alarms?.[inc.number]
  if (!a) return null
  const up = async () => {
    try {
      await api.post(`/incidents/${inc.number}/alarm`, {})
      toast.success(`${inc.number}: ${a.nextName}. The recommendation now has the extra units.`)
      reload()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const down = async () => {
    const why = prompt('Why is the alarm brought down?')
    if (!why) return
    try {
      await api.post(`/incidents/${inc.number}/alarm`, { level: a.level - 1, reason: why })
      reload()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section aria-label="Alarm level" className={cx('flex flex-wrap items-center gap-2 rounded-md border px-2 py-1 text-xs', a.level > 1 ? 'border-orange-500/60 bg-orange-500/10' : 'border-slate-800')}>
      <Flame size={13} className={a.level > 1 ? 'text-orange-300' : 'text-slate-500'} aria-hidden />
      <span className={cx('font-bold uppercase tracking-wide', a.level > 1 ? 'text-orange-200' : 'text-slate-400')}>{a.name}</span>
      {can.dispatch(me) && inc.status !== 'closed' && (
        <span className="ml-auto flex gap-1">
          {a.level > 1 && <button className="btn h-6 px-1.5 py-0 text-[11px]" onClick={down}>Bring down</button>}
          {a.nextName && (
            <button className="btn h-6 border-orange-500/60 px-2 py-0 text-[11px] text-orange-200 hover:bg-orange-500/20" onClick={up} title={`Adds ${(a.nextAdds ?? []).map((r) => `${r.count} × ${r.capability}`).join(', ')}`}>
              Upgrade to {a.nextName} <span className="kbd h-4">ALARM</span>
            </button>
          )}
        </span>
      )}
      {a.nextName && <span className="w-full text-[10px] text-slate-500">The {a.nextName} adds {(a.nextAdds ?? []).map((r) => `${r.count} × ${r.capability}`).join(', ')}</span>}
    </section>
  )
}

// ---- 4. fire-ground accountability ----

/** The roll-call clock: when the next PAR is due, who is on scene, who did not answer, the benchmarks. */
export function FiregroundBox({ inc, view }: { inc: Incident; view: View }) {
  const now = useNow()
  const me = useStore((s) => s.me)
  const [missing, setMissing] = useState<string[]>([])
  const f = inc.fireground
  const ct = view.callTypes.find((c) => c.code === inc.type)
  const scene = inc.units.filter((a) => !a.cleared && a.onScene).map((a) => a.unit)
  const closed = inc.status === 'closed'
  const isFire = ct?.discipline === 'fire' || inc.units.some((a) => view.units.find((u) => u.id === a.unit && view.settings.unitTypes.find((t) => t.code === u.type)?.discipline === 'fire'))
  if (!f && (!isFire || closed || scene.length === 0)) return null
  const post = async (path: string, body: object, ok?: string) => {
    try {
      await api.post(`/incidents/${inc.number}/${path}`, body)
      if (ok) toast.success(ok)
      setMissing([])
    } catch (e) {
      toast.error(errText(e))
    }
  }
  if (!f || f.ended) {
    if (closed || !can.dispatch(me)) return null
    return (
      <section aria-label="Accountability" className="flex items-center gap-2 rounded-md border border-slate-800 px-2 py-1 text-xs">
        <ClipboardCheck size={13} className="text-slate-500" aria-hidden />
        <span className="text-slate-400">{f?.ended ? 'Accountability clock stopped' : 'No accountability clock'}</span>
        <button className="btn ml-auto h-6 px-2 py-0 text-[11px]" onClick={() => post('par/start', { minutes: 0 }, 'Accountability clock started')}>Start roll calls</button>
      </section>
    )
  }
  const due = ms(view.roadmap?.parDue?.[inc.number])
  const over = due > 0 && now > due
  const missingNow = f.missing ?? []
  return (
    <section aria-label="Accountability" className={cx('rounded-md border', missingNow.length ? 'flash border-rose-500 bg-rose-950/40' : over ? 'border-amber-500 bg-amber-500/10' : 'border-rose-900/50 bg-rose-950/10')}>
      <div className="flex items-center gap-2 border-b border-slate-800 px-2 py-1 text-[11px]">
        <ClipboardCheck size={13} className="text-rose-200" aria-hidden />
        <span className="font-bold uppercase tracking-wider text-rose-200">Accountability</span>
        <span className="font-mono text-slate-300" title="Since the clock started">{clock(f.started, now)} elapsed</span>
        <span className={cx('ml-auto font-mono font-bold', over ? 'text-amber-200' : 'text-slate-200')} role={over ? 'alert' : undefined}>
          {over ? `PAR DUE ${clock(due, now)} ago` : `PAR in ${clock(now, due)}`}
        </span>
      </div>
      {missingNow.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 border-b border-rose-800 px-2 py-1 text-xs font-bold text-white" role="alert">
          <Siren size={14} aria-hidden /> NOT ACCOUNTED FOR: {missingNow.join(', ')}
          {can.dispatch(me) && missingNow.map((u) => (
            <button key={u} className="btn h-6 border-white/50 bg-white/10 px-2 py-0 text-[11px] text-white" onClick={() => post('par/found', { unit: u }, `${u} accounted for`)}>{u} found</button>
          ))}
        </div>
      )}
      <div className="space-y-1 px-2 py-1 text-xs">
        <div className="flex flex-wrap items-center gap-1">
          <span className="text-[10px] uppercase text-slate-500">On scene</span>
          {scene.length === 0 && <span className="text-slate-500">no one yet</span>}
          {scene.map((u) => {
            const off = missing.includes(u)
            return (
              <button key={u} disabled={!can.dispatch(me)} aria-pressed={off} title={off ? 'Will be reported missing' : 'Click if this crew did not answer'}
                className={cx('chip cursor-pointer font-mono', off ? 'bg-rose-500/40 text-white line-through' : 'bg-emerald-500/15 text-emerald-200')}
                onClick={() => setMissing(off ? missing.filter((x) => x !== u) : [...missing, u])}>{u}</button>
            )
          })}
          {can.dispatch(me) && scene.length > 0 && (
            <span className="ml-auto flex gap-1">
              {missing.length > 0 ? (
                <button className="btn h-6 border-rose-500 px-2 py-0 text-[11px] text-rose-200" onClick={() => post('par', { missing }, `PAR: ${missing.join(', ')} not accounted for — tell command`)}>Report {missing.join(', ')} missing</button>
              ) : (
                <button className="btn btn-go h-6 px-2 py-0 text-[11px]" onClick={() => post('par', {}, 'PAR recorded: all accounted for')}>All accounted for <span className="kbd h-4 border-white/40 bg-transparent text-white">PAR</span></button>
              )}
            </span>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-1">
          <span className="text-[10px] uppercase text-slate-500">Benchmarks</span>
          {(view.roadmap?.benchmarks ?? []).map((b) => {
            const done = f.benchmarks.find((m) => m.code === b.code)
            return (
              <button key={b.code} disabled={!!done || !can.dispatch(me)} onClick={() => post('benchmark', { code: b.code }, `${b.name} recorded`)}
                className={cx('chip', done ? 'bg-sky-500/20 text-sky-200' : 'cursor-pointer bg-slate-800 text-slate-400 hover:bg-slate-700')}
                title={done ? `${b.name} at ${hhmm(done.at)} (${done.by})` : `Record: ${b.name} (BM ${b.code})`}>
                {done ? '✓ ' : ''}{b.name}{done ? ` ${hhmm(done.at).slice(0, 5)}` : ''}
              </button>
            )
          })}
        </div>
        {f.lastPar && <div className="text-[10px] text-slate-500">Last full roll call {hhmm(f.lastPar)} · {f.checks.length} roll call{f.checks.length === 1 ? '' : 's'} · every {f.minutes} min</div>}
      </div>
    </section>
  )
}

// ---- 3. paging ----

/** Who was paged for the incident and how they answered, and the button to page more. */
export function PagingBox({ inc, view }: { inc: Incident; view: View }) {
  const me = useStore((s) => s.me)
  const now = useNow()
  const [open, setOpen] = useState(false)
  const rb = view.roadmap?.responders?.[inc.number]
  const groups = view.roadmap?.pageGroups ?? []
  if (!rb && (groups.length === 0 || inc.status === 'closed')) return null
  return (
    <section aria-label="Responders paged" className="rounded-md border border-slate-800">
      <div className="flex items-center gap-2 px-2 py-1 text-[11px]">
        <BellRing size={12} className="text-sky-300" aria-hidden />
        <span className="font-bold uppercase tracking-wider text-sky-300">Responders</span>
        {rb ? (
          <span className="text-slate-300">
            {rb.paged} paged · <b className="text-emerald-300">{rb.responding + rb.toScene} responding</b>{rb.toScene ? ` (${rb.toScene} to scene)` : ''}{rb.unavailable ? ` · ${rb.unavailable} not available` : ''}
          </span>
        ) : <span className="text-slate-500">none paged</span>}
        {can.dispatch(me) && inc.status !== 'closed' && groups.length > 0 && <button className="btn ml-auto h-6 px-2 py-0 text-[11px]" onClick={() => setOpen(true)}><BellRing size={11} aria-hidden /> Page</button>}
      </div>
      {rb && rb.replies.length > 0 && (
        <ul className="border-t border-slate-800 px-2 py-0.5 text-[11px]">
          {rb.replies.map((r) => (
            <li key={r.name + r.at} className="flex gap-2">
              <span className={cx('w-3', r.answer === 'unavailable' ? 'text-slate-500' : 'text-emerald-300')}>{r.answer === 'unavailable' ? '✕' : '●'}</span>
              <span className={r.answer === 'unavailable' ? 'text-slate-500' : 'text-slate-100'}>{r.name}</span>
              <span className="text-slate-400">{r.answer === 'responding' ? 'to the station' : r.answer === 'scene' ? 'to the scene' : 'not available'}</span>
              {r.answer !== 'unavailable' && r.eta ? (
                <span className="ml-auto font-mono text-slate-300">{Math.max(0, Math.round((ms(r.at) + r.eta * 60000 - now) / 60000))} min</span>
              ) : <span className="ml-auto font-mono text-slate-500">{ago(r.at, now)} ago</span>}
            </li>
          ))}
        </ul>
      )}
      {open && <PageDialog inc={inc} view={view} onClose={() => setOpen(false)} />}
    </section>
  )
}

function PageDialog({ inc, view, onClose }: { inc: Incident; view: View; onClose: () => void }) {
  const groups = view.roadmap?.pageGroups ?? []
  const [pick, setPick] = useState<string[]>([])
  const [text, setText] = useState(view.summaries?.[inc.number] ?? '')
  const send = async () => {
    try {
      const r = await api.post<{ id: string; people: number }>('/pages', { incident: inc.number, groups: pick, text })
      toast.success(`${r.id}: ${r.people} people paged`)
      onClose()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Modal open onClose={onClose} title={`Page responders for ${inc.number}`} icon={<BellRing size={16} className="text-sky-300" aria-hidden />}
      footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn btn-go" disabled={!pick.length} onClick={send}><Send size={13} aria-hidden /> Page</button></>}>
      <div className="space-y-3">
        <div>
          <span className="label">Groups</span>
          <div className="flex flex-wrap gap-1">
            {groups.map((g) => (
              <button key={g.id} aria-pressed={pick.includes(g.id)} onClick={() => setPick(pick.includes(g.id) ? pick.filter((x) => x !== g.id) : [...pick, g.id])}
                className={cx('chip cursor-pointer', pick.includes(g.id) ? 'bg-sky-500/30 text-sky-100' : 'bg-slate-800 text-slate-300')}>{g.name} · {g.members.length}</button>
            ))}
          </div>
        </div>
        <Field label="Text" htmlFor="pgtext" hint="The subject is the call type and address; each person gets a link to answer"><textarea id="pgtext" rows={4} className="input" value={text} maxLength={1200} onChange={(e) => setText(e.target.value)} /></Field>
      </div>
    </Modal>
  )
}

// ---- 5. tows ----

/** Calling a tow from the rotation, and the tows asked for on the incident. */
export function TowBox({ inc, view }: { inc: Incident; view: View }) {
  const me = useStore((s) => s.me)
  const companies = view.roadmap?.towCompanies ?? []
  const tows = (view.roadmap?.tows ?? []).filter((t) => t.incident === inc.number)
  const classes = view.roadmap?.towClasses ?? ['light']
  const zones = view.settings.towZones ?? []
  const [cls, setCls] = useState(classes[0])
  const [zone, setZone] = useState(zones[0] ?? '')
  const [vehicle, setVehicle] = useState('')
  const disc = view.callTypes.find((c) => c.code === inc.type)?.discipline
  const wants = tows.length > 0 || disc === 'police' || ['MVA', 'MVAPIN', 'TOW', 'VF'].includes(inc.type)
  if (companies.length === 0 || !wants || (inc.status === 'closed' && tows.length === 0)) return null
  const request = async () => {
    try {
      const t = await api.post<{ id: string; name: string; phone: string }>('/tows', { incident: inc.number, class: cls, zone, kind: 'rotation', vehicle })
      toast.success(`Call ${t.name} ${t.phone}`, { duration: 15000 })
      setVehicle('')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const answer = async (id: string, result: string) => {
    try {
      await api.post(`/tows/${id}/answer`, { result })
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section aria-label="Tows" className="rounded-md border border-slate-800">
      <div className="flex flex-wrap items-center gap-1.5 px-2 py-1 text-[11px]">
        <Truck size={12} className="text-amber-300" aria-hidden />
        <span className="font-bold uppercase tracking-wider text-amber-300">Tow</span>
        {can.dispatch(me) && inc.status !== 'closed' && (
          <>
            <select aria-label="Tow class" className="input h-6 w-auto py-0 text-[11px]" value={cls} onChange={(e) => setCls(e.target.value)}>{classes.map((c) => <option key={c}>{c}</option>)}</select>
            {zones.length > 0 && <select aria-label="Tow zone" className="input h-6 w-auto py-0 text-[11px]" value={zone} onChange={(e) => setZone(e.target.value)}>{zones.map((z) => <option key={z}>{z}</option>)}</select>}
            <input aria-label="Vehicle" className="input h-6 w-36 py-0 text-[11px]" placeholder="vehicle, plate" value={vehicle} onChange={(e) => setVehicle(e.target.value)} />
            <button className="btn ml-auto h-6 px-2 py-0 text-[11px]" onClick={request}>Next on rotation <span className="kbd h-4">TOW</span></button>
          </>
        )}
      </div>
      {tows.length > 0 && (
        <ul className="divide-y divide-slate-800 border-t border-slate-800 text-[11px]">
          {tows.map((t) => (
            <li key={t.id} className="flex flex-wrap items-center gap-1.5 px-2 py-1">
              <span className="font-mono text-slate-500">{t.id}</span>
              <b className="text-slate-100">{t.name}</b>
              <a className="font-mono text-sky-300 underline" href={`tel:${t.phone}`}>{t.phone}</a>
              <span className="text-slate-500">{t.class}{t.zone ? ` · ${t.zone}` : ''}{t.kind !== 'rotation' ? ` · ${t.kind === 'owner' ? "owner's choice" : "officer's choice"}` : ''}</span>
              <span className={cx('chip', t.result === 'accepted' || t.result === 'arrived' ? 'bg-emerald-500/20 text-emerald-200' : t.result === 'called' ? 'bg-amber-500/20 text-amber-200' : 'bg-slate-700 text-slate-300')}>{t.result}</span>
              {can.dispatch(me) && t.result === 'called' && (
                <span className="ml-auto flex gap-1">
                  <button className="btn h-5 px-1.5 py-0 text-[10px]" onClick={() => answer(t.id, 'accepted')}>Accepted</button>
                  <button className="btn h-5 px-1.5 py-0 text-[10px]" onClick={() => answer(t.id, 'declined')}>Declined</button>
                  <button className="btn h-5 px-1.5 py-0 text-[10px]" onClick={() => answer(t.id, 'no-answer')}>No answer</button>
                </span>
              )}
              {can.dispatch(me) && t.result === 'accepted' && (
                <span className="ml-auto flex gap-1">
                  <button className="btn h-5 px-1.5 py-0 text-[10px]" onClick={() => answer(t.id, 'arrived')}>Arrived</button>
                  <button className="btn h-5 px-1.5 py-0 text-[10px]" onClick={() => answer(t.id, 'cancelled')}>Cancel</button>
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

// ---- 7. the call-out list ----

/** Whom to call for this incident, with the numbers, and logging each call. */
export function CallOutBox({ inc, view }: { inc: Incident; view: View }) {
  const me = useStore((s) => s.me)
  const list = view.roadmap?.callOut?.[inc.number]
  if (!list?.length || inc.status === 'closed') return null
  const log = async (c: CallOut) => {
    try {
      await api.post(`/incidents/${inc.number}/contacts`, { kind: 'notify', who: c.entry?.name ?? c.list, detail: c.entry?.phones[0]?.number ?? '', status: 'done' })
      toast.success(`Logged: called ${c.entry?.name}`)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section aria-label="Call-out list" className="rounded-md border border-slate-800">
      <div className="flex items-center gap-1.5 px-2 py-1 text-[11px]"><BookUser size={12} className="text-violet-300" aria-hidden /><span className="font-bold uppercase tracking-wider text-violet-300">To call</span></div>
      <ul className="divide-y divide-slate-800 border-t border-slate-800 text-[11px]">
        {list.map((c, i) => (
          <li key={i} className="flex flex-wrap items-center gap-1.5 px-2 py-1">
            {c.list && <span className="chip bg-violet-500/15 text-violet-200">{c.list}{c.backup ? ' · backup' : ''}</span>}
            {c.entry ? (
              <>
                <b className="text-slate-100">{c.entry.name}</b>
                {c.entry.org && <span className="text-slate-500">{c.entry.org}</span>}
                {c.entry.phones.map((p) => <a key={p.number} className="font-mono text-sky-300 underline" href={`tel:${p.number}`} title={p.label}>{p.number}{p.label ? ` ${p.label}` : ''}</a>)}
                {c.until && <span className="text-slate-500">on call until {new Date(c.until).toLocaleString([], { weekday: 'short', hour: '2-digit', minute: '2-digit' })}</span>}
                {can.dispatch(me) && <button className="btn ml-auto h-5 px-1.5 py-0 text-[10px]" onClick={() => log(c)}><PhoneIcon size={10} aria-hidden /> Called</button>}
              </>
            ) : <span className="text-amber-300">No one on call and no backup</span>}
          </li>
        ))}
      </ul>
    </section>
  )
}

// ---- 8 and 11: area cautions and warrants at the address ----

/** Caution areas the incident is in, and active warrants or papers at the address (details for police). */
export function SafetyFlags({ inc, view }: { inc: Incident; view: View }) {
  const me = useStore((s) => s.me)
  const cautions = view.roadmap?.cautions?.[inc.number] ?? []
  const pf = view.roadmap?.papers?.[inc.number]
  const [papers, setPapers] = useState<Paper[] | null>(null)
  if (!cautions.length && !pf) return null
  const show = async () => {
    try {
      setPapers((await api.get<{ items: Paper[] }>(`/incidents/${inc.number}/papers`)).items)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <div className="space-y-1">
      {cautions.map((c) => (
        <div key={c.id} role="note" className="flex items-start gap-1.5 rounded border border-yellow-600/60 bg-yellow-500/10 px-2 py-1 text-xs text-yellow-100">
          <Hexagon size={13} className="mt-0.5 shrink-0" aria-hidden /> <span><b>{c.name}:</b> {c.note}</span>
        </div>
      ))}
      {pf && (
        <div role="alert" className="flex flex-wrap items-center gap-1.5 rounded border border-rose-600/70 bg-rose-500/10 px-2 py-1 text-xs text-rose-100">
          <Gavel size={13} aria-hidden />
          <b>{pf.count} active {pf.count === 1 ? 'paper' : 'papers'} at this address</b>
          <span className="text-rose-200/80">({pf.kinds.join(', ')})</span>
          {pf.caution && <span className="chip bg-rose-600 text-white">CAUTION: {pf.caution}</span>}
          {can.dispatch(me) && <button className="btn ml-auto h-5 px-1.5 py-0 text-[10px]" onClick={show}>Details</button>}
        </div>
      )}
      {papers && (
        <Modal open onClose={() => setPapers(null)} title={`Warrants and papers at ${inc.number}`} icon={<Gavel size={16} className="text-rose-300" aria-hidden />} footer={<button className="btn" onClick={() => setPapers(null)}>Close</button>}>
          <ul className="space-y-2 text-sm">
            {papers.map((p) => (
              <li key={p.id} className="rounded border border-slate-800 p-2">
                <div className="flex flex-wrap items-center gap-2"><b className="text-slate-100">{p.subject.name}</b>{p.subject.dob && <span className="font-mono text-slate-400">DOB {p.subject.dob}</span>}<span className="chip bg-slate-700 text-slate-200">{p.kind}</span>{p.caution && <span className="chip bg-rose-600 text-white">{p.caution}</span>}</div>
                <div className="text-xs text-slate-400">{p.number} {p.court} · {p.charge} · {p.location?.address}</div>
                {p.attempts.length > 0 && <div className="text-xs text-slate-500">{p.attempts.length} attempt{p.attempts.length === 1 ? '' : 's'}, last {hhmm(p.attempts[p.attempts.length - 1].at)}: {p.attempts[p.attempts.length - 1].result}</div>}
              </li>
            ))}
            {papers.length === 0 && <li className="text-slate-500">None (or served since).</li>}
          </ul>
          <p className="mt-2 text-xs text-slate-500">Confirm every warrant with the issuing agency before acting on it. Looking at these is recorded.</p>
        </Modal>
      )}
    </div>
  )
}

// ---- 10. patient-care reports ----

/** Each EMS unit's response to the patient-care report system: download it, send it, and what it said. */
export function EPCRBox({ inc, view }: { inc: Incident; view: View }) {
  const me = useStore((s) => s.me)
  const ems = [...new Set(inc.units.map((a) => a.unit))].filter((u) => {
    const unit = view.units.find((x) => x.id === u)
    return unit && view.settings.unitTypes.find((t) => t.code === unit.type)?.discipline === 'ems'
  })
  if (!ems.length || !can.dispatch(me)) return null
  const send = async (u: string) => {
    try {
      const r = await api.post<{ ok: boolean; error?: string }>(`/incidents/${inc.number}/nemsis`, { unit: u })
      r.ok ? toast.success(`${u} sent to the patient-care report system`) : toast.error(r.error ?? 'not taken')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section aria-label="Patient-care reports" className="flex flex-wrap items-center gap-1.5 rounded-md border border-slate-800 px-2 py-1 text-[11px]">
      <FileHeart size={12} className="text-pink-300" aria-hidden />
      <span className="font-bold uppercase tracking-wider text-pink-300">Patient care</span>
      {ems.map((u) => {
        const r = inc.epcr?.[u]
        return (
          <span key={u} className="flex items-center gap-1">
            <span className="font-mono text-slate-200">{u}</span>
            {r && <span className={cx('chip', r.ok ? 'bg-emerald-500/20 text-emerald-200' : 'bg-rose-500/20 text-rose-200')} title={r.error}>{r.ok ? `sent ${hhmm(r.at).slice(0, 5)}` : 'not taken'}</span>}
            <a className="btn h-5 px-1.5 py-0 text-[10px]" href={`./api/incidents/${inc.number}/nemsis?unit=${u}`} download title="The NEMSIS 3.5 record of this unit's response"><Download size={10} aria-hidden /> NEMSIS</a>
            {view.roadmap?.epcr && <button className="btn h-5 px-1.5 py-0 text-[10px]" onClick={() => send(u)}>{r ? 'Send again' : 'Send'}</button>}
          </span>
        )
      })}
    </section>
  )
}

// ---- 2. a unit's crew ----

/** A unit's crew and staffing, with the editor. */
export function CrewLine({ view, unitId }: { view: View; unitId: string }) {
  const me = useStore((s) => s.me)
  const u = view.units.find((x) => x.id === unitId)
  const [edit, setEdit] = useState(false)
  if (!u) return null
  const short = view.roadmap?.staffing?.[u.id]
  const members = view.roadmap?.members ?? []
  return (
    <>
      <span className="text-slate-500">Crew</span>
      <span className="text-slate-200">
        {u.crew?.length ? u.crew.map((c) => `${c.name}${c.role ? ` (${c.role})` : ''}`).join(', ') : <span className="text-slate-500">not recorded</span>}
        {short && <span className="chip ml-1 bg-amber-500/20 text-amber-200">short: {short.join(', ')}</span>}
        {can.dispatch(me) && members.length > 0 && <button className="ml-1 underline" onClick={() => setEdit(true)} aria-label={`Change the crew of ${u.id}`}>change</button>}
      </span>
      {edit && <CrewDialog view={view} unitId={u.id} onClose={() => setEdit(false)} />}
    </>
  )
}

export function CrewDialog({ view, unitId, onClose, save }: { view: View; unitId: string; onClose: () => void; save?: (crew: { member: string; role?: string }[]) => Promise<void> }) {
  const u = view.units.find((x) => x.id === unitId)
  const [crew, setCrew] = useState<{ member: string; role?: string }[]>((u?.crew ?? []).map((c: CrewSeat) => ({ member: c.member, role: c.role })))
  const [q, setQ] = useState('')
  const members = (view.roadmap?.members ?? []).filter((m) => !m.inactive)
  const shown = members.filter((m) => !q || m.name.toLowerCase().includes(q.toLowerCase()) || m.id.includes(q))
  const doSave = async () => {
    try {
      if (save) await save(crew)
      else await api.put(`/units/${unitId}/crew`, { crew })
      toast.success(`${unitId} crew saved`)
      onClose()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Modal open onClose={onClose} title={`Crew of ${unitId}`} icon={<HardHat size={16} className="text-amber-300" aria-hidden />}
      footer={<><button className="btn mr-auto" onClick={() => setCrew([])}>Clear</button><button className="btn" onClick={onClose}>Cancel</button><button className="btn btn-go" onClick={doSave}>Save</button></>}>
      <div className="space-y-2">
        <ul className="space-y-1">
          {crew.map((c, i) => {
            const m = members.find((x) => x.id === c.member)
            return (
              <li key={c.member} className="flex items-center gap-2 text-sm">
                <span className="w-44 truncate text-slate-100">{m?.name ?? c.member}</span>
                <span className="w-40 truncate text-xs text-slate-500">{(m?.quals ?? []).join(', ')}</span>
                <input aria-label={`${m?.name} role`} className="input h-7 w-32 py-0 text-xs" placeholder="role" value={c.role ?? ''} onChange={(e) => setCrew(crew.map((x, j) => (j === i ? { ...x, role: e.target.value } : x)))} />
                <button className="btn btn-ghost h-7 px-1.5" aria-label={`Take ${m?.name} off`} onClick={() => setCrew(crew.filter((_, j) => j !== i))}><Trash2 size={12} aria-hidden /></button>
              </li>
            )
          })}
          {crew.length === 0 && <li className="text-xs text-slate-500">No one aboard: staffing is not recorded for {unitId}.</li>}
        </ul>
        <Field label="Add from the roster" htmlFor="crq"><input id="crq" className="input" placeholder="name or number" value={q} onChange={(e) => setQ(e.target.value)} /></Field>
        <ul className="max-h-56 overflow-y-auto rounded border border-slate-800 text-sm">
          {shown.filter((m) => !crew.some((c) => c.member === m.id)).slice(0, 60).map((m) => (
            <li key={m.id}>
              <button className="flex w-full items-center gap-2 px-2 py-1 text-left hover:bg-slate-800" onClick={() => setCrew([...crew, { member: m.id }])}>
                <Plus size={12} aria-hidden /> <span className="text-slate-100">{m.name}</span> <span className="text-xs text-slate-500">{(m.quals ?? []).join(', ')}</span>
                {m.riding && m.riding !== unitId && <span className="chip ml-auto bg-slate-700 text-slate-300">on {m.riding}: moves</span>}
              </button>
            </li>
          ))}
        </ul>
      </div>
    </Modal>
  )
}

// ---- 9. unit status history ----

export function HistoryButton({ unit }: { unit: string }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button className="btn h-6 px-1.5 py-0 text-[10px]" onClick={() => setOpen(true)} title="This unit's statuses over a day"><HistoryIcon size={10} aria-hidden /> History</button>
      {open && <HistoryDialog unit={unit} onClose={() => setOpen(false)} />}
    </>
  )
}

const dayStart = (d: string) => new Date(d + 'T00:00:00')
const isoDay = (t: Date) => `${t.getFullYear()}-${String(t.getMonth() + 1).padStart(2, '0')}-${String(t.getDate()).padStart(2, '0')}`

export function HistoryDialog({ unit, onClose }: { unit: string; onClose: () => void }) {
  const [day, setDay] = useState(isoDay(new Date()))
  const [h, setH] = useState<UnitHistory | null>(null)
  const range = useMemo(() => {
    const from = dayStart(day)
    const to = new Date(Math.min(from.getTime() + 86400000, Date.now()))
    return { from, to }
  }, [day])
  useEffect(() => {
    setH(null)
    api.get<UnitHistory>(`/units/${unit}/history?from=${range.from.toISOString()}&to=${range.to.toISOString()}`).then(setH).catch((e) => toast.error(errText(e)))
  }, [unit, range])
  const span = range.to.getTime() - range.from.getTime() || 1
  const pct = (sec: number) => (h?.known ? Math.round((sec / h.known) * 100) : 0)
  return (
    <Modal open onClose={onClose} xl title={`${unit}: status history`} icon={<HistoryIcon size={16} className="text-sky-300" aria-hidden />}
      footer={<><a className="btn mr-auto" href={`./api/units/${unit}/history?format=csv&from=${range.from.toISOString()}&to=${range.to.toISOString()}`} download><Download size={13} aria-hidden /> CSV</a><button className="btn" onClick={onClose}>Close</button></>}>
      <div className="space-y-3">
        <Field label="Day" htmlFor="hday"><input id="hday" type="date" className="input w-44" value={day} max={isoDay(new Date())} onChange={(e) => e.target.value && setDay(e.target.value)} /></Field>
        {!h ? <p className="text-sm text-slate-500">Reading the ledger…</p> : (
          <>
            <div className="relative h-8 overflow-hidden rounded border border-slate-800 bg-slate-900" aria-label="The day, status by status">
              {h.spans.map((s, i) => {
                const left = ((ms(s.from) - range.from.getTime()) / span) * 100
                const width = (s.seconds * 1000 / span) * 100
                return <div key={i} className="absolute top-0 h-full" style={{ left: `${left}%`, width: `${Math.max(width, 0.15)}%`, background: statusMeta(s.status).dot }} title={`${hhmm(s.from)}–${hhmm(s.to)} ${statusMeta(s.status).label}${s.incident ? ` ${s.incident}` : ''}`} />
              })}
            </div>
            <div className="flex flex-wrap gap-3 text-xs text-slate-300">
              <span><b className="text-slate-100">{h.incidents.length}</b> incidents</span>
              <span><b className="text-slate-100">{pct(h.committed)}%</b> of the known time on incidents ({Math.round(h.committed / 60)} min)</span>
              {Object.entries(h.totals).sort((a, b) => b[1] - a[1]).map(([k, v]) => <span key={k}><span className="inline-block h-2 w-2 rounded-sm" style={{ background: statusMeta(k).dot }} /> {statusMeta(k).label} {Math.round(v / 60)} min</span>)}
            </div>
            <div className="max-h-80 overflow-y-auto rounded border border-slate-800">
              <table className="w-full text-xs">
                <thead><tr className="text-left text-[10px] uppercase text-slate-500"><th className="px-2 py-1">From</th><th>To</th><th>Status</th><th>Incident</th><th>Minutes</th><th>By</th><th>Why</th></tr></thead>
                <tbody className="divide-y divide-slate-800">
                  {h.spans.map((s, i) => (
                    <tr key={i}>
                      <td className="px-2 font-mono">{hhmm(s.from)}</td><td className="font-mono">{s.open ? 'now' : hhmm(s.to)}</td>
                      <td className={statusMeta(s.status).fg}>{statusMeta(s.status).label}{s.markOut ? ` (${s.markOut})` : ''}</td>
                      <td className="font-mono">{s.incident}</td><td className="font-mono">{(s.seconds / 60).toFixed(1)}</td><td>{s.by}</td><td className="text-slate-400">{s.reason}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {h.marks.length > 0 && <ul className="text-xs text-slate-400">{h.marks.map((m, i) => <li key={i}><span className="font-mono">{hhmm(m.at)}</span> {m.text} {m.by && <span className="text-slate-500">· {m.by}</span>}</li>)}</ul>}
            {h.spans.length === 0 && <p className="text-sm text-slate-500">Nothing recorded for {unit} on this day.</p>}
          </>
        )}
      </div>
    </Modal>
  )
}

// ---- 7. the contacts directory ----

export function DirectoryDialog({ onClose }: { onClose: () => void }) {
  const [q, setQ] = useState('')
  const [items, setItems] = useState<DirEntry[]>([])
  const [onCall, setOnCall] = useState<{ list: { id: string; name: string }; now?: DirEntry; until?: string; backup: boolean }[]>([])
  useEffect(() => {
    const t = setTimeout(() => {
      api.get<{ items: DirEntry[]; onCall: typeof onCall }>(`/directory?q=${encodeURIComponent(q)}`).then((r) => { setItems(r.items); setOnCall(r.onCall) }).catch((e) => toast.error(errText(e)))
    }, 200)
    return () => clearTimeout(t)
  }, [q])
  const copy = (n: string) => navigator.clipboard?.writeText(n).then(() => toast.success(`${n} copied`))
  return (
    <Modal open onClose={onClose} xl title="Contacts directory" icon={<BookUser size={16} className="text-violet-300" aria-hidden />} footer={<button className="btn" onClick={onClose}>Close</button>}>
      <div className="space-y-3">
        <input autoFocus aria-label="Search the directory" className="input" placeholder="water, medical examiner, 5550150…" value={q} onChange={(e) => setQ(e.target.value)} />
        {onCall.length > 0 && !q && (
          <div className="grid gap-1 sm:grid-cols-2">
            {onCall.map((o) => (
              <div key={o.list.id} className="rounded border border-violet-800/60 bg-violet-950/20 px-2 py-1 text-xs">
                <div className="text-[10px] uppercase text-violet-300">On call: {o.list.name}</div>
                {o.now ? <div className="text-slate-100">{o.now.name} {o.now.phones[0] && <a className="font-mono text-sky-300 underline" href={`tel:${o.now.phones[0].number}`}>{o.now.phones[0].number}</a>} <span className="text-slate-500">{o.backup ? '(backup: no shift now)' : o.until ? `until ${new Date(o.until).toLocaleString([], { weekday: 'short', hour: '2-digit', minute: '2-digit' })}` : ''}</span></div> : <div className="text-amber-300">no one on call</div>}
              </div>
            ))}
          </div>
        )}
        <ul className="max-h-[55vh] divide-y divide-slate-800 overflow-y-auto rounded border border-slate-800 text-sm">
          {items.map((d) => (
            <li key={d.id} className="px-2 py-1.5">
              <div className="flex flex-wrap items-center gap-2"><b className="text-slate-100">{d.name}</b>{d.org && <span className="text-slate-400">{d.org}</span>}{d.category && <span className="chip bg-slate-800 text-slate-300">{d.category}</span>}</div>
              <div className="flex flex-wrap gap-2 text-xs">
                {d.phones.map((p) => (
                  <span key={p.number} className="flex items-center gap-1"><span className="text-slate-500">{p.label}</span><a className="font-mono text-sky-300 underline" href={`tel:${p.number}`}>{p.number}</a><button className="btn btn-ghost h-5 px-1" aria-label={`Copy ${p.number}`} onClick={() => copy(p.number)}><Copy size={10} aria-hidden /></button></span>
                ))}
                {d.email && <a className="text-sky-300 underline" href={`mailto:${d.email}`}>{d.email}</a>}
              </div>
              {d.notes && <div className="text-xs text-slate-500">{d.notes}</div>}
            </li>
          ))}
          {items.length === 0 && <li className="p-2 text-sm text-slate-500">Nothing found.</li>}
        </ul>
      </div>
    </Modal>
  )
}

// ---- 6. the schedule ----

export function ScheduleDialog({ view, onClose }: { view: View; onClose: () => void }) {
  const me = useStore((s) => s.me)
  const [items, setItems] = useState<Scheduled[]>([])
  const [d, setD] = useState<(Omit<Scheduled, 'start' | 'until'> & { start: string; until: string; address: string; units: string[] }) | null>(null)
  const load = () => api.get<{ items: Scheduled[] }>('/schedule').then((r) => setItems(r.items)).catch((e) => toast.error(errText(e)))
  useEffect(() => { load() }, [])
  const local = (iso?: string) => {
    if (!iso) return ''
    const t = new Date(iso)
    return `${isoDay(t)}T${String(t.getHours()).padStart(2, '0')}:${String(t.getMinutes()).padStart(2, '0')}`
  }
  const edit = (x?: Scheduled) => setD(x ? { ...x, start: local(x.start), until: x.until ? local(x.until).slice(0, 10) : '', address: x.location?.address ?? '', units: x.units ?? [] }
    : { id: '', kind: 'call', title: '', type: view.callTypes.find((c) => c.code === 'TRANSFER')?.code ?? view.callTypes[0]?.code, start: local(new Date(Date.now() + 3600000).toISOString()), until: '', address: '', units: [], lead: 0, minutes: 0, repeat: '' })
  const save = async () => {
    if (!d) return
    try {
      const loc = d.address ? { ...(d.location ?? {}), address: d.address } : undefined
      await api.put('/schedule', { ...d, start: new Date(d.start).toISOString(), until: d.until ? new Date(d.until + 'T23:59:00').toISOString() : undefined, location: loc, minutes: d.kind === 'event' ? d.minutes : 0 })
      toast.success('Saved')
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const remove = async (id: string) => {
    if (!confirm('Remove this scheduled item? Incidents it made stay.')) return
    try {
      await api.del(`/schedule/${id}`)
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Modal open onClose={onClose} xl title="Scheduled calls and special events" icon={<CalendarClock size={16} className="text-teal-300" aria-hidden />}
      footer={<>{can.dispatch(me) && <button className="btn btn-go mr-auto" onClick={() => edit()}><Plus size={13} aria-hidden /> Schedule a call or event</button>}<button className="btn" onClick={onClose}>Close</button></>}>
      {!d ? (
        <table className="w-full text-sm">
          <thead><tr className="text-left text-[10px] uppercase text-slate-500"><th>Next</th><th>What</th><th>Type</th><th>Where</th><th>Repeats</th><th>Units</th><th /></tr></thead>
          <tbody className="divide-y divide-slate-800">
            {items.map((x) => (
              <tr key={x.id} className={cx('cursor-pointer hover:bg-slate-800/50', x.off && 'opacity-50')} onClick={() => edit(x)}>
                <td className="py-1 font-mono text-xs">{x.next ? new Date(x.next).toLocaleString([], { weekday: 'short', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : 'done'}</td>
                <td><span className={cx('chip mr-1', x.kind === 'event' ? 'bg-teal-500/20 text-teal-200' : 'bg-sky-500/20 text-sky-200')}>{x.kind}</span>{x.title}</td>
                <td className="text-xs text-slate-400">{typeName(view, x.type)}</td>
                <td className="text-xs text-slate-400">{x.location?.address}</td>
                <td className="text-xs text-slate-400">{({ '': 'once', daily: 'daily', weekdays: 'weekdays', weekly: 'weekly' } as Record<string, string>)[x.repeat ?? '']}</td>
                <td className="font-mono text-xs">{(x.units ?? []).join(' ')}</td>
                <td className="font-mono text-xs text-slate-500">{x.incident}</td>
              </tr>
            ))}
            {items.length === 0 && <tr><td colSpan={7} className="py-2 text-slate-500">Nothing scheduled.</td></tr>}
          </tbody>
        </table>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="flex gap-2 sm:col-span-2" role="radiogroup" aria-label="Kind">
            {(['call', 'event'] as const).map((k) => <button key={k} role="radio" aria-checked={d.kind === k} className={cx('btn', d.kind === k && 'border-teal-400 bg-teal-500/15 text-teal-100')} onClick={() => setD({ ...d, kind: k, type: k === 'event' ? (view.callTypes.find((c) => c.code === 'EVENT')?.code ?? d.type) : d.type, minutes: k === 'event' ? d.minutes || 240 : 0 })}>{k === 'call' ? 'A scheduled call' : 'A special event'}</button>)}
          </div>
          <Field label="Title" htmlFor="stitle"><input id="stitle" className="input" value={d.title} onChange={(e) => setD({ ...d, title: e.target.value })} placeholder={d.kind === 'event' ? 'County fair' : 'Dialysis transport'} /></Field>
          <Field label="Call type" htmlFor="stype"><select id="stype" className="input" value={d.type} onChange={(e) => setD({ ...d, type: e.target.value })}>{view.callTypes.map((c) => <option key={c.code} value={c.code}>{c.name} ({c.code})</option>)}</select></Field>
          <Field label="Starts" htmlFor="sstart"><input id="sstart" type="datetime-local" className="input" value={d.start} onChange={(e) => setD({ ...d, start: e.target.value })} /></Field>
          {d.kind === 'event' ? (
            <Field label="Lasts (hours)" htmlFor="smin"><input id="smin" type="number" min={0.25} step={0.25} className="input w-28" value={(d.minutes ?? 0) / 60} onChange={(e) => setD({ ...d, minutes: Math.round(Number(e.target.value) * 60) })} /></Field>
          ) : (
            <Field label="Make the incident early (minutes)" htmlFor="slead" hint="So units are sent in time"><input id="slead" type="number" min={0} max={1440} className="input w-28" value={d.lead ?? 0} onChange={(e) => setD({ ...d, lead: Number(e.target.value) })} /></Field>
          )}
          <div className="sm:col-span-2"><Field label="Where" htmlFor="saddr"><input id="saddr" className="input" value={d.address} onChange={(e) => setD({ ...d, address: e.target.value })} placeholder="1 Main St" /></Field></div>
          <Field label="Repeats" htmlFor="srep"><select id="srep" className="input" value={d.repeat ?? ''} onChange={(e) => setD({ ...d, repeat: e.target.value as Scheduled['repeat'] })}><option value="">once</option><option value="daily">every day</option><option value="weekdays">weekdays</option><option value="weekly">every week</option></select></Field>
          {d.repeat && <Field label="Until (last day)" htmlFor="suntil"><input id="suntil" type="date" className="input" value={d.until} onChange={(e) => setD({ ...d, until: e.target.value })} /></Field>}
          <div className="sm:col-span-2">
            <span className="label">Units to send when it starts (if available)</span>
            <div className="flex flex-wrap gap-1">{view.units.map((u) => <button key={u.id} aria-pressed={d.units.includes(u.id)} onClick={() => setD({ ...d, units: d.units.includes(u.id) ? d.units.filter((x) => x !== u.id) : [...d.units, u.id] })} className={cx('chip cursor-pointer font-mono', d.units.includes(u.id) ? 'bg-sky-500/30 text-sky-100' : 'bg-slate-800 text-slate-400')}>{u.id}</button>)}</div>
          </div>
          <div className="sm:col-span-2"><Field label="Notes" htmlFor="snotes"><textarea id="snotes" rows={2} className="input" value={d.notes ?? ''} onChange={(e) => setD({ ...d, notes: e.target.value })} /></Field></div>
          <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!d.off} onChange={(e) => setD({ ...d, off: e.target.checked })} /> Paused</label>
          <div className="flex justify-end gap-2 sm:col-span-2">
            {d.id && <button className="btn mr-auto text-rose-300" onClick={() => remove(d.id)}><Trash2 size={13} aria-hidden /> Remove</button>}
            <button className="btn" onClick={() => setD(null)}>Back</button>
            <button className="btn btn-go" onClick={save}>Save</button>
          </div>
        </div>
      )}
    </Modal>
  )
}

/** Today on the floor: events running and calls coming, who is on call, units entering and leaving areas. */
export function TodayBox({ view }: { view: View }) {
  const now = useNow()
  const [sched, setSched] = useState(false)
  const r = view.roadmap
  const up = (r?.upcoming ?? []).slice(0, 6)
  const alerts = (r?.fenceAlerts ?? []).filter((a) => now - ms(a.at) < 3600000).slice(-6).reverse()
  return (
    <div className="space-y-2">
      <div>
        <div className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-teal-300">
          <CalendarClock size={12} aria-hidden /> Scheduled today <span className="font-mono text-slate-500">{r?.upcoming.length ?? 0}</span>
          <button className="ml-auto text-[10px] font-normal normal-case text-slate-400 underline" onClick={() => setSched(true)}>schedule…</button>
        </div>
        {up.length === 0 ? <p className="text-[11px] text-slate-500">Nothing in the next 24 hours.</p> : (
          <ul className="text-[11px]">
            {up.map((u) => (
              <li key={u.item.id + u.at} className="flex gap-1.5">
                <span className={cx('font-mono', u.running ? 'text-teal-300' : 'text-slate-400')}>{u.running ? 'NOW' : hhmm(u.at).slice(0, 5)}</span>
                <span className="text-slate-200">{u.item.title}</span>
                <span className="truncate text-slate-500">{u.item.location?.address}{u.running ? ` · until ${hhmm(u.end).slice(0, 5)}` : ''}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
      {(r?.onCallNow.length ?? 0) > 0 && (
        <div>
          <div className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-violet-300"><Users size={12} aria-hidden /> On call now</div>
          <ul className="text-[11px]">
            {r!.onCallNow.map((o) => <li key={o.id} className="flex gap-1.5"><span className="text-slate-400">{o.list}</span><span className="text-slate-200">{o.entry?.name ?? 'no one'}</span>{o.entry?.phones[0] && <a className="font-mono text-sky-300 underline" href={`tel:${o.entry.phones[0].number}`}>{o.entry.phones[0].number}</a>}</li>)}
          </ul>
        </div>
      )}
      {alerts.length > 0 && (
        <div>
          <div className="flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-orange-300"><AlertTriangle size={12} aria-hidden /> Area alerts</div>
          <ul className="text-[11px]">{alerts.map((a) => <li key={a.seq + a.fence} className="flex gap-1.5"><span className="font-mono text-slate-400">{hhmm(a.at).slice(0, 5)}</span><b className="font-mono text-slate-100">{a.unit}</b><span className="text-slate-300">{a.enter ? 'entered' : 'left'} {a.name}</span></li>)}</ul>
        </div>
      )}
      {sched && <ScheduleDialog view={view} onClose={() => setSched(false)} />}
    </div>
  )
}

/** Toasts each new area alert once. */
export function useFenceToasts(view: View | null) {
  const [seen] = useState(() => new Set<string>())
  const [start] = useState(() => Date.now())
  useEffect(() => {
    for (const a of view?.roadmap?.fenceAlerts ?? []) {
      const k = `${a.seq}|${a.fence}`
      if (seen.has(k)) continue
      seen.add(k)
      if (ms(a.at) >= start - 5000) toast.warning(`${a.unit} ${a.enter ? 'entered' : 'left'} ${a.name}`, { duration: 10000 })
    }
  }, [view?.roadmap?.fenceAlerts, seen, start])
}

export { ago }
