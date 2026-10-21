import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { ClipboardCheck, Hospital, MessageSquare, Plus, Send, Siren, X } from 'lucide-react'
import { api, errText } from '../api'
import { can, useStore } from '../store'
import { ago, cx, hhmm, useNow } from '../util'
import type { Facility, UnitMessage, View } from '../types'

/** The passdown log: what this shift needs to know from the last, and what the next needs from this one. */
export function Passdown({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const [text, setText] = useState('')
  const [open, setOpen] = useState(false)
  const list = (view.entry?.passdown ?? []).filter((p) => !p.for || p.for === me?.duty || me?.duty === 'supervisor' || me?.duty === 'dispatcher')
  const add = async () => {
    try {
      await api.post('/passdown', { text, for: '' })
      setText('')
      setOpen(false)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  if (!list.length && !can.dispatch(me)) return null
  return (
    <section>
      <h3 className="mb-1 flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400">
        <ClipboardCheck size={12} aria-hidden /> Passdown <span className="font-mono text-slate-300">{list.length}</span>
        {can.dispatch(me) && <button className="btn btn-ghost ml-auto h-5 px-1 text-[10px] normal-case tracking-normal" onClick={() => setOpen(!open)}><Plus size={10} aria-hidden /> For the next shift</button>}
      </h3>
      {open && (
        <form className="mb-1 flex gap-1" onSubmit={(e) => { e.preventDefault(); if (text.trim()) add() }}>
          <input className="input h-7 text-xs" autoFocus value={text} onChange={(e) => setText(e.target.value)} placeholder="Road closed on Orange Ave until 18:00; M3 has a broken stretcher…" aria-label="Passdown entry" />
          <button className="btn h-7 text-xs" disabled={!text.trim()}>Add</button>
        </form>
      )}
      <ul className="space-y-0.5 text-xs">
        {list.slice(0, 6).map((p) => <li key={p.id} className="text-slate-200"><span className="font-mono text-[10px] text-slate-500">{hhmm(p.at)} {p.by}</span> {p.text}</li>)}
        {!list.length && <li className="text-[11px] text-slate-500">Nothing passed down in the last two days.</li>}
      </ul>
    </section>
  )
}

/** Active BOLOs: issue one, add to it, cancel it; units see them on their terminals. */
export function BOLOs({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const now = useNow()
  const [text, setText] = useState('')
  const [plate, setPlate] = useState('')
  const [open, setOpen] = useState(false)
  const list = view.entry?.bolos ?? []
  const issue = async () => {
    try {
      await api.post('/bolos', { text, plate })
      setText('')
      setPlate('')
      setOpen(false)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const cancel = (id: string) => api.post(`/bolos/${id}/cancel`, { why: 'cancelled by ' + (me?.username ?? '') }).catch((e) => toast.error(errText(e)))
  return (
    <section>
      <h3 className="mb-1 flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400">
        <Siren size={12} aria-hidden /> BOLOs <span className="font-mono text-slate-300">{list.length}</span>
        {can.dispatch(me) && <button className="btn btn-ghost ml-auto h-5 px-1 text-[10px] normal-case tracking-normal" onClick={() => setOpen(!open)}><Plus size={10} aria-hidden /> Issue</button>}
      </h3>
      {open && (
        <form className="mb-1 grid grid-cols-[1fr_7rem_auto] gap-1" onSubmit={(e) => { e.preventDefault(); if (text.trim()) issue() }}>
          <input className="input h-7 text-xs" autoFocus value={text} onChange={(e) => setText(e.target.value)} placeholder="Silver Honda Civic, armed robbery suspect, northbound Williamson Rd" aria-label="BOLO text" />
          <input className="input h-7 font-mono text-xs uppercase" value={plate} onChange={(e) => setPlate(e.target.value)} placeholder="Plate" aria-label="Plate" />
          <button className="btn h-7 text-xs" disabled={!text.trim()}>Issue</button>
        </form>
      )}
      <ul className="space-y-0.5 text-xs">
        {list.map((b) => (
          <li key={b.id} className="flex items-start gap-1.5">
            <span className="chip shrink-0 bg-rose-500/20 font-mono text-rose-200">{b.id}</span>
            <span className="min-w-0 text-slate-200">{b.text}{b.plate && <span className="ml-1 font-mono text-amber-200">{b.plate}</span>}<span className="ml-1 text-[10px] text-slate-500">{ago(b.issued, now)} ago · {b.by}{b.incident ? ` · ${b.incident}` : ''}</span></span>
            {can.dispatch(me) && <button className="btn btn-ghost ml-auto h-5 shrink-0 px-1" onClick={() => cancel(b.id)} aria-label={`Cancel ${b.id}`}><X size={11} /></button>}
          </li>
        ))}
        {!list.length && <li className="text-[11px] text-slate-500">None out. Issue one here or with BOLO text on the command line.</li>}
      </ul>
    </section>
  )
}

const FAC: Record<Facility['status'], string> = { open: 'bg-emerald-500/20 text-emerald-200', diversion: 'bg-amber-500 text-black', closed: 'bg-rose-500 text-white' }

/** Receiving facilities and whether each is taking patients, kept by the EMS dispatcher. */
export function Facilities({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const list = view.entry?.facilities ?? []
  if (!list.length) return null
  const set = (f: Facility, status: Facility['status']) => api.post(`/facilities/${f.id}/status`, { status, note: '' }).catch((e) => toast.error(errText(e)))
  return (
    <section>
      <h3 className="mb-1 flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400"><Hospital size={12} aria-hidden /> Facilities</h3>
      <ul className="space-y-0.5 text-xs">
        {list.map((f) => (
          <li key={f.id} className="flex items-center gap-1.5">
            <span className={cx('chip w-20 justify-center uppercase', FAC[f.status])}>{f.status}</span>
            <span className="truncate text-slate-200">{f.name}</span>
            {f.kind && <span className="text-[10px] text-slate-500">{f.kind}</span>}
            {can.dispatch(me) && (
              <span className="ml-auto flex shrink-0 gap-0.5">
                {(['open', 'diversion', 'closed'] as const).filter((s) => s !== f.status).map((s) => <button key={s} className="btn h-5 px-1 text-[10px]" onClick={() => set(f, s)}>{s}</button>)}
              </span>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}

const GROUPS = ['ALL', 'ALL-FIRE', 'ALL-EMS', 'ALL-POLICE']

/** Messages to and from the crews' terminals. What units send to dispatch stays bold until a dispatcher
 * reads it; urgent ones are red. Write to a unit, several, a discipline or everyone (or MSG on the command line). */
export function Messages({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const selected = useStore((s) => s.selected)
  const now = useNow()
  const [to, setTo] = useState('')
  const [text, setText] = useState('')
  const [urgent, setUrgent] = useState(false)
  const [all, setAll] = useState(false)
  const list = (view.messages ?? []).slice().reverse()
  const unread = list.filter((m) => m.fromUnit && m.to.includes('DISPATCH') && !m.read?.DISPATCH)
  const shown = all ? list : list.slice(0, 6)
  const send = async () => {
    const dest = to.split(/[\s,]+/).map((x) => x.trim().toUpperCase()).filter(Boolean)
    try {
      await api.post('/messages', { to: dest, incident: dest.length ? '' : selected ?? '', text, urgent })
      setText('')
      setUrgent(false)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const read = (m: UnitMessage) => api.post(`/messages/${m.id}/read`).catch(() => {})
  // A new message from a crew is announced once.
  const seen = useRef<Set<string> | null>(null)
  useEffect(() => {
    if (seen.current === null) { seen.current = new Set(unread.map((m) => m.id)); return }
    for (const m of unread) {
      if (seen.current.has(m.id)) continue
      seen.current.add(m.id)
      ;(m.urgent ? toast.error : toast.info)(`${m.from}: ${m.text}`, { description: m.incident ? `on ${m.incident}` : 'message from a unit' })
    }
  }, [unread])
  if (!can.dispatch(me)) return null
  return (
    <section aria-label="Messages">
      <h3 className="mb-1 flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400">
        <MessageSquare size={12} aria-hidden /> Messages
        {unread.length > 0 ? <span className="chip bg-sky-500 font-mono text-white">{unread.length} new</span> : <span className="font-mono text-slate-300">{list.length}</span>}
        {list.length > 6 && <button className="btn btn-ghost ml-auto h-5 px-1 text-[10px] normal-case tracking-normal" onClick={() => setAll(!all)}>{all ? 'Fewer' : 'All today'}</button>}
      </h3>
      <form className="mb-1 grid grid-cols-[6.5rem_1fr_auto_auto] gap-1" onSubmit={(e) => { e.preventDefault(); if (text.trim()) send() }}>
        <input className="input h-7 font-mono text-xs uppercase" list="msg-to" value={to} onChange={(e) => setTo(e.target.value)} placeholder={selected ? `on ${selected.slice(-6)}` : 'E1 M2, ALL'} aria-label="To (units, ALL, ALL-FIRE…)" title="Units, ALL, ALL-FIRE, ALL-EMS or ALL-POLICE; empty: the units on the selected incident" />
        <datalist id="msg-to">{[...GROUPS, ...view.units.map((u) => u.id)].map((x) => <option key={x} value={x} />)}</datalist>
        <input className="input h-7 text-xs" value={text} maxLength={1000} onChange={(e) => setText(e.target.value)} placeholder="Message to the terminals" aria-label="Message" />
        <label className={cx('btn h-7 px-1.5 text-[10px]', urgent && 'bg-rose-600 text-white')} title="Urgent: the terminal shows it full width and sounds"><input type="checkbox" className="sr-only" checked={urgent} onChange={(e) => setUrgent(e.target.checked)} />!</label>
        <button className="btn h-7 px-2 text-xs" disabled={!text.trim() || (!to.trim() && !selected)} aria-label="Send"><Send size={12} aria-hidden /></button>
      </form>
      <ul className="space-y-0.5 text-xs">
        {shown.map((m) => {
          const isNew = m.fromUnit && m.to.includes('DISPATCH') && !m.read?.DISPATCH
          const readBy = Object.keys(m.read ?? {}).filter((k) => k !== 'DISPATCH')
          return (
            <li key={m.id}>
              <button type="button" className={cx('block w-full rounded px-1 text-left hover:bg-slate-800/60', isNew && 'bg-sky-950/60 font-semibold', m.urgent && 'text-rose-200')} onClick={() => isNew && read(m)} title={isNew ? 'Mark read' : undefined}>
                <span className="font-mono text-[10px] text-slate-500">{hhmm(m.at)}</span>{' '}
                <span className="font-mono text-slate-300">{m.from} → {m.to.join(', ')}</span>{m.incident && <span className="font-mono text-[10px] text-slate-500"> {m.incident}</span>}{' '}
                <span className={m.urgent ? 'text-rose-200' : 'text-slate-100'}>{m.urgent && '! '}{m.text}</span>
                {!m.fromUnit && readBy.length > 0 && <span className="ml-1 text-[10px] text-emerald-400">read by {readBy.join(', ')}</span>}
                {!m.fromUnit && readBy.length === 0 && now - new Date(m.at).getTime() > 120000 && <span className="ml-1 text-[10px] text-amber-400">not read yet</span>}
              </button>
            </li>
          )
        })}
        {!list.length && <li className="text-[11px] text-slate-500">No messages today. Write one here or with MSG E1 / text.</li>}
      </ul>
    </section>
  )
}
