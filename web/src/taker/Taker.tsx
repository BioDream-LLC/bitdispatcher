import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { toast } from 'sonner'
import { AlertTriangle, BookOpen, Car, ClipboardList, History as HistoryIcon, MessageSquare, Search, ShieldAlert, Users, FileClock, PhoneCall, Eraser, Send, Crosshair, Plus, X } from 'lucide-react'
import { api, errText } from '../api'
import { store, useStore } from '../store'
import { cx, clock, ms, phone, prio, typeName, useNow } from '../util'
import type { Call, CallDraftView, Dup, Incident, View } from '../types'
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
import { KeyMapHost } from '../keys'

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
  // Working calls: each draft in its own tab, kept on the server with its owner (calldrafts.go).
  const [slots, setSlots] = useState<Slot[]>([])
  const [active, setActive] = useState('')
  const [maxSlots, setMaxSlots] = useState(1)
  const pending = useRef<Record<string, number>>({})
  const [tab, setTab] = useState<TabId>('entry')
  const [dups, setDups] = useState<Dup[]>([])
  const [busy, setBusy] = useState(false)
  const comment = useRef<HTMLTextAreaElement>(null)
  const now = useNow()
  const activeRef = useRef('')
  activeRef.current = active
  const submittedRef = useRef<Set<string>>(new Set())
  const unsaved = useRef<Record<string, Draft>>({})
  const putNow = useCallback((id: string, keepalive = false) => {
    const next = unsaved.current[id]
    if (!next) return
    delete unsaved.current[id]
    window.clearTimeout(pending.current[id])
    // keepalive lets the last keystrokes reach the server even as the page goes away.
    fetch(`/api/drafts/${id}`, { method: 'PUT', body: JSON.stringify(next), keepalive, credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-BitDispatcher': '1' } }).catch(() => {})
  }, [])
  const putLater = useCallback((id: string, next: Draft) => {
    if (!id || submittedRef.current.has(id)) return
    unsaved.current[id] = next
    window.clearTimeout(pending.current[id])
    pending.current[id] = window.setTimeout(() => putNow(id), 300)
  }, [putNow])
  useEffect(() => {
    const flush = () => Object.keys(unsaved.current).forEach((id) => putNow(id, true))
    window.addEventListener('pagehide', flush)
    window.addEventListener('beforeunload', flush)
    return () => {
      window.removeEventListener('pagehide', flush)
      window.removeEventListener('beforeunload', flush)
    }
  }, [putNow])
  const update = useCallback((patch: Partial<Draft> | ((d: Draft) => Draft)) => {
    setD((cur) => {
      const next = typeof patch === 'function' ? patch(cur) : { ...cur, ...patch }
      saveDraft(next)
      putLater(activeRef.current, next)
      return next
    })
  }, [putLater])
  // The person's working calls, recovered at any console.
  useEffect(() => {
    let live = true
    api.get<{ items: CallDraftView[]; max: number }>('/drafts').then(async (r) => {
      if (!live) return
      setMaxSlots(r.max)
      if (r.items.length > 0) {
        const ss = r.items.map((x) => ({ id: x.id, draft: { ...emptyDraft(), ...((x.content as Partial<Draft>) ?? {}) } }))
        const keep = sessionStorage.getItem('bd.taker.active')
        const pick = ss.find((x) => x.id === keep) ?? ss[0]
        setSlots(ss)
        setActive(pick.id)
        setD(pick.draft)
        saveDraft(pick.draft)
        return
      }
      const cur = loadDraft()
      const x = await api.post<{ id: string }>('/drafts', { call: cur.call && !cur.number ? cur.call : undefined, content: cur })
      if (!live) return
      setSlots([{ id: x.id, draft: cur }])
      setActive(x.id)
    }).catch(() => { /* working calls are off for this person: one entry, as before */ })
    return () => { live = false }
  }, [])
  useEffect(() => {
    if (active) sessionStorage.setItem('bd.taker.active', active)
  }, [active])
  /** Moves to another working call, keeping this one exactly as it is. */
  const switchTo = useCallback((id: string) => {
    if (id === activeRef.current) return
    const t = slots.find((x) => x.id === id)
    if (!t) return
    putNow(activeRef.current)
    setSlots(slots.map((x) => (x.id === activeRef.current ? { ...x, draft: d } : x)))
    setD(t.draft)
    saveDraft(t.draft)
    setActive(id)
    setDups([])
    store.select(t.draft.number ?? null)
    store.selectCall(t.draft.call ?? null)
  }, [d, slots, putNow])
  /** Opens a new working call (within the agency's limit), leaving the others as they are. */
  const newCall = useCallback(async (start?: Draft): Promise<boolean> => {
    const content = start ?? emptyDraft()
    try {
      const x = await api.post<{ id: string }>('/drafts', { call: content.call, content })
      setSlots((ss) => [...ss.map((s) => (s.id === activeRef.current ? { ...s, draft: d } : s)), { id: x.id, draft: content }])
      setActive(x.id)
      setD(content)
      saveDraft(content)
      setDups([])
      setTab('entry')
      if (!content.call) store.selectCall(null)
      store.select(null)
      return true
    } catch (e) {
      toast.error(errText(e))
      return false
    }
  }, [d])
  /** Closes a working call's tab: a submitted one goes; one with information in it is discarded only with a reason. */
  const closeSlot = useCallback(async (id: string) => {
    const s = slots.find((x) => x.id === id)
    if (!s) return
    const draft = id === active ? d : s.draft
    if (!submittedRef.current.has(id)) {
      const populated = dirty(draft)
      let why = ''
      if (populated) {
        if (!confirm(`Discard working call ${id}? What was typed in it is lost (the ledger records that it was discarded).`)) return
        why = prompt('Why is it discarded? (a duplicate, the caller hung up…)')?.trim() ?? ''
        if (!why) return
      }
      try {
        await api.post(`/drafts/${id}/close`, { how: 'discarded', why, populated })
      } catch (e) {
        return void toast.error(errText(e))
      }
    }
    const rest = slots.filter((x) => x.id !== id)
    setSlots(rest)
    if (id === active) {
      if (rest.length) {
        setActive(rest[0].id)
        setD(rest[0].draft)
        saveDraft(rest[0].draft)
      } else {
        setActive('')
        setD(emptyDraft())
        newCall(emptyDraft())
      }
    }
  }, [slots, active, d, newCall])
  const free = slots.length < maxSlots
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
    // A call already being taken in another tab: go to it.
    const there = slots.find((x) => x.id !== active && x.draft.call === c.id)
    if (there) return switchTo(there.id)
    if (dirty(d) && slots.length > 0 && free) {
      newCall(fromCall(c))
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
    // Next call never silently clears a working call: with a tab free it opens a new one.
    if (slots.length > 0 && (submittedRef.current.has(active) || dirty(d) && free)) {
      if (submittedRef.current.has(active)) {
        const id = active
        if (slots.length - 1 < maxSlots) {
          setSlots((ss) => ss.filter((x) => x.id !== id))
          newCall()
          return
        }
      } else {
        newCall()
        return
      }
    }
    if (dirty(d) && !confirm('Clear this entry? It has not been submitted.')) return
    if (slots.length > 0 && dirty(d)) {
      // No tab free: this working call is discarded (recorded) and a new one takes its place.
      const id = active
      api.post(`/drafts/${id}/close`, { how: 'discarded', why: 'cleared for the next call', populated: true })
        .then(() => { setSlots((ss) => ss.filter((x) => x.id !== id)); return newCall() })
        .catch((e) => toast.error(errText(e)))
      return
    }
    update(() => emptyDraft())
    setDups([])
    store.selectCall(null)
    store.select(null)
    setTab('entry')
  }, [d, update, slots, active, free, maxSlots, newCall])

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
      // The working call is now the incident: only its own tab changes.
      if (active && !submittedRef.current.has(active)) {
        submittedRef.current.add(active)
        api.post(`/drafts/${active}/close`, { how: 'submitted', incident: n, populated: true }).catch(() => {})
      }
      toast.success(`${n} submitted: dispatch has it. Keep typing; F2 clears for the next call.`)
      comment.current?.focus()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }, [busy, d, inc, raw, update, active])

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
      <KeyMapHost screen="taker" handlers={{
        clear, submit, rebid, narrative: () => comment.current?.focus(), search: () => setTab('search'), queries: () => setTab('search'),
        recall: () => window.dispatchEvent(new CustomEvent('bd:search', { detail: '' })), previous: () => setTab('previous'), premise: () => setTab('hazards'),
        comments: () => setTab('comments'), audit: () => setTab('history'), email: () => window.dispatchEvent(new CustomEvent('bd:open-comm', { detail: '' })),
        initiate: () => newCall(), nextPage: () => { const i = slots.findIndex((x) => x.id === active); if (slots.length > 1) switchTo(slots[(i + 1) % slots.length].id) },
        prevPage: () => { const i = slots.findIndex((x) => x.id === active); if (slots.length > 1) switchTo(slots[(i - 1 + slots.length) % slots.length].id) },
        display911: () => setTab('entry'),
      }} />
      <main className="grid min-h-0 flex-1 grid-cols-[minmax(300px,20%)_1fr_minmax(340px,25%)] gap-px bg-slate-800">
        <div className="flex min-h-0 flex-col bg-slate-950">
          <Boundary name="calls"><Queue view={view} callsOnly onCreateFromCall={(c: Call) => store.selectCall(c.id)} onNew={clear} /></Boundary>
          <MyIncidents view={raw} current={d.number} onPick={(n) => { const i = raw.incidents.find((x) => x.number === n); if (i) update(() => fromIncident(i)) }} />
        </div>
        <section className="flex min-h-0 min-w-0 flex-row bg-slate-950" aria-label="Call entry and working calls">
        {slots.length > 0 && (
          <WorkingCalls view={raw} slots={slots.map((x) => (x.id === active ? { ...x, draft: d } : x))} active={active} max={maxSlots} now={now} submitted={submittedRef.current}
            onPick={switchTo} onNew={() => newCall()} onClose={closeSlot} />
        )}
        <div className="flex min-h-0 min-w-0 flex-1 flex-col" aria-label="Call entry" role="region">
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

type Slot = { id: string; draft: Draft }

/** The working calls, in tabs to the left of call entry: each its own draft. The active one is marked in
 * words and by its border, not by colour alone. */
function WorkingCalls({ view, slots, active, max, now, submitted, onPick, onNew, onClose }: {
  view: View; slots: Slot[]; active: string; max: number; now: number; submitted: Set<string>
  onPick: (id: string) => void; onNew: () => void; onClose: (id: string) => void
}) {
  return (
    <nav className="flex w-36 shrink-0 flex-col gap-1 border-r border-slate-800 bg-slate-900/60 p-1" aria-label="Working calls">
      <div className="flex items-center justify-between px-1 text-[10px] font-bold uppercase tracking-wider text-slate-400">
        <span>Working calls</span><span className="font-mono">{slots.length}/{max}</span>
      </div>
      <div role="tablist" aria-orientation="vertical" aria-label="Working calls" className="flex flex-col gap-1">
        {slots.map((s, i) => {
          const x = s.draft
          const on = s.id === active
          const what = x.number ? x.number : x.type ? typeName(view, x.type) : 'New call'
          const where = x.address || (x.callback ? phone(x.callback) : '')
          const attention = !x.number && (!x.type || !x.address)
          const done = submitted.has(s.id) || !!x.number
          return (
            <div key={s.id} className={cx('group relative rounded border-2 text-left', on ? 'border-sky-300 bg-slate-800' : 'border-slate-700 bg-slate-900 hover:border-slate-500')}>
              <button role="tab" aria-selected={on} className="w-full px-1.5 py-1 text-left" onClick={() => onPick(s.id)}
                aria-label={`Working call ${i + 1}${on ? ' (active)' : ''}: ${what}${where ? ', ' + where : ''}${attention ? ', needs a type or address' : ''}${done ? ', submitted' : ''}`}>
                <div className="flex items-center gap-1 text-[10px] font-bold text-slate-300">
                  <span className="font-mono">{i + 1}</span>
                  {on && <span className="chip h-4 bg-sky-500/30 px-1 text-[9px] text-sky-100">ACTIVE</span>}
                  {done && <span className="chip h-4 bg-emerald-500/20 px-1 text-[9px] text-emerald-200">SENT</span>}
                  {attention && <span className="text-amber-300" title="Needs a type or address">!</span>}
                  <span className="ml-auto font-mono font-normal text-slate-500">{clock(x.started, now)}</span>
                </div>
                <div className="truncate text-[11px] font-semibold text-slate-100">{what}</div>
                <div className="truncate text-[10px] text-slate-400">{where || '—'}</div>
              </button>
              <button className="btn btn-ghost absolute right-0.5 top-0.5 hidden h-4 px-0.5 group-hover:inline-flex group-focus-within:inline-flex" onClick={() => onClose(s.id)} aria-label={`Close working call ${i + 1}`}><X size={10} aria-hidden /></button>
            </div>
          )
        })}
      </div>
      <button className="btn h-7 gap-1 text-[11px]" onClick={onNew} disabled={slots.length >= max} title={slots.length >= max ? `The agency allows ${max} at a time` : 'Start another call without losing this one'}>
        <Plus size={12} aria-hidden /> New working call
      </button>
    </nav>
  )
}
