import { useSyncExternalStore } from 'react'
import { ApiError, api } from './api'
import type { Health, Notice, Session, View } from './types'
import { parseLens, type Lens } from './lens'
import { loadSnapshot, saveSnapshot } from './offline'

// One store for the whole interface: the server's state document, refreshed when the event stream says
// something changed, plus what this dispatcher has selected. Nothing polls.

export type Activity = Notice & { at: number }

type Store = {
  me: Session | null
  meChecked: boolean
  view: View | null
  offset: number // server clock minus this clock, in ms
  stream: 'connecting' | 'live' | 'down'
  activity: Activity[]
  selected: string | null // incident number
  selectedCall: string | null
  selectedUnit: string | null
  pick: string[] // units chosen for the next dispatch
  loadError: string | null
  lens: Lens // the disciplines this window shows; empty for all
  offline: number // when the board shown was kept, while the server cannot be reached; 0 when live
}

let s: Store = {
  me: null, meChecked: false, view: null, offset: 0, stream: 'connecting', activity: [], selected: null, selectedCall: null,
  selectedUnit: null, pick: [], loadError: null, lens: initialLens(), offline: 0,
}

// initialLens is this window's lens: from its address (a screen opened with ?d=police), or what this
// browser tab last used.
function initialLens(): Lens {
  const q = new URLSearchParams(location.hash.split('?')[1] ?? location.search)
  if (q.has('d')) return parseLens(q.get('d'))
  return parseLens(sessionStorage.getItem('bd.lens'))
}

// Every window of this console (its screens, on other monitors) shares one selection: choosing an incident
// on the queue screen opens it on the incident screen and the map. Same browser, same origin, nothing sent.
const SELECTION = ['selected', 'selectedCall', 'selectedUnit', 'pick'] as const
type Selection = Pick<Store, (typeof SELECTION)[number]>
const channel = typeof BroadcastChannel === 'undefined' ? null : new BroadcastChannel('bitdispatcher.selection')
let fromPeer = false
channel?.addEventListener('message', (ev: MessageEvent<{ sel?: Selection; hello?: boolean }>) => {
  if (ev.data.hello) {
    // A new screen asks what is selected.
    if (s.selected || s.selectedCall || s.selectedUnit) channel.postMessage({ sel: pickSel(s) })
    return
  }
  if (!ev.data.sel) return
  fromPeer = true
  try {
    set(ev.data.sel)
  } finally {
    fromPeer = false
  }
})
channel?.postMessage({ hello: true })
const pickSel = (x: Store): Selection => ({ selected: x.selected, selectedCall: x.selectedCall, selectedUnit: x.selectedUnit, pick: x.pick })
const listeners = new Set<() => void>()

function set(patch: Partial<Store>) {
  const before = s
  s = { ...s, ...patch }
  if (!fromPeer && channel && SELECTION.some((k) => k in patch && patch[k] !== before[k])) channel.postMessage({ sel: pickSel(s) })
  listeners.forEach((l) => l())
}

export function useStore<T>(pick: (s: Store) => T): T {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => pick(s),
  )
}

export const store = {
  get: () => s,
  set,
  select(n: string | null) {
    set({ selected: n, pick: [], selectedCall: null })
  },
  selectCall(id: string | null) {
    set({ selectedCall: id })
  },
  selectUnit(id: string | null) {
    set({ selectedUnit: id })
  },
  togglePick(u: string) {
    const p = s.pick.includes(u) ? s.pick.filter((x) => x !== u) : [...s.pick, u]
    set({ pick: p })
  },
  setPick(p: string[]) {
    set({ pick: p })
  },
  setLens(l: Lens) {
    sessionStorage.setItem('bd.lens', l.join(','))
    set({ lens: l })
  },
}

export async function checkMe() {
  try {
    const r = await api.get<{ signedIn: boolean; session?: Session }>('/auth/me')
    set({ me: r.signedIn ? r.session! : null, meChecked: true })
    if (r.signedIn) localStorage.setItem('bd.me', JSON.stringify(r.session))
    else localStorage.removeItem('bd.me')
  } catch {
    // The server cannot be reached: a console that was signed in keeps working from its last board.
    const last = localStorage.getItem('bd.me')
    if (last) {
      set({ me: JSON.parse(last) as Session, meChecked: true })
      await goOffline()
      return
    }
    set({ meChecked: true })
  }
}

let loading: Promise<void> | null = null
let again = false

export async function load() {
  if (loading) {
    again = true
    return loading
  }
  loading = (async () => {
    do {
      again = false
      try {
        const v = await api.get<View>('/state')
        set({ view: v, offset: new Date(v.serverTime).getTime() - Date.now(), loadError: null, offline: 0 })
        saveSnapshot(v)
      } catch (e) {
        set({ loadError: e instanceof Error ? e.message : String(e) })
        await goOffline()
      }
    } while (again)
  })()
  try {
    await loading
  } finally {
    loading = null
  }
}

let es: EventSource | null = null
let timer: number | undefined

// connect opens the event stream; it reconnects by itself (EventSource does), and every reconnect reloads.
export function connect() {
  if (es) return
  es = new EventSource('./api/stream')
  es.addEventListener('hello', () => {
    set({ stream: 'live' })
    load()
  })
  es.addEventListener('change', (ev) => {
    const n = JSON.parse((ev as MessageEvent).data) as Notice
    if (n.type !== 'unit.heard' && n.type !== 'unit.location') {
      set({ activity: [{ ...n, at: Date.now() }, ...s.activity].slice(0, 60) })
    }
    window.clearTimeout(timer)
    timer = window.setTimeout(load, 80)
  })
  es.addEventListener('health', (ev) => {
    const h = JSON.parse((ev as MessageEvent).data) as Health
    if (s.view) set({ view: { ...s.view, health: h } })
  })
  es.addEventListener('signedout', () => {
    disconnect()
    set({ me: null })
  })
  es.onerror = () => {
    set({ stream: 'down' })
    // The stream drops for a moment on any restart; a server that stays away puts the console offline.
    window.setTimeout(() => {
      if (s.stream === 'down') fetch('./api/health', { cache: 'no-store' }).then((r) => { if (!r.ok) throw new Error() }).catch(() => goOffline())
    }, 4000)
  }
}

// goOffline shows the last board this browser kept, when the server cannot be reached.
async function goOffline() {
  if (s.offline) return
  const snap = s.view ? { at: Date.now(), view: s.view } : await loadSnapshot()
  if (snap) set({ view: s.view ?? snap.view, offline: snap.at })
}

export function disconnect() {
  es?.close()
  es = null
}

window.addEventListener('bd:signedout', () => {
  localStorage.removeItem('bd.me')
  disconnect()
  set({ me: null, view: null })
})

export function serverNow(): number {
  return Date.now() + s.offset
}

export const can = {
  dispatch: (me: Session | null) => !!me && me.role !== 'viewer' && me.role !== 'field',
  supervise: (me: Session | null) => !!me && (me.role === 'supervisor' || me.role === 'admin'),
  admin: (me: Session | null) => me?.role === 'admin',
}

/** Signs out here and on the server. */
export async function signOut(): Promise<boolean> {
  try {
    await api.post('/auth/logout')
  } catch (e) {
    // Open NCIC or state requests are finished or handed over first (the server refuses otherwise).
    if (e instanceof ApiError && e.status === 409) {
      window.dispatchEvent(new CustomEvent('bd:signoff-blocked', { detail: e.data }))
      return false
    }
  }
  window.dispatchEvent(new Event('bd:signedout'))
  return true
}

// A screen reached by a link in this window (#/screen/police?d=police) takes the lens its address names.
window.addEventListener('hashchange', () => {
  const q = new URLSearchParams(location.hash.split('?')[1] ?? '')
  if (q.has('d')) store.setLens(parseLens(q.get('d')))
})
