import type { Lens } from './lens'
import { screenDef, type ScreenKind } from './screens'

// A position's screens: which windows it opens, with which lens, on which monitor and where. Kept in this
// browser (the workstation), per position, so the next shift at this desk gets the same layout.

export type Bounds = { x: number; y: number; w: number; h: number }
export type Slot = { id: string; kind: ScreenKind; lens: Lens; monitor?: string /* Monitor.key */; bounds?: Bounds }
export type Layout = { slots: Slot[]; offer: boolean } // offer: offer to open them at sign-in

const key = (position: string) => `bd.screens.${position || 'default'}`

export function loadLayout(position: string): Layout {
  try {
    const l = JSON.parse(localStorage.getItem(key(position)) ?? '') as Layout
    if (Array.isArray(l.slots)) return { slots: l.slots, offer: !!l.offer }
  } catch {
    /* none yet */
  }
  return { slots: [], offer: true }
}

export function saveLayout(position: string, l: Layout) {
  localStorage.setItem(key(position), JSON.stringify(l))
  window.dispatchEvent(new Event('bd:layout'))
}

/** Remembers where a screen's window is, so it opens there next time. */
export function rememberBounds(position: string, slot: string, b: Bounds) {
  const l = loadLayout(position)
  const s = l.slots.find((x) => x.id === slot)
  if (!s || (s.bounds && s.bounds.x === b.x && s.bounds.y === b.y && s.bounds.w === b.w && s.bounds.h === b.h)) return
  s.bounds = b
  saveLayout(position, l)
}

// The Window Management API (Chrome and Edge): the monitors, by name and place. Other browsers open
// windows where they are told, or where they were last.
// key is where the monitor is (left,top): two monitors of the same model often report the same label.
export type Monitor = { key: string; label: string; left: number; top: number; width: number; height: number; availLeft: number; availTop: number; availWidth: number; availHeight: number; isPrimary: boolean; isInternal?: boolean }
type ScreenDetails = { screens: Monitor[]; currentScreen: Monitor; addEventListener: (t: string, f: () => void) => void }

export const canPlace = () => 'getScreenDetails' in window

export async function monitors(): Promise<{ list: Monitor[]; current?: Monitor } | null> {
  if (!canPlace()) return null
  const d = await (window as unknown as { getScreenDetails: () => Promise<ScreenDetails> }).getScreenDetails()
  const list: Monitor[] = []
  for (const m of d.screens) {
    const x = plain(m)
    if (list.some((y) => y.key === x.key)) continue
    list.push(x)
  }
  // Unnamed or same-named monitors are told apart by number, in order from the left.
  list.sort((a, b) => a.left - b.left || a.top - b.top)
  const same = new Map<string, number>()
  list.forEach((m) => same.set(m.label, (same.get(m.label) ?? 0) + 1))
  list.forEach((m, i) => {
    if (!m.label || (same.get(m.label) ?? 0) > 1) m.label = `${m.label || 'Monitor'} ${i + 1}`
  })
  const cur = plain(d.currentScreen)
  return { list, current: list.find((m) => m.key === cur.key) ?? cur }
}

// plain copies a ScreenDetailed's fields (they are getters, lost by spreading).
function plain(m: Monitor): Monitor {
  return { key: `${m.left},${m.top}`, label: m.label, left: m.left, top: m.top, width: m.width, height: m.height, availLeft: m.availLeft, availTop: m.availTop, availWidth: m.availWidth, availHeight: m.availHeight, isPrimary: m.isPrimary, isInternal: m.isInternal }
}

export function slotURL(s: Slot): string {
  const base = location.origin + location.pathname
  const q = new URLSearchParams()
  if (s.lens.length) q.set('d', s.lens.join(','))
  q.set('slot', s.id)
  return `${base}#${screenDef(s.kind).route}?${q}`
}

/** Opens (or brings forward) one screen's window: on its monitor, filling it, else where it was last. */
export function openSlot(s: Slot, mons?: Monitor[]): Window | null {
  const m = mons?.find((x) => x.key === s.monitor) ?? mons?.find((x) => x.label === s.monitor)
  let f = 'popup'
  if (m) f += `,left=${m.availLeft},top=${m.availTop},width=${m.availWidth},height=${m.availHeight}`
  else if (s.bounds) f += `,left=${s.bounds.x},top=${s.bounds.y},width=${s.bounds.w},height=${s.bounds.h}`
  else f += ',width=1600,height=900'
  const w = window.open(slotURL(s), `bd-screen-${s.id}`, f)
  if (w && m) {
    // A window already open keeps its place; move it.
    try {
      w.moveTo(m.availLeft, m.availTop)
      w.resizeTo(m.availWidth, m.availHeight)
    } catch {
      /* the browser decides */
    }
  }
  w?.focus()
  return w
}

export const newSlotId = () => Math.random().toString(36).slice(2, 8)

// Which screens are open now: each screen window answers on this channel.
export const screensChannel = typeof BroadcastChannel === 'undefined' ? null : new BroadcastChannel('bitdispatcher.screens')
