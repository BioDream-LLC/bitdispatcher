// The API's JSON, as the Go side writes it (internal/cad, internal/api, internal/bitvoice).

export type Civic = {
  country?: string; state?: string; county?: string; city?: string; number?: string; numberSuffix?: string
  preDir?: string; street?: string; streetType?: string; postDir?: string; landmark?: string; location?: string
  name?: string; postalCode?: string; building?: string; unit?: string; floor?: string; room?: string
}

export type Location = {
  address?: string
  civic?: Civic
  lat?: number
  lon?: number
  radius?: number
  confidence?: number
  method?: string
  source?: string
  at?: string
}

export type Station = { id: string; name: string; address?: string; lat: number; lon: number }

export type UnitType = { code: string; name: string; discipline: string; capabilities: string[]; cover?: boolean }

export type Position = { lat: number; lon: number; accuracy?: number; speed?: number; heading?: number; source: string; at: string }

export type Unit = {
  id: string; name?: string; type: string; station?: string; capabilities?: string[]; radioId?: number; radioAlias?: string; agency?: string
  status: string; statusAt: string; incident?: string; position?: Position; lastHeard?: string; emergency?: string; covering?: string
  markOut?: MarkOut; checked?: string; checkedBy?: string
  caps: string[]
}

export type MarkOut = { code: string; location?: Location; note?: string }
export type MarkOutType = { code: string; name: string; minutes: number; discipline?: string; interruptible?: boolean }

export type Requirement = { capability: string; count: number }
export type CallType = { code: string; name: string; priority: number; discipline: string; runCard: Requirement[] | null }

export type Assignment = {
  unit: string; status: string; recommended: boolean; dispatched: string
  enroute?: string; onScene?: string; transport?: string; atDest?: string; cleared?: string
}

export type Entry = { seq: number; at: string; actor: string; kind: string; text: string; unit?: string; priority?: boolean }

export type Incident = {
  reports?: string[]
  number: string; type: string; priority: number; status: 'pending' | 'active' | 'closed'
  location?: Location; caller?: { name?: string; callback?: string }; calls: string[]; units: Assignment[]
  timeline: Entry[]; created: string; createdBy: string; updated: string; closed?: string; disposition?: string
}

export type Media = { id: string; url: string; purpose: string }
export type TextLine = { at: string; from: string; name?: string; via: string; text: string }

export type Call = {
  id: string; sources: string[]; state: 'ringing' | 'answered' | 'ended'; callback?: string; callerName?: string; class?: string
  provider?: string; location?: Location; locations: number; agent?: string; position?: string; start: string; answer?: string; end?: string
  media?: Media[]; video?: { live: boolean; viewUrl?: string; source?: string }; text?: TextLine[]; textOnly?: boolean; notes?: string[]
  canRebid?: boolean; problems?: string[]; ali?: string; incident?: string; updated: string
}

export type Emergency = {
  key: string; unit?: string; radioUnit?: string; radioId?: number; alias?: string; channel?: string; channelName?: string
  raised: string; ackedBy?: string; ackedAt?: string; ackedWhere?: string
}

export type Settings = {
  agency: string; incidentPrefix: string; timers: Record<string, number>; avlStaleSeconds: number; locationStaleSeconds: number
  roadFactor: number; speedKmh: number; mapTiles: string; mapAttribution: string; mapCenter: [number, number]; mapZoom: number
  dispositions: string[]; unitTypes: UnitType[]; markOuts?: MarkOutType[]; shifts?: string[]; photosRecordedOnly?: boolean; ori?: string; ncicMode?: string; ncicAddr?: string
}

export type Failure = { at: string; what: string; call?: string; error: string }

export type BitVoiceHealth = {
  configured: boolean; connected: boolean; since?: string; lastEvent?: string; lastError?: string; lastErrorAt?: string
  lastEido?: string; lastPush?: string; lastRadio?: string; lastRadioWebhook?: string; emergencyMode: boolean; emergencyModeBy?: string
  failures: Failure[]; events: number
}

export type Health = {
  bitvoice: BitVoiceHealth
  ali: { listening?: string; error?: string; last?: string; count: number }
  ledger: number
  tiles: boolean
  map?: MapHealth
  photos?: PhotoHealth
  advisory?: AdvisoryStatus
  site?: string
  sites?: { name: string; url: string; ok: boolean; last?: string; error?: string; copied: number }[]
}

export type PhotoHealth = { available: boolean; recorded: boolean; attribution: string; job?: string; jobFrac?: number }
export type PhotoSource = { id: string; name: string; url: string; maxZoom: number; resolution: string; attribution: string; terms: string; bbox: [number, number, number, number] }
export type PhotoLevel = { zoom: number; radiusM: number; tiles: number; metresPerPixel: number }
export type PhotoPlan = { lat: number; lon: number; radiusM: number; coreRadiusM: number; outerZoom: number; coreZoom: number; rate: number; sources: string[]; tiles: number; bytes: number; seconds: number; levels: PhotoLevel[]; date?: string }
export type PhotoJob = { plan: PhotoPlan; by: string; started: string; finished?: string; done: number; fetched: number; missing: number; failed: number; bytes: number; zoom: number; error?: string }
export type PhotoStats = { tiles: number; empty: number; bytes: number; byZoom: Record<string, number>; bySource: Record<string, number>; maxZoom: number }
export type PhotoInfo = { sources: PhotoSource[]; recorded?: PhotoPlan; stats?: PhotoStats; job?: PhotoJob; online: boolean }
export type MapArea = { name: string; lat: number; lon: number; radiusM: number }
export type MapHealth = { recorded: boolean; area?: MapArea; built?: string; roads: boolean; gis?: boolean; job?: string; jobFrac?: number }
export type MapStats = { features: number; byLayer: Record<string, number>; addresses: number; intersections: number; roadSegments: number; roadKm: number; nodes: number; seconds: number }
export type MapExtract = { id: string; name: string; parent?: string; url: string; bytes?: number }
export type MapJob = {
  area: MapArea; extracts: MapExtract[]; started: string; finished?: string; by: string; error?: string; stats?: MapStats; update?: boolean; changes?: number
  progress: { phase: string; detail: string; fraction: number; counts?: Record<string, number> }
}
export type MapInfo = { recorded: boolean; area?: MapArea; built?: string; source?: string; stats?: MapStats; job?: MapJob; maxZoom: number }
export type SearchResult = { label: string; detail?: string; kind: 'address' | 'intersection' | 'street' | 'place' | 'poi' | 'area'; lat: number; lon: number; exact: boolean; note?: string; source?: string; ranged?: boolean }
export type RouteView = { id: string; ok: boolean; seconds?: number; metres?: number; via?: string[]; path?: [number, number][] }

export type Verb = { words: string[]; usage: string; meaning: string }

export type View = {
  serverTime: string; seq: number; settings: Settings; stations: Station[]; units: Unit[]; callTypes: CallType[]
  incidents: Incident[]; calls: Call[]; emergencies: Emergency[]; health: Health; verbs: Verb[]; statuses: string[]
  premises?: Record<string, Premise[]>; prior?: Record<string, History[]>
}

export type Premise = { id: string; name?: string; address: string; lat: number; lon: number; hazards?: string[]; notes?: string; contacts?: string[]; expires?: string; source?: string; updated: string; by: string }
export type History = { number: string; type?: string; address?: string; lat: number; lon: number; at: string; disposition?: string; note?: string; source?: string }

export type Session = { username: string; name: string; role: 'viewer' | 'field' | 'dispatcher' | 'supervisor' | 'admin'; console: string; from: string; created: string; last: string }

export type Candidate = { unit: string; type: string; capability: string; metres?: number; seconds?: number; from: string; routed?: boolean; reasons: string[] }
export type RecLine = { capability: string; count: number; covered: string[]; units: Candidate[]; short: number; next: Candidate[] }
export type Recommendation = { incident: string; type: string; lines: RecLine[]; units: string[]; notes: string[]; estimate: string; generated: string }

export type LedgerEvent = {
  seq: number; id: string; at: string; type: string; actor: string; role?: string; session?: string; console?: string
  incident?: string; unit?: string; call?: string; parent?: string; data?: unknown; prev: string; hash: string
}

export type Notice = { seq: number; type: string; incident?: string; unit?: string; call?: string; actor: string }

export type Move = { unit: string; from: string; fromName: string; to: string; metres?: number; seconds?: number; routed?: boolean; left: number; opens?: boolean; reason: string }
export type Gap = { station: string; stationName: string; type: string; typeName: string; discipline: string; homed: string[]; since?: string; move?: Move; others: Move[] }
export type Coverage = { gaps: Gap[]; returns: { unit: string; covering: string; home: string; reason: string }[]; estimate: string; generated: string }

export type AdvisoryEvidence = { text: string; source: string; incident?: string; call?: string; seq?: number }
export type Suggestion = {
  id: string; rule: string; ruleName: string; severity: 'urgent' | 'info'; verdict: 'suggest' | 'cannot-determine'; title: string; detail: string; explain: string
  incident?: string; other?: string; call?: string; unit?: string; action?: { label: string; command: string }
  relied: AdvisoryEvidence[]; assumedAbsent: string[]; since: string
}
export type AdvisoryStatus = { mode: 'off' | 'shadow' | 'live'; version: number; lastRun?: string; error?: string; current: number; degraded: boolean; faults?: { rule: string; name: string; message: string }[] }
