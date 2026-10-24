import { describe, expect, it } from 'vitest'
import { HARMONICS, renderTone, toneLength, tonePulses, type ToneName } from './tones'

const RATE = 48000
const TONES: ToneName[] = ['alarm', 'alert', 'chime', 'tick']

function rms(x: Float32Array) {
  let s = 0, n = 0
  for (const v of x) if (v !== 0) { s += v * v; n++ }
  return Math.sqrt(s / Math.max(1, n))
}
// pulses counts the separate bursts of sound (gaps over 15 ms).
function pulses(x: Float32Array) {
  const win = RATE / 1000
  let count = 0, on = false, quiet = 0
  for (let i = 0; i < x.length; i += win) {
    let e = 0
    for (let j = i; j < Math.min(x.length, i + win); j++) e = Math.max(e, Math.abs(x[j]))
    if (e > 0.005) { if (!on) count++; on = true; quiet = 0 } else if (++quiet > 15) on = false
  }
  return count
}
// energy at a frequency over one pulse (a single DFT bin).
function at(x: Float32Array, from: number, n: number, f: number) {
  let re = 0, im = 0
  for (let i = 0; i < n; i++) { const a = (2 * Math.PI * f * i) / RATE; re += x[from + i] * Math.cos(a); im += x[from + i] * Math.sin(a) }
  return Math.hypot(re, im) / n
}

describe('the priority tones', () => {
  it('never clip and get louder with priority', () => {
    const levels = TONES.map((t) => rms(renderTone(t, RATE)))
    for (const t of TONES) expect(Math.max(...renderTone(t, RATE).map(Math.abs))).toBeLessThan(0.5)
    for (let i = 1; i < levels.length; i++) expect(levels[i - 1]).toBeGreaterThan(levels[i])
    // Urgent is at least 9 dB over low: told apart by loudness as well as rhythm.
    expect(20 * Math.log10(levels[0] / levels[3])).toBeGreaterThan(9)
  })
  it('are told apart by rhythm: more, faster pulses for higher priority', () => {
    expect(TONES.map((t) => pulses(renderTone(t, RATE)))).toEqual([5, 3, 2, 1])
    expect(toneLength('alarm')).toBeLessThan(2)
    expect(toneLength('tick')).toBeLessThan(0.1)
  })
  it('have fundamentals of 150 to 1000 Hz with four or more harmonics, so they carry and are heard with some hearing loss', () => {
    for (const t of TONES) for (const p of tonePulses(t)) {
      expect(p.freq).toBeGreaterThanOrEqual(150)
      expect(p.freq).toBeLessThanOrEqual(1000)
    }
    expect(HARMONICS.length - 1).toBeGreaterThanOrEqual(4)
    // The harmonics are really in the sound, within 15 dB of the fundamental.
    const x = renderTone('alarm', RATE)
    const p = tonePulses('alarm')[0]
    const n = Math.round(p.dur * RATE * 0.8), s0 = Math.round((p.at + 0.015) * RATE)
    const f0 = at(x, s0, n, p.freq)
    for (let k = 2; k <= 5; k++) expect(20 * Math.log10(at(x, s0, n, p.freq * k) / f0)).toBeGreaterThan(-15)
  })
  it('start and stop without a click', () => {
    for (const t of TONES) {
      const x = renderTone(t, RATE)
      let jump = 0
      for (let i = 1; i < x.length; i++) jump = Math.max(jump, Math.abs(x[i] - x[i - 1]))
      // A click would be a jump of the order of the level itself between two samples.
      expect(jump).toBeLessThan(0.05)
    }
  })
  it('none is silent', () => expect(tonePulses('none')).toEqual([]))
})
