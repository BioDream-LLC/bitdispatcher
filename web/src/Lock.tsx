import { useEffect, useRef, useState } from 'react'
import { Lock as LockIcon, LogOut, Unlock } from 'lucide-react'
import { api, ApiError, errText } from './api'
import { checkMe, signOut, store, useStore } from './store'
import { Logo } from './ui'

/** Locks this console now: everything stays open underneath, nothing is shown until its person unlocks. */
export async function lockNow() {
  await api.post('/auth/lock').catch(() => {})
  const me = store.get().me
  if (me) store.set({ me: { ...me, locked: true } })
}

/** The lock screen, and the console's inactivity lock and input heartbeat (the server enforces both). */
export function LockGuard() {
  const me = useStore((s) => s.me)
  const lockMinutes = useStore((s) => s.view?.settings.security?.lockMinutes ?? 0)
  const last = useRef(Date.now())
  const sent = useRef(0)
  // A request refused because the console is locked shows the lock screen.
  useEffect(() => {
    const on = () => {
      const m = store.get().me
      if (m && !m.locked) store.set({ me: { ...m, locked: true } })
    }
    window.addEventListener('bd:locked', on)
    return () => window.removeEventListener('bd:locked', on)
  }, [])
  // Input at the console: kept locally for the timer, and told to the server at most once a minute.
  useEffect(() => {
    // Crews' terminals, and a station's screen on a read-only account (a wall display), are not locked.
    if (!me || me.role === 'field' || (me.role === 'viewer' && (me.console ?? '').startsWith('STATION-'))) return
    const on = () => {
      last.current = Date.now()
      if (Date.now() - sent.current > 60_000 && !store.get().me?.locked) {
        sent.current = Date.now()
        api.post('/auth/active').catch(() => {})
      }
    }
    const evs = ['keydown', 'pointerdown', 'wheel'] as const
    evs.forEach((e) => window.addEventListener(e, on, { passive: true }))
    const t = window.setInterval(() => {
      if (lockMinutes > 0 && !store.get().me?.locked && Date.now() - last.current > lockMinutes * 60_000) lockNow()
    }, 15_000)
    return () => {
      evs.forEach((e) => window.removeEventListener(e, on))
      window.clearInterval(t)
    }
  }, [me, lockMinutes])
  // Ctrl+Shift+L locks.
  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.shiftKey && e.key.toLowerCase() === 'l') {
        e.preventDefault()
        lockNow()
      }
    }
    window.addEventListener('keydown', on)
    return () => window.removeEventListener('keydown', on)
  }, [])
  if (!me?.locked) return null
  return <LockScreen name={me.name || me.username} position={me.console} />
}

function LockScreen({ name, position }: { name: string; position: string }) {
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [needCode, setNeedCode] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const unlock = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.post('/auth/unlock', { password, code })
      await checkMe()
      setPassword('')
      setCode('')
    } catch (err) {
      const msg = errText(err)
      if (err instanceof ApiError && /code/i.test(msg)) setNeedCode(true)
      setError(msg)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="fixed inset-0 z-[100] grid place-items-center bg-slate-950/[.97] backdrop-blur-xl" role="dialog" aria-modal="true" aria-labelledby="lock-h">
      <form onSubmit={unlock} className="panel w-full max-w-sm space-y-3 rounded-xl p-6 shadow-2xl">
        <div className="flex items-center gap-2">
          <Logo size={24} />
          <h1 id="lock-h" className="flex items-center gap-1.5 text-lg font-bold text-slate-50"><LockIcon size={16} className="text-amber-300" aria-hidden /> Locked</h1>
          <span className="ml-auto font-mono text-xs text-slate-400">{position}</span>
        </div>
        <p className="text-sm text-slate-300">{name}'s console. Everything is still open underneath: incidents, notes and typing are kept. Only {name} can unlock it.</p>
        <div>
          <label className="label" htmlFor="lock-p">Password</label>
          <input id="lock-p" type="password" className="input" autoFocus autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
        </div>
        {needCode && (
          <div>
            <label className="label" htmlFor="lock-c">One-time code</label>
            <input id="lock-c" className="input font-mono tracking-[0.3em]" inputMode="numeric" maxLength={6} value={code} onChange={(e) => setCode(e.target.value.replace(/[^0-9]/g, ''))} />
          </div>
        )}
        {error && <p role="alert" className="rounded-md border border-rose-800 bg-rose-950/50 px-3 py-2 text-sm text-rose-200">{error}</p>}
        <div className="flex gap-2">
          <button className="btn btn-go flex-1 py-2" disabled={busy || !password}><Unlock size={14} aria-hidden /> Unlock</button>
          <button type="button" className="btn btn-ghost" onClick={() => signOut()}><LogOut size={14} aria-hidden /> Sign out</button>
        </div>
        <p className="text-[11px] text-slate-500">Locking and unlocking are recorded in the ledger.</p>
      </form>
    </div>
  )
}
