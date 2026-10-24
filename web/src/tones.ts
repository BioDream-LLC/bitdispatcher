// The communication priorities' tones, made with WebAudio (no audio files), shared by the console and the
// administrator's preview so what an administrator hears is what the floor hears.
//
// Designed after the alarm-sound guidance used for medical and industrial alarms (IEC 60601-1-8): priorities
// are told apart by their rhythm, not only their pitch; higher priority means more, faster pulses; each pulse
// is a fundamental between 150 and 1000 Hz with several harmonics, so it is heard over a room and through
// a headset, and a person with some high-frequency hearing loss still hears it.
//   alarm (urgent): the high-priority burst, 3 + 2 pulses, about 1.7 s, loudest
//   alert (high):   3 pulses, the medium-priority pattern
//   chime (medium): two soft descending notes
//   tick (low):     one short, quiet pulse

export type ToneName = 'alarm' | 'alert' | 'chime' | 'tick' | 'none'

/** One pulse: when (seconds from the start), how long, its fundamental and its peak level (0..1). */
export type Pulse = { at: number; dur: number; freq: number; gain: number }

/** The harmonics of every pulse, relative to the fundamental's level. */
export const HARMONICS = [1, 0.5, 0.35, 0.25, 0.18]

export function tonePulses(tone: ToneName): Pulse[] {
  switch (tone) {
    case 'alarm': {
      // x x x _ x x : the first half of the standard's high-priority burst; the pitches C5 A4 F4 – A4 F4 rise and fall like the standard high-priority melody.
      const f = [523, 440, 349, 440, 349]
      const at = [0, 0.2, 0.4, 0.95, 1.15]
      return at.map((t, i) => ({ at: t, dur: 0.15, freq: f[i], gain: 0.3 }))
    }
    case 'alert':
      return [0, 0.25, 0.5].map((t, i) => ({ at: t, dur: 0.18, freq: [523, 440, 349][i], gain: 0.18 }))
    case 'chime':
      return [{ at: 0, dur: 0.3, freq: 659, gain: 0.08 }, { at: 0.32, dur: 0.45, freq: 523, gain: 0.07 }]
    case 'tick':
      return [{ at: 0, dur: 0.06, freq: 880, gain: 0.05 }]
    default:
      return []
  }
}

/** Seconds a tone lasts. */
export function toneLength(tone: ToneName): number {
  return tonePulses(tone).reduce((m, p) => Math.max(m, p.at + p.dur), 0)
}

// Rise and fall of each pulse (seconds): soft enough not to click, quick enough to stay a pulse.
const EDGE = 0.012

/** scheduleTone plays a tone on any audio context (a live one, or an offline one to check it). */
export function scheduleTone(c: BaseAudioContext, tone: ToneName, start: number, volume = 1) {
  for (const p of tonePulses(tone)) {
    const g = c.createGain()
    const t0 = start + p.at
    g.gain.setValueAtTime(0, t0)
    g.gain.linearRampToValueAtTime(p.gain * volume, t0 + EDGE)
    g.gain.setValueAtTime(p.gain * volume, t0 + p.dur - EDGE)
    g.gain.linearRampToValueAtTime(0, t0 + p.dur)
    g.connect(c.destination)
    HARMONICS.forEach((h, i) => {
      const o = c.createOscillator()
      const hg = c.createGain()
      o.type = 'sine'
      o.frequency.value = p.freq * (i + 1)
      hg.gain.value = h / HARMONICS.reduce((a, b) => a + b, 0)
      o.connect(hg).connect(g)
      o.start(t0)
      o.stop(t0 + p.dur + 0.01)
    })
  }
}

/** renderTone computes a tone's samples without a sound card, the same sum of harmonics WebAudio makes:
 * used by the tests that check its level, rhythm and pitch. */
export function renderTone(tone: ToneName, rate = 48000): Float32Array {
  const out = new Float32Array(Math.ceil((toneLength(tone) + 0.05) * rate))
  const norm = HARMONICS.reduce((a, b) => a + b, 0)
  for (const p of tonePulses(tone)) {
    const s0 = Math.round(p.at * rate)
    const n = Math.round(p.dur * rate)
    const edge = EDGE * rate
    for (let i = 0; i < n; i++) {
      const env = Math.min(1, i / edge, (n - i) / edge) * p.gain
      let v = 0
      HARMONICS.forEach((h, k) => { v += (h / norm) * Math.sin((2 * Math.PI * p.freq * (k + 1) * i) / rate) })
      out[s0 + i] += env * v
    }
  }
  return out
}

// ---------- playing on the console ----------

let actx: AudioContext | null = null
type WithWebkit = typeof window & { webkitAudioContext?: typeof AudioContext }

function context(): AudioContext | null {
  try {
    if (!actx) {
      const Ctx = window.AudioContext ?? (window as WithWebkit).webkitAudioContext
      if (!Ctx) return null
      actx = new Ctx()
      actx.onstatechange = () => window.dispatchEvent(new CustomEvent('bd:sound', { detail: actx?.state }))
    }
    if (actx.state === 'suspended') void actx.resume()
    return actx
  } catch {
    return null
  }
}

/** soundReady is false while the browser keeps sound off until someone touches the page. */
export function soundReady(): boolean {
  return !!actx && actx.state === 'running'
}

/** unlockSound starts sound on the first key or click, so a tone that arrives later is heard: browsers
 * keep a page silent until a person has used it. */
export function unlockSound() {
  const go = () => {
    const c = context()
    if (c && c.state === 'running') {
      window.removeEventListener('pointerdown', go, true)
      window.removeEventListener('keydown', go, true)
    }
  }
  window.addEventListener('pointerdown', go, true)
  window.addEventListener('keydown', go, true)
}

/** playTone sounds a tone once on the console. A preview plays on its own audio context, closed after. */
export function playTone(tone: ToneName | undefined, volume = 1, preview = false) {
  if (!tone || tone === 'none') return
  if (preview) {
    const Ctx = window.AudioContext ?? (window as WithWebkit).webkitAudioContext
    if (!Ctx) return
    const c = new Ctx()
    void c.resume?.()
    scheduleTone(c, tone, c.currentTime + 0.02, volume)
    window.setTimeout(() => c.close?.().catch(() => {}), (toneLength(tone) + 0.5) * 1000)
    return
  }
  const c = context()
  if (!c) return
  scheduleTone(c, tone, c.currentTime + 0.02, volume)
}
