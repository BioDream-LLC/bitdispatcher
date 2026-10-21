import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { BellRing, Volume2, VolumeX } from 'lucide-react'
import { api } from './api'
import { useStore } from './store'
import { cx, prio, statusMeta, useNow } from './util'
import type { StationAlert } from './types'

const KEY = 'bd.station'

/** Station is the screen in a firehouse: the station's units and their statuses, and, when units are
 * dispatched, the tones, the call read aloud twice, and the call in large type until someone
 * acknowledges it. Open it on a read-only account at #/station?st=ST1; the inactivity lock leaves it be. */
export default function Station() {
  const view = useStore((s) => s.view)
  const me = useStore((s) => s.me)
  const [params, setParams] = useSearchParams()
  const st = (params.get('st') ?? localStorage.getItem(KEY) ?? '').toUpperCase()
  const station = view?.stations.find((s) => s.id === st)
  const now = useNow()
  const [sound, setSound] = useState(false)
  const opened = useRef(Date.now())
  const played = useRef(new Set<string>())

  useEffect(() => {
    if (!st) return
    localStorage.setItem(KEY, st)
    if (me?.role === 'viewer') api.post('/auth/station', { station: st }).catch(() => {})
  }, [st, me?.role])

  const mine = useMemo(() => (view?.stationAlerts ?? []).filter((a) => a.station === st), [view?.stationAlerts, st])
  // The alert on screen: the newest not yet acknowledged, from the last ten minutes.
  const live = mine.slice().reverse().find((a) => !a.acked && now - new Date(a.at).getTime() < 10 * 60_000)

  // A new alert (one that arrived after the screen opened) sounds once.
  useEffect(() => {
    if (!live || played.current.has(live.id)) return
    played.current.add(live.id)
    if (new Date(live.at).getTime() < opened.current - 5000 || !sound) return
    toneOut(live)
  }, [live, sound])

  if (!view) return <div className="grid h-full place-items-center text-slate-500">Loading…</div>
  if (!station) {
    return (
      <div className="grid h-full place-items-center bg-slate-950 p-8">
        <div className="w-full max-w-md space-y-3">
          <h1 className="text-xl font-bold text-slate-100">Which station is this screen in?</h1>
          <ul className="grid grid-cols-2 gap-2">
            {view.stations.map((s) => <li key={s.id}><button className="btn h-14 w-full justify-center text-base" onClick={() => setParams({ st: s.id })}>{s.name}</button></li>)}
          </ul>
        </div>
      </div>
    )
  }
  const units = view.units.filter((u) => (u.covering || u.station) === st)
  const inc = live ? view.incidents.find((i) => i.number === live.incident) : undefined
  return (
    <div className={cx('flex h-full flex-col', live ? (live.priority === 1 ? 'flash bg-rose-950' : 'bg-amber-950') : 'bg-slate-950')}>
      <header className="flex items-center gap-4 border-b border-slate-800 px-6 py-3">
        <span className="text-2xl font-black text-slate-50">{station.name}</span>
        <span className="font-mono text-3xl tabular-nums text-slate-200">{new Date(now).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}</span>
        <button className={cx('btn ml-auto h-11 px-4 text-base', !sound && 'bg-amber-500 text-black')} onClick={() => { setSound(!sound); if (!sound) beepTest() }} aria-pressed={sound}>
          {sound ? <Volume2 size={18} aria-hidden /> : <VolumeX size={18} aria-hidden />} {sound ? 'Sound on' : 'Press to turn the sound on'}
        </button>
      </header>
      {live ? (
        <main role="alert" className="flex flex-1 flex-col justify-center gap-6 px-10">
          <div className="flex items-center gap-4">
            <BellRing size={56} className="text-white" aria-hidden />
            <span className={cx('rounded px-4 py-1 font-mono text-5xl font-black', prio(live.priority).bg, prio(live.priority).fg)}>P{live.priority}</span>
            <span className="text-6xl font-black text-white">{live.typeName || live.type}</span>
          </div>
          <div className="text-7xl font-black leading-tight text-amber-200">{live.address || 'Location to follow'}</div>
          <div className="flex flex-wrap gap-4">{live.units.map((u) => <span key={u} className="rounded-lg bg-white px-6 py-2 font-mono text-5xl font-black text-black">{u}</span>)}</div>
          {inc?.timeline.filter((e) => e.kind === 'priority-note').slice(-2).map((e) => <p key={e.seq} className="text-3xl font-bold text-rose-100">{e.text}</p>)}
          <div className="flex items-center gap-4">
            <button className="btn h-16 bg-white px-10 text-2xl font-black text-black" onClick={() => api.post(`/station-alerts/${live.id}/ack`).catch(() => {})}>Acknowledge</button>
            <button className="btn h-16 px-8 text-xl" onClick={() => toneOut(live)}>Read it again</button>
            <span className="font-mono text-xl text-slate-300">{live.incident} · {Math.round((now - new Date(live.at).getTime()) / 1000)} s ago{live.again ? ' · toned again' : ''}</span>
          </div>
        </main>
      ) : (
        <main className="flex-1 p-6">
          <ul className="grid grid-cols-[repeat(auto-fill,minmax(16rem,1fr))] gap-4" aria-label="Units at this station">
            {units.map((u) => (
              <li key={u.id} className={cx('rounded-xl border-2 p-4', statusMeta(u.status).ring, 'border-slate-700')}>
                <div className="font-mono text-5xl font-black text-slate-50">{u.id}</div>
                <div className={cx('mt-1 text-2xl font-bold', statusMeta(u.status).fg)}>{statusMeta(u.status).label}</div>
                {u.incident && <div className="text-lg text-slate-300">{u.incident}</div>}
              </li>
            ))}
            {!units.length && <li className="text-xl text-slate-500">No units at this station.</li>}
          </ul>
          {mine.length > 0 && (
            <section className="mt-8">
              <h2 className="text-sm font-bold uppercase tracking-wider text-slate-400">Today's alerts</h2>
              <ul className="mt-2 space-y-1 text-lg">
                {mine.slice(-8).reverse().map((a) => (
                  <li key={a.id} className="text-slate-300"><span className="font-mono text-slate-500">{new Date(a.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span> {a.units.join(', ')} · {a.typeName || a.type} · {a.address}{a.acked ? '' : ' · not acknowledged'}</li>
                ))}
              </ul>
            </section>
          )}
        </main>
      )}
    </div>
  )
}

/** toneOut: rising tones, then the call read aloud twice. */
function toneOut(a: StationAlert) {
  try {
    const ctx = new AudioContext()
    const steps = a.priority === 1 ? [660, 880, 1100, 660, 880, 1100] : [660, 880, 1100]
    steps.forEach((f, i) => {
      const o = ctx.createOscillator()
      const g = ctx.createGain()
      const t = ctx.currentTime + i * 0.35
      o.frequency.value = f
      g.gain.setValueAtTime(0.0001, t)
      g.gain.exponentialRampToValueAtTime(0.25, t + 0.05)
      g.gain.exponentialRampToValueAtTime(0.0001, t + 0.32)
      o.connect(g).connect(ctx.destination)
      o.start(t)
      o.stop(t + 0.34)
    })
    window.setTimeout(() => say(a.speech, 2), steps.length * 350 + 300)
  } catch {
    say(a.speech, 2)
  }
}

function say(text: string, times: number) {
  if (!('speechSynthesis' in window)) return
  for (let i = 0; i < times; i++) {
    const u = new SpeechSynthesisUtterance(text)
    u.rate = 0.95
    window.speechSynthesis.speak(u)
  }
}

function beepTest() {
  try {
    const ctx = new AudioContext()
    const o = ctx.createOscillator()
    const g = ctx.createGain()
    g.gain.value = 0.05
    o.connect(g).connect(ctx.destination)
    o.start()
    o.stop(ctx.currentTime + 0.1)
  } catch {
    // no audio
  }
}
