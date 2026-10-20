// The call-taker's entry before (and while) it is an incident: kept in this browser tab, so a lock, a reload
// or a slow network never loses what was typed. Submitted, it becomes the incident; the workspace stays on
// it, and every later change goes straight to the incident.

import type { Call, CallType, Contact, Incident, Person, SearchResult, Vehicle } from '../types'

export type Note = { at: string; text: string }

export type Draft = {
  call?: string // the 911 call being taken
  number?: string // the incident, once submitted
  type: string
  priority: number // 0: the call type's
  modifiers: string[]
  address: string
  lat: string
  lon: string
  verified: boolean // picked from the recorded map
  callerName: string
  callback: string
  callerElsewhere: boolean
  callerAddress: string
  callerLat: string
  callerLon: string
  notes: Note[] // written before submission, sent with their own times
  persons: Person[]
  vehicles: Vehicle[]
  contacts: Contact[]
  started: string
  dupsSeen?: string // the duplicates the call-taker has looked at (a key), so submit goes ahead
}

export const emptyDraft = (): Draft => ({
  type: '', priority: 0, modifiers: [], address: '', lat: '', lon: '', verified: false, callerName: '', callback: '', callerElsewhere: false,
  callerAddress: '', callerLat: '', callerLon: '', notes: [], persons: [], vehicles: [], contacts: [], started: new Date().toISOString(),
})

const KEY = 'bd.taker.draft'

export function loadDraft(): Draft {
  try {
    const raw = sessionStorage.getItem(KEY)
    if (raw) return { ...emptyDraft(), ...(JSON.parse(raw) as Draft) }
  } catch {
    /* a broken draft starts again */
  }
  return emptyDraft()
}

export function saveDraft(d: Draft) {
  sessionStorage.setItem(KEY, JSON.stringify(d))
}

/** dirty is whether there is anything a person typed that is not yet on an incident. */
export function dirty(d: Draft): boolean {
  if (d.number) return false
  return !!(d.type || d.address || d.notes.length || d.persons.length || d.vehicles.length || d.contacts.length || d.modifiers.length || d.callerName)
}

/** fromCall starts an entry from a 911 call: its location, caller and callback. */
export function fromCall(c: Call): Draft {
  const d = emptyDraft()
  d.call = c.id
  d.callback = c.callback ?? ''
  d.callerName = c.callerName ?? ''
  if (c.location) {
    d.address = c.location.address ?? ''
    if (c.location.lat != null && c.location.lon != null) {
      d.lat = c.location.lat.toFixed(6)
      d.lon = c.location.lon.toFixed(6)
    }
  }
  return d
}

/** fromIncident takes up an incident already made: its fields to update, and its first call. */
export function fromIncident(inc: Incident): Draft {
  return {
    ...emptyDraft(), number: inc.number, call: inc.calls[0], type: inc.type, priority: inc.priority, modifiers: inc.modifiers ?? [],
    address: inc.location?.address ?? '', lat: inc.location?.lat?.toFixed(6) ?? '', lon: inc.location?.lon?.toFixed(6) ?? '', verified: !!inc.locVerified,
    callerName: inc.caller?.name ?? '', callback: inc.caller?.callback ?? '', callerElsewhere: !!inc.callerAt, callerAddress: inc.callerAt?.address ?? '',
  }
}

export function pickAddress(d: Draft, r: SearchResult): Draft {
  return { ...d, address: r.label, lat: r.lat.toFixed(6), lon: r.lon.toFixed(6), verified: r.exact }
}

/** rankTypes orders call types for what was typed: exact code, code prefix, a word of the name, anywhere. */
export function rankTypes(types: CallType[], q: string): CallType[] {
  const s = q.trim().toLowerCase()
  if (!s) return [...types].sort((a, b) => (a.group ?? '').localeCompare(b.group ?? '') || a.priority - b.priority || a.name.localeCompare(b.name))
  const score = (c: CallType) => {
    const code = c.code.toLowerCase()
    const name = c.name.toLowerCase()
    if (code === s) return 0
    if (code.startsWith(s)) return 1
    if (name.split(/[\s/,-]+/).some((w) => w.startsWith(s))) return 2
    if (name.includes(s) || (c.group ?? '').toLowerCase().includes(s)) return 3
    return 9
  }
  return types.map((c) => [score(c), c] as const).filter(([n]) => n < 9).sort((a, b) => a[0] - b[0] || a[1].priority - b[1].priority || a[1].name.localeCompare(b[1].name)).map(([, c]) => c)
}

const num = (s: string) => (s.trim() === '' ? undefined : Number(s))

/** location is the incident location to send, or undefined. */
export function location(d: Draft) {
  const lat = num(d.lat)
  const lon = num(d.lon)
  if (!d.address.trim() && (lat == null || lon == null)) return undefined
  return { address: d.address.trim() || undefined, lat: lat != null && !Number.isNaN(lat) ? lat : undefined, lon: lon != null && !Number.isNaN(lon) ? lon : undefined, source: 'dispatcher' }
}

export function callerAt(d: Draft) {
  if (!d.callerElsewhere) return undefined
  const lat = num(d.callerLat)
  const lon = num(d.callerLon)
  return { address: d.callerAddress.trim() || undefined, lat, lon, source: 'dispatcher' }
}

/** noteText is a note written before submission, with when it was written. */
export function noteText(n: Note, submitted: string): string {
  const late = Date.parse(submitted) - Date.parse(n.at) > 30_000
  if (!late) return n.text
  const t = new Date(n.at)
  const hh = `${String(t.getHours()).padStart(2, '0')}:${String(t.getMinutes()).padStart(2, '0')}:${String(t.getSeconds()).padStart(2, '0')}`
  return `${n.text} (written ${hh}, before the incident was submitted)`
}

/** dupKey identifies the duplicates shown, so a second press of submit goes ahead with them seen. */
export const dupKey = (ids: string[]) => ids.slice().sort().join(',')
