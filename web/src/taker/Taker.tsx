import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { toast } from 'sonner'
import { AlertTriangle, BookOpen, Car, ClipboardList, History as HistoryIcon, MessageSquare, Search, ShieldAlert, Users, FileClock, PhoneCall, Eraser, Send, Crosshair } from 'lucide-react'
import { api, errText } from '../api'
import { store, useStore } from '../store'
import { cx, clock, ms, phone, prio, typeName, useNow } from '../util'
import type { Call, Dup, Incident, View } from '../types'
import StatusBar from '../console/StatusBar'
import EmergencyBanner from '../console/EmergencyBanner'
import { CareBanner, OfflineBar, TDoSBanner, TrainingBar } from '../console/Dream'
import Queue from '../console/Queue'
import { CallBox } from '../console/IncidentPanel'
import Boundary from '../Boundary'
import { applyLens } from '../lens'
import { callerAt, dirty, dupKey, emptyDraft, fromCall, fromIncident, loadDraft, location, noteText, saveDraft, type Draft } from './draft'
import EntryTab from './EntryTab'
import { PersonsTab, VehiclesTab } from './People'
import { HazardsTab, PreviousList, SearchTab, useAlerts } from './Lookup'
import PlaceViews from '../views/PlaceViews'
import { CommentsTab, HistoryTab } from './Narrative'
import ResourcesTab, { useResources } from './Resources'

const MapView = lazy(() => import('../console/MapView'))

export const TABS = [
  { id: 'entry', label: 'Call Entry', icon: PhoneCall },
  { id: 'persons', label: 'Persons', icon: Users },
  { id: 'vehicles', label: 'Vehicles', icon: Car },
  { id: 'previous', label: 'Previous', icon: FileClock },
  { id: 'hazards', label: 'Hazards', icon: ShieldAlert },
  { id: 'search', label: 'CAD Search', icon: Search },
  { id: 'comments', label: 'Comments', icon: MessageSquare },
  { id: 'history', label: 'History', icon: HistoryIcon },
  { id: 'resources', label: 'Resources', icon: BookOpen },
] as const
export type TabId = (typeof TABS)[number]['id']

/** The call-taker's workspace: one call, one incident, nine tabs across the top with only the chosen one
 * shown, the call and the narrative always beside them. F2 clears for the next call, F4 submits (and
 * updates after), F6 rebids, F8 writes a comment, Alt+1–9 changes tab. */
export default function Taker() {
  const raw = useStore((s) => s.view)
  const lens = useStore((s) => s.lens)
  const view = useMemo(() => (raw ? applyLens(raw, lens) : null), [raw, lens])
  const selCall = useStore((s) => s.selectedCall)
  const [d, setD] = useState<Draft>(loadDraft)
  const [tab, setTab] = useState<TabId>('entry')
  const [dups, setDups] = useState<Dup[]>([])
  const [busy, setBusy] = useState(false)
  const comment = useRef<HTMLTextAreaElement>(null)
  const now = useNow()
  const update = useCallback((patch: Partial<Draft> | ((d: Draft) => Draft)) => {
    setD((cur) => {
      const next = typeof patch === 'function' ? patch(cur) : { ...cur, ...patch }
      saveDraft(next)
      return next
    })
  }, [])
  const inc = raw?.incidents.find((i) => i.number === d.number)
  const call = raw?.calls.find((c) => c.id === (d.call ?? inc?.calls[0]))
  // Choosing a call in the list takes it, unless an entry is under way for another.
  useEffect(() => {
    if (!selCall || !raw || selCall === d.call) return
    const c = raw.calls.find((x) => x.id === selCall)
    if (!c) return
    if (c.incident) {
      const i = raw.incidents.find((x) => x.number === c.incident)
      update(() => (i ? { ...fromIncident(i), call: c.id } : { ...emptyDraft(), call: c.id, number: c.incident }))
      return
    }
    if (dirty(d) && !confirm('Leave the entry you are writing for this call? (Its notes are kept until you clear it with F2.)')) {
      store.selectCall(d.call ?? null)
      return
    }
    update(() => fromCall(c))
    setTab('entry')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selCall])
  // The incident the dispatcher works keeps in step: the selection follows it on other screens.
  useEffect(() => {
    if (d.number) store.select(d.number)
  }, [d.number])

  const clear = useCallback(() => {
    if (dirty(d) && !confirm('Clear this entry? It has not been submitted.')) return
    update(() => emptyDraft())
    setDups([])
    store.selectCall(null)
    store.select(null)
    setTab('entry')
  }, [d, update])

  const submit = useCallback(async () => {
    if (busy) return
    if (d.number) {
      // After submission F4 saves what changed on the entry tab: type, priority, location, caller.
      setBusy(true)
      try {
        const loc = location(d)
        await api.patch(`/incidents/${d.number}`, { type: d.type || undefined, priority: d.priority || undefined, location: loc, caller: { name: d.callerName, callback: d.callback } })
        await api.put(`/incidents/${d.number}/modifiers`, { modifiers: d.modifiers })
        if (d.callerElsewhere) await api.put(`/incidents/${d.number}/caller-location`, { location: callerAt(d) })
        if (d.verified && inc && !inc.locVerified) await api.post(`/incidents/${d.number}/verify`)
        toast.success(`${d.number} updated`)
      } catch (e) {
        toast.error(errText(e))
      } finally {
        setBusy(false)
      }
      return
    }
    if (!d.type) {
      setTab('entry')
      toast.error('Choose the call type first')
      document.getElementById('ct-taker')?.focus()
      return
    }
    setBusy(true)
    try {
      // Before a second incident: is it one already open?
      const loc = location(d)
      const q = new URLSearchParams({ type: d.type, address: d.address, callback: d.callback })
      if (loc?.lat != null && loc.lon != null) { q.set('lat', String(loc.lat)); q.set('lon', String(loc.lon)) }
      const found = await api.get<Dup[]>(`/duplicates?${q}`).catch(() => [] as Dup[])
      if (found.length && d.dupsSeen !== dupKey(found.map((x) => x.incident))) {
        setDups(found)
        update({ dupsSeen: dupKey(found.map((x) => x.incident)) })
        setTab('entry')
        toast.warning('Possible same emergency: check before making a second incident (F4 again to create anyway)')
        return
      }
      const r = await api.post<{ number: string }>('/incidents', {
        type: d.type, priority: d.priority || undefined, location: loc, caller: { name: d.callerName || undefined, callback: d.callback || undefined },
        call: d.call && !raw?.calls.find((c) => c.id === d.call)?.incident ? d.call : undefined, modifiers: d.modifiers, callerAt: callerAt(d), locVerified: d.verified,
      })
      const n = r.number
      const at = new Date().toISOString()
      // What was entered before submission follows, each with its own time.
      for (const note of d.notes) await api.post(`/incidents/${n}/notes`, { text: noteText(note, at) })
      for (const p of d.persons) await api.post(`/incidents/${n}/persons`, { ...p, id: '' })
      for (const v of d.vehicles) await api.post(`/incidents/${n}/vehicles`, { ...v, id: '' })
      for (const c of d.contacts) await api.post(`/incidents/${n}/contacts`, { ...c, id: '' })
      update({ number: n, notes: [], persons: [], vehicles: [], contacts: [] })
      setDups([])
      toast.success(`${n} submitted: dispatch has it. Keep typing; F2 clears for the next call.`)
      comment.current?.focus()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }, [busy, d, inc, raw, update])

  const attachTo = async (num: string) => {
    try {
      if (d.call) await api.post(`/calls/${encodeURIComponent(d.call)}/attach`, { incident: num })
      for (const note of d.notes) await api.post(`/incidents/${num}/notes`, { text: noteText(note, new Date().toISOString()) })
      for (const p of d.persons) await api.post(`/incidents/${num}/persons`, { ...p, id: '' })
      for (const v of d.vehicles) await api.post(`/incidents/${num}/vehicles`, { ...v, id: '' })
      update({ number: num, notes: [], persons: [], vehicles: [], contacts: [] })
      setDups([])
      toast.success(`This call is now on ${num}`)
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const rebid = useCallback(async () => {
    if (!d.call) return toast.error('No call to rebid')
    try {
      await api.post(`/calls/${encodeURIComponent(d.call)}/rebid`)
      toast.success('Rebid asked: a newer location shows here when it comes')
    } catch (e) {
      toast.error(errText(e))
    }
  }, [d.call])

  // The workspace's keys.
  useEffect(() => {
    const on = (e: KeyboardEvent) => {
      if (document.querySelector('[role=dialog]')) return
      if (e.key === 'F2') { e.preventDefault(); clear() }
      else if (e.key === 'F4' || (e.ctrlKey && e.key === 'Enter')) { e.preventDefault(); submit() }
      else if (e.key === 'F6') { e.preventDefault(); rebid() }
      else if (e.key === 'F8') { e.preventDefault(); comment.current?.focus() }
      else if (e.key === 'F3') { e.preventDefault(); setTab('search') }
      else if (e.altKey && /^[1-9]$/.test(e.key)) { e.preventDefault(); setTab(TABS[Number(e.key) - 1].id) }
    }
    window.addEventListener('keydown', on)
    return () => window.removeEventListener('keydown', on)
  }, [clear, submit, rebid])

  const alerts = useAlerts(view, d, inc)
  const placeAt: [number, number] | null = inc?.location?.lat != null && inc.location.lon != null ? [inc.location.lat, inc.location.lon]
    : d.lat && d.lon && !isNaN(+d.lat) && !isNaN(+d.lon) ? [+d.lat, +d.lon] : null
  const res = useResources(view, d, inc, call, tab === 'resources')
  if (!view || !raw) return <div className="grid h-full place-items-center text-sm text-slate-500">Loading the call queue…</div>
  const counts: Partial<Record<TabId, number | string>> = {
    persons: (inc?.persons ?? d.persons).filter((p) => !p.removed).length || undefined,
    vehicles: (inc?.vehicles ?? d.vehicles).filter((v) => !v.removed).length || undefined,
    previous: alerts.prev.length || undefined,
    hazards: alerts.count || undefined,
    comments: (inc ? inc.timeline.filter((t) => t.kind === 'note' || t.kind === 'priority-note').length : d.notes.length) || undefined,
    resources: res.fresh ? 'new' : res.count || undefined,
  }
  return (
    <div className="flex h-full min-h-[600px] flex-col overflow-hidden bg-slate-950">
      <Boundary name="status"><StatusBar view={view} screen="full" onScreens={() => {}} /></Boundary>
      <Boundary name="training"><TrainingBar view={raw} /></Boundary>
      <Boundary name="offline"><OfflineBar view={raw} /></Boundary>
      <Boundary name="emergency"><EmergencyBanner view={raw} /></Boundary>
      <Boundary name="tdos"><TDoSBanner view={raw} /></Boundary>
      <Boundary name="care"><CareBanner view={raw} /></Boundary>
      <main className="grid min-h-0 flex-1 grid-cols-[minmax(300px,20%)_1fr_minmax(340px,25%)] gap-px bg-slate-800">
        <div className="flex min-h-0 flex-col bg-slate-950">
          <Boundary name="calls"><Queue view={view} callsOnly onCreateFromCall={(c: Call) => store.selectCall(c.id)} onNew={clear} /></Boundary>
          <MyIncidents view={raw} current={d.number} onPick={(n) => { const i = raw.incidents.find((x) => x.number === n); if (i) update(() => fromIncident(i)) }} />
        </div>
        <section className="flex min-h-0 min-w-0 flex-col bg-slate-950" aria-label="Call entry">
          <Header view={raw} d={d} inc={inc} call={call} now={now} busy={busy} onSubmit={submit} onClear={clear} onRebid={rebid} alerts={alerts.critical} onAlerts={() => setTab('hazards')} />
          <div role="tablist" aria-label="Call entry tabs" className="flex shrink-0 gap-px overflow-x-auto border-b border-slate-800 bg-slate-900/70 px-1">
            {TABS.map((t, i) => (
              <button key={t.id} role="tab" aria-selected={tab === t.id} aria-controls={'tab-' + t.id} id={'tabbtn-' + t.id} onClick={() => setTab(t.id)}
                className={cx('flex shrink-0 items-center gap-1 border-b-2 px-2 py-1.5 text-[11px] font-semibold', tab === t.id ? 'border-sky-400 text-slate-50' : 'border-transparent text-slate-400 hover:text-slate-200')}
                title={`Alt+${i + 1}`}>
                <t.icon size={13} aria-hidden /> {t.label}
                {counts[t.id] != null && (
                  <span className={cx('chip h-4 px-1 text-[10px]', t.id === 'hazards' ? 'bg-rose-500 text-white' : counts[t.id] === 'new' ? 'bg-sky-500 text-white' : 'bg-slate-700 text-slate-200')}>{counts[t.id]}</span>
                )}
              </button>
            ))}
          </div>
          <div id={'tab-' + tab} role="tabpanel" aria-labelledby={'tabbtn-' + tab} className="min-h-0 flex-1 overflow-y-auto p-3">
            <Boundary key={tab} name={tab}>
              {tab === 'entry' && (
                <EntryTab view={raw} d={d} inc={inc} call={call} update={update} dups={dups} onAttach={attachTo} onDismissDups={() => setDups([])}
                  map={<Suspense fallback={null}><MapView view={view} rec={null} onCreateAt={(lat, lon, address) => update({ lat: lat.toFixed(6), lon: lon.toFixed(6), address: address ?? d.address, verified: !!address })} /></Suspense>} />
              )}
              {tab === 'persons' && <PersonsTab view={raw} d={d} inc={inc} call={call} update={update} />}
              {tab === 'vehicles' && <VehiclesTab view={raw} d={d} inc={inc} call={call} update={update} />}
              {tab === 'previous' && <PreviousList alerts={alerts} />}
              {tab === 'hazards' && <HazardsTab alerts={alerts} />}
              {tab === 'hazards' && placeAt && (raw?.placeViews || alerts.premises.some((p) => p.photos?.length)) && (
                <section className="mt-4"><h3 className="label">Views of the place</h3><PlaceViews lat={placeAt[0]} lon={placeAt[1]} premises={alerts.premises} /></section>
              )}
              {tab === 'search' && <SearchTab />}
              {tab === 'comments' && <CommentsTab view={raw} d={d} inc={inc} update={update} />}
              {tab === 'history' && <HistoryTab view={raw} inc={inc} />}
              {tab === 'resources' && <ResourcesTab res={res} inc={inc} />}
            </Boundary>
          </div>
        </section>
        <aside className="flex min-h-0 flex-col bg-slate-950" aria-label="The call and the narrative">
          <div className="min-h-0 flex-1 overflow-y-auto p-2">
            {call ? <CallBox call={call} view={raw} now={now} /> : <p className="p-2 text-xs text-slate-500">No call taken. Choose one from the list, or type an entry without one (a walk-in, a radio report).</p>}
          </div>
          <QuickComment ref_={comment} d={d} inc={inc} update={update} />
        </aside>
      </main>
    </div>
  )
}

function Header({ view, d, inc, call, now, busy, onSubmit, onClear, onRebid, alerts, onAlerts }: {
  view: View; d: Draft; inc?: Incident; call?: Call; now: number; busy: boolean; onSubmit: () => void; onClear: () => void; onRebid: () => void; alerts: string[]; onAlerts: () => void
}) {
  const units = (inc?.units ?? []).filter((a) => !a.cleared)
  return (
    <div className="shrink-0 border-b border-slate-800 bg-slate-900/40 px-3 py-1.5">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        {inc ? (
          <>
            <span className={cx('chip font-mono', prio(inc.priority).bg, prio(inc.priority).fg)}>P{inc.priority}</span>
            <span className="font-mono text-sm font-bold text-slate-50">{inc.number}</span>
            <span className="text-slate-200">{typeName(view, inc.type)}</span>
            <span className={cx('chip uppercase', inc.status === 'pending' ? 'bg-amber-500/20 text-amber-200' : inc.status === 'active' ? 'bg-emerald-500/20 text-emerald-200' : 'bg-slate-700 text-slate-300')}>{inc.status}</span>
            {units.length > 0 && <span className="font-mono text-slate-300">{units.map((a) => `${a.unit} ${a.status.slice(0, 2).toUpperCase()}`).join(' · ')}</span>}
            <span className="text-slate-500">submitted {clock(inc.created, now)} ago</span>
          </>
        ) : (
          <span className="chip bg-slate-800 text-slate-300">Not submitted</span>
        )}
        {call && <span className="font-mono text-slate-400"><PhoneCall size={11} className="mr-0.5 inline" aria-hidden />{phone(call.callback)} {clock(call.start, call.end ? ms(call.end) : now)}</span>}
        <span className="ml-auto flex items-center gap-1">
          {call && <button className="btn h-7 text-xs" onClick={onRebid} disabled={!call.canRebid} title="Ask for the caller's location again"><Crosshair size={12} aria-hidden /> Rebid <span className="kbd">F6</span></button>}
          <button className="btn h-7 text-xs" onClick={onClear}><Eraser size={12} aria-hidden /> Next call <span className="kbd">F2</span></button>
          <button className="btn btn-go h-7 text-xs" onClick={onSubmit} disabled={busy}>
            <Send size={12} aria-hidden /> {inc ? 'Update' : 'Submit'} <span className="kbd">F4</span>
          </button>
        </span>
      </div>
      {alerts.length > 0 && (
        <button className="mt-1 flex w-full items-center gap-1.5 rounded border border-rose-800 bg-rose-950/50 px-2 py-0.5 text-left text-[11px] font-semibold text-rose-100" onClick={onAlerts}>
          <AlertTriangle size={12} className="shrink-0 text-rose-300" aria-hidden /> <span className="truncate">{alerts.join(' · ')}</span> <span className="ml-auto shrink-0 text-rose-300">Hazards ›</span>
        </button>
      )}
      {inc?.newLocation && <NewLocation inc={inc} />}
      {!inc && d.type && location(d)?.lat == null && <p className="mt-0.5 text-[11px] text-amber-300">No map position yet: units cannot be ranked by distance.</p>}
    </div>
  )
}

function NewLocation({ inc }: { inc: Incident }) {
  const o = inc.newLocation!
  const decide = (accept: boolean) => api.post(`/incidents/${inc.number}/location`, { accept, why: accept ? 'newer caller location' : 'kept' }).then(() => toast.success(accept ? 'Incident location changed; dispatch is told' : 'Kept the incident location')).catch((e) => toast.error(errText(e)))
  return (
    <div className="mt-1 flex items-center gap-2 rounded border border-amber-700 bg-amber-950/40 px-2 py-1 text-[11px] text-amber-100" role="alert">
      <Crosshair size={12} aria-hidden /> Newer caller location from <b className="uppercase">{o.location.source ?? 'the call'}</b>{o.location.radius ? ` ±${Math.round(o.location.radius)} m` : ''}, {Math.round(o.metres)} m from the incident's.
      <button className="btn btn-go ml-auto h-6 px-2 text-[11px]" onClick={() => decide(true)}>Use it</button>
      <button className="btn h-6 px-2 text-[11px]" onClick={() => decide(false)}>Keep</button>
    </div>
  )
}

function MyIncidents({ view, current, onPick }: { view: View; current?: string; onPick: (n: string) => void }) {
  const me = useStore((s) => s.me)
  const mine = view.incidents.filter((i) => i.createdBy === me?.username && i.status !== 'closed').slice(0, 8)
  if (!mine.length) return null
  return (
    <div className="max-h-[30%] shrink-0 overflow-y-auto border-t border-slate-800">
      <div className="panel-head"><ClipboardList size={12} aria-hidden /> My open incidents</div>
      <ul className="divide-y divide-slate-800/70 text-xs">
        {mine.map((i) => (
          <li key={i.number}>
            <button className={cx('flex w-full items-center gap-1.5 px-2 py-1 text-left hover:bg-slate-800/60', i.number === current && 'bg-slate-800')} onClick={() => onPick(i.number)}>
              <span className={cx('chip font-mono', prio(i.priority).bg, prio(i.priority).fg)}>P{i.priority}</span>
              <span className="font-mono text-slate-200">{i.number.slice(-6)}</span>
              <span className="truncate text-slate-400">{typeName(view, i.type)}</span>
              <span className="ml-auto font-mono text-[10px] text-slate-500">{i.units.filter((a) => !a.cleared).map((a) => a.unit).join(' ') || i.status}</span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}

/** The comment field, always there: before submission it keeps notes with their time; after, it adds them
 * to the incident at once. */
function QuickComment({ ref_, d, inc, update }: { ref_: React.RefObject<HTMLTextAreaElement>; d: Draft; inc?: Incident; update: (p: Partial<Draft> | ((d: Draft) => Draft)) => void }) {
  const [text, setText] = useState('')
  const [priority, setPriority] = useState(false)
  const notes = inc ? inc.timeline.filter((t) => t.kind === 'note' || t.kind === 'priority-note').slice(-4) : d.notes.slice(-4).map((n) => ({ at: n.at, text: n.text, actor: 'you (not yet sent)', kind: 'note' }))
  const send = async () => {
    const t = text.trim()
    if (!t) return
    if (inc) {
      try {
        await api.post(`/incidents/${inc.number}/notes`, { text: t, priority })
        setText('')
        setPriority(false)
      } catch (e) {
        toast.error(errText(e))
      }
    } else {
      update((cur) => ({ ...cur, notes: [...cur.notes, { at: new Date().toISOString(), text: t }] }))
      setText('')
    }
  }
  return (
    <div className="shrink-0 border-t border-slate-800 bg-slate-900/50 p-2">
      <div className="mb-1 flex items-center gap-1 text-[10px] font-bold uppercase tracking-wider text-slate-400">
        <MessageSquare size={11} aria-hidden /> Narrative {inc ? <span className="normal-case tracking-normal text-slate-500">on {inc.number}</span> : <span className="normal-case tracking-normal text-amber-300">kept until submitted</span>}
      </div>
      <ul className="mb-1 max-h-24 space-y-0.5 overflow-y-auto text-[11px]">
        {notes.map((n, i) => (
          <li key={i} className={cx('text-slate-300', 'kind' in n && n.kind === 'priority-note' && 'font-semibold text-amber-200')}>
            <span className="font-mono text-[10px] text-slate-500">{new Date(n.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })} {n.actor}</span> {n.text}
          </li>
        ))}
      </ul>
      <textarea ref={ref_} className="input min-h-[52px] text-sm" placeholder="What the caller says (Enter adds it; Shift+Enter for a new line) — F8" aria-label="Add to the narrative" value={text}
        onChange={(e) => setText(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter' && !e.shiftKey && !e.ctrlKey) { e.preventDefault(); send() } }} />
      {inc && (
        <label className="mt-1 flex items-center gap-1.5 text-[11px] text-slate-400">
          <input type="checkbox" checked={priority} onChange={(e) => setPriority(e.target.checked)} /> Priority note: every dispatcher and unit sees it
        </label>
      )}
    </div>
  )
}

