import { useEffect, useRef, useState } from 'react'
import { MessageSquare, Send } from 'lucide-react'
import { api, errText } from '../api'
import { cx } from '../util'
import type { UnitMessage } from '../types'

const QUICK = ['Copy', 'Delayed', 'Need PD', 'Need EMS', 'Need another unit', 'Call me']

/** MdtMessages: messages for this unit from dispatch (and other units), and a reply line with quick replies.
 * An unread message is marked read when shown; an urgent one fills the top of the screen until it is. */
export default function MdtMessages({ unit, messages, unitParam }: { unit: string; messages: UnitMessage[]; unitParam: string }) {
  const [text, setText] = useState('')
  const [to, setTo] = useState('DISPATCH')
  const [err, setErr] = useState('')
  const incoming = messages.filter((m) => m.from !== unit)
  const unread = incoming.filter((m) => !m.read?.[unit])
  const urgent = unread.find((m) => m.urgent)
  const marked = useRef(new Set<string>())
  // Ordinary messages are read when they are on the screen; urgent ones when the crew presses Got it.
  useEffect(() => {
    for (const m of unread) {
      if (m.urgent || marked.current.has(m.id)) continue
      marked.current.add(m.id)
      api.post(`/mdt/message/${m.id}/read${unitParam}`).catch(() => {})
    }
  }, [unread, unitParam])
  // A new message sounds once.
  const heard = useRef<Set<string> | null>(null)
  useEffect(() => {
    if (heard.current === null) { heard.current = new Set(unread.map((m) => m.id)); return }
    if (unread.some((m) => !heard.current!.has(m.id))) beep(unread.some((m) => m.urgent))
    unread.forEach((m) => heard.current!.add(m.id))
  }, [unread])
  const send = async (t: string) => {
    try {
      await api.post('/mdt/message' + unitParam, { to: [to], text: t })
      setText('')
      setErr('')
    } catch (e) {
      setErr(errText(e))
    }
  }
  const last = messages.slice(-8).reverse()
  return (
    <>
      {urgent && (
        <div role="alert" className="flash fixed inset-x-0 top-0 z-50 flex items-center gap-3 bg-rose-700 px-4 py-3 text-lg font-bold text-white shadow-2xl">
          <MessageSquare size={22} aria-hidden /> <span className="min-w-0 flex-1">{urgent.from}: {urgent.text}</span>
          <button className="btn h-11 bg-white px-4 text-base font-bold text-rose-700" onClick={() => { marked.current.add(urgent.id); api.post(`/mdt/message/${urgent.id}/read${unitParam}`).catch(() => {}) }}>Got it</button>
        </div>
      )}
      <section className="rounded-lg border border-sky-900/70 bg-sky-950/20 p-2" aria-label="Messages">
        <div className="mb-1 flex items-center gap-2 text-[11px] font-bold uppercase tracking-wider text-sky-300">
          <MessageSquare size={12} aria-hidden /> Messages {unread.length > 0 && <span className="chip bg-sky-500 text-white">{unread.length} new</span>}
        </div>
        <ul className="space-y-1 text-base">
          {last.length === 0 && <li className="text-sm text-slate-500">No messages today.</li>}
          {last.map((m) => (
            <li key={m.id} className={cx('rounded px-2 py-1', m.from === unit ? 'bg-slate-950/40 text-slate-300' : m.urgent ? 'bg-rose-950/60 font-semibold text-rose-100 ring-1 ring-rose-700' : 'bg-slate-950/70 text-slate-100')}>
              <span className="mr-2 font-mono text-xs text-slate-500">{new Date(m.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
              <span className="mr-1 font-mono text-xs text-sky-300">{m.from === unit ? `to ${m.to.join(', ')}` : m.from}</span>{m.text}
              {m.from === unit && m.read?.DISPATCH && <span className="ml-1 text-xs text-emerald-400">read</span>}
            </li>
          ))}
        </ul>
        <div className="mt-2 flex flex-wrap gap-1">
          {QUICK.map((q) => <button key={q} type="button" className="btn h-9 px-3 text-sm" onClick={() => send(q)}>{q}</button>)}
        </div>
        <form className="mt-1 flex gap-1" onSubmit={(e) => { e.preventDefault(); if (text.trim()) send(text.trim()) }}>
          <input className="input h-11 w-28 font-mono text-sm uppercase" value={to} onChange={(e) => setTo(e.target.value.toUpperCase())} aria-label="To: DISPATCH or a unit" />
          <input className="input h-11 text-base" placeholder="Message" value={text} maxLength={1000} onChange={(e) => setText(e.target.value)} aria-label="Message" />
          <button className="btn h-11 px-4" disabled={!text.trim()} aria-label="Send message"><Send size={16} aria-hidden /> Send</button>
        </form>
        {err && <p role="alert" className="mt-1 text-sm text-rose-300">{err}</p>}
      </section>
    </>
  )
}

/** beep sounds a short tone (two for urgent) with the Web Audio API; silent where audio is blocked. */
function beep(urgent: boolean) {
  try {
    const ctx = new AudioContext()
    const tones = urgent ? [0, 0.25] : [0]
    for (const t of tones) {
      const o = ctx.createOscillator()
      const g = ctx.createGain()
      o.frequency.value = urgent ? 1200 : 880
      g.gain.setValueAtTime(0.15, ctx.currentTime + t)
      g.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + t + 0.2)
      o.connect(g).connect(ctx.destination)
      o.start(ctx.currentTime + t)
      o.stop(ctx.currentTime + t + 0.2)
    }
  } catch {
    // no audio here
  }
}
