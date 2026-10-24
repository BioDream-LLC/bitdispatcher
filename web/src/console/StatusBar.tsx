import { useState } from 'react'
import { DirectoryDialog, ScheduleDialog } from './Roadmap'
import { BreakButton } from './Dream'
import { Link } from 'react-router-dom'
import { Activity, CircleHelp, Cog, LogOut, Moon, Radio, Server, Siren, Sun, Phone, FileText, Database, MapIcon, MonitorCheck, LayoutDashboard, BookUser, CalendarClock, Gavel } from 'lucide-react'
import { DISCIPLINES, lensName } from '../lens'
import { screenDef, type ScreenKind } from '../screens'
import { api } from '../api'
import { can, store, useStore } from '../store'
import { Logo, Tip } from '../ui'
import { ago, cx, discipline, ms, useNow } from '../util'
import type { View } from '../types'
import { PublicAlertsButton } from './PublicAlerts'

const THEMES = ['midnight', 'dark', 'night', 'light'] as const

function Health({ ok, warn, icon, label, detail }: { ok: boolean; warn?: boolean; icon: React.ReactNode; label: string; detail: React.ReactNode }) {
  return (
    <Tip text={detail} side="bottom">
      <span
        className={cx(
          'inline-flex h-6 items-center gap-1 rounded px-1.5 text-[11px] font-semibold',
          ok ? 'text-emerald-300' : warn ? 'bg-amber-500/15 text-amber-300' : 'bg-rose-500/20 text-rose-300',
        )}
        role="status"
        aria-label={`${label}: ${ok ? 'ok' : warn ? 'attention' : 'down'}`}
      >
        {icon}
        {label}
        <span className={cx('h-1.5 w-1.5 rounded-full', ok ? 'bg-emerald-400' : warn ? 'bg-amber-400' : 'bg-rose-400 live-dot')} aria-hidden />
      </span>
    </Tip>
  )
}

function Clock({ now }: { now: number }) {
  const d = new Date(now)
  const local = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
  const utc = d.toISOString().slice(11, 19)
  const date = d.toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' })
  return (
    <div className="flex shrink-0 items-baseline gap-2 whitespace-nowrap font-mono tnum">
      <span className="text-[15px] font-bold text-slate-50">{local}</span>
      <span className="hidden text-[11px] text-slate-500 min-[1700px]:inline">{utc}Z</span>
      <span className="hidden text-[11px] text-slate-500 min-[1900px]:inline">{date}</span>
    </div>
  )
}

export default function StatusBar({ view, screen = 'full', onScreens, layout }: { view: View; screen?: ScreenKind; onScreens?: () => void; layout?: React.ReactNode }) {
  const now = useNow()
  const me = useStore((s) => s.me)
  const lens = useStore((s) => s.lens)
  const stream = useStore((s) => s.stream)
  const h = view.health
  const bv = h.bitvoice
  const pending = view.incidents.filter((i) => i.status === 'pending')
  const active = view.incidents.filter((i) => i.status === 'active')
  const ringing = view.calls.filter((c) => c.state === 'ringing').length
  const live911 = view.calls.filter((c) => c.state !== 'ended' && !c.incident).length
  const p1 = pending.filter((i) => i.priority === 1).length
  const oldestPending = pending.reduce((m, i) => Math.min(m, ms(i.created)), Infinity)
  const byDisc: Record<string, { av: number; all: number }> = {}
  for (const u of view.units) {
    const d = discipline(view, u)
    byDisc[d] ??= { av: 0, all: 0 }
    byDisc[d].all++
    if (u.status === 'available') byDisc[d].av++
  }
  const openEm = view.emergencies.filter((e) => !e.ackedAt).length
  const theme = (document.documentElement.dataset.theme ?? 'midnight') as (typeof THEMES)[number]
  const nextTheme = () => {
    const t = THEMES[(THEMES.indexOf(theme) + 1) % THEMES.length]
    document.documentElement.dataset.theme = t
    localStorage.setItem('bd-theme', t)
    store.set({})
  }
  const bvAge = bv.lastEvent ? ago(bv.lastEvent, now) : 'never'
  return (
    <header className="flex h-10 shrink-0 items-center gap-3 border-b border-slate-800 bg-slate-950 px-2.5" aria-label="Status">
      <div className="flex items-center gap-1.5">
        <Logo size={20} />
        <span className="text-sm font-bold tracking-tight text-slate-50">bitdispatcher</span>
      </div>
      <div className="hidden min-w-0 flex-col leading-tight lg:flex">
        <span className="truncate text-[11px] font-semibold text-slate-300">{view.settings.agency || 'Agency not named (Admin › Settings)'}</span>
        <span className="truncate text-[10px] text-slate-500">
          {me?.name || me?.username} · {me?.console || 'no position'} · {me?.role}
        </span>
      </div>
      {layout && <div className="flex shrink-0 items-center gap-0.5">{layout}</div>}
      {screen !== 'full' && (
        <span className="chip h-6 shrink-0 gap-1 whitespace-nowrap bg-sky-500/15 px-2 text-[11px] font-bold text-sky-200" title={screenDef(screen).hint}>
          <MonitorCheck size={12} aria-hidden />{screenDef(screen).title}
        </span>
      )}
      {lens.length > 0 && (
        <button type="button" onClick={onScreens} className="inline-flex h-6 shrink-0 items-center gap-1 whitespace-nowrap rounded px-1.5 text-[10px] font-black uppercase tracking-wider text-slate-950" title={`This screen shows ${lensName(lens)}: change in Screens`}
          style={{ background: `linear-gradient(90deg, ${lens.map((d) => DISCIPLINES.find((x) => x.d === d)?.color ?? '#94a3b8').join(', ')}${lens.length === 1 ? ', ' + (DISCIPLINES.find((x) => x.d === lens[0])?.color ?? '#94a3b8') : ''})` }}>
          {lens.map((d) => DISCIPLINES.find((x) => x.d === d)?.short ?? d).join(' + ')}
        </button>
      )}
      <Clock now={now} />
      <div className="mx-1 h-6 w-px bg-slate-800" aria-hidden />
      {/* Workload at a glance: every number is something the dispatcher acts on. */}
      <div className="flex shrink-0 items-center gap-2 whitespace-nowrap text-[11px] font-semibold tnum">
        <Tip text="911 calls in progress not yet on an incident (ringing first)" side="bottom">
          <span className={cx('inline-flex items-center gap-1 rounded px-1.5 py-0.5', ringing ? 'flash text-white' : live911 ? 'bg-sky-500/15 text-sky-300' : 'text-slate-500')}>
            <Phone size={12} aria-hidden /> {live911} call{live911 === 1 ? '' : 's'}
            {ringing > 0 && <span>· {ringing} ringing</span>}
          </span>
        </Tip>
        <Tip text={pending.length ? `Oldest pending for ${ago(oldestPending, now)}` : 'No incident is waiting for a unit'} side="bottom">
          <span className={cx('inline-flex items-center gap-1 rounded px-1.5 py-0.5', p1 ? 'bg-rose-500/25 text-rose-200' : pending.length ? 'bg-amber-500/15 text-amber-300' : 'text-slate-500')}>
            <FileText size={12} aria-hidden /> {pending.length} pending{p1 ? ` · ${p1} P1` : ''}
            {pending.length > 0 && <span className="font-mono text-[10px] opacity-80">{ago(oldestPending, now)}</span>}
          </span>
        </Tip>
        <span className="text-slate-400">
          <span className="text-slate-200">{active.length}</span> active
        </span>
        {Object.entries(byDisc).map(([d, n]) => (
          <Tip key={d} text={`${d}: ${n.av} of ${n.all} units available`} side="bottom">
            <span className={cx('rounded px-1.5 py-0.5 uppercase', n.av === 0 ? 'bg-rose-500/20 text-rose-300' : n.av <= 1 ? 'bg-amber-500/15 text-amber-300' : 'text-slate-400')}>
              {DISCIPLINES.find((x) => x.d === d)?.short ?? d.slice(0, 4)} <span className="text-slate-100">{n.av}</span>/{n.all}
            </span>
          </Tip>
        ))}
        {openEm > 0 && (
          <span className="flash inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-white">
            <Siren size={12} aria-hidden /> {openEm} EMERGENCY
          </span>
        )}
      </div>
      <div className="ml-auto flex shrink-0 items-center gap-1">
        <Health
          ok={stream === 'live'}
          icon={<Activity size={12} aria-hidden />}
          label="Live"
          detail={stream === 'live' ? 'This screen is receiving changes as they happen' : 'Not receiving changes: what you see may be out of date. Reconnecting…'}
        />
        {bv.configured ? (
          <Health
            ok={bv.connected && bv.failures.length === 0 && !psapTrouble(bv.psaps)}
            warn={bv.connected && (bv.failures.length > 0 || psapTrouble(bv.psaps))}
            icon={<Server size={12} aria-hidden />}
            label="BitVoice"
            detail={
              <div className="space-y-0.5">
                <div>{bv.connected ? `Event stream connected, last event ${bvAge} ago (${bv.events} events)` : `Not connected${bv.lastError ? ': ' + bv.lastError : ''}`}</div>
                <div>Last EIDO {bv.lastEido ? ago(bv.lastEido, now) + ' ago' : 'never'}{bv.lastPush ? ` · last pushed ${ago(bv.lastPush, now)} ago` : ''}</div>
                {(bv.psaps ?? []).map((p) => (
                  <div key={p.uri} className={psapTrouble([p]) ? 'text-amber-300' : 'text-slate-300'}>
                    {p.name || p.uri}: queue {p.queue || 'unknown'} {p.max ? `${p.length}/${p.max}` : ''} · service {p.service || 'unknown'}{p.reason ? ` (${p.reason})` : ''}
                  </div>
                ))}
                {bv.emergencyMode && <div className="text-rose-300">BitVoice emergency mode ON{bv.emergencyModeBy ? ` (by ${bv.emergencyModeBy})` : ''}</div>}
                {bv.failures.slice(0, 3).map((f, i) => (
                  <div key={i} className="text-amber-300">BitVoice could not deliver {f.what} {ago(f.at, now)} ago: {f.error}</div>
                ))}
              </div>
            }
          />
        ) : null}
        {bv.configured && (
          <Health
            ok={!!bv.lastRadio && now - ms(bv.lastRadio) < 15 * 60000}
            warn
            icon={<Radio size={12} aria-hidden />}
            label="Radio"
            detail={bv.lastRadio ? `Last radio event ${ago(bv.lastRadio, now)} ago` : 'No radio event received yet'}
          />
        )}
        {(h.ali.listening || h.ali.error) && (
          <Health
            ok={!!h.ali.listening}
            icon={<Phone size={12} aria-hidden />}
            label="ALI"
            detail={h.ali.error ? `ALI listener failed: ${h.ali.error}` : `Listening on ${h.ali.listening}; ${h.ali.count} spills, last ${h.ali.last ? ago(h.ali.last, now) + ' ago' : 'never'}`}
          />
        )}
        <Health ok={h.tiles} warn icon={<MapIcon size={12} aria-hidden />} label="Map" detail={h.tiles ? 'Map tiles configured and cached locally' : 'No map tiles configured: units and calls are still drawn (Admin › Settings)'} />
        <Tip text={`Ledger: ${h.ledger} events, hash-chained`} side="bottom">
          <span className="inline-flex h-6 items-center gap-1 px-1 font-mono text-[10px] text-slate-500">
            <Database size={11} aria-hidden />
            {h.ledger}
          </span>
        </Tip>
        <BreakButton view={view} />
        <RoadmapButtons view={view} />
        <PublicAlertsButton view={view} />
        {onScreens && (
          <button className="btn btn-ghost px-1.5" onClick={onScreens} aria-label="Screens: spread this position over monitors" title="Screens: spread this position over monitors (Ctrl+Shift+S)">
            <MonitorCheck size={14} aria-hidden />
          </button>
        )}
        {can.supervise(me) && (
          <Link to="/board" className="btn btn-ghost px-1.5" aria-label="Supervisor board" title="Supervisor board"><LayoutDashboard size={14} aria-hidden /></Link>
        )}
        <button className="btn btn-ghost px-1.5" onClick={nextTheme} aria-label={`Theme: ${theme}. Change theme`} title={`Theme: ${theme}`}>
          {theme === 'light' ? <Sun size={14} aria-hidden /> : <Moon size={14} aria-hidden />}
        </button>
        {can.supervise(me) && (
          <Link to="/admin" className="btn btn-ghost px-1.5" aria-label="Administration" title="Administration">
            <Cog size={14} aria-hidden />
          </Link>
        )}
        {view.health.sites && view.health.sites.length > 0 && (
          <span className={cx('chip', view.health.sites.every((x) => x.ok) ? 'bg-emerald-500/10 text-emerald-300' : 'bg-rose-500/20 text-rose-200')}
            title={view.health.sites.map((x) => `${x.name}: ${x.ok ? 'linked' : x.error ?? 'not linked'}`).join('\n')}>
            {view.health.site} · {view.health.sites.filter((x) => x.ok).length}/{view.health.sites.length} sites
          </span>
        )}
        <Link to="/rms" className="btn btn-ghost px-1.5" aria-label="Incident reports" title="Incident reports (RMS)"><FileText size={14} aria-hidden /></Link>
        <Link to="/help?a=console" className="btn btn-ghost px-1.5" aria-label="Help: the manual" title="Help: the manual"><CircleHelp size={14} aria-hidden /></Link>
        <button
          className="btn btn-ghost px-1.5"
          aria-label="Sign out"
          title="Sign out"
          onClick={async () => {
            await api.post('/auth/logout').catch(() => {})
            window.dispatchEvent(new Event('bd:signedout'))
          }}
        >
          <LogOut size={14} aria-hidden />
        </button>
      </div>
    </header>
  )
}

// psapTrouble: a PSAP the ESInet routes to whose queue or service is not normal (NENA state events).
function psapTrouble(ps?: { queue?: string; service?: string }[]) {
  return (ps ?? []).some((p) => (p.queue && p.queue !== 'Active') || (p.service && p.service !== 'Normal'))
}

/** The contacts directory, the schedule and the papers to serve, one click from every console. */
function RoadmapButtons({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const [open, setOpen] = useState<'' | 'dir' | 'sched'>('')
  const police = me && (me.duty === 'police' || me.duty === 'dispatcher' || me.duty === 'records' || me.role === 'supervisor' || me.role === 'admin')
  return (
    <>
      <button className="btn btn-ghost px-1.5" onClick={() => setOpen('dir')} aria-label="Contacts directory" title="Contacts directory and who is on call (DIR)"><BookUser size={14} aria-hidden /></button>
      <button className="btn btn-ghost px-1.5" onClick={() => setOpen('sched')} aria-label="Scheduled calls and events" title="Scheduled calls and special events"><CalendarClock size={14} aria-hidden /></button>
      {police && <Link to="/papers" className="btn btn-ghost px-1.5" aria-label="Warrants and papers" title="Warrants and papers to serve"><Gavel size={14} aria-hidden /></Link>}
      {open === 'dir' && <DirectoryDialog onClose={() => setOpen('')} />}
      {open === 'sched' && <ScheduleDialog view={view} onClose={() => setOpen('')} />}
    </>
  )
}
