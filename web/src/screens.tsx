import type { Lens } from './lens'

// The screens a position can spread over its monitors. Each is the same live picture (one sign-in, one
// selection), arranged for one job.

export type ScreenKind = 'full' | 'calls' | 'units' | 'stations' | 'police' | 'map' | 'incident' | 'board'

// A layout box, in a 12 × 8 grid, for the screen's thumbnail.
type Box = { x: number; y: number; w: number; h: number; label: string; tone: 'queue' | 'map' | 'panel' | 'units' | 'cmd' | 'kpi' | 'table' | 'cards' }

export type ScreenDef = { kind: ScreenKind; route: string; title: string; hint: string; lens?: Lens; boxes: Box[] }

export const SCREENS: ScreenDef[] = [
  {
    kind: 'full', route: '/', title: 'Full console', hint: 'Everything on one monitor: queue, map, incident, units, command line.',
    boxes: [{ x: 0, y: 0, w: 3, h: 7, label: 'Queue', tone: 'queue' }, { x: 3, y: 0, w: 6, h: 5, label: 'Map', tone: 'map' }, { x: 9, y: 0, w: 3, h: 7, label: 'Incident', tone: 'panel' }, { x: 3, y: 5, w: 6, h: 2, label: 'Units', tone: 'units' }, { x: 0, y: 7, w: 12, h: 1, label: '', tone: 'cmd' }],
  },
  {
    kind: 'calls', route: '/screen/calls', title: 'Incidents and call entry', hint: '911 calls and incidents with the incident you are working, new incident (F2) and the command line.',
    boxes: [{ x: 0, y: 0, w: 5, h: 7, label: 'Calls and incidents', tone: 'queue' }, { x: 5, y: 0, w: 7, h: 7, label: 'Incident', tone: 'panel' }, { x: 0, y: 7, w: 12, h: 1, label: '', tone: 'cmd' }],
  },
  {
    kind: 'stations', route: '/screen/stations', title: 'Fire houses', hint: 'Every station with its apparatus and their status, who is covering, empty houses and move-ups.', lens: ['fire', 'ems'],
    boxes: [0, 1, 2, 3, 4, 5].map((i) => ({ x: (i % 3) * 4, y: Math.floor(i / 3) * 3.5, w: 3.8, h: 3.3, label: `ST${i + 1}`, tone: 'cards' as Box['tone'] })).concat([{ x: 0, y: 7, w: 12, h: 1, label: '', tone: 'cmd' }]),
  },
  {
    kind: 'police', route: '/screen/police', title: 'Police status and mark-outs', hint: 'Every car: what it is doing, where, how long, and when its status check is due. Mark-outs in one click.', lens: ['police'],
    boxes: [{ x: 0, y: 0, w: 12, h: 1, label: '', tone: 'kpi' }, { x: 0, y: 1, w: 7, h: 6, label: 'Units, mark-outs, checks', tone: 'table' }, { x: 7, y: 1, w: 5, h: 6, label: 'Map', tone: 'map' }, { x: 0, y: 7, w: 12, h: 1, label: '', tone: 'cmd' }],
  },
  {
    kind: 'units', route: '/screen/units', title: 'Unit status', hint: 'The whole unit board, large, by discipline.',
    boxes: [{ x: 0, y: 0, w: 12, h: 7, label: 'Units', tone: 'units' }, { x: 0, y: 7, w: 12, h: 1, label: '', tone: 'cmd' }],
  },
  {
    kind: 'map', route: '/screen/map', title: 'Map', hint: 'The map alone, following the incident selected on any screen.',
    boxes: [{ x: 0, y: 0, w: 12, h: 8, label: 'Map', tone: 'map' }],
  },
  {
    kind: 'incident', route: '/screen/incident', title: 'Incident and situation', hint: 'The selected incident in full, or what needs attention next.',
    boxes: [{ x: 0, y: 0, w: 12, h: 7, label: 'Incident', tone: 'panel' }, { x: 0, y: 7, w: 12, h: 1, label: '', tone: 'cmd' }],
  },
  {
    kind: 'board', route: '/board', title: 'Supervisor board', hint: 'At a glance: what is available, what is waiting and where the work is piling up. Read-only: safe on a wall.',
    boxes: [{ x: 0, y: 0, w: 2, h: 2, label: '', tone: 'kpi' }, { x: 2, y: 0, w: 2, h: 2, label: '', tone: 'kpi' }, { x: 4, y: 0, w: 2, h: 2, label: '', tone: 'kpi' }, { x: 6, y: 0, w: 2, h: 2, label: '', tone: 'kpi' }, { x: 8, y: 0, w: 4, h: 2, label: '', tone: 'kpi' },
      { x: 0, y: 2, w: 7, h: 6, label: 'Where the work is', tone: 'map' }, { x: 7, y: 2, w: 5, h: 3, label: 'By discipline', tone: 'units' }, { x: 7, y: 5, w: 5, h: 3, label: 'Areas', tone: 'table' }],
  },
]

export const screenDef = (k: ScreenKind) => SCREENS.find((s) => s.kind === k)!

const TONE: Record<Box['tone'], [string, string]> = {
  queue: ['#1e293b', '#38bdf8'], map: ['#0f2a2a', '#2dd4bf'], panel: ['#1e1b3a', '#a78bfa'], units: ['#2a1f12', '#fb923c'],
  cmd: ['#111827', '#64748b'], kpi: ['#2a1520', '#fb7185'], table: ['#14213d', '#818cf8'], cards: ['#2a1414', '#f87171'],
}

/** A small drawing of a screen's layout. */
export function Thumb({ kind, className }: { kind: ScreenKind; className?: string }) {
  const d = screenDef(kind)
  return (
    <svg viewBox="0 0 120 80" className={className} role="img" aria-label={`Layout of ${d.title}`}>
      <rect x="0" y="0" width="120" height="80" rx="4" fill="#020617" stroke="#334155" />
      {d.boxes.map((b, i) => {
        const [bg, fg] = TONE[b.tone]
        const x = 2 + b.x * 9.667, y = 2 + b.y * 9.5, w = b.w * 9.667 - 1.5, h = b.h * 9.5 - 1.5
        return (
          <g key={i}>
            <rect x={x} y={y} width={w} height={h} rx="1.5" fill={bg} stroke={fg} strokeOpacity="0.55" strokeWidth="0.6" />
            {b.tone === 'map' && <path d={`M${x + 2} ${y + h * 0.7} q${w * 0.25} ${-h * 0.4} ${w * 0.5} ${-h * 0.1} t${w * 0.45} ${-h * 0.25}`} stroke={fg} strokeOpacity="0.5" fill="none" strokeWidth="0.8" />}
            {b.tone === 'units' && Array.from({ length: Math.min(14, Math.floor(w / 6) * Math.max(1, Math.floor(h / 6))) }, (_, j) => {
              const cols = Math.max(1, Math.floor(w / 6))
              return <rect key={j} x={x + 1.5 + (j % cols) * 6} y={y + 1.5 + Math.floor(j / cols) * 5} width="4.6" height="3.6" rx="0.6" fill={['#34d399', '#facc15', '#fb7185', '#2dd4bf', '#34d399'][j % 5]} fillOpacity="0.7" />
            })}
            {(b.tone === 'queue' || b.tone === 'table') && Array.from({ length: Math.floor((h - 3) / 3.2) }, (_, j) => (
              <rect key={j} x={x + 1.5} y={y + 2 + j * 3.2} width={w - 3} height="2" rx="0.5" fill={fg} fillOpacity={j === 1 ? 0.6 : 0.2} />
            ))}
            {b.tone === 'cards' && [0, 1, 2].map((j) => <rect key={j} x={x + 1.5 + j * (w / 3.2)} y={y + h - 5} width={w / 3.6} height="3.4" rx="0.5" fill={['#34d399', '#fb7185', '#34d399'][j]} fillOpacity="0.7" />)}
            {b.tone === 'kpi' && (
              <>
                <rect x={x + 2} y={y + 2.5} width={Math.min(w - 4, 10)} height="5.5" rx="0.8" fill={fg} fillOpacity="0.75" />
                <rect x={x + 2} y={y + h - 4} width={w - 4} height="1.6" rx="0.6" fill={fg} fillOpacity="0.3" />
              </>
            )}
            {b.label && <text x={x + 2} y={y + 5.5} fontSize="4.2" fontWeight="700" fill="#e2e8f0" fillOpacity="0.85">{b.label}</text>}
          </g>
        )
      })}
    </svg>
  )
}
