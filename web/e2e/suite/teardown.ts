import { readFileSync, rmSync } from 'node:fs'
import { state } from './setup'

export default async function teardown() {
  const s = JSON.parse(readFileSync(state, 'utf8'))
  for (const pid of s.pids) {
    try { process.kill(pid) } catch { /* gone */ }
  }
  await new Promise((r) => setTimeout(r, 500))
  rmSync(s.data, { recursive: true, force: true })
}
