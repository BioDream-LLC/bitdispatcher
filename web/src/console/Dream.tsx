import { TrainingTools } from './TrainingTools'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { BellOff, ClipboardCopy, Coffee, Drama, GraduationCap, HeartHandshake, NotebookPen, Plane, Play, RotateCcw, ShieldAlert, Trophy, Video, WifiOff } from 'lucide-react'
import { drop, keep, outbox, send, type OutItem } from '../offline'
import { api, errText } from '../api'
import { can, useStore } from '../store'
import { Modal } from '../ui'
import { ago, cx, hasPoint, useNow } from '../util'
import type { Check, Incident, Review, View } from '../types'

/** SummaryBox is the incident in a paragraph, for the crews and the next dispatcher. */
export function SummaryBox({ inc, text }: { inc: Incident; text: string }) {
  return (
    <section aria-label="Summary" className="rounded-md border border-sky-900/50 bg-sky-950/20 p-1.5 text-xs text-slate-200">
      <div className="mb-0.5 flex items-center gap-1 text-[10px] font-bold uppercase tracking-wider text-sky-300">
        Summary
        <button className="ml-auto rounded px-1 text-slate-400 hover:text-sky-200" aria-label={`Copy the summary of ${inc.number}`} title="Copy"
          onClick={() => navigator.clipboard?.writeText(text).then(() => toast.success('Summary copied')).catch(() => {})}>
          <ClipboardCopy size={11} aria-hidden />
        </button>
      </div>
      <p className="leading-snug">{text}</p>
    </section>
  )
}

/** DroneBox is the drones over an incident, and the button that asks for one. */
export function DroneBox({ inc, view }: { inc: Incident; view: View }) {
  const me = useStore((s) => s.me)
  const now = useNow()
  const [busy, setBusy] = useState(false)
  const ds = (view.drones ?? []).filter((d) => d.incident === inc.number && d.status !== 'docked')
  const asked = inc.timeline.some((t) => t.kind === 'drone')
  if (!ds.length && !(view.health.drone && can.dispatch(me) && inc.status !== 'closed')) return null
  return (
    <section aria-label="Drone" className="rounded-md border border-cyan-900/60 bg-cyan-950/15 p-1.5 text-xs">
      <div className="flex items-center gap-1.5 text-[11px] font-semibold text-cyan-200">
        <Plane size={12} aria-hidden /> Drone
        {ds.length === 0 && view.health.drone && can.dispatch(me) && inc.status !== 'closed' && (
          <button className="btn ml-auto h-6 px-2 py-0 text-[11px]" disabled={busy || !hasPoint(inc.location)} title={hasPoint(inc.location) ? 'Ask the drone system to fly to this incident' : 'Place the incident on the map first'}
            onClick={async () => {
              setBusy(true)
              try {
                const r = await api.post<{ detail: string }>(`/incidents/${inc.number}/drone`)
                toast.success(`Drone: ${r.detail}`)
              } catch (e) {
                toast.error(errText(e))
              } finally {
                setBusy(false)
              }
            }}>{asked ? 'Ask again' : 'Send a drone'}</button>
        )}
      </div>
      {ds.map((d) => (
        <div key={d.id} className="mt-1 flex flex-wrap items-center gap-1.5">
          <span className="font-semibold text-slate-100">{d.name ?? d.id}</span>
          <span className={cx('chip uppercase', d.status === 'onscene' ? 'bg-cyan-500 text-slate-950' : 'bg-cyan-500/20 text-cyan-200')}>{d.status === 'onscene' ? 'over the scene' : d.status}</span>
          {d.battery ? <span className="font-mono text-[10px] text-slate-400">battery {d.battery}%</span> : null}
          <span className="text-[10px] text-slate-500">{ago(d.updated, now)} ago</span>
          {d.videoUrl && <a className="btn ml-auto h-6 px-2 py-0 text-[11px]" href={d.videoUrl} target="_blank" rel="noreferrer"><Video size={11} aria-hidden /> Live picture</a>}
        </div>
      ))}
    </section>
  )
}

/** BreakButton puts this person on a break, or back. */
export function BreakButton({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  if (!me) return null
  const b = view.breaks?.[me.username]
  const on = !!b?.on
  return (
    <button className={cx('btn h-7 px-1.5 text-[11px]', on ? 'border-teal-500 bg-teal-500/20 text-teal-100' : 'btn-ghost')} aria-pressed={on} aria-label={on ? 'On break: press when back' : 'Go on a break'}
      title={on ? 'On a break: press when you are back' : 'Going on a break: the supervisor board shows it'}
      onClick={() => api.post('/me/break', { on: !on }).then(() => toast.success(on ? 'Welcome back' : 'Enjoy your break')).catch((e) => toast.error(errText(e)))}>
      <Coffee size={14} aria-hidden />{on && ' On break'}
    </button>
  )
}

/** CareBanner tells a person, gently, after a critical call they worked. */
export function CareBanner({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const [gone, setGone] = useState(() => new Set<string>(JSON.parse(localStorage.getItem('bd.care.seen') ?? '[]')))
  if (!me) return null
  const cc = (view.care ?? []).find((c) => c.status !== 'closed' && c.people.includes(me.username) && !gone.has(c.incident)
    && view.incidents.find((i) => i.number === c.incident)?.status === 'closed')
  if (!cc) return null
  const dismiss = () => {
    const n = new Set(gone).add(cc.incident)
    localStorage.setItem('bd.care.seen', JSON.stringify([...n].slice(-50)))
    setGone(n)
  }
  return (
    <div role="status" className="flex items-center gap-2 border-b border-teal-900 bg-teal-950/60 px-3 py-1.5 text-sm text-teal-50">
      <HeartHandshake size={16} className="shrink-0 text-teal-300" aria-hidden />
      <span>That was a hard call ({cc.incident}). Take a moment when you can.{view.settings.supportLine ? <> {view.settings.supportLine}.</> : null}{cc.offered?.includes(me.username) ? ' Your supervisor has offered you a break.' : ''}</span>
      <button className="btn h-7 text-xs" onClick={() => api.post('/me/break', { on: true }).then(dismiss).catch((e) => toast.error(errText(e)))}><Coffee size={13} aria-hidden /> Take a break</button>
      <button className="btn btn-ghost ml-auto h-7 text-xs" onClick={dismiss}>Not now</button>
    </div>
  )
}

/** TDoSBanner warns of a possible telephony denial-of-service. */
export function TDoSBanner({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const t = view.tdos
  if (!t?.active) return null
  return (
    <div role="alert" className="flex flex-wrap items-center gap-2 border-b border-orange-800 bg-orange-950/70 px-3 py-1.5 text-sm text-orange-50">
      <ShieldAlert size={16} className="shrink-0 text-orange-300" aria-hidden />
      <b>Possible telephony denial-of-service</b>
      <span className="text-orange-100/90">{(t.reasons ?? []).join(' · ')}</span>
      {t.ackedBy ? <span className="ml-auto text-xs text-orange-200/80">acknowledged by {t.ackedBy}</span> : can.supervise(me) ? (
        <button className="btn ml-auto h-7 text-xs" onClick={() => api.post('/tdos/ack').catch((e) => toast.error(errText(e)))}><BellOff size={13} aria-hidden /> Acknowledge</button>
      ) : <span className="ml-auto text-xs text-orange-200">tell the supervisor; keep answering</span>}
    </div>
  )
}

type Scenario = { id: string; name: string; about: string; teaches: string[]; minutes: number }
type Score = { scenario: string; name: string; score: number; checks: Check[]; reviews: Review[] }

/** TrainingBar marks the training environment and runs its scenarios. */
export function TrainingBar({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const [list, setList] = useState<Scenario[] | null>(null)
  const [score, setScore] = useState<Score | null>(null)
  const t = view.training
  useEffect(() => {
    if (t && !t.scenario && can.dispatch(me) && !me?.exercise && me?.duty !== 'admin') api.get<{ items: Scenario[] }>('/training/scenarios').then((r) => setList(r.items)).catch(() => {})
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [!!t])
  if (!t) return null
  const start = async (id: string) => {
    try {
      await api.post('/training/start', { id })
      setList(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <>
      <div role="status" className="flex items-center gap-2 border-b border-violet-800 bg-violet-950/70 px-3 py-1 text-sm text-violet-50">
        <GraduationCap size={16} className="text-violet-300" aria-hidden />
        <b>TRAINING</b> <span className="text-violet-200/80">a board of its own: nothing here reaches the live board</span>
        {t.scenario && <span className="chip bg-violet-500/30 text-violet-100">{t.name} · {t.played}/{t.steps}</span>}
        <span className="ml-auto flex items-center gap-1">
          <TrainingTools view={view} />
          <button className="btn h-7 text-xs" onClick={() => api.get<{ items: Scenario[] }>('/training/scenarios').then((r) => setList(r.items)).catch((e) => toast.error(errText(e)))}><Drama size={13} aria-hidden /> Scenarios</button>
          {t.scenario && <button className="btn h-7 text-xs" onClick={() => api.get<Score>('/training/score').then(setScore).catch((e) => toast.error(errText(e)))}><Trophy size={13} aria-hidden /> Score</button>}
          <button className="btn h-7 text-xs" title="Start again with an empty board" onClick={() => api.post('/training/reset').then(() => toast.success('The training board is empty again')).catch((e) => toast.error(errText(e)))}><RotateCcw size={13} aria-hidden /> Reset</button>
        </span>
      </div>
      <Modal open={!!list} onClose={() => setList(null)} title="Training scenarios" icon={<Drama size={16} className="text-violet-300" aria-hidden />} wide>
        <p className="mb-3 text-sm text-slate-400">Each plays calls, transcripts, crash data and radio traffic into this training board, on your agency's own map and units. Reset first for a clean board.</p>
        <ul className="space-y-2">
          {(list ?? []).map((s) => (
            <li key={s.id} className="rounded-lg border border-slate-800 p-3">
              <div className="flex items-center gap-2">
                <b className="text-slate-50">{s.name}</b>
                <span className="text-xs text-slate-500">about {s.minutes} min</span>
                <button className="btn btn-go ml-auto h-7 text-xs" onClick={() => start(s.id)}><Play size={12} aria-hidden /> Start</button>
              </div>
              <p className="mt-1 text-sm text-slate-300">{s.about}</p>
              <p className="mt-1 text-xs text-slate-400">Practises: {s.teaches.join('; ')}.</p>
            </li>
          ))}
        </ul>
      </Modal>
      <Modal open={!!score} onClose={() => setScore(null)} title={score ? `${score.name}: ${score.score}%` : ''} icon={<Trophy size={16} className="text-amber-300" aria-hidden />} wide>
        {score && <Checks checks={score.checks} />}
        {score?.reviews.map((r) => (
          <div key={r.incident} className="mt-3">
            <div className="text-sm font-semibold text-slate-200">{r.incident} · {r.type} · quality review {r.score}%</div>
            <Checks checks={r.checks} />
          </div>
        ))}
      </Modal>
    </>
  )
}

/** Checks lists a review's checks, passed, failed or not applicable. */
export function Checks({ checks }: { checks: Check[] }) {
  return (
    <ul className="mt-1 space-y-0.5 text-sm">
      {checks.map((c) => (
        <li key={c.name} className="flex gap-2">
          <span className={cx('w-5 shrink-0 text-center font-bold', c.skip ? 'text-slate-500' : c.ok ? 'text-emerald-400' : 'text-rose-400')} aria-label={c.skip ? 'not applicable' : c.ok ? 'passed' : 'missed'}>{c.skip ? '–' : c.ok ? '✓' : '✗'}</span>
          <span className="text-slate-200">{c.name}</span>
          <span className="ml-auto text-right text-xs text-slate-400">{c.detail}</span>
        </li>
      ))}
    </ul>
  )
}

/** OfflineBar is the console's state when the server cannot be reached, and its offline log. */
export function OfflineBar({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const offline = useStore((s) => s.offline)
  const [items, setItems] = useState(outbox())
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    const on = () => setItems(outbox())
    window.addEventListener('bd:outbox', on)
    window.addEventListener('storage', on)
    return () => {
      window.removeEventListener('bd:outbox', on)
      window.removeEventListener('storage', on)
    }
  }, [])
  if (!offline && items.length === 0) return null
  const sendAll = async () => {
    setBusy(true)
    const r = await send()
    setBusy(false)
    if (r.sent) toast.success(`${r.sent} offline entr${r.sent === 1 ? 'y' : 'ies'} sent`)
    r.failed.forEach((f) => toast.error(`${f.item.kind}: ${f.error}`))
  }
  return (
    <>
      <div role={offline ? 'alert' : 'status'} className={cx('flex flex-wrap items-center gap-2 border-b px-3 py-1.5 text-sm', offline ? 'border-amber-700 bg-amber-950/80 text-amber-50' : 'border-sky-800 bg-sky-950/70 text-sky-50')}>
        <WifiOff size={16} className="shrink-0" aria-hidden />
        {offline ? (
          <span><b>The server cannot be reached.</b> This is the board as it was at {new Date(offline).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}; it does not change. Keep working on paper or in the offline log: it is sent when the server is back.</span>
        ) : (
          <span>{items.length} entr{items.length === 1 ? 'y' : 'ies'} made offline, waiting to be sent.</span>
        )}
        <span className="ml-auto flex gap-1">
          {can.dispatch(me) && <button className="btn h-7 text-xs" onClick={() => setOpen(true)}><NotebookPen size={13} aria-hidden /> Offline log{items.length ? ` (${items.length})` : ''}</button>}
          {!offline && items.length > 0 && <button className="btn btn-go h-7 text-xs" disabled={busy} onClick={sendAll}>Send them now</button>}
        </span>
      </div>
      <OfflineLog open={open} onClose={() => setOpen(false)} view={view} items={items} />
    </>
  )
}

function OfflineLog({ open, onClose, view, items }: { open: boolean; onClose: () => void; view: View; items: OutItem[] }) {
  const me = useStore((s) => s.me)
  const [kind, setKind] = useState<'incident' | 'note' | 'status'>('incident')
  const [type, setType] = useState(view.callTypes[0]?.code ?? 'OTHER')
  const [address, setAddress] = useState('')
  const [text, setText] = useState('')
  const [incident, setIncident] = useState('')
  const [unit, setUnit] = useState('')
  const [status, setStatus] = useState('enroute')
  const by = me?.username ?? '?'
  const add = (e: React.FormEvent) => {
    e.preventDefault()
    if (kind === 'incident' && address.trim()) keep({ kind, by, type, address: address.trim(), text: text.trim() })
    else if (kind === 'note' && incident && text.trim()) keep({ kind, by, incident, text: text.trim() })
    else if (kind === 'status' && unit) keep({ kind, by, unit, status, incident: incident || undefined })
    else return toast.error('Fill in what this entry needs')
    setAddress('')
    setText('')
    toast.success('Kept in the offline log')
  }
  const open_ = view.incidents.filter((i) => i.status !== 'closed')
  return (
    <Modal open={open} onClose={onClose} title="Offline log" icon={<NotebookPen size={16} className="text-amber-300" aria-hidden />} wide>
      <p className="mb-2 text-sm text-slate-400">Kept in this browser and sent, in order and marked as entered offline with the time, when the server is back. Nothing is sent before then.</p>
      <form onSubmit={add} className="grid grid-cols-2 gap-2 rounded-lg border border-slate-800 p-3 text-sm">
        <label className="col-span-2 flex gap-3">
          {(['incident', 'note', 'status'] as const).map((k) => (
            <span key={k} className="flex items-center gap-1"><input type="radio" name="kind" checked={kind === k} onChange={() => setKind(k)} /> {k === 'incident' ? 'New incident' : k === 'note' ? 'Note' : 'Unit status'}</span>
          ))}
        </label>
        {kind === 'incident' && (
          <>
            <label className="flex flex-col gap-1"><span className="text-xs text-slate-400">Call type</span>
              <select className="input" value={type} onChange={(e) => setType(e.target.value)}>{view.callTypes.map((c) => <option key={c.code} value={c.code}>{c.name}</option>)}</select></label>
            <label className="flex flex-col gap-1"><span className="text-xs text-slate-400">Address</span><input className="input" value={address} onChange={(e) => setAddress(e.target.value)} /></label>
          </>
        )}
        {kind !== 'incident' && (
          <label className="flex flex-col gap-1"><span className="text-xs text-slate-400">Incident{kind === 'status' ? ' (optional)' : ''}</span>
            <select className="input" value={incident} onChange={(e) => setIncident(e.target.value)}><option value="">—</option>{open_.map((i) => <option key={i.number} value={i.number}>{i.number} {i.type} {i.location?.address ?? ''}</option>)}</select></label>
        )}
        {kind === 'status' && (
          <>
            <label className="flex flex-col gap-1"><span className="text-xs text-slate-400">Unit</span>
              <select className="input" value={unit} onChange={(e) => setUnit(e.target.value)}><option value="">—</option>{view.units.map((u) => <option key={u.id} value={u.id}>{u.id}</option>)}</select></label>
            <label className="flex flex-col gap-1"><span className="text-xs text-slate-400">Status</span>
              <select className="input" value={status} onChange={(e) => setStatus(e.target.value)}>{view.statuses.map((x) => <option key={x} value={x}>{x}</option>)}</select></label>
          </>
        )}
        {kind !== 'status' && <label className="col-span-2 flex flex-col gap-1"><span className="text-xs text-slate-400">{kind === 'note' ? 'Note' : 'Narrative'}</span><input className="input" value={text} onChange={(e) => setText(e.target.value)} /></label>}
        <button className="btn btn-go col-span-2 justify-self-end">Keep it</button>
      </form>
      <ol className="mt-3 space-y-1 text-sm">
        {items.map((it) => (
          <li key={it.id} className="flex items-center gap-2 rounded border border-slate-800 px-2 py-1">
            <span className="font-mono text-xs text-slate-500">{new Date(it.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
            <span className="text-slate-200">{it.kind === 'incident' ? `New ${it.type} at ${it.address}${it.text ? `: ${it.text}` : ''}` : it.kind === 'note' ? `${it.incident}: ${it.text}` : `${it.unit} ${it.status}${it.incident ? ` on ${it.incident}` : ''}`}</span>
            <button className="btn btn-ghost ml-auto h-6 px-1 text-xs" onClick={() => drop(it.id)} aria-label="Remove this entry">Remove</button>
          </li>
        ))}
      </ol>
    </Modal>
  )
}
