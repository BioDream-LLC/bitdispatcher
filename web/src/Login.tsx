import { useEffect, useMemo, useRef, useState } from 'react'
import { KeyRound, LogIn, MonitorCheck, PlayCircle, ShieldCheck, Smartphone } from 'lucide-react'
import { renderSVG } from 'uqr'
import { api, ApiError, errText } from './api'
import { checkMe } from './store'
import { Logo } from './ui'
import { cx } from './util'
import type { Duty, Need } from './types'

export const DUTY_NAMES: Record<Duty, string> = {
  calltaker: 'Call-taker', police: 'Police dispatcher', fire: 'Fire dispatcher', ems: 'EMS dispatcher', dispatcher: 'Dispatcher (all disciplines)',
  supervisor: 'Supervisor', admin: 'Administrator', records: 'Records', field: 'Field crew (mobile terminal)', viewer: 'Viewer',
}

// This browser's own position name when it is not an enrolled workstation: made once and kept, so its
// screens and layout stay its own.
function webPosition() {
  let p = localStorage.getItem('bd.webpos')
  if (!p) {
    p = 'WEB-' + Math.random().toString(36).slice(2, 6).toUpperCase()
    localStorage.setItem('bd.webpos', p)
  }
  return p
}

/** The sign-in page: a password, then the one-time code and the duty when the server asks. The position is
 * the workstation this browser is enrolled as: nobody is asked which console they are at. */
export default function Login() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [unit, setUnit] = useState(localStorage.getItem('bd-unit') ?? '')
  const [need, setNeed] = useState<Need | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [training, setTraining] = useState(false)
  const [exercise, setExercise] = useState('')
  const [enrolOpen, setEnrolOpen] = useState(false)
  const [ws, setWs] = useState(localStorage.getItem('bd.ws.name') ?? '')
  const codeRef = useRef<HTMLInputElement>(null)
  // The public demonstration account, when this server has one: one button signs a visitor in.
  const [demo, setDemo] = useState<{ name: string; role: string; duties?: Duty[] } | null>(null)
  const [demoDuty, setDemoDuty] = useState<Duty>((localStorage.getItem('bd.demo.duty') as Duty) || 'dispatcher')
  const [notice, setNotice] = useState('')
  const [unreachable, setUnreachable] = useState(false) // the server has not answered: this may be the saved copy
  // Ask until the server answers: a page opened while it was restarting (or kept by the offline cache)
  // would otherwise never offer the demo account or show the notice.
  useEffect(() => {
    let stop = false
    let timer: ReturnType<typeof setTimeout> | undefined
    const ask = (wait: number) => {
      api.get<{ demo?: { name: string; role: string; duties?: Duty[] }; notice?: string }>('/auth/me').then(
        (r) => { if (!stop) { setDemo(r.demo ?? null); setNotice(r.notice ?? ''); setUnreachable(false) } },
        () => { if (!stop) { setUnreachable(true); timer = setTimeout(() => ask(Math.min(wait * 2, 30_000)), wait) } },
      )
    }
    ask(2000)
    return () => { stop = true; clearTimeout(timer) }
  }, [])
  useEffect(() => {
    if (need?.code || need?.enrol) codeRef.current?.focus()
  }, [need])
  const signInDemo = async () => {
    setBusy(true)
    setError('')
    localStorage.setItem('bd.demo.duty', demoDuty)
    try {
      await api.post('/auth/demo', { console: webPosition(), training, duty: demoDuty })
      await checkMe()
    } catch (err) {
      setError(errText(err))
    } finally {
      setBusy(false)
    }
  }
  const attempt = async (duty?: Duty) => {
    setBusy(true)
    setError('')
    try {
      await api.post('/auth/login', {
        username, password, code, duty: duty ?? '', training, exercise: training ? exercise.trim() : '',
        workstation: localStorage.getItem('bd.ws') ?? '', console: webPosition(), unit: unit.trim().toUpperCase(),
      })
      if (unit) localStorage.setItem('bd-unit', unit.trim().toUpperCase())
      await checkMe()
    } catch (err) {
      if (err instanceof ApiError && err.data?.need) {
        const n = err.data.need as Need
        setNeed(n)
        if (err.status !== 202) setError(err.message)
        if (n.duties?.length === 1) return attempt(n.duties[0] as Duty)
      } else {
        setError(errText(err))
      }
    } finally {
      setBusy(false)
    }
  }
  const submit = (e: React.FormEvent) => {
    e.preventDefault()
    attempt()
  }
  const qr = useMemo(() => (need?.enrol ? renderSVG(need.enrol.uri, { border: 2 }) : ''), [need?.enrol])
  const askingDuty = !!need?.duties?.length
  return (
    <div className="grid h-full place-items-center overflow-y-auto bg-[radial-gradient(ellipse_at_top,var(--color-slate-900),var(--color-slate-950))] p-6">
      <div className="w-full max-w-md space-y-3">
        {demo && !askingDuty && (
          <section aria-labelledby="demo-h" className="panel rounded-xl border-l-4 border-l-sky-400 p-4 shadow-2xl">
            <h2 id="demo-h" className="text-sm font-semibold text-slate-50">Try it with the demo account</h2>
            <p className="mt-1 text-xs leading-relaxed text-slate-400">
              A working console on invented data: simulated 911 calls arrive as you watch, and everything goes back to the demonstration data every
              night. Every call and record here is fictional.
            </p>
            {(demo.duties?.length ?? 0) > 1 && (
              <div className="mt-2 flex flex-wrap gap-1" role="radiogroup" aria-label="Work as">
                {demo.duties!.map((d) => (
                  <button key={d} type="button" role="radio" aria-checked={demoDuty === d} onClick={() => setDemoDuty(d)}
                    className={cx('chip h-6 cursor-pointer px-2 text-[11px]', demoDuty === d ? 'bg-sky-500 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700')}>{DUTY_NAMES[d] ?? d}</button>
                ))}
              </div>
            )}
            <button type="button" className="mt-3 inline-flex w-full items-center justify-center gap-1.5 rounded-md bg-sky-500 py-2 text-sm font-semibold text-white shadow hover:bg-sky-400 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-sky-300 disabled:opacity-60" onClick={signInDemo} disabled={busy}>
              <PlayCircle size={15} aria-hidden /> Sign in as the demo user
            </button>
          </section>
        )}
        {unreachable && (
          <p role="alert" className="w-full rounded-lg border border-rose-700 bg-rose-950/60 p-3 text-xs leading-relaxed text-rose-100">
            <b>The bitdispatcher server is not answering.</b> This page may be the copy this browser saved, so signing in will not work yet. It keeps trying; check the network connection, or that nothing between here and the server (a firewall or web filter) blocks it.
          </p>
        )}
        {notice && <section aria-label="System use notice" className="w-full whitespace-pre-wrap rounded-lg border border-amber-600/60 bg-amber-950/40 p-3 text-xs leading-relaxed text-amber-100">{notice}</section>}
        <form onSubmit={submit} className="panel w-full space-y-4 rounded-xl p-6 shadow-2xl" aria-labelledby="login-h">
          <div className="flex items-center gap-2">
            <Logo size={28} />
            <h1 id="login-h" className="text-lg font-bold text-slate-50">bitdispatcher</h1>
            <span className="ml-auto inline-flex items-center gap-1 text-xs text-slate-400" title={ws ? 'This computer is enrolled as a workstation' : 'Not an enrolled workstation: it signs in as a web position'}>
              <MonitorCheck size={13} className={ws ? 'text-emerald-400' : 'text-slate-600'} aria-hidden /> {ws || '911 CAD'}
            </span>
          </div>
          {!askingDuty && (
            <>
              <div>
                <label className="label" htmlFor="u">Username</label>
                <input id="u" className="input" autoComplete="username" autoFocus value={username} onChange={(e) => { setUsername(e.target.value); setNeed(null) }} />
              </div>
              <div>
                <label className="label" htmlFor="p">Password</label>
                <input id="p" type="password" className="input" autoComplete="current-password" value={password} onChange={(e) => { setPassword(e.target.value); setNeed(null) }} />
              </div>
            </>
          )}
          {need?.enrol && (
            <div className="rounded-lg border border-sky-800 bg-sky-950/40 p-3 text-xs text-sky-100">
              <div className="flex items-center gap-1.5 font-semibold"><Smartphone size={14} aria-hidden /> Set up your authenticator app</div>
              <p className="mt-1 text-sky-200/80">Your agency asks for a one-time code at sign-in. Scan this with an authenticator app (or type the key), then enter the six-digit code it shows.</p>
              <div className="mt-2 flex items-center gap-3">
                <div className="h-32 w-32 shrink-0 rounded bg-white p-1" aria-label="QR code for the authenticator app" role="img" dangerouslySetInnerHTML={{ __html: qr }} />
                <div className="min-w-0 break-all font-mono text-[11px] text-slate-200">{need.enrol.secret.replace(/(.{4})/g, '$1 ')}</div>
              </div>
            </div>
          )}
          {(need?.code || need?.enrol) && !askingDuty && (
            <div>
              <label className="label" htmlFor="c">One-time code</label>
              <input id="c" ref={codeRef} className="input font-mono tracking-[0.3em]" inputMode="numeric" autoComplete="one-time-code" maxLength={7} value={code} onChange={(e) => setCode(e.target.value.replace(/[^0-9]/g, ''))} />
            </div>
          )}
          {need?.unit && (
            <div>
              <label className="label" htmlFor="unit">Your unit</label>
              <input id="unit" className="input font-mono uppercase" autoFocus placeholder="E1, M12, 2A31" value={unit} onChange={(e) => setUnit(e.target.value)} />
              <p className="mt-1 text-xs text-slate-500">A mobile terminal signs in as its unit.</p>
            </div>
          )}
          {askingDuty && (
            <fieldset>
              <legend className="label">What are you working?</legend>
              <p className="mb-2 text-xs text-slate-400">Signed in as <b className="text-slate-200">{username}</b>{ws && <> at <b className="text-slate-200">{ws}</b></>}. Your screens and what you may do follow the duty you choose.</p>
              <div className="grid grid-cols-2 gap-1.5">
                {need!.duties!.map((d, i) => (
                  <button key={d} type="button" autoFocus={d === (need!.last || need!.duties![0])} disabled={busy}
                    className={cx('btn justify-start py-2 text-left text-sm', d === (need!.last || need!.duties![0]) && 'ring-1 ring-sky-400')}
                    onClick={() => attempt(d as Duty)}>
                    <span className="kbd mr-1">{i + 1}</span>{DUTY_NAMES[d as Duty] ?? d}
                  </button>
                ))}
              </div>
              <button type="button" className="btn btn-ghost mt-2 text-xs" onClick={() => setNeed(null)}>Back</button>
            </fieldset>
          )}
          {!askingDuty && (
            <div className="space-y-1.5">
              <label className="flex items-start gap-2 text-sm text-slate-300">
                <input type="checkbox" className="mt-1" checked={training} onChange={(e) => setTraining(e.target.checked)} />
                <span>Training<span className="block text-xs text-slate-500">A board of its own. Nothing reaches the live board.</span></span>
              </label>
              {training && (
                <div className="ml-6">
                  <label className="label" htmlFor="ex">Exercise (optional)</label>
                  <input id="ex" className="input" placeholder="The name your instructor gave, or leave empty to practise alone" value={exercise} onChange={(e) => setExercise(e.target.value)} />
                </div>
              )}
            </div>
          )}
          {error && <p role="alert" className="rounded-md border border-rose-800 bg-rose-950/50 px-3 py-2 text-sm text-rose-200">{error}</p>}
          {!askingDuty && (
            <button className="btn btn-go w-full py-2 text-sm" name="Sign in" disabled={busy || !username || !password || ((!!need?.code || !!need?.enrol) && code.length < 6)}>
              <LogIn size={15} aria-hidden /> {busy ? 'Signing in…' : 'Sign in'}
            </button>
          )}
          <div className="flex items-center gap-1.5 text-xs text-slate-500">
            <ShieldCheck size={13} className="text-emerald-400" aria-hidden /> Sign-ins, views and changes are recorded in a tamper-evident ledger.
            <button type="button" className="ml-auto text-slate-400 underline-offset-2 hover:underline" onClick={() => setEnrolOpen((o) => !o)}>Enrol this computer</button>
          </div>
          {enrolOpen && <Enrol onDone={(name) => { setWs(name); setEnrolOpen(false) }} />}
        </form>
      </div>
      <DutyKeys need={need} onPick={(d) => attempt(d)} />
    </div>
  )
}

// Number keys choose a duty while the choice is shown.
function DutyKeys({ need, onPick }: { need: Need | null; onPick: (d: Duty) => void }) {
  useEffect(() => {
    if (!need?.duties?.length) return
    const on = (e: KeyboardEvent) => {
      const n = Number(e.key)
      if (n >= 1 && n <= need.duties!.length) onPick(need.duties![n - 1] as Duty)
    }
    window.addEventListener('keydown', on)
    return () => window.removeEventListener('keydown', on)
  }, [need, onPick])
  return null
}

function Enrol({ onDone }: { onDone: (name: string) => void }) {
  const [c, setC] = useState('')
  const [err, setErr] = useState('')
  const go = async () => {
    setErr('')
    try {
      const r = await api.post<{ token: string; workstation: string }>('/workstations/enrol', { code: c, label: navigator.userAgent.slice(0, 80) })
      localStorage.setItem('bd.ws', r.token)
      localStorage.setItem('bd.ws.name', r.workstation)
      onDone(r.workstation)
    } catch (e) {
      setErr(errText(e))
    }
  }
  return (
    <div className="rounded-lg border border-slate-700 p-3">
      <label className="label" htmlFor="enrol">Enrolment code from an administrator</label>
      <div className="flex gap-1.5">
        <input id="enrol" className="input font-mono uppercase tracking-widest" placeholder="ABCDE-FGHJK" value={c} onChange={(e) => setC(e.target.value)} />
        <button type="button" className="btn" disabled={c.replace(/-/g, '').length < 10} onClick={go}><KeyRound size={14} aria-hidden /> Enrol</button>
      </div>
      <p className="mt-1 text-[11px] text-slate-500">Admin › Workstations makes a code. This computer then signs in as that console position every time.</p>
      {err && <p role="alert" className="mt-1 text-xs text-rose-300">{err}</p>}
    </div>
  )
}
