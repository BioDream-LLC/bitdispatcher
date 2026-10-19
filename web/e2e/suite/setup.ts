import { spawn, type ChildProcess } from 'node:child_process'
import { mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(fileURLToPath(new URL('.', import.meta.url)), '../../..')
export const state = join(tmpdir(), 'bd-e2e-state.json')

function waitFor(p: ChildProcess, re: RegExp, ms = 20000): Promise<RegExpMatchArray> {
  return new Promise((ok, fail) => {
    let buf = ''
    const t = setTimeout(() => fail(new Error('timed out waiting for ' + re + ' in:\n' + buf)), ms)
    const on = (d: Buffer) => {
      buf += d.toString()
      const m = buf.match(re)
      if (m) { clearTimeout(t); ok(m) }
    }
    p.stdout?.on('data', on)
    p.stderr?.on('data', on)
  })
}

export default async function setup() {
  const data = mkdtempSync(join(tmpdir(), 'bd-e2e-'))
  const bd = spawn(join(root, 'bin/bitdispatcher'), ['-data', data, '-addr', '127.0.0.1:9643', '-demo'], { stdio: ['ignore', 'pipe', 'pipe'] })
  const [, password] = await waitFor(bd, /sign in as\s+admin\s+with password\s+(\S+)/)
  await waitFor(bd, /serving/)
  const sim = spawn(join(root, 'bin/bvsim'), ['-addr', '127.0.0.1:9644', '-every', '8s', '-calls', '2', '-eido-url', 'https://127.0.0.1:9643/api/ng911/eido', '-eido-token', 'bd_eido_e2e',
    '-radio-url', 'https://127.0.0.1:9643/api/radio/events', '-radio-secret', 'bd_radio_e2e',
    '-hook-url', 'https://127.0.0.1:9643/api/hooks', '-hook-token', 'bd_hook_e2e', '-crash-every', '0'], { stdio: ['ignore', 'pipe', 'pipe'] })
  await new Promise((r) => setTimeout(r, 1500))
  writeFileSync(state, JSON.stringify({ data, password, pids: [bd.pid, sim.pid] }))
  bd.unref()
  sim.unref()
}
