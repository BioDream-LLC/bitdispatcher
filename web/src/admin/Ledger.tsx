import { Fragment, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ShieldCheck, ShieldX, Search } from 'lucide-react'
import { api, errText } from '../api'
import { cx } from '../util'
import type { LedgerEvent } from '../types'
import { Page } from './Admin'

type Report = { events: number; ok: boolean; problem?: string; lastHash?: string; checked: string }

export default function Ledger() {
  const [items, setItems] = useState<LedgerEvent[]>([])
  const [type, setType] = useState('')
  const [actor, setActor] = useState('')
  const [q, setQ] = useState('')
  const [open, setOpen] = useState<number | null>(null)
  const [rep, setRep] = useState<Report | null>(null)
  const load = async () => {
    const p = new URLSearchParams({ type, actor, q, limit: '500' })
    try {
      setItems((await api.get<{ items: LedgerEvent[] }>(`/ledger?${p}`)).items)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  return (
    <Page
      title="Ledger and audit"
      intro="Every action, append-only. Each event carries the SHA-256 hash of the one before it, so a changed, removed or reordered event breaks the chain. Corrections are new events, never edits."
      actions={
        <button className="btn" onClick={async () => setRep(await api.get<Report>('/ledger/verify'))}>
          <ShieldCheck size={14} aria-hidden /> Verify the whole chain
        </button>
      }
    >
      {rep && (
        <div role="status" className={cx('mb-3 flex items-center gap-2 rounded-lg border p-3 text-sm', rep.ok ? 'border-emerald-800 bg-emerald-950/30 text-emerald-200' : 'border-rose-800 bg-rose-950/40 text-rose-200')}>
          {rep.ok ? <ShieldCheck size={16} aria-hidden /> : <ShieldX size={16} aria-hidden />}
          {rep.ok ? `All ${rep.events} events verify. Last hash ${rep.lastHash?.slice(0, 16)}…` : `The chain does not verify: ${rep.problem}`}
          <span className="ml-auto text-xs opacity-70">checked {new Date(rep.checked).toLocaleTimeString()}</span>
        </div>
      )}
      <form className="mb-3 flex flex-wrap gap-2" onSubmit={(e) => { e.preventDefault(); load() }}>
        <select className="input w-48" value={type} onChange={(e) => setType(e.target.value)} aria-label="Event type">
          <option value="">Every type</option>
          {['incident.', 'unit.', 'call.', 'radio.', 'session.', 'user.', 'settings.', 'integration.', 'calltype.', 'station.'].map((t) => <option key={t} value={t}>{t}*</option>)}
        </select>
        <input className="input w-40" placeholder="Actor" value={actor} onChange={(e) => setActor(e.target.value)} aria-label="Actor" />
        <input className="input w-64" placeholder="Incident, unit, call or text" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Search" />
        <button className="btn"><Search size={13} aria-hidden /> Search</button>
        <span className="self-center text-xs text-slate-500">{items.length} shown, newest first</span>
      </form>
      <div className="panel overflow-x-auto rounded-lg">
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b border-slate-800 text-left text-[10px] uppercase tracking-wider text-slate-500">
              {['#', 'Time', 'Type', 'Actor', 'Role', 'Position', 'Incident', 'Unit', 'Call', 'Hash'].map((h) => <th key={h} className="px-2 py-1.5 font-semibold">{h}</th>)}
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-800/60 font-mono">
            {items.map((e) => (
              <Fragment key={e.seq}>
                <tr className="cursor-pointer hover:bg-slate-800/50" onClick={() => setOpen(open === e.seq ? null : e.seq)}>
                  <td className="px-2 py-1 text-slate-500">{e.seq}</td>
                  <td className="px-2 whitespace-nowrap text-slate-300">{new Date(e.at).toLocaleString()}</td>
                  <td className="px-2 text-sky-300">{e.type}</td>
                  <td className="px-2 text-slate-200">{e.actor}</td>
                  <td className="px-2 text-slate-400">{e.role}</td>
                  <td className="px-2 text-slate-400">{e.console}</td>
                  <td className="px-2 text-slate-300">{e.incident}</td>
                  <td className="px-2 text-slate-300">{e.unit}</td>
                  <td className="max-w-32 truncate px-2 text-slate-400">{e.call}</td>
                  <td className="px-2 text-slate-600">{e.hash.slice(0, 10)}</td>
                </tr>
                {open === e.seq && (
                  <tr>
                    <td colSpan={10} className="bg-slate-950 px-3 py-2">
                      <pre className="max-h-72 overflow-auto whitespace-pre-wrap text-[11px] text-slate-300">{JSON.stringify(e, null, 2)}</pre>
                    </td>
                  </tr>
                )}
              </Fragment>
            ))}
          </tbody>
        </table>
      </div>
    </Page>
  )
}
