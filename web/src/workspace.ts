import { useEffect, useState } from 'react'
import type { Duty } from './types'

// The console's workspace: which sections it shows and how they share the screen. Sections are docked:
// dropped on a section's edge, one splits that space with it; dropped on its middle, the two trade
// places. Nothing overlaps and nothing is left empty. Each user keeps a workspace per duty, in this
// browser; three stock workspaces per duty are a click away, and a lock stops changes.

export type SectionId = 'queue' | 'map' | 'incident' | 'units' | 'stations' | 'police'
export type Pane = { t: 'leaf'; s: SectionId } | { t: 'split'; dir: 'row' | 'col'; kids: Pane[]; sizes: number[] }
export type Workspace = { root: Pane | null; cmd: boolean; locked: boolean; stock?: string }
export type Edge = 'left' | 'right' | 'top' | 'bottom' | 'center'

export type SectionDef = { id: SectionId; title: string; hint: string; tone: 'queue' | 'map' | 'panel' | 'units' | 'cards' | 'table' }
export const SECTIONS: SectionDef[] = [
  { id: 'queue', title: 'Calls and incidents', hint: 'The 911 calls coming in and the incidents waiting or working.', tone: 'queue' },
  { id: 'map', title: 'Map', hint: 'Incidents, units and calls where they are.', tone: 'map' },
  { id: 'incident', title: 'Incident', hint: 'The incident you are working, or what needs attention next.', tone: 'panel' },
  { id: 'units', title: 'Unit board', hint: 'Every unit and its status, by discipline.', tone: 'units' },
  { id: 'stations', title: 'Fire houses', hint: 'Each station with its apparatus, who covers it and move-ups.', tone: 'cards' },
  { id: 'police', title: 'Police board', hint: 'Every car: what it is doing, how long, status checks and mark-outs.', tone: 'table' },
]
export const sectionDef = (id: SectionId) => SECTIONS.find((s) => s.id === id)!

const leaf = (s: SectionId): Pane => ({ t: 'leaf', s })
const row = (kids: Pane[], sizes: number[]): Pane => ({ t: 'split', dir: 'row', kids, sizes })
const col = (kids: Pane[], sizes: number[]): Pane => ({ t: 'split', dir: 'col', kids, sizes })

export type Stock = { id: string; title: string; hint: string; ws: Workspace }
const full: Stock = {
  id: 'full', title: 'Full console', hint: 'Everything at once: calls, map, units and the incident.',
  ws: { root: row([leaf('queue'), col([leaf('map'), leaf('units')], [0.74, 0.26]), leaf('incident')], [0.22, 0.5, 0.28]), cmd: true, locked: true },
}
const starter: Stock = {
  id: 'starter', title: 'Getting started', hint: 'Just the calls and the incident you are working. Add more when you are ready.',
  ws: { root: row([leaf('queue'), leaf('incident')], [0.38, 0.62]), cmd: true, locked: true },
}
const mapFirst: Stock = {
  id: 'map-first', title: 'Map first', hint: 'A large map with the units under it, and the incident beside.',
  ws: { root: row([col([leaf('map'), leaf('units')], [0.7, 0.3]), leaf('incident')], [0.64, 0.36]), cmd: true, locked: true },
}
const police: Stock = {
  id: 'police', title: 'Police cars and map', hint: 'The police board with status checks, the map and the incident.',
  ws: { root: row([leaf('police'), col([leaf('map'), leaf('incident')], [0.55, 0.45])], [0.58, 0.42]), cmd: true, locked: true },
}
const houses: Stock = {
  id: 'houses', title: 'Fire houses', hint: 'Calls, every station with its apparatus, and the incident.',
  ws: { root: row([leaf('queue'), leaf('stations'), leaf('incident')], [0.24, 0.46, 0.3]), cmd: true, locked: true },
}
const overview: Stock = {
  id: 'overview', title: 'Overview', hint: 'For watching the floor: map, units, fire houses and police.',
  ws: { root: row([col([leaf('map'), leaf('units')], [0.6, 0.4]), col([leaf('stations'), leaf('police')], [0.5, 0.5])], [0.55, 0.45]), cmd: false, locked: true },
}

/** The three stock workspaces of a duty, the first its default. */
export function stocksFor(duty?: Duty | string): Stock[] {
  switch (duty) {
    case 'police': return [police, starter, full]
    case 'fire':
    case 'ems': return [houses, starter, full]
    case 'supervisor':
    case 'admin': return [full, overview, starter]
    default: return [full, starter, mapFirst]
  }
}

const copy = (w: Workspace): Workspace => JSON.parse(JSON.stringify(w))

export const sectionsIn = (p: Pane | null): SectionId[] => (!p ? [] : p.t === 'leaf' ? [p.s] : p.kids.flatMap(sectionsIn))

// tidy merges a split's one child into its place, a split inside one of the same direction into it,
// and makes the sizes add up to 1.
function tidy(p: Pane | null): Pane | null {
  if (!p || p.t === 'leaf') return p
  const kids: Pane[] = []
  const sizes: number[] = []
  p.kids.forEach((k, i) => {
    const t = tidy(k)
    if (!t) return
    if (t.t === 'split' && t.dir === p.dir) {
      t.kids.forEach((kk, j) => { kids.push(kk); sizes.push(t.sizes[j] * p.sizes[i]) })
    } else {
      kids.push(t)
      sizes.push(p.sizes[i])
    }
  })
  if (!kids.length) return null
  if (kids.length === 1) return kids[0]
  const sum = sizes.reduce((a, b) => a + b, 0) || 1
  return { t: 'split', dir: p.dir, kids, sizes: sizes.map((s) => s / sum) }
}

/** The workspace without that section. */
export function removeSection(w: Workspace, s: SectionId): Workspace {
  const drop = (p: Pane | null): Pane | null => {
    if (!p) return null
    if (p.t === 'leaf') return p.s === s ? null : p
    const kids = p.kids.map(drop)
    return { ...p, kids: kids.filter((k): k is Pane => !!k), sizes: p.sizes.filter((_, i) => kids[i]) }
  }
  return { ...copy(w), root: tidy(drop(copy(w).root)), stock: undefined }
}

/** Drops a section on another's edge (sharing its space) or middle (trading places, or taking its place
 *  when the section was not shown). On an empty workspace it fills the screen. */
export function dock(w: Workspace, s: SectionId, target: SectionId | null, edge: Edge): Workspace {
  const out = copy(w)
  out.stock = undefined
  if (target === s) return w
  const had = sectionsIn(out.root).includes(s)
  if (!out.root || !target || !sectionsIn(out.root).includes(target)) {
    if (had) return w
    return { ...out, root: out.root ? tidy(row([out.root, leaf(s)], [0.7, 0.3])) : leaf(s) }
  }
  if (edge === 'center') {
    const swap = (p: Pane): Pane => {
      if (p.t === 'split') return { ...p, kids: p.kids.map(swap) }
      if (p.s === target) return leaf(s)
      if (p.s === s) return leaf(target)
      return p
    }
    return { ...out, root: swap(out.root) }
  }
  const base = had ? removeSection(out, s).root! : out.root
  const dir = edge === 'left' || edge === 'right' ? 'row' : 'col'
  const before = edge === 'left' || edge === 'top'
  const put = (p: Pane): Pane => {
    if (p.t === 'leaf') {
      if (p.s !== target) return p
      return { t: 'split', dir, kids: before ? [leaf(s), p] : [p, leaf(s)], sizes: [0.5, 0.5] }
    }
    return { ...p, kids: p.kids.map(put) }
  }
  return { ...out, root: tidy(put(base)) }
}

/** Shows a section (to the right of everything) or takes it away. */
export function toggleSection(w: Workspace, s: SectionId): Workspace {
  return sectionsIn(w.root).includes(s) ? removeSection(w, s) : dock(w, s, null, 'right')
}

/** Moves the border after child i of the split at path, to that fraction of the split. */
export function resize(w: Workspace, path: number[], i: number, at: number): Workspace {
  const out = copy(w)
  let p = out.root
  for (const j of path) p = p && p.t === 'split' ? p.kids[j] : null
  if (!p || p.t !== 'split' || i < 0 || i + 1 >= p.kids.length) return w
  const start = p.sizes.slice(0, i).reduce((a, b) => a + b, 0)
  const pair = p.sizes[i] + p.sizes[i + 1]
  const min = Math.min(0.06, pair / 2)
  const a = Math.min(Math.max(at - start, min), pair - min)
  p.sizes[i] = a
  p.sizes[i + 1] = pair - a
  return { ...out, stock: undefined }
}

/** Which edge of a box a point is nearest, its middle third being the centre. */
export function edgeAt(x: number, y: number, w: number, h: number): Edge {
  const fx = x / w, fy = y / h
  if (fx > 0.3 && fx < 0.7 && fy > 0.3 && fy < 0.7) return 'center'
  const d = { left: fx, right: 1 - fx, top: fy, bottom: 1 - fy }
  return (Object.keys(d) as Edge[]).reduce((a, b) => (d[b as keyof typeof d] < d[a as keyof typeof d] ? b : a))
}

function valid(p: unknown): p is Pane {
  const x = p as Pane
  if (!x || typeof x !== 'object') return false
  if (x.t === 'leaf') return SECTIONS.some((s) => s.id === x.s)
  return x.t === 'split' && (x.dir === 'row' || x.dir === 'col') && Array.isArray(x.kids) && Array.isArray(x.sizes) && x.kids.length === x.sizes.length && x.kids.every(valid)
}

const storeKey = (user: string, duty: string) => `bd.workspace.${user || '-'}.${duty || '-'}`

export function loadWorkspace(user: string, duty: string): Workspace {
  try {
    const w = JSON.parse(localStorage.getItem(storeKey(user, duty)) ?? '') as Workspace
    if (w && (w.root === null || valid(w.root))) {
      // A section shown twice (an old or edited copy) is kept once.
      const seen = new Set<SectionId>()
      const ok = sectionsIn(w.root).every((s) => !seen.has(s) && seen.add(s))
      if (ok) return { root: tidy(w.root), cmd: w.cmd !== false, locked: !!w.locked, stock: w.stock }
    }
  } catch {
    /* none kept yet */
  }
  const s = stocksFor(duty)[0]
  return { ...copy(s.ws), stock: s.id }
}

export function saveWorkspace(user: string, duty: string, w: Workspace) {
  localStorage.setItem(storeKey(user, duty), JSON.stringify(w))
  window.dispatchEvent(new CustomEvent('bd:workspace', { detail: storeKey(user, duty) }))
}

/** The signed-in user's workspace for this duty, kept up to date wherever it is changed. */
export function useWorkspace(user: string, duty: string): [Workspace, (w: Workspace) => void] {
  const [w, setW] = useState(() => loadWorkspace(user, duty))
  useEffect(() => {
    setW(loadWorkspace(user, duty))
    const on = () => setW(loadWorkspace(user, duty))
    window.addEventListener('bd:workspace', on)
    window.addEventListener('storage', on)
    return () => {
      window.removeEventListener('bd:workspace', on)
      window.removeEventListener('storage', on)
    }
  }, [user, duty])
  return [w, (n: Workspace) => saveWorkspace(user, duty, n)]
}

/** Picks a stock workspace, keeping the lock as it is. */
export const withStock = (w: Workspace, s: Stock): Workspace => ({ ...copy(s.ws), locked: w.locked, stock: s.id })
