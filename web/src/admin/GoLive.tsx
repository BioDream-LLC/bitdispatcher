import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { CheckCircle2, CircleAlert, CircleX, PlayCircle } from 'lucide-react'
import { api, errText } from '../api'
import type { GoLiveResult, SetupView } from '../types'
import { Page } from './Admin'

/** The go-live check: automatic tests before the first live call. None of them pages, texts or queries
 * anyone; test calls are worked out and thrown away. */
export default function GoLive() {
  const [g, setG] = useState<GoLiveResult | null>(null)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    api.get<SetupView>('/admin/setup').then((v) => setG(v.goLive ?? null)).catch(() => {})
  }, [])
  const run = async () => {
    setBusy(true)
    try {
      setG(await api.post<GoLiveResult>('/admin/golive'))
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Page title="Go-live check" intro="Automatic tests to run before the first live call, and again after any big change. They check the ledger and a restore of it, make a test call of every call type at every station (nothing is written and no unit is touched), and look at the certificate, the authenticators, the phone system, paging and the second site. Nothing is paged, texted or sent anywhere."
      actions={<button className="btn btn-go" onClick={run} disabled={busy}><PlayCircle size={13} aria-hidden /> {busy ? 'Checking…' : 'Run the check'}</button>}>
      {!g ? (
        <p className="text-sm text-slate-400">Not run yet.</p>
      ) : (
        <>
          <section className={`panel mb-4 rounded-lg p-4 text-sm ${g.fail ? 'border-l-2 border-red-500' : 'border-l-2 border-emerald-500'}`} data-testid="golive-summary">
            <div className="text-base font-semibold text-slate-100">{g.fail ? 'Not ready: fix what failed, then run it again' : g.warn ? 'Ready, with things to look at' : 'Ready'}</div>
            <div className="mt-1 text-xs text-slate-400">{g.pass} passed, {g.warn} to look at, {g.fail} failed. Run {new Date(g.at).toLocaleString()} by {g.by}; the result is in the ledger.</div>
          </section>
          <ul className="space-y-2">
            {g.checks.map((c) => (
              <li key={c.id} className="panel flex items-start gap-3 rounded-lg p-3 text-sm" data-testid={'check-' + c.id}>
                <span className="mt-0.5" aria-label={c.result === 'pass' ? 'Passed' : c.result === 'warn' ? 'To look at' : 'Failed'}>
                  {c.result === 'pass' ? <CheckCircle2 size={16} className="text-emerald-400" aria-hidden /> : c.result === 'warn' ? <CircleAlert size={16} className="text-amber-400" aria-hidden /> : <CircleX size={16} className="text-red-400" aria-hidden />}
                </span>
                <div className="min-w-0 flex-1">
                  <div className="font-semibold text-slate-100">{c.title}</div>
                  {c.detail && <p className="mt-0.5 text-xs text-slate-300">{c.detail}</p>}
                </div>
                {c.fix && c.result !== 'pass' && <Link className="btn h-7 shrink-0 py-0 text-xs" to={'/admin/' + c.fix}>Fix</Link>}
              </li>
            ))}
          </ul>
          <p className="mt-4 text-xs text-slate-500">The check cannot see copies of the data kept elsewhere, the phone system's own wiring, or the radio. Confirm those on the <Link className="text-sky-300 underline" to="/admin/setup">setup checklist</Link>.</p>
        </>
      )}
    </Page>
  )
}
