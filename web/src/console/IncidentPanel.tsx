import { useEffect, useMemo, useRef, useState } from 'react'
import IncidentFiles from '../Files'
import ReplayDialog from './Replay'
import { toast } from 'sonner'
import { useNavigate } from 'react-router-dom'
import {
  CheckCircle2, ChevronRight, Crosshair, FileText, Lock, MapPin, MessageSquareText, Phone, RefreshCw, Send, Star, Truck, Video, X, Eye, Image as ImageIcon, Unlock, Undo2,
  Pencil, Play,
} from 'lucide-react'
import { AdvisoryCards, useAdvisory } from './Advisory'
import PremiseBox from './PremiseBox'
import { ViewsButton } from '../views/PlaceViews'
import CallAssist from './CallAssist'
import { SummaryBox, DroneBox } from './Dream'
import { openMarkOut } from './MarkOut'
import { run as runLine } from './UnitBoard'
import { api, errText } from '../api'
import EntryFacts, { AckCell } from './Entry'
import { dispatchUnits } from './ask'
import { can, store, useStore } from '../store'
import { Field, Modal, Priority, StatusChip, Tip } from '../ui'
import { activity, checkDue, ago, clock, cx, hasPoint, hhmm, mi, locText, locationStale, mins, ms, overdue, phone, prio, statusMeta, typeName, uncertainty, useNow, avl } from '../util'
import type { Call, Incident, Recommendation, View } from '../types'

const NEXT: Record<string, { to: string; label: string }[]> = {
  dispatched: [{ to: 'enroute', label: 'ER' }, { to: 'onscene', label: 'OS' }, { to: 'available', label: 'AV' }],
  enroute: [{ to: 'onscene', label: 'OS' }, { to: 'available', label: 'AV' }],
  onscene: [{ to: 'transporting', label: 'TR' }, { to: 'available', label: 'AV' }],
  transporting: [{ to: 'atdestination', label: 'AD' }, { to: 'available', label: 'AV' }],
  atdestination: [{ to: 'available', label: 'AV' }],
}

async function setStatus(unit: string, status: string, incident?: string) {
  try {
    await api.post(`/units/${unit}/status`, { status, incident })
  } catch (e) {
    toast.error(errText(e))
  }
}

// WHEP viewer for a caller's live video: opens blurred and muted, the dispatcher chooses to reveal.
function CallerVideo({ call }: { call: Call }) {
  const ref = useRef<HTMLVideoElement>(null)
  const pc = useRef<RTCPeerConnection | null>(null)
  const [state, setState] = useState<'idle' | 'connecting' | 'playing' | 'failed'>('idle')
  const [reveal, setReveal] = useState(false)
  const [err, setErr] = useState('')
  const start = async () => {
    setState('connecting')
    setErr('')
    try {
      const p = new RTCPeerConnection()
      pc.current = p
      p.addTransceiver('video', { direction: 'recvonly' })
      p.addTransceiver('audio', { direction: 'recvonly' })
      p.ontrack = (ev) => {
        if (ref.current && ev.streams[0]) ref.current.srcObject = ev.streams[0]
        setState('playing')
      }
      await p.setLocalDescription(await p.createOffer())
      await new Promise<void>((res) => {
        if (p.iceGatheringState === 'complete') return res()
        const t = setTimeout(res, 2000)
        p.onicegatheringstatechange = () => p.iceGatheringState === 'complete' && (clearTimeout(t), res())
      })
      const r = await fetch(`./api/calls/${encodeURIComponent(call.id)}/whep`, {
        method: 'POST', headers: { 'Content-Type': 'application/sdp', 'X-BitDispatcher': '1' }, body: p.localDescription!.sdp,
      })
      if (!r.ok) throw new Error((await r.json().catch(() => ({}))).message ?? r.statusText)
      await p.setRemoteDescription({ type: 'answer', sdp: await r.text() })
    } catch (e) {
      setState('failed')
      setErr(errText(e))
    }
  }
  useEffect(() => () => {
    pc.current?.close()
  }, [])
  return (
    <div className="rounded-md border border-slate-800 bg-slate-950 p-1.5">
      <div className="mb-1 flex items-center gap-1.5 text-[11px]">
        <Video size={12} className={call.video?.live ? 'text-rose-300' : 'text-slate-400'} aria-hidden />
        <span className="font-semibold text-slate-200">Caller video</span>
        <span className={cx('chip', call.video?.live ? 'bg-rose-500 text-white' : 'bg-slate-800 text-slate-400')}>{call.video?.live ? 'LIVE' : 'not streaming'}</span>
        {state === 'idle' && <button className="btn ml-auto h-6 px-2 py-0" onClick={start}>Watch</button>}
        {state === 'playing' && (
          <button className="btn ml-auto h-6 px-2 py-0" onClick={() => setReveal(!reveal)}>
            <Eye size={12} aria-hidden /> {reveal ? 'Blur' : 'Reveal'}
          </button>
        )}
      </div>
      {state !== 'idle' && (
        <div className="relative aspect-video overflow-hidden rounded bg-black">
          <video ref={ref} autoPlay playsInline muted={!reveal} className={cx('h-full w-full object-contain transition', !reveal && 'scale-105 blur-xl')} />
          {!reveal && state === 'playing' && <div className="absolute inset-0 grid place-items-center text-xs font-semibold text-white/90">Blurred and muted · Reveal to view</div>}
          {state === 'connecting' && <div className="absolute inset-0 grid place-items-center text-xs text-slate-300">Connecting…</div>}
          {state === 'failed' && <div className="absolute inset-0 grid place-items-center p-2 text-center text-xs text-rose-300">{err}</div>}
        </div>
      )}
    </div>
  )
}

export function CallBox({ call, view, now }: { call: Call; view: View; now: number }) {
  const me = useStore((s) => s.me)
  const [text, setText] = useState('')
  const [busy, setBusy] = useState('')
  const live = call.state !== 'ended'
  const fromBV = call.sources.includes('bitvoice') || call.sources.includes('eido')
  const bvOK = view.health.bitvoice.configured
  const act = async (name: string, path: string, body?: unknown) => {
    setBusy(name)
    try {
      const r = await api.post<{ detail?: string }>(`/calls/${encodeURIComponent(call.id)}/${path}`, body)
      toast.success(`${name}: ${r?.detail || 'done'}`)
    } catch (e) {
      toast.error(`${name}: ${errText(e)}`)
    } finally {
      setBusy('')
    }
  }
  const photos = (call.media ?? []).map((m, n) => ({ m, n })).filter(({ m }) => m.purpose === 'caller-photo' || m.purpose === 'caller-video')
  const stale = locationStale(view, call.location, now)
  return (
    <div className="space-y-1.5 rounded-md border border-sky-900/60 bg-sky-950/20 p-2">
      <div className="flex items-center gap-1.5 text-xs">
        <Phone size={13} className="text-sky-300" aria-hidden />
        <span className="font-mono font-bold text-slate-50">{phone(call.callback)}</span>
        {call.class && <span className="chip bg-slate-800 font-mono text-slate-300">{call.class}</span>}
        <span className={cx('chip uppercase', call.state === 'ended' ? 'bg-slate-700 text-slate-300' : 'bg-emerald-500/20 text-emerald-300')}>{call.state}</span>
        <span className="font-mono text-slate-400 tnum">{clock(call.start, call.end ? ms(call.end) : now)}</span>
        <span className="ml-auto truncate font-mono text-[10px] text-slate-500">{call.sources.join('+')}{call.agent ? ` · taker ${call.agent}` : ''}</span>
      </div>
      {call.callerName && <div className="text-xs text-slate-300">{call.callerName}{call.provider ? <span className="text-slate-500"> · {call.provider}</span> : null}</div>}
      <div className="flex items-start gap-1 text-xs">
        <MapPin size={12} className={cx('mt-0.5 shrink-0', stale ? 'text-amber-300' : 'text-emerald-300')} aria-hidden />
        <div className="min-w-0">
          <div className="text-slate-100">{locText(call.location)}</div>
          <div className="font-mono text-[10px] text-slate-400">
            {uncertainty(call.location)}
            {call.location?.source && ` · ${call.location.source}`}
            {call.location?.at && ` · ${ago(call.location.at, now)} old`}
            {stale && <span className="ml-1 font-bold text-amber-300">STALE</span>}
          </div>
        </div>
      </div>
      {(call.problems ?? []).map((p) => (
        <div key={p} className="text-[11px] text-amber-300">⚠ {p}</div>
      ))}
      <CallAssist call={call} view={view} />
      {can.dispatch(me) && live && fromBV && (
        <div className="flex flex-wrap gap-1">
          <button className="btn h-6 px-2 py-0 text-[11px]" disabled={!bvOK || !!busy || !call.canRebid} onClick={() => act('Rebid', 'rebid')} title={call.canRebid ? 'Ask for the caller’s location again' : 'This call has no location reference to rebid'}>
            <Crosshair size={12} aria-hidden /> {busy === 'Rebid' ? 'Rebidding…' : 'Rebid'}
          </button>
          <button className="btn h-6 px-2 py-0 text-[11px]" disabled={!bvOK || !!busy} onClick={() => act('Video link', 'video-link')}>
            <Video size={12} aria-hidden /> Send video link
          </button>
          <button className="btn h-6 px-2 py-0 text-[11px]" disabled={!bvOK || !!busy} onClick={() => act('EIDO', 'eido')}>
            <RefreshCw size={12} aria-hidden /> EIDO
          </button>
        </div>
      )}
      {call.video?.viewUrl && <CallerVideo call={call} />}
      {photos.length > 0 && (
        <div className="flex gap-1 overflow-x-auto">
          {photos.map(({ m, n }) => (
            <a key={m.id} href={`./api/calls/${encodeURIComponent(call.id)}/media/${n}`} target="_blank" rel="noreferrer" className="group relative h-14 w-20 shrink-0 overflow-hidden rounded border border-slate-700 bg-slate-950" title="Open caller media (blurred here)">
              {m.purpose === 'caller-photo' ? (
                <img src={`./api/calls/${encodeURIComponent(call.id)}/media/${n}`} alt="Caller photo" className="h-full w-full object-cover blur-md group-hover:blur-none" />
              ) : (
                <span className="grid h-full place-items-center text-[10px] text-slate-400"><ImageIcon size={14} aria-hidden /> clip</span>
              )}
            </a>
          ))}
        </div>
      )}
      {(call.text?.length || call.textOnly) && (
        <div className="rounded border border-violet-900/60 bg-violet-950/20 p-1.5">
          <div className="mb-1 flex items-center gap-1 text-[11px] font-semibold text-violet-300">
            <MessageSquareText size={12} aria-hidden /> Text with caller {call.textOnly && <span className="chip bg-violet-500 text-white">TEXT ONLY</span>}
          </div>
          <ul className="max-h-28 space-y-0.5 overflow-y-auto text-xs">
            {(call.text ?? []).map((l, i) => (
              <li key={i} className={cx(l.from === 'caller' ? 'text-slate-100' : 'text-sky-300')}>
                <span className="font-mono text-[10px] text-slate-500">{hhmm(l.at)} {l.via} </span>
                <span className="font-semibold">{l.from === 'caller' ? 'Caller' : l.name || 'Taker'}:</span> {l.text}
              </li>
            ))}
          </ul>
          {can.dispatch(me) && live && bvOK && (
            <form className="mt-1 flex gap-1" onSubmit={async (e) => { e.preventDefault(); if (!text.trim()) return; await act('Text', 'text', { text }); setText('') }}>
              <input className="input h-7 py-0 text-xs" value={text} onChange={(e) => setText(e.target.value)} placeholder="Text the caller…" aria-label="Text the caller" maxLength={1000} />
              <button className="btn h-7" disabled={!text.trim() || !!busy} aria-label="Send text"><Send size={12} aria-hidden /></button>
            </form>
          )}
        </div>
      )}
      {(call.notes ?? []).length > 0 && (
        <div className="text-[11px] text-slate-400">
          {call.notes!.map((n, i) => <div key={i}>{call.alert ? `${call.alert.sender}: ` : "Call-taker note: "}<span className="text-slate-200">{n}</span></div>)}
        </div>
      )}
    </div>
  )
}

function RecommendBox({ inc, rec, reload }: { inc: Incident; rec: Recommendation | null; reload: () => void }) {
  const me = useStore((s) => s.me)
  const pick = useStore((s) => s.pick)
  const [busy, setBusy] = useState(false)
  const dispatch = async () => {
    if (!pick.length) return
    setBusy(true)
    try {
      if (await dispatchUnits(inc.number, pick, rec?.units ?? [])) {
        store.setPick([])
        reload()
      }
    } finally {
      setBusy(false)
    }
  }
  // What changed since the last recommendation: units in and out, marked so the dispatcher sees it.
  const prev = useRef<{ inc: string; units: string[] } | null>(null)
  const [diff, setDiff] = useState<{ added: string[]; gone: string[] } | null>(null)
  useEffect(() => {
    if (!rec) return
    const p = prev.current
    if (p && p.inc === inc.number) {
      const added = rec.units.filter((u) => !p.units.includes(u))
      const gone = p.units.filter((u) => !rec.units.includes(u) && !inc.units.some((a) => a.unit === u && !a.cleared))
      if (added.length || gone.length) setDiff({ added, gone })
    } else setDiff(null)
    prev.current = { inc: inc.number, units: rec.units }
  }, [rec, inc.number, inc.units])
  if (!rec) return <div className="px-2 py-1 text-xs text-slate-500">Working out the recommendation…</div>
  const allRec = rec.units
  const same = pick.length === allRec.length && pick.every((u) => allRec.includes(u))
  return (
    <section aria-label="Recommendation" className="rounded-md border border-lime-900/50 bg-lime-950/10">
      <div className="flex items-center gap-1.5 border-b border-slate-800 px-2 py-1">
        <Star size={12} className="text-lime-300" aria-hidden />
        <span className="text-[11px] font-bold uppercase tracking-wider text-lime-300">Recommended</span>
        <Tip text={rec.estimate}>
          {rec.lines.some((l) => l.units.some((c) => c.routed)) ? (
            <span className="cursor-help text-[10px] text-sky-300 underline decoration-dotted">by road</span>
          ) : (
            <span className="cursor-help text-[10px] text-slate-500 underline decoration-dotted">estimated</span>
          )}
        </Tip>
        {can.dispatch(me) && (
          <div className="ml-auto flex gap-1">
            {!same && allRec.length > 0 && <button className="btn h-6 px-2 py-0 text-[11px]" onClick={() => store.setPick(allRec)}><Undo2 size={11} aria-hidden /> Use recommendation</button>}
            <button className="btn btn-go h-6 px-2 py-0 text-[11px]" disabled={!pick.length || busy} onClick={dispatch}>
              <Truck size={12} aria-hidden /> Dispatch {pick.length || ''} <span className="kbd h-4 border-white/40 bg-transparent text-white">Ctrl ↵</span>
            </button>
          </div>
        )}
      </div>
      {(rec.warnings ?? []).length > 0 && (
        <div className="border-b border-slate-800 bg-amber-500/10 px-2 py-1 text-[11px] font-semibold text-amber-200" role="alert">
          {rec.warnings.map((w) => <div key={w}>⚠ {w} — you will be asked to confirm</div>)}
        </div>
      )}
      {rec.plan && (
        <div className="border-b border-slate-800 px-2 py-1 text-[11px]">
          <span className="text-slate-400">Plan</span> <b className="text-slate-100">{rec.plan}</b>
          <span className="ml-1.5 inline-flex flex-wrap gap-1 align-middle">
            {(rec.planned ?? []).map((p) => (
              <span key={p.unit} className={cx('chip font-mono', p.can ? 'bg-lime-500/15 text-lime-200' : 'bg-slate-800 text-slate-500 line-through')} title={p.why ? `${p.unit}: ${p.why}` : `${p.unit}: ${p.due} due`}>{p.due}·{p.unit}</span>
            ))}
          </span>
        </div>
      )}
      {diff && (
        <div className="border-b border-slate-800 bg-sky-500/10 px-2 py-0.5 text-[10px] text-sky-200" role="status">
          Changed: {diff.added.length > 0 && <>now <b className="font-mono">{diff.added.join(' ')}</b></>} {diff.gone.length > 0 && <>no longer <span className="font-mono line-through">{diff.gone.join(' ')}</span></>}
          <button className="ml-1 underline" onClick={() => setDiff(null)}>seen</button>
        </div>
      )}
      <ul className="divide-y divide-slate-800/70 text-xs">
        {rec.lines.map((l) => (
          <li key={l.capability} className="px-2 py-1">
            <div className="flex items-center gap-1.5 text-[11px]">
              <span className="font-semibold uppercase tracking-wide text-slate-300">{l.count}× {l.capability}</span>
              {l.covered.length > 0 && <span className="text-slate-500">on scene: <span className="font-mono text-slate-300">{l.covered.join(' ')}</span></span>}
              {l.short > 0 && <span className="chip bg-rose-500/20 text-rose-300">SHORT {l.short}</span>}
            </div>
            <div className="mt-0.5 flex flex-wrap gap-1">
              {[...l.units.map((c) => ({ c, next: false })), ...l.next.map((c) => ({ c, next: true }))].map(({ c, next }) => {
                const on = pick.includes(c.unit)
                return (
                  <Tip key={c.unit} text={<ul>{c.reasons.map((r) => <li key={r}>{r}</li>)}</ul>}>
                    <button
                      type="button"
                      disabled={!can.dispatch(me)}
                      onClick={() => store.togglePick(c.unit)}
                      aria-pressed={on}
                      className={cx(
                        'inline-flex items-center gap-1 rounded border px-1.5 py-0.5 font-mono text-[11px]',
                        on ? 'border-rose-400 bg-rose-500/20 text-rose-100' : next ? 'border-dashed border-slate-700 text-slate-400 hover:border-slate-500' : 'border-lime-700/70 text-lime-200 hover:border-lime-500',
                      )}
                    >
                      <span className="font-bold">{c.unit}</span>
                      <span className={cx('text-[10px] opacity-80', !c.routed && c.seconds ? 'italic' : '')}>{c.routed ? '' : c.seconds ? '~' : ''}{mins(c.seconds)}{c.metres ? ` ${mi(c.metres)}` : ''}</span>
                      {c.from !== 'avl' && <span className={cx('text-[9px] uppercase', c.from === 'station' ? 'text-slate-500' : 'text-amber-300')}>{c.from === 'station' ? 'stn' : c.from === 'stale-avl' ? 'stale' : '?'}</span>}
                    </button>
                  </Tip>
                )
              })}
              {l.short > 0 && l.units.length === 0 && l.next.length === 0 && <span className="text-[11px] text-rose-300">none available</span>}
            </div>
          </li>
        ))}
      </ul>
      {rec.notes.length > 0 && (
        <ul className="border-t border-slate-800 px-2 py-1 text-[11px] text-amber-300">
          {rec.notes.map((n) => <li key={n}>• {n}</li>)}
        </ul>
      )}
      {pick.some((u) => !allRec.includes(u)) && <div className="border-t border-slate-800 px-2 py-1 text-[10px] text-slate-400">Your choice differs from the recommendation: you will be asked why, and the ledger records both.</div>}
    </section>
  )
}

function UnitsOn({ inc, view, now }: { inc: Incident; view: View; now: number }) {
  const me = useStore((s) => s.me)
  if (inc.units.length === 0) return null
  return (
    <section aria-label="Units on the incident">
      <table className="w-full text-xs">
        <thead>
          <tr className="text-left text-[10px] uppercase tracking-wider text-slate-500">
            <th className="px-1 font-semibold">Unit</th><th className="font-semibold">Status</th><th className="font-semibold">In status</th>
            <th className="font-semibold">DP</th><th className="font-semibold">ACK</th><th className="font-semibold">ER</th><th className="font-semibold">OS</th><th />
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-800/60">
          {inc.units.map((a, idx) => {
            const u = view.units.find((x) => x.id === a.unit)
            const cleared = !!a.cleared
            const over = u && !cleared ? overdue(view, u, now) : 0
            return (
              <tr key={a.unit + idx} className={cx(cleared && 'opacity-45', over > 0 && 'bg-amber-500/10')}>
                <td className="px-1 py-0.5 font-mono font-bold text-slate-100">
                  {a.unit}{a.recommended ? '' : <span className="ml-0.5 text-[9px] text-slate-500" title="Not from the recommendation">*</span>}
                  {u?.emergency && <span className="chip flash ml-1 text-white">EMER</span>}
                </td>
                <td>{cleared ? <span className="text-[10px] text-slate-500">cleared {hhmm(a.cleared)}</span> : <StatusChip status={u?.status ?? a.status} />}</td>
                <td className={cx('font-mono tnum', over > 0 ? 'font-bold text-amber-300' : 'text-slate-300')}>{!cleared && u ? clock(u.statusAt, now) : ''}</td>
                <td className="font-mono text-[10px] text-slate-400">{hhmm(a.dispatched).slice(0, 5)}</td>
                <td>{!cleared && <AckCell inc={inc.number} unit={a.unit} status={u?.status ?? a.status} dispatched={a.dispatched} acked={a.acked || a.enroute || a.onScene} ackSeconds={view.settings.ackSeconds || 60} now={now} />}</td>
                <td className="font-mono text-[10px] text-slate-400">{a.enroute ? hhmm(a.enroute).slice(0, 5) : ''}</td>
                <td className="font-mono text-[10px] text-slate-400">{a.onScene ? hhmm(a.onScene).slice(0, 5) : ''}</td>
                <td className="text-right">
                  {!cleared && can.dispatch(me) && u && (NEXT[u.status] ?? []).map((n) => (
                    <button key={n.to} className={cx('btn ml-0.5 h-5 px-1 py-0 font-mono text-[10px]', statusMeta(n.to).fg)} onClick={() => setStatus(a.unit, n.to)} title={statusMeta(n.to).label}>
                      {n.label}
                    </button>
                  ))}
                </td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </section>
  )
}

function Timeline({ inc, now }: { inc: Incident; now: number }) {
  const me = useStore((s) => s.me)
  const [text, setText] = useState('')
  const [priority, setPriority] = useState(false)
  const end = useRef<HTMLDivElement>(null)
  useEffect(() => {
    end.current?.scrollIntoView({ block: 'end' })
  }, [inc.timeline.length])
  const add = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!text.trim()) return
    try {
      await api.post(`/incidents/${inc.number}/notes`, { text, priority })
      setText('')
      setPriority(false)
    } catch (err) {
      toast.error(errText(err))
    }
  }
  const kindColor: Record<string, string> = {
    created: 'text-sky-300', dispatch: 'text-orange-300', status: 'text-slate-300', note: 'text-slate-100', 'priority-note': 'text-rose-200 font-semibold',
    change: 'text-violet-300', call: 'text-sky-300', close: 'text-emerald-300', reopen: 'text-amber-300', 'auto-dispatch': 'text-amber-200', correction: 'text-violet-200', shared: 'text-sky-200', protocol: 'text-teal-200', file: 'text-slate-300', emergency: 'text-rose-300 font-bold',
    radio: 'text-teal-200 italic', drone: 'text-cyan-300',
  }
  return (
    <section className="flex min-h-0 flex-1 flex-col" aria-label="Timeline">
      <div className="panel-head -mx-2 border-t">
        <FileText size={12} aria-hidden /> Narrative and timeline <span className="font-mono text-slate-500">{inc.timeline.length}</span>
      </div>
      <ol className="min-h-0 flex-1 space-y-0.5 overflow-y-auto py-1 text-xs">
        {inc.timeline.map((t) => (
          <li key={t.seq + t.kind + t.text} className={cx('flex gap-1.5', t.kind === 'priority-note' && 'rounded bg-rose-500/10 px-1')}>
            <span className="shrink-0 font-mono text-[10px] text-slate-500 tnum" title={new Date(t.at).toLocaleString()}>{hhmm(t.at)}</span>
            <span className="w-14 shrink-0 truncate font-mono text-[10px] text-slate-500" title={t.actor}>{t.actor}</span>
            <span className={cx('min-w-0 break-words', kindColor[t.kind] ?? 'text-slate-300')}>{t.text}</span>
          </li>
        ))}
        <div ref={end} />
      </ol>
      {can.dispatch(me) && (
        <form onSubmit={add} className="flex shrink-0 gap-1 pt-1">
          <input className="input h-7 py-0 text-xs" value={text} onChange={(e) => setText(e.target.value)} placeholder={`Note to ${inc.number}…`} aria-label="Add a note" maxLength={4000} />
          <button type="button" className={cx('btn h-7 px-1.5', priority && 'border-rose-400 bg-rose-500/20 text-rose-200')} aria-pressed={priority} onClick={() => setPriority(!priority)} title="Priority note: every dispatcher and unit sees it">
            !
          </button>
          <button className="btn h-7" disabled={!text.trim()} aria-label="Add note"><Send size={12} aria-hidden /></button>
        </form>
      )}
      <span className="sr-only">{ago(inc.updated, now)}</span>
    </section>
  )
}

export default function IncidentPanel({ view, inc, rec, reloadRec }: { view: View; inc: Incident; rec: Recommendation | null; reloadRec: () => void }) {
  const now = useNow()
  const me = useStore((s) => s.me)
  const [closing, setClosing] = useState(false)
  const [disp, setDisp] = useState('')
  const [closeText, setCloseText] = useState('')
  const [reopenWhy, setReopenWhy] = useState('')
  const [editType, setEditType] = useState(false)
  const [editPri, setEditPri] = useState(false)
  const adv = useAdvisory(view.seq)
  const calls = useMemo(() => inc.calls.map((id) => view.calls.find((c) => c.id === id)).filter(Boolean) as Call[], [inc.calls, view.calls])
  const live = inc.units.filter((a) => !a.cleared)
  const firstOS = inc.units.map((a) => ms(a.onScene)).filter(Boolean).sort()[0]
  const firstDP = inc.units.map((a) => ms(a.dispatched)).filter(Boolean).sort()[0]
  const change = async (body: Record<string, unknown>) => {
    try {
      await api.patch(`/incidents/${inc.number}`, body)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const closed = inc.status === 'closed'
  return (
    <aside className="panel flex min-h-0 flex-1 flex-col" aria-label={`Incident ${inc.number}`}>
      <div className="panel-head">
        <span className="font-mono text-slate-200">{inc.number}</span>
        <span className={cx('chip normal-case', inc.status === 'pending' ? 'bg-amber-500/20 text-amber-300' : inc.status === 'active' ? 'bg-orange-500/20 text-orange-300' : 'bg-slate-700 text-slate-300')}>{inc.status}</span>
        <span className="font-mono normal-case tracking-normal text-slate-400 tnum">{clock(inc.created, closed ? ms(inc.closed) : now)}</span>
        <button className="btn btn-ghost ml-auto h-5 px-1" onClick={() => store.select(null)} aria-label="Close incident panel (Esc)"><X size={13} aria-hidden /></button>
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto p-2">
        {adv && <AdvisoryCards list={adv.suggestions} status={adv.status} incident={inc.number} />}
        <div>
          <div className="flex items-center gap-2">
            {editPri ? (
              <span className="flex gap-0.5" role="group" aria-label="Priority">
                {[1, 2, 3, 4, 5].map((p) => (
                  <button key={p} className={cx('h-7 w-7 rounded font-mono text-sm font-bold', prio(p).bg, prio(p).fg, p !== inc.priority && 'opacity-50 hover:opacity-100')}
                    onClick={() => { if (p !== inc.priority) change({ priority: p }); setEditPri(false) }} autoFocus={p === inc.priority}>{p}</button>
                ))}
              </span>
            ) : <Priority p={inc.priority} onClick={can.dispatch(me) && !closed ? () => setEditPri(true) : undefined} />}
            {editType && can.dispatch(me) ? (
              <select className="input h-7 py-0 text-sm" autoFocus defaultValue={inc.type} onBlur={() => setEditType(false)} onChange={(e) => { change({ type: e.target.value }); setEditType(false) }}>
                {view.callTypes.map((c) => <option key={c.code} value={c.code}>{c.name} ({c.code})</option>)}
              </select>
            ) : (
              <button className="truncate text-left text-base font-bold text-slate-50 hover:underline" onClick={() => !closed && setEditType(true)} title="Change call type">
                {typeName(view, inc.type)} <span className="font-mono text-xs text-slate-500">{inc.type}</span>
              </button>
            )}
          </div>
          <div className="mt-1 flex items-start gap-1 text-xs">
            <MapPin size={12} className={cx('mt-0.5 shrink-0', hasPoint(inc.location) ? 'text-slate-300' : 'text-amber-300')} aria-hidden />
            <div>
              <div className="text-slate-100">{locText(inc.location)}</div>
              <div className="font-mono text-[10px] text-slate-500">
                {hasPoint(inc.location) ? `${inc.location.lat.toFixed(5)}, ${inc.location.lon.toFixed(5)}` : 'not on the map'}
                {inc.location?.radius ? ` · ±${Math.round(inc.location.radius)} m` : ''}
                {inc.location?.source ? ` · ${inc.location.source}` : ''}
              </div>
            </div>
            {hasPoint(inc.location) && (view.placeViews || view.premises?.[inc.number]?.some((p) => p.photos?.length)) && (
              <ViewsButton lat={inc.location.lat} lon={inc.location.lon} premises={view.premises?.[inc.number]} className="ml-auto shrink-0" />
            )}
          </div>
          {inc.caller && (inc.caller.name || inc.caller.callback) && (
            <div className="mt-0.5 flex items-center gap-1 text-xs text-slate-300">
              <Phone size={11} aria-hidden /> {inc.caller.name} <span className="font-mono">{phone(inc.caller.callback)}</span>
            </div>
          )}
          {/* Response benchmarks, measured from the ledger's times. */}
          <div className="mt-1 flex flex-wrap gap-2 font-mono text-[10px] text-slate-400">
            <span>created {hhmm(inc.created)} by {inc.createdBy}</span>
            {firstDP ? <span>→ 1st dispatch <span className="text-slate-200">{clock(ms(inc.created), firstDP)}</span></span> : null}
            {firstOS ? <span>→ 1st on scene <span className="text-slate-200">{clock(ms(inc.created), firstOS)}</span></span> : null}
            {closed && <span className="text-emerald-300">closed {hhmm(inc.closed)} · {inc.disposition}</span>}
            {inc.neris && <span className={inc.neris.ok ? 'text-sky-300' : 'text-rose-300'} title={inc.neris.error || `NERIS ${inc.neris.id ?? ''}`}>NERIS {inc.neris.ok ? 'sent' : 'refused'} {hhmm(inc.neris.at)}</span>}
            {closed && can.supervise(me) && view.health.neris && (
              <button className="underline hover:text-slate-200" onClick={async () => {
                try { const r = await api.post<{ ok: boolean; error?: string }>(`/incidents/${inc.number}/neris`); r.ok ? toast.success('Sent to NERIS') : toast.error(r.error ?? 'NERIS refused it') } catch (e) { toast.error(errText(e)) }
              }}>{inc.neris ? 'Send to NERIS again' : 'Send to NERIS'}</button>
            )}
          </div>
        </div>
        <EntryFacts inc={inc} view={view} />
        {!closed && view.summaries?.[inc.number] && <SummaryBox inc={inc} text={view.summaries[inc.number]} />}
        <PremiseBox premises={view.premises?.[inc.number]} prior={view.prior?.[inc.number]} />
        <DroneBox inc={inc} view={view} />
        {calls.map((c) => <CallBox key={c.id} call={c} view={view} now={now} />)}
        {!closed && <RecommendBox inc={inc} rec={rec} reload={reloadRec} />}
        <UnitsOn inc={inc} view={view} now={now} />
        <IncidentFiles inc={inc} />
        {!closed && can.dispatch(me) && <ShareBox inc={inc} />}
        <Timeline inc={inc} now={now} />
      </div>
      {can.dispatch(me) && (
        <div className="flex shrink-0 items-center gap-1 border-t border-slate-800 p-1.5">
          {!closed ? (
            <>
              <span className="text-[10px] text-slate-500">{live.length} unit{live.length === 1 ? '' : 's'} on it</span>
              <ReportButton inc={inc} />
              <button className="btn h-7" onClick={() => { setDisp(view.settings.dispositions[0]); setClosing(true) }}>
                <Lock size={12} aria-hidden /> Close <span className="kbd h-4">CL</span>
              </button>
            </>
          ) : can.supervise(me) ? (
            <div className="flex w-full gap-1">
            <CorrectButton inc={inc} view={view} />
            <ReplayButton number={inc.number} view={view} />
            <form className="flex flex-1 gap-1" onSubmit={async (e) => { e.preventDefault(); try { await api.post(`/incidents/${inc.number}/reopen`, { reason: reopenWhy }); setReopenWhy('') } catch (err) { toast.error(errText(err)) } }}>
              <input className="input h-7 py-0 text-xs" placeholder="Why reopen?" value={reopenWhy} onChange={(e) => setReopenWhy(e.target.value)} aria-label="Reason to reopen" />
              <button className="btn h-7" disabled={!reopenWhy.trim()}><Unlock size={12} aria-hidden /> Reopen</button>
            </form>
            </div>
          ) : (
            <span className="text-[11px] text-slate-500">Closed. A supervisor can reopen it.</span>
          )}
        </div>
      )}
      <Modal
        open={closing}
        onClose={() => setClosing(false)}
        title={`Close ${inc.number}`}
        icon={<CheckCircle2 size={16} className="text-emerald-300" aria-hidden />}
        footer={
          <>
            <button className="btn" onClick={() => setClosing(false)}>Cancel</button>
            <button
              className="btn btn-go"
              onClick={async () => {
                try {
                  await api.post(`/incidents/${inc.number}/close`, { disposition: disp, text: closeText })
                  setClosing(false)
                  setCloseText('')
                  store.select(null)
                  toast.success(`${inc.number} closed`)
                } catch (e) {
                  toast.error(errText(e))
                }
              }}
            >
              Close incident
            </button>
          </>
        }
      >
        <div className="space-y-3">
          {live.length > 0 && <p className="rounded bg-amber-500/10 px-2 py-1 text-xs text-amber-300">{live.map((a) => a.unit).join(', ')} will be cleared to available.</p>}
          <div className="grid grid-cols-3 gap-1">
            {view.settings.dispositions.map((d) => (
              <button key={d} type="button" className={cx('btn justify-start', disp === d && 'border-emerald-400 bg-emerald-500/15 text-emerald-200')} onClick={() => setDisp(d)} aria-pressed={disp === d}>
                {d}
              </button>
            ))}
          </div>
          <textarea className="input" rows={3} placeholder="Closing note (optional)" value={closeText} onChange={(e) => setCloseText(e.target.value)} aria-label="Closing note" />
        </div>
      </Modal>
    </aside>
  )
}

export function UnitDetail({ view, id }: { view: View; id: string }) {
  const now = useNow()
  const me = useStore((s) => s.me)
  const u = view.units.find((x) => x.id === id)
  if (!u) return null
  const st = view.stations.find((s) => s.id === u.station)
  const state = avl(view, u, now)
  return (
    <div className="space-y-1.5 rounded-md border border-slate-800 p-2 text-xs">
      <div className="flex items-center gap-1.5">
        <span className="font-mono text-base font-black text-slate-50">{u.id}</span>
        <StatusChip status={u.status} big />
        <span className="text-slate-400">{view.settings.unitTypes.find((t) => t.code === u.type)?.name ?? u.type}</span>
        <button className="btn btn-ghost ml-auto h-5 px-1" onClick={() => store.selectUnit(null)} aria-label="Close unit"><X size={12} aria-hidden /></button>
      </div>
      <div className="grid grid-cols-2 gap-x-2 gap-y-0.5 text-[11px]">
        <span className="text-slate-500">In status</span><span className="font-mono text-slate-200">{clock(u.statusAt, now)}</span>
        {u.status === 'markout' ? (
          <>
            <span className="text-slate-500">Marked out</span><span className="font-semibold text-teal-300">{activity(view, u)}{u.markOut?.note ? ` · ${u.markOut.note}` : ''}</span>
            <span className="text-slate-500">Where</span><span className="text-slate-200">{u.markOut?.location?.address || (u.markOut?.location ? 'its AVL position' : 'not given')}</span>
          </>
        ) : (
          <><span className="text-slate-500">Incident</span>{u.incident ? <button className="text-left font-mono text-sky-300 underline" onClick={() => store.select(u.incident!)}>{u.incident}</button> : <span className="text-slate-400">none</span>}</>
        )}
        {checkDue(view, u) > 0 && (
          <>
            <span className="text-slate-500">Status check</span>
            <span className={cx('font-mono', now > checkDue(view, u) ? 'font-bold text-amber-300' : 'text-slate-200')}>
              {now > checkDue(view, u) ? `due ${clock(checkDue(view, u), now)} ago` : `in ${clock(now, checkDue(view, u))}`}{u.checked ? ` · last ${ago(u.checked, now)} ago by ${u.checkedBy}` : ''}
            </span>
          </>
        )}
        <span className="text-slate-500">Station</span><span className="text-slate-200">{st?.name ?? 'none'}</span>
        <span className="text-slate-500">AVL</span>
        <span className={cx(state === 'fresh' ? 'text-emerald-300' : state === 'stale' ? 'text-amber-300' : 'text-slate-500')}>
          {u.position ? `${ago(u.position.at, now)} ago · ${u.position.source}${u.position.speed ? ` · ${Math.round(u.position.speed)} km/h` : ''}` : 'no position'}
        </span>
        <span className="text-slate-500">Radio</span><span className="text-slate-200">{u.radioId ? `ID ${u.radioId}` : 'not mapped'}{u.radioAlias ? ` · ${u.radioAlias}` : ''}{u.lastHeard ? ` · heard ${ago(u.lastHeard, now)} ago` : ''}</span>
        <span className="text-slate-500">Can do</span><span className="text-slate-300">{u.caps.join(', ')}</span>
      </div>
      {can.dispatch(me) && (
        <div className="flex flex-wrap gap-1">
          {view.statuses.filter((s) => s !== u.status && s !== 'markout' && (u.incident || !['dispatched', 'enroute', 'onscene', 'transporting', 'atdestination'].includes(s))).map((s) => (
            <button key={s} className={cx('btn h-6 px-1.5 py-0 font-mono text-[10px]', statusMeta(s).fg)} onClick={() => setStatus(u.id, s)}>
              {statusMeta(s).code} <ChevronRight size={10} aria-hidden />
            </button>
          ))}
          <button className="btn h-6 px-1.5 py-0 font-mono text-[10px] text-teal-300" onClick={() => openMarkOut(u.id)} title="Mark out: meal, court, fuel…">MO…</button>
          {checkDue(view, u) > 0 && <button className="btn h-6 px-1.5 py-0 text-[10px] text-emerald-300" onClick={() => runLine(`SC ${u.id}`)} title="Status check: all right, its timer starts again">Check OK</button>}
        </div>
      )}
    </div>
  )
}

/** Opens the incident's report, or starts one with the incident's location and time. */
function ReportButton({ inc }: { inc: Incident }) {
  const nav = useNavigate()
  const n = inc.reports?.[0]
  return (
    <button className="btn ml-auto h-7" title={n ? `Open report ${n}` : 'Write the incident report'} onClick={async () => {
      if (n) return nav(`/rms/${n}`)
      try {
        const r = await api.post<{ report: { number: string } }>('/rms/reports', { incident: inc.number, offenses: [], people: [], property: [], vehicles: [] })
        nav(`/rms/${r.report.number}`)
      } catch (e) {
        toast.error(errText(e))
      }
    }}><FileText size={12} aria-hidden /> {n ? n : 'Report'}</button>
  )
}

/** ShareBox sends the incident to a neighbouring agency's CAD (mutual aid, a call on the line, a pursuit
 * crossing over), with why; it arrives there as a call for their dispatcher. Shown when there are neighbours. */
function ShareBox({ inc }: { inc: Incident }) {
  const [partners, setPartners] = useState<{ id: string; name: string }[]>([])
  const [to, setTo] = useState('')
  const [why, setWhy] = useState('')
  const [open, setOpen] = useState(false)
  useEffect(() => { api.get<{ id: string; name: string }[]>('/partners').then((p) => { setPartners(p); setTo(p[0]?.id ?? '') }).catch(() => {}) }, [])
  if (!partners.length) return null
  return (
    <section className="rounded-md border border-slate-800 p-2 text-xs" aria-label="Send to a neighbouring agency">
      {!open ? (
        <button className="btn h-6 px-2 text-[11px]" onClick={() => setOpen(true)}>Send to a neighbouring agency…</button>
      ) : (
        <form className="flex flex-wrap items-center gap-1" onSubmit={async (e) => {
          e.preventDefault()
          try { await api.post(`/incidents/${inc.number}/share`, { partner: to, reason: why }); toast.success(`Sent to ${partners.find((p) => p.id === to)?.name}`); setOpen(false); setWhy('') } catch (er) { toast.error(errText(er)) }
        }}>
          <select className="input h-7 w-auto text-xs" value={to} onChange={(e) => setTo(e.target.value)} aria-label="Agency">{partners.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select>
          <input className="input h-7 min-w-40 flex-1 text-xs" value={why} onChange={(e) => setWhy(e.target.value)} placeholder="Why: mutual aid, need a ladder; on your side of the line…" aria-label="Why" required />
          <button className="btn h-7 text-xs">Send</button>
          <button type="button" className="btn btn-ghost h-7 text-xs" onClick={() => setOpen(false)}>Cancel</button>
        </form>
      )}
    </section>
  )
}

const UNIT_TIMES: [string, string][] = [['dispatched', 'Sent'], ['enroute', 'En route'], ['onScene', 'On scene'], ['transport', 'Transport'], ['atDest', 'At hospital'], ['cleared', 'Cleared']]
// A datetime-local input works in local time without seconds' zone; the API wants RFC3339.
const toLocal = (iso?: string) => { if (!iso || iso.startsWith('0001')) return ''; const d = new Date(iso); return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 19) }

/** CorrectButton puts a closed incident's record right without reopening it: the call type, priority,
 * disposition, address, and units' times. Only what changes is sent, with why; it is on the timeline. */
function CorrectButton({ inc, view }: { inc: Incident; view: View }) {
  const [open, setOpen] = useState(false)
  const [d, setD] = useState({ type: '', priority: 0, disposition: '', address: '', reason: '' })
  const [times, setTimes] = useState<Record<string, Record<string, string>>>({})
  const start = () => {
    setD({ type: inc.type, priority: inc.priority, disposition: inc.disposition ?? '', address: inc.location?.address ?? '', reason: '' })
    setTimes(Object.fromEntries(inc.units.map((u) => [u.unit, Object.fromEntries(UNIT_TIMES.map(([k]) => [k, toLocal((u as unknown as Record<string, string>)[k])]))])))
    setOpen(true)
  }
  const save = async () => {
    const body: Record<string, unknown> = { reason: d.reason }
    if (d.type !== inc.type) body.type = d.type
    if (d.priority !== inc.priority) body.priority = d.priority
    if (d.disposition !== (inc.disposition ?? '')) body.disposition = d.disposition
    if (d.address.trim() !== (inc.location?.address ?? '')) body.address = d.address
    const ut: Record<string, Record<string, string>> = {}
    for (const u of inc.units) {
      for (const [k] of UNIT_TIMES) {
        const was = toLocal((u as unknown as Record<string, string>)[k])
        const now = times[u.unit]?.[k] ?? ''
        if (now !== was) (ut[u.unit] ??= {})[k] = now ? new Date(now).toISOString() : '0001-01-01T00:00:00Z'
      }
    }
    if (Object.keys(ut).length) body.unitTimes = ut
    try {
      const r = await api.post<{ changes: unknown[] }>(`/incidents/${inc.number}/correct`, body)
      toast.success(`Corrected: ${r.changes.length} change${r.changes.length === 1 ? '' : 's'}`)
      setOpen(false)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <>
      <button type="button" className="btn h-7 shrink-0" onClick={start}><Pencil size={12} aria-hidden /> Correct the record</button>
      <Modal open={open} onClose={() => setOpen(false)} wide title={`Correct ${inc.number}`} icon={<Pencil size={16} aria-hidden />}
        footer={<><button className="btn btn-ghost" onClick={() => setOpen(false)}>Cancel</button><button className="btn btn-go" disabled={!d.reason.trim()} onClick={save}>Correct</button></>}>
        <div className="space-y-3 text-sm">
          <p className="text-xs text-slate-400">The incident stays closed. What you change is on its timeline with what it was, who and why. Reports already sent (NERIS, NIBRS) are not changed: send them again.</p>
          <div className="grid gap-2 md:grid-cols-3">
            <Field label="Call type" htmlFor="cr-type"><select id="cr-type" className="input" value={d.type} onChange={(e) => setD({ ...d, type: e.target.value })}>{view.callTypes.map((c) => <option key={c.code} value={c.code}>{c.code} · {c.name}</option>)}</select></Field>
            <Field label="Priority" htmlFor="cr-p"><select id="cr-p" className="input" value={d.priority} onChange={(e) => setD({ ...d, priority: Number(e.target.value) })}>{[1, 2, 3, 4, 5].map((n) => <option key={n} value={n}>P{n}</option>)}</select></Field>
            <Field label="Disposition" htmlFor="cr-d"><select id="cr-d" className="input" value={d.disposition} onChange={(e) => setD({ ...d, disposition: e.target.value })}>{view.settings.dispositions.map((x) => <option key={x}>{x}</option>)}</select></Field>
          </div>
          <Field label="Address" htmlFor="cr-a"><input id="cr-a" className="input" value={d.address} onChange={(e) => setD({ ...d, address: e.target.value })} /></Field>
          {inc.units.length > 0 && (
            <table className="w-full text-xs">
              <thead><tr className="text-slate-400"><th className="text-left">Unit</th>{UNIT_TIMES.map(([k, l]) => <th key={k} className="text-left font-normal">{l}</th>)}</tr></thead>
              <tbody>
                {inc.units.map((u) => (
                  <tr key={u.unit}>
                    <td className="pr-2 font-mono font-bold">{u.unit}</td>
                    {UNIT_TIMES.map(([k, l]) => (
                      <td key={k} className="pr-1"><input type="datetime-local" step={1} className="input h-7 px-1 text-[11px]" aria-label={`${u.unit} ${l}`} value={times[u.unit]?.[k] ?? ''}
                        onChange={(e) => setTimes({ ...times, [u.unit]: { ...times[u.unit], [k]: e.target.value } })} /></td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          )}
          <Field label="Why" htmlFor="cr-why"><input id="cr-why" className="input" value={d.reason} onChange={(e) => setD({ ...d, reason: e.target.value })} placeholder="E.g. M1 gave its on-scene time late by radio" required /></Field>
        </div>
      </Modal>
    </>
  )
}

function ReplayButton({ number, view }: { number: string; view: View }) {
  const [open, setOpen] = useState(false)
  return (
    <>
      <button type="button" className="btn h-7 shrink-0" onClick={() => setOpen(true)}><Play size={12} aria-hidden /> Replay</button>
      {open && <ReplayDialog number={number} view={view} onClose={() => setOpen(false)} />}
    </>
  )
}
