import { NavLink, Navigate, Route, Routes, Link, useLocation } from 'react-router-dom'
import { HelpLink } from '../Help'
import { lazy, Suspense } from 'react'
import { ArrowLeft, Award, BarChart3, BookOpen, FileLock2, Hospital, Layers3, Lightbulb, MonitorSmartphone, Network, Route as RouteIcon, ShieldAlert, FileInput, Building2, Cable, ClipboardList, Database, Map as MapIcon, Settings as Cog, Truck, Users as UsersIcon, ListChecks } from 'lucide-react'
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

// Charts are large; they load when the report is opened.
const Reports = lazy(() => import('./Reports'))
const Quality = lazy(() => import('./Quality'))

const PAGES = [
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
  { to: 'premises', label: 'Premises & hazards', icon: ShieldAlert, admin: false },
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
  if (!can.supervise(me)) return <Navigate to="/" replace />
  const pages = PAGES.filter((p) => !p.admin || can.admin(me))
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
            <Route path="ledger" element={<Ledger />} />
            <Route path="disclosure" element={<Disclosure view={view} />} />
            <Route path="advisory" element={<AdvisoryAdmin />} />
            <Route path="guidance" element={<GuidanceAdmin view={view} />} />
            <Route path="protocols" element={<Protocols view={view} />} />
            <Route path="gis" element={<GisAdmin />} />
            <Route path="sites" element={<Sites />} />
            <Route path="premises" element={<Premises />} />
            <Route path="migrate" element={<Migrate />} />
            <Route path="quality" element={<Suspense fallback={<p className="text-sm text-slate-500">Loading…</p>}><Quality /></Suspense>} />
            <Route path="reports" element={<Suspense fallback={<p className="text-sm text-slate-500">Loading…</p>}><Reports view={view} /></Suspense>} />
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
