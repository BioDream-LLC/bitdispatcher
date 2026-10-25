import { useEffect, useRef, useState } from 'react'
import { AlertTriangle, Check, Mail, Paperclip, VolumeX } from 'lucide-react'
import { api, errText } from '../api'
import { playTone, soundReady, unlockSound } from '../tones'
import { cx } from '../util'
import type { CommFile, CommList, CommPriority, CommView, PriorityRule } from '../types'

const ORDER: CommPriority[] = ['urgent', 'high', 'medium', 'low']
const LABEL: Record<CommPriority, string> = { urgent: 'URGENT', high: 'HIGH', medium: 'MEDIUM', low: 'LOW' }
const CLASS: Record<CommPriority, string> = {
  urgent: 'border-rose-500 bg-rose-500/25 text-rose-50',
  high: 'border-orange-500 bg-orange-500/20 text-orange-100',
  medium: 'border-amber-400 bg-amber-400/15 text-amber-100',
  low: 'border-slate-600 bg-slate-800 text-slate-200',
}
const live = (c: CommView) => c.state !== 'cancelled' && c.state !== 'expired'

/** MdtBulletins: the messages, BOLOs and officer-safety notices addressed to this crew, with the agency's
 * priority tones, repeats and acknowledgement, as on the dispatch console. The crew's acknowledgement is
 * recorded against the terminal's sign-on (the unit), and opening one records that it was read. */
export default function MdtBulletins() {
  const [list, setList] = useState<CommList | null>(null)
  const [open, setOpen] = useState<string>()
  const [detail, setDetail] = useState<{ versions: CommView[]; files: CommFile[] } | null>(null)
  const [err, setErr] = useState('')
  const [silenced, setSilenced] = useState<Set<CommPriority>>(new Set())
  const [soundOn, setSoundOn] = useState(soundReady())
  const seen = useRef<Set<string>>(new Set())
  const timers = useRef<Record<string, number>>({})

  const load = () => api.get<CommList>('/comms?folder=inbox').then((l) => { setList(l); setErr('') }).catch((e) => setErr(errText(e)))
  useEffect(() => {
    load()
    const t = window.setInterval(load, 8000)
    return () => window.clearInterval(t)
  }, [])
  useEffect(() => unlockSound(), [])
  useEffect(() => {
    const t = window.setInterval(() => setSoundOn(soundReady()), 2000)
    return () => window.clearInterval(t)
  }, [])

  const items = list?.items ?? []
  const rules = list?.priorities ?? []
  const rule = (p: CommPriority): PriorityRule | undefined => rules.find((r) => r.level === p)

  // The same rules as the console: a new item sounds once (the highest priority among several), and
  // repeats while it still needs acknowledging, unless the agency lets that level be silenced and it was.
  useEffect(() => {
    const ids = new Set(items.map((i) => i.id))
    for (const id of Object.keys(timers.current)) {
      if (!ids.has(id)) { window.clearInterval(timers.current[id]); delete timers.current[id] }
    }
    let first: PriorityRule | undefined
    for (const it of items) {
      const r = rule(it.priority)
      if (!r || r.tone === 'none') continue
      const needs = (it.needsAck && !it.ackedByMe) || (!it.openedByMe && live(it))
      const quiet = r.silenceable && silenced.has(it.priority)
      if (needs && !seen.current.has(it.id)) {
        seen.current.add(it.id)
        if (!quiet && (!first || ORDER.indexOf(r.level) < ORDER.indexOf(first.level))) first = r
      }
      const repeat = it.needsAck && !it.ackedByMe && (r.repeat ?? 0) > 0 && !quiet
      if (repeat && !timers.current[it.id]) timers.current[it.id] = window.setInterval(() => playTone(r.tone), (r.repeat ?? 0) * 1000)
      if (!repeat && timers.current[it.id]) { window.clearInterval(timers.current[it.id]); delete timers.current[it.id] }
    }
    if (first) playTone(first.tone)
  }, [items, rules, silenced])
  useEffect(() => () => { for (const id of Object.keys(timers.current)) window.clearInterval(timers.current[id]) }, [])

  const show = async (id: string) => {
    setOpen(id)
    try {
      setDetail(await api.get(`/comms/${encodeURIComponent(id)}`))
      await api.post(`/comms/${encodeURIComponent(id)}/open`)
      load()
    } catch (e) { setErr(errText(e)) }
  }
  const ack = async (id: string) => {
    try { await api.post(`/comms/${encodeURIComponent(id)}/ack`); load() } catch (e) { setErr(errText(e)) }
  }

  const shown = items.filter(live).sort((a, b) => ORDER.indexOf(a.priority) - ORDER.indexOf(b.priority) || b.issued.localeCompare(a.issued))
  const interrupting = shown.filter((i) => (i.priority === 'urgent' || i.priority === 'high') && i.needsAck && !i.ackedByMe)
  const unread = shown.filter((i) => !i.openedByMe).length
  if (!list && !err) return null
  const current = detail?.versions[detail.versions.length - 1]

  return (
    <section className="rounded-lg border border-slate-700 bg-slate-900/60 p-2" aria-label="Messages and BOLOs for this unit">
      {interrupting.length > 0 && (
        <div role="alert" className="mb-2 space-y-1">
          {!soundOn && (
            <button className="btn w-full justify-center border-amber-400 bg-amber-500/20 py-2 text-sm font-bold text-amber-100" onClick={() => playTone('tick')}>
              <VolumeX size={15} aria-hidden /> Sound is off on this terminal: tap to turn the alert sounds on
            </button>
          )}
          {interrupting.map((it) => (
            <div key={it.id} className={cx('flex items-center gap-2 rounded border-2 px-2 py-2', CLASS[it.priority])}>
              <AlertTriangle size={18} aria-hidden />
              <span className="font-mono text-xs font-bold">{LABEL[it.priority]}</span>
              <button className="min-w-0 flex-1 truncate text-left text-base font-semibold" onClick={() => show(it.id)}>{it.subject}</button>
              <button className="btn btn-go h-10 px-3" onClick={() => ack(it.id)}><Check size={15} aria-hidden /> Acknowledge</button>
              {rule(it.priority)?.silenceable && !silenced.has(it.priority) && (
                <button className="btn btn-ghost h-10 px-2" aria-label={`Silence the ${LABEL[it.priority]} tone`} onClick={() => setSilenced((s) => new Set(s).add(it.priority))}><VolumeX size={15} aria-hidden /></button>
              )}
            </div>
          ))}
        </div>
      )}
      <div className="mb-1 flex items-center gap-2 text-[11px] font-bold uppercase tracking-wider text-sky-300">
        <Mail size={13} aria-hidden /> Messages and BOLOs {unread > 0 && <span className="chip bg-sky-500/25 text-sky-100">{unread} unread</span>}
      </div>
      {err && <div className="text-xs text-rose-300">{err}</div>}
      {shown.length === 0 ? (
        <div className="text-xs text-slate-500">Nothing in force for this unit.</div>
      ) : (
        <ul className="space-y-1">
          {shown.slice(0, 12).map((it) => (
            <li key={it.id}>
              <button className={cx('flex w-full items-center gap-2 rounded border px-2 py-1.5 text-left text-sm', open === it.id ? 'border-sky-400' : 'border-slate-700', !it.openedByMe && 'font-semibold')} onClick={() => show(it.id)}>
                <span className={cx('chip border text-[10px]', CLASS[it.priority])}>{LABEL[it.priority]}</span>
                <span className="text-[10px] uppercase text-slate-400">{it.kind}</span>
                <span className="min-w-0 flex-1 truncate">{it.subject}</span>
                {it.plate && <span className="font-mono text-amber-200">{it.plate}</span>}
                {it.needsAck && !it.ackedByMe && <span className="chip bg-rose-500/30 text-rose-100">ack</span>}
                {it.state === 'updated' && <span className="chip bg-sky-500/20 text-sky-100">updated</span>}
              </button>
            </li>
          ))}
        </ul>
      )}
      {open && current && (
        <div className="mt-2 space-y-1 rounded border border-slate-600 bg-slate-950/60 p-2 text-sm" aria-label="The open message">
          <div className="flex items-center gap-2">
            <span className={cx('chip border', CLASS[current.priority])}>{LABEL[current.priority]}</span>
            <b className="flex-1">{current.subject}</b>
            {detail!.versions.length > 1 && <span className="text-xs text-slate-400">version {current.version} of {detail!.versions.length}</span>}
          </div>
          <div className="text-xs text-slate-400">from {current.fromName || current.from}{current.fromAgency ? ` · ${current.fromAgency}` : ''} · {new Date(current.issued).toLocaleString()}{current.incident ? ` · incident ${current.incident}` : ''}</div>
          <p className="whitespace-pre-wrap text-slate-100">{current.body}</p>
          {(current.plate || current.vehicle || current.person || current.description) && (
            <div className="text-xs text-amber-100">{[current.plate && `plate ${current.plate}${current.plateState ? ` (${current.plateState})` : ''}`, current.vehicle, current.person, current.description].filter(Boolean).join(' · ')}</div>
          )}
          {detail!.files.filter((f) => !f.removed).map((f) => (
            <a key={f.id} className="flex items-center gap-1 text-xs text-sky-300 underline" href={`/api/comms/files/${encodeURIComponent(f.id)}`} target="_blank" rel="noreferrer"><Paperclip size={12} aria-hidden />{f.name}</a>
          ))}
          <div className="flex gap-2 pt-1">
            {current.needsAck && !current.ackedByMe && <button className="btn btn-go h-9" onClick={() => ack(current.id)}><Check size={14} aria-hidden /> Acknowledge</button>}
            <button className="btn h-9" onClick={() => { setOpen(undefined); setDetail(null) }}>Close</button>
          </div>
        </div>
      )}
    </section>
  )
}
