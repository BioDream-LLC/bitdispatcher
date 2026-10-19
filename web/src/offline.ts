// Working through an outage: the console keeps the last board it had (in this browser), shows it when the
// server cannot be reached, and keeps what the dispatcher records meanwhile (new incidents, notes, unit
// statuses) in an outbox that is sent, marked as entered offline, when the server is back.

import { api } from './api'
import type { View } from './types'

const DB = 'bitdispatcher'
const STORE = 'snapshot'

function db(): Promise<IDBDatabase> {
  return new Promise((res, rej) => {
    if (typeof indexedDB === 'undefined') return rej(new Error('no IndexedDB'))
    const r = indexedDB.open(DB, 1)
    r.onupgradeneeded = () => r.result.createObjectStore(STORE)
    r.onsuccess = () => res(r.result)
    r.onerror = () => rej(r.error)
  })
}

let last = 0

/** saveSnapshot keeps the board, at most every ten seconds. */
export async function saveSnapshot(v: View) {
  if (Date.now() - last < 10000) return
  last = Date.now()
  try {
    const d = await db()
    d.transaction(STORE, 'readwrite').objectStore(STORE).put({ at: Date.now(), view: v }, 'board')
  } catch {
    // A browser without storage simply has no offline board.
  }
}

/** loadSnapshot is the last board kept, if any. */
export async function loadSnapshot(): Promise<{ at: number; view: View } | null> {
  try {
    const d = await db()
    return await new Promise((res) => {
      const r = d.transaction(STORE).objectStore(STORE).get('board')
      r.onsuccess = () => res(r.result ?? null)
      r.onerror = () => res(null)
    })
  } catch {
    return null
  }
}

export type OutItem =
  | { id: string; at: number; by: string; kind: 'incident'; type: string; address: string; text: string }
  | { id: string; at: number; by: string; kind: 'note'; incident: string; text: string }
  | { id: string; at: number; by: string; kind: 'status'; unit: string; status: string; incident?: string }

const KEY = 'bd.outbox'

export function outbox(): OutItem[] {
  try {
    return JSON.parse(localStorage.getItem(KEY) ?? '[]')
  } catch {
    return []
  }
}

function save(items: OutItem[]) {
  localStorage.setItem(KEY, JSON.stringify(items))
  window.dispatchEvent(new Event('bd:outbox'))
}

type New<T> = T extends unknown ? Omit<T, 'id' | 'at'> & { at?: number } : never

/** keep adds an entry to the outbox. */
export function keep(item: New<OutItem>) {
  save([...outbox(), { ...item, id: Math.random().toString(36).slice(2), at: item.at ?? Date.now() } as OutItem])
}

export function drop(id: string) {
  save(outbox().filter((x) => x.id !== id))
}

const when = (t: number) => new Date(t).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

/** send sends the outbox in order; what the server refuses stays, with the reason. */
export async function send(): Promise<{ sent: number; failed: { item: OutItem; error: string }[] }> {
  const items = outbox()
  const failed: { item: OutItem; error: string }[] = []
  let sent = 0
  for (const it of items) {
    const mark = `(entered offline at ${when(it.at)} by ${it.by})`
    try {
      if (it.kind === 'incident') {
        await api.post('/incidents', { type: it.type, location: { address: it.address, source: 'dispatcher' }, narrative: `${it.text} ${mark}`.trim() })
      } else if (it.kind === 'note') {
        await api.post(`/incidents/${it.incident}/notes`, { text: `${it.text} ${mark}` })
      } else {
        await api.post(`/units/${it.unit}/status`, { status: it.status, incident: it.incident, reason: mark })
      }
      sent++
      drop(it.id)
    } catch (e) {
      failed.push({ item: it, error: e instanceof Error ? e.message : String(e) })
    }
  }
  return { sent, failed }
}

/** registerWorker keeps the interface itself available offline (the service worker caches the app). */
export function registerWorker() {
  if ('serviceWorker' in navigator && location.protocol === 'https:') {
    navigator.serviceWorker.register('./sw.js').catch(() => {})
  }
}
