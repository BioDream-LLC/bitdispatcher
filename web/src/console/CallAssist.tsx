import { Fragment, useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import { Car, Copy, Languages, Layers, MapPin, Mic, Send, ShieldAlert, Sparkles, Volume2 } from 'lucide-react'
import { api, errText } from '../api'
import { can, useStore } from '../store'
import { cx, hhmm } from '../util'
import { floorText, kmh, langName, methodText } from '../dream'
import type { Call, Detail, Phrase, SearchResult, View } from '../types'

// What the call-taker no longer types: the live transcript (translated when it is not English), the
// details pulled out of it, the reply in the caller's language, and the vehicle's crash data.

const KIND: Record<string, string> = {
  address: 'Address', weapon: 'Weapon', vehicle: 'Vehicle', plate: 'Plate', person: 'Person', injury: 'Patient', hazard: 'Hazard',
  count: 'People', direction: 'Went', name: 'Caller',
}

/** CallAssist is the transcript, details, reply and crash data of one call. */
export default function CallAssist({ call, view }: { call: Call; view: View }) {
  return (
    <>
      <LocationLine call={call} />
      {call.crash && <CrashCard call={call} />}
      {call.alert && <AlertCard call={call} />}
      {(call.details ?? []).length > 0 && <Details call={call} view={view} />}
      {(call.transcript ?? []).length > 0 && <Transcript call={call} />}
      {(call.transcript?.length || call.lang) && call.state !== 'ended' ? <Reply call={call} view={view} /> : null}
    </>
  )
}

function LocationLine({ call }: { call: Call }) {
  const f = floorText(call)
  const m = methodText(call.location?.method)
  const fixes = call.fixes?.length ?? 0
  if (!f && !m && fixes < 2) return null
  return (
    <div className="flex flex-wrap items-center gap-1 text-[10px] text-slate-400">
      {m && <span className={cx('chip', call.location?.method?.toUpperCase() === 'DBH' ? 'bg-emerald-500/15 text-emerald-300' : 'bg-slate-800 text-slate-300')}><MapPin size={10} aria-hidden /> {m}</span>}
      {f && <span className="chip bg-emerald-500/15 text-emerald-300"><Layers size={10} aria-hidden /> {f}</span>}
      {fixes > 1 && <span className="chip bg-slate-800 text-slate-300" title="The map shows the trail of the caller's positions">{fixes} positions received</span>}
    </div>
  )
}

function CrashCard({ call }: { call: Call }) {
  const c = call.crash!
  const r = call.crashRisk
  const v = [c.vehicle.color, c.vehicle.year, c.vehicle.make, c.vehicle.model].filter(Boolean).join(' ')
  return (
    <section aria-label="Vehicle crash data" className={cx('rounded border p-1.5 text-xs', r?.risk === 'high' ? 'border-rose-700 bg-rose-950/40' : 'border-amber-800/70 bg-amber-950/20')}>
      <div className="flex items-center gap-1.5 text-[11px] font-semibold">
        <Car size={13} className={r?.risk === 'high' ? 'text-rose-300' : 'text-amber-300'} aria-hidden />
        <span className="text-slate-100">Vehicle crash data</span>
        {r && <span className={cx('chip', r.risk === 'high' ? 'bg-rose-500 text-white' : r.risk === 'medium' ? 'bg-amber-500/30 text-amber-100' : 'bg-slate-800 text-slate-300')}>{r.risk.toUpperCase()} INJURY RISK</span>}
        <span className="ml-auto text-[10px] font-normal text-slate-400">{c.provider}</span>
      </div>
      <dl className="mt-1 grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5 text-[11px]">
        {v && <><dt className="text-slate-500">Vehicle</dt><dd className="text-slate-200">{v}{c.vehicle.plate ? <span className="ml-1 font-mono">{c.vehicle.plate}</span> : null}</dd></>}
        {c.deltaVKmh ? <><dt className="text-slate-500">Delta-V</dt><dd className="text-slate-200">{kmh(c.deltaVKmh)}{c.direction ? `, ${c.direction} impact` : ''}</dd></> : null}
        <dt className="text-slate-500">Crash</dt>
        <dd className="text-slate-200">{[c.rollover && 'ROLLOVER', c.multipleImpacts && 'several impacts', c.airbags?.length && `airbags: ${c.airbags.join(', ')}`, c.speedKmh && `speed ${kmh(c.speedKmh)}`].filter(Boolean).join(' · ') || 'no more data'}</dd>
        {c.occupants ? <><dt className="text-slate-500">Occupants</dt><dd className="text-slate-200">{c.occupants}{c.belted === false ? ', NOT belted' : c.belted ? ', belted' : ''}</dd></> : null}
        {c.fuel && <><dt className="text-slate-500">Power</dt><dd className={cx(['electric', 'hybrid', 'hydrogen', 'cng'].includes(c.fuel) ? 'font-semibold text-amber-200' : 'text-slate-200')}>{c.fuel}</dd></>}
      </dl>
      {(r?.reasons?.length ?? 0) > 0 && <div className="mt-1 text-[11px] text-slate-300">{r!.reasons.join(' · ')}</div>}
      {(r?.suggest?.length ?? 0) > 0 && <ul className="mt-1 list-disc pl-4 text-[11px] text-amber-100">{r!.suggest.map((x) => <li key={x}>{x}</li>)}</ul>}
      <div className="mt-1 text-[10px] text-slate-500">A rule of thumb from field-triage criteria, to help pick the response. Not a diagnosis.</div>
    </section>
  )
}

function Details({ call, view }: { call: Call; view: View }) {
  const me = useStore((s) => s.me)
  const ds = [...(call.details ?? [])].sort((a, b) => Number(!!b.urgent) - Number(!!a.urgent))
  const useAddress = async (d: Detail) => {
    if (!call.incident) {
      await navigator.clipboard?.writeText(d.text).catch(() => {})
      toast.info(`Copied ${d.text}: make the incident, then paste it as the location`)
      return
    }
    try {
      const [lat, lon] = view.settings.mapCenter
      const r = await api.get<SearchResult[]>(`/map/search?q=${encodeURIComponent(d.text)}&lat=${lat}&lon=${lon}&limit=3`).catch(() => [] as SearchResult[])
      const hit = (r ?? []).find((x) => x.exact) ?? (r ?? [])[0]
      const location = hit ? { address: hit.label, lat: hit.lat, lon: hit.lon, source: 'dispatcher' } : { address: d.text, source: 'dispatcher' }
      await api.patch(`/incidents/${call.incident}`, { location })
      toast.success(`${call.incident} placed at ${location.address}${hit?.exact ? '' : ' (check it on the map)'}`)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section aria-label="Key details" className="rounded border border-slate-800 bg-slate-900/60 p-1.5">
      <div className="mb-1 flex items-center gap-1 text-[11px] font-semibold text-slate-300"><Sparkles size={12} className="text-sky-300" aria-hidden /> Heard on the call</div>
      <ul className="flex flex-wrap gap-1">
        {ds.map((d) => (
          <li key={d.kind + d.text} title={d.quote ? `“${d.quote}”` : undefined}
            className={cx('inline-flex items-center gap-1 rounded border px-1.5 py-0.5 text-[11px]', d.urgent ? 'border-rose-700 bg-rose-500/15 text-rose-100' : 'border-slate-700 bg-slate-800/70 text-slate-200')}>
            {d.urgent && <ShieldAlert size={11} aria-hidden />}
            <span className="text-[9px] font-bold uppercase tracking-wider text-slate-400">{KIND[d.kind] ?? d.kind}</span> {d.text}
            {d.kind === 'address' && can.dispatch(me) && (
              <button className="ml-0.5 rounded px-1 text-[10px] text-sky-300 hover:bg-sky-500/20" onClick={() => useAddress(d)} aria-label={`Use ${d.text} as the incident's location`}>
                {call.incident ? 'use' : <Copy size={10} aria-hidden />}
              </button>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}

function Transcript({ call }: { call: Call }) {
  const ref = useRef<HTMLUListElement>(null)
  const n = call.transcript?.length ?? 0
  const [orig, setOrig] = useState(false)
  useEffect(() => {
    ref.current?.scrollTo({ top: ref.current.scrollHeight })
  }, [n])
  const foreign = (call.transcript ?? []).some((l) => l.english)
  return (
    <section aria-label="Transcript" className="rounded border border-slate-800 bg-slate-950 p-1.5">
      <div className="mb-1 flex items-center gap-1 text-[11px] font-semibold text-slate-300">
        <Mic size={12} className={call.state !== 'ended' ? 'text-rose-300 live-dot' : 'text-slate-500'} aria-hidden /> Transcript
        {call.lang && call.lang !== 'en' && <span className="chip bg-fuchsia-500/20 text-fuchsia-200"><Languages size={10} aria-hidden /> {langName[call.lang] ?? call.lang} · translated</span>}
        {foreign && <button className="ml-auto text-[10px] text-sky-300 hover:underline" onClick={() => setOrig(!orig)}>{orig ? 'English' : 'Show original'}</button>}
      </div>
      <ul ref={ref} className="max-h-40 space-y-0.5 overflow-y-auto text-xs" aria-live="polite">
        {(call.transcript ?? []).map((l, i) => {
          const english = l.english && !orig
          return (
            <li key={i} className={l.speaker === 'caller' ? 'text-slate-100' : 'text-sky-300'}>
              <span className="font-mono text-[10px] text-slate-500">{hhmm(l.at)} </span>
              <span className="font-semibold">{l.speaker === 'caller' ? 'Caller' : l.speaker === 'taker' ? 'Taker' : 'Other'}:</span>{' '}
              {english ? l.english : l.text}
              {english && <span className={cx('ml-1 text-[9px] uppercase', l.gist ? 'text-amber-300' : 'text-slate-500')} title={l.speaker === 'caller' ? `Said: ${l.text}` : `Sent: ${l.text}`}>{l.gist ? 'gist only' : l.by === 'model' ? 'model' : l.by === 'phrasebook' ? 'phrasebook' : ''}</span>}
              {l.speaker === 'taker' && l.english && (
                <>
                  <span className="ml-1 text-[10px] text-slate-500">→ “{l.text}”</span>
                  <button className="ml-1 align-middle text-slate-400 hover:text-sky-300" onClick={() => speak(l.text, l.lang ?? 'es')} aria-label={`Say “${l.text}” aloud`}><Volume2 size={11} aria-hidden /></button>
                </>
              )}
            </li>
          )
        })}
      </ul>
    </section>
  )
}

let phraseCache: Phrase[] | null = null

function Reply({ call, view }: { call: Call; view: View }) {
  const me = useStore((s) => s.me)
  const [phrases, setPhrases] = useState<Phrase[]>(phraseCache ?? [])
  const [text, setText] = useState('')
  const [preview, setPreview] = useState('')
  const [send, setSend] = useState(!!call.textOnly || !!call.text?.length)
  const [busy, setBusy] = useState(false)
  const lang = call.lang && call.lang !== 'en' ? call.lang : ''
  useEffect(() => {
    if (!phraseCache) api.get<{ items: Phrase[] }>('/phrases').then((r) => { phraseCache = r.items; setPhrases(r.items) }).catch(() => {})
  }, [])
  useEffect(() => {
    if (!lang || !text.trim()) return setPreview('')
    const t = setTimeout(() => api.post<{ text: string }>('/translate', { text, from: 'en', to: lang }).then((r) => setPreview(r.text)).catch(() => setPreview('')), 300)
    return () => clearTimeout(t)
  }, [text, lang])
  if (!can.dispatch(me) || !lang) return null
  const go = async (en: string) => {
    setBusy(true)
    try {
      const r = await api.post<{ text: string; sent: string }>(`/calls/${encodeURIComponent(call.id)}/reply`, { text: en, send: send && view.health.bitvoice.configured })
      toast.success(`${langName[lang] ?? lang}: “${r.text}”${r.sent ? ` · ${r.sent}` : ''}`)
      setText('')
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <section aria-label={`Reply in ${langName[lang] ?? lang}`} className="rounded border border-fuchsia-900/60 bg-fuchsia-950/15 p-1.5">
      <div className="mb-1 flex items-center gap-1 text-[11px] font-semibold text-fuchsia-200">
        <Languages size={12} aria-hidden /> Say it in {langName[lang] ?? lang}
        <label className="ml-auto flex items-center gap-1 text-[10px] font-normal text-slate-300">
          <input type="checkbox" checked={send} onChange={(e) => setSend(e.target.checked)} disabled={!view.health.bitvoice.configured} /> also text it
        </label>
      </div>
      <div className="flex flex-wrap gap-1">
        {phrases.slice(0, 12).map((p) => (
          <button key={p.key} className="rounded border border-slate-700 bg-slate-800/70 px-1.5 py-0.5 text-[10px] text-slate-200 hover:bg-slate-700" disabled={busy} onClick={() => go(p.en)} title={p.es}>{p.en}</button>
        ))}
      </div>
      <form className="mt-1 flex gap-1" onSubmit={(e) => { e.preventDefault(); if (text.trim()) go(text) }}>
        <input className="input h-7 py-0 text-xs" value={text} onChange={(e) => setText(e.target.value)} placeholder="Type in English…" aria-label="Reply in English, to be translated" maxLength={500} />
        <button className="btn h-7" disabled={!text.trim() || busy} aria-label="Translate and say"><Send size={12} aria-hidden /></button>
      </form>
      {preview && <div className="mt-0.5 text-[11px] text-fuchsia-100">→ {preview}</div>}
      <div className="mt-0.5 text-[10px] text-slate-500">Read it to the caller, or press <Volume2 size={10} className="inline" aria-hidden /> to have this console say it. Kept on the transcript.</div>
    </section>
  )
}

/** speak says text aloud in a language, on this console, when the browser can. */
export function speak(text: string, lang: string) {
  if (typeof speechSynthesis === 'undefined') return
  const u = new SpeechSynthesisUtterance(text)
  u.lang = lang === 'es' ? 'es-US' : lang
  speechSynthesis.cancel()
  speechSynthesis.speak(u)
}

/** AlertCard: everything an outside alert gave, with the sender's own event ID for when the dispatcher calls them. */
function AlertCard({ call }: { call: Call }) {
  const a = call.alert!
  const rows: [string, string | undefined][] = [
    ['Sender', a.sender], ['Event ID', a.id], ['Event', a.event], ['Premise', a.premise], ['Zone', a.zone], ['Account', a.account], ['Permit', a.permit],
    ['Keyholder', a.keyholder], ['Operator', a.operator], ['Call back', a.callback], ['Plate', [a.plate, a.state].filter(Boolean).join(' ') || undefined],
    ['Vehicle', a.vehicle], ['Hotlist', a.hotlist], ['Camera', a.camera], ['Heading', a.heading], ['Rounds', a.rounds ? String(a.rounds) : undefined], ['Confidence', a.confidence], ['Detail', a.detail],
  ]
  return (
    <section className={cx('rounded-md border p-2 text-xs', a.verified || a.kind === 'gunshot' ? 'border-rose-600 bg-rose-950/30' : 'border-orange-700/70 bg-orange-950/20')} aria-label="Alert">
      <div className="mb-1 flex items-center gap-1.5 font-semibold text-slate-100">
        {({ alarm: 'Alarm', lpr: 'Plate reader', gunshot: 'Gunshot detection', camera: 'Camera', cad: 'Incident', other: 'Alert' })[a.kind]} from {a.sender}
        {a.verified && <span className="chip bg-rose-600 text-white">VERIFIED</span>}
        {a.status === 'cancelled' && <span className="chip bg-emerald-600 text-white">CANCELLED</span>}
      </div>
      <dl className="grid grid-cols-[6rem_1fr] gap-x-2 gap-y-0.5">
        {rows.filter(([, v]) => v).map(([k, v]) => <Fragment key={k}><dt className="text-slate-500">{k}</dt><dd className="text-slate-200">{v}</dd></Fragment>)}
      </dl>
      {a.image && <a className="mt-1 inline-block text-sky-300 underline" href={a.image} target="_blank" rel="noreferrer noopener">Open the sender's picture</a>}
    </section>
  )
}
