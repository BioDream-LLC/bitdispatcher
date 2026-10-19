import { useEffect, useState } from 'react'
import { Coffee, Fuel, Gavel, GraduationCap, Wrench, FileText, Car, Building2, Footprints, ShieldCheck, Briefcase, Flame, MapPinned, Timer, Undo2 } from 'lucide-react'
import { Modal } from '../ui'
import AddressInput from '../AddressInput'
import { useStore } from '../store'
import { cx, markOutsFor } from '../util'
import { run } from './UnitBoard'

// Icons for the usual mark-out codes; any other code gets a timer.
const ICON: Record<string, typeof Coffee> = {
  MEAL: Coffee, FUEL: Fuel, COURT: Gavel, TRN: GraduationCap, MAINT: Wrench, RPT: FileText, TS: Car, SEC: Building2,
  FU: Footprints, PTL: ShieldCheck, ADMIN: Briefcase, INSP: Flame, JAIL: Undo2,
}
export const markOutIcon = (code?: string) => (code && ICON[code]) || Timer

/** Opens the mark-out dialog for a unit, from anywhere on any screen. It waits a moment, so a right-click
 * menu it is chosen from has closed first (two Radix layers opening and closing at once can leave the page
 * not taking clicks). */
export const openMarkOut = (unit: string) => window.setTimeout(() => window.dispatchEvent(new CustomEvent('bd:markout', { detail: unit })), 0)

/** Mark a unit out: what it is doing (each kind with its own status-check time), where, and a note. */
export default function MarkOutDialog() {
  const view = useStore((s) => s.view)
  const [unit, setUnit] = useState<string | null>(null)
  const [code, setCode] = useState('')
  const [where, setWhere] = useState('')
  const [note, setNote] = useState('')
  useEffect(() => {
    const on = (e: Event) => {
      setUnit((e as CustomEvent<string>).detail)
      setCode('')
      setWhere('')
      setNote('')
    }
    window.addEventListener('bd:markout', on)
    return () => window.removeEventListener('bd:markout', on)
  }, [])
  const u = view?.units.find((x) => x.id === unit)
  // Closing (rather than unmounting an open dialog) lets Radix give the page its clicks back.
  const close = () => setUnit(null)
  if (!view || !u) return <Modal open={false} onClose={close} title="Mark out">{null}</Modal>
  const kinds = markOutsFor(view, u)
  const submit = async (c = code) => {
    if (!c) return
    const line = `MO ${u.id} ${c}${where.trim() ? ' ' + where.trim().replace(/\//g, ' ') : ''}${note.trim() ? ' / ' + note.trim() : ''}`
    await run(line)
    close()
  }
  return (
    <Modal open onClose={close} wide icon={<MapPinned size={18} className="text-teal-300" aria-hidden />} title={<>Mark out <span className="font-mono">{u.id}</span></>}
      footer={
        <>
          <span className="mr-auto font-mono text-[11px] text-slate-500">MO {u.id} {code || '<kind>'}{where ? ' ' + where : ''}{note ? ' / ' + note : ''}</span>
          <button className="btn" onClick={close}>Cancel</button>
          <button className="btn btn-go" disabled={!code} onClick={() => submit()}>Mark out</button>
        </>
      }>
      <div className="space-y-3">
        <div className="grid grid-cols-2 gap-1.5 sm:grid-cols-3">
          {kinds.map((k) => {
            const Icon = markOutIcon(k.code)
            return (
              <button key={k.code} type="button" onClick={() => setCode(k.code)} onDoubleClick={() => submit(k.code)} aria-pressed={code === k.code}
                className={cx('flex items-center gap-2 rounded-lg border px-2 py-1.5 text-left transition', code === k.code ? 'border-teal-400 bg-teal-500/15' : 'border-slate-800 bg-slate-900/60 hover:border-slate-600')}>
                <Icon size={18} className={code === k.code ? 'text-teal-300' : 'text-slate-400'} aria-hidden />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-[13px] font-semibold text-slate-100">{k.name}</span>
                  <span className="block font-mono text-[10px] text-slate-500">{k.code} · {k.minutes ? `check ${k.minutes} min` : 'no timer'}{k.interruptible ? ' · can be pulled' : ''}</span>
                </span>
              </button>
            )
          })}
        </div>
        <div className="grid gap-2 sm:grid-cols-[1fr_16rem]">
          <div>
            <label className="label" htmlFor="mo-where">Where</label>
            <AddressInput id="mo-where" value={where} onChange={setWhere} onPick={() => {}} placeholder={u.position ? 'Blank: where its AVL says it is' : 'Address, intersection or place'} />
          </div>
          <div>
            <label className="label" htmlFor="mo-note">Note</label>
            <input id="mo-note" className="input" value={note} onChange={(e) => setNote(e.target.value)} placeholder="Courtroom 2, with 2A12…" onKeyDown={(e) => { if (e.key === 'Enter') submit() }} />
          </div>
        </div>
      </div>
    </Modal>
  )
}
