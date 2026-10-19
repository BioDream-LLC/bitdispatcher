// Figures for the manual (the drawing helpers are from BitVoice, Apache-2.0, BioDream LLC): vector illustrations drawn from a few shapes so they all look alike. Each is
// self-contained SVG (its own dark panel), so it reads the same in Help, on the web page and in the PDF.
// An article shows one with a line `::figure name`.

type Tone = 'sky' | 'violet' | 'emerald' | 'amber' | 'rose' | 'teal' | 'slate' | 'fuchsia' | 'orange'

const tones: Record<Tone, { fill: string; stroke: string; text: string }> = {
  sky: { fill: '#0c2a43', stroke: '#38bdf8', text: '#bae6fd' },
  violet: { fill: '#22184a', stroke: '#a78bfa', text: '#ddd6fe' },
  emerald: { fill: '#082f26', stroke: '#34d399', text: '#a7f3d0' },
  amber: { fill: '#3a2606', stroke: '#fbbf24', text: '#fde68a' },
  rose: { fill: '#3b0f1c', stroke: '#fb7185', text: '#fecdd3' },
  teal: { fill: '#062f30', stroke: '#2dd4bf', text: '#99f6e4' },
  slate: { fill: '#162033', stroke: '#64748b', text: '#e2e8f0' },
  fuchsia: { fill: '#36103a', stroke: '#e879f9', text: '#f5d0fe' },
  orange: { fill: '#3a1a06', stroke: '#fb923c', text: '#fed7aa' },
}

const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')

/** A rounded box with a bold title and an optional line under it. */
function box(x: number, y: number, w: number, h: number, title: string, sub = '', tone: Tone = 'slate') {
  const t = tones[tone]
  const cy = sub ? y + h / 2 - 3 : y + h / 2 + 5
  return `<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="10" fill="${t.fill}" stroke="${t.stroke}" stroke-width="1.5"/>` +
    `<text x="${x + w / 2}" y="${cy}" text-anchor="middle" class="t" fill="${t.text}">${esc(title)}</text>` +
    (sub ? `<text x="${x + w / 2}" y="${cy + 17}" text-anchor="middle" class="s">${esc(sub)}</text>` : '')
}

/** A tall box with its title at the top, for lines of text under it. */
function card(x: number, y: number, w: number, h: number, title: string, tone: Tone = 'slate') {
  const c = tones[tone]
  return `<rect x="${x}" y="${y}" width="${w}" height="${h}" rx="10" fill="${c.fill}" stroke="${c.stroke}" stroke-width="1.5"/>` +
    `<text x="${x + w / 2}" y="${y + 24}" text-anchor="middle" class="t" fill="${c.text}">${esc(title)}</text>`
}

/** A line with an arrow head; dashed for "if" paths. */
function arrow(d: string, tone: Tone = 'slate', dashed = false) {
  const c = tone === 'slate' ? '#94a3b8' : tones[tone].stroke
  return `<path d="${d}" fill="none" stroke="${c}" stroke-width="1.6"${dashed ? ' stroke-dasharray="5 4"' : ''} marker-end="url(#tip-${tone})"/>`
}

/** Text placed freely: a label on an arrow, a note. */
function label(x: number, y: number, s: string, anchor: 'start' | 'middle' | 'end' = 'middle', cls = 's') {
  // A bold label needs its own colour: the boxes give theirs as an attribute, which the class would override.
  return `<text x="${x}" y="${y}" text-anchor="${anchor}" class="${cls}"${cls === 't' ? ' fill="#e2e8f0"' : ''}>${esc(s)}</text>`
}

/** A cloud (a carrier, the internet). */
function cloud(x: number, y: number, w: number, title: string, tone: Tone = 'teal') {
  const t = tones[tone]
  const h = w * 0.5
  return `<path d="M${x + w * 0.22} ${y + h} h${w * 0.6} a${w * 0.16} ${w * 0.16} 0 0 0 0 -${w * 0.32} a${w * 0.2} ${w * 0.2} 0 0 0 -${w * 0.3} -${w * 0.1} a${w * 0.22} ${w * 0.22} 0 0 0 -${w * 0.38} ${w * 0.1} a${w * 0.14} ${w * 0.14} 0 0 0 ${w * 0.08} ${w * 0.32} z" fill="${t.fill}" stroke="${t.stroke}" stroke-width="1.5"/>` +
    `<text x="${x + w * 0.5}" y="${y + h * 0.72}" text-anchor="middle" class="t" fill="${t.text}">${esc(title)}</text>`
}

/** The panel every figure sits on, with its arrow heads and fonts. */
function frame(w: number, h: number, alt: string, body: string) {
  const tips = (Object.keys(tones) as Tone[]).map((k) => {
    const c = k === 'slate' ? '#94a3b8' : tones[k].stroke
    return `<marker id="tip-${k}" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0 0L10 5L0 10z" fill="${c}"/></marker>`
  }).join('')
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${w} ${h}" role="img" aria-label="${esc(alt)}" class="figure-svg">` +
    `<defs>${tips}<linearGradient id="panel" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#0f1a2e"/><stop offset="1" stop-color="#070d1a"/></linearGradient>` +
    `<style>.t{font:600 13px system-ui,-apple-system,"Segoe UI",sans-serif}.s{font:11.5px system-ui,-apple-system,"Segoe UI",sans-serif;fill:#94a3b8}.h{font:700 12px system-ui,sans-serif;fill:#c4b5fd;letter-spacing:.06em}.m{font:12px ui-monospace,Menlo,monospace;fill:#fcd34d}</style></defs>` +
    `<rect width="${w}" height="${h}" rx="14" fill="url(#panel)" stroke="#1e293b"/>${body}</svg>`
}

export interface Figure { title: string; svg: string }

export const figures: Record<string, Figure> = {
  console: {
    title: 'The dispatcher screen: calls and incidents on the left, the map in the middle, the selected incident on the right',
    svg: frame(820, 330, 'The console layout: status bar on top, call and incident queue left, map centre, incident or situation panel right, unit board under the map, command line at the bottom.',
      `<rect x="20" y="20" width="780" height="26" rx="6" fill="#162033" stroke="#64748b"/>` + label(410, 38, 'Status bar: workload, clock, every interface\u2019s health', 'middle', 's') +
      card(20, 56, 190, 214, 'Queue', 'sky') + label(115, 106, '911 calls, ringing first') + label(115, 126, 'Incidents by priority') + label(115, 146, 'timers, NO UNITS · LATE') +
      card(220, 56, 370, 150, 'Map', 'emerald') + label(405, 106, 'units, incidents, callers, routes') + label(405, 126, 'search, right-click: what is here') + label(405, 146, 'Photos (F7), Day/Night') +
      card(220, 214, 370, 56, 'Unit board', 'amber') + label(405, 254, 'every unit, status, timer, AVL') +
      card(600, 56, 200, 214, 'Incident / Situation', 'violet') + label(700, 106, 'recommendation, units') + label(700, 126, 'timeline, caller actions') + label(700, 146, 'or: waiting, timers,') + label(700, 166, 'move-up, coverage') +
      `<rect x="20" y="280" width="780" height="30" rx="6" fill="#082f26" stroke="#34d399"/>` + label(410, 300, 'Command line: type anywhere. D E1 M2 · OS E1 · NEW SF 12 Main St · MU E4 ST1', 'middle', 'm')),
  },
  lifecycle: {
    title: 'From a 911 call to a closed incident, and what each step writes to the ledger',
    svg: frame(820, 190, 'Call arrives, incident created, units recommended and dispatched, statuses en route and on scene, close.',
      ['911 call', 'Incident', 'Recommend', 'Dispatch', 'En route', 'On scene', 'Close'].map((s, i) => {
        const x = 18 + i * 114
        const subs = ['BitVoice, ALI', 'F2 or NEW', 'run card, road', 'one click / REC', 'ER or MDT', 'OS', 'disposition']
        const tones2: Tone[] = ['sky', 'violet', 'amber', 'rose', 'teal', 'emerald', 'slate']
        return box(x, 60, 100, 56, s, subs[i], tones2[i]) + (i < 6 ? arrow(`M${x + 100} 88 H${x + 112}`) : '')
      }).join('') + label(410, 150, 'Every step is one attributed, time-stamped, hash-chained ledger event: reports, audits and replays come from it.')),
  },
  map: {
    title: 'How the recorded map is made and used',
    svg: frame(820, 220, 'Geofabrik extracts are downloaded and recorded into bitdispatcher\u2019s own store; tiles, routing and search come from the store; beyond it the online map.',
      cloud(20, 60, 170, 'Geofabrik', 'teal') + label(105, 160, 'OpenStreetMap state extracts') +
      arrow('M195 100 H255', 'teal') + label(225, 90, 'once') +
      `<rect x="260" y="50" width="220" height="120" rx="14" fill="#1a1440" stroke="#a78bfa" stroke-width="2"/>` + label(370, 80, 'Recorded map', 'middle', 't') +
      label(370, 104, 'roads, addresses, buildings') + label(370, 124, 'hydrants, places, boundaries') + label(370, 144, 'map-<time>.db, SQLite') +
      [70, 110, 150].map((y) => arrow(`M480 ${y} H545`, 'violet')).join('') +
      box(550, 46, 250, 44, 'Dispatch map tiles', 'night and day palettes', 'emerald') +
      box(550, 96, 250, 44, 'Road routing', 'drive times, routes on the map', 'amber') +
      box(550, 146, 250, 44, 'Search', 'address, A & B, place, what is here', 'sky') +
      label(410, 205, 'No internet is needed after recording. Outside the circle the online tile source is shown, muted, with the edge drawn.')),
  },
  moveup: {
    title: 'A move-up: Station 1 has no engine, so E4 covers it from Station 4',
    svg: frame(820, 200, 'Station 1 has its engine out on a call; bitdispatcher suggests the nearest engine from a station that can spare one.',
      box(40, 70, 170, 56, 'Station 1', 'engine out on a call', 'rose') +
      box(320, 70, 170, 56, 'Station 3', 'two engines: can spare one', 'emerald') +
      box(600, 70, 170, 56, 'Station 4', 'one engine: would empty', 'amber') +
      arrow('M320 98 H214', 'emerald') + label(267, 90, 'E3: 6 min', 'middle', 't') +
      arrow('M600 130 C 420 190, 260 190, 140 130', 'amber', true) + label(400, 186, 'E4: 13 min, empties Station 4: offered after') +
      label(410, 40, 'One click on Move (or MU E3 ST1); RET E3 sends it home when Station 1\u2019s engine is back')),
  },
  bitvoice: {
    title: 'Every hook BitVoice offers a CAD, and which way each runs',
    svg: frame(820, 260, 'BitVoice sends events, EIDO, ALI and radio webhooks to bitdispatcher; bitdispatcher calls BitVoice back to rebid, text, send video links and acknowledge emergencies.',
      `<rect x="40" y="40" width="220" height="180" rx="16" fill="#0c2a43" stroke="#38bdf8" stroke-width="2"/>` + label(150, 70, 'BitVoice', 'middle', 't') + label(150, 92, 'call handling, radio') +
      `<rect x="560" y="40" width="220" height="180" rx="16" fill="#1a1440" stroke="#a78bfa" stroke-width="2"/>` + label(670, 70, 'bitdispatcher', 'middle', 't') + label(670, 92, 'CAD') +
      [['Event stream (SSE)', 110], ['EIDO push and pull', 134], ['ALI spill', 158], ['Radio webhooks', 182]].map(([s, y]) => arrow(`M262 ${y} H556`, 'sky') + label(410, (y as number) - 4, s as string)).join('') +
      arrow('M556 206 H262', 'violet') + label(410, 202, 'rebid · text · video link · emergency ack · WHEP') +
      label(410, 245, 'Optional: without BitVoice, ALI and EIDO from any call-handling system work the same way.')),
  },
  photos: {
    title: 'Aerial photos are recorded in two levels of detail',
    svg: frame(820, 230, 'Full detail within a core radius, less detail out to the edge of the recorded map, online beyond.',
      `<circle cx="410" cy="120" r="95" fill="#13211b" stroke="#34d399" stroke-width="1.5"/>` +
      `<circle cx="410" cy="120" r="40" fill="#3a2606" stroke="#fbbf24" stroke-width="2"/>` +
      label(410, 116, 'zoom 19', 'middle', 't') + label(410, 134, 'rooftops') +
      label(410, 48, 'zoom 16 to the edge') + label(410, 212, 'beyond: fetched when asked, then kept') +
      box(30, 70, 200, 56, 'VGIN (Virginia)', '1 ft, 6 and 3 in in cities', 'amber') +
      box(590, 70, 200, 56, 'USGS NAIP', 'about 1 m, everywhere in the US', 'teal')),
  },
}
