// Local types for the supervisor workspace. These mirror the JSON tags on the Go structs in
// internal/cad (places.go, critical.go, critreport.go, shift.go, keytests.go) and the API handler
// shapes in internal/api/final.go. They are defined here so this folder does not depend on the shared
// web/src/types.ts, which another engineer edits at the same time.

// ---- location exceptions (places.go) ----

export type Stamp = { by: string; at: string }

export type Decision = { answer: 'yes' | 'no'; note?: string; by: string; at: string }

export type Followup = {
  disposition: string
  owner?: string
  due?: string
  note?: string
  by: string
  at: string
  done?: boolean
  doneAt?: string
  doneBy?: string
}

export type ExNote = { at: string; by: string; what: string; note?: string }

export type PlaceLink = {
  how: string
  address: string
  lat?: number
  lon?: number
  addrKey?: string
  at: string
  by: string
}

export type LocException = {
  id: string
  incident: string
  type?: string
  kind: string // forced, estimated, map, caller, unverified
  entered?: string
  accepted?: string
  lat?: number
  lon?: number
  callerLat?: number
  callerLon?: number
  source?: string
  confidence?: string
  nearest?: string
  alternatives?: string[]
  reason?: string
  reasonNote?: string
  by: string
  duty?: string
  console?: string
  at: string
  period?: string
  ack?: Stamp
  located?: Decision
  followup?: Followup
  queue: string // new, review, verified, rejected, merged
  queueOwner?: string
  linked?: PlaceLink
  notes?: ExNote[]
}

// The GET /api/exceptions row wraps a LocException with live incident facts.
export type ExceptionRow = LocException & {
  open: boolean
  status: string
  units: string[] | null
  location: string
}

export type PlacePolicy = {
  kinds: string[]
  reasons: string[]
  dispositions: string[]
  unitKinds: string[]
  noteAlways: boolean
  queue: string[]
}

// ---- critical incidents (critical.go) ----

export type Trigger = {
  kind: string
  rule?: string
  name: string
  detail?: string
  source?: string
  at: string
  by?: string
}

export type CaseRole = { user: string; role: string }

export type QAMeasure = {
  id: string
  name: string
  kind: string // interval or check
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
  callTypes: string[] | null
  measures: QAMeasure[]
  policy?: string
  version?: number
}

export type QAAnswer = {
  measure: string
  result: string // met, not-met, na, unable, followup
  comment?: string
  citation?: string
  reason?: string
  by: string
  at: string
}

export type ReportVersion = {
  version: number
  cutoff: string
  by: string
  at: string
  hash: string
  note?: string
  kind?: string // full, foia, accreditation
}

export type CriticalCase = {
  id: string
  incident: string
  opened: string
  period: string
  summary: string
  triggers: Trigger[]
  designated: boolean
  changes?: ExNote[]
  ack?: Stamp
  review: string // pending, open, complete
  policyVersion: number
  template: QATemplate
  qa?: QAAnswer[]
  findings?: string
  narrative?: string
  actions?: ExNote[]
  reports?: ReportVersion[]
  roles?: CaseRole[]
}

export type PhraseAlert = {
  id: string
  rule: string
  name: string
  severity: string
  incident?: string
  call?: string
  source: string // call, radio, narrative
  channel?: string
  speaker?: string
  at: string
  confidence?: number
  found: string
  context: string
  negated?: boolean
  negation?: string
  uncertain?: boolean
  testing?: boolean
  count: number
  disposition?: string
  disposedBy?: string
  disposedAt?: string
  note?: string
}

export type CriticalFeed = {
  items: CriticalCase[] | null
  phrases: PhraseAlert[] | null
  dispositions: string[]
  results: string[]
}

// ---- the assembled full review (critreport.go) ----

export type SourceItem = { name: string; present: boolean; note?: string }

export type Milestone = { name: string; at?: string; seq?: number; source: string; note?: string }

export type UnitActivity = {
  unit: string
  recommended: boolean
  times: Record<string, string>
  seqs: Record<string, number>
}

export type RadioTag = {
  at: string
  unit?: string
  channel?: string
  tag: string
  name: string
  found: string
  context: string
  negated?: boolean
  text: string
}

export type RequestMeta = {
  id: string
  type: string
  category: string
  subject?: string
  requester?: string
  owner?: string
  requestedAt: string
  sentAt?: string
  destination?: string
  status: string
}

export type QARow = {
  measure: QAMeasure
  seconds?: number
  fromSeq?: number
  toSeq?: number
  fromAt?: string
  toAt?: string
  within?: boolean
  evidence?: string[]
  prompt?: string
  answer?: QAAnswer
}

export type ReportIncident = {
  number: string
  type: string
  priority: number
  location?: { address?: string }
}

export type CriticalReport = {
  case: CriticalCase
  incident: ReportIncident
  version: number
  cutoff: string
  generated: string
  timeZone: string
  sources: SourceItem[]
  timeline: unknown[]
  personnel: CaseRole[] | null
  milestones: Milestone[]
  units: UnitActivity[] | null
  radio: RadioTag[] | null
  radioLines?: unknown[]
  requests: RequestMeta[] | null
  qa: QARow[]
  limitations: string[] | null
}

export type ReportResponse = { report: CriticalReport; text: string }

export type DerivedView = { view: string; text: string; redactions: string[] | null; source: string }

// ---- Shift Intelligence (shift.go) ----

export type Period = { id: string; name: string; from: string; to: string }

export type ReportItem = {
  key: string
  section: string
  incident?: string
  at: string
  title: string
  significance?: string
  status: string
  owner?: string
  due?: string
  next?: string
  restricted?: boolean
}

export type ShiftAck = {
  by: string
  at: string
  version: number
  accepted?: string[]
  reassign?: Record<string, string>
  note?: string
}

export type ShiftReport = {
  period: Period
  version: number // 0: a draft
  addendum?: boolean
  items: ReportItem[] | null
  summary?: string
  handoff?: string
  note?: string
  by?: string
  at?: string
  acks?: ShiftAck[]
  custom?: boolean
  generated: string
}

export type PeriodsResponse = {
  periods: Period[]
  issued: Record<string, number>
  sections: string[]
  names: Record<string, string>
}

export type ShiftReportResponse = { draft: ShiftReport; issued: ShiftReport[] | null }

export type Span = { from: string; to?: string; why?: string }

export type ShiftNote = { person: string; period: string; text: string; kind: string; at: string; by: string }

export type PersonShift = {
  user: string
  name?: string
  duties: string[]
  consoles?: string[]
  signOns?: Stamp[]
  signOffs?: Stamp[]
  calls: number
  answerSecs?: number
  callSecs?: number
  created: number
  createSecs?: number
  dispatches: number
  statuses: number
  radio: number
  queries: number
  critical?: string[]
  breaks?: Span[]
  breakMins: number
  locks?: Span[]
  trainee?: boolean
  notes?: ShiftNote[]
  prompt?: string
}

export type PeopleResponse = { period: Period; people: PersonShift[] | null }

export type TrackedFollowup = {
  id: string
  key: string
  incident?: string
  title: string
  owner?: string
  due?: string
  opened: string
  period: string
  done?: boolean
  doneAt?: string
  history: ExNote[] | null
}

export type FollowupsResponse = { items: TrackedFollowup[] | null }

// ---- key tests (keytests.go) ----

export type KeyTestResult = {
  key: string
  action: string
  got?: string
  ok: boolean
  fix?: string
}

export type KeyTest = {
  console: string
  duty: string
  profile: string
  browser?: string
  platform?: string
  results: KeyTestResult[]
  passed: boolean
  at: string
  by: string
}

export type KeyTestsResponse = { items: KeyTest[] | null }
