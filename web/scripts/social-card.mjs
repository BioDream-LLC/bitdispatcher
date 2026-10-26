// Builds site/static/social-card.png, the 1200x630 picture shown when a bitdispatcher.com link is shared
// (Open Graph and X), from the logo and the console screenshot, with Chromium.
// Run with: make social-card
//
//   node scripts/social-card.mjs <repo root>
import { chromium } from '@playwright/test'
import { readFileSync } from 'fs'

const root = process.argv[2]
const logo = readFileSync(`${root}/web/public/favicon.svg`, 'utf8')
const shot = readFileSync(`${root}/docs/screenshots/console.png`).toString('base64')

const html = `<!doctype html><html><head><meta charset="utf-8"><style>
* { box-sizing: border-box; margin: 0; }
body { width: 1200px; height: 630px; overflow: hidden; background: #0f0f0d; color: #e9e4d6;
  font-family: "Helvetica Neue", Roboto, Arial, sans-serif; position: relative; }
.bar { position: absolute; left: 0; top: 0; width: 1200px; height: 10px; background: linear-gradient(90deg, #f43f5e, #f97316, #ffb000); }
.text { position: absolute; left: 64px; top: 70px; width: 560px; }
.brand { display: flex; align-items: center; gap: 18px; }
.brand svg { width: 72px; height: 72px; }
.brand span { font-size: 54px; letter-spacing: -0.02em; color: #e9e4d6; }
.brand b { color: #ffb000; }
h1 { margin-top: 44px; font-size: 58px; line-height: 1.06; letter-spacing: -0.02em; font-weight: 800; color: #fbf9f3; font-stretch: condensed; }
p { margin-top: 26px; font-size: 27px; line-height: 1.35; color: #a39d8c; }
.foot { position: absolute; left: 64px; bottom: 50px; font-size: 24px; color: #e9e4d6; letter-spacing: 0.01em; }
.foot i { font-style: normal; color: #ffb000; }
.shot { position: absolute; left: 660px; top: 92px; width: 760px; border-radius: 12px; border: 2px solid #34322c;
  box-shadow: 0 30px 80px rgba(0,0,0,.6); }
</style></head><body>
<div class="bar"></div>
<div class="text">
  <div class="brand">${logo}<span>bit<b>dispatcher</b></span></div>
  <h1>The 911 CAD built for NG9-1-1</h1>
  <p>Free and open source. One program on your own server, used from the browser.</p>
</div>
<div class="foot">bitdispatcher.com <i>·</i> Apache-2.0 <i>·</i> BioDream LLC</div>
<img class="shot" src="data:image/png;base64,${shot}">
</body></html>`

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1200, height: 630 } })
await page.setContent(html, { waitUntil: 'load' })
await page.screenshot({ path: `${root}/site/static/social-card.png`, type: 'png' })
await browser.close()
