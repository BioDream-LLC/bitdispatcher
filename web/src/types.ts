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
  altitude?: number
  vRadius?: number
}

export type Station = { id: string; name: string; address?: string; lat: number; lon: number }

export type UnitType = { code: string; name: string; discipline: string; capabilities: string[]; cover?: boolean; minStaff?: number }

export type Position = { lat: number; lon: number; accuracy?: number; speed?: number; heading?: number; source: string; at: string }

export type Unit = {
  id: string; name?: string; type: string; station?: string; capabilities?: string[]; radioId?: number; radioAlias?: string; agency?: string
  status: string; statusAt: string; incident?: string; position?: Position; lastHeard?: string; emergency?: string; covering?: string
  markOut?: MarkOut; checked?: string; checkedBy?: string; area?: string; minStaff?: number; crew?: CrewSeat[]
  caps: string[]
}

export type MarkOut = { code: string; location?: Location; note?: string }
export type MarkOutType = { code: string; name: string; minutes: number; discipline?: string; interruptible?: boolean }

export type Requirement = { capability: string; count: number }
export type CallType = { code: string; name: string; priority: number; discipline: string; runCard: Requirement[] | null; critical?: boolean; drone?: boolean; autoDispatch?: boolean; notify?: string[]; group?: string
  alarms?: AlarmLevel[]; par?: boolean; parMinutes?: number; nemsis?: string }
export type AlarmLevel = { name: string; runCard: Requirement[] }

export type Assignment = {
  unit: string; status: string; recommended: boolean; dispatched: string
  enroute?: string; onScene?: string; transport?: string; atDest?: string; cleared?: string; acked?: string; ackedBy?: string
}

export type Entry = { seq: number; at: string; actor: string; kind: string; text: string; unit?: string; priority?: boolean }

export type Incident = { files?: IncidentFile[]; protocol?: ProtocolRun; neris?: { id?: string; at: string; ok: boolean; status: number; error?: string; by: string };
  reports?: string[]
  number: string; type: string; priority: number; status: 'pending' | 'active' | 'closed'
  location?: Location; caller?: { name?: string; callback?: string }; calls: string[]; units: Assignment[]
  timeline: Entry[]; created: string; createdBy: string; updated: string; closed?: string; disposition?: string
  modifiers?: string[]; persons?: Person[]; vehicles?: Vehicle[]; callerAt?: Location; notices?: Contact[]; talkgroup?: string; locVerified?: boolean
  newLocation?: { location: Location; call?: string; metres: number; at: string }; guidance?: Shown[]
  changed?: { from?: Location; to: Location; at: string; by: string; why?: string }
  alarm?: number; fireground?: Fireground; epcr?: Record<string, EPCRRecord>
}

export type Person = {
  id: string; role: string; name?: string; aliases?: string[]; dob?: string; sex?: string; race?: string; height?: string; weight?: string; hair?: string; eyes?: string
  clothing?: string; address?: string; phone?: string; idType?: string; idNumber?: string; idState?: string; notes?: string; safety?: boolean
  added?: string; addedBy?: string; updated?: string; removed?: boolean
}
export type Vehicle = {
  id: string; role: string; plate?: string; state?: string; vin?: string; year?: string; make?: string; model?: string; color?: string; body?: string; description?: string
  driver?: string; owner?: string; contact?: string; direction?: string; notes?: string; added?: string; addedBy?: string; updated?: string; removed?: boolean
}
export type Contact = { id: string; kind: 'notify' | 'mutualaid' | 'resource' | 'transfer' | 'referral' | 'outbound' | 'instructions'; who: string; detail?: string; status: 'pending' | 'done' | 'cancelled'; at: string; by: string; done?: string; doneBy?: string; auto?: boolean }
export type Shown = { doc: string; version: string; anchor: string; heading: string; at?: string; by?: string }
export type Modifier = { code: string; name: string; priority?: number; disciplines?: string[]; notify?: string[]; safety?: boolean }
export type BOLO = { id: string; kind: string; text: string; plate?: string; vehicle?: string; person?: string; incident?: string; disciplines?: string[]; issued: string; by: string; expires?: string; cancelled?: string; cancelledBy?: string; updates?: Entry[] }
export type Facility = { id: string; name: string; kind?: string; lat?: number; lon?: number; phone?: string; status: 'open' | 'diversion' | 'closed'; note?: string; until?: string; updated?: string; by?: string }
export type Passdown = { id: string; text: string; for?: string; at: string; by: string; until?: string }
export type ResponsePlan = { id: string; name: string; callTypes?: string[]; modifiers?: string[]; area?: string; addresses?: string[]; from?: string; to?: string; order: string[]; note?: string; updated?: string }
export type EntryView = { modifiers: Modifier[]; bolos: BOLO[]; facilities: Facility[]; passdown: Passdown[]; guidance: number }
export type Passage = {
  doc: string; title: string; kind: 'policy' | 'guide' | 'prompt' | 'checklist'; version: string; effective: string; source?: string
  heading: string; path: string[]; anchor: string; text: string; why: string[]; score: number; status: 'current' | 'outdated'; conflict?: string
}
export type GuidanceResult = { matches: Passage[]; noMatch: boolean; note?: string }
export type GuidanceDoc = {
  id: string; title: string; kind: string; version: string; effective: string; expires?: string; supersedes?: string; source?: string
  approved: boolean; approvedBy?: string; approvedAt?: string; text: string; callTypes?: string[]; modifiers?: string[]; words?: string[]; always?: boolean
}
export type PrevMatch = { incident: string; type: string; typeName: string; at: string; address?: string; status?: string; disposition?: string; by: string[]; why: string[]; confidence: 'confirmed' | 'possible'; source: string; safety?: boolean }
export type Hit = { kind: string; ref: string; title: string; detail?: string; at?: string; match: 'exact' | 'partial'; source: string }
export type Dup = { incident: string; type: string; typeName: string; address?: string; created: string; metres: number; why: string[]; units: string[] }
export type ReviewDoc = { incident: string; view: string; title: string; generated: string; sections: { heading: string; lines: string[] }[]; chain?: { events: number; ok: boolean; problem?: string; lastHash?: string; checked: string } }
export type FOIARule = { id: string; name: string; jurisdiction?: string; recordTypes?: string[]; kind: 'pattern' | 'words' | 'category'; pattern?: string; words?: string[]; category?: string; basis: string; explain: string; from?: string; to?: string; enabled: boolean; sample?: boolean }
export type FOIAProposal = { id: string; start: number; end: number; text: string; rule: string; basis: string; explain: string; confidence: 'high' | 'review'; source: string }
export type FOIADecision = { id: string; start: number; end: number; text: string; accept: boolean; basis: string; note?: string; by?: string; at?: string }
export type FOIAItem = { incident: string; text: string; proposals: FOIAProposal[]; decisions: FOIADecision[]; released?: string; hash?: string; assembled: string }
export type FOIARequest = {
  id: string; requester: string; contact?: string; received: string; due: string; description?: string; incidents: string[]; view: 'summary' | 'detailed'
  status: 'received' | 'in-review' | 'awaiting-approval' | 'released' | 'denied' | 'closed'; assigned?: string; items?: FOIAItem[] | null; notes?: Entry[]
  released?: string; approvedBy?: string; started?: string; updated: string
}
export type Workstation = { id: string; name?: string; duties?: string[]; enrolled?: { hash: string; at: string; by: string; label?: string }[] }
export type Need = { duties?: string[]; last?: string; code?: boolean; enrol?: { secret: string; uri: string }; unit?: boolean }

export type Media = { id: string; url: string; purpose: string }
export type TextLine = { at: string; from: string; name?: string; via: string; text: string }

export type Call = { handling?: Handling;
  id: string; sources: string[]; system?: string; state: 'ringing' | 'answered' | 'data' | 'ended'; callback?: string; callerName?: string; class?: string
  provider?: string; location?: Location; locations: number; agent?: string; position?: string; start: string; answer?: string; end?: string
  media?: Media[]; video?: { live: boolean; viewUrl?: string; source?: string }; text?: TextLine[]; textOnly?: boolean; notes?: string[]
  canRebid?: boolean; problems?: string[]; ali?: string; incident?: string; updated: string
  transcript?: TranscriptLine[]; lang?: string; details?: Detail[]; line?: string; crash?: Crash; crashRisk?: CrashRisk; alert?: Alert; fixes?: Fix[]; screen?: Screen
}

export type TranscriptLine = { at: string; speaker: 'caller' | 'taker' | 'other'; text: string; lang?: string; english?: string; by?: string; gist?: boolean }
export type Detail = { kind: string; text: string; quote?: string; urgent?: boolean }
export type Fix = { lat: number; lon: number; radius?: number; altitude?: number; method?: string; at: string }
export type Screen = { nonEmergency: boolean; reason?: string; by?: string }
export type Crash = {
  provider?: string; at?: string; lat?: number; lon?: number; heading?: number; deltaVKmh?: number; direction?: string; rollover?: boolean; multipleImpacts?: boolean
  airbags?: string[]; occupants?: number; belted?: boolean; speedKmh?: number; fuel?: string; voiceConnected?: boolean; callback?: string
  vehicle: { year?: number; make?: string; model?: string; color?: string; vin?: string; plate?: string }
}
export type CrashRisk = { risk: 'high' | 'medium' | 'low' | 'unknown'; reasons: string[]; suggest: string[]; summary: string }
export type Drone = { id: string; name?: string; status: string; lat?: number; lon?: number; altM?: number; battery?: number; incident?: string; videoUrl?: string; pilot?: string; updated: string }
export type RadioLine = { at: string; unit?: string; radioUnit?: string; channel?: string; channelName?: string; text: string; incident?: string }
export type CareNote = { at: string; by: string; action: string; user?: string; note?: string }
export type CareCase = { incident: string; reason: string; at: string; people: string[]; takers?: string[]; status: 'open' | 'debrief' | 'closed'; offered?: string[]; actions?: CareNote[] }
export type Break = { on: boolean; since?: string; lastEnd?: string; count: number }
export type TDoS = { active: boolean; since?: string; last?: string; reasons: string[] | null; perMin: number; baseline: number; repeats?: string[]; ackedBy?: string; ackedAt?: string }
export type TrainingView = { scenario?: string; name?: string; started?: string; steps: number; played: number }
export type Phrase = { key: string; en: string; es: string }
export type Workload = {
  username: string; name: string; role: string; console: string; signedIn: string; calls: number; incidents: number; actions: number
  critical: string[]; onBreak: boolean; sinceBreak: number; level: 'ok' | 'watch' | 'relieve'; reasons: string[]
}
export type Check = { name: string; ok: boolean; skip?: boolean; detail: string }
export type Review = { incident: string; type: string; priority: number; created: string; taker?: string; score: number; checks: Check[]; critical?: string }

export type Emergency = {
  key: string; unit?: string; radioUnit?: string; radioId?: number; alias?: string; channel?: string; channelName?: string
  raised: string; ackedBy?: string; ackedAt?: string; ackedWhere?: string
}

export type Settings = {
  agency: string; incidentPrefix: string; timers: Record<string, number>; avlStaleSeconds: number; autoDispatchSeconds?: number; signInNotice?: string; locationStaleSeconds: number
  roadFactor: number; speedKmh: number; mapTiles: string; mapAttribution: string; mapCenter: [number, number]; mapZoom: number
  dispositions: string[]; unitTypes: UnitType[]; markOuts?: MarkOutType[]; shifts?: string[]; photosRecordedOnly?: boolean; ori?: string; ncicMode?: string; ncicAddr?: string
  nonEmergencyLines?: string[]; nonEmergencyWords?: string[]; nonEmergencyReply?: string; criticalWords?: string[]; careOff?: boolean; supportLine?: string
  breakMinutes?: number; crashHighKmh?: number; crashMediumKmh?: number; tdosPerMinute?: number; clusterMeters?: number; clusterMinutes?: number
  qa?: { answerSeconds?: number; entrySeconds?: number; dispatchSeconds?: number; questions?: Record<string, string[]> }
  modifiers?: Modifier[]; duties?: { callTakersDispatch?: boolean; crossDiscipline?: boolean; selfAttach?: boolean }
  security?: { mfa?: '' | 'off' | 'admins' | 'everyone'; lockMinutes?: number; stagedChanges?: boolean }
  ackSeconds?: number; overrideReasonOff?: boolean; talkgroups?: string[]; moveMeters?: number; foiaDays?: number
  qualifications?: string[]; capNeeds?: CapNeed[] | null; parMinutes?: number; benchmarks?: Benchmark[]; towClasses?: string[]; towZones?: string[]
}

export type Failure = { at: string; what: string; call?: string; error: string }

export type BitVoiceHealth = {
  configured: boolean; connected: boolean; since?: string; lastEvent?: string; lastError?: string; lastErrorAt?: string
  lastEido?: string; lastPush?: string; lastRadio?: string; lastRadioWebhook?: string; emergencyMode: boolean; emergencyModeBy?: string
  psaps?: { uri: string; name: string; queue?: string; length: number; max: number; service?: string; reason?: string }[]
  failures: Failure[]; events: number
}

export type Health = {
  bitvoice: BitVoiceHealth
  // The other BitVoice systems followed besides the main one.
  moreBitvoice?: { id: string; name: string; url: string; health: BitVoiceHealth }[]
  ali: { listening?: string; error?: string; last?: string; count: number }
  ledger: number
  tiles: boolean
  map?: MapHealth
  photos?: PhotoHealth
  advisory?: AdvisoryStatus
  site?: string
  sites?: { name: string; url: string; ok: boolean; last?: string; error?: string; copied: number }[]
  drone?: boolean; hooks?: boolean; model?: boolean; neris?: boolean; lis?: string
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

export type View = { placeViews?: boolean; messages?: UnitMessage[]; stationAlerts?: StationAlert[];
  serverTime: string; seq: number; settings: Settings; stations: Station[]; units: Unit[]; callTypes: CallType[]
  incidents: Incident[]; calls: Call[]; emergencies: Emergency[]; health: Health; verbs: Verb[]; statuses: string[]
  premises?: Record<string, Premise[]>; prior?: Record<string, History[]>
  summaries?: Record<string, string>; drones?: Drone[]; radio?: RadioLine[]; care?: CareCase[]; breaks?: Record<string, Break>; tdos?: TDoS; training?: TrainingView
  entry?: EntryView
  roadmap?: RoadmapView
}

export type Premise = { id: string; name?: string; address: string; lat: number; lon: number; hazards?: string[]; notes?: string; contacts?: string[]; expires?: string; source?: string; photos?: PremisePhoto[]; updated: string; by: string }
export type PremisePhoto = { id: string; caption?: string; facing?: '' | 'N' | 'E' | 'S' | 'W'; type: string; sha256: string; size: number; added: string; by: string }
/** PlaceView is one picture of a place from a provider: an oblique aerial photo, a straight-down one, or a street photo. */
export type PlaceView = { ref: string; provider: string; kind: 'oblique' | 'ortho' | 'street'; facing?: string; azimuth?: number; distance: number; captured?: string; attribution: string; license?: string }
export type PlaceViews = { views: PlaceView[]; problems?: Record<string, string>; google?: string }
export type ViewsConfig = { eagleviewOn: boolean; eagleviewClientId: string; eagleviewSecretSet: boolean; eagleviewSandbox: boolean; panoramaxOn: boolean; panoramaxUrl: string; mapillaryOn: boolean; mapillaryTokenSet: boolean; googleLink: boolean }
export type History = { number: string; type?: string; address?: string; lat: number; lon: number; at: string; disposition?: string; note?: string; source?: string }

export type Duty = 'calltaker' | 'police' | 'fire' | 'ems' | 'dispatcher' | 'supervisor' | 'admin' | 'records' | 'field' | 'viewer'
export type Session = {
  username: string; name: string; role: 'viewer' | 'field' | 'dispatcher' | 'supervisor' | 'admin'; console: string; from: string; created: string; last: string
  training?: boolean; exercise?: string; duty: Duty; locked?: boolean; lockedAt?: string; active?: string; workstation?: string
}

export type Candidate = { unit: string; type: string; capability: string; metres?: number; seconds?: number; from: string; routed?: boolean; reasons: string[] }
export type RecLine = { capability: string; count: number; covered: string[]; units: Candidate[]; short: number; next: Candidate[] }
export type PlanUnit = { unit: string; due: number; status: string; can: boolean; why?: string }
export type Recommendation = {
  incident: string; type: string; lines: RecLine[]; units: string[]; notes: string[]; estimate: string; generated: string
  plan?: string; planId?: string; planned?: PlanUnit[]; warnings: string[]
}

export type LedgerEvent = {
  seq: number; id: string; at: string; type: string; actor: string; role?: string; session?: string; console?: string; duty?: string
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

/** UnitMessage is a message between the floor and the crews' terminals (or between units). */
export type UnitMessage = { id: string; at: string; from: string; fromUnit: boolean; to: string[]; incident?: string; text: string; urgent?: boolean; read?: Record<string, string> }

/** Alert is what an outside system reported: an alarm, a plate-reader hit, a gunshot detection, a camera. */
export type Alert = { kind: 'alarm' | 'lpr' | 'gunshot' | 'camera' | 'cad' | 'other'; sender: string; id: string; event: string; detail?: string; status?: string
  account?: string; premise?: string; zone?: string; permit?: string; verified?: boolean; callback?: string; operator?: string; keyholder?: string
  plate?: string; state?: string; vehicle?: string; hotlist?: string; camera?: string; heading?: string; image?: string; rounds?: number; confidence?: string }
export type NERISConfig = { on: boolean; clientId: string; secretSet: boolean; department: string; center: string; test: boolean; auto: boolean }
export type StationAlert = { id: string; at: string; station: string; units: string[]; incident: string; type: string; typeName: string; priority: number; address?: string; lat?: number; lon?: number; speech: string; again?: boolean; acked?: string; ackedBy?: string }
export type Determinant = { code: string; text: string; priority?: number; callType?: string; instructions?: string[] }
export type Protocol = { id: string; name: string; version: string; approved: boolean; example?: boolean; callTypes: string[]; start: string
  questions: { id: string; text: string; answers: { label: string; next?: string; determinant?: string }[] }[]; determinants: Determinant[]; updated?: string; by?: string }
export type ProtocolRun = { protocol: string; version: string; system?: string; answers: { question: string; text: string; answer: string; at: string; by: string }[]; current?: string; determinant?: Determinant; at?: string; by?: string }
export type IncidentFile = { id: string; name: string; caption?: string; type: string; sha256: string; size: number; added: string; by: string; unit?: string; hidden?: boolean; hiddenBy?: string; why?: string }
export type EvidenceItem = { n: number; name: string; type: string; kind?: string; size: number; sha256: string; received: string; recording?: boolean; taken?: string; takenLocal?: string; hasGps?: boolean; lat?: number; lon?: number }
export type Handling = {
  callerId?: { verified?: string; attest?: string; why?: string }; esinet?: boolean; service?: string; cama?: string; aliLookup?: string
  locCheck?: { show: string; text: string; reasons?: string[]; previous?: Location }; textVia?: string[]
  routes?: { service?: string; psap?: string; why: string; at: string }[]; evidence?: { held: boolean; items: EvidenceItem[] }
  recording?: string; summary?: string; summaryBy?: string; briefing?: string; briefedBy?: string; taker?: string
  endCause?: string; endedBy?: string
  lvf?: { address: string; valid: boolean; matched?: string; invalid?: string[]; position?: string; within?: boolean; suggestions?: { address: string; why: string }[]; at: string }
}

// ---- the roadmap features (internal/cad alarm.go, crew.go, fireground.go, pages.go, tow.go, schedule.go,
// directory.go, geofence.go, history.go, warrants.go; internal/api roadmap.go) ----

export type CrewSeat = { member: string; name: string; role?: string; since: string }
export type Member = { id: string; name: string; rank?: string; quals?: string[]; agency?: string; station?: string; volunteer?: boolean; phone?: string; email?: string; topic?: string; inactive?: boolean; note?: string; riding?: string }
export type MemberBrief = { id: string; name: string; rank?: string; quals?: string[]; station?: string; volunteer?: boolean; inactive?: boolean; riding?: string }
export type CapNeed = { capability: string; qualification: string; count: number }

export type PARCheck = { at: string; by: string; ok: string[]; missing?: string[]; pax?: Record<string, number>; note?: string }
export type BenchmarkMark = { code: string; name: string; at: string; by: string }
export type Benchmark = { code: string; name: string }
export type Fireground = { started: string; startedBy: string; auto?: boolean; minutes: number; lastPar?: string; checks: PARCheck[]; missing?: string[]; benchmarks: BenchmarkMark[]; ended?: string }

export type PageGroup = { id: string; name: string; members: string[]; gateways: string[]; units?: string[]; stations?: string[]; types?: string[]; note?: string; off?: boolean }
export type PageResult = { gateway: string; kind: string; to: string; ok: boolean; detail?: string; at: string }
export type PageReply = { answer: 'responding' | 'scene' | 'unavailable'; eta?: number; at: string; name: string }
export type Page = { id: string; at: string; by: string; incident?: string; groups: string[]; subject: string; text: string; auto?: boolean; to: { member: string; name: string }[]; results: PageResult[]; replies: Record<string, PageReply> }
export type Gateway = { id: string; name: string; kind: string; url?: string; token?: string; account?: string; from?: string; smtpHost?: string; smtpPort?: number; smtpUser?: string; smtpPassword?: string; smtpStartTls?: boolean; to?: string }
export type GatewayKind = { kind: string; name: string; fields: string[]; note: string }
export type ResponderBrief = { paged: number; responding: number; toScene: number; unavailable: number; replies: PageReply[] }

export type TowCompany = { id: string; name: string; phone: string; address?: string; classes: string[]; zones?: string[]; off?: boolean; note?: string }
export type Tow = { id: string; at: string; by: string; incident?: string; company: string; name: string; phone: string; class: string; zone?: string; kind: 'rotation' | 'owner' | 'police'; skipped?: string[]; vehicle?: string; result: 'called' | 'accepted' | 'declined' | 'no-answer' | 'cancelled' | 'arrived'; resultAt?: string; note?: string }
export type TowTurn = { company: TowCompany; lastTurn?: string; why?: string }

export type Scheduled = { id: string; kind: 'call' | 'event'; title: string; type: string; priority?: number; location?: Location; start: string; minutes?: number; lead?: number; units?: string[]; notes?: string; repeat?: '' | 'daily' | 'weekdays' | 'weekly'; until?: string; off?: boolean; last?: string; incident?: string; by?: string; next?: string }
export type UpcomingItem = { item: Scheduled; at: string; end: string; running?: boolean }

export type Phone = { label?: string; number: string }
export type DirEntry = { id: string; name: string; org?: string; category?: string; phones: Phone[]; email?: string; address?: string; notes?: string; tags?: string[]; types?: string[] }
export type OnCallShift = { entry: string; from: string; to: string }
export type OnCallList = { id: string; name: string; types?: string[]; shifts: OnCallShift[]; backup?: string; notes?: string }
export type CallOut = { entry?: DirEntry; list?: string; until?: string; backup?: boolean; notify?: string }
export type OnCallBrief = { list: string; id: string; entry?: DirEntry; until?: string; backup?: boolean }

export type Geofence = { id: string; name: string; kind: 'alert' | 'caution'; ring: [number, number][]; note?: string; enter?: boolean; leave?: boolean; disciplines?: string[]; units?: string[]; color?: string; off?: boolean }
export type FenceAlert = { at: string; unit: string; fence: string; name: string; enter: boolean; seq: number }

export type StatusSpan = { from: string; to: string; status: string; incident?: string; markOut?: string; reason?: string; by?: string; seconds: number; open?: boolean }
export type UnitMark = { at: string; kind: string; text: string; by?: string }
export type UnitHistory = { unit: string; from: string; to: string; spans: StatusSpan[]; marks: UnitMark[]; totals: Record<string, number>; incidents: string[]; committed: number; known: number }

export type EPCRRecord = { unit: string; at: string; ok: boolean; status?: number; error?: string; by: string }

export type PaperSubject = { name: string; dob?: string; sex?: string; description?: string }
export type PaperAttempt = { at: string; by: string; unit?: string; result: string; note?: string; incident?: string }
export type Paper = { id: string; kind: string; number?: string; court?: string; subject: PaperSubject; location?: Location; charge?: string; issued?: string; expires?: string; caution?: string; notes?: string; status: 'active' | 'served' | 'recalled' | 'returned'; statusAt?: string; statusBy?: string; attempts: PaperAttempt[]; entered: string; by: string }
export type PaperFlag = { count: number; kinds: string[]; caution?: string }

export type AlarmBrief = { level: number; name: string; nextName?: string; nextAdds?: Requirement[] }
export type RoadmapView = {
  members: MemberBrief[]; pageGroups: PageGroup[]; gateways: { id: string; name: string; kind: string }[]; pages: Page[]
  towCompanies: TowCompany[]; tows: Tow[]; towClasses: string[]; upcoming: UpcomingItem[]; callOut: Record<string, CallOut[]>
  geofences: Geofence[]; fenceAlerts: FenceAlert[]; cautions: Record<string, { id: string; name: string; note: string }[]>; papers: Record<string, PaperFlag>
  benchmarks: Benchmark[]; alarms: Record<string, AlarmBrief>; staffing: Record<string, string[]>; epcr: boolean; parDue: Record<string, string>
  responders: Record<string, ResponderBrief>; onCallNow: OnCallBrief[]; publicAlerts?: PublicAlertBrief[]
}

// ---- public alerts (internal/cad/publicalerts.go, internal/api/publicalerts.go, internal/alerting) ----
export type AlertText = { headline: string; description?: string; instruction?: string; short?: string; long?: string }
export type AlertArea = { desc: string; polygon?: [number, number][]; geocodes?: string[]; fence?: string }
export type PublicAlertBrief = {
  id: string; state: 'pending' | 'issued' | 'rejected' | 'updated' | 'cancelled'; status: 'Actual' | 'Exercise' | 'Test'; msgType: 'Alert' | 'Update' | 'Cancel'
  event: string; eventName: string; headline: string; short?: string; area: AlertArea; expires: string; sent?: string; by: string; approvedBy?: string; incident?: string
}
export type PublicAlert = Omit<PublicAlertBrief, 'headline' | 'short'> & {
  identifier: string; of?: string; category: string; response?: string; urgency: string; severity: string; certainty: string
  text: AlertText; spanish?: AlertText; block?: string[]; lists?: string[]; receivers?: string[]; created: string; rejected?: string
  supersededBy?: string; results: PageResult[]; reached: number
}
export type Subscriber = { name?: string; phone?: string; email?: string; topic?: string; lang?: string; lat?: number; lon?: number }
export type AlertList = { id: string; name: string; gateways: string[]; people: Subscriber[]; note?: string }
export type CAPReceiver = { id: string; name: string; url: string; token?: string; off?: boolean }
export type SAMEEvent = { code: string; name: string; category: string; warning: boolean }
export type AlertingSettings = {
  sender: string; senderName: string; jurisdiction: string; geocodes: string[]; feed: boolean; twoPerson: boolean; timeZone?: string
  web?: string; contact?: string; receivers: CAPReceiver[]; feedUrl: string; events: SAMEEvent[]; channels: string[]; ready: string[]
}
