import { useEffect, useMemo, useRef, useState } from 'react'
import { CheckCircle2, Keyboard, XCircle } from 'lucide-react'
import { toast } from 'sonner'
import { api, errText } from './api'
import { useStore } from './store'
import { Modal } from './ui'
import type { KeyBinding, KeyProfile } from './types'

// Key profiles: the agency maps keys (and Shift+keys) to actions per duty and screen (Admin › Operations
// rules); the PremierOne-style layout is one of the stock profiles. A key does exactly what the same
// command or button does: the server checks the person's right either way. Ctrl+/ shows the map.

type Keys = { profile: KeyProfile; actions: Record<string, string>; locked: boolean }
let cache: { key: string; v: Keys } | null = null

export function useKeyProfile(): Keys | null {
  const me = useStore((s) => s.me)
  const k = me ? me.username + '|' + me.duty : ''
  const [v, setV] = useState<Keys | null>(cache?.key === k ? cache.v : null)
  useEffect(() => {
    if (!k || cache?.key === k) return
    api.get<Keys>('/me/keys').then((x) => { cache = { key: k, v: x }; setV(x) }).catch(() => {})
  }, [k])
  return v
}

/** keyName is a key press as profiles write it: F7, Shift+F11, Ctrl+K. */
export function keyName(e: KeyboardEvent): string {
  const parts: string[] = []
  if (e.ctrlKey || e.metaKey) parts.push('Ctrl')
  if (e.altKey) parts.push('Alt')
  if (e.shiftKey && (e.key.length > 1 || e.ctrlKey || e.altKey)) parts.push('Shift')
  parts.push(e.key.length === 1 ? e.key.toUpperCase() : e.key)
  return parts.join('+')
}

export function bindingFor(p: KeyProfile | undefined, key: string, screen: string): KeyBinding | undefined {
  if (!p) return undefined
  return p.bindings.find((b) => b.key === key && b.screen === screen) ?? p.bindings.find((b) => b.key === key && !b.screen)
}

/** The key currently being acted on, for the command line to tell the server which key sent a command. */
export let lastKey = ''

/** KeyMapHost runs the profile's keys on a screen: an action this screen handles takes the key before
 * anything else on the page; others are left to the screen's own keys. */
export function KeyMapHost({ screen, handlers }: { screen: 'console' | 'taker'; handlers: Partial<Record<string, () => void>> }) {
  const keys = useKeyProfile()
  const [map, setMap] = useState(false)
  const ref = useRef(handlers)
  ref.current = handlers
  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      const name = keyName(e)
      if (name === 'Ctrl+/') {
        e.preventDefault()
        setMap((m) => !m)
        return
      }
      if (!/^(F\d{1,2}|.*\+F\d{1,2}|Ctrl\+.|Alt\+.)$/.test(name)) return
      const b = bindingFor(keys?.profile, name, screen)
      const h = b && ref.current[b.action]
      if (!b || !h) return
      if (document.querySelector('[role=dialog]') && b.action !== 'help') return
      e.preventDefault()
      e.stopPropagation()
      lastKey = name
      h()
      setTimeout(() => { lastKey = '' }, 0)
    }
    window.addEventListener('keydown', on, true)
    return () => window.removeEventListener('keydown', on, true)
  }, [keys, screen])
  if (!map || !keys) return null
  return <KeyMap keys={keys} screen={screen} onClose={() => setMap(false)} />
}

function KeyMap({ keys, screen, onClose }: { keys: Keys; screen: string; onClose: () => void }) {
  const [q, setQ] = useState('')
  const duty = useStore((s) => s.me?.duty)
  const rows = useMemo(() => keys.profile.bindings.filter((b) => !b.screen || b.screen === screen)
    .filter((b) => !q || (b.key + ' ' + (keys.actions[b.action] ?? b.action)).toLowerCase().includes(q.toLowerCase())), [keys, screen, q])
  return (
    <Modal open onClose={onClose} title={`Keys: ${keys.profile.name}`} icon={<Keyboard size={16} aria-hidden />}>
      <p className="mb-2 text-xs text-slate-400">The agency's key map for this duty and screen{keys.locked ? ' (a trainee works it as set)' : ''}. A key does what the same command or button does. Ctrl+/ shows this map.</p>
      <div className="mb-2 flex gap-2">
        <input className="input" placeholder="Find a key or action" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Find a key or action" autoFocus />
        <button type="button" className="btn h-8 shrink-0 text-xs" onClick={() => { onClose(); window.dispatchEvent(new Event('bd:keytest')) }}>Test the keys here</button>
      </div>
      <table className="w-full text-sm">
        <thead><tr className="text-left text-[11px] uppercase text-slate-500"><th className="py-1">Key</th><th>Does</th></tr></thead>
        <tbody>
          {rows.map((b) => {
            const t = keyTestResult(duty, b.key)
            return <tr key={b.key + b.screen} className="border-t border-slate-800"><td className="py-1"><span className="kbd">{t && !t.ok ? 'Fn+' + b.key : b.key}</span></td><td>{b.label || keys.actions[b.action] || b.action}{t && !t.ok && <span className="ml-1 text-[11px] text-amber-300">(did not reach bitdispatcher in this workstation's test: {t.fix})</span>}</td></tr>
          })}
        </tbody>
      </table>
    </Modal>
  )
}

// ---- the workstation key test ----

type TestResult = { key: string; action: string; ok: boolean; got?: string; fix?: string }
const TEST_KEY = (duty?: string) => 'bd.keytest.' + (duty ?? '')

/** keyTestResult is what this workstation's last key test found for a key, if it was tested. */
export function keyTestResult(duty: string | undefined, key: string): TestResult | undefined {
  try {
    const r = JSON.parse(localStorage.getItem(TEST_KEY(duty)) ?? 'null') as { results: TestResult[] } | null
    return r?.results.find((x) => x.key === key)
  } catch {
    return undefined
  }
}

/** keyTested says whether this workstation has tested its keys for the duty. */
export function keyTested(duty?: string): boolean {
  return !!localStorage.getItem(TEST_KEY(duty))
}

const HARDWARE: Record<string, string> = {
  AudioVolumeDown: 'volume down', AudioVolumeUp: 'volume up', AudioVolumeMute: 'mute', MicrophoneToggle: 'the microphone', BrightnessDown: 'brightness down',
  BrightnessUp: 'brightness up', MediaPlayPause: 'play or pause', MediaTrackNext: 'next track', MediaTrackPrevious: 'previous track', LaunchApplication1: 'an application key',
}

/** KeyTestHost runs the key test when asked (bd:keytest): each published key of the duty's profile is
 * pressed in turn; a key that does not reach the browser, or arrives as a laptop function such as volume
 * or the microphone, is recorded with what to use instead. The result is kept here and on the server. */
export function KeyTestHost() {
  const keys = useKeyProfile()
  const me = useStore((s) => s.me)
  const [open, setOpen] = useState(false)
  const [i, setI] = useState(0)
  const [results, setResults] = useState<TestResult[]>([])
  const list = useMemo(() => {
    if (!keys) return [] as { key: string; action: string }[]
    const seen = new Set<string>()
    const out: { key: string; action: string }[] = []
    for (const b of keys.profile.bindings) {
      if (!/F\d{1,2}$/.test(b.key) || seen.has(b.key)) continue
      seen.add(b.key)
      out.push({ key: b.key, action: b.label || keys.actions[b.action] || b.action })
    }
    return out
  }, [keys])
  useEffect(() => {
    const on = () => { setOpen(true); setI(0); setResults([]) }
    window.addEventListener('bd:keytest', on)
    return () => window.removeEventListener('bd:keytest', on)
  }, [])
  const cur = list[i]
  const record = (r: TestResult) => {
    const next = [...results.filter((x) => x.key !== r.key), r]
    setResults(next)
    if (i + 1 < list.length) setI(i + 1)
    else finish(next)
  }
  const finish = async (rs: TestResult[]) => {
    localStorage.setItem(TEST_KEY(me?.duty), JSON.stringify({ at: new Date().toISOString(), results: rs }))
    try {
      await api.post('/me/keytest', { console: me?.console, duty: me?.duty, profile: keys?.profile.id, browser: navigator.userAgent.slice(0, 120), platform: navigator.platform, results: rs })
    } catch (e) {
      toast.error(errText(e))
    }
    const bad = rs.filter((x) => !x.ok).length
    if (bad) toast.warning(`${bad} key(s) did not reach bitdispatcher: the screen now shows Fn with them. See the key map (Ctrl+/).`)
    else toast.success('Every published key reached bitdispatcher on this workstation')
    setOpen(false)
  }
  useEffect(() => {
    if (!open || !cur) return
    const on = (e: KeyboardEvent) => {
      e.preventDefault()
      e.stopPropagation()
      const name = keyName(e)
      if (name === cur.key) return record({ ...cur, ok: true })
      if (HARDWARE[e.key]) return record({ ...cur, ok: false, got: e.key, fix: `Fn+${cur.key}: this keyboard sent ${HARDWARE[e.key]} instead; hold Fn, or turn on Fn Lock (often Fn+Esc)` })
      if (/^F\d{1,2}$/.test(e.key) || name.includes('+F')) return record({ ...cur, ok: false, got: name, fix: `this keyboard sent ${name}; check Fn Lock` })
    }
    window.addEventListener('keydown', on, true)
    return () => window.removeEventListener('keydown', on, true)
  }, [open, cur, results]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!open) return null
  return (
    <Modal open onClose={() => setOpen(false)} title="Test the keys on this workstation" icon={<Keyboard size={16} aria-hidden />}>
      {!keys ? <p className="text-sm text-slate-400">Loading the key map…</p> : !cur ? <p className="text-sm text-slate-400">This duty's key map has no function keys.</p> : (
        <div className="space-y-3 text-sm">
          <p className="text-xs text-slate-400">Key {i + 1} of {list.length} in "{keys.profile.name}" for {me?.duty}. Press the key below. Nothing is done while the test runs.</p>
          <div className="rounded-lg border border-slate-700 bg-slate-900 p-4 text-center">
            <div className="kbd mx-auto inline-block px-4 py-2 text-2xl">{cur.key}</div>
            <div className="mt-2 text-slate-300">{cur.action}</div>
          </div>
          <div className="flex gap-2">
            <button type="button" className="btn" onClick={() => record({ ...cur, ok: false, fix: `Fn+${cur.key}: nothing reached the browser; hold Fn, turn on Fn Lock, or ask for an alternate key in the agency's profile` })}>
              <XCircle size={14} aria-hidden /> Nothing happened (or the laptop did something else)
            </button>
            <button type="button" className="btn btn-ghost ml-auto" onClick={() => finish(results)}>Stop and keep what was tested</button>
          </div>
          <ul className="max-h-32 space-y-0.5 overflow-y-auto text-xs">
            {results.map((r) => <li key={r.key} className="flex items-center gap-1">{r.ok ? <CheckCircle2 size={12} className="text-emerald-400" aria-hidden /> : <XCircle size={12} className="text-rose-400" aria-hidden />} <span className="kbd">{r.key}</span> {r.ok ? 'works' : r.fix}</li>)}
          </ul>
        </div>
      )}
    </Modal>
  )
}
