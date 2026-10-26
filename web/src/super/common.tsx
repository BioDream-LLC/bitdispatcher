import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { toast } from 'sonner'
import { errText } from '../api'
import { cx } from '../util'

// Shared, small building blocks for the supervisor workspace. They keep the look of the rest of web/src
// (dark slate, btn / input / label / chip classes) and keep states in words, not colour alone (WCAG).

// usePoll runs a loader now and again every 10 seconds, and gives back a manual refresh.
export function usePoll(load: () => void | Promise<void>, deps: unknown[] = []): () => void {
  const saved = useRef(load)
  saved.current = load
  const refresh = useCallback(() => {
    void saved.current()
  }, [])
  useEffect(() => {
    refresh()
    const t = window.setInterval(refresh, 10_000)
    return () => window.clearInterval(t)
  }, deps)
  return refresh
}

// run wraps a write: it reports the error with a toast, so every form handles failures the same way.
export async function run(fn: () => Promise<unknown>, onOk?: () => void): Promise<boolean> {
  try {
    await fn()
    onOk?.()
    return true
  } catch (e) {
    toast.error(errText(e))
    return false
  }
}

// Busy is a submit button that shows it is working and cannot be pressed twice.
export function Busy({ children, onClick, disabled, go, ghost, className, title }: {
  children: ReactNode
  onClick: () => Promise<unknown> | unknown
  disabled?: boolean
  go?: boolean
  ghost?: boolean
  className?: string
  title?: string
}) {
  const [busy, setBusy] = useState(false)
  return (
    <button
      type="button"
      className={cx('btn', go && 'btn-go', ghost && 'btn-ghost', className)}
      disabled={busy || disabled}
      aria-busy={busy}
      title={title}
      onClick={async () => {
        setBusy(true)
        try {
          await onClick()
        } finally {
          setBusy(false)
        }
      }}
    >
      {busy ? 'Working…' : children}
    </button>
  )
}

// Panel is a titled section with the shared panel styling.
export function Panel({ title, icon, right, children, className }: {
  title?: ReactNode
  icon?: ReactNode
  right?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <section className={cx('panel rounded-lg', className)}>
      {(title || right) && (
        <div className="flex items-center gap-2 border-b border-slate-800 px-3 py-2">
          {icon}
          {title && <h3 className="text-sm font-semibold text-slate-200">{title}</h3>}
          {right && <div className="ml-auto flex items-center gap-2">{right}</div>}
        </div>
      )}
      <div className="p-3">{children}</div>
    </section>
  )
}

// Empty is the no-rows line.
export function Empty({ children }: { children: ReactNode }) {
  return <p className="px-1 py-3 text-sm text-slate-500">{children}</p>
}

// Tag is a plain chip; tone is a word label, never colour alone.
export function Tag({ children, tone, title }: { children: ReactNode; tone?: 'warn' | 'bad' | 'good' | 'info'; title?: string }) {
  const cls = {
    warn: 'bg-amber-500/20 text-amber-200',
    bad: 'bg-rose-500/20 text-rose-200',
    good: 'bg-emerald-500/20 text-emerald-200',
    info: 'bg-sky-500/20 text-sky-200',
  }[tone ?? 'info']
  return <span className={cx('chip', cls)} title={title}>{children}</span>
}

// Toggle is an "open only / show all" switch used on several lists.
export function AllToggle({ all, onChange }: { all: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex cursor-pointer items-center gap-1.5 text-xs text-slate-400">
      <input type="checkbox" checked={all} onChange={(e) => onChange(e.target.checked)} />
      Show all (not only open)
    </label>
  )
}

// NoteField is a labelled textarea.
export function NoteField({ id, label, value, onChange, rows = 2, hint, placeholder }: {
  id: string
  label: string
  value: string
  onChange: (v: string) => void
  rows?: number
  hint?: string
  placeholder?: string
}) {
  return (
    <div>
      <label className="label" htmlFor={id}>{label}</label>
      <textarea id={id} className="input w-full" rows={rows} value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} />
      {hint && <p className="mt-1 text-xs text-slate-500">{hint}</p>}
    </div>
  )
}

// TextField is a labelled single-line input.
export function TextField({ id, label, value, onChange, placeholder, type = 'text' }: {
  id: string
  label: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
  type?: string
}) {
  return (
    <div>
      <label className="label" htmlFor={id}>{label}</label>
      <input id={id} type={type} className="input w-full" value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} />
    </div>
  )
}

// SelectField is a labelled select.
export function SelectField({ id, label, value, onChange, options, placeholder }: {
  id: string
  label: string
  value: string
  onChange: (v: string) => void
  options: string[]
  placeholder?: string
}) {
  return (
    <div>
      <label className="label" htmlFor={id}>{label}</label>
      <select id={id} className="input w-full" value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">{placeholder ?? 'Choose…'}</option>
        {options.map((o) => (
          <option key={o} value={o}>{o}</option>
        ))}
      </select>
    </div>
  )
}

// ---- time helpers (kept local so this folder needs no shared util beyond cx) ----

export function dateTime(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d.getTime())) return '—'
  return d.toLocaleString([], { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false })
}

export function timeOnly(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d.getTime())) return '—'
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
}

export function dayOnly(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d.getTime())) return '—'
  return d.toLocaleDateString([], { year: 'numeric', month: 'short', day: 'numeric' })
}

// A zero time from Go marshals as this; treat it as empty.
export function isZeroTime(iso?: string): boolean {
  return !iso || iso.startsWith('0001-01-01')
}

// toRFC3339 turns a datetime-local value (local wall time) into an RFC3339 string the API expects.
export function toRFC3339(local: string): string {
  if (!local) return ''
  const d = new Date(local)
  if (isNaN(d.getTime())) return ''
  return d.toISOString()
}

// datetimeLocalNow is the current time formatted for a datetime-local input.
export function datetimeLocalNow(): string {
  const d = new Date()
  const off = d.getTimezoneOffset()
  const local = new Date(d.getTime() - off * 60000)
  return local.toISOString().slice(0, 16)
}

// seconds renders a computed interval plainly.
export function seconds(s?: number): string {
  if (s === undefined || s === null) return '—'
  if (s < 60) return `${Math.round(s)} s`
  const m = Math.floor(s / 60)
  const r = Math.round(s % 60)
  return `${m} min ${r} s`
}
