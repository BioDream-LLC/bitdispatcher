import { useEffect, useState } from 'react'
import { serverNow } from './store'
import type { Location, MarkOutType, Unit, View } from './types'

// useNow re-renders every second with the server's clock, so every timer on screen agrees with the server.
let tick = serverNow()
const subs = new Set<(n: number) => void>()
window.setInterval(() => {
  tick = serverNow()
  subs.forEach((f) => f(tick))
}, 1000)

export function useNow(): number {
  const [n, setN] = useState(tick)
  useEffect(() => {
    subs.add(setN)
    return () => {
      subs.delete(setN)
    }
  }, [])
  return n
}

export const ms = (iso?: string) => (iso ? new Date(iso).getTime() : 0)

// clock is an elapsed time as m:ss, or h:mm:ss past an hour.
export function clock(from: string | number | undefined, now: number): string {
  const t = typeof from === 'number' ? from : ms(from)
  if (!t) return '–'
  let sec = Math.max(0, Math.floor((now - t) / 1000))
  const h = Math.floor(sec / 3600)
  sec -= h * 3600
  const m = Math.floor(sec / 60)
  const ss = String(sec % 60).padStart(2, '0')
  return h ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}

// ago is a short age: 12s, 4m, 2h.
export function ago(from: string | number | undefined, now: number): string {
  const t = typeof from === 'number' ? from : ms(from)
  if (!t) return 'never'
  const sec = Math.max(0, Math.floor((now - t) / 1000))
  if (sec < 60) return `${sec}s`
  if (sec < 3600) return `${Math.floor(sec / 60)}m`
  if (sec < 86400) return `${Math.floor(sec / 3600)}h${String(Math.floor((sec % 3600) / 60)).padStart(2, '0')}`
  return `${Math.floor(sec / 86400)}d`
}

export function hhmm(iso?: string): string {
  if (!iso) return ''
  return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
}

export function phone(n?: string): string {
  if (!n) return ''
  const d = n.replace(/^\+1/, '')
  if (/^\d{10}$/.test(d)) return `(${d.slice(0, 3)}) ${d.slice(3, 6)}-${d.slice(6)}`
  return n
}

export type StatusMeta = { code: string; label: string; bg: string; fg: string; ring: string; dot: string }

// Status colours follow common CAD practice, and every status also has its own short code, so colour is never
// the only signal (WCAG 1.4.1).
export const STATUS: Record<string, StatusMeta> = {
  available: { code: 'AV', label: 'Available', bg: 'bg-emerald-500/15', fg: 'text-emerald-300', ring: 'ring-emerald-500/40', dot: '#34d399' },
  dispatched: { code: 'DP', label: 'Dispatched', bg: 'bg-yellow-400/20', fg: 'text-yellow-300', ring: 'ring-yellow-400/50', dot: '#facc15' },
  enroute: { code: 'ER', label: 'En route', bg: 'bg-orange-500/20', fg: 'text-orange-300', ring: 'ring-orange-500/50', dot: '#fb923c' },
  onscene: { code: 'OS', label: 'On scene', bg: 'bg-rose-500/20', fg: 'text-rose-300', ring: 'ring-rose-500/50', dot: '#fb7185' },
  transporting: { code: 'TR', label: 'Transporting', bg: 'bg-violet-500/20', fg: 'text-violet-300', ring: 'ring-violet-500/50', dot: '#a78bfa' },
  atdestination: { code: 'AD', label: 'At destination', bg: 'bg-fuchsia-500/20', fg: 'text-fuchsia-300', ring: 'ring-fuchsia-500/50', dot: '#e879f9' },
  markout: { code: 'MO', label: 'Marked out', bg: 'bg-teal-500/15', fg: 'text-teal-300', ring: 'ring-teal-500/40', dot: '#2dd4bf' },
  oos: { code: 'OOS', label: 'Out of service', bg: 'bg-slate-500/15', fg: 'text-slate-400', ring: 'ring-slate-500/40', dot: '#64748b' },
}

export const statusMeta = (s: string): StatusMeta => STATUS[s] ?? STATUS.oos

export const PRIORITY: Record<number, { bg: string; fg: string; border: string; hex: string }> = {
  1: { bg: 'bg-rose-500', fg: 'text-white', border: 'border-rose-500', hex: '#f43f5e' },
  2: { bg: 'bg-orange-500', fg: 'text-white', border: 'border-orange-500', hex: '#f97316' },
  3: { bg: 'bg-amber-400', fg: 'text-slate-950', border: 'border-amber-400', hex: '#fbbf24' },
  4: { bg: 'bg-sky-500', fg: 'text-white', border: 'border-sky-500', hex: '#0ea5e9' },
  5: { bg: 'bg-slate-500', fg: 'text-white', border: 'border-slate-500', hex: '#64748b' },
}
export const prio = (p: number) => PRIORITY[p] ?? PRIORITY[5]

export function discipline(view: View, u: Unit): string {
  return view.settings.unitTypes.find((t) => t.code === u.type)?.discipline ?? 'other'
}

export function typeName(view: View, code: string): string {
  return view.callTypes.find((c) => c.code === code)?.name ?? code
}

export function markOutType(view: View, code?: string): MarkOutType | undefined {
  return code ? view.settings.markOuts?.find((t) => t.code === code) : undefined
}

/** The mark-outs a unit may use: those for any discipline and those for its own. */
export function markOutsFor(view: View, u: Unit): MarkOutType[] {
  const d = discipline(view, u)
  return (view.settings.markOuts ?? []).filter((t) => !t.discipline || t.discipline === d)
}

// timerMinutes is how long a unit may stay as it is before a status check is due: its mark-out's own time,
// or its status's; 0 for none.
export function timerMinutes(view: View, u: Unit): number {
  if (u.status === 'available' || u.status === 'oos') return 0
  // The server's figure: the call type's safety timer, the unit's agency's or the centre's.
  if (u.checkMinutes !== undefined) return u.checkMinutes
  if (u.status === 'markout') {
    const t = markOutType(view, u.markOut?.code)
    if (t) return t.minutes
  }
  return view.settings.timers[u.status] ?? 0
}

// checkDue is when the unit's next status check is due (ms), or 0: its timer runs from the later of its last
// status change and its last status check.
export function checkDue(view: View, u: Unit): number {
  const mins = timerMinutes(view, u)
  if (!mins) return 0
  return Math.max(ms(u.statusAt), ms(u.checked)) + mins * 60000
}

// overdue reports a unit past its status timer, and by how much.
export function overdue(view: View, u: Unit, now: number): number {
  const due = checkDue(view, u)
  return due && now > due ? now - due : 0
}

// activity is what a unit is doing, in words: its incident, or its mark-out.
/** Minutes a unit's mark-out has run past the longest its type allows (0: within it, or no limit). */
export function markOutOver(view: View, u: Unit, now = Date.now()): number {
  if (u.status !== 'markout') return 0
  const max = markOutType(view, u.markOut?.code)?.maxMinutes ?? 0
  if (!max) return 0
  const mins = Math.floor((now - ms(u.statusAt)) / 60000)
  return mins >= max ? mins - max + 1 : 0
}

export function activity(view: View, u: Unit): string {
  if (u.status === 'markout') {
    const name = markOutType(view, u.markOut?.code)?.name ?? u.markOut?.code ?? 'Marked out'
    return markOutOver(view, u) ? `${name} · OVER ${markOutType(view, u.markOut?.code)?.maxMinutes} MIN` : name
  }
  if (u.incident) {
    const i = view.incidents.find((x) => x.number === u.incident)
    return i ? `${u.incident} ${typeName(view, i.type)}` : u.incident
  }
  return ''
}

export type AVLState = 'fresh' | 'stale' | 'none'
export function avl(view: View, u: Unit, now: number): AVLState {
  if (!u.position) return 'none'
  return now - ms(u.position.at) <= view.settings.avlStaleSeconds * 1000 ? 'fresh' : 'stale'
}

export function hasPoint(l?: Location): l is Location & { lat: number; lon: number } {
  return !!l && typeof l.lat === 'number' && typeof l.lon === 'number'
}

export function locText(l?: Location): string {
  if (!l) return 'No location'
  if (l.address) return l.address
  if (hasPoint(l)) return `${l.lat.toFixed(5)}, ${l.lon.toFixed(5)}`
  return 'No location'
}

export function uncertainty(l?: Location): string {
  if (!l) return ''
  const parts: string[] = []
  if (l.radius) parts.push(`±${Math.round(l.radius)} m`)
  if (l.confidence) parts.push(`${l.confidence}%`)
  if (l.method) parts.push(l.method)
  return parts.join(' · ')
}

export function locationStale(view: View, l: Location | undefined, now: number): boolean {
  if (!l?.at) return false
  return now - ms(l.at) > view.settings.locationStaleSeconds * 1000
}

export function km(m?: number): string {
  if (!m) return ''
  return m < 1000 ? `${Math.round(m)} m` : `${(m / 1000).toFixed(1)} km`
}

// mi is a distance in miles (feet when short), as US dispatchers and crews speak it.
export function mi(m?: number): string {
  if (m === undefined || m === null) return ''
  return m < 300 ? `${Math.round(m * 3.28084)} ft` : `${(m / 1609.344).toFixed(1)} mi`
}

export function mins(sec?: number): string {
  if (!sec) return '–'
  if (sec < 60) return '<1m'
  return `${Math.round(sec / 60)}m`
}

export const cx = (...c: (string | false | null | undefined)[]) => c.filter(Boolean).join(' ')
