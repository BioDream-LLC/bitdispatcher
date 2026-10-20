import { describe, expect, it } from 'vitest'
import { dirty, emptyDraft, fromCall, location, noteText, rankTypes } from './draft'
import type { Call, CallType } from '../types'

const ct = (code: string, name: string, priority = 3, group?: string): CallType => ({ code, name, priority, discipline: 'police', runCard: [], group })

describe('call entry draft', () => {
  it('ranks call types: exact code, code prefix, a word of the name, then anywhere', () => {
    const types = [ct('SF', 'Structure fire', 1), ct('SFA', 'Smoke from attic', 2), ct('ROB', 'Robbery', 1), ct('ALARM', 'Burglar alarm'), ct('BURG', 'Burglary in progress', 1)]
    expect(rankTypes(types, 'sf').map((c) => c.code)).toEqual(['SF', 'SFA'])
    expect(rankTypes(types, 'burg').map((c) => c.code)).toEqual(['BURG', 'ALARM'])
    expect(rankTypes(types, 'fire').map((c) => c.code)).toEqual(['SF'])
    expect(rankTypes(types, 'zzz')).toEqual([])
  })
  it('starts from a call, and is dirty only with typed work not yet submitted', () => {
    const c = { id: 'c1', sources: [], state: 'answered', callback: '+15405550100', callerName: 'DOE, J', location: { lat: 37.27, lon: -79.94 }, locations: 1, start: '', updated: '' } as Call
    const d = fromCall(c)
    expect(d.call).toBe('c1')
    expect(d.lat).toBe('37.270000')
    expect(dirty(emptyDraft())).toBe(false)
    expect(dirty({ ...d, notes: [{ at: '', text: 'x' }] })).toBe(true)
    expect(dirty({ ...d, notes: [{ at: '', text: 'x' }], number: '26-000001' })).toBe(false)
    expect(location(d)).toEqual({ address: undefined, lat: 37.27, lon: -79.94, source: 'dispatcher' })
  })
  it('keeps the time of a note written before submission', () => {
    expect(noteText({ at: '2026-10-20T13:00:00Z', text: 'gun seen' }, '2026-10-20T13:00:10Z')).toBe('gun seen')
    expect(noteText({ at: '2026-10-20T13:00:00Z', text: 'gun seen' }, '2026-10-20T13:02:00Z')).toMatch(/gun seen \(written \d\d:00:00, before the incident was submitted\)/)
  })
})
