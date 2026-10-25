import { useEffect, useState } from 'react'
import { Ban } from 'lucide-react'
import { api } from '../api'
import type { RoadsView } from '../types'

/** MdtRoads is the roads closed now, for the crew: what, where, the detour. */
export default function MdtRoads() {
  const [roads, setRoads] = useState<RoadsView | null>(null)
  useEffect(() => {
    const load = () => api.get<RoadsView>('/roads').then(setRoads).catch(() => {})
    load()
    const t = window.setInterval(load, 30000)
    return () => clearInterval(t)
  }, [])
  if (!roads?.closures.length) return null
  return (
    <section aria-label="Roads closed" className="rounded-lg border border-rose-900/70 bg-rose-950/20 p-2">
      <div className="mb-1 flex items-center gap-1.5 text-sm font-bold text-rose-200"><Ban size={14} aria-hidden /> Roads closed · {roads.closures.length}</div>
      <ul className="space-y-1 text-sm">
        {roads.closures.map((c) => (
          <li key={c.id}>
            <span className="font-semibold text-slate-100">{(c.roads ?? []).join(', ') || 'Drawn area'}</span> <span className="text-slate-300">· {c.reason}</span>
            {c.detour && <div className="text-xs text-slate-400">Detour: {c.detour}</div>}
            {c.until && <div className="text-xs text-slate-500">until {new Date(c.until).toLocaleString([], { hour: '2-digit', minute: '2-digit', month: 'short', day: 'numeric' })}</div>}
          </li>
        ))}
      </ul>
    </section>
  )
}
