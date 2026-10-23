import { beforeEach, describe, expect, it } from 'vitest'
import { dock, edgeAt, loadWorkspace, removeSection, resize, saveWorkspace, sectionsIn, stocksFor, toggleSection, withStock, type Workspace } from './workspace'

const empty: Workspace = { root: null, cmd: true, locked: false }

describe('workspace', () => {
  beforeEach(() => localStorage.clear())

  it('starts with the duty’s first stock workspace, locked', () => {
    const w = loadWorkspace('ann', 'dispatcher')
    expect(w.stock).toBe('full')
    expect(w.locked).toBe(true)
    expect(sectionsIn(w.root)).toEqual(['queue', 'map', 'units', 'incident'])
    expect(loadWorkspace('ann', 'police').stock).toBe('police')
    expect(loadWorkspace('ann', 'fire').stock).toBe('houses')
    for (const d of ['dispatcher', 'police', 'fire', 'ems', 'supervisor', 'admin', 'viewer']) expect(stocksFor(d)).toHaveLength(3)
  })

  it('docks a section on an edge, sharing that space, and trades places in the middle', () => {
    let w = dock(empty, 'map', null, 'center')
    expect(w.root).toEqual({ t: 'leaf', s: 'map' })
    w = dock(w, 'queue', 'map', 'left')
    expect(w.root).toEqual({ t: 'split', dir: 'row', kids: [{ t: 'leaf', s: 'queue' }, { t: 'leaf', s: 'map' }], sizes: [0.5, 0.5] })
    w = dock(w, 'units', 'map', 'bottom')
    expect(sectionsIn(w.root)).toEqual(['queue', 'map', 'units'])
    // Same direction as its parent: one more child, not a split inside a split.
    w = dock(w, 'incident', 'queue', 'right')
    expect(w.root?.t === 'split' && w.root.kids.length).toBe(3)
    w = dock(w, 'units', 'queue', 'center')
    expect(sectionsIn(w.root)).toEqual(['units', 'incident', 'map', 'queue'])
    // Moving one already shown takes it from where it was.
    w = dock(w, 'map', 'units', 'top')
    expect(sectionsIn(w.root).filter((s) => s === 'map')).toHaveLength(1)
    // A section new to the workspace dropped in the middle takes that place.
    w = dock(w, 'police', 'incident', 'center')
    expect(sectionsIn(w.root)).not.toContain('incident')
    expect(w.stock).toBeUndefined()
  })

  it('removes sections down to nothing, and toggles them back', () => {
    let w = stocksFor('dispatcher')[0].ws
    for (const s of sectionsIn(w.root)) w = removeSection(w, s)
    expect(w.root).toBeNull()
    w = toggleSection(w, 'stations')
    expect(w.root).toEqual({ t: 'leaf', s: 'stations' })
    w = toggleSection(w, 'stations')
    expect(w.root).toBeNull()
  })

  it('resizes within limits', () => {
    const w = resize(stocksFor('dispatcher')[1].ws, [], 0, 0.99)
    const r = w.root!
    expect(r.t === 'split' && r.sizes[0]).toBeCloseTo(0.94)
    expect(r.t === 'split' && r.sizes[0] + r.sizes[1]).toBeCloseTo(1)
  })

  it('keeps it per user and duty; a stock keeps the lock; bad copies are not used', () => {
    saveWorkspace('ann', 'dispatcher', { ...empty, locked: true })
    expect(loadWorkspace('ann', 'dispatcher').root).toBeNull()
    expect(loadWorkspace('bob', 'dispatcher').stock).toBe('full')
    expect(loadWorkspace('ann', 'supervisor').stock).toBe('full')
    expect(withStock({ ...empty, locked: false }, stocksFor('police')[0]).locked).toBe(false)
    localStorage.setItem('bd.workspace.ann.dispatcher', JSON.stringify({ root: { t: 'split', dir: 'row', kids: [{ t: 'leaf', s: 'map' }, { t: 'leaf', s: 'map' }], sizes: [0.5, 0.5] } }))
    expect(loadWorkspace('ann', 'dispatcher').stock).toBe('full')
  })

  it('finds the edge a drop is nearest', () => {
    expect(edgeAt(50, 50, 100, 100)).toBe('center')
    expect(edgeAt(5, 50, 100, 100)).toBe('left')
    expect(edgeAt(50, 95, 100, 100)).toBe('bottom')
  })
})
