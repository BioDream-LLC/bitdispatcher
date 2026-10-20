import { useState } from 'react'
import { toast } from 'sonner'
import { ClipboardCheck, Hospital, Plus, Siren, X } from 'lucide-react'
import { api, errText } from '../api'
import { can, useStore } from '../store'
import { ago, cx, hhmm, useNow } from '../util'
import type { Facility, View } from '../types'

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
