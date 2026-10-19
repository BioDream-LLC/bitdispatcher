// The dispatch map style: drawn from bitdispatcher's own recorded map (internal/mapdata), for a console
// that is looked at for twelve hours. Night is the default: dark land, bright roads, and colour kept for
// what matters to a dispatcher (public-safety places, hydrants, jurisdiction lines, the edge of the
// recorded map). Day is the same map for a bright room.
import type { ExpressionSpecification, LayerSpecification, StyleSpecification } from 'maplibre-gl'

export type Theme = 'night' | 'day'

type Pal = {
  outside: string; land: string; wood: string; park: string; grass: string; farm: string; residential: string; commercial: string
  industrial: string; campus: string; hospital: string; airport: string; cemetery: string; military: string; parking: string; water: string
  waterLine: string; building: string; buildingLine: string; casing: string; motorway: string; trunk: string; primary: string; secondary: string
  tertiary: string; minor: string; service: string; path: string; rail: string; label: string; labelMinor: string; halo: string; place: string
  suburb: string; number: string; county: string; state: string; town: string; edge: string; waterLabel: string; parkLabel: string
}

const NIGHT: Pal = {
  outside: '#070a10', land: '#101722', wood: '#0f1f19', park: '#11251c', grass: '#12201a', farm: '#131a15', residential: '#131b27',
  commercial: '#1a1828', industrial: '#1a1a20', campus: '#1a1a2c', hospital: '#2a1420', airport: '#171c2a', cemetery: '#13211b',
  military: '#221e14', parking: '#18202c', water: '#0b2944', waterLine: '#1c4d78', building: '#1f2939', buildingLine: '#2d3a50',
  casing: '#04060a', motorway: '#f59e0b', trunk: '#f97316', primary: '#fde047', secondary: '#f1f5f9', tertiary: '#cbd5e1',
  minor: '#8b9bb3', service: '#5b6b82', path: '#4b5a70', rail: '#6b7280', label: '#f1f5f9', labelMinor: '#c4cede', halo: '#05080d',
  place: '#ffffff', suburb: '#94a3b8', number: '#8fa3bf', county: '#c084fc', state: '#e879f9', town: '#8b5cf6', edge: '#22d3ee',
  waterLabel: '#7cb7ea', parkLabel: '#6ee7a5',
}

const DAY: Pal = {
  outside: '#d4d8de', land: '#f3f1ec', wood: '#cfe3c8', park: '#c6e8c3', grass: '#dcecd2', farm: '#eef0dc', residential: '#ebe8e2',
  commercial: '#f1e4ea', industrial: '#e6e1e8', campus: '#ece6f5', hospital: '#f8dce4', airport: '#e3e5ee', cemetery: '#d3e6d2',
  military: '#efe6d2', parking: '#e4e6ea', water: '#a7cdf2', waterLine: '#7fb2e5', building: '#d9d4cc', buildingLine: '#c2bbb0',
  casing: '#8a8378', motorway: '#e8890c', trunk: '#f0a14a', primary: '#f7d55c', secondary: '#ffffff', tertiary: '#ffffff',
  minor: '#ffffff', service: '#ffffff', path: '#a39c90', rail: '#8d8d8d', label: '#1f2733', labelMinor: '#38414e', halo: '#ffffff',
  place: '#0f172a', suburb: '#55606f', number: '#6b6355', county: '#9333ea', state: '#a21caf', town: '#7c3aed', edge: '#0891b2',
  waterLabel: '#2a6aa8', parkLabel: '#2f7a46',
}

const REG = ['Noto Sans Regular']
const MED = ['Noto Sans Medium']
const ITA = ['Noto Sans Italic']

// w scales a line width with zoom: exponential, as roads look on paper maps.
const w = (stops: [number, number][]): ExpressionSpecification => ['interpolate', ['exponential', 1.6], ['zoom'], ...stops.flat()] as ExpressionSpecification

const kind = (...k: string[]): ExpressionSpecification => ['in', ['get', 'kind'], ['literal', k]]

// Road classes from minor to major, so major roads draw on top.
const ROADS: { k: string[]; casing: [number, number][]; fill: [number, number][]; color: keyof Pal; minzoom: number }[] = [
  { k: ['path'], casing: [[15, 0], [18, 0]], fill: [[15, 0.8], [18, 2.2]], color: 'path', minzoom: 15 },
  { k: ['track'], casing: [[13, 0], [18, 0]], fill: [[13, 0.8], [18, 3]], color: 'path', minzoom: 13 },
  { k: ['service', 'construction'], casing: [[14, 1.2], [18, 9]], fill: [[14, 0.6], [18, 7]], color: 'service', minzoom: 14 },
  { k: ['minor'], casing: [[11, 0.6], [13, 2.4], [16, 8], [18, 17]], fill: [[11, 0.3], [13, 1.4], [16, 6.5], [18, 14.5]], color: 'minor', minzoom: 11 },
  { k: ['tertiary', 'tertiary_link'], casing: [[10, 0.8], [13, 3.4], [16, 10], [18, 20]], fill: [[10, 0.5], [13, 2.2], [16, 8], [18, 17.5]], color: 'tertiary', minzoom: 10 },
  { k: ['secondary', 'secondary_link'], casing: [[9, 1], [13, 4.2], [16, 12], [18, 23]], fill: [[9, 0.6], [13, 2.8], [16, 10], [18, 20]], color: 'secondary', minzoom: 9 },
  { k: ['primary', 'primary_link'], casing: [[7, 1], [13, 5], [16, 13.5], [18, 26]], fill: [[7, 0.6], [13, 3.4], [16, 11], [18, 22.5]], color: 'primary', minzoom: 7 },
  { k: ['trunk', 'trunk_link'], casing: [[6, 1.2], [13, 5.6], [16, 14.5], [18, 28]], fill: [[6, 0.7], [13, 3.8], [16, 12], [18, 24]], color: 'trunk', minzoom: 6 },
  { k: ['motorway', 'motorway_link'], casing: [[5, 1.2], [13, 6], [16, 15.5], [18, 30]], fill: [[5, 0.7], [13, 4.2], [16, 13], [18, 26]], color: 'motorway', minzoom: 5 },
]

// MapLibre's style parser does not read oklch(), so the palette is plain hex.
// With photos on, the photo is the base: land, water and building fills are left out, roads become thin
// lines over the photo, and every label gets a dark halo so it reads over rooftops and trees.
const PHOTO: Pal = { ...NIGHT, halo: '#000000', minor: '#e2e8f0', service: '#cbd5e1', tertiary: '#f8fafc', secondary: '#ffffff', number: '#fde68a',
  labelMinor: '#f1f5f9', suburb: '#e2e8f0', casing: '#000000' }

export function dispatchStyle(opts: { theme: Theme; base: string; online: boolean; recorded: boolean; bounds?: [number, number, number, number]; attribution: string
  photos?: { attribution: string }; gis?: boolean }): StyleSpecification {
  const photos = !!opts.photos
  const p = photos ? PHOTO : opts.theme === 'day' ? DAY : NIGHT
  const night = photos || opts.theme === 'night'
  // Road widths over a photo: the photo shows the road itself, the line only says which one it is.
  const rw = photos ? 0.4 : 1
  const scaled = (st: [number, number][]) => st.map(([z, v]) => [z, v * rw]) as [number, number][]
  const layers: LayerSpecification[] = [{ id: 'bg', type: 'background', paint: { 'background-color': p.outside } }]
  const style: StyleSpecification = {
    version: 8,
    glyphs: opts.base + 'api/map/fonts/{fontstack}/{range}',
    sources: {},
    layers,
  }
  if (opts.online) {
    // Outside the recorded map: online tiles, fetched and cached by the server, deliberately muted so a
    // dispatcher can see at a glance where recorded data ends.
    style.sources.online = { type: 'raster', tiles: [opts.base + 'api/tiles/{z}/{x}/{y}'], tileSize: 256, maxzoom: 19, attribution: opts.attribution || '© OpenStreetMap contributors' }
    layers.push({ id: 'online', type: 'raster', source: 'online', paint: night ? { 'raster-saturation': -0.9, 'raster-brightness-max': 0.38, 'raster-contrast': 0.1 } : { 'raster-saturation': -0.7, 'raster-opacity': 0.75 } })
  }
  if (opts.photos) {
    style.sources.photos = { type: 'raster', tiles: [opts.base + 'api/photos/tiles/{z}/{x}/{y}'], tileSize: 256, maxzoom: 20, attribution: opts.photos.attribution }
    layers.push({ id: 'photos', type: 'raster', source: 'photos', paint: { 'raster-contrast': 0.08, 'raster-saturation': -0.1, 'raster-fade-duration': 0 } })
  }
  if (!opts.recorded) return style
  style.sources.bd = { type: 'vector', tiles: [opts.base + 'api/map/tiles/{z}/{x}/{y}'], minzoom: 0, maxzoom: 15, bounds: opts.bounds, attribution: '© OpenStreetMap contributors (recorded)' }
  // Layer options are checked by MapLibre when the style loads; the type here keeps the table readable.
  type VL = { id: string; type: 'fill' | 'line' | 'symbol' | 'circle'; 'source-layer': string; minzoom?: number; maxzoom?: number; filter?: unknown; layout?: Record<string, unknown>; paint?: Record<string, unknown> }
  const L = (l: VL) => layers.push({ source: 'bd', ...l } as unknown as LayerSpecification)

  // Land inside the recorded area.
  if (!photos) L({ id: 'area', type: 'fill', 'source-layer': 'area', paint: { 'fill-color': p.land } })
  const landColor: ExpressionSpecification = ['match', ['get', 'kind'],
    'wood', p.wood, 'park', p.park, 'golf', p.park, 'grass', p.grass, 'scrub', p.grass, 'wetland', p.grass, 'farm', p.farm,
    'residential', p.residential, 'commercial', p.commercial, 'industrial', p.industrial, 'school', p.campus, 'hospital', p.hospital,
    'airport', p.airport, 'apron', p.parking, 'cemetery', p.cemetery, 'military', p.military, 'parking', p.parking, 'pitch', p.park,
    'prison', p.military, 'bare', p.industrial, p.land]
  if (!photos) L({ id: 'land', type: 'fill', 'source-layer': 'land', paint: { 'fill-color': landColor, 'fill-antialias': false } })
  if (!photos) L({ id: 'land-outline', type: 'line', 'source-layer': 'land', minzoom: 13, filter: kind('hospital', 'school', 'prison', 'military'),
    paint: { 'line-color': ['match', ['get', 'kind'], 'hospital', '#f472b6', 'school', '#a78bfa', '#a3a3a3'], 'line-width': 1, 'line-opacity': 0.5, 'line-dasharray': [3, 2] } })
  if (!photos) L({ id: 'water', type: 'fill', 'source-layer': 'water', paint: { 'fill-color': p.water } })
  L({ id: 'water-edge', type: 'line', 'source-layer': 'water', minzoom: 12, paint: { 'line-color': p.waterLine, 'line-width': 0.8 } })
  L({ id: 'waterway', type: 'line', 'source-layer': 'waterway', paint: { 'line-color': p.waterLine,
    'line-width': ['interpolate', ['exponential', 1.6], ['zoom'], 8, ['match', ['get', 'kind'], 'river', 0.8, 0.2], 13, ['match', ['get', 'kind'], 'river', 2.5, 'canal', 1.6, 0.8],
      18, ['match', ['get', 'kind'], 'river', 10, 'canal', 7, 2.5]] } })
  if (!photos) L({ id: 'runway', type: 'line', 'source-layer': 'aeroway', filter: ['==', ['geometry-type'], 'LineString'],
    paint: { 'line-color': night ? '#334155' : '#b8bcc6',
      'line-width': ['interpolate', ['exponential', 1.6], ['zoom'], 10, ['match', ['get', 'kind'], 'runway', 2, 0.3], 14, ['match', ['get', 'kind'], 'runway', 18, 3], 18, ['match', ['get', 'kind'], 'runway', 120, 30]] } })
  if (!photos) L({ id: 'building', type: 'fill', 'source-layer': 'building', minzoom: 14,
    paint: { 'fill-color': ['match', ['get', 'kind'], 'hospital', night ? '#47213a' : '#f0b8c8', 'fire', night ? '#4a1d1d' : '#f3b2b2', 'school', night ? '#2c2547' : '#d9cdf2', p.building],
      'fill-outline-color': p.buildingLine, 'fill-opacity': ['interpolate', ['linear'], ['zoom'], 14, 0.5, 16, 1] } })

  // Roads: tunnels, then casings, then fills, each class minor to major; bridges again on top.
  const roadFilter = (r: (typeof ROADS)[number], extra: ExpressionSpecification): ExpressionSpecification => ['all', kind(...r.k), extra]
  const notBridgeTunnel: ExpressionSpecification = ['all', ['!', ['has', 'bridge']], ['!', ['has', 'tunnel']]]
  for (const r of ROADS) {
    L({ id: `tunnel-${r.k[0]}`, type: 'line', 'source-layer': 'road', minzoom: r.minzoom, filter: roadFilter(r, ['has', 'tunnel']),
      paint: { 'line-color': p[r.color], 'line-width': w(scaled(r.fill)), 'line-opacity': 0.45, 'line-dasharray': [1.5, 1] } })
  }
  for (const r of ROADS) {
    if (r.k[0] === 'path' || r.k[0] === 'track') continue
    L({ id: `casing-${r.k[0]}`, type: 'line', 'source-layer': 'road', minzoom: r.minzoom, filter: roadFilter(r, notBridgeTunnel),
      layout: { 'line-cap': 'round', 'line-join': 'round' }, paint: { 'line-color': p.casing, 'line-width': w(photos ? scaled(r.fill).map(([z, v]) => [z, v + 1.5]) as [number, number][] : r.casing), 'line-opacity': photos ? 0.5 : 1 } })
  }
  for (const r of ROADS) {
    const dashed = r.k[0] === 'path' || r.k[0] === 'track'
    L({ id: `road-${r.k[0]}`, type: 'line', 'source-layer': 'road', minzoom: r.minzoom, filter: roadFilter(r, notBridgeTunnel),
      layout: { 'line-cap': dashed ? 'butt' : 'round', 'line-join': 'round' },
      paint: { 'line-color': ['case', ['has', 'private'], p.service, p[r.color]], 'line-width': w(scaled(r.fill)), ...(dashed ? { 'line-dasharray': [2, 1.5] } : {}),
        'line-opacity': photos ? 0.85 : r.k[0] === 'minor' && night ? ['interpolate', ['linear'], ['zoom'], 11, 0.55, 14, 1] : 1 } })
  }
  for (const r of ROADS) {
    if (r.k[0] === 'path' || r.k[0] === 'track') continue
    L({ id: `bridge-casing-${r.k[0]}`, type: 'line', 'source-layer': 'road', minzoom: Math.max(r.minzoom, 12), filter: roadFilter(r, ['has', 'bridge']),
      paint: { 'line-color': night ? '#000000' : '#5c564d', 'line-width': w(scaled(r.casing).map(([z, v]) => [z, v + 2.5 * rw]) as [number, number][]), 'line-opacity': photos ? 0.5 : 1 } })
    L({ id: `bridge-${r.k[0]}`, type: 'line', 'source-layer': 'road', minzoom: Math.max(r.minzoom, 12), filter: roadFilter(r, ['has', 'bridge']),
      layout: { 'line-join': 'round' }, paint: { 'line-color': p[r.color], 'line-width': w(scaled(r.fill)), 'line-opacity': photos ? 0.85 : 1 } })
  }
  L({ id: 'rail', type: 'line', 'source-layer': 'rail', paint: { 'line-color': p.rail, 'line-width': w([[8, 0.6], [14, 2], [18, 4]]) } })
  if (!photos) L({ id: 'rail-ties', type: 'line', 'source-layer': 'rail', minzoom: 13, paint: { 'line-color': p.rail, 'line-width': w([[13, 4], [18, 10]]), 'line-dasharray': [0.15, 2.5] } })
  L({ id: 'oneway', type: 'symbol', 'source-layer': 'road', minzoom: 16, filter: ['has', 'oneway'],
    layout: { 'symbol-placement': 'line', 'symbol-spacing': 120, 'text-field': ['match', ['get', 'oneway'], '-1', '←', '→'], 'text-font': MED, 'text-size': 13, 'text-keep-upright': false },
    paint: { 'text-color': night ? '#0b1220' : '#5b6472', 'text-opacity': 0.8 } })

  // Jurisdiction lines: counties matter for who answers a call.
  L({ id: 'bound-town', type: 'line', 'source-layer': 'boundary', minzoom: 10, filter: ['==', ['get', 'kind'], 'town'],
    paint: { 'line-color': p.town, 'line-width': w([[10, 0.8], [16, 2]]), 'line-dasharray': [1, 2], 'line-opacity': 0.8 } })
  L({ id: 'bound-county', type: 'line', 'source-layer': 'boundary', filter: ['==', ['get', 'kind'], 'county'],
    paint: { 'line-color': p.county, 'line-width': w([[7, 1], [12, 2], [16, 3.5]]), 'line-dasharray': [4, 2, 1, 2], 'line-opacity': 0.85 } })
  L({ id: 'bound-state', type: 'line', 'source-layer': 'boundary', filter: ['==', ['get', 'kind'], 'state'],
    paint: { 'line-color': p.state, 'line-width': w([[3, 1], [10, 2.5], [16, 5]]), 'line-dasharray': [5, 2], 'line-opacity': 0.9 } })
  // Where the recorded map ends.
  L({ id: 'edge', type: 'line', 'source-layer': 'area', paint: { 'line-color': p.edge, 'line-width': 2, 'line-dasharray': [3, 2], 'line-opacity': 0.9 } })
  L({ id: 'edge-label', type: 'symbol', 'source-layer': 'area', minzoom: 8,
    layout: { 'symbol-placement': 'line', 'symbol-spacing': 600, 'text-field': 'RECORDED MAP ENDS · ONLINE MAP BEYOND', 'text-font': MED, 'text-size': 10, 'text-letter-spacing': 0.15, 'text-offset': [0, -1] },
    paint: { 'text-color': p.edge, 'text-halo-color': p.halo, 'text-halo-width': 1.5 } })

  // Labels: water and parks, then streets, house numbers, places and public-safety points last so they win.
  L({ id: 'waterway-label', type: 'symbol', 'source-layer': 'waterway', minzoom: 13, filter: kind('river', 'canal', 'stream'),
    layout: { 'symbol-placement': 'line', 'text-field': ['get', 'name'], 'text-font': ITA, 'text-size': 11, 'symbol-spacing': 400 },
    paint: { 'text-color': p.waterLabel, 'text-halo-color': p.halo, 'text-halo-width': 1.2 } })
  L({ id: 'area-label', type: 'symbol', 'source-layer': 'label',
    layout: { 'text-field': ['get', 'name'], 'text-font': ['match', ['get', 'kind'], ['lake', 'river', 'pool'], ['literal', ITA], ['literal', REG]] as unknown as ExpressionSpecification, 'text-size': ['interpolate', ['linear'], ['zoom'], 10, 10, 16, 13],
      'text-max-width': 8, 'symbol-sort-key': ['match', ['get', 'kind'], 'hospital', 0, 'school', 1, 'lake', 2, 3] },
    paint: { 'text-color': ['match', ['get', 'kind'], ['lake', 'river', 'pool'], p.waterLabel, ['park', 'wood', 'golf'], p.parkLabel, 'hospital', night ? '#f9a8d4' : '#be185d', 'school', night ? '#c4b5fd' : '#6d28d9', p.labelMinor],
      'text-halo-color': p.halo, 'text-halo-width': 1.4 } })
  L({ id: 'road-label-minor', type: 'symbol', 'source-layer': 'road', minzoom: 14, filter: kind('minor', 'service', 'tertiary', 'tertiary_link'),
    layout: { 'symbol-placement': 'line', 'text-field': ['get', 'name'], 'text-font': MED, 'text-size': ['interpolate', ['linear'], ['zoom'], 14, 10, 18, 14], 'symbol-spacing': 280, 'text-padding': 4 },
    paint: { 'text-color': p.labelMinor, 'text-halo-color': p.halo, 'text-halo-width': photos ? 2.2 : 1.6 } })
  L({ id: 'road-label-major', type: 'symbol', 'source-layer': 'road', minzoom: 11, filter: kind('primary', 'secondary', 'trunk', 'motorway', 'primary_link', 'trunk_link'),
    layout: { 'symbol-placement': 'line', 'text-field': ['coalesce', ['get', 'name'], ['get', 'ref']], 'text-font': MED,
      'text-size': ['interpolate', ['linear'], ['zoom'], 11, 10, 18, 15], 'symbol-spacing': 320, 'text-padding': 4 },
    paint: { 'text-color': p.label, 'text-halo-color': p.halo, 'text-halo-width': photos ? 2.4 : 1.8 } })
  // Route numbers in a box, like a shield: interstates and US routes stand out at low zoom.
  L({ id: 'road-ref', type: 'symbol', 'source-layer': 'road', minzoom: 8, maxzoom: 15, filter: ['all', kind('motorway', 'trunk', 'primary'), ['has', 'ref']],
    layout: { 'symbol-placement': 'line', 'symbol-spacing': 500, 'text-field': ['get', 'ref'], 'text-font': MED, 'text-size': 10, 'text-rotation-alignment': 'viewport', 'text-padding': 30 },
    paint: { 'text-color': night ? '#0b0f17' : '#ffffff', 'text-halo-color': ['match', ['get', 'kind'], 'motorway', p.motorway, 'trunk', p.trunk, '#a16207'], 'text-halo-width': 3 } })
  L({ id: 'housenum', type: 'symbol', 'source-layer': 'addr', minzoom: 17,
    layout: { 'text-field': ['get', 'name'], 'text-font': REG, 'text-size': 10, 'text-padding': 1, 'text-allow-overlap': false },
    paint: { 'text-color': p.number, 'text-halo-color': p.halo, 'text-halo-width': photos ? 1.8 : 1 } })
  L({ id: 'building-num', type: 'symbol', 'source-layer': 'building', minzoom: 17, filter: ['has', 'num'],
    layout: { 'text-field': ['get', 'num'], 'text-font': REG, 'text-size': 10, 'text-padding': 1 },
    paint: { 'text-color': p.number, 'text-halo-color': p.halo, 'text-halo-width': 1 } })

  // Hydrants: red dots from street level.
  L({ id: 'hydrant', type: 'circle', 'source-layer': 'poi', minzoom: 15, filter: ['==', ['get', 'kind'], 'hydrant'],
    paint: { 'circle-color': '#ef4444', 'circle-radius': ['interpolate', ['linear'], ['zoom'], 15, 2, 18, 5], 'circle-stroke-color': night ? '#fecaca' : '#7f1d1d', 'circle-stroke-width': 1 } })
  const safety = ['fire', 'police', 'hospital', 'ems']
  const safetyColor: ExpressionSpecification = night
    ? ['match', ['get', 'kind'], 'fire', '#f87171', 'ems', '#fb923c', 'police', '#60a5fa', 'hospital', '#f472b6', '#e2e8f0']
    : ['match', ['get', 'kind'], 'fire', '#b91c1c', 'ems', '#c2410c', 'police', '#1d4ed8', 'hospital', '#be185d', '#1f2733']
  L({ id: 'safety-dot', type: 'circle', 'source-layer': 'poi', filter: kind(...safety),
    paint: { 'circle-color': safetyColor, 'circle-radius': ['interpolate', ['linear'], ['zoom'], 10, 3, 16, 6], 'circle-stroke-color': p.halo, 'circle-stroke-width': 1.5 } })
  L({ id: 'safety-label', type: 'symbol', 'source-layer': 'poi', minzoom: 12, filter: kind(...safety),
    layout: { 'text-field': ['get', 'name'], 'text-font': MED, 'text-size': ['interpolate', ['linear'], ['zoom'], 12, 10, 17, 13], 'text-anchor': 'top', 'text-offset': [0, 0.7], 'text-max-width': 9, 'symbol-sort-key': 0 },
    paint: { 'text-color': safetyColor, 'text-halo-color': p.halo, 'text-halo-width': 1.6 } })
  L({ id: 'poi-other', type: 'symbol', 'source-layer': 'poi', minzoom: 14,
    filter: ['all', ['has', 'name'], kind('school', 'care', 'government', 'prison', 'clinic', 'civic', 'airport', 'helipad')],
    layout: { 'text-field': ['get', 'name'], 'text-font': REG, 'text-size': 10.5, 'text-max-width': 8, 'symbol-sort-key': ['match', ['get', 'kind'], 'care', 1, 'school', 1, 'airport', 1, 3] },
    paint: { 'text-color': night ? ['match', ['get', 'kind'], 'care', '#fdba74', 'school', '#c4b5fd', 'clinic', '#f9a8d4', 'helipad', '#f9a8d4', p.labelMinor]
      : ['match', ['get', 'kind'], 'care', '#c2410c', 'school', '#6d28d9', 'clinic', '#be185d', 'helipad', '#be185d', p.labelMinor], 'text-halo-color': p.halo, 'text-halo-width': 1.3 } })
  // Shops, fuel and churches only at street level, where a dispatcher uses them as landmarks.
  L({ id: 'poi-landmark', type: 'symbol', 'source-layer': 'poi', minzoom: 16.5, filter: ['all', ['has', 'name'], kind('worship', 'fuel', 'business', 'pharmacy')],
    layout: { 'text-field': ['get', 'name'], 'text-font': REG, 'text-size': 10, 'text-max-width': 8, 'symbol-sort-key': 5 },
    paint: { 'text-color': p.labelMinor, 'text-opacity': 0.85, 'text-halo-color': p.halo, 'text-halo-width': photos ? 1.8 : 1.2 } })
  L({ id: 'poi-small', type: 'symbol', 'source-layer': 'poi', minzoom: 16, filter: kind('gate', 'crossing', 'aed', 'water', 'exit'),
    layout: { 'text-field': ['match', ['get', 'kind'], 'gate', 'GATE', 'crossing', 'RR XING', 'aed', 'AED', 'water', 'WATER', 'exit', ['concat', 'EXIT ', ['coalesce', ['get', 'ref'], '']], ''], 'text-font': MED, 'text-size': 9, 'text-letter-spacing': 0.05 },
    paint: { 'text-color': ['match', ['get', 'kind'], 'aed', '#4ade80', 'water', '#38bdf8', 'crossing', '#fbbf24', '#e2e8f0'], 'text-halo-color': p.halo, 'text-halo-width': 1.5 } })
  L({ id: 'exit', type: 'symbol', 'source-layer': 'poi', minzoom: 12, maxzoom: 16, filter: ['all', ['==', ['get', 'kind'], 'exit'], ['has', 'ref']],
    layout: { 'text-field': ['concat', 'EXIT ', ['get', 'ref']], 'text-font': MED, 'text-size': 9 },
    paint: { 'text-color': night ? '#0b0f17' : '#ffffff', 'text-halo-color': '#16a34a', 'text-halo-width': 2.5 } })
  L({ id: 'place-minor', type: 'symbol', 'source-layer': 'place', maxzoom: 17, filter: kind('suburb', 'neighbourhood', 'locality', 'hamlet'),
    layout: { 'text-field': ['get', 'name'], 'text-font': MED, 'text-transform': 'uppercase', 'text-letter-spacing': 0.12, 'text-size': ['interpolate', ['linear'], ['zoom'], 11, 9, 16, 12], 'text-max-width': 7 },
    paint: { 'text-color': p.suburb, 'text-halo-color': p.halo, 'text-halo-width': 1.5 } })
  if (opts.gis) {
    // The county's own layers, over OpenStreetMap's: zones, address numbers and hydrants it keeps itself.
    style.sources.gis = { type: 'vector', tiles: [opts.base + 'api/map/gis/{z}/{x}/{y}'], minzoom: 0, maxzoom: 15, attribution: 'County GIS' }
    const G = (l: VL) => layers.push({ source: 'gis', ...l } as unknown as LayerSpecification)
    G({ id: 'gis-zone', type: 'line', 'source-layer': 'boundary', minzoom: 10, filter: ['==', ['get', 'kind'], 'zone'],
      paint: { 'line-color': '#fb923c', 'line-width': w([[10, 0.8], [16, 2.2]]), 'line-dasharray': [6, 3], 'line-opacity': 0.75 } })
    G({ id: 'gis-zone-label', type: 'symbol', 'source-layer': 'label', minzoom: 11, filter: ['==', ['get', 'kind'], 'zone'],
      layout: { 'text-field': ['get', 'name'], 'text-font': MED, 'text-size': 11, 'text-letter-spacing': 0.08, 'text-transform': 'uppercase' },
      paint: { 'text-color': '#fdba74', 'text-halo-color': p.halo, 'text-halo-width': 1.6, 'text-opacity': 0.85 } })
    G({ id: 'gis-num', type: 'symbol', 'source-layer': 'addr', minzoom: 17,
      layout: { 'text-field': ['get', 'name'], 'text-font': REG, 'text-size': 10, 'text-padding': 1 },
      paint: { 'text-color': photos ? '#fde68a' : night ? '#a5b4cb' : '#5b5346', 'text-halo-color': p.halo, 'text-halo-width': photos ? 1.8 : 1 } })
    G({ id: 'gis-hydrant', type: 'circle', 'source-layer': 'poi', minzoom: 15, filter: ['==', ['get', 'kind'], 'hydrant'],
      paint: { 'circle-color': '#ef4444', 'circle-radius': ['interpolate', ['linear'], ['zoom'], 15, 2.5, 18, 6], 'circle-stroke-color': '#ffffff', 'circle-stroke-width': 1.2 } })
    G({ id: 'gis-hydrant-id', type: 'symbol', 'source-layer': 'poi', minzoom: 18, filter: ['==', ['get', 'kind'], 'hydrant'],
      layout: { 'text-field': ['get', 'name'], 'text-font': REG, 'text-size': 9, 'text-offset': [0, 1.1], 'text-anchor': 'top' },
      paint: { 'text-color': '#fca5a5', 'text-halo-color': p.halo, 'text-halo-width': 1.2 } })
  }
  L({ id: 'place-major', type: 'symbol', 'source-layer': 'place', maxzoom: 14, filter: kind('city', 'town', 'village'),
    layout: { 'text-field': ['get', 'name'], 'text-font': MED, 'text-size': ['interpolate', ['linear'], ['zoom'], 5, ['match', ['get', 'kind'], 'city', 13, 10], 12, ['match', ['get', 'kind'], 'city', 20, 'town', 16, 13]],
      'symbol-sort-key': ['match', ['get', 'kind'], 'city', 0, 'town', 1, 2], 'text-max-width': 8 },
    paint: { 'text-color': p.place, 'text-halo-color': p.halo, 'text-halo-width': 2 } })
  return style
}
