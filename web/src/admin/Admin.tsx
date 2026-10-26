import { NavLink, Navigate, Route, Routes, Link, useLocation } from 'react-router-dom'
import { HelpLink } from '../Help'
import { lazy, Suspense } from 'react'
import { ArrowLeft, Award, BarChart3, BookOpen, FileLock2, Hospital, Layers3, Lightbulb, MonitorSmartphone, Network, Route as RouteIcon, ShieldAlert, FileInput, Building2, Cable, ClipboardList, Database, Map as MapIcon, Settings as Cog, Truck, Users as UsersIcon, ListChecks, HardHat, BellRing, Truck as TowIcon, BookUser, Hexagon, Megaphone, Landmark, SlidersHorizontal, BadgeCheck, Ban, Cctv, Siren, Bot, ListTodo, Rocket, LifeBuoy, Flag, ShieldQuestion, PhoneIncoming, Waypoints, ArrowRightLeft } from 'lucide-react'
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

const PAGES = [
  { to: 'setup', label: 'Setup checklist', icon: ListTodo, admin: true },
  { to: 'golive', label: 'Go-live check', icon: Rocket, admin: true },
  { to: 'support', label: 'Support', icon: LifeBuoy, admin: true },
  { to: 'agencies', label: 'Agencies', icon: Landmark, admin: true },
  { to: 'units', label: 'Units', icon: Truck, admin: true },
  { to: 'stations', label: 'Stations', icon: Building2, admin: true },
  { to: 'facilities', label: 'Facilities', icon: Hospital, admin: true },
  { to: 'calltypes', label: 'Call types & run cards', icon: ClipboardList, admin: true },
  { to: 'plans', label: 'Response plans', icon: RouteIcon, admin: true },
  { to: 'map', label: 'Map', icon: MapIcon, admin: true },
  { to: 'gis', label: 'County GIS', icon: Layers3, admin: true },
  { to: 'settings', label: 'Settings', icon: Cog, admin: true },
  { to: 'integrations', label: 'Integrations', icon: Cable, admin: true },
  { to: 'users', label: 'Users & positions', icon: UsersIcon, admin: true },
  { to: 'workstations', label: 'Workstations', icon: MonitorSmartphone, admin: true },
  { to: 'sites', label: 'Sites', icon: Network, admin: true },
  { to: 'continuity', label: 'Continuity & overflow', icon: ArrowRightLeft, admin: true },
  { to: 'operations', label: 'Operations rules', icon: SlidersHorizontal, admin: true },
  { to: 'personnel', label: 'Personnel & crews', icon: HardHat, admin: false },
  { to: 'qualifications', label: 'Qualifications & teams', icon: BadgeCheck, admin: false },
  { to: 'paging', label: 'Paging groups', icon: BellRing, admin: false },
  { to: 'publicalerts', label: 'Public alerts', icon: Megaphone, admin: false },
  { to: 'tows', label: 'Tow rotation', icon: TowIcon, admin: false },
  { to: 'directory', label: 'Contacts & on call', icon: BookUser, admin: false },
  { to: 'geofences', label: 'Areas (geofences)', icon: Hexagon, admin: false },
  { to: 'roads', label: 'Road closures & traffic', icon: Ban, admin: false },
  { to: 'cameras', label: 'Cameras', icon: Cctv, admin: false },
  { to: 'alarms', label: 'Alarm companies & permits', icon: Siren, admin: true },
  { to: 'intake', label: 'Non-emergency intake', icon: Bot, admin: true },
  { to: 'premises', label: 'Premises & hazards', icon: ShieldAlert, admin: false },
  { to: 'flags', label: 'Premise & safety flags', icon: Flag, admin: false },
  { to: 'callentry', label: 'Call entry rules', icon: PhoneIncoming, admin: true },
  { to: 'critical', label: 'Critical incidents', icon: ShieldQuestion, admin: true },
  { to: 'pip', label: 'PSC PIP connection', icon: Waypoints, admin: true },
  { to: 'migrate', label: 'Migration', icon: FileInput, admin: true },
  { to: 'advisory', label: 'Advisory panel', icon: Lightbulb, admin: false },
  { to: 'guidance', label: 'Approved guidance', icon: BookOpen, admin: true },
  { to: 'protocols', label: 'Call-taking protocols', icon: ListChecks, admin: true },
  { to: 'reports', label: 'Operations report', icon: BarChart3, admin: false },
  { to: 'quality', label: 'Quality review', icon: Award, admin: false },
  { to: 'ledger', label: 'Ledger & audit', icon: Database, admin: false },
  { to: 'disclosure', label: 'Disclosure rules', icon: FileLock2, admin: true },
]

export default function Admin() {
  const me = useStore((s) => s.me)
  const view = useStore((s) => s.view)
  const views = useViews(me ? me.username + '|' + me.duty : undefined)
  if (!can.supervise(me)) return <Navigate to="/" replace />
  // Supervisory views the agency has not given this person are not offered (and the server refuses them).
  const pages = PAGES.filter((p) => (!p.admin || can.admin(me)) && (!VIEW_OF_PAGE[p.to] || !views || views.has(VIEW_OF_PAGE[p.to])))
  return (
    <div className="flex h-full bg-slate-950">
      <nav className="flex w-56 shrink-0 flex-col border-r border-slate-800 bg-slate-900 p-2" aria-label="Administration">
        <Link to="/" className="mb-3 flex items-center gap-2 rounded px-2 py-1.5 text-sm font-semibold text-slate-200 hover:bg-slate-800">
          <ArrowLeft size={14} aria-hidden /> <Logo size={18} /> Dispatch
        </Link>
        {pages.map((p) => (
          <NavLink
            key={p.to}
            to={p.to}
            className={({ isActive }) => cx('flex items-center gap-2 rounded px-2 py-1.5 text-sm', isActive ? 'bg-slate-800 text-slate-50' : 'text-slate-400 hover:bg-slate-800/60 hover:text-slate-200')}
          >
            <p.icon size={14} aria-hidden /> {p.label}
          </NavLink>
        ))}
        <div className="mt-auto px-2 text-[10px] text-slate-500">
          Every change here is an attributed event in the ledger.
          {view && <div className="mt-1 font-mono">ledger #{view.seq}</div>}
        </div>
      </nav>
      <main className="min-w-0 flex-1 overflow-y-auto p-5">
        {!view ? (
          <p className="text-sm text-slate-500">Loading…</p>
        ) : (
          <Routes>
            <Route index element={<Navigate to={pages[0].to} replace />} />
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
            <Route path="*" element={<Navigate to={pages[0].to} replace />} />
          </Routes>
        )}
      </main>
    </div>
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
