// Renders the macOS installer's artwork from the web interface's icon (web/public/favicon.svg):
//   bitdispatcher.icns            the apps' icon (16 to 1024 pixels)
//   background.tiff               the Installer window's picture, light appearance (1x and 2x)
//   background-dark.tiff          the same for the dark appearance
// The results are committed; run this only to change them (needs the web folder's Playwright):
//   node packaging/macos/art.mjs        (from the repository root, on a Mac)
import { execFileSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync, mkdirSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createRequire } from 'node:module'

const require = createRequire(join(process.cwd(), 'web', 'package.json'))
const { chromium } = require('@playwright/test')
const out = 'packaging/macos'
const svg = readFileSync('web/public/favicon.svg', 'utf8')
const tmp = mkdtempSync(join(tmpdir(), 'bd-art-'))
const browser = await chromium.launch()

async function shot(html, w, h, scale, file) {
  const page = await browser.newPage({ viewport: { width: w, height: h }, deviceScaleFactor: scale })
  await page.setContent(`<!doctype html><html><body style="margin:0;background:transparent">${html}</body></html>`)
  await page.screenshot({ path: file, omitBackground: true })
  await page.close()
}

// The icon: the shield on its dark tile, as in the browser tab, with the margin macOS icons keep.
const iconset = join(tmp, 'bitdispatcher.iconset')
mkdirSync(iconset)
for (const s of [16, 32, 128, 256, 512]) {
  for (const k of [1, 2]) {
    const name = k === 1 ? `icon_${s}x${s}.png` : `icon_${s}x${s}@2x.png`
    const pad = Math.round(s * 0.09)
    await shot(`<div style="width:${s}px;height:${s}px;padding:${pad}px;box-sizing:border-box">${svg.replace('<svg ', `<svg width="${s - 2 * pad}" height="${s - 2 * pad}" `)}</div>`, s, s, k, join(iconset, name))
  }
}
execFileSync('iconutil', ['-c', 'icns', iconset, '-o', join(out, 'bitdispatcher.icns')])

// The Installer picture sits under the list of steps, bottom left; the rest stays clear.
function background(dark) {
  const ink = dark ? '#e5e7eb' : '#0b0f17'
  const soft = dark ? '#9ca3af' : '#4b5563'
  const icon = svg.replace('<svg ', '<svg width="56" height="56" ')
  return `<div style="position:relative;width:620px;height:418px;font-family:-apple-system,'Helvetica Neue',sans-serif">
    <div style="position:absolute;left:22px;bottom:26px;width:150px">
      ${icon}
      <div style="margin-top:10px;font-weight:700;font-size:17px;color:${ink}">bitdispatcher</div>
      <div style="margin-top:3px;font-size:11px;line-height:1.35;color:${soft}">Open-source 911<br>computer-aided dispatch</div>
      <div style="margin-top:9px;width:40px;height:3px;border-radius:2px;background:linear-gradient(90deg,#f43f5e,#f97316)"></div>
      <div style="margin-top:9px;font-size:10px;color:${soft}">BioDream LLC · Apache-2.0</div>
    </div></div>`
}
for (const [dark, name] of [[false, 'background'], [true, 'background-dark']]) {
  const one = join(tmp, `${name}.png`)
  const two = join(tmp, `${name}@2x.png`)
  await shot(background(dark), 620, 418, 1, one)
  await shot(background(dark), 620, 418, 2, two)
  execFileSync('tiffutil', ['-cathidpicheck', one, two, '-out', join(out, `${name}.tiff`)])
}
await browser.close()
rmSync(tmp, { recursive: true, force: true })
console.log('wrote bitdispatcher.icns, background.tiff, background-dark.tiff')
