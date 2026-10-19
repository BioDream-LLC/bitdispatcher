import { describe, expect, it } from 'vitest'
import { clusters, floorText, metres, minutes, repeats } from './dream'
import type { Call } from './types'

const call = (id: string, lat: number, lon: number, startMin: number, extra: Partial<Call> = {}): Call => ({
  id, sources: ['bitvoice'], state: 'answered', locations: 1, start: new Date(Date.UTC(2026, 9, 19, 12, startMin)).toISOString(), updated: '',
  location: { lat, lon, radius: 10 }, ...extra,
})

describe('clusters', () => {
  it('groups callers near each other in time and place', () => {
    const cs = [call('a', 37.27, -79.94, 0), call('b', 37.2705, -79.9402, 2), call('c', 37.30, -79.90, 1), call('d', 37.2702, -79.9401, 30)]
    const g = clusters(cs, {})
    expect(g).toHaveLength(1)
    expect(g[0].calls.map((c) => c.id)).toEqual(['a', 'b'])
  })
  it('names the incident one of them is on, and never joins two incidents', () => {
    const g = clusters([call('a', 37.27, -79.94, 0, { incident: '26-000001' }), call('b', 37.2701, -79.94, 1)], {})
    expect(g[0].incident).toBe('26-000001')
    expect(clusters([call('a', 37.27, -79.94, 0, { incident: 'x' }), call('b', 37.2701, -79.94, 1, { incident: 'y' })], {})).toHaveLength(0)
  })
  it('leaves out non-emergency calls', () => {
    expect(clusters([call('a', 37.27, -79.94, 0), call('b', 37.2701, -79.94, 1, { screen: { nonEmergency: true } })], {})).toHaveLength(0)
  })
})

describe('helpers', () => {
  it('measures, counts repeats, and says the floor', () => {
    expect(Math.round(metres(37.27, -79.94, 37.28, -79.94))).toBe(1112)
    const now = Date.UTC(2026, 9, 19, 12, 5)
    const r = repeats([call('a', 0, 0, 0, { callback: '+1' }), call('b', 0, 0, 4, { callback: '+1' }), call('c', 0, 0, 4, { callback: '+2' })], now)
    expect(r.get('+1')).toBe(2)
    expect(floorText({ location: { civic: { floor: '3' }, altitude: 312.4, vRadius: 3 } })).toBe('floor 3 · altitude 312 m ±3')
    expect(minutes(135)).toBe('2 h 15')
  })
})
