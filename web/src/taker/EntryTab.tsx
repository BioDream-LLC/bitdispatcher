import { useMemo, useState } from 'react'
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

/** Call Entry: the call type and modifiers, priority, the incident's location (verified on the map) and the
 * caller (and where they are, when not at the incident), with the call's ANI/ALI beside it. */
export default function EntryTab({ view, d, call, update, dups, onAttach, onDismissDups, map }: {
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
        <div>
          <label className="label" htmlFor="addr-taker">Incident location</label>
          <AddressInput id="addr-taker" value={d.address} onChange={(v) => update({ address: v, verified: false })} onPick={(r) => update(pickAddress(d, r))} placeholder="1234 Williamson Rd, Orange Ave & Williamson Rd, or a place" />
          <div className="mt-1 grid grid-cols-[1fr_1fr_auto] items-center gap-2">
            <input className="input h-7 font-mono text-xs" aria-label="Latitude" value={d.lat} onChange={(e) => update({ lat: e.target.value, verified: false })} placeholder="latitude" />
            <input className="input h-7 font-mono text-xs" aria-label="Longitude" value={d.lon} onChange={(e) => update({ lon: e.target.value, verified: false })} placeholder="longitude" />
            {d.verified ? (
              <span className="inline-flex items-center gap-1 text-[11px] font-semibold text-emerald-300"><CheckCircle2 size={12} aria-hidden /> verified</span>
            ) : (
              <button type="button" className="btn h-7 px-2 text-[11px]" disabled={!d.address && !d.lat} onClick={() => update({ verified: true })} title="The caller confirmed it and it is right on the map">Verify</button>
            )}
          </div>
          {call?.location?.lat != null && (
            <button type="button" className="btn btn-ghost mt-1 h-6 px-1.5 text-[11px]" onClick={() => update({ lat: call.location!.lat!.toFixed(6), lon: call.location!.lon!.toFixed(6), address: call.location!.address ?? d.address, verified: false })}>
              <MapPin size={11} aria-hidden /> Use the caller's location ({call.location.method ?? 'ALI'}{call.location.radius ? ` ±${Math.round(call.location.radius)} m` : ''})
            </button>
          )}
          <p className="mt-0.5 text-[11px] text-slate-500">Right-click the map to place it, or pick a suggestion. Cross streets show on the map.</p>
        </div>
        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className="label" htmlFor="cn-taker">Caller / reporting party</label>
            <input id="cn-taker" className="input" value={d.callerName} onChange={(e) => update({ callerName: e.target.value })} placeholder="LAST, FIRST" />
          </div>
          <div>
            <label className="label" htmlFor="cb-taker">Callback number</label>
            <input id="cb-taker" className="input font-mono" value={d.callback} onChange={(e) => update({ callback: e.target.value })} placeholder="+1 540 555 0100" />
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
            </dl>
          ) : <p className="px-2 py-1.5 text-xs text-slate-500">No call: a walk-in, an officer's report or a radio call.</p>}
        </div>
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
