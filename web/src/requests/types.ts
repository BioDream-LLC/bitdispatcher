// The NCIC and state request queue, inquiry returns, hit confirmations, tows and the vehicle/person code
// lists, as the Go side writes them (internal/cad/requests.go, vehicles.go, tow.go, persons.go, and
// internal/vehcodes). These are defined here, locally, because the shared types.ts is edited by others.

// ---- code lists (internal/vehcodes/codes.go, vin.go) ----
export type Code = { code: string; label: string }
export type CodeList = { id: string; name: string; source: string; codes: Code[] }
export type VINCheck = {
  vin: string
  complete: boolean
  valid: boolean
  checkDigit: boolean
  year?: string
  wmi?: string
  problems?: string[]
}
export type Codes = {
  lists: CodeList[]
  defaultState: string
  relations: string[]
  vehicleRoles: string[]
  personRoles: string[]
  towSteps: string[]
}

// ---- people (internal/cad/persons.go) ----
export type PersonPhone = { number: string; type?: string; ext?: string; verified?: boolean; source?: string }
export type SafetyConcern = {
  title: string
  reason: string
  weapons?: string
  instruction: string
  source: string
  reliability?: string
  scope: string // incident | permanent
  flag?: string
}
export type VehicleLink = { person: string; relation: string }

// Person as entered on an incident (a superset of the shared Person; the new fields are all optional so a
// value posted to POST/PUT /api/incidents/{n}/persons matches the Go struct).
export type Person = {
  id: string
  role: string
  roles?: string[]
  name?: string
  first?: string
  middle?: string
  last?: string
  suffix?: string
  locKind?: string // incident | other | unknown | transient
  building?: string
  floor?: string
  unit?: string
  phones?: PersonPhone[]
  ethnicity?: string
  smt?: string
  build?: string
  ssn?: string // shown masked once saved
  dl?: string
  dlState?: string
  approxAge?: string
  safetyNote?: SafetyConcern
  returns?: InqReturn[]
  aliases?: string[]
  dob?: string
  sex?: string
  race?: string
  height?: string
  weight?: string
  hair?: string
  eyes?: string
  clothing?: string
  address?: string
  phone?: string
  idType?: string
  idNumber?: string
  idState?: string
  notes?: string
  safety?: boolean
  added?: string
  addedBy?: string
  updated?: string
  removed?: boolean
}

export type Vehicle = {
  id: string
  role: string
  roles?: string[]
  plate?: string
  state?: string
  vin?: string
  year?: string
  make?: string
  model?: string
  color?: string
  body?: string
  description?: string
  plateType?: string
  plateCondition?: string
  vinNote?: string
  damage?: string
  links?: VehicleLink[]
  returns?: InqReturn[]
  driver?: string
  owner?: string
  contact?: string
  direction?: string
  notes?: string
  added?: string
  addedBy?: string
  updated?: string
  removed?: boolean
}

// ---- person search and matches ----
export type PersonQuery = {
  first?: string
  middle?: string
  last?: string
  alias?: string
  dob?: string
  age?: number
  phone?: string
  address?: string
  ssn?: string
  dl?: string
  sex?: string
  race?: string
  plate?: string
  phonetic?: boolean
  partial?: boolean
}
export type PersonRef = { incident: string; person: string; role: string; at: string }
export type PersonHit = {
  key: string
  name: string
  first?: string
  middle?: string
  last?: string
  dob?: string
  approxAge?: string
  sex?: string
  race?: string
  address?: string
  phone?: string
  ssn?: string // masked
  dl?: string // masked
  aliases?: string[]
  incidents: PersonRef[]
  flags?: string[]
  why: string[]
}
export type PersonIdent = { incident: string; person: string; ssn?: string; dl?: string; dlState?: string }

// ---- inquiry returns (internal/cad/requests.go) ----
export type InqReturn = {
  id: string
  request?: string
  source: string // dmv | vcic | ncic | nlets | other-state
  kind?: string // vehicle | person
  at?: string
  by?: string
  requester?: string
  purpose?: string
  fields?: Record<string, string>
  hit?: string // stolen, wanted, warrant, missing...
  hitDetail?: string
  hitOri?: string
  confirmed?: string // the hit confirmation outcome, once answered
  superseded?: boolean
  simulated?: boolean
  text?: string // only present on the protected record
}

// ---- requests (internal/cad/requests.go) ----
export type ExNote = { at: string; by: string; what: string; note?: string }
export type Handoff = { from: string; to: string; at: string; note?: string }
export type Request = {
  id: string
  type: string // dmv | vcic | ncic | nlets | other-state | hit-confirmation
  category: string // person | vehicle | plate | vin | article | gun | boat | securities | other
  incident?: string
  record?: string // P1, V2
  subject?: string
  requester?: string
  requestedAt: string
  owner: string
  sentBy?: string
  sentAt?: string
  destination?: string // ORI
  method?: string // message switch | telephone | teletype
  priority?: string // urgent | routine
  due?: string
  receivedBy?: string
  responseAt?: string
  responder?: string
  status: string
  disposition?: string
  of?: string
  return?: string
  attempts?: ExNote[]
  notes?: ExNote[]
  handoff?: Handoff
  chain: ExNote[]
  overdue?: boolean
  updated: string
}
export type RequestUpdate = {
  status?: string
  disposition?: string
  destination?: string
  method?: string
  sent?: boolean
  received?: boolean
  responder?: string
  attempt?: string
  note?: string
  due?: string
}
export type RequestList = {
  items: Request[]
  labels: Record<string, string>
  statuses: string[]
  outcomes: string[]
  types: string[]
  categories: string[]
  queues: string[]
  warnMinutes: number
  now: string
}

// ---- tows (internal/cad/tow.go, vehicles.go) ----
export type TowCompany = {
  id: string
  name: string
  phone: string
  address?: string
  classes: string[]
  zones?: string[]
  off?: boolean
  note?: string
}
export type Tow = {
  id: string
  at: string
  by: string
  incident?: string
  company: string
  name: string
  phone: string
  class: string
  zone?: string
  kind: string // rotation | owner | police
  skipped?: string[]
  vehicle?: string
  result: string // called | accepted | declined | no-answer | cancelled | arrived
  resultAt?: string
  note?: string
  requester?: string
  reason?: string
  requestNo?: string
  override?: string
  times?: Record<string, string>
  driver?: string
  truck?: string
  destination?: string
  impound?: string
  hold?: string
  release?: string
}
export type TowTurn = { company: TowCompany; lastTurn?: string; why?: string }
export type TowAsk = {
  incident: string
  vehicle: string
  requester?: string
  reason?: string
  requestNo?: string
  class: string
  zone?: string
  kind?: string
  company?: string
  note?: string
  override?: string
}
export type TowDetail = {
  id: string
  step?: string
  at?: string
  driver?: string
  truck?: string
  destination?: string
  impound?: string
  hold?: string
  release?: string
  requestNo?: string
  note?: string
}
export type IncidentTows = {
  byVehicle: Record<string, Tow[]>
  vehicles: Vehicle[]
  rotation: TowTurn[] // next first
  classes: string[]
  zones: string[]
}

// ---- plain-English constants ----
export const STATUS_LABELS: Record<string, string> = {
  new: 'New',
  assigned: 'Assigned',
  sent: 'Sent',
  'awaiting-return': 'Awaiting return',
  'possible-hit': 'Possible hit',
  'hit-confirmation-sent': 'Hit confirmation sent',
  'confirmed-valid': 'Confirmed valid',
  'not-valid': 'Not valid or not confirmed',
  'will-not-extradite': 'Will not extradite',
  'extradition-limited': 'Extradition limited',
  'pending-information': 'Pending further information',
  'no-record': 'No record',
  error: 'Error',
  cancelled: 'Cancelled',
  reassigned: 'Reassigned',
  complete: 'Complete',
}

export const OUTCOME_LABELS: Record<string, string> = {
  'confirmed-valid': 'Confirmed valid',
  'not-valid': 'Not valid or not confirmed',
  'will-not-extradite': 'Will not extradite',
  'extradition-limited': 'Extradition limited',
  cancelled: 'Cancelled',
  'pending-information': 'Pending further information',
}

export const TYPE_LABELS: Record<string, string> = {
  dmv: 'DMV',
  vcic: 'VCIC',
  ncic: 'NCIC',
  nlets: 'Nlets',
  'other-state': 'Other state',
  'hit-confirmation': 'Hit confirmation',
}

export const CATEGORY_LABELS: Record<string, string> = {
  person: 'Person',
  vehicle: 'Vehicle',
  plate: 'Plate',
  vin: 'VIN',
  article: 'Article',
  gun: 'Gun',
  boat: 'Boat',
  securities: 'Securities',
  other: 'Other',
}

/** A request is done (needs no more work) when its status is one of these. */
export function requestClosed(status: string): boolean {
  return ['confirmed-valid', 'not-valid', 'will-not-extradite', 'extradition-limited', 'no-record', 'cancelled', 'complete'].includes(status)
}

/** dueState says, in words, whether a request is overdue or nearly due. */
export function dueState(r: Request, nowMs: number, warnMinutes: number): '' | 'nearly-due' | 'overdue' {
  if (!r.due || requestClosed(r.status)) return ''
  const due = Date.parse(r.due)
  if (isNaN(due)) return ''
  if (nowMs > due) return 'overdue'
  if (nowMs > due - warnMinutes * 60_000) return 'nearly-due'
  return ''
}
