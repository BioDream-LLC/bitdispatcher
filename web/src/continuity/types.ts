// The shapes of continuity and overflow (internal/cad/continuity.go, internal/api/continuity.go and
// overflow.go).

export type MapSchedule = { mode?: '' | 'off' | 'daily' | 'weekly'; hour?: number; weekday?: number; maxDropPct?: number }
export type ConnCheck = { name: string; url: string }
export type ContinuityPolicy = {
  checks?: ConnCheck[]; checkSeconds?: number; steps?: string[]; contacts?: string[]
  packMinutes?: number; packDirs?: string[]; overflowMinutes?: number
}
export type LinkState = { name: string; up: boolean; since: string; error?: string }
export type Outage = { name: string; from: string; to?: string; error?: string }
export type Relocation = { active: boolean; site?: string; note?: string; by: string; since: string; ended?: string; done: number[] }
export type OverflowBrief = { partner: string; name: string; direction: 'in' | 'out'; status: string; until: string }
export type MapAutoHealth = { mode: string; next?: string; last?: string; result?: string; error?: string }
export type ContinuityHealth = { internet: 'up' | 'down' | 'unknown'; down?: LinkState[]; relocation?: Relocation; overflow?: OverflowBrief[]; mapAuto?: MapAutoHealth }

export type ContinuityInfo = {
  health: ContinuityHealth; links: LinkState[]; outages: Outage[]; steps?: string[]; contacts?: string[]
  relocation?: Relocation | null; site?: string; packMinutes?: number; checks: number; packAt?: string; packErrors?: string[] | null
  sites?: { name: string; url: string; ok: boolean; last?: string; error?: string; copied: number }[] | null
}

export type OverflowType = { code: string; name: string; priority: number }
export type OverflowSession = {
  partner: string; partnerName: string; direction: 'in' | 'out'; status: 'asked' | 'active' | 'declined' | 'ended'
  note?: string; by: string; since: string; until: string; answeredBy?: string; answered?: string; endedBy?: string; ended?: string
  dispatch: boolean; agencies?: string[]; callTypes?: OverflowType[]
}
export type OverflowUnit = { unit: string; status: string }
export type OverflowNote = { at: string; by: string; text: string; delivered: boolean; theirs?: boolean }
export type OverflowCall = {
  ref: string; partner: string; taken: string; by: string; type: string; typeName?: string; priority: number; address: string
  lat?: number; lon?: number; callerName?: string; callback?: string; narrative?: string; delivered: boolean; tries?: number; error?: string
  number?: string; status?: string; units?: OverflowUnit[]; closed?: boolean; disposition?: string; notes?: OverflowNote[]; updated: string
}
export type OverflowPartner = { id: string; name: string; ask: boolean; help: boolean; dispatch: boolean; autoAccept: boolean; problem?: string }
export type OverflowInfo = {
  partners: OverflowPartner[]; supervisor: boolean; sessions: OverflowSession[]; calls: OverflowCall[]
  takenForUs: { number: string; partner: string; by: string; type: string; status: string; created: string }[]; minutes: number
}
export type PartnerOverflow = { ask: boolean; help: boolean; autoAccept: boolean; dispatch: boolean; agencies?: string[]; callTypes?: string[] }
export type PartnerView = { id: string; name: string; url: string; outTokenSet: boolean; inTokenSet: boolean; receive: string; overflow: PartnerOverflow; overflowProblem?: string }

export const live = (o: { status: string; until: string }, now: number) => (o.status === 'active' || o.status === 'asked') && Date.parse(o.until) > now
