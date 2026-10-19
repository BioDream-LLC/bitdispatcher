import { toast } from 'sonner'
import { Siren, Check } from 'lucide-react'
import { api, errText } from '../api'
import { can, store, useStore } from '../store'
import { clock, useNow } from '../util'
import type { View } from '../types'

// An emergency button takes the full width, flashing, until a person acknowledges it. It is never inside a
// dialog or a scrolled list, so nothing can hide it.
export default function EmergencyBanner({ view }: { view: View }) {
  const now = useNow()
  const me = useStore((s) => s.me)
  const open = view.emergencies.filter((e) => !e.ackedAt)
  if (open.length === 0) return null
  return (
    <div role="alert" aria-live="assertive" className="shrink-0">
      {open.map((e) => {
        const unit = e.unit ? view.units.find((u) => u.id === e.unit) : undefined
        return (
          <div key={e.key} className="flash flex items-center gap-3 px-3 py-1.5 text-white">
            <Siren size={20} aria-hidden />
            <span className="text-sm font-black tracking-wide">EMERGENCY</span>
            <span className="font-mono text-lg font-black">{e.unit || e.alias || `Radio ${e.radioId ?? e.radioUnit}`}</span>
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
            <span className="ml-auto font-mono text-lg font-bold tnum">{clock(e.raised, now)}</span>
            {can.dispatch(me) && (
              <button
                className="btn border-white/60 bg-white/15 text-white hover:bg-white/25"
                onClick={async () => {
                  try {
                    const r = await api.post<{ bitvoice: string }>(`/emergencies/${e.key}/ack`)
                    toast.success(`Acknowledged · BitVoice: ${r.bitvoice}`)
                  } catch (err) {
                    toast.error(errText(err))
                  }
                }}
              >
                <Check size={14} aria-hidden /> Acknowledge <span className="kbd border-white/40 bg-transparent text-white">ACK</span>
              </button>
            )}
          </div>
        )
      })}
    </div>
  )
}
