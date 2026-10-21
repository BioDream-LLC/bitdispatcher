import { useState } from 'react'
import { toast } from 'sonner'
import { Archive, BadgeCheck, Eye, FileVideo, Image as ImageIcon, MapPin, Route, ShieldAlert, ShieldQuestion, Sparkles } from 'lucide-react'
import { api, errText } from '../api'
import { can, useStore } from '../store'
import { cx } from '../util'
import type { Call, EvidenceItem } from '../types'

const LVF_PART: Record<string, string> = { RD: 'street', HNO: 'house number', A3: 'city', A1: 'state', PC: 'postal code', STS: 'street type', POD: 'direction', PRD: 'direction' }
const VIA: Record<string, string> = { tty: 'TTY', rtt: 'Real-time text', msrp: 'Text to 911 (MSRP)', sip: 'Text to 911 (SIP)' }

/** CallerIDChip says whether the caller's number was verified (STIR/SHAKEN), as the call-handling
 * system found it: a failed check is a reason to doubt the number, not the emergency. */
export function CallerIDChip({ call }: { call: Call }) {
  const c = call.handling?.callerId
  if (!c) return null
  const ok = c.verified === 'passed'
  const bad = c.verified === 'failed'
  return (
    <span title={c.why} className={cx('chip', ok ? 'bg-emerald-500/20 text-emerald-300' : bad ? 'bg-rose-500/25 text-rose-200' : 'bg-slate-800 text-slate-300')}>
      {ok ? <BadgeCheck size={10} aria-hidden /> : bad ? <ShieldAlert size={10} aria-hidden /> : <ShieldQuestion size={10} aria-hidden />}
      {ok ? 'number verified' : bad ? 'number NOT verified' : 'number unchecked'}{c.attest ? ` · ${c.attest}` : ''}
    </span>
  )
}

/** HandlingBox is what the call-handling system knows beyond the basics: how the call came, how a legacy
 * call's location was looked up, its own check of a changed location, the ways text came, how the ESInet
 * routed it, and the recording's summary. */
export default function HandlingBox({ call }: { call: Call }) {
  const h = call.handling
  if (!h) return null
  const chips: { text: string; title?: string; tone?: string }[] = []
  if (h.esinet) chips.push({ text: 'ESInet', title: 'Came from the emergency services network (NG9-1-1)' })
  if (h.service && h.service !== 'urn:service:sos') chips.push({ text: h.service.replace('urn:service:', ''), title: 'The emergency service asked for' })
  if (h.cama) chips.push({ text: `CAMA ${h.cama === '-' ? '(no digits)' : h.cama}`, title: 'A legacy 911 trunk: the location is looked up by the callback number', tone: 'bg-amber-500/20 text-amber-200' })
  for (const v of h.textVia ?? []) chips.push({ text: VIA[v] ?? v, tone: 'bg-violet-500/20 text-violet-200' })
  return (
    <div className="space-y-1 text-[11px]">
      {chips.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {chips.map((c) => <span key={c.text} title={c.title} className={cx('chip', c.tone ?? 'bg-slate-800 text-slate-300')}>{c.text}</span>)}
        </div>
      )}
      {h.lvf && (
        <div className={cx('rounded border px-1.5 py-1', h.lvf.valid && h.lvf.within !== false ? 'border-emerald-800 bg-emerald-950/30 text-emerald-100' : 'border-amber-700 bg-amber-950/30 text-amber-100')}>
          <span className="font-semibold">{h.lvf.valid ? 'Address in the address data' : 'Address NOT in the address data'}</span>
          {h.lvf.valid ? ` (${h.lvf.matched})` : h.lvf.invalid?.length ? `: ${h.lvf.invalid.map((x) => LVF_PART[x] ?? x).join(', ')} not found` : ''}
          {h.lvf.position && <div className={cx('text-[10px]', h.lvf.within === false ? 'text-amber-200' : 'text-slate-300')}>{h.lvf.within === false ? '⚠ ' : ''}{h.lvf.position}</div>}
          {(h.lvf.suggestions ?? []).length > 0 && (
            <ul className="mt-0.5 text-slate-200">{h.lvf.suggestions!.slice(0, 3).map((s) => <li key={s.address}>Did they mean <span className="font-semibold">{s.address}</span>? <span className="text-slate-400">({s.why})</span></li>)}</ul>
          )}
        </div>
      )}
      {h.aliLookup && <div className="text-slate-400"><MapPin size={10} className="inline" aria-hidden /> ALI lookup: <span className="text-slate-200">{h.aliLookup}</span></div>}
      {h.locCheck && (
        <div className={cx('rounded border px-1.5 py-1', h.locCheck.show === 'old' ? 'border-amber-700 bg-amber-950/30 text-amber-100' : 'border-sky-800 bg-sky-950/30 text-sky-100')}>
          <span className="font-semibold">Location check (BitVoice):</span> {h.locCheck.text}
          {(h.locCheck.reasons ?? []).length > 0 && <ul className="list-disc pl-4 text-slate-300">{h.locCheck.reasons!.map((r) => <li key={r}>{r}</li>)}</ul>}
        </div>
      )}
      {(h.routes ?? []).length > 0 && (
        <details className="text-slate-400">
          <summary className="cursor-pointer"><Route size={10} className="inline" aria-hidden /> Routed by the ESInet ({h.routes!.length})</summary>
          <ul className="pl-3">{h.routes!.map((r, i) => <li key={i}>{new Date(r.at).toLocaleTimeString()} {r.psap ? `→ ${r.psap}: ` : ''}{r.why}</li>)}</ul>
        </details>
      )}
      {h.summary && (
        <div className="rounded border border-slate-700 bg-slate-900/60 px-1.5 py-1 text-slate-200">
          <Sparkles size={10} className="inline text-violet-300" aria-hidden /> <span className="text-slate-400">{h.summaryBy || 'Summary'}:</span> {h.summary}
          <div className="text-[10px] text-slate-500">Written by a language model from the recording: check it against what was said.</div>
        </div>
      )}
      {h.recording && <div className="text-slate-500">Recorded in BitVoice: <span className="font-mono">{h.recording}</span></div>}
      {h.evidence && h.evidence.items.length > 0 && <EvidenceList call={call} />}
    </div>
  )
}

const size = (b: number) => (b > 1 << 20 ? `${(b / (1 << 20)).toFixed(1)} MB` : `${Math.max(1, Math.round(b / 1024))} KB`)

/** EvidenceList is the caller's photos and clips: shown as their facts (hash, the camera's time and GPS)
 * until a person reveals one; each reveal is recorded with who looked. A supervisor or dispatcher holds
 * them as evidence so they are kept past the retention. */
function EvidenceList({ call }: { call: Call }) {
  const me = useStore((s) => s.me)
  const ev = call.handling!.evidence!
  const [shown, setShown] = useState<Record<number, boolean>>({})
  const [busy, setBusy] = useState(false)
  const src = (it: EvidenceItem) => `./api/calls/${encodeURIComponent(call.id)}/evidence/${it.n}`
  return (
    <section aria-label="Caller photos and clips" className="rounded border border-slate-700 bg-slate-950/60 p-1.5">
      <div className="mb-1 flex items-center gap-1.5 font-semibold text-slate-200">
        <ImageIcon size={12} aria-hidden /> Caller photos and clips ({ev.items.length})
        {ev.held && <span className="chip bg-emerald-600 text-white">held as evidence</span>}
        {can.dispatch(me) && (
          <button className="btn ml-auto h-6 px-2 py-0 text-[11px]" disabled={busy} onClick={async () => {
            setBusy(true)
            try { await api.post(`/calls/${encodeURIComponent(call.id)}/evidence/hold`, { hold: !ev.held }) } catch (e) { toast.error(errText(e)) } finally { setBusy(false) }
          }}><Archive size={11} aria-hidden /> {ev.held ? 'Release' : 'Hold as evidence'}</button>
        )}
      </div>
      <ul className="space-y-1.5">
        {ev.items.map((it) => (
          <li key={it.n} aria-label={`File ${it.n}: ${it.name}`} className="rounded border border-slate-800 p-1">
            <div className="flex flex-wrap items-center gap-1.5">
              {it.kind === 'video' || it.type.startsWith('video/') ? <FileVideo size={12} aria-hidden /> : <ImageIcon size={12} aria-hidden />}
              <span className="font-semibold text-slate-100">{it.name}</span>
              <span className="text-slate-500">{size(it.size)}{it.recording ? ' · recorded from the live video' : ''}</span>
              {can.dispatch(me) && !shown[it.n] && (
                <button className="btn ml-auto h-6 px-2 py-0 text-[11px]" onClick={() => setShown({ ...shown, [it.n]: true })}><Eye size={11} aria-hidden /> Reveal (recorded)</button>
              )}
            </div>
            <div className="font-mono text-[10px] text-slate-400">
              {it.takenLocal ? `taken ${it.takenLocal} (the camera's clock)` : 'no camera time'} · received {new Date(it.received).toLocaleTimeString()}
              {it.hasGps ? ` · GPS ${it.lat!.toFixed(5)}, ${it.lon!.toFixed(5)}` : ' · no GPS'}
            </div>
            <div className="break-all font-mono text-[10px] text-slate-500">SHA-256 {it.sha256}</div>
            {shown[it.n] && (
              it.type.startsWith('video/')
                ? <video src={src(it)} controls className="mt-1 max-h-64 w-full rounded bg-black" aria-label={`Clip ${it.name}`} />
                : <img src={src(it)} alt={`Photo ${it.name}`} className="mt-1 max-h-64 rounded" />
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}
