import { useEffect, useMemo, useRef, useState } from 'react'
import { Keyboard } from 'lucide-react'
import { api } from './api'
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
  const rows = useMemo(() => keys.profile.bindings.filter((b) => !b.screen || b.screen === screen)
    .filter((b) => !q || (b.key + ' ' + (keys.actions[b.action] ?? b.action)).toLowerCase().includes(q.toLowerCase())), [keys, screen, q])
  return (
    <Modal open onClose={onClose} title={`Keys: ${keys.profile.name}`} icon={<Keyboard size={16} aria-hidden />}>
      <p className="mb-2 text-xs text-slate-400">The agency's key map for this duty and screen{keys.locked ? ' (a trainee works it as set)' : ''}. A key does what the same command or button does. Ctrl+/ shows this map.</p>
      <input className="input mb-2" placeholder="Find a key or action" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Find a key or action" autoFocus />
      <table className="w-full text-sm">
        <thead><tr className="text-left text-[11px] uppercase text-slate-500"><th className="py-1">Key</th><th>Does</th></tr></thead>
        <tbody>
          {rows.map((b) => <tr key={b.key + b.screen} className="border-t border-slate-800"><td className="py-1"><span className="kbd">{b.key}</span></td><td>{b.label || keys.actions[b.action] || b.action}</td></tr>)}
        </tbody>
      </table>
    </Modal>
  )
}
