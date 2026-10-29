import { NavLink, Navigate, Route, Routes, Link, useLocation, useNavigate } from 'react-router-dom'
import { HelpLink } from '../Help'
import { lazy, Suspense, useState } from 'react'
import { ArrowLeft, Award, BarChart3, BookOpen, FileLock2, Hospital, Layers3, Lightbulb, MonitorSmartphone, Network, Route as RouteIcon, ShieldAlert, FileInput, Building2, Cable, ClipboardList, Database, Map as MapIcon, Settings as Cog, Truck, Users as UsersIcon, ListChecks, HardHat, BellRing, BookUser, Hexagon, Megaphone, Landmark, SlidersHorizontal, BadgeCheck, Ban, Cctv, Siren, Bot, ListTodo, Rocket, LifeBuoy, Flag, ShieldQuestion, PhoneIncoming, Waypoints, ArrowRightLeft, Drone, Wrench, PhoneCall, MapPinned, ScrollText, ChevronRight, Search } from 'lucide-react'
import { can, useStore } from '../store'
import { Logo } from '../ui'
import { cx } from '../util'
import Units from './Units'
import Stations from './Stations'
import CallTypes from './CallTypes'
import SettingsPage from './Settings'
import Integrations from './Integrations'
import Users from './Users'
import Ledger from './Ledger'
import MapAdmin from './MapAdmin'
import AdvisoryAdmin from './AdvisoryAdmin'
import GisAdmin from './GisAdmin'
import Sites from './Sites'
import Drones from './Drones'
import Premises from './Premises'
import Migrate from './Migrate'
import Workstations from './Workstations'
import Plans from './Plans'
import Facilities from './Facilities'
import GuidanceAdmin from './GuidanceAdmin'
import Protocols from './Protocols'
import Disclosure from './Disclosure'
import Personnel from './Personnel'
import Paging from './Paging'
import PublicAlerts from './PublicAlerts'
import Tows from './Tows'
import Directory from './Directory'
import Geofences from './Geofences'
import RoadsAdmin from './RoadsAdmin'
import CamerasAdmin from './CamerasAdmin'
import AlarmsAdmin from './AlarmsAdmin'
import IntakeAdmin from './IntakeAdmin'
import Setup from './Setup'
import GoLive from './GoLive'
import Support from './Support'
import { Agencies } from './Agencies'
import { OperationsRules } from './OperationsRules'
import { Qualifications } from './Qualifications'
import { CallEntryRules, CriticalAdmin, FlagsAdmin, PipAdmin } from './FinalAdmin'
import { ContinuityAdmin } from './ContinuityAdmin'
import { useViews, VIEW_OF_PAGE } from '../access'

// Charts are large; they load when the report is opened.
const Reports = lazy(() => import('./Reports'))
const Quality = lazy(() => import('./Quality'))

// The admin pages in groups, each with its own colour. Every page belongs to exactly one group.
type NavPage = { to: string; label: string; icon: typeof Truck; admin: boolean }
type Group = { id: string; label: string; icon: typeof Truck; tone: Tone; pages: NavPage[] }
type Tone = 'emerald' | 'sky' | 'orange' | 'rose' | 'teal' | 'violet' | 'amber' | 'slate'

// Full class names, so Tailwind keeps them.
const TONE: Record<Tone, { tile: string; text: string }> = {
  emerald: { tile: 'bg-emerald-500/20 text-emerald-300 ring-1 ring-inset ring-emerald-400/25', text: 'text-emerald-400' },
  sky: { tile: 'bg-sky-500/20 text-sky-300 ring-1 ring-inset ring-sky-400/25', text: 'text-sky-400' },
  orange: { tile: 'bg-orange-500/20 text-orange-300 ring-1 ring-inset ring-orange-400/25', text: 'text-orange-400' },
  rose: { tile: 'bg-rose-500/20 text-rose-300 ring-1 ring-inset ring-rose-400/25', text: 'text-rose-400' },
  teal: { tile: 'bg-teal-500/20 text-teal-300 ring-1 ring-inset ring-teal-400/25', text: 'text-teal-400' },
  violet: { tile: 'bg-violet-500/20 text-violet-300 ring-1 ring-inset ring-violet-400/25', text: 'text-violet-400' },
  amber: { tile: 'bg-amber-500/20 text-amber-300 ring-1 ring-inset ring-amber-400/25', text: 'text-amber-400' },
  slate: { tile: 'bg-slate-500/25 text-slate-200 ring-1 ring-inset ring-slate-400/25', text: 'text-slate-300' },
}

const GROUPS: Group[] = [
  { id: 'start', label: 'Getting started', icon: Rocket, tone: 'emerald', pages: [
    { to: 'setup', label: 'Setup checklist', icon: ListTodo, admin: true },
    { to: 'golive', label: 'Go-live check', icon: Rocket, admin: true },
    { to: 'support', label: 'Support', icon: LifeBuoy, admin: true },
    { to: 'migrate', label: 'Migration', icon: FileInput, admin: true },
  ] },
  { id: 'people', label: 'People', icon: UsersIcon, tone: 'sky', pages: [
    { to: 'agencies', label: 'Agencies', icon: Landmark, admin: true },
    { to: 'users', label: 'Users & positions', icon: UsersIcon, admin: true },
    { to: 'personnel', label: 'Personnel & crews', icon: HardHat, admin: false },
    { to: 'qualifications', label: 'Qualifications & teams', icon: BadgeCheck, admin: false },
    { to: 'workstations', label: 'Workstations', icon: MonitorSmartphone, admin: true },
  ] },
  { id: 'resources', label: 'Units & resources', icon: Truck, tone: 'orange', pages: [
    { to: 'units', label: 'Units', icon: Truck, admin: true },
    { to: 'stations', label: 'Stations', icon: Building2, admin: true },
    { to: 'facilities', label: 'Facilities', icon: Hospital, admin: true },
    { to: 'drones', label: 'Drones', icon: Drone, admin: true },
    { to: 'tows', label: 'Tow rotation', icon: Wrench, admin: false },
    { to: 'directory', label: 'Contacts & on call', icon: BookUser, admin: false },
  ] },
  { id: 'calls', label: 'Call handling', icon: PhoneCall, tone: 'rose', pages: [
    { to: 'calltypes', label: 'Call types & run cards', icon: ClipboardList, admin: true },
    { to: 'plans', label: 'Response plans', icon: RouteIcon, admin: true },
    { to: 'callentry', label: 'Call entry rules', icon: PhoneIncoming, admin: true },
    { to: 'protocols', label: 'Call-taking protocols', icon: ListChecks, admin: true },
    { to: 'guidance', label: 'Approved guidance', icon: BookOpen, admin: true },
    { to: 'advisory', label: 'Advisory panel', icon: Lightbulb, admin: false },
    { to: 'critical', label: 'Critical incidents', icon: ShieldQuestion, admin: true },
    { to: 'intake', label: 'Non-emergency intake', icon: Bot, admin: true },
    { to: 'operations', label: 'Operations rules', icon: SlidersHorizontal, admin: true },
  ] },
  { id: 'map', label: 'Map & places', icon: MapPinned, tone: 'teal', pages: [
    { to: 'map', label: 'Map', icon: MapIcon, admin: true },
    { to: 'gis', label: 'County GIS', icon: Layers3, admin: true },
    { to: 'geofences', label: 'Areas (geofences)', icon: Hexagon, admin: false },
    { to: 'roads', label: 'Road closures & traffic', icon: Ban, admin: false },
    { to: 'premises', label: 'Premises & hazards', icon: ShieldAlert, admin: false },
    { to: 'flags', label: 'Premise & safety flags', icon: Flag, admin: false },
    { to: 'cameras', label: 'Cameras', icon: Cctv, admin: false },
    { to: 'alarms', label: 'Alarm companies & permits', icon: Siren, admin: true },
  ] },
  { id: 'alerts', label: 'Paging & alerts', icon: BellRing, tone: 'violet', pages: [
    { to: 'paging', label: 'Paging groups', icon: BellRing, admin: false },
    { to: 'publicalerts', label: 'Public alerts', icon: Megaphone, admin: false },
  ] },
  { id: 'records', label: 'Records & quality', icon: ScrollText, tone: 'amber', pages: [
    { to: 'reports', label: 'Operations report', icon: BarChart3, admin: false },
    { to: 'quality', label: 'Quality review', icon: Award, admin: false },
    { to: 'ledger', label: 'Ledger & audit', icon: Database, admin: false },
    { to: 'disclosure', label: 'Disclosure rules', icon: FileLock2, admin: true },
  ] },
  { id: 'system', label: 'System', icon: Cog, tone: 'slate', pages: [
    { to: 'settings', label: 'Settings', icon: Cog, admin: true },
    { to: 'integrations', label: 'Integrations', icon: Cable, admin: true },
    { to: 'sites', label: 'Sites', icon: Network, admin: true },
    { to: 'continuity', label: 'Continuity & overflow', icon: ArrowRightLeft, admin: true },
    { to: 'pip', label: 'PSC PIP connection', icon: Waypoints, admin: true },
  ] },
]
const PAGES = GROUPS.flatMap((g) => g.pages)

export default function Admin() {
  const me = useStore((s) => s.me)
  const view = useStore((s) => s.view)
  const views = useViews(me ? me.username + '|' + me.duty : undefined)
  if (!can.supervise(me)) return <Navigate to="/" replace />
  // Supervisory views the agency has not given this person are not offered (and the server refuses them).
  const pages = PAGES.filter((p) => (!p.admin || can.admin(me)) && (!VIEW_OF_PAGE[p.to] || !views || views.has(VIEW_OF_PAGE[p.to])))
  return (
    <div className="flex h-full min-h-0 bg-slate-950">
      <AdminNav pages={pages} seq={view?.seq} />
      <main className="min-w-0 flex-1 overflow-y-auto p-5">
        {!view ? (
          <p className="text-sm text-slate-500">Loading…</p>
        ) : (
          <Routes>
            <Route index element={<Navigate to={'/admin/' + pages[0].to} replace />} />
            <Route path="setup" element={<Setup />} />
            <Route path="golive" element={<GoLive />} />
            <Route path="support" element={<Support view={view} />} />
            <Route path="agencies" element={<Agencies view={view} />} />
            <Route path="operations" element={<OperationsRules view={view} />} />
            <Route path="qualifications" element={<Qualifications view={view} />} />
            <Route path="units" element={<Units view={view} />} />
            <Route path="stations" element={<Stations view={view} />} />
            <Route path="facilities" element={<Facilities view={view} />} />
            <Route path="calltypes" element={<CallTypes view={view} />} />
            <Route path="plans" element={<Plans view={view} />} />
            <Route path="map" element={<MapAdmin view={view} />} />
            <Route path="settings" element={<SettingsPage view={view} />} />
            <Route path="integrations" element={<Integrations view={view} />} />
            <Route path="users" element={<Users />} />
            <Route path="workstations" element={<Workstations />} />
            <Route path="ledger" element={views && !views.has('ledger') ? <NotGiven /> : <Ledger />} />
            <Route path="disclosure" element={<Disclosure view={view} />} />
            <Route path="advisory" element={<AdvisoryAdmin />} />
            <Route path="guidance" element={<GuidanceAdmin view={view} />} />
            <Route path="protocols" element={<Protocols view={view} />} />
            <Route path="gis" element={<GisAdmin />} />
            <Route path="sites" element={<Sites />} />
            <Route path="drones" element={<Drones />} />
            <Route path="premises" element={<Premises />} />
            <Route path="flags" element={<FlagsAdmin />} />
            <Route path="callentry" element={<CallEntryRules />} />
            <Route path="critical" element={<CriticalAdmin />} />
            <Route path="pip" element={<PipAdmin />} />
            <Route path="continuity" element={<ContinuityAdmin view={view} />} />
            <Route path="personnel" element={<Personnel view={view} />} />
            <Route path="paging" element={<Paging view={view} />} />
            <Route path="publicalerts" element={<PublicAlerts view={view} />} />
            <Route path="tows" element={<Tows view={view} />} />
            <Route path="directory" element={<Directory view={view} />} />
            <Route path="geofences" element={<Geofences view={view} />} />
            <Route path="roads" element={<RoadsAdmin view={view} />} />
            <Route path="cameras" element={<CamerasAdmin view={view} />} />
            <Route path="alarms" element={<AlarmsAdmin view={view} />} />
            <Route path="intake" element={<IntakeAdmin view={view} />} />
            <Route path="migrate" element={<Migrate />} />
            <Route path="quality" element={views && !views.has('quality') ? <NotGiven /> : <Suspense fallback={<p className="text-sm text-slate-500">Loading…</p>}><Quality /></Suspense>} />
            <Route path="reports" element={views && !views.has('operations') ? <NotGiven /> : <Suspense fallback={<p className="text-sm text-slate-500">Loading…</p>}><Reports view={view} /></Suspense>} />
            <Route path="*" element={<Navigate to={'/admin/' + pages[0].to} replace />} />
          </Routes>
        )}
      </main>
    </div>
  )
}

const OPEN_KEY = 'bd.admin.groups'

// AdminNav is the admin sidebar: pages in collapsible groups (the current page's group is always
// open; the others remember being opened), a filter, and one line per page, never wrapped.
function AdminNav({ pages, seq }: { pages: NavPage[]; seq?: number }) {
  const here = useLocation().pathname.split('/')[2] ?? ''
  const go = useNavigate()
  const [q, setQ] = useState('')
  const [open, setOpen] = useState<Record<string, boolean>>(() => {
    try { return JSON.parse(localStorage.getItem(OPEN_KEY) ?? '{}') } catch { return {} }
  })
  const allowed = new Set(pages.map((p) => p.to))
  const find = q.trim().toLowerCase()
  const toggle = (id: string, now: boolean) => {
    const next = { ...open, [id]: !now }
    setOpen(next)
    try { localStorage.setItem(OPEN_KEY, JSON.stringify(next)) } catch { /* private window */ }
  }
  return (
    <nav className="flex h-full w-64 shrink-0 flex-col border-r border-slate-800 bg-slate-900" aria-label="Administration">
      <div className="shrink-0 space-y-2 border-b border-slate-800 p-2">
        <Link to="/" className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm font-semibold text-slate-200 hover:bg-slate-800">
          <ArrowLeft size={14} aria-hidden /> <Logo size={18} /> Dispatch
        </Link>
        <label className="relative block">
          <span className="sr-only">Find a page</span>
          <Search size={13} className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-500" aria-hidden />
          <input className="input h-8 w-full pl-8 text-[13px]" placeholder="Find a page" value={q} onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Escape') setQ('')
              if (e.key === 'Enter' && find) {
                const first = GROUPS.flatMap((g) => g.pages).find((p) => allowed.has(p.to) && p.label.toLowerCase().includes(find))
                if (first) { go('/admin/' + first.to); setQ('') }
              }
            }} />
        </label>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-2">
        {GROUPS.map((g) => {
          const items = g.pages.filter((p) => allowed.has(p.to) && (!find || p.label.toLowerCase().includes(find)))
          if (!items.length) return null
          const t = TONE[g.tone]
          const isOpen = !!find || items.some((p) => p.to === here) || !!open[g.id]
          return (
            <div key={g.id} className="mb-1">
              <button type="button" aria-expanded={isOpen} onClick={() => toggle(g.id, isOpen)}
                className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[11px] font-semibold uppercase tracking-wide text-slate-400 hover:bg-slate-800/60 hover:text-slate-200">
                <g.icon size={13} className={t.text} aria-hidden />
                <span className="flex-1 truncate">{g.label}</span>
                <span className="rounded bg-slate-800 px-1.5 text-[10px] font-medium text-slate-400">{items.length}</span>
                <ChevronRight size={13} className={cx('transition-transform', isOpen && 'rotate-90')} aria-hidden />
              </button>
              {isOpen && (
                <div className="mt-0.5 space-y-px">
                  {items.map((p) => (
                    <NavLink key={p.to} to={'/admin/' + p.to} title={p.label}
                      className={({ isActive }) => cx('flex items-center gap-2.5 whitespace-nowrap rounded-md py-1 pl-2 pr-2 text-[13px]',
                        isActive ? 'relative bg-slate-800 text-slate-50 before:absolute before:inset-y-1 before:left-0 before:w-0.5 before:rounded-full before:bg-sky-400' : 'text-slate-300 hover:bg-slate-800/60 hover:text-slate-50')}>
                      <span className={cx('grid h-6 w-6 shrink-0 place-items-center rounded-md', t.tile)}>
                        <p.icon size={14} strokeWidth={2} aria-hidden />
                      </span>
                      <span className="truncate">{p.label}</span>
                    </NavLink>
                  ))}
                </div>
              )}
            </div>
          )
        })}
        {find && !pages.some((p) => p.label.toLowerCase().includes(find)) && <p className="px-2 py-3 text-xs text-slate-500">No page matches.</p>}
      </div>
      <div className="shrink-0 border-t border-slate-800 px-3 py-2 text-[10px] text-slate-500">
        Every change here is an attributed event in the ledger.
        {seq !== undefined && <div className="mt-0.5 font-mono">ledger #{seq}</div>}
      </div>
    </nav>
  )
}

export function Page({ title, intro, actions, children }: { title: string; intro: string; actions?: React.ReactNode; children: React.ReactNode }) {
  const { pathname } = useLocation()
  return (
    <div className="mx-auto max-w-6xl">
      <div className="mb-4 flex items-start gap-4">
        <div>
          <h1 className="text-xl font-bold text-slate-50">{title}</h1>
          <p className="mt-1 max-w-3xl text-sm text-slate-400">{intro}</p>
        </div>
        <div className="ml-auto flex shrink-0 gap-2">{actions}<HelpLink path={pathname} /></div>
      </div>
      {children}
    </div>
  )
}

export function Table({ head, children, empty }: { head: string[]; children: React.ReactNode; empty?: React.ReactNode }) {
  return (
    <div className="panel overflow-x-auto rounded-lg">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-slate-800 text-left text-[11px] uppercase tracking-wider text-slate-500">
            {head.map((h) => <th key={h} className="px-3 py-2 font-semibold">{h}</th>)}
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-800">{children}</tbody>
      </table>
      {empty}
    </div>
  )
}

/** A supervisory view the agency has not given this person. */
function NotGiven() {
  return <p className="text-sm text-slate-400">The agency has not given you this view. An administrator decides who sees it (Admin › Operations rules).</p>
}
