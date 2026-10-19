import type { Incident, Unit, View } from './types'

// No imports beyond types: the store reads parseLens as it starts.
const discipline = (view: View, u: Unit) => view.settings.unitTypes.find((t) => t.code === u.type)?.discipline ?? 'other'

// A lens limits a screen to some disciplines: a fire screen, a police screen. It only hides; every screen
// still shows every emergency, and every 911 call not yet on an incident (a caller may need anyone).

export const DISCIPLINES = [
  { d: 'fire', label: 'Fire', short: 'FIRE', color: '#f87171' },
  { d: 'ems', label: 'EMS', short: 'EMS', color: '#60a5fa' },
  { d: 'police', label: 'Police', short: 'PD', color: '#a5b4fc' },
  { d: 'other', label: 'Other', short: 'OTH', color: '#94a3b8' },
] as const

export type Lens = string[] // disciplines; empty for all

export const lensName = (l: Lens) =>
  !l.length ? 'All disciplines' : l.map((d) => DISCIPLINES.find((x) => x.d === d)?.label ?? d).join(' + ')

export function parseLens(s: string | null | undefined): Lens {
  return (s ?? '').split(',').map((x) => x.trim()).filter((x) => DISCIPLINES.some((d) => d.d === x))
}

/** The disciplines an incident needs: its call type's, those of units sent to it, and those its run card asks for. */
export function incidentDisciplines(view: View, i: Incident): Set<string> {
  const out = new Set<string>()
  const ct = view.callTypes.find((c) => c.code === i.type)
  if (ct?.discipline) out.add(ct.discipline)
  for (const r of ct?.runCard ?? []) {
    for (const t of view.settings.unitTypes) if (t.capabilities.includes(r.capability)) out.add(t.discipline)
  }
  for (const a of i.units) {
    const u = view.units.find((x) => x.id === a.unit)
    if (u && !a.cleared) out.add(discipline(view, u))
  }
  if (!out.size) out.add('other')
  return out
}

/** The view as a screen with this lens sees it. */
export function applyLens(view: View, lens: Lens): View {
  if (!lens.length) return view
  const want = new Set(lens)
  const incidents = view.incidents.filter((i) => [...incidentDisciplines(view, i)].some((d) => want.has(d)))
  const keep = new Set(incidents.map((i) => i.number))
  const units = view.units.filter((u) => want.has(discipline(view, u)))
  const calls = view.calls.filter((c) => !c.incident || keep.has(c.incident))
  return { ...view, incidents, units, calls }
}
