import { useState } from 'react'
import { LogIn, ShieldCheck } from 'lucide-react'
import { api, errText } from './api'
import { checkMe } from './store'
import { Logo } from './ui'

export default function Login() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [console_, setConsole] = useState(localStorage.getItem('bd-console') ?? '')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.post('/auth/login', { username, password, console: console_ })
      localStorage.setItem('bd-console', console_)
      await checkMe()
    } catch (err) {
      setError(errText(err))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="grid h-full place-items-center bg-[radial-gradient(ellipse_at_top,var(--color-slate-900),var(--color-slate-950))] p-6">
      <form onSubmit={submit} className="panel w-full max-w-sm space-y-4 rounded-xl p-6 shadow-2xl" aria-labelledby="login-h">
        <div className="flex items-center gap-2">
          <Logo size={28} />
          <h1 id="login-h" className="text-lg font-bold text-slate-50">
            bitdispatcher
          </h1>
          <span className="ml-auto text-xs text-slate-500">911 CAD</span>
        </div>
        <div>
          <label className="label" htmlFor="u">Username</label>
          <input id="u" className="input" autoComplete="username" autoFocus value={username} onChange={(e) => setUsername(e.target.value)} />
        </div>
        <div>
          <label className="label" htmlFor="p">Password</label>
          <input id="p" type="password" className="input" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} />
        </div>
        <div>
          <label className="label" htmlFor="c">Position</label>
          <input id="c" className="input font-mono" placeholder="D1, Fire 2, or a unit: E1" value={console_} onChange={(e) => setConsole(e.target.value)} />
          <p className="mt-1 text-xs text-slate-500">The console you are at, or on a mobile terminal your unit. Every action you take is recorded with it.</p>
        </div>
        {error && (
          <p role="alert" className="rounded-md border border-rose-800 bg-rose-950/50 px-3 py-2 text-sm text-rose-200">
            {error}
          </p>
        )}
        <button className="btn btn-go w-full py-2 text-sm" disabled={busy || !username || !password}>
          <LogIn size={15} aria-hidden /> {busy ? 'Signing in…' : 'Sign in'}
        </button>
        <p className="flex items-center gap-1.5 text-xs text-slate-500">
          <ShieldCheck size={13} className="text-emerald-400" aria-hidden /> Sign-ins, views and changes are recorded in a tamper-evident ledger.
        </p>
      </form>
    </div>
  )
}
