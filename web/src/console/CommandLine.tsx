import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from 'react'
import { ChevronRight, Terminal } from 'lucide-react'
import { api, errText } from '../api'
import { can, store, useStore } from '../store'
import { cx, statusMeta } from '../util'
import { lastKey } from '../keys'
import { Modal } from '../ui'
import type { CommandResult, View } from '../types'

type Result = CommandResult & { line?: string }

export type CommandLineHandle = { focus: (prefill?: string) => void }

// The keyboard path for everything: D E1 M2, OS E1, N text, NEW SF 12 Main St. Results stay on screen.
const CommandLine = forwardRef<CommandLineHandle, { view: View }>(function CommandLine({ view }, ref) {
  const me = useStore((s) => s.me)
  const selected = useStore((s) => s.selected)
  const [line, setLine] = useState('')
  const [hist, setHist] = useState<string[]>(() => JSON.parse(sessionStorage.getItem('bd-hist') ?? '[]'))
  const [hi, setHi] = useState(-1)
  const [last, setLast] = useState<Result | null>(null)
  const [busy, setBusy] = useState(false)
  const input = useRef<HTMLInputElement>(null)
  useImperativeHandle(ref, () => ({
    focus: (prefill?: string) => {
      if (prefill !== undefined) setLine(prefill)
      input.current?.focus()
    },
  }))
  const words = line.trim().split(/\s+/)
  const verb = words[0]?.toUpperCase() ?? ''
  const v = view.verbs.find((x) => x.words.includes(verb))
  // Completion: verbs first, then units, call types or dispositions as the verb expects.
  const last1 = line.endsWith(' ') ? '' : (words[words.length - 1] ?? '').toUpperCase()
  let options: string[] = []
  if (words.length <= 1 && !line.endsWith(' ')) options = view.verbs.flatMap((x) => x.words).filter((w) => w.startsWith(last1))
  else if (['NEW', 'I', 'T', 'TYPE'].includes(verb) && words.length <= 2) options = view.callTypes.map((c) => c.code).filter((c) => c.startsWith(last1))
  else if (['CL', 'CLOSE'].includes(verb)) options = view.settings.dispositions.map((d) => d.toUpperCase()).filter((d) => d.startsWith(last1))
  else if (v && !['N', 'NOTE', 'PN', 'NEW', 'I', 'P', 'PRI', 'RO', 'REOPEN', 'ACK', 'HELP'].includes(verb)) options = view.units.map((u) => u.id).filter((id) => id.startsWith(last1) && !words.slice(1, -1).includes(id))
  options = options.slice(0, 12)
  const [form, setForm] = useState<{ unit: string } | null>(null)
  const run = async (prefix = '') => {
    let text = line.trim()
    // A function key supplies the command (F7 field initiate, F11 unit status) and processes the line.
    if (prefix && !text.toUpperCase().startsWith(prefix.trim() + ' ')) text = (prefix + text).trim()
    if (!text) return
    setBusy(true)
    const key = lastKey
    try {
      const r = await api.post<Result>('/command' + (key ? '?key=' + encodeURIComponent(key) : ''), { line: text, incident: selected ?? '' })
      setLast({ ...r, line: line.trim() })
      if (r.ok) {
        setLine('')
        if (r.incident !== undefined && r.incident !== selected) store.select(r.incident || null)
      }
      if (r.form === 'unit-status' && r.unit) setForm({ unit: r.unit })
      const h = [text, ...hist.filter((x) => x !== text)].slice(0, 50)
      setHist(h)
      sessionStorage.setItem('bd-hist', JSON.stringify(h))
      setHi(-1)
    } catch (e) {
      setLast({ ok: false, message: errText(e), line: text })
    } finally {
      setBusy(false)
    }
  }
  const runRef = useRef(run)
  runRef.current = run
  useEffect(() => {
    const on = (e: Event) => runRef.current((e as CustomEvent<string>).detail ?? '')
    window.addEventListener('bd:cmd-run', on)
    return () => window.removeEventListener('bd:cmd-run', on)
  }, [])
  if (!can.dispatch(me)) {
    return <div className="flex h-8 shrink-0 items-center border-t border-slate-800 bg-slate-950 px-3 text-[11px] text-slate-500">Viewing only: your role cannot change incidents.</div>
  }
  return (
    <div className="flex h-9 shrink-0 items-center gap-2 border-t border-slate-800 bg-slate-950 px-2">
      <Terminal size={14} className="text-slate-500" aria-hidden />
      <span className={cx('font-mono text-[11px]', selected ? 'text-sky-300' : 'text-slate-600')}>{selected ?? 'no incident'}</span>
      <ChevronRight size={12} className="text-slate-600" aria-hidden />
      <div className="relative flex-1">
        <input
          ref={input}
          className="w-full bg-transparent font-mono text-sm text-slate-50 outline-none placeholder:text-slate-600"
          value={line}
          spellCheck={false}
          autoComplete="off"
          aria-label="Command line"
          placeholder="Command: D E1 M2 · ER E1 · OS E1 · N note · NEW SF 12 Main St · REC · HELP      ( / to focus )"
          onChange={(e) => {
            setLine(e.target.value)
            setHi(-1)
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.ctrlKey) {
              e.preventDefault()
              run()
            } else if (e.key === 'Tab' && options.length) {
              e.preventDefault()
              const head = line.endsWith(' ') ? line : line.slice(0, line.length - last1.length)
              setLine(head + options[0] + ' ')
            } else if (e.key === 'ArrowUp') {
              e.preventDefault()
              const n = Math.min(hi + 1, hist.length - 1)
              if (n >= 0) {
                setHi(n)
                setLine(hist[n])
              }
            } else if (e.key === 'ArrowDown') {
              e.preventDefault()
              const n = hi - 1
              setHi(n)
              setLine(n >= 0 ? hist[n] : '')
            } else if (e.key === 'Escape') {
              setLine('')
              input.current?.blur()
            }
          }}
          disabled={busy}
        />
      </div>
      <div className="flex max-w-[45%] items-center gap-1 overflow-hidden">
        {last && !last.ok && last.line === line.trim() ? (
          // A command that failed stays on the line to be fixed: its reason shows until the line changes.
          <span role="status" className="truncate whitespace-pre font-mono text-[11px] text-rose-300" title={last.message}>✗ {last.message.split('\n')[0]}</span>
        ) : line && options.length > 0 ? (
          <span className="truncate font-mono text-[11px] text-slate-500">
            <span className="kbd mr-1">Tab</span>
            {options.join(' ')}
          </span>
        ) : v && line ? (
          <span className="truncate font-mono text-[11px] text-slate-400">{v.usage} — {v.meaning}</span>
        ) : last ? (
          <span role="status" className={cx('truncate whitespace-pre font-mono text-[11px]', last.ok ? 'text-emerald-300' : 'text-rose-300')} title={last.message}>
            {last.ok ? '✓ ' : '✗ '}
            {last.message.split('\n')[0]}
          </span>
        ) : null}
      </div>
      {form && <UnitStatusForm view={view} unit={form.unit} onClose={() => setForm(null)} />}
    </div>
  )
})

/** The unit status details form: a unit alone on the line opens it, so no status is guessed. */
function UnitStatusForm({ view, unit, onClose }: { view: View; unit: string; onClose: () => void }) {
  const codes = view.settings.commands?.statusCodes?.filter((c) => !c.retired) ?? [
    { code: 'ER', status: 'enroute' }, { code: 'AS', status: 'onscene' }, { code: 'TR', status: 'transporting' }, { code: 'AD', status: 'atdest' }, { code: 'AV', status: 'available' }, { code: 'OOS', status: 'oos' },
  ]
  const [note, setNote] = useState('')
  const [err, setErr] = useState('')
  const delim = view.settings.commands?.delim || '.'
  const go = async (code: string) => {
    try {
      const r = await api.post<CommandResult>('/command', { line: `US ${unit}${delim}${code}${note ? delim + delim + delim + delim + note : ''}` })
      if (!r.ok) return setErr(r.message)
      onClose()
    } catch (e) {
      setErr(errText(e))
    }
  }
  const u = view.units.find((x) => x.id === unit)
  return (
    <Modal open onClose={onClose} title={`Unit status: ${unit}`}>
      <p className="mb-2 text-xs text-slate-400">{u ? `${unit} is ${statusMeta(u.status).label}${u.incident ? ' on ' + u.incident : ''}.` : ''} Choose its new status.</p>
      <div className="flex flex-wrap gap-1" role="group" aria-label="Statuses">
        {codes.map((c) => <button key={c.code} className="btn h-8" onClick={() => go(c.code)}>{c.code} · {statusMeta(c.status).label}</button>)}
        {view.settings.markOuts?.filter((m) => !m.retired).map((m) => <button key={m.code} className="btn h-8" onClick={() => go(m.code)} title={m.name}>{m.code}</button>)}
      </div>
      <input className="input mt-2" placeholder="Comment (optional)" value={note} onChange={(e) => setNote(e.target.value)} aria-label="Comment" />
      {err && <p role="alert" className="mt-2 text-sm text-rose-300">{err}</p>}
    </Modal>
  )
}

export default CommandLine
