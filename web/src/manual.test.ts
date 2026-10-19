import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { manual, parts } from './manual'
import { figures } from './figures'

// The manual must follow the code: every screen has a chapter that its Help link opens, and every
// chapter, figure and link points at something that exists.
const app = readFileSync(resolve(__dirname, 'App.tsx'), 'utf8')
const admin = readFileSync(resolve(__dirname, 'admin/Admin.tsx'), 'utf8')
const screens = [...new Set([
  ...[...app.matchAll(/<Route path="(\/[^"*:]*)"/g)].map((m) => m[1]),
  ...[...admin.matchAll(/<Route path="([a-z]+)"/g)].map((m) => '/admin/' + m[1]),
])]
const commandLine = readFileSync(resolve(__dirname, '../../internal/cad/command.go'), 'utf8')
const verbs = [...commandLine.matchAll(/\{\[\]string\{"([A-Z]+)"/g)].map((m) => m[1])

describe('the manual follows the screens', () => {
  it('finds the screens', () => expect(screens.length).toBeGreaterThan(10))
  it.each(screens)('%s has a chapter', (p) => {
    expect(manual.some((a) => a.route === p), `${p} has no article in manual.ts`).toBe(true)
  })
  it('chapters point at real screens', () => {
    for (const a of manual) if (a.route) expect(screens.includes(a.route), `${a.id} points at ${a.route}`).toBe(true)
  })
  it('every command-line verb is in the manual', () => {
    const text = manual.map((a) => a.body).join('\n')
    expect(verbs.length).toBeGreaterThan(10)
    for (const v of verbs.filter((v) => v !== 'HELP')) expect(text.includes('`' + v), `verb ${v} is not in the manual`).toBe(true)
  })
})

describe('the manual is complete', () => {
  it('ids are unique and every article is in exactly one part', () => {
    const ids = manual.map((a) => a.id)
    expect(new Set(ids).size).toBe(ids.length)
    const seen = parts.flatMap((p) => p.ids)
    expect(new Set(seen).size, 'an article is in two parts').toBe(seen.length)
    for (const a of manual) expect(seen.includes(a.id), `${a.id} is in no part`).toBe(true)
    for (const id of seen) expect(ids.includes(id), `part lists ${id}, which is no article`).toBe(true)
  })
  it('figures and links point at things that exist', () => {
    for (const a of manual) {
      for (const m of a.body.matchAll(/^::figure (\S+)/gm)) expect(figures[m[1]], `${a.id} shows figure ${m[1]}`).toBeTruthy()
      for (const m of a.body.matchAll(/\]\(#([a-z0-9-]+)\)/g)) expect(manual.some((x) => x.id === m[1]), `${a.id} links to chapter ${m[1]}`).toBe(true)
      for (const m of a.body.matchAll(/\]\(#(\/[^)?]*)/g)) expect(screens.includes(m[1]), `${a.id} links to screen ${m[1]}`).toBe(true)
    }
  })
  it('every figure is used', () => {
    const text = manual.map((a) => a.body).join('\n')
    for (const f of Object.keys(figures)) expect(text.includes('::figure ' + f), `figure ${f} is not used`).toBe(true)
  })
})
