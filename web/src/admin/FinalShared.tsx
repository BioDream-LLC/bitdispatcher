// Shared types and helpers for the final round of administration pages: critical incidents, premise and
// safety flags, call entry rules, and the PSC PIP connection. The TypeScript shapes here mirror the Go
// structs (internal/cad model.go, places.go, flags.go, persons.go, vehicles.go, requests.go, shift.go,
// critical.go and internal/phrase phrase.go). They are defined locally so these pages compile on their
// own while another engineer adds the same fields to the shared types.

import { api } from '../api'
import { store, useStore } from '../store'

// ---- the duties a rule can name ----

export const DUTIES = ['calltaker', 'police', 'fire', 'ems', 'dispatcher', 'supervisor', 'records', 'admin'] as const
export type Duty = (typeof DUTIES)[number]

// ---- phrase rules (internal/phrase/phrase.go Rule) ----

export type PhraseRule = {
  id: string
  name: string
  phrases: string[]
  negations?: string[]
  negationWindow?: number
  sources?: string[] // call, radio, text, narrative
  callTypes?: string[]
  minConfidence?: number
  severity?: string // urgent, high, medium, low
  testing?: boolean
  off?: boolean
}

export const PHRASE_SOURCES = ['call', 'radio', 'text', 'narrative'] as const
export const SEVERITIES = ['critical', 'high', 'medium', 'low'] as const
export const PHRASE_SEVERITIES = ['urgent', 'high', 'medium', 'low'] as const

// A match returned by POST /api/admin/phrases/try (internal/phrase phrase.go Match).
export type PhraseMatch = {
  ruleId?: string
  rule?: string
  name?: string
  found?: string
  context?: string
  negated?: boolean
  uncertain?: boolean
  testing?: boolean
  severity?: string
  source?: string
}

// ---- critical incidents (internal/cad/critical.go) ----

export type CriticalRule = {
  id: string
  name: string
  kind: string // calltype, modifier, priority, event, phrase
  match: string
  version?: number
  off?: boolean
}

export type QAMeasure = {
  id: string
  name: string
  kind: string // interval, check
  from?: string
  to?: string
  standardSec?: number
  scored?: boolean
  naReasons?: string[]
  specialist?: boolean
  expect?: string
}

export type QATemplate = {
  id: string
  name: string
  callTypes: string[]
  measures: QAMeasure[]
  policy?: string
  version?: number
}

export type CriticalPolicy = {
  rules?: CriticalRule[]
  phrases?: PhraseRule[]
  tags?: PhraseRule[]
  qa?: QATemplate[]
  phraseDispositions?: string[]
  version?: number
}

export const CRITICAL_KINDS = ['calltype', 'modifier', 'priority', 'event', 'phrase'] as const
// The incident timestamps a QA interval can run from or to.
export const QA_EVENTS = ['received', 'answered', 'located', 'typed', 'created', 'dispatched', 'enroute', 'onscene'] as const

// ---- premise and safety flags (internal/cad/flags.go) ----

export type FlagCategory = {
  code: string
  name: string
  scope?: string // address, person, both
  severity: string
  restricted?: boolean
  create?: string[]
  approve?: string[]
  view?: string[]
  reviewDays?: number
  expireDays?: number
  icon?: string
  fields?: string[]
  off?: boolean
}

export type FlagScope = {
  addrKey?: string
  address?: string
  lat?: number
  lon?: number
  business?: string
  complex?: string
  building?: string
  floor?: string
  unit?: string
  person?: string
  personName?: string
  verified?: boolean
}

export type FlagVersion = {
  version: number
  title: string
  warning: string
  instruction: string
  severity: string
  state: string
  expires?: string
  changed: string
  changedBy: string
  note?: string
}

export type Flag = {
  id: string
  version: number
  category: string
  title: string
  warning: string
  instruction: string
  severity: string
  source?: string
  reliability?: string
  scope: FlagScope
  details?: Record<string, string>
  state: string
  incident?: string
  creator: string
  created: string
  verifier?: string
  verified?: string
  effective?: string
  review?: string
  expires?: string
  changed: string
  changedBy: string
  note?: string
  history?: FlagVersion[]
}

export type FlagPolicy = {
  categories?: FlagCategory[]
  nearMetres?: number
}

export const FLAG_STATES = ['proposed', 'active', 'retired', 'rejected'] as const
export const FLAG_SCOPES = ['address', 'person', 'both'] as const

// ---- people's identifiers (internal/cad/persons.go) ----

export type PersonPolicy = {
  identDuties?: string[]
}

// ---- vehicles (internal/cad/vehicles.go, vehcodes.Code) ----

export type VehCode = { code: string; label: string }

export type VehiclePolicy = {
  defaultState?: string
  codes?: Record<string, VehCode[]> // by list id -> rows
}

// A controlled list as GET /api/codes returns it.
export type CodeList = { id: string; name: string; source?: string; codes: VehCode[] }
export type CodesResponse = { lists: CodeList[]; defaultState: string }

// ---- NCIC and state requests (internal/cad/requests.go) ----

export type RequestPolicy = {
  labels?: Record<string, string>
  queues?: string[]
  urgentMinutes?: number
  routineMinutes?: number
  dueMinutes?: number
  warnMinutes?: number
  signOutCheckOff?: boolean
}

// The request statuses and their plain names (internal/cad/requests.go StatusLabels).
export const REQUEST_STATUSES: { code: string; label: string }[] = [
  { code: 'new', label: 'New' },
  { code: 'assigned', label: 'Assigned' },
  { code: 'sent', label: 'Sent' },
  { code: 'awaiting-return', label: 'Awaiting return' },
  { code: 'possible-hit', label: 'Possible hit' },
  { code: 'hit-confirmation-sent', label: 'Hit confirmation sent' },
  { code: 'confirmed-valid', label: 'Confirmed valid' },
  { code: 'not-valid', label: 'Not valid or not confirmed' },
  { code: 'will-not-extradite', label: 'Will not extradite' },
  { code: 'extradition-limited', label: 'Extradition limited' },
  { code: 'pending-information', label: 'Pending further information' },
  { code: 'no-record', label: 'No record' },
  { code: 'error', label: 'Error' },
  { code: 'cancelled', label: 'Cancelled' },
  { code: 'reassigned', label: 'Reassigned' },
  { code: 'complete', label: 'Complete' },
]

// ---- operational periods and Shift Intelligence (internal/cad/shift.go) ----

export type ShiftPeriod = { name: string; start: string } // start HH:MM

export type ShiftPolicy = {
  periods?: ShiftPeriod[]
  timeZone?: string
  sections?: string[]
  majorPriority?: number
  requireSummary?: boolean
  recipients?: string[]
  workloadRatio?: number
}

// The report sections (internal/cad/shift.go ReportSections / SectionNames).
export const REPORT_SECTIONS: { code: string; label: string }[] = [
  { code: 'major', label: 'Major and open incidents' },
  { code: 'exceptions', label: 'Forced and unverified locations' },
  { code: 'critical', label: 'Critical incidents' },
  { code: 'flags', label: 'New and critical flags' },
  { code: 'comms', label: 'BOLOs and urgent messages' },
  { code: 'staffing', label: 'Units short of staff' },
  { code: 'checks', label: 'Overdue status checks' },
  { code: 'requests', label: 'Open NCIC and state requests' },
  { code: 'followups', label: 'Follow-up owed' },
]

export type ShiftPeriodInfo = { id: string; name: string; from?: string; to?: string }

// ---- unverified locations and their review (internal/cad/places.go PlacePolicy) ----

export type PlacePolicy = {
  forceDuties?: string[]
  traineesForce?: boolean
  forceReasons?: string[]
  forceNoteAlways?: boolean
  supervisorApprove?: boolean
  dispositions?: string[]
  followupDays?: number
  unitKinds?: string[]
}

// ---- the whole settings object, as these pages read and write it ----

// FinalSettings is the part of the settings object these pages touch. Everything else is carried through
// unchanged when the whole object is saved.
export type FinalSettings = {
  places?: PlacePolicy
  flags?: FlagPolicy
  persons?: PersonPolicy
  vehicles?: VehiclePolicy
  requests?: RequestPolicy
  shift?: ShiftPolicy
  critical?: CriticalPolicy
  [k: string]: unknown
}

// liveSettings reads the current settings object from the live view in the store.
export function liveSettings(): FinalSettings | null {
  const v = useStore((s) => s.view)
  return (v?.settings as FinalSettings | undefined) ?? null
}

// saveSettings writes the whole settings object with one sub-object replaced. The settings PUT is not held
// up by staged changes (the server lets it through), so a plain save is enough, the way OperationsRules
// and Settings save. It reads the freshest settings straight from the store so a save never drops a
// change made elsewhere.
export async function saveSettings(patch: Partial<FinalSettings>): Promise<void> {
  await api.put('/admin/settings', { ...latestSettings(), ...patch })
}

// latestSettings reads the settings object from the store without a hook (for use inside event handlers).
export function latestSettings(): FinalSettings {
  return (store.get().view?.settings as FinalSettings | undefined) ?? ({} as FinalSettings)
}

// ---- small helpers used across the pages ----

export const asNum = (s: string): number | undefined => (s.trim() === '' ? undefined : Number(s))
export const parseList = (s: string) => s.split(',').map((x) => x.trim()).filter(Boolean)
export const toLines = (xs?: string[]) => (xs ?? []).join('\n')
export const fromLines = (s: string) => s.split('\n').map((x) => x.trim()).filter(Boolean)

// A short random id for a new row.
export function newId(prefix: string): string {
  return prefix + Math.random().toString(36).slice(2, 8)
}

// dateInput turns an ISO time into a yyyy-mm-dd value for a date field, or '' .
export function dateInput(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  return d.toISOString().slice(0, 10)
}

// dateDay turns a yyyy-mm-dd value into an ISO time at the start of that day, or '' .
export function dayISO(v: string): string {
  if (!v) return ''
  const d = new Date(v + 'T00:00:00')
  return isNaN(d.getTime()) ? '' : d.toISOString()
}

export function shortDate(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  return d.toLocaleDateString()
}
