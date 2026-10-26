import { useEffect, useMemo, useState } from 'react'
import { api, errText } from '../api'
import { toast } from 'sonner'
import ProtocolPanel from './Protocol'
import HandlingBox, { CallerIDChip } from '../console/Handling'
import { CheckCircle2, Link2, MapPin, PhoneForwarded, X } from 'lucide-react'
import AddressInput from '../AddressInput'
import { cx, phone, prio } from '../util'
import type { Call, Dup, Incident, View } from '../types'
import { rankTypes, pickAddress, type Draft } from './draft'

const ALI_FIELDS = (c?: Call) => {
  if (!c) return []
  const l = c.location
  return [
    ['ANI', phone(c.callback)],
    ['Class', c.class ?? '—'],
    ['Provider', c.provider ?? '—'],
    ['Position', l?.lat != null && l.lon != null ? `${l.lat.toFixed(5)}, ${l.lon.toFixed(5)}` : '—'],
    ['Accuracy', l?.radius ? `±${Math.round(l.radius)} m${l.confidence ? ` (${l.confidence}%)` : ''}` : '—'],
    ['Method', l?.method ?? '—'],
    ['Source', [l?.source, l?.at && new Date(l.at).toLocaleTimeString()].filter(Boolean).join(' · ') || '—'],
  ]
}

/** Call Entry, top to bottom in the order a call is handled: the incident location first (verified,
 * estimated, the caller's, a map point or forced for this incident, with the place: business, building,
 * floor, unit), then the caller and the callback read back, then the call type, priority and modifiers;
 * the call's ANI/ALI beside it and the narrative always open on the right. */
export default function EntryTab({ view, d, inc, call, update, dups, onAttach, onDismissDups, map }: {
  view: View; d: Draft; inc?: Incident; call?: Call; update: (p: Partial<Draft>) => void; dups: Dup[]; onAttach: (n: string) => void; onDismissDups: () => void; map: React.ReactNode
}) {
  const ct = view.callTypes.find((c) => c.code === d.type)
  const mods = (view.entry?.modifiers ?? []).filter((m) => !m.disciplines?.length || !ct || m.disciplines.includes(ct.discipline))
  return (
    <div className="grid gap-3 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
      <div className="min-w-0 space-y-3">
        {dups.length > 0 && (
          <div className="rounded-lg border border-amber-600 bg-amber-950/40 p-2" role="alert">
            <div className="mb-1 flex items-center gap-1.5 text-xs font-bold text-amber-100">
              <Link2 size={13} aria-hidden /> Is it one of these? Open incidents nearby
              <button className="btn btn-ghost ml-auto h-6 px-1" aria-label="Not the same" onClick={onDismissDups}><X size={13} /></button>
            </div>
            <ul className="space-y-1">
              {dups.map((x) => (
                <li key={x.incident} className="flex items-center gap-2 text-xs text-amber-50">
                  <span className="font-mono font-bold">{x.incident}</span> {x.typeName}
                  <span className="truncate text-amber-200/80">{x.address} · {x.why.join(', ')}{x.units.length ? ` · ${x.units.join(' ')}` : ''}</span>
                  <button className="btn btn-go ml-auto h-6 shrink-0 px-2 text-[11px]" onClick={() => onAttach(x.incident)}>Add this call to it</button>
                </li>
              ))}
            </ul>
            <p className="mt-1 text-[11px] text-amber-200/80">Not the same? Press <span className="kbd">F4</span> again to make a new incident.</p>
          </div>
        )}
        <LocationSection d={d} inc={inc} call={call} update={update} />
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className="label" htmlFor="cn-taker">Caller / reporting party</label>
            <input id="cn-taker" data-field="callerName" className="input" value={d.callerName} onChange={(e) => update({ callerName: e.target.value })} placeholder="LAST, FIRST" />
          </div>
          <div>
            <label className="label" htmlFor="cb-taker">Callback number</label>
            <input id="cb-taker" data-field="callback" className="input font-mono" value={d.callback} onChange={(e) => update({ callback: e.target.value, callbackVerified: false })} placeholder="+1 540 555 0100" />
            <label className="mt-0.5 flex items-center gap-1 text-[11px] text-slate-400"><input type="checkbox" checked={!!d.callbackVerified} onChange={(e) => update({ callbackVerified: e.target.checked })} /> Read back to the caller and confirmed</label>
          </div>
        </div>
        <label className="flex items-center gap-2 text-xs text-slate-300">
          <input type="checkbox" checked={d.callerElsewhere} onChange={(e) => update({ callerElsewhere: e.target.checked })} /> The caller is not at the incident
        </label>
        {d.callerElsewhere && (
          <div className="ml-5">
            <label className="label" htmlFor="ca-taker">Where the caller is</label>
            <AddressInput id="ca-taker" value={d.callerAddress} onChange={(v) => update({ callerAddress: v })} onPick={(r) => update({ callerAddress: r.label, callerLat: r.lat.toFixed(6), callerLon: r.lon.toFixed(6) })} placeholder="The caller's own address or place" />
          </div>
        )}
        <TypePicker view={view} value={d.type} onPick={(code) => update({ type: code })} />
        <div className="space-y-2">
          <fieldset>
            <legend className="label">Priority</legend>
            <div className="flex gap-1">
              <button type="button" aria-pressed={d.priority === 0} className={cx('btn h-7 px-2 text-xs', d.priority === 0 && 'ring-1 ring-sky-400')} onClick={() => update({ priority: 0 })}>Type's{ct ? ` P${ct.priority}` : ''}</button>
              {[1, 2, 3, 4, 5].map((p) => (
                <button key={p} type="button" aria-pressed={d.priority === p} className={cx('btn h-7 w-8 justify-center font-mono text-xs', d.priority === p && cx(prio(p).bg, prio(p).fg))} onClick={() => update({ priority: p })}>P{p}</button>
              ))}
            </div>
          </fieldset>
          <fieldset className="min-w-0">
            <legend className="label">Modifiers</legend>
            <div className="flex flex-wrap gap-1">
              {mods.map((m) => {
                const on = d.modifiers.includes(m.code)
                return (
                  <button key={m.code} type="button" aria-pressed={on} title={[m.priority && `raises to P${m.priority}`, m.notify?.length && `notify ${m.notify.join(', ')}`].filter(Boolean).join(' · ') || undefined}
                    className={cx('chip h-6 cursor-pointer px-2 text-[11px]', on ? (m.safety ? 'bg-rose-500 text-white' : 'bg-sky-500 text-white') : 'bg-slate-800 text-slate-300 hover:bg-slate-700')}
                    onClick={() => update({ modifiers: on ? d.modifiers.filter((x) => x !== m.code) : [...d.modifiers, m.code] })}>
                    {m.name}
                  </button>
                )
              })}
            </div>
          </fieldset>
        </div>
      </div>
      <div className="min-w-0 space-y-2">
        <div className="panel rounded-md">
          <div className="panel-head"><PhoneForwarded size={12} aria-hidden /> ANI / ALI <span className="ml-auto normal-case tracking-normal text-slate-500">rebids and location providers update it</span></div>
          {call ? (
            <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 px-2 py-1.5 text-xs">
              {ALI_FIELDS(call).map(([k, v]) => (
                <div key={k} className="contents"><dt className="text-slate-500">{k}</dt><dd className="truncate font-mono text-slate-200">{v}</dd></div>
              ))}
              {call.ali && <><dt className="text-slate-500">ALI</dt><dd className="whitespace-pre-wrap font-mono text-[10px] text-slate-400">{call.ali}</dd></>}
              {call.handling && <div className="col-span-2 space-y-1 pt-1"><CallerIDChip call={call} /><HandlingBox call={call} /></div>}
              {view.health.lis && call.callback && call.state !== 'ended' && (
                <div className="col-span-2 pt-1"><AskLIS id={call.id} name={view.health.lis} /></div>
              )}
            </dl>
          ) : <p className="px-2 py-1.5 text-xs text-slate-500">No call: a walk-in, an officer's report or a radio call.</p>}
        </div>
        <ProtocolPanel inc={inc} type={inc?.type ?? d.type} />
        <div className="relative grid h-[340px] overflow-hidden rounded-md border border-slate-800">{map}</div>
      </div>
    </div>
  )
}

/** The call type: type a code or a word; arrow keys and Enter choose. Hundreds of types are fine. */
function TypePicker({ view, value, onPick }: { view: View; value: string; onPick: (code: string) => void }) {
  const [q, setQ] = useState('')
  const [hi, setHi] = useState(0)
  const [open, setOpen] = useState(false)
  const list = useMemo(() => rankTypes(view.callTypes, q).slice(0, 60), [view.callTypes, q])
  const cur = view.callTypes.find((c) => c.code === value)
  const choose = (code: string) => {
    onPick(code)
    setQ('')
    setOpen(false)
  }
  return (
    <div className="relative">
      <label className="label" htmlFor="ct-taker">Call type</label>
      <input id="ct-taker" className="input font-mono" role="combobox" aria-expanded={open} aria-controls="ct-taker-list" autoComplete="off"
        placeholder={cur ? `${cur.code} — ${cur.name}` : 'Code or word: SF, chest, burglary…'} value={q}
        onFocus={() => setOpen(true)} onBlur={() => setTimeout(() => setOpen(false), 150)}
        onChange={(e) => { setQ(e.target.value); setHi(0); setOpen(true) }}
        onKeyDown={(e) => {
          if (e.key === 'ArrowDown') { e.preventDefault(); setHi((h) => Math.min(list.length - 1, h + 1)) }
          else if (e.key === 'ArrowUp') { e.preventDefault(); setHi((h) => Math.max(0, h - 1)) }
          else if (e.key === 'Enter' && !e.ctrlKey && list[hi]) { e.preventDefault(); choose(list[hi].code) }
          else if (e.key === 'Escape') setOpen(false)
        }} />
      {cur && !open && (
        <div className="mt-1 flex items-center gap-1.5 text-xs">
          <span className={cx('chip font-mono', prio(cur.priority).bg, prio(cur.priority).fg)}>P{cur.priority}</span>
          <span className="font-mono font-bold text-slate-200">{cur.code}</span> <span className="text-slate-300">{cur.name}</span>
          <span className="text-slate-500">{cur.discipline}{cur.group ? ` · ${cur.group}` : ''}</span>
        </div>
      )}
      {open && list.length > 0 && (
        <ul id="ct-taker-list" role="listbox" aria-label="Call types" className="absolute z-40 mt-1 max-h-72 w-full overflow-y-auto rounded-md border border-slate-700 bg-slate-900 shadow-xl">
          {list.map((c, i) => (
            <li key={c.code} role="option" aria-selected={i === hi}>
              <button type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => choose(c.code)}
                className={cx('flex w-full items-center gap-2 px-2 py-1 text-left text-xs', i === hi ? 'bg-slate-800' : 'hover:bg-slate-800/60')}>
                <span className={cx('chip font-mono', prio(c.priority).bg, prio(c.priority).fg)}>P{c.priority}</span>
                <span className="w-16 shrink-0 font-mono font-bold text-slate-300">{c.code}</span>
                <span className="min-w-0 truncate text-slate-100">{c.name}</span>
                <span className="ml-auto shrink-0 text-[10px] text-slate-500">{c.group ?? c.discipline}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/** AskLIS asks the location server (RapidSOS or another HELD server) where the caller's phone is now. */
function AskLIS({ id, name }: { id: string; name: string }) {
  const [busy, setBusy] = useState(false)
  return (
    <button type="button" className="btn h-6 px-2 text-[11px]" disabled={busy} onClick={async () => {
      setBusy(true)
      try { await api.post(`/calls/${encodeURIComponent(id)}/lis`); toast.success(`${name}: location updated`) } catch (e) { toast.error(errText(e)) } finally { setBusy(false) }
    }}><MapPin size={11} aria-hidden /> {busy ? 'Asking…' : `Ask ${name}`}</button>
  )
}

type PlacePolicy = { kinds: string[]; reasons: string[]; unitKinds: string[]; noteAlways: boolean }
let policyCache: PlacePolicy | null = null
function usePlacePolicy() {
  const [p, setP] = useState<PlacePolicy | null>(policyCache)
  useEffect(() => { if (!policyCache) api.get<PlacePolicy>('/places/policy').then((x) => { policyCache = x; setP(x) }).catch(() => {}) }, [])
  return p
}

const VERIFY_WORDS: Record<string, string> = { verified: 'Verified address point', estimated: 'Estimated (address range)', caller: "The caller's device location", map: 'A point I placed on the map', forced: 'Forced for this incident', unverified: 'Not verified yet' }

/** The incident location: the address, its point, how it was settled and the place within it. Nothing
 * here holds up an urgent call: an unverified or forced location is sent and the supervisor reviews it. */
function LocationSection({ d, inc, call, update }: { d: Draft; inc?: Incident; call?: Call; update: (p: Partial<Draft>) => void }) {
  const pol = usePlacePolicy()
  const verify = d.placeVerify || (d.verified ? 'verified' : '')
  const label = verify && verify !== 'verified' ? verify.toUpperCase() : ''
  return (
    <div className="space-y-2 rounded-md border border-slate-800 p-2">
      <div className="flex items-center gap-2">
        <label className="label mb-0" htmlFor="addr-taker">1 · Incident location</label>
        {label && <span className={cx('chip h-5 px-1.5 text-[10px] font-bold', verify === 'forced' ? 'bg-rose-600 text-white' : 'bg-amber-500 text-black')} role="status">{label}</span>}
        {verify === 'verified' && <span className="inline-flex items-center gap-1 text-[11px] font-semibold text-emerald-300"><CheckCircle2 size={12} aria-hidden /> VERIFIED</span>}
        {inc?.place?.linked && <span className="chip h-5 bg-emerald-700/40 px-1.5 text-[10px] text-emerald-100">linked to {inc.place.linked.address}</span>}
      </div>
      <AddressInput id="addr-taker" value={d.address} onChange={(v) => update({ address: v, verified: false, placeVerify: verify === 'forced' ? 'forced' : '', entered: d.entered && d.placeVerify === 'forced' ? d.entered : v })} onPick={(r) => update(pickAddress(d, r))} placeholder="1234 Williamson Rd, Orange Ave & Williamson Rd, or a place" />
      <div className="grid grid-cols-[1fr_1fr_auto] items-center gap-2">
        <input className="input h-7 font-mono text-xs" data-field="map" aria-label="Latitude" value={d.lat} onChange={(e) => update({ lat: e.target.value, verified: false })} placeholder="latitude" />
        <input className="input h-7 font-mono text-xs" aria-label="Longitude" value={d.lon} onChange={(e) => update({ lon: e.target.value, verified: false })} placeholder="longitude" />
        {verify !== 'verified' && (
          <button type="button" className="btn h-7 px-2 text-[11px]" disabled={!d.address && !d.lat} onClick={() => update({ verified: true, placeVerify: 'verified' })} title="The caller confirmed it and it is an address point on the map">Verify</button>
        )}
      </div>
      {call?.location?.lat != null && (
        <button type="button" className="btn btn-ghost h-6 px-1.5 text-[11px]" onClick={() => update({ lat: call.location!.lat!.toFixed(6), lon: call.location!.lon!.toFixed(6), address: call.location!.address ?? d.address, verified: false, placeVerify: call.location!.address ? '' : 'caller' })}>
          <MapPin size={11} aria-hidden /> Use the caller's location ({call.location.method ?? 'ALI'}{call.location.radius ? ` ±${Math.round(call.location.radius)} m` : ''})
        </button>
      )}
      <div className="grid grid-cols-2 gap-2">
        <label className="label" htmlFor="pv-taker">How the location is settled
          <select id="pv-taker" data-field="verify" className="input" value={verify} onChange={(e) => update({ placeVerify: e.target.value as Draft['placeVerify'], verified: e.target.value === 'verified', entered: d.entered || d.address })}>
            <option value="">Choose…</option>
            {(pol?.kinds ?? Object.keys(VERIFY_WORDS)).map((k) => <option key={k} value={k}>{VERIFY_WORDS[k] ?? k}</option>)}
          </select>
        </label>
        <label className="label" htmlFor="near-taker">Nearest verified address or cross streets
          <input id="near-taker" className="input" value={d.nearest ?? ''} onChange={(e) => update({ nearest: e.target.value })} placeholder="optional" />
        </label>
      </div>
      {verify === 'forced' && (
        <div className="space-y-1 rounded border border-rose-800 bg-rose-950/30 p-2">
          <p className="text-[11px] text-rose-100">Forced for this incident: the call goes now. The address as typed ({JSON.stringify(d.entered || d.address)}) and the map point are kept, and the supervisor is alerted to review it. Place the point on the map.</p>
          <label className="label" htmlFor="fr-taker">Why it is forced
            <select id="fr-taker" data-field="reason" className="input" value={d.forceReason ?? ''} onChange={(e) => update({ forceReason: e.target.value })}>
              <option value="">Choose…</option>
              {(pol?.reasons ?? []).map((r) => <option key={r} value={r}>{r}</option>)}
            </select>
          </label>
          {(pol?.noteAlways || (d.forceReason ?? '').endsWith('(explain)')) && (
            <label className="label" htmlFor="fn-taker">Explain<input id="fn-taker" data-field="reasonNote" className="input" value={d.forceNote ?? ''} onChange={(e) => update({ forceNote: e.target.value })} /></label>
          )}
        </div>
      )}
      <fieldset className="grid grid-cols-2 gap-x-2 gap-y-1 lg:grid-cols-3">
        <legend className="label">The place</legend>
        <label className="label" htmlFor="pn-taker">Business or building name<input id="pn-taker" className="input" value={d.placeName ?? ''} onChange={(e) => update({ placeName: e.target.value })} /></label>
        <label className="label" htmlFor="cx-taker">Complex or campus<input id="cx-taker" className="input" value={d.complex ?? ''} onChange={(e) => update({ complex: e.target.value })} /></label>
        <label className="label" htmlFor="bl-taker">Building<input id="bl-taker" className="input" value={d.building ?? ''} onChange={(e) => update({ building: e.target.value })} placeholder="4, B, North" /></label>
        <label className="label" htmlFor="fl-taker">Floor<input id="fl-taker" className="input" value={d.floor ?? ''} onChange={(e) => update({ floor: e.target.value })} /></label>
        <div className="grid grid-cols-[auto_1fr] gap-1">
          <label className="label" htmlFor="uk-taker">Kind<select id="uk-taker" className="input" value={d.unitKind ?? ''} onChange={(e) => update({ unitKind: e.target.value })}>
            <option value="">unit</option>{(pol?.unitKinds ?? []).map((k) => <option key={k} value={k}>{k}</option>)}</select></label>
          <label className="label" htmlFor="un-taker">Number<input id="un-taker" className="input" value={d.unit ?? ''} onChange={(e) => update({ unit: e.target.value })} placeholder="204" /></label>
        </div>
        <label className="label" htmlFor="dt-taker">Entrance, gate, access<input id="dt-taker" className="input" value={d.detail ?? ''} onChange={(e) => update({ detail: e.target.value })} /></label>
      </fieldset>
      <p className="text-[11px] text-slate-500">Right-click the map to place the point, or pick a suggestion. A change here does not change the agency's address records.</p>
    </div>
  )
}
