// The wishlist's console logic, kept free of React so it is tested on its own: grouping callers about the
// same thing, repeat callers, the caller's floor, and what a crash notification says.

import type { Call, Settings } from './types'

const R = 6371000

/** Metres between two points. */
export function metres(lat1: number, lon1: number, lat2: number, lon2: number): number {
  const p = Math.PI / 180
  const a = Math.sin(((lat2 - lat1) * p) / 2) ** 2 + Math.cos(lat1 * p) * Math.cos(lat2 * p) * Math.sin(((lon2 - lon1) * p) / 2) ** 2
  return 2 * R * Math.asin(Math.min(1, Math.sqrt(a)))
}

export type Cluster = { key: string; calls: Call[]; incident?: string }

/**
 * clusters groups live calls made near each other within a short time: several people calling about one
 * fire. Each call is in at most one group; a group has two or more calls. A group whose calls include one
 * already on an incident names that incident, so the others can be attached to it in one step.
 */
export function clusters(calls: Call[], s: Pick<Settings, 'clusterMeters' | 'clusterMinutes'>): Cluster[] {
  const near = s.clusterMeters && s.clusterMeters > 0 ? s.clusterMeters : 300
  const within = (s.clusterMinutes && s.clusterMinutes > 0 ? s.clusterMinutes : 10) * 60000
  const live = calls.filter((c) => c.location?.lat != null && c.location?.lon != null && !c.screen?.nonEmergency)
    .sort((a, b) => Date.parse(a.start) - Date.parse(b.start))
  const used = new Set<string>()
  const out: Cluster[] = []
  for (const c of live) {
    if (used.has(c.id)) continue
    const group = [c]
    for (const o of live) {
      if (o.id === c.id || used.has(o.id)) continue
      const close = group.some((g) => metres(g.location!.lat!, g.location!.lon!, o.location!.lat!, o.location!.lon!) <= near + Math.min(g.location?.radius ?? 0, 300) + Math.min(o.location?.radius ?? 0, 300)
        && Math.abs(Date.parse(g.start) - Date.parse(o.start)) <= within)
      if (close) group.push(o)
    }
    if (group.length < 2) continue
    // Calls on different incidents are not one thing.
    const incs = [...new Set(group.map((g) => g.incident).filter(Boolean))] as string[]
    if (incs.length > 1) continue
    group.forEach((g) => used.add(g.id))
    out.push({ key: group.map((g) => g.id).join('+'), calls: group, incident: incs[0] })
  }
  return out
}

/** repeats counts each callback number's calls in the last ten minutes. */
export function repeats(calls: Call[], now: number): Map<string, number> {
  const m = new Map<string, number>()
  for (const c of calls) {
    if (!c.callback || now - Date.parse(c.start) > 10 * 60000) continue
    m.set(c.callback, (m.get(c.callback) ?? 0) + 1)
  }
  return m
}

/** floorText is what the caller's location says about the floor: the civic floor, or the altitude. */
export function floorText(c: Pick<Call, 'location'>): string {
  const l = c.location
  if (!l) return ''
  const parts: string[] = []
  if (l.civic?.floor) parts.push(`floor ${l.civic.floor}`)
  if (l.altitude != null) parts.push(`altitude ${Math.round(l.altitude)} m${l.vRadius ? ` ±${Math.round(l.vRadius)}` : ''}`)
  return parts.join(' · ')
}

/** methodText names how a caller's position was found. */
export function methodText(method?: string): string {
  switch ((method ?? '').toUpperCase()) {
    case 'DBH':
      return 'device location'
    case 'GPS':
    case 'A-GPS':
      return 'GPS'
    case 'CELL':
      return 'cell sector'
    case 'MANUAL':
      return 'on file'
    default:
      return method ?? ''
  }
}

/** kmh shows a speed in km/h and mph. */
export function kmh(v?: number): string {
  if (!v) return ''
  return `${Math.round(v)} km/h (${Math.round(v / 1.609)} mph)`
}

export const langName: Record<string, string> = { es: 'Spanish', en: 'English' }

/** minutes is a number of minutes as h:mm when an hour or more. */
export function minutes(m: number): string {
  if (m < 60) return `${Math.round(m)} min`
  return `${Math.floor(m / 60)} h ${String(Math.round(m % 60)).padStart(2, '0')}`
}
