import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ArrowLeft, Download, Printer, ShieldCheck, ShieldX } from 'lucide-react'
import { api, ApiError, errText } from '../api'
import type { ReviewDoc } from '../types'
import { Logo } from '../ui'
import { cx } from '../util'

type View = 'initial' | 'summary' | 'detailed' | 'audit'

const TABS: { key: View; label: string }[] = [
  { key: 'initial', label: 'As first entered' },
  { key: 'summary', label: 'Summary' },
  { key: 'detailed', label: 'Detailed' },
  { key: 'audit', label: 'Full audit' },
]

/** Incident review: the same incident read four ways, each opening recorded in the ledger. */
export default function Review() {
  const { number } = useParams()
  const nav = useNavigate()
  const [view, setView] = useState<View>('initial')
  const [doc, setDoc] = useState<ReviewDoc | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [open, setOpen] = useState('')

  useEffect(() => {
    if (!number) return
    let live = true
    setLoading(true)
    setError(null)
    api
      .get<ReviewDoc>(`/review/${encodeURIComponent(number)}?view=${view}`)
      .then((d) => {
        if (!live) return
        setDoc(d)
      })
      .catch((e) => {
        if (!live) return
        setDoc(null)
        setError(e instanceof ApiError && e.status === 403 ? 'Reading a review needs the records duty, a supervisor or an administrator.' : errText(e))
      })
      .finally(() => {
        if (live) setLoading(false)
      })
    return () => {
      live = false
    }
  }, [number, view])

  const exportHref = number ? `./api/review/${encodeURIComponent(number)}?view=${view}&export=text` : '#'

  return (
    <div className="flex h-full flex-col bg-slate-950 print:bg-white print:text-black">
      <header className="flex items-center gap-2 border-b border-slate-800 bg-slate-900 px-3 py-2 print:hidden">
        <Link to="/" className="flex items-center gap-2 rounded px-2 py-1 text-sm font-semibold text-slate-200 hover:bg-slate-800">
          <ArrowLeft size={14} aria-hidden /> <Logo size={18} /> Dispatch
        </Link>
        <h1 className="ml-1 font-mono text-sm font-bold text-slate-100">Review {number}</h1>
        <form
          className="ml-auto flex items-center gap-1"
          onSubmit={(e) => {
            e.preventDefault()
            const n = open.trim()
            if (n) {
              setOpen('')
              nav(`/review/${encodeURIComponent(n)}`)
            }
          }}
        >
          <label className="sr-only" htmlFor="review-open">Open another incident</label>
          <input id="review-open" className="input h-7 w-40 text-xs" placeholder="Open incident number" value={open} onChange={(e) => setOpen(e.target.value)} />
          <button className="btn h-7 text-xs" type="submit" disabled={!open.trim()}>Open</button>
        </form>
        <a className="btn h-7 text-xs" href={exportHref} download aria-label="Download this view as text">
          <Download size={13} aria-hidden /> Download text
        </a>
        <button className="btn h-7 text-xs" type="button" onClick={() => window.print()}>
          <Printer size={13} aria-hidden /> Print
        </button>
      </header>

      <nav className="flex gap-1 border-b border-slate-800 bg-slate-900 px-3 pt-2 print:hidden" aria-label="Review views">
        {TABS.map((t) => (
          <button
            key={t.key}
            type="button"
            aria-current={view === t.key ? 'page' : undefined}
            onClick={() => setView(t.key)}
            className={cx(
              'rounded-t-md border border-b-0 px-3 py-1.5 text-xs font-semibold',
              view === t.key ? 'border-slate-700 bg-slate-950 text-slate-50' : 'border-transparent text-slate-400 hover:bg-slate-800/60 hover:text-slate-200',
            )}
          >
            {t.label}
          </button>
        ))}
      </nav>

      <main className="min-h-0 flex-1 overflow-y-auto p-4 print:overflow-visible print:p-0">
        <div className="mx-auto max-w-4xl">
          {loading ? (
            <p className="text-sm text-slate-500">Loading…</p>
          ) : error ? (
            <p className="rounded border border-rose-800 bg-rose-950/30 p-3 text-sm text-rose-200">{error}</p>
          ) : doc ? (
            <article className="space-y-4">
              <div>
                <h2 className="text-xl font-bold text-slate-50 print:text-black">{doc.title}</h2>
                <p className="text-xs text-slate-500">Generated {new Date(doc.generated).toLocaleString()}</p>
              </div>

              {view === 'audit' && doc.chain && (
                <div
                  className={cx(
                    'flex items-center gap-2 rounded border p-2 text-sm',
                    doc.chain.ok ? 'border-emerald-800 bg-emerald-950/30 text-emerald-200' : 'border-rose-800 bg-rose-950/30 text-rose-200',
                  )}
                >
                  {doc.chain.ok ? <ShieldCheck size={16} aria-hidden /> : <ShieldX size={16} aria-hidden />}
                  {doc.chain.ok ? (
                    <span>Ledger hash chain verified: {doc.chain.events} events.</span>
                  ) : (
                    <span>Ledger hash chain broken: {doc.chain.problem ?? 'the chain did not verify'}.</span>
                  )}
                </div>
              )}

              {doc.sections.map((s, i) => (
                <section key={i} className="panel rounded-lg p-3 print:border-slate-300 print:bg-white">
                  <h3 className="mb-2 text-[11px] font-bold uppercase tracking-wider text-slate-400 print:text-black">{s.heading}</h3>
                  {s.lines.length === 0 ? (
                    <p className="text-sm text-slate-500">Nothing recorded.</p>
                  ) : (
                    <ul className={cx('space-y-0.5', view === 'audit' ? 'font-mono text-[11px] leading-relaxed text-slate-300 print:text-black' : 'text-sm text-slate-200 print:text-black')}>
                      {s.lines.map((l, j) => (
                        <li key={j} className="whitespace-pre-wrap break-words">{l}</li>
                      ))}
                    </ul>
                  )}
                </section>
              ))}

              <p className="text-xs text-slate-500 print:hidden">Opening and exporting a view are recorded in the ledger.</p>
            </article>
          ) : (
            <p className="text-sm text-slate-500">No review to show.</p>
          )}
          <div className="mt-4 print:hidden">
            <Link to="/" className="text-xs text-sky-300 underline">Back to dispatch</Link>
          </div>
        </div>
      </main>
    </div>
  )
}
