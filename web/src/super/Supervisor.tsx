import { useEffect, useState } from 'react'
import { MapPin, ShieldAlert, Radio, ClipboardList, Users, ListChecks, Keyboard } from 'lucide-react'
import { api } from '../api'
import { cx } from '../util'
import type { PeriodsResponse } from './types'
import Exceptions from './Exceptions'
import Critical from './Critical'
import Phrases from './Phrases'
import ShiftIntel from './ShiftIntel'
import People from './People'
import Followups from './Followups'
import KeyTests from './KeyTests'

type TabKey = 'exceptions' | 'critical' | 'phrases' | 'shift' | 'people' | 'followups' | 'keytests'

const TABS: { key: TabKey; label: string; icon: typeof MapPin }[] = [
  { key: 'exceptions', label: 'Location exceptions', icon: MapPin },
  { key: 'critical', label: 'Critical incidents', icon: ShieldAlert },
  { key: 'phrases', label: 'Phrase alerts', icon: Radio },
  { key: 'shift', label: 'Shift Intelligence', icon: ClipboardList },
  { key: 'people', label: 'People', icon: Users },
  { key: 'followups', label: 'Follow-up', icon: ListChecks },
  { key: 'keytests', label: 'Key tests', icon: Keyboard },
]

// The Shift Intelligence tab and the People tab share one period selection.
type PeriodMode = { kind: 'period'; id: string } | { kind: 'range'; from: string; to: string }

/** Supervisor is the supervisor workspace: one page, a tab for each section. It takes no props; the
 * wiring engineer mounts it on a route. */
export default function Supervisor() {
  const [tab, setTab] = useState<TabKey>('exceptions')
  const [periods, setPeriods] = useState<PeriodsResponse | null>(null)
  const [mode, setMode] = useState<PeriodMode | null>(null)

  // Load the periods once, so both the Shift and People tabs start on the current period.
  useEffect(() => {
    let live = true
    api
      .get<PeriodsResponse>('/shift/periods')
      .then((r) => {
        if (!live) return
        setPeriods(r)
        if (r.periods.length > 0) setMode({ kind: 'period', id: r.periods[0].id })
      })
      .catch(() => {
        /* the individual tabs report their own load errors */
      })
    return () => {
      live = false
    }
  }, [])

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <header className="panel-head flex items-center gap-3 px-4 py-2">
        <a href="#/" className="btn btn-ghost h-6 px-1.5 text-xs">← Dispatch</a>
        <h1 className="text-sm font-semibold text-slate-100">Supervisor workspace</h1>
        <span className="text-xs text-slate-500">Every action here is recorded as you.</span>
      </header>

      <div role="tablist" aria-label="Supervisor sections" className="flex flex-wrap gap-1 border-b border-slate-800 px-3 py-2">
        {TABS.map((t) => {
          const Icon = t.icon
          const active = tab === t.key
          return (
            <button
              key={t.key}
              role="tab"
              id={`tab-${t.key}`}
              aria-selected={active}
              aria-controls={`panel-${t.key}`}
              tabIndex={active ? 0 : -1}
              className={cx('btn flex items-center gap-1.5', active ? 'btn-go' : 'btn-ghost')}
              onClick={() => setTab(t.key)}
            >
              <Icon size={14} aria-hidden />
              {t.label}
            </button>
          )
        })}
      </div>

      <main className="flex-1 overflow-y-auto p-4">
        <div role="tabpanel" id={`panel-${tab}`} aria-labelledby={`tab-${tab}`}>
          {tab === 'exceptions' && <Exceptions />}
          {tab === 'critical' && <Critical />}
          {tab === 'phrases' && <Phrases />}
          {tab === 'shift' && mode && <ShiftIntel mode={mode} periods={periods} setMode={setMode} />}
          {tab === 'people' && mode && (
            mode.kind === 'period'
              ? <People periodId={mode.id} />
              : <People periodId="" from={mode.from} to={mode.to} />
          )}
          {tab === 'followups' && <Followups />}
          {tab === 'keytests' && <KeyTests />}
          {(tab === 'shift' || tab === 'people') && !mode && (
            <p className="text-sm text-slate-500">Loading the operational periods…</p>
          )}
        </div>
      </main>
    </div>
  )
}
