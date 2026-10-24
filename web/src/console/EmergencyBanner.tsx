import { useState } from 'react'
import { toast } from 'sonner'
import { Siren, Check, ChevronDown, ChevronUp } from 'lucide-react'
import { api, errText } from '../api'
import { can, store, useStore } from '../store'
import { clock, useNow } from '../util'
import type { Emergency, View } from '../types'

// An emergency button takes the full width, flashing, until a person acknowledges it. It is never inside a
// dialog or a scrolled list, so nothing can hide it. Several open at once share one bar: the newest
// flashes, and the bar says how many more are open; they are listed one click away.
export default function EmergencyBanner({ view }: { view: View }) {
  const now = useNow()
  const me = useStore((s) => s.me)
  const [list, setList] = useState(false)
  const open = view.emergencies.filter((e) => !e.ackedAt).sort((a, b) => b.raised.localeCompare(a.raised))
  if (open.length === 0) return null
  const ack = async (e: Emergency) => {
    try {
      const r = await api.post<{ bitvoice: string }>(`/emergencies/${e.key}/ack`)
      toast.success(`${who(e)} acknowledged · BitVoice: ${r.bitvoice}`)
    } catch (err) {
      toast.error(errText(err))
    }
  }
  const ackAll = async () => {
    if (!confirm(`Acknowledge all ${open.length} open emergencies? Each is recorded as acknowledged by you.`)) return
    let failed = 0
    for (const e of open) {
      try {
        await api.post(`/emergencies/${e.key}/ack`)
      } catch {
        failed++
      }
    }
    if (failed) toast.error(`${failed} could not be acknowledged`)
    else toast.success(`All ${open.length} acknowledged`)
    setList(false)
  }
  const [top, ...rest] = open
  return (
    <div role="alert" aria-live="assertive" className="relative shrink-0">
      <Row e={top} view={view} now={now} canAck={can.dispatch(me)} onAck={ack}>
        {rest.length > 0 && (
          <button
            className="btn border-white/60 bg-white/15 text-white hover:bg-white/25"
            aria-expanded={list}
            onClick={() => setList(!list)}
          >
            {list ? <ChevronUp size={14} aria-hidden /> : <ChevronDown size={14} aria-hidden />}
            {rest.length} more open
          </button>
        )}
      </Row>
      {list && rest.length > 0 && (
        <div className="absolute inset-x-0 top-full z-50 border-b-2 border-red-500 bg-red-950/95 shadow-xl" aria-label="Other open emergencies">
          {rest.map((e) => (
            <Row key={e.key} e={e} view={view} now={now} canAck={can.dispatch(me)} onAck={ack} quiet />
          ))}
          {can.dispatch(me) && (
            <div className="flex justify-end px-3 py-1.5">
              <button className="btn border-white/60 bg-white/10 text-white hover:bg-white/25" onClick={ackAll}>
                <Check size={14} aria-hidden /> Acknowledge all {open.length}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function who(e: Emergency) {
  return e.unit || e.alias || `Radio ${e.radioId ?? e.radioUnit}`
}

function Row({ e, view, now, canAck, onAck, quiet, children }: {
  e: Emergency; view: View; now: number; canAck: boolean; onAck: (e: Emergency) => void; quiet?: boolean; children?: React.ReactNode
}) {
  const unit = e.unit ? view.units.find((u) => u.id === e.unit) : undefined
  return (
    <div className={`${quiet ? 'border-t border-red-800/70' : 'flash'} flex items-center gap-3 px-3 py-1.5 text-white`}>
      <Siren size={quiet ? 16 : 20} aria-hidden />
      <span className="text-sm font-black tracking-wide">EMERGENCY</span>
      <span className={`font-mono font-black ${quiet ? 'text-base' : 'text-lg'}`}>{who(e)}</span>
      <span className="text-sm">
        {e.channelName || e.channel ? `on ${e.channelName || e.channel}` : ''}
        {!e.unit && ' · radio not mapped to a unit'}
        {unit?.incident && (
          <button className="ml-2 underline" onClick={() => store.select(unit.incident!)}>
            on {unit.incident}
          </button>
        )}
      </span>
      {unit?.position && <span className="font-mono text-xs opacity-90">AVL {unit.position.lat.toFixed(4)}, {unit.position.lon.toFixed(4)}</span>}
      <span className={`ml-auto font-mono font-bold tnum ${quiet ? 'text-sm' : 'text-lg'}`}>{clock(e.raised, now)}</span>
      {children}
      {canAck && (
        <button className="btn border-white/60 bg-white/15 text-white hover:bg-white/25" onClick={() => onAck(e)} aria-label={`Acknowledge ${who(e)}`}>
          <Check size={14} aria-hidden /> Acknowledge {!quiet && <span className="kbd border-white/40 bg-transparent text-white">ACK</span>}
        </button>
      )}
    </div>
  )
}
