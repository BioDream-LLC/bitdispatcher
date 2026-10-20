import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Download, Syringe, UploadCloud } from 'lucide-react'
import { api, errText } from '../api'
import AddressInput from '../AddressInput'
import { can, useStore } from '../store'
import { Modal } from '../ui'
import { cx } from '../util'
import type { View } from '../types'

type Kind = 'call' | 'say' | 'move' | 'unit' | 'emergency' | 'radio'
const KINDS: [Kind, string][] = [['call', 'A new caller'], ['say', 'The caller says more'], ['move', 'The caller moves'], ['unit', 'A unit changes status'], ['emergency', 'An emergency button'], ['radio', 'A unit on the radio']]

/** The instructor's tools on a training board: put calls and changing conditions into the exercise as it
 * runs, download its timeline for the debrief, and (an administrator) publish staged configuration. */
export function TrainingTools({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const [inject, setInject] = useState(false)
  const [staging, setStaging] = useState(false)
  return (
    <>
      {me?.exercise && <span className="chip bg-violet-500/30 text-violet-100" title="Everyone who joined this exercise works on this one board">exercise: {me.exercise}</span>}
      {can.supervise(me) && <button className="btn h-7 text-xs" onClick={() => setInject(true)}><Syringe size={13} aria-hidden /> Inject</button>}
      <a className="btn h-7 text-xs" href="./api/training/timeline" download title="Every event on this board, for the debrief"><Download size={13} aria-hidden /> Timeline</a>
      {can.admin(me) && <button className="btn h-7 text-xs" onClick={() => setStaging(true)} title="Configuration changed on this board, to publish to the live board"><UploadCloud size={13} aria-hidden /> Staged changes</button>}
      {inject && <Inject view={view} onClose={() => setInject(false)} />}
      {staging && <Staging onClose={() => setStaging(false)} />}
    </>
  )
}

function Inject({ view, onClose }: { view: View; onClose: () => void }) {
  const [kind, setKind] = useState<Kind>('call')
  const [address, setAddress] = useState('')
  const [pos, setPos] = useState<{ lat: number; lon: number } | null>(null)
  const [name, setName] = useState('')
  const [callback, setCallback] = useState('+15405550100')
  const [lines, setLines] = useState('0 | 911, what is the address of your emergency?\n3 | ')
  const [call, setCall] = useState('')
  const [unit, setUnit] = useState('')
  const [status, setStatus] = useState('oos')
  const [text, setText] = useState('')
  const calls = view.calls.filter((c) => c.state !== 'ended')
  const send = async () => {
    const parsed = lines.split('\n').map((l) => l.split('|')).filter((p) => p.length > 1 && p[1].trim()).map((p, i) => ({ after: Number(p[0]) || 0, speaker: i === 0 && /address of your emergency/i.test(p[1]) ? 'taker' : 'caller', text: p[1].trim() }))
    const body: Record<string, unknown> = { kind }
    if (kind === 'call') Object.assign(body, { lat: pos?.lat, lon: pos?.lon, address, name, callback, lines: parsed })
    if (kind === 'say') Object.assign(body, { call, lines: parsed })
    if (kind === 'move') Object.assign(body, { call, lat: pos?.lat, lon: pos?.lon })
    if (kind === 'unit') Object.assign(body, { unit, status })
    if (kind === 'emergency') Object.assign(body, { unit })
    if (kind === 'radio') Object.assign(body, { unit, text })
    try {
      await api.post('/training/inject', body)
      toast.success('Injected')
      if (kind !== 'call') onClose()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const needPos = kind === 'call' || kind === 'move'
  return (
    <Modal open onClose={onClose} wide title="Inject into the exercise" icon={<Syringe size={16} className="text-violet-300" aria-hidden />}
      footer={<><button className="btn" onClick={onClose}>Close</button><button className="btn btn-go" disabled={needPos && !pos} onClick={send}>Inject</button></>}>
      <div className="space-y-3 text-sm">
        <div className="flex flex-wrap gap-1">{KINDS.map(([k, l]) => <button key={k} className={cx('btn h-7 text-xs', kind === k && 'ring-1 ring-violet-400')} onClick={() => setKind(k)}>{l}</button>)}</div>
        {(kind === 'say' || kind === 'move') && (
          <label className="block text-xs text-slate-400">Call
            <select className="input mt-0.5" value={call} onChange={(e) => setCall(e.target.value)}><option value="">—</option>{calls.map((c) => <option key={c.id} value={c.id}>{c.callerName || c.callback} · {c.id}</option>)}</select>
          </label>
        )}
        {needPos && (
          <div>
            <label className="label" htmlFor="inj-addr">Where</label>
            <AddressInput id="inj-addr" value={address} onChange={setAddress} onPick={(r) => setPos({ lat: r.lat, lon: r.lon })} placeholder="An address in your area" />
            {pos && <p className="mt-0.5 font-mono text-[11px] text-slate-500">{pos.lat.toFixed(5)}, {pos.lon.toFixed(5)}</p>}
          </div>
        )}
        {kind === 'call' && (
          <div className="grid grid-cols-2 gap-2">
            <label className="text-xs text-slate-400">Caller name<input className="input mt-0.5" value={name} onChange={(e) => setName(e.target.value)} placeholder="SMITH, A" /></label>
            <label className="text-xs text-slate-400">Callback<input className="input mt-0.5 font-mono" value={callback} onChange={(e) => setCallback(e.target.value)} /></label>
          </div>
        )}
        {(kind === 'call' || kind === 'say') && (
          <label className="block text-xs text-slate-400">What is said: one line each, <span className="font-mono">seconds | words</span> (the first line answers the call)
            <textarea className="input mt-0.5 font-mono text-xs" rows={5} value={lines} onChange={(e) => setLines(e.target.value)} />
          </label>
        )}
        {(kind === 'unit' || kind === 'emergency' || kind === 'radio') && (
          <div className="grid grid-cols-2 gap-2">
            <label className="text-xs text-slate-400">Unit<select className="input mt-0.5" value={unit} onChange={(e) => setUnit(e.target.value)}><option value="">—</option>{view.units.map((u) => <option key={u.id}>{u.id}</option>)}</select></label>
            {kind === 'unit' && <label className="text-xs text-slate-400">Becomes<select className="input mt-0.5" value={status} onChange={(e) => setStatus(e.target.value)}>{['oos', 'available'].map((s) => <option key={s}>{s}</option>)}</select></label>}
            {kind === 'radio' && <label className="text-xs text-slate-400">Says<input className="input mt-0.5" value={text} onChange={(e) => setText(e.target.value)} placeholder="Second floor fully involved, need a second alarm" /></label>}
          </div>
        )}
        <p className="text-xs text-slate-500">Every inject is recorded on the exercise's timeline.</p>
      </div>
    </Modal>
  )
}

function Staging({ onClose }: { onClose: () => void }) {
  const [diff, setDiff] = useState<Record<string, string[]> | null>(null)
  const [parts, setParts] = useState<string[]>([])
  const [why, setWhy] = useState('')
  useEffect(() => {
    api.get<Record<string, string[]>>('/training/staged').then((d) => { setDiff(d); setParts(Object.keys(d)) }).catch((e) => toast.error(errText(e)))
  }, [])
  const publish = async () => {
    try {
      const r = await api.post<{ changes: number }>('/training/publish', { parts, why })
      toast.success(`${r.changes} changes published to the live board`)
      onClose()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const empty = diff && Object.keys(diff).length === 0
  return (
    <Modal open onClose={onClose} wide title="Staged configuration" icon={<UploadCloud size={16} className="text-violet-300" aria-hidden />}
      footer={<><button className="btn" onClick={onClose}>Close</button><button className="btn btn-go" disabled={!parts.length || !why.trim() || !!empty} onClick={publish}>Publish to the live board</button></>}>
      <div className="space-y-2 text-sm">
        <p className="text-slate-400">Changes made here in Administration, tried on this board. Publishing copies the chosen parts to the live board, as you, with your reason.</p>
        {empty && <p className="text-slate-500">Nothing differs from the live board.</p>}
        {diff && Object.entries(diff).map(([part, lines]) => (
          <label key={part} className="block rounded border border-slate-800 p-2">
            <span className="flex items-center gap-2 font-semibold text-slate-100"><input type="checkbox" checked={parts.includes(part)} onChange={(e) => setParts(e.target.checked ? [...parts, part] : parts.filter((x) => x !== part))} /> {part}</span>
            <span className="mt-0.5 block font-mono text-xs text-slate-400">{lines.join(' · ')}</span>
          </label>
        ))}
        <input className="input" placeholder="Why (recorded on the live board)" value={why} onChange={(e) => setWhy(e.target.value)} aria-label="Reason" />
      </div>
    </Modal>
  )
}
