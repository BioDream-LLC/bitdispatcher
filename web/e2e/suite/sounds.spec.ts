import { expect, test } from '@playwright/test'
import { signIn } from './helpers'

// The priority tones as the browser's own audio engine makes them: each Play button on Operations rules is
// rendered offline (no sound card) and measured, so what an administrator previews is checked, not assumed.
test('each priority tone sounds, is distinct, and does not clip in the real audio engine', async ({ page }) => {
  await page.addInitScript(() => {
    const made: OfflineAudioContext[] = []
    ;(window as unknown as { __made: OfflineAudioContext[] }).__made = made
    class Recorder extends OfflineAudioContext {
      constructor() {
        super(1, 48000 * 3, 48000)
        made.push(this)
      }
      resume() { return Promise.resolve() }
      close() { return Promise.resolve() }
    }
    ;(window as unknown as { AudioContext: unknown }).AudioContext = Recorder
  })
  const errors = await signIn(page, 'admin', undefined, 'admin')
  await page.goto('/#/admin/operations')
  await expect(page.getByRole('heading', { name: 'Operations rules' })).toBeVisible()
  const measured: Record<string, { peak: number; rms: number; pulses: number }> = {}
  for (const level of ['urgent', 'high', 'medium', 'low']) {
    const play = page.getByRole('button', { name: new RegExp(`^Play the .* tone for ${level}$`) })
    await play.scrollIntoViewIfNeeded()
    // A fresh offline context each time: the shared one is replaced so one tone is rendered alone.
    await page.evaluate(() => {
      const w = window as unknown as { __made: OfflineAudioContext[] }
      w.__made.length = 0
    })
    await play.click()
    measured[level] = await page.evaluate(async () => {
      const w = window as unknown as { __made: OfflineAudioContext[] }
      const c = w.__made[w.__made.length - 1]
      if (!c) return { peak: -1, rms: -1, pulses: -1 }
      const buf = await c.startRendering()
      const x = buf.getChannelData(0)
      let peak = 0, s = 0, n = 0, pulses = 0, on = false, quiet = 0
      for (let i = 0; i < x.length; i++) { const a = Math.abs(x[i]); peak = Math.max(peak, a); if (a > 1e-4) { s += a * a; n++ } }
      for (let i = 0; i < x.length; i += 48) {
        let e = 0
        for (let j = i; j < Math.min(x.length, i + 48); j++) e = Math.max(e, Math.abs(x[j]))
        if (e > 0.005) { if (!on) pulses++; on = true; quiet = 0 } else if (++quiet > 15) on = false
      }
      return { peak, rms: Math.sqrt(s / Math.max(1, n)), pulses }
    })
  }
  console.log(JSON.stringify(measured))
  expect(measured.urgent.pulses).toBe(5)
  expect(measured.high.pulses).toBe(3)
  expect(measured.medium.pulses).toBe(2)
  expect(measured.low.pulses).toBe(1)
  for (const m of Object.values(measured)) {
    expect(m.peak).toBeGreaterThan(0.02)
    expect(m.peak).toBeLessThan(0.9)
  }
  expect(measured.urgent.rms).toBeGreaterThan(measured.high.rms)
  expect(measured.high.rms).toBeGreaterThan(measured.medium.rms)
  expect(measured.medium.rms).toBeGreaterThan(measured.low.rms)
  expect(errors).toEqual([])
})
