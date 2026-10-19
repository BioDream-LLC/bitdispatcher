import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Command } from 'cmdk'
import { DndContext, PointerSensor, useSensor, useSensors } from '@dnd-kit/core'
import { manual } from './manual'
import NcicPanel from './rms/NcicPanel'
import type { Meta } from './rms/Rms'
import { toast } from 'sonner'
import { api, errText } from './api'
import { can, store, useStore } from './store'
import { cx, typeName } from './util'
import type { Call, Recommendation } from './types'
import StatusBar from './console/StatusBar'
import EmergencyBanner from './console/EmergencyBanner'
import Queue from './console/Queue'
import MapView from './console/MapView'
import IncidentPanel, { UnitDetail } from './console/IncidentPanel'
import Situation from './console/Situation'
import UnitBoard from './console/UnitBoard'
import CommandLine, { type CommandLineHandle } from './console/CommandLine'
import NewIncident, { type NewSeed } from './console/NewIncident'
import Boundary from './Boundary'
import StationBoard from './console/StationBoard'
import PoliceBoard from './console/PoliceBoard'
import MarkOutDialog from './console/MarkOut'
import ScreensDialog, { openAll } from './console/ScreensDialog'
import { applyLens, parseLens } from './lens'
import { screenDef, type ScreenKind } from './screens'
import { loadLayout, rememberBounds, screensChannel } from './layout'
import { MonitorCheck, X } from 'lucide-react'

// slotOf is the layout slot this window was opened as, if it is one of the position's screens.
const slotOf = () => new URLSearchParams(location.hash.split('?')[1] ?? '').get('slot')

/** The dispatcher's console: the full layout, or one screen of a position spread over several monitors. */
export default function Console({ screen = 'full' }: { screen?: ScreenKind }) {
  const raw = useStore((s) => s.view)
  const lens = useStore((s) => s.lens)
  const view = useMemo(() => (raw ? applyLens(raw, lens) : null), [raw, lens])
  const me = useStore((s) => s.me)
  const selected = useStore((s) => s.selected)
  const pick = useStore((s) => s.pick)
  const selUnitId = useStore((s) => s.selectedUnit)
  const loadError = useStore((s) => s.loadError)
  const [rec, setRec] = useState<Recommendation | null>(null)
  const [seed, setSeed] = useState<NewSeed>(null)
  const [palette, setPalette] = useState(false)
  const cmd = useRef<CommandLineHandle>(null)
  const nav = useNavigate()
  // A small movement first, so a click on a unit is still a click.
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))
  const dispatchUnit = (unit: string, inc: string) => api.post<{ ok: boolean; message: string }>('/command', { line: `D ${unit} #${inc}`, incident: inc })
    .then((r) => (r.ok ? toast.success(r.message) : toast.error(r.message))).catch((e) => toast.error(errText(e)))
  const [ncic, setNcic] = useState(false)
  const [rmsMeta, setRmsMeta] = useState<Meta | null>(null)
  // The selected incident is shown even when this screen's lens would hide it: someone chose it.
  const inc = raw?.incidents.find((i) => i.number === selected)
  const [screens, setScreens] = useState(false)
  const position = me?.console || me?.username || ''
  const [offer, setOffer] = useState(() => screen === 'full' && !slotOf() && !sessionStorage.getItem('bd.screens.offered'))
  const layout = loadLayout(position)
  const def = screenDef(screen)

  // A screen opened without a lens in its address starts with its own (police for the police board).
  useEffect(() => {
    const q = new URLSearchParams(location.hash.split('?')[1] ?? '')
    if (q.has('d')) return
    // Not kept for the tab: going back to the full console gives its own lens again.
    store.set({ lens: screen === 'full' ? parseLens(sessionStorage.getItem('bd.lens')) : def.lens ?? [] })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [screen])

  // A screen window answers "which screens are open?" and remembers where it is, so it opens there again.
  useEffect(() => {
    const slot = slotOf()
    document.title = screen === 'full' ? 'bitdispatcher' : `${def.title} · bitdispatcher`
    if (!slot) return
    const on = (ev: MessageEvent<{ ping?: boolean }>) => ev.data.ping && screensChannel?.postMessage({ pong: slot })
    screensChannel?.addEventListener('message', on)
    const save = () => rememberBounds(position, slot, { x: window.screenX, y: window.screenY, w: window.outerWidth, h: window.outerHeight })
    const t = window.setInterval(save, 3000)
    window.addEventListener('beforeunload', save)
    return () => {
      screensChannel?.removeEventListener('message', on)
      window.clearInterval(t)
      window.removeEventListener('beforeunload', save)
    }
  }, [screen, position, def.title])
  const seq = view?.seq

  // The recommendation follows every change, so it never shows a unit that has just been sent elsewhere.
  const loadRec = useCallback(async () => {
    if (!selected) return setRec(null)
    try {
      const r = await api.get<Recommendation>(`/incidents/${selected}/recommend`)
      setRec(r)
      return r
    } catch {
      setRec(null)
    }
  }, [selected])
  const prevSel = useRef<string | null>(null)
  useEffect(() => {
    loadRec().then((r) => {
      // On selecting an incident, start from the recommendation; later refreshes keep the dispatcher's choice.
      if (r && prevSel.current !== selected) store.setPick(r.units)
      prevSel.current = selected
    })
  }, [selected, seq, loadRec])

  const dispatchPicked = useCallback(async () => {
    const s = store.get()
    if (!s.selected || !s.pick.length) return
    try {
      await api.post(`/incidents/${s.selected}/dispatch`, { units: s.pick, recommended: rec?.units ?? [] })
      toast.success(`Dispatched ${s.pick.join(' ')} to ${s.selected}`)
      store.setPick([])
    } catch (e) {
      toast.error(errText(e))
    }
  }, [rec])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      const typing = t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setPalette((p) => !p)
      } else if ((e.ctrlKey || e.metaKey) && e.shiftKey && e.key.toLowerCase() === 's') {
        e.preventDefault()
        setScreens(true)
      } else if (e.key === 'F9' && can.dispatch(me)) {
        e.preventDefault()
        api.get<Meta>('/rms/meta').then((m) => { setRmsMeta(m); setNcic(true) }).catch((err) => toast.error(errText(err)))
      } else if (e.key === 'F2' && can.dispatch(me)) {
        e.preventDefault()
        setSeed({})
      } else if (e.ctrlKey && e.key === 'Enter' && !document.querySelector('[role=dialog]')) {
        e.preventDefault()
        dispatchPicked()
      } else if (!typing && (e.key === '/' || e.key === '`')) {
        e.preventDefault()
        cmd.current?.focus()
      } else if (!typing && e.key === 'Escape') {
        store.select(null)
        store.selectUnit(null)
        store.selectCall(null)
      } else if (!typing && (e.key === ']' || e.key === '[') && view) {
        const open = view.incidents.filter((i) => i.status !== 'closed').sort((a, b) => a.priority - b.priority || a.number.localeCompare(b.number))
        if (!open.length) return
        const i = open.findIndex((x) => x.number === store.get().selected)
        const n = e.key === ']' ? (i + 1) % open.length : (i - 1 + open.length) % open.length
        store.select(open[n].number)
      } else if (!typing && /^[a-zA-Z]$/.test(e.key) && !e.ctrlKey && !e.metaKey && !e.altKey && can.dispatch(me)) {
        // Typing a letter anywhere starts a command.
        cmd.current?.focus(e.key.toUpperCase())
        e.preventDefault()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [me, view, dispatchPicked])

  if (!view) {
    return <div className="grid h-full place-items-center text-sm text-slate-500">{loadError ? `Cannot load: ${loadError}` : 'Loading the dispatch picture…'}</div>
  }
  const fromCall = (c: Call) => setSeed({ call: c })
  const queue = <Boundary name="queue"><Queue view={view} onCreateFromCall={fromCall} onNew={() => setSeed({})} /></Boundary>
  const map = <Boundary name="map"><MapView view={view} rec={rec} onCreateAt={(lat, lon, address) => setSeed({ lat, lon, address })} /></Boundary>
  const panel = <Boundary key={inc?.number ?? '-'} name="incident">{inc ? <IncidentPanel view={raw!} inc={inc} rec={rec} reloadRec={loadRec} /> : <Situation view={view} onCreateFromCall={fromCall} />}</Boundary>
  const cell = 'flex min-h-0 flex-col bg-slate-950'
  return (
    <div className="flex h-full min-h-[600px] flex-col overflow-hidden bg-slate-950">
      <Boundary name="status"><StatusBar view={view} screen={screen} onScreens={() => setScreens(true)} /></Boundary>
      <Boundary name="emergency"><EmergencyBanner view={raw!} /></Boundary>
      {offer && screen === 'full' && layout.offer && layout.slots.length > 0 && (
        <div className="flex items-center gap-2 border-b border-sky-900 bg-sky-950/60 px-3 py-1.5 text-sm text-sky-100" role="status">
          <MonitorCheck size={15} className="text-sky-300" aria-hidden />
          Position {position} has {layout.slots.length} screen{layout.slots.length === 1 ? '' : 's'}: {layout.slots.map((x) => screenDef(x.kind).title).join(', ')}.
          <button className="btn btn-go h-7 text-xs" onClick={() => { openAll(layout.slots); sessionStorage.setItem('bd.screens.offered', '1'); setOffer(false) }}>Open them</button>
          <button className="btn btn-ghost h-7 text-xs" onClick={() => setScreens(true)}>Arrange…</button>
          <button className="btn btn-ghost ml-auto px-1" aria-label="Not now" onClick={() => { sessionStorage.setItem('bd.screens.offered', '1'); setOffer(false) }}><X size={14} /></button>
        </div>
      )}
      <DndContext sensors={sensors} onDragEnd={(e) => {
        const unit = String(e.active.id).replace(/^unit:/, '')
        const inc = e.over ? String(e.over.id).replace(/^inc:/, '') : ''
        if (inc) dispatchUnit(unit, inc)
      }}>
      {screen === 'full' && (
        <main className="grid min-h-0 flex-1 grid-cols-[minmax(300px,22%)_1fr_minmax(380px,28%)] grid-rows-[1fr_minmax(150px,26%)] gap-px bg-slate-800">
          <div className="row-span-2 flex min-h-0 bg-slate-950"><div className="flex min-h-0 flex-1 flex-col">{queue}</div></div>
          {map}
          <div className={cx('row-span-2', cell)}>{panel}</div>
          <Boundary name="unit board"><UnitBoard view={view} /></Boundary>
        </main>
      )}
      {screen === 'calls' && (
        <main className="grid min-h-0 flex-1 grid-cols-[minmax(380px,42%)_1fr] gap-px bg-slate-800">
          <div className={cell}>{queue}</div>
          <div className={cell}>{panel}</div>
        </main>
      )}
      {screen === 'units' && <main className={cx('min-h-0 flex-1', cell)}><Boundary name="unit board"><UnitBoard view={view} size="lg" /></Boundary></main>}
      {screen === 'stations' && <main className={cx('min-h-0 flex-1', cell)}><Boundary name="fire houses"><StationBoard view={view} /></Boundary></main>}
      {screen === 'police' && (
        <main className="grid min-h-0 flex-1 grid-cols-[1fr_minmax(340px,36%)] gap-px bg-slate-800">
          <div className={cell}><Boundary name="police board"><PoliceBoard view={view} /></Boundary></div>
          <div className="grid min-h-0 grid-rows-[1fr_auto] bg-slate-950">
            {map}
            {selUnitId && <div className="max-h-[45%] overflow-y-auto border-t border-slate-800 p-1.5"><UnitDetail view={raw!} id={selUnitId} /></div>}
          </div>
        </main>
      )}
      {screen === 'map' && <main className="flex min-h-0 flex-1">{map}</main>}
      {screen === 'incident' && <main className={cx('min-h-0 flex-1', cell)}>{panel}</main>}
      </DndContext>
      {screen !== 'map' && <Boundary name="command line"><CommandLine ref={cmd} view={view} /></Boundary>}
      <ScreensDialog open={screens} onClose={() => setScreens(false)} />
      <MarkOutDialog />
      <NewIncident view={view} seed={seed} onClose={() => setSeed(null)} />
      {rmsMeta && <NcicPanel open={ncic} onOpenChange={setNcic} meta={rmsMeta} reason={selected ?? ''} />}
      <Command.Dialog
        open={palette}
        onOpenChange={setPalette}
        label="Command palette"
        className="panel fixed left-1/2 top-[14vh] z-50 w-[600px] max-w-[94vw] -translate-x-1/2 overflow-hidden rounded-xl shadow-2xl"
        overlayClassName="fixed inset-0 z-40 bg-black/50"
      >
        <Command.Input className="w-full border-b border-slate-800 bg-transparent px-4 py-3 text-sm text-slate-50 outline-none" placeholder="Go to an incident, unit or screen…" />
        <Command.List className="max-h-[50vh] overflow-y-auto p-1 text-sm">
          <Command.Empty className="px-3 py-2 text-slate-500">Nothing matches.</Command.Empty>
          <Command.Group heading="Actions" className="text-[10px] uppercase text-slate-500">
            {can.dispatch(me) && <Item onSelect={() => { setPalette(false); setSeed({}) }}>New incident <span className="kbd ml-auto">F2</span></Item>}
            {selected && pick.length > 0 && <Item onSelect={() => { setPalette(false); dispatchPicked() }}>Dispatch {pick.join(' ')} to {selected}</Item>}
            <Item onSelect={() => { setPalette(false); cmd.current?.focus() }}>Command line <span className="kbd ml-auto">/</span></Item>
            <Item onSelect={() => nav('/rms')}>Incident reports (RMS)</Item>
            <Item onSelect={() => { setPalette(false); setScreens(true) }}>Screens: spread this position over monitors <span className="kbd ml-auto">Ctrl+Shift+S</span></Item>
            {can.supervise(me) && <Item onSelect={() => nav('/board')}>Supervisor board</Item>}
            {can.dispatch(me) && <Item onSelect={() => { setPalette(false); api.get<Meta>('/rms/meta').then((m) => { setRmsMeta(m); setNcic(true) }) }}>NCIC inquiry <span className="kbd ml-auto">F9</span></Item>}
          </Command.Group>
          <Command.Group heading="Incidents" className="text-[10px] uppercase text-slate-500">
            {view.incidents.map((i) => (
              <Item key={i.number} value={`${i.number} ${i.type} ${typeName(view, i.type)} ${i.location?.address ?? ''}`} onSelect={() => { store.select(i.number); setPalette(false) }}>
                <span className="font-mono">{i.number}</span> <span className="text-slate-400">P{i.priority}</span> {typeName(view, i.type)}
                <span className="ml-auto truncate text-xs text-slate-500">{i.location?.address}</span>
              </Item>
            ))}
          </Command.Group>
          <Command.Group heading="Units" className="text-[10px] uppercase text-slate-500">
            {view.units.map((u) => (
              <Item key={u.id} value={`unit ${u.id} ${u.type} ${u.status}`} onSelect={() => { store.selectUnit(u.id); setPalette(false) }}>
                <span className="font-mono">{u.id}</span> <span className="text-slate-400">{u.type}</span> <span className="ml-auto text-xs text-slate-500">{u.status}{u.incident ? ` · ${u.incident}` : ''}</span>
              </Item>
            ))}
          </Command.Group>
          <Command.Group heading="Manual" className="text-[10px] uppercase text-slate-500">
            {manual.map((a) => (
              <Item key={a.id} value={`help manual ${a.title} ${a.keys}`} onSelect={() => nav(`/help?a=${a.id}`)}>Help › {a.title}</Item>
            ))}
          </Command.Group>
          {can.supervise(me) && (
            <Command.Group heading="Administration" className="text-[10px] uppercase text-slate-500">
              {['units', 'stations', 'calltypes', 'map', 'settings', 'integrations', 'users', 'reports', 'ledger'].map((p) => (
                <Item key={p} value={`admin ${p}`} onSelect={() => nav(`/admin/${p}`)}>Admin › {p}</Item>
              ))}
            </Command.Group>
          )}
        </Command.List>
      </Command.Dialog>
    </div>
  )
}

function Item({ children, onSelect, value }: { children: React.ReactNode; onSelect: () => void; value?: string }) {
  return (
    <Command.Item value={value} onSelect={onSelect} className={cx('flex cursor-pointer items-center gap-2 rounded px-3 py-1.5 text-sm normal-case text-slate-200 data-[selected=true]:bg-slate-800')}>
      {children}
    </Command.Item>
  )
}
