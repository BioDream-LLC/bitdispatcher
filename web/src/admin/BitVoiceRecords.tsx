import { useEffect, useState } from 'react'
import { Phone } from 'lucide-react'
import { api, errText } from '../api'
import type { View } from '../types'

type Answer = { calls: number; answered: number; abandoned: number; target: number; withinTarget: number; within20: number; average: number; longest: number; byTaker?: { ext: string; name: string; calls: number; average: number }[] }
type Exposure = { people?: { who: string; looks: number; calls: number; last: string }[] }
type I3Log = { day: string; items: { logEventId: string; received: string; from: string; signer: string; chained: boolean; type: string; callId?: string }[] }

const day = (iso: string) => new Date(iso).toLocaleDateString('en-CA')

/** BitVoiceRecords is call handling's side of the same period, from BitVoice: 911 answer times (the NENA
 * standard: 90% within 15 seconds), who looked at callers' photos and video, and the signed i3 log events
 * received. Shown beside the dispatch report; BitVoice's numbers are its own. */
export default function BitVoiceRecords({ view, from, to }: { view: View; from: string; to: string }) {
  const [ans, setAns] = useState<Answer | null>(null)
  const [exp, setExp] = useState<Exposure | null>(null)
  const [log, setLog] = useState<I3Log | null>(null)
  const [err, setErr] = useState('')
  const on = view.health.bitvoice.configured
  useEffect(() => {
    if (!on) return
    const q = `from=${day(from)}&to=${day(to)}`
    setErr('')
    api.get<Answer>(`/bitvoice/records/answer?${q}`).then(setAns).catch((e) => setErr(errText(e)))
    api.get<Exposure>(`/bitvoice/records/exposure?${q}`).then(setExp).catch(() => {})
    api.get<I3Log>(`/bitvoice/records/i3log?day=${day(to)}`).then(setLog).catch(() => {})
  }, [on, from, to])
  if (!on) return null
  const pct = (n: number) => `${Math.round(n)}%`
  return (
    <section className="panel mt-4 space-y-3 rounded-lg p-4" aria-labelledby="bvr-h">
      <h2 id="bvr-h" className="flex items-center gap-2 font-semibold text-slate-100"><Phone size={15} className="text-sky-300" aria-hidden /> Call handling (BitVoice), by day</h2>
      {err && <p className="text-xs text-amber-300">BitVoice did not answer: {err}</p>}
      {ans && (
        <div className="grid gap-2 text-sm sm:grid-cols-3 lg:grid-cols-6">
          {[['911 calls', ans.calls], ['Answered', ans.answered], ['Abandoned', ans.abandoned], [`Within ${ans.target} s`, pct(ans.withinTarget)], ['Within 20 s', pct(ans.within20)], ['Average answer', `${ans.average.toFixed(1)} s`]].map(([k, v]) => (
            <div key={String(k)} className="rounded border border-slate-800 p-2"><div className="text-[11px] text-slate-400">{k}</div><div className="font-mono text-lg text-slate-50">{v}</div></div>
          ))}
        </div>
      )}
      {ans && ans.withinTarget < 90 && ans.answered > 0 && <p className="text-xs text-amber-300">Below the NENA standard: 90% of 911 calls answered within {ans.target} seconds.</p>}
      {(ans?.byTaker ?? []).length > 0 && (
        <table className="text-xs"><thead><tr className="text-slate-400"><th className="pr-4 text-left">Call-taker</th><th className="pr-4 text-right">Calls</th><th className="text-right">Average answer</th></tr></thead>
          <tbody>{ans!.byTaker!.map((t) => <tr key={t.ext + t.name}><td className="pr-4">{t.name || '(not answered at a position)'}</td><td className="pr-4 text-right font-mono">{t.calls}</td><td className="text-right font-mono">{t.average.toFixed(1)} s</td></tr>)}</tbody>
        </table>
      )}
      {(exp?.people ?? []).length > 0 && (
        <div className="text-xs">
          <div className="text-slate-400">Who looked at callers' photos and video (BitVoice's exposure record)</div>
          <ul>{exp!.people!.map((p) => <li key={p.who}>{p.who}: {p.looks} look{p.looks === 1 ? '' : 's'} on {p.calls} call{p.calls === 1 ? '' : 's'}, last {new Date(p.last).toLocaleString()}</li>)}</ul>
          <div className="text-[10px] text-slate-500">Looks from bitdispatcher show here as its API token; who in bitdispatcher looked is in its own ledger.</div>
        </div>
      )}
      {log && (
        <div className="text-xs">
          <div className="text-slate-400">i3 log events received on {log.day} (NENA-STA-010, signed): {log.items.length}</div>
          {log.items.length > 0 && <ul className="max-h-40 overflow-y-auto font-mono text-[10px]">{log.items.slice(0, 100).map((e) => <li key={e.logEventId}>{new Date(e.received).toLocaleTimeString()} {e.type} {e.callId ?? ''} from {e.from} · {e.signer || 'unsigned'}{e.chained ? ' · traced to the PCA' : ''}</li>)}</ul>}
        </div>
      )}
    </section>
  )
}
