import { describe, expect, it } from 'vitest'
import { ago, clock, km, phone, statusMeta, uncertainty } from './util'

describe('formatting', () => {
  it('formats elapsed timers', () => {
    const t = Date.UTC(2026, 9, 18, 12, 0, 0)
    expect(clock(t, t + 65_000)).toBe('1:05')
    expect(clock(t, t + 3_725_000)).toBe('1:02:05')
    expect(clock(undefined, t)).toBe('–')
    expect(ago(t, t + 90_000)).toBe('1m')
  })
  it('formats North American callback numbers and leaves others alone', () => {
    expect(phone('+17175550100')).toBe('(717) 555-0100')
    expect(phone('+442071234567')).toBe('+442071234567')
  })
  it('gives every status a text code, so colour is never the only signal', () => {
    for (const s of ['available', 'dispatched', 'enroute', 'onscene', 'transporting', 'atdestination', 'oos']) {
      expect(statusMeta(s).code.length).toBeGreaterThan(1)
    }
  })
  it('describes location uncertainty', () => {
    expect(uncertainty({ radius: 35.4, confidence: 90, method: 'GPS' })).toBe('±35 m · 90% · GPS')
    expect(km(1500)).toBe('1.5 km')
  })
})
