import { useEffect, useRef, useState } from 'react'
import { GripVertical, LayoutTemplate, Lock, LockOpen, Plus, Terminal, Trash2, X, Check } from 'lucide-react'
import { cx } from '../util'
import {
  SECTIONS, dock, edgeAt, removeSection, resize, sectionDef, sectionsIn, stocksFor, toggleSection, withStock,
  type Edge, type Pane, type SectionDef, type SectionId, type Workspace,
} from '../workspace'

// A section being dragged (from the Layout menu or by its title bar), with the pointer, and where it
// would land: the section under the pointer and its nearest edge. Pointer events, not the browser's
// drag and drop: the same with a mouse, a pen or a touch screen, and over the map.
type Drag = { s: SectionId; x: number; y: number; target: SectionId | null; edge: Edge | null; empty: boolean }
let drag: Drag | null = null
const setDrag = (d: Drag | null) => {
  drag = d
  window.dispatchEvent(new Event('bd:wsdrag'))
}
function useDrag() {
  const [d, setD] = useState<Drag | null>(drag)
  useEffect(() => {
    const on = () => setD(drag)
    window.addEventListener('bd:wsdrag', on)
    return () => window.removeEventListener('bd:wsdrag', on)
  }, [])
  return d
}
function under(x: number, y: number): Pick<Drag, 'target' | 'edge' | 'empty'> {
  for (const el of document.elementsFromPoint(x, y)) {
    const h = el as HTMLElement
    if (h.dataset.emptyWorkspace) return { target: null, edge: 'center', empty: true }
    const s = h.dataset.section as SectionId | undefined
    if (s) {
      const r = h.getBoundingClientRect()
      return { target: s, edge: edgeAt(x - r.left, y - r.top, r.width, r.height), empty: false }
    }
  }
  return { target: null, edge: null, empty: false }
}
let dragged = false // the last press became a drag: its click is not a click
/** Starts following the pointer; past a few pixels it is a drag, and letting go drops the section. */
function startDrag(e: React.PointerEvent, s: SectionId, onStart?: () => void) {
  if (e.button !== 0) return
  const x0 = e.clientX, y0 = e.clientY
  dragged = false
  const move = (ev: PointerEvent) => {
    if (!drag && Math.hypot(ev.clientX - x0, ev.clientY - y0) < 6) return
    if (!drag) { dragged = true; onStart?.() }
    ev.preventDefault()
    setDrag({ s, x: ev.clientX, y: ev.clientY, ...under(ev.clientX, ev.clientY) })
  }
  const up = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('pointercancel', cancel)
    const d = drag
    setDrag(null)
    if (d && (d.target || d.empty) && d.target !== d.s) window.dispatchEvent(new CustomEvent('bd:wsdrop', { detail: d }))
  }
  const cancel = () => {
    window.removeEventListener('pointermove', move)
    window.removeEventListener('pointerup', up)
    window.removeEventListener('pointercancel', cancel)
    setDrag(null)
  }
  window.addEventListener('pointermove', move)
  window.addEventListener('pointerup', up)
  window.addEventListener('pointercancel', cancel)
}

/** The console's sections, as the workspace arranges them. */
export default function WorkspaceView({ ws, onChange, render }: { ws: Workspace; onChange: (w: Workspace) => void; render: (s: SectionId) => React.ReactNode }) {
  const d = useDrag()
  const cur = useRef({ ws, onChange })
  cur.current = { ws, onChange }
  useEffect(() => {
    const drop = (e: Event) => {
      const v = (e as CustomEvent<Drag>).detail
      cur.current.onChange(dock(cur.current.ws, v.s, v.target, v.edge ?? 'center'))
    }
    window.addEventListener('bd:wsdrop', drop)
    return () => window.removeEventListener('bd:wsdrop', drop)
  }, [])
  const ghost = d && (
    <div className="pointer-events-none fixed z-[60] rounded border border-sky-400 bg-sky-900/90 px-2 py-1 text-xs font-bold text-sky-50 shadow-lg" style={{ left: d.x + 14, top: d.y + 14 }} aria-hidden>
      {sectionDef(d.s).title}
    </div>
  )
  if (!ws.root) {
    return (
      <main
        data-empty-workspace="1"
        className={cx('grid min-h-0 flex-1 place-items-center bg-slate-950 p-6 text-center', d && 'outline-2 -outline-offset-8 outline-dashed outline-sky-500')}
        aria-label="Workspace"
      >
        {ghost}
        <div className="max-w-sm text-sm text-slate-400">
          <LayoutTemplate size={28} className="mx-auto mb-2 text-slate-600" aria-hidden />
          No sections on this screen. Open <b className="text-slate-200">Layout</b> at the top to add some, or pick one of the ready-made layouts.
        </div>
      </main>
    )
  }
  return (
    <main className={cx('flex min-h-0 min-w-0 flex-1 bg-slate-800', d && 'select-none')} aria-label="Workspace">
      {ghost}
      <PaneView p={ws.root} path={[]} ws={ws} onChange={onChange} render={render} drag={d} />
    </main>
  )
}

function PaneView({ p, path, ws, onChange, render, drag }: { p: Pane; path: number[]; ws: Workspace; onChange: (w: Workspace) => void; render: (s: SectionId) => React.ReactNode; drag: Drag | null }) {
  const box = useRef<HTMLDivElement>(null)
  if (p.t === 'leaf') return <Section s={p.s} ws={ws} onChange={onChange} render={render} drag={drag} />
  const rowDir = p.dir === 'row'
  const startResize = (i: number) => (e: React.PointerEvent) => {
    const el = box.current
    if (!el) return
    e.preventDefault()
    const r = el.getBoundingClientRect()
    const move = (ev: PointerEvent) => {
      const at = rowDir ? (ev.clientX - r.left) / r.width : (ev.clientY - r.top) / r.height
      onChange(resize(ws, path, i, at))
    }
    const up = () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerup', up)
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up)
  }
  return (
    <div ref={box} className={cx('flex min-h-0 min-w-0 flex-1', rowDir ? 'flex-row' : 'flex-col')}>
      {p.kids.map((k, i) => (
        <div key={k.t === 'leaf' ? k.s : `split-${i}`} className="contents">
          {i > 0 && (ws.locked ? (
            <div className={cx('shrink-0 bg-slate-800', rowDir ? 'w-px' : 'h-px')} aria-hidden />
          ) : (
            <div
              role="separator"
              aria-orientation={rowDir ? 'vertical' : 'horizontal'}
              aria-label="Drag to resize"
              title="Drag to resize"
              onPointerDown={startResize(i - 1)}
              className={cx('shrink-0 bg-slate-700 hover:bg-sky-500', rowDir ? 'w-1.5 cursor-col-resize' : 'h-1.5 cursor-row-resize')}
            />
          ))}
          <div className="flex min-h-0 min-w-0" style={{ flex: `${p.sizes[i]} 1 0px` }}>
            <PaneView p={k} path={[...path, i]} ws={ws} onChange={onChange} render={render} drag={drag} />
          </div>
        </div>
      ))}
    </div>
  )
}

const EDGE_BOX: Record<Edge, string> = {
  left: 'inset-y-0 left-0 w-1/2', right: 'inset-y-0 right-0 w-1/2', top: 'inset-x-0 top-0 h-1/2', bottom: 'inset-x-0 bottom-0 h-1/2', center: 'inset-[15%]',
}

function Section({ s, ws, onChange, render, drag }: { s: SectionId; ws: Workspace; onChange: (w: Workspace) => void; render: (s: SectionId) => React.ReactNode; drag: Drag | null }) {
  const d = sectionDef(s)
  const edge = drag && drag.target === s && drag.s !== s ? drag.edge : null
  return (
    <section className="relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden bg-slate-950" aria-label={d.title} data-section={s}>
      {!ws.locked && (
        <div className="flex h-6 shrink-0 items-center gap-1 border-b border-sky-900/70 bg-sky-950/50 px-1 text-[11px] font-semibold text-sky-100">
          <span
            onPointerDown={(e) => startDrag(e, s)}
            className="inline-flex cursor-grab touch-none items-center gap-1 rounded px-1 hover:bg-sky-900 active:cursor-grabbing"
            title={`Drag ${d.title} to another place`}
            aria-label={`Move ${d.title}`}
          >
            <GripVertical size={13} aria-hidden />{d.title}
          </span>
          <button type="button" className="ml-auto rounded p-0.5 text-sky-300 hover:bg-rose-900 hover:text-rose-100" onClick={() => onChange(removeSection(ws, s))} aria-label={`Take ${d.title} off the screen`} title={`Take ${d.title} off the screen`}>
            <X size={13} aria-hidden />
          </button>
        </div>
      )}
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">{render(s)}</div>
      {drag && drag.s !== s && (
        // Over the section while dragging (the map would take the pointer otherwise); the section is found by
        // its data-section, under this.
        <div className="pointer-events-none absolute inset-0 z-30" aria-hidden>
          <div className="absolute inset-0 bg-slate-950/40" />
          {edge && (
            <div className={cx('absolute grid place-items-center rounded border-2 border-sky-400 bg-sky-500/25 p-1 text-center text-xs font-bold text-sky-50', EDGE_BOX[edge])}>
              {edge === 'center' ? (sectionsIn(ws.root).includes(drag.s) ? `Trade places with ${d.title}` : `Put it here instead of ${d.title}`) : `${sectionDef(drag.s).title} ${edge === 'left' || edge === 'right' ? 'beside' : edge === 'top' ? 'above' : 'below'} ${d.title}`}
            </div>
          )}
        </div>
      )}
    </section>
  )
}

const TONE: Record<SectionDef['tone'], [string, string]> = {
  queue: ['#1e293b', '#38bdf8'], map: ['#0f2a2a', '#2dd4bf'], panel: ['#1e1b3a', '#a78bfa'], units: ['#2a1f12', '#fb923c'], cards: ['#2a1414', '#f87171'], table: ['#14213d', '#818cf8'],
}

// A small drawing of a section, in its colours: rows for lists, a road for the map, tiles for units.
function Glyph({ tone, x, y, w, h }: { tone: SectionDef['tone']; x: number; y: number; w: number; h: number }) {
  const [bg, fg] = TONE[tone]
  return (
    <g>
      <rect x={x} y={y} width={w} height={h} rx="1.5" fill={bg} stroke={fg} strokeOpacity="0.6" strokeWidth="0.7" />
      {tone === 'map' && <path d={`M${x + 2} ${y + h * 0.7} q${w * 0.25} ${-h * 0.4} ${w * 0.5} ${-h * 0.1} t${w * 0.45} ${-h * 0.25}`} stroke={fg} strokeOpacity="0.7" fill="none" strokeWidth="1" />}
      {(tone === 'queue' || tone === 'table' || tone === 'panel') && Array.from({ length: Math.max(1, Math.floor((h - 3) / 3.4)) }, (_, j) => (
        <rect key={j} x={x + 1.5} y={y + 2 + j * 3.4} width={Math.max(1, (w - 3) * (tone === 'panel' && j % 2 ? 0.6 : 1))} height="2" rx="0.5" fill={fg} fillOpacity={j === 0 ? 0.7 : 0.25} />
      ))}
      {(tone === 'units' || tone === 'cards') && Array.from({ length: Math.min(12, Math.max(1, Math.floor((w - 2) / 6)) * Math.max(1, Math.floor((h - 2) / 5))) }, (_, j) => {
        const cols = Math.max(1, Math.floor((w - 2) / 6))
        return <rect key={j} x={x + 1.5 + (j % cols) * 6} y={y + 1.5 + Math.floor(j / cols) * 5} width="4.6" height="3.6" rx="0.6" fill={['#34d399', '#facc15', '#fb7185', '#2dd4bf'][j % 4]} fillOpacity="0.75" />
      })}
    </g>
  )
}

function drawPane(p: Pane, x: number, y: number, w: number, h: number, out: React.ReactNode[]) {
  if (p.t === 'leaf') {
    out.push(<Glyph key={out.length} tone={sectionDef(p.s).tone} x={x + 0.6} y={y + 0.6} w={Math.max(1, w - 1.2)} h={Math.max(1, h - 1.2)} />)
    return
  }
  let at = 0
  p.kids.forEach((k, i) => {
    const f = p.sizes[i]
    if (p.dir === 'row') drawPane(k, x + at * w, y, f * w, h, out)
    else drawPane(k, x, y + at * h, w, f * h, out)
    at += f
  })
}

/** A small drawing of a whole workspace. */
export function WorkspaceThumb({ ws, className, label }: { ws: Workspace; className?: string; label: string }) {
  const out: React.ReactNode[] = []
  const cmdH = ws.cmd ? 6 : 0
  if (ws.root) drawPane(ws.root, 2, 2, 116, 76 - cmdH, out)
  return (
    <svg viewBox="0 0 120 80" className={className} role="img" aria-label={label}>
      <rect x="0" y="0" width="120" height="80" rx="4" fill="#020617" stroke="#334155" />
      {out}
      {ws.cmd && <rect x="2.6" y={80 - cmdH - 1.4} width="114.8" height={cmdH - 1} rx="1" fill="#111827" stroke="#64748b" strokeOpacity="0.6" strokeWidth="0.6" />}
    </svg>
  )
}

/** The lock: while locked, nothing on the screen can be moved, resized or taken away. */
export function LockButton({ ws, onChange }: { ws: Workspace; onChange: (w: Workspace) => void }) {
  return (
    <button
      type="button"
      className={cx('btn px-1.5', ws.locked ? 'btn-ghost' : 'bg-amber-500/20 text-amber-200 hover:bg-amber-500/30')}
      onClick={() => onChange({ ...ws, locked: !ws.locked })}
      aria-pressed={!ws.locked}
      aria-label={ws.locked ? 'Layout locked: click to arrange sections' : 'Layout unlocked: click to lock it'}
      title={ws.locked ? 'Layout locked. Click to unlock and drag sections around.' : 'Layout unlocked: drag sections by their title bars. Click to lock it.'}
    >
      {ws.locked ? <Lock size={14} aria-hidden /> : <LockOpen size={14} aria-hidden />}
    </button>
  )
}

/** The Layout menu: the role's ready-made layouts, and every section to drag onto the screen. */
export function LayoutMenu({ ws, onChange, duty }: { ws: Workspace; onChange: (w: Workspace) => void; duty?: string }) {
  const [open, setOpen] = useState(false)
  const [hidden, setHidden] = useState(false) // while dragging from it, out of the way of the drop places
  const root = useRef<HTMLDivElement>(null)
  const drag = useDrag()
  useEffect(() => {
    if (!drag) setHidden(false)
  }, [drag])
  useEffect(() => {
    if (!open) return
    const away = (e: PointerEvent) => { if (!root.current?.contains(e.target as Node)) setOpen(false) }
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false) }
    window.addEventListener('pointerdown', away)
    window.addEventListener('keydown', esc)
    return () => {
      window.removeEventListener('pointerdown', away)
      window.removeEventListener('keydown', esc)
    }
  }, [open])
  const shown = sectionsIn(ws.root)
  const stocks = stocksFor(duty)
  return (
    <div ref={root} className="relative">
      <button type="button" className={cx('btn h-7 gap-1 px-2 text-xs', open ? 'bg-slate-800 text-slate-50' : 'btn-ghost')} onClick={() => setOpen((o) => !o)} aria-expanded={open} aria-haspopup="dialog" title="Layout: choose what this screen shows">
        <LayoutTemplate size={14} aria-hidden />Layout
      </button>
      {open && (
        <div
          role="dialog"
          aria-label="Layout"
          className={cx('panel absolute left-0 top-9 z-50 w-[560px] max-w-[92vw] rounded-xl p-3 shadow-2xl transition-opacity', hidden && 'pointer-events-none opacity-0')}
        >
          <div className="mb-1.5 flex items-baseline gap-2">
            <h2 className="text-xs font-bold uppercase tracking-wider text-slate-400">Ready-made layouts</h2>
            <span className="text-[11px] text-slate-500">for {duty || 'your duty'}</span>
          </div>
          <div className="grid grid-cols-3 gap-2">
            {stocks.map((s) => (
              <button
                key={s.id}
                type="button"
                className={cx('rounded-lg border p-1.5 text-left hover:border-sky-500', ws.stock === s.id ? 'border-sky-500 bg-sky-950/40' : 'border-slate-700')}
                onClick={() => onChange(withStock(ws, s))}
                aria-pressed={ws.stock === s.id}
                title={s.hint}
              >
                <WorkspaceThumb ws={s.ws} className="w-full" label={`Layout of ${s.title}`} />
                <div className="mt-1 flex items-center gap-1 text-xs font-semibold text-slate-100">{ws.stock === s.id && <Check size={12} className="text-sky-300" aria-hidden />}{s.title}</div>
                <div className="text-[10px] leading-tight text-slate-400">{s.hint}</div>
              </button>
            ))}
          </div>
          <div className="mb-1.5 mt-3 flex items-center gap-2">
            <h2 className="text-xs font-bold uppercase tracking-wider text-slate-400">Sections</h2>
            <span className="text-[11px] text-slate-500">{ws.locked ? 'Unlock to arrange your own.' : 'Drag one onto the screen, or click to add or take away.'}</span>
            {ws.locked && (
              <button type="button" className="btn btn-ghost ml-auto h-6 gap-1 px-1.5 text-[11px]" onClick={() => onChange({ ...ws, locked: false })}>
                <LockOpen size={12} aria-hidden />Unlock layout
              </button>
            )}
          </div>
          <div className="grid grid-cols-4 gap-2">
            {SECTIONS.map((d) => {
              const on = shown.includes(d.id)
              return (
                <button
                  key={d.id}
                  type="button"
                  disabled={ws.locked}
                  onPointerDown={(e) => { if (!ws.locked) startDrag(e, d.id, () => setHidden(true)) }}
                  onClick={() => { if (!dragged) onChange(toggleSection(ws, d.id)); dragged = false }}
                  aria-pressed={on}
                  aria-label={`${d.title}: ${on ? 'on the screen, click to take away' : 'click to add'}`}
                  title={d.hint}
                  className={cx('relative rounded-lg border p-1.5 text-left disabled:cursor-not-allowed disabled:opacity-50', on ? 'border-emerald-600 bg-emerald-950/30' : 'border-slate-700 hover:border-sky-500', !ws.locked && 'cursor-grab touch-none')}
                >
                  <svg viewBox="0 0 40 26" className="w-full" aria-hidden><Glyph tone={d.tone} x={1} y={1} w={38} h={24} /></svg>
                  <div className="mt-1 flex items-center gap-1 text-[11px] font-semibold text-slate-100">
                    {on ? <Check size={11} className="text-emerald-300" aria-hidden /> : <Plus size={11} className="text-slate-400" aria-hidden />}{d.title}
                  </div>
                </button>
              )
            })}
            <button
              type="button"
              disabled={ws.locked}
              onClick={() => onChange({ ...ws, cmd: !ws.cmd, stock: undefined })}
              aria-pressed={ws.cmd}
              aria-label={`Command line: ${ws.cmd ? 'on the screen, click to take away' : 'click to add'}`}
              title="The command line along the bottom: type a command, or any letter to start one."
              className={cx('rounded-lg border p-1.5 text-left disabled:cursor-not-allowed disabled:opacity-50', ws.cmd ? 'border-emerald-600 bg-emerald-950/30' : 'border-slate-700 hover:border-sky-500')}
            >
              <div className="grid h-[38px] place-items-center rounded border border-slate-600 bg-slate-900 text-slate-400"><Terminal size={16} aria-hidden /></div>
              <div className="mt-1 flex items-center gap-1 text-[11px] font-semibold text-slate-100">
                {ws.cmd ? <Check size={11} className="text-emerald-300" aria-hidden /> : <Plus size={11} className="text-slate-400" aria-hidden />}Command line
              </div>
            </button>
          </div>
          <div className="mt-3 flex items-center gap-2 border-t border-slate-800 pt-2 text-[11px] text-slate-400">
            <span>{ws.stock ? `Showing “${stocks.find((s) => s.id === ws.stock)?.title ?? ws.stock}”.` : 'Showing your own layout.'} Kept for you in this browser.</span>
            <button type="button" disabled={ws.locked || !ws.root} className="btn btn-ghost ml-auto h-6 gap-1 px-1.5 text-[11px]" onClick={() => onChange({ ...ws, root: null, stock: undefined })}>
              <Trash2 size={12} aria-hidden />Clear the screen
            </button>
            <LockButton ws={ws} onChange={onChange} />
          </div>
        </div>
      )}
    </div>
  )
}
