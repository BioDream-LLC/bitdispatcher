import { describe, expect, it } from 'vitest'
import { applyLens, parseLens } from './lens'
import { checkDue, overdue } from './util'
import type { View } from './types'

const unit = (id: string, type: string, status = 'available', extra = {}) => ({ id, type, status, statusAt: '2026-10-19T10:00:00Z', caps: [], ...extra })
const view = {
  settings: {
    timers: { onscene: 20, markout: 30 },
    unitTypes: [{ code: 'engine', name: 'Engine', discipline: 'fire', capabilities: ['engine'] }, { code: 'patrol', name: 'Patrol', discipline: 'police', capabilities: ['police'] }],
    markOuts: [{ code: 'MEAL', name: 'Meal', minutes: 45 }],
  },
  callTypes: [
    { code: 'SF', name: 'Fire', priority: 1, discipline: 'fire', runCard: [{ capability: 'engine', count: 1 }] },
    { code: 'MVA', name: 'Crash', priority: 1, discipline: 'fire', runCard: [{ capability: 'engine', count: 1 }, { capability: 'police', count: 1 }] },
    { code: 'THEFT', name: 'Theft', priority: 3, discipline: 'police', runCard: [{ capability: 'police', count: 1 }] },
  ],
  units: [unit('E1', 'engine'), unit('P1', 'patrol', 'markout', { markOut: { code: 'MEAL' } })],
  incidents: [
    { number: 'A', type: 'SF', units: [], status: 'pending' },
    { number: 'B', type: 'MVA', units: [], status: 'pending' },
    { number: 'C', type: 'THEFT', units: [], status: 'pending' },
  ],
  calls: [{ id: 'c1' }, { id: 'c2', incident: 'A' }, { id: 'c3', incident: 'C' }],
  emergencies: [{ key: 'x' }],
} as unknown as View

describe('lens', () => {
  it('parses only known disciplines', () => expect(parseLens('police,bogus, fire')).toEqual(['police', 'fire']))
  it('shows everything without a lens', () => expect(applyLens(view, [])).toBe(view))
  it('a police screen sees police calls, and fire calls that need police', () => {
    const v = applyLens(view, ['police'])
    expect(v.incidents.map((i) => i.number)).toEqual(['B', 'C'])
    expect(v.units.map((u) => u.id)).toEqual(['P1'])
    // Calls not yet on an incident stay; calls on hidden incidents go. Emergencies always stay.
    expect(v.calls.map((c) => c.id)).toEqual(['c1', 'c3'])
    expect(v.emergencies.length).toBe(1)
  })
})

describe('status checks', () => {
  it('a mark-out uses its own time, and a check restarts it', () => {
    const p = view.units[1]
    expect(checkDue(view, p)).toBe(Date.parse('2026-10-19T10:45:00Z'))
    const checked = { ...p, checked: '2026-10-19T10:40:00Z' }
    expect(checkDue(view, checked)).toBe(Date.parse('2026-10-19T11:25:00Z'))
    expect(overdue(view, checked, Date.parse('2026-10-19T11:30:00Z'))).toBe(5 * 60000)
    expect(checkDue(view, view.units[0])).toBe(0) // available: no timer
  })
})
