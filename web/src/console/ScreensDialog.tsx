import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ExternalLink, Monitor as MonitorIcon, MonitorCheck, Plus, ScanSearch, Trash2 } from 'lucide-react'
import { Modal } from '../ui'
import { store, useStore } from '../store'
import { cx } from '../util'
import { DISCIPLINES, lensName, type Lens } from '../lens'
import { SCREENS, Thumb, screenDef } from '../screens'
import { canPlace, loadLayout, monitors, newSlotId, openSlot, saveLayout, screensChannel, type Layout, type Monitor, type Slot } from '../layout'

/** Lens chips: none chosen means every discipline. */
export function LensChips({ lens, onChange, size = 'sm' }: { lens: Lens; onChange: (l: Lens) => void; size?: 'sm' | 'xs' }) {
  return (
    <div className="inline-flex items-center gap-0.5" role="group" aria-label="Disciplines shown">
      <button type="button" className={cx('chip', size === 'xs' ? 'h-5 px-1.5 text-[10px]' : 'h-6 px-2 text-[11px]', !lens.length ? 'bg-slate-200 text-slate-900' : 'bg-slate-800 text-slate-400 hover:text-slate-200')}
        aria-pressed={!lens.length} onClick={() => onChange([])}>All</button>
      {DISCIPLINES.map((d) => {
        const on = lens.includes(d.d)
        return (
          <button key={d.d} type="button" aria-pressed={on} onClick={() => onChange(on ? lens.filter((x) => x !== d.d) : [...lens, d.d])}
            className={cx('chip gap-1', size === 'xs' ? 'h-5 px-1.5 text-[10px]' : 'h-6 px-2 text-[11px]', on ? 'text-slate-950' : 'bg-slate-800 text-slate-400 hover:text-slate-200')}
            style={on ? { background: d.color } : undefined}>
            {!on && <span className="h-1.5 w-1.5 rounded-full" style={{ background: d.color }} aria-hidden />}{d.label}
          </button>
        )
      })}
    </div>
  )
}

/** Which screen windows are open: each answers a ping with its slot. */
function useOpenSlots(): Set<string> {
  const [open, setOpen] = useState<Set<string>>(new Set())
  useEffect(() => {
    const ch = screensChannel
    if (!ch) return
    const seen = new Set<string>()
    const on = (ev: MessageEvent<{ pong?: string }>) => {
      if (ev.data.pong) seen.add(ev.data.pong)
    }
    ch.addEventListener('message', on)
    const ping = () => {
      seen.clear()
      ch.postMessage({ ping: true })
      window.setTimeout(() => setOpen(new Set(seen)), 300)
    }
    ping()
    const t = window.setInterval(ping, 2000)
    return () => {
      window.clearInterval(t)
      ch.removeEventListener('message', on)
    }
  }, [])
  return open
}

/** Opens every screen of a layout; a browser that blocks the second pop-up is reported, not hidden. */
export async function openAll(slots: Slot[]) {
  let mons: Monitor[] | undefined
  try {
    mons = (await monitors())?.list
  } catch {
    /* permission refused: windows open where they were last */
  }
  let opened = 0
  for (const s of slots) if (openSlot(s, mons)) opened++
  if (opened < slots.length) toast.warning(`The browser opened ${opened} of ${slots.length} screens. Allow pop-ups for this site (and window management in Chrome or Edge), then open them again.`)
  else if (opened) toast.success(`${opened} screen${opened === 1 ? '' : 's'} opened`)
}

/** The arrangement of the monitors, as the operating system reports it, with each screen placed on its own. */
function Arrangement({ mons, slots, current }: { mons: Monitor[]; slots: Slot[]; current?: Monitor }) {
  const minX = Math.min(...mons.map((m) => m.left)), minY = Math.min(...mons.map((m) => m.top))
  const maxX = Math.max(...mons.map((m) => m.left + m.width)), maxY = Math.max(...mons.map((m) => m.top + m.height))
  const W = 560, k = W / (maxX - minX), H = (maxY - minY) * k
  return (
    <svg viewBox={`-4 -4 ${W + 8} ${H + 8}`} className="w-full" style={{ maxHeight: 220 }} role="img" aria-label={`${mons.length} monitors`}>
      {mons.map((m) => {
        const x = (m.left - minX) * k, y = (m.top - minY) * k, w = m.width * k, h = m.height * k
        const here = slots.filter((s) => s.monitor === m.key)
        const isCur = current && current.key === m.key
        return (
          <g key={m.key}>
            <rect x={x + 2} y={y + 2} width={w - 4} height={h - 4} rx="6" fill={here.length ? '#0c1a2e' : '#0b1220'} stroke={isCur ? '#38bdf8' : '#475569'} strokeWidth={isCur ? 2 : 1} />
            <text x={x + 10} y={y + 18} fontSize="11" fontWeight="700" fill="#e2e8f0">{m.label}{m.isPrimary ? ' · primary' : ''}</text>
            <text x={x + 10} y={y + 31} fontSize="9" fill="#64748b">{m.width}×{m.height}{isCur ? ' · this window' : ''}</text>
            {here.slice(0, 3).map((s, i) => {
              const tw = Math.min(w - 20, 110), th = tw * (80 / 120)
              return (
                <g key={s.id} transform={`translate(${x + 10 + i * 8}, ${y + 38 + i * 8})`}>
                  <foreignObject width={tw} height={th}><Thumb kind={s.kind} className="h-full w-full drop-shadow" /></foreignObject>
                </g>
              )
            })}
            {here.length > 0 && <text x={x + w - 10} y={y + h - 10} textAnchor="end" fontSize="10" fontWeight="700" fill="#38bdf8">{here.map((s) => screenDef(s.kind).title).join(' · ')}</text>}
          </g>
        )
      })}
    </svg>
  )
}

/** Screens: the position's windows across its monitors. */
export default function ScreensDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const me = useStore((s) => s.me)
  const lens = useStore((s) => s.lens)
  const position = me?.console || me?.username || ''
  const [l, setL] = useState<Layout>(() => loadLayout(position))
  const [mons, setMons] = useState<{ list: Monitor[]; current?: Monitor } | null>(null)
  const openNow = useOpenSlots()
  useEffect(() => {
    if (open) setL(loadLayout(position))
  }, [open, position])
  // Monitors already allowed are read without asking.
  useEffect(() => {
    if (!open || !canPlace()) return
    navigator.permissions?.query({ name: 'window-management' as PermissionName }).then((p) => {
      if (p.state === 'granted') monitors().then(setMons).catch(() => {})
    }).catch(() => {})
  }, [open])
  const put = (n: Layout) => {
    setL(n)
    saveLayout(position, n)
  }
  const upSlot = (id: string, p: Partial<Slot>) => put({ ...l, slots: l.slots.map((s) => (s.id === id ? { ...s, ...p } : s)) })
  const add = (kind: Slot['kind']) => {
    const d = screenDef(kind)
    // The next monitor without a screen, when they are known.
    const free = mons?.list.find((m) => !l.slots.some((s) => s.monitor === m.key) && m.key !== mons.current?.key)
    put({ ...l, slots: [...l.slots, { id: newSlotId(), kind, lens: d.lens ?? [], monitor: free?.key }] })
  }
  const detect = async () => {
    try {
      const m = await monitors()
      setMons(m)
      if (m) toast.success(`${m.list.length} monitor${m.list.length === 1 ? '' : 's'} found`)
    } catch {
      toast.error('Window management was not allowed: allow it in the browser’s site settings to place screens on monitors')
    }
  }
  return (
    <Modal open={open} onClose={onClose} xl icon={<MonitorCheck size={18} className="text-sky-300" aria-hidden />} title={<>Screens <span className="ml-1 text-xs font-normal text-slate-400">for position {position || '(none)'}, kept on this workstation</span></>}
      footer={
        <>
          <label className="mr-auto flex items-center gap-2 text-xs text-slate-400">
            <input type="checkbox" checked={l.offer} onChange={(e) => put({ ...l, offer: e.target.checked })} /> Offer to open these screens at sign-in
          </label>
          <button className="btn btn-go" disabled={!l.slots.length} onClick={() => openAll(l.slots)}><ExternalLink size={14} aria-hidden /> Open all {l.slots.length || ''}</button>
        </>
      }>
      <div className="space-y-5">
        <section className="flex flex-wrap items-center gap-3 rounded-lg border border-slate-800 bg-slate-900/50 p-3">
          <div className="text-sm font-semibold text-slate-100">This window shows</div>
          <LensChips lens={lens} onChange={(x) => store.setLens(x)} />
          <span className="text-xs text-slate-500">{lensName(lens)}. Emergencies and 911 calls not yet on an incident are always shown.</span>
        </section>

        <section>
          <div className="mb-2 flex items-center gap-2">
            <h3 className="text-sm font-semibold text-slate-100">Monitors</h3>
            {canPlace() ? (
              <button className="btn h-7 text-xs" onClick={detect}><ScanSearch size={13} aria-hidden /> {mons ? 'Detect again' : 'Detect monitors'}</button>
            ) : (
              <span className="text-xs text-slate-500">This browser cannot place windows on monitors (Chrome and Edge can). Drag each screen where you want it once: it opens there next time.</span>
            )}
          </div>
          {mons && <Arrangement mons={mons.list} slots={l.slots} current={mons.current} />}
        </section>

        <section>
          <h3 className="mb-2 text-sm font-semibold text-slate-100">Your screens <span className="font-normal text-slate-500">({l.slots.length})</span></h3>
          {!l.slots.length && <p className="rounded border border-dashed border-slate-700 p-3 text-xs text-slate-500">No screens yet: add the ones you want below. A common layout: incidents and call entry on one monitor, fire houses on the next, police status on the third, and this window as the map.</p>}
          <ul className="grid gap-2 md:grid-cols-2">
            {l.slots.map((s) => {
              const d = screenDef(s.kind)
              const isOpen = openNow.has(s.id)
              return (
                <li key={s.id} className="flex gap-3 rounded-lg border border-slate-800 bg-slate-900/60 p-2">
                  <Thumb kind={s.kind} className="h-[60px] w-[90px] shrink-0" />
                  <div className="min-w-0 flex-1 space-y-1.5">
                    <div className="flex items-center gap-2">
                      <span className={cx('h-2 w-2 rounded-full', isOpen ? 'bg-emerald-400 live-dot' : 'bg-slate-600')} title={isOpen ? 'Open now' : 'Not open'} aria-label={isOpen ? 'open' : 'closed'} />
                      <span className="truncate text-sm font-semibold text-slate-100">{d.title}</span>
                      <button className="btn btn-ghost ml-auto px-1" onClick={() => put({ ...l, slots: l.slots.filter((x) => x.id !== s.id) })} aria-label={`Remove ${d.title}`}><Trash2 size={13} /></button>
                    </div>
                    {s.kind !== 'map' && <LensChips size="xs" lens={s.lens} onChange={(x) => upSlot(s.id, { lens: x })} />}
                    <div className="flex items-center gap-1.5">
                      {mons ? (
                        <select className="input h-7 flex-1 py-0 text-xs" value={s.monitor ?? ''} onChange={(e) => upSlot(s.id, { monitor: e.target.value || undefined })} aria-label={`Monitor for ${d.title}`}>
                          <option value="">Where it was last</option>
                          {mons.list.map((m) => <option key={m.key} value={m.key}>{m.label}{m.isPrimary ? ' (primary)' : ''}{m.key === mons.current?.key ? ' (this window)' : ''}</option>)}
                        </select>
                      ) : (
                        <span className="flex-1 truncate text-[11px] text-slate-500"><MonitorIcon size={11} className="mr-1 inline" aria-hidden />{s.monitor ? 'its monitor, once detected' : s.bounds ? `at ${s.bounds.x}, ${s.bounds.y}` : 'opens where you put it'}</span>
                      )}
                      <button className="btn h-7 text-xs" onClick={() => {
                        if (!openSlot(s, mons?.list)) toast.warning('The browser blocked the window: allow pop-ups for this site')
                      }}><ExternalLink size={12} aria-hidden /> {isOpen ? 'Bring forward' : 'Open'}</button>
                    </div>
                  </div>
                </li>
              )
            })}
          </ul>
        </section>

        <section>
          <h3 className="mb-2 text-sm font-semibold text-slate-100">Add a screen</h3>
          <div className="grid grid-cols-2 gap-2 md:grid-cols-4">
            {SCREENS.filter((d) => d.kind !== 'full').map((d) => (
              <button key={d.kind} type="button" onClick={() => add(d.kind)} className="group flex flex-col gap-1.5 rounded-lg border border-slate-800 bg-slate-900/40 p-2 text-left transition hover:border-sky-600 hover:bg-slate-900">
                <Thumb kind={d.kind} className="w-full transition group-hover:scale-[1.02]" />
                <span className="flex items-center gap-1 text-[13px] font-semibold text-slate-100"><Plus size={12} className="text-sky-400" aria-hidden />{d.title}</span>
                <span className="text-[11px] leading-snug text-slate-400">{d.hint}</span>
                {d.lens && <span className="text-[10px] uppercase tracking-wider text-slate-500">starts with {lensName(d.lens)}</span>}
              </button>
            ))}
          </div>
        </section>
      </div>
    </Modal>
  )
}
