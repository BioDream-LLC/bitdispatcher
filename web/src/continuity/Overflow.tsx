import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { ArrowRightLeft, Check, Clock, Send, Truck, X } from 'lucide-react'
import { api, errText } from '../api'
import { useStore } from '../store'
import { ago, cx, useNow } from '../util'
import type { OverflowCall, OverflowInfo, OverflowSession } from './types'

const hm = (t?: string) => (t ? new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '')

/** Overflow: asking a neighbouring centre to take our calls, answering theirs, and, while we help one,
 * entering its calls and following what becomes of them. */
export default function Overflow() {
  const now = useNow()
  const seq = useStore((s) => s.view?.seq)
  const [info, setInfo] = useState<OverflowInfo | null>(null)
  const load = useCallback(() => {
    api.get<OverflowInfo>('/overflow').then(setInfo).catch((e) => toast.error(errText(e)))
  }, [])
  useEffect(() => {
    load()
    const t = setInterval(load, 10000)
    return () => clearInterval(t)
  }, [load, seq])
  if (!info) return <div className="grid h-full place-items-center text-sm text-slate-500">Loading overflow…</div>

  const sess = (dir: 'in' | 'out', p: string) => info.sessions.find((s) => s.direction === dir && s.partner === p && (s.status === 'asked' || s.status === 'active') && Date.parse(s.until) > now)
  const helping = info.sessions.filter((s) => s.direction === 'in' && s.status === 'active' && Date.parse(s.until) > now)
  return (
    <div className="flex h-full flex-col overflow-hidden">
      <header className="panel-head flex items-center gap-3 px-4 py-2">
        <a href="#/" className="btn btn-ghost h-6 px-1.5 text-xs">← Dispatch</a>
        <h1 className="flex items-center gap-2 text-sm font-semibold text-slate-100"><ArrowRightLeft size={15} aria-hidden /> Overflow</h1>
        <span className="text-xs text-slate-500">Neighbouring centres that run bitdispatcher take each other's calls. Every action is recorded at both centres.</span>
        <Link to="/continuity" className="btn btn-ghost ml-auto h-6 px-1.5 text-xs">Continuity</Link>
      </header>
      <div className="flex-1 space-y-4 overflow-auto p-4">
        {info.partners.length === 0 && (
          <p className="panel rounded-lg p-4 text-sm text-slate-400">No neighbouring centre is set up for overflow. An administrator sets it up in Admin › Continuity, after the neighbour is added in Admin › Integrations › Neighbouring agencies, and both centres have agreed it in writing.</p>
        )}
        <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-3" aria-label="Neighbouring centres">
          {info.partners.map((p) => (
            <PartnerCard key={p.id} p={p} out={sess('out', p.id)} inn={sess('in', p.id)} supervisor={info.supervisor} minutes={info.minutes} onDone={load} />
          ))}
        </section>
        {helping.map((s) => <TakeCall key={s.partner} s={s} onDone={load} />)}
        {info.calls.length > 0 && (
          <section className="panel rounded-lg p-4" aria-labelledby="calls-h">
            <h2 id="calls-h" className="mb-2 font-semibold text-slate-100">Calls we took for neighbours</h2>
            <div className="space-y-2">{info.calls.map((c) => <CallCard key={c.ref} c={c} s={info.sessions.find((x) => x.direction === 'in' && x.partner === c.partner)} onDone={load} now={now} />)}</div>
          </section>
        )}
        {info.takenForUs.length > 0 && (
          <section className="panel rounded-lg p-4" aria-labelledby="ours-h">
            <h2 id="ours-h" className="mb-2 font-semibold text-slate-100">Our incidents a neighbour took</h2>
            <table className="w-full text-sm">
              <thead><tr className="text-left text-xs text-slate-500"><th>Incident</th><th>Type</th><th>Status</th><th>Taken by</th><th>Created</th></tr></thead>
              <tbody>
                {info.takenForUs.map((t) => (
                  <tr key={t.number} className="border-t border-slate-800">
                    <td className="py-1 font-mono">{t.number}</td><td>{t.type}</td><td>{t.status}</td><td>{t.by}</td><td>{hm(t.created)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
        )}
      </div>
    </div>
  )
}

function PartnerCard({ p, out, inn, supervisor, minutes, onDone }: {
  p: OverflowInfo['partners'][number]; out?: OverflowSession; inn?: OverflowSession; supervisor: boolean; minutes: number; onDone: () => void
}) {
  const [note, setNote] = useState('')
  const [mins, setMins] = useState(minutes)
  const [busy, setBusy] = useState(false)
  const act = async (path: string, body: unknown, ok: string) => {
    setBusy(true)
    try {
      const r = await api.post<{ warning?: string }>(`/overflow/${p.id}/${path}`, body)
      if (r && typeof r === 'object' && 'warning' in r && r.warning) toast.warning(r.warning)
      else toast.success(ok)
      onDone()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="panel space-y-2 rounded-lg p-3">
      <div className="flex items-baseline gap-2">
        <h2 className="font-semibold text-slate-100">{p.name}</h2>
        <span className="font-mono text-[11px] text-slate-500">{p.id}</span>
      </div>
      <p className="text-[11px] text-slate-500">
        {p.ask ? 'We may ask them to take our calls' : 'We do not ask them'}{p.ask && p.dispatch ? ', and assign our units' : ''}. {p.help ? `We take their calls${p.autoAccept ? ' (accepted on its own)' : ''}.` : 'We do not take their calls.'}
      </p>
      {p.problem && <p className="text-xs text-rose-300">{p.problem}</p>}
      {out && (
        <div className={cx('rounded border p-2 text-xs', out.status === 'active' ? 'border-sky-700 bg-sky-950/30 text-sky-100' : 'border-slate-700 text-slate-300')}>
          {out.status === 'active' ? `${p.name} is taking our calls until ${hm(out.until)}` : `Asked ${p.name} at ${hm(out.since)}; waiting for their supervisor`}
          {out.note ? ` · ${out.note}` : ''}
          {supervisor && <button className="btn ml-2 h-6 px-2 text-[11px]" disabled={busy} onClick={() => act('end', { direction: 'out' }, 'Overflow ended')}><X size={11} aria-hidden /> End</button>}
        </div>
      )}
      {inn && (
        <div className={cx('rounded border p-2 text-xs', inn.status === 'asked' ? 'border-amber-600 bg-amber-950/30 text-amber-100' : 'border-sky-700 bg-sky-950/30 text-sky-100')}>
          {inn.status === 'asked' ? `${p.name} asks us to take their calls until ${hm(inn.until)}` : `We take ${p.name}'s calls until ${hm(inn.until)}`}
          {inn.note ? ` · ${inn.note}` : ''}{inn.dispatch ? ' · we may assign their units' : ''}
          {supervisor && inn.status === 'asked' && (
            <span className="ml-2 inline-flex gap-1">
              <button className="btn btn-go h-6 px-2 text-[11px]" disabled={busy} onClick={() => act('answer', { accept: true }, 'Accepted')}><Check size={11} aria-hidden /> Accept</button>
              <button className="btn h-6 px-2 text-[11px]" disabled={busy} onClick={() => act('answer', { accept: false }, 'Declined')}>Decline</button>
            </span>
          )}
          {supervisor && inn.status === 'active' && <button className="btn ml-2 h-6 px-2 text-[11px]" disabled={busy} onClick={() => act('end', { direction: 'in' }, 'Overflow ended')}><X size={11} aria-hidden /> End</button>}
        </div>
      )}
      {supervisor && p.ask && !out && !p.problem && (
        <div className="space-y-1">
          <input className="input h-8 w-full text-sm" placeholder="Why (they see this): storm, 40 calls waiting" value={note} onChange={(e) => setNote(e.target.value)} aria-label={`Why we ask ${p.name}`} />
          <div className="flex items-center gap-2 text-xs text-slate-400">
            <label className="flex items-center gap-1">For <input type="number" min={15} max={10080} className="input h-7 w-20 text-xs" value={mins} onChange={(e) => setMins(Number(e.target.value))} /> minutes</label>
            <button className="btn btn-danger ml-auto h-8 text-xs" disabled={busy} onClick={() => act('ask', { note, minutes: mins }, `Asked ${p.name} to take our calls`)}>Ask {p.name} to take our calls</button>
          </div>
        </div>
      )}
    </div>
  )
}

function TakeCall({ s, onDone }: { s: OverflowSession; onDone: () => void }) {
  const types = s.callTypes ?? []
  const [f, setF] = useState({ type: types[0]?.code ?? '', priority: 0, address: '', callerName: '', callback: '', narrative: '' })
  const [busy, setBusy] = useState(false)
  const set = (p: Partial<typeof f>) => setF({ ...f, ...p })
  const send = async () => {
    setBusy(true)
    try {
      const c = await api.post<OverflowCall>(`/overflow/${s.partner}/calls`, f)
      if (c.delivered) toast.success(`Sent: ${s.partnerName} incident ${c.number}`)
      else toast.warning(`Kept here and tried again every 20 seconds: ${c.error}. Tell ${s.partnerName} by radio or phone if it is urgent.`)
      setF({ ...f, address: '', callerName: '', callback: '', narrative: '', priority: 0 })
      onDone()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <section className="panel space-y-2 rounded-lg border border-sky-800 p-4" aria-labelledby={`take-${s.partner}`}>
      <h2 id={`take-${s.partner}`} className="font-semibold text-sky-100">Take a call for {s.partnerName}</h2>
      <p className="text-xs text-slate-400">The call becomes an incident in {s.partnerName}'s CAD, under their numbers and call types. {s.dispatch ? 'You may also assign their units below.' : 'Their dispatchers send their units.'}</p>
      <div className="grid gap-2 md:grid-cols-[2fr_1fr]">
        <label className="text-xs text-slate-400">Location
          <input aria-label="Location" className="input mt-0.5 w-full text-sm" value={f.address} onChange={(e) => set({ address: e.target.value })} placeholder="Address, intersection or landmark, with the town" />
        </label>
        <label className="text-xs text-slate-400">Call type
          <select aria-label="Call type" className="input mt-0.5 w-full text-sm" value={f.type} onChange={(e) => set({ type: e.target.value })}>
            {types.map((t) => <option key={t.code} value={t.code}>{t.name} ({t.code}, P{t.priority})</option>)}
          </select>
        </label>
        <label className="text-xs text-slate-400">Caller
          <input aria-label="Caller" className="input mt-0.5 w-full text-sm" value={f.callerName} onChange={(e) => set({ callerName: e.target.value })} />
        </label>
        <label className="text-xs text-slate-400">Callback
          <input aria-label="Callback" className="input mt-0.5 w-full text-sm" value={f.callback} onChange={(e) => set({ callback: e.target.value })} inputMode="tel" />
        </label>
        <label className="text-xs text-slate-400 md:col-span-2">What happened
          <textarea aria-label="What happened" className="input mt-0.5 w-full text-sm" rows={3} value={f.narrative} onChange={(e) => set({ narrative: e.target.value })} />
        </label>
      </div>
      <div className="flex items-center gap-2">
        <label className="text-xs text-slate-400">Priority
          <select aria-label="Priority" className="input ml-1 h-7 text-xs" value={f.priority} onChange={(e) => set({ priority: Number(e.target.value) })}>
            <option value={0}>the call type's</option>{[1, 2, 3, 4, 5].map((n) => <option key={n} value={n}>{n}</option>)}
          </select>
        </label>
        <button className="btn btn-go ml-auto h-8 text-xs" disabled={busy || !f.type || !f.address.trim()} onClick={send}><Send size={13} aria-hidden /> Send to {s.partnerName}</button>
      </div>
    </section>
  )
}

function CallCard({ c, s, onDone, now }: { c: OverflowCall; s?: OverflowSession; onDone: () => void; now: number }) {
  const [note, setNote] = useState('')
  const [units, setUnits] = useState<{ id: string; type: string; status: string; station: string }[] | null>(null)
  const [busy, setBusy] = useState(false)
  const run = async (f: () => Promise<unknown>, ok: string) => {
    setBusy(true)
    try { await f(); toast.success(ok); onDone() } catch (e) { toast.error(errText(e)) } finally { setBusy(false) }
  }
  const canDispatch = !!s?.dispatch && s.status === 'active' && c.delivered && !c.closed
  return (
    <div className={cx('rounded border p-2 text-sm', !c.delivered ? 'border-amber-600 bg-amber-950/20' : c.closed ? 'border-slate-800 opacity-70' : 'border-slate-700')}>
      <div className="flex flex-wrap items-baseline gap-2">
        <span className="font-mono text-xs text-slate-400">{hm(c.taken)}</span>
        <span className="font-semibold text-slate-100">{c.typeName || c.type}</span>
        <span className="text-slate-300">{c.address}</span>
        <span className="text-xs text-slate-500">taken by {c.by}</span>
        <span className="ml-auto text-xs">
          {c.delivered ? <span className="text-emerald-300">{c.number} · {c.closed ? `closed${c.disposition ? ': ' + c.disposition : ''}` : c.status || 'received'}</span>
            : <span className="inline-flex items-center gap-1 text-amber-200"><Clock size={11} aria-hidden /> not delivered yet ({c.tries ?? 0} tries): {c.error}</span>}
        </span>
      </div>
      {(c.units ?? []).length > 0 && <p className="mt-1 text-xs text-slate-300">Units: {(c.units ?? []).map((u) => `${u.unit} ${u.status}`).join(', ')}</p>}
      {(c.notes ?? []).length > 0 && (
        <ul className="mt-1 space-y-0.5 text-xs">
          {(c.notes ?? []).map((n, i) => <li key={i} className={n.theirs ? 'text-sky-200' : 'text-slate-400'}>{hm(n.at)} {n.by}: {n.text}{!n.delivered && !n.theirs ? ' (not sent yet)' : ''}</li>)}
        </ul>
      )}
      {!c.closed && (
        <div className="mt-1 flex gap-2">
          <input className="input h-7 flex-1 text-xs" placeholder="Add a note for them" value={note} onChange={(e) => setNote(e.target.value)} aria-label="Note" />
          <button className="btn h-7 text-xs" disabled={busy || !note.trim()} onClick={() => run(() => api.post(`/overflow/calls/${c.ref}/notes`, { text: note }), 'Note added').then(() => setNote(''))}>Add</button>
          {canDispatch && <button className="btn h-7 text-xs" disabled={busy} onClick={() => api.get<typeof units>(`/overflow/${c.partner}/units`).then(setUnits).catch((e) => toast.error(errText(e)))}><Truck size={12} aria-hidden /> Their units</button>}
        </div>
      )}
      {units && canDispatch && (
        <div className="mt-1 flex flex-wrap gap-1">
          {units.filter((u) => u.status === 'available').map((u) => (
            <button key={u.id} className="btn h-6 px-2 text-[11px]" disabled={busy} title={`${u.type}${u.station ? ' · ' + u.station : ''}`}
              onClick={() => run(() => api.post(`/overflow/calls/${c.ref}/dispatch`, { units: [u.id] }), `${u.id} assigned`).then(() => setUnits(null))}>{u.id}</button>
          ))}
          {units.filter((u) => u.status === 'available').length === 0 && <span className="text-xs text-slate-500">No unit of theirs is available.</span>}
        </div>
      )}
      <span className="sr-only">{ago(c.updated, now)}</span>
    </div>
  )
}
