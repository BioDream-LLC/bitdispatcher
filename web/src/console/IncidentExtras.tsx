import { useState } from 'react'
import { BookOpen, ChevronDown, ChevronRight } from 'lucide-react'
import { cx } from '../util'
import type { Incident, View } from '../types'
import { FlagsBox, useFlags } from '../taker/Flags'
import ResourcesTab, { useResources } from '../taker/Resources'
import { emptyDraft } from '../taker/draft'

/** The incident's place on one line, with an unverified or forced location named in capitals and the
 * address entered kept beside the accepted one. */
export function PlaceLine({ inc }: { inc: Incident }) {
  const p = inc.place
  if (!p) return null
  const parts = [p.name, p.complex !== p.name ? p.complex : '', p.building && 'Bldg ' + p.building, p.floor && 'Floor ' + p.floor, p.unit && (p.unitKind || 'Unit') + ' ' + p.unit].filter(Boolean)
  const un = p.verify && p.verify !== 'verified'
  return (
    <div className="mt-0.5 space-y-0.5 text-xs">
      {un && (
        <div className={cx('rounded px-1.5 py-0.5 font-semibold', p.verify === 'forced' ? 'bg-rose-950/60 text-rose-100' : 'bg-amber-950/50 text-amber-100')} role="status">
          {p.verify.toUpperCase()} LOCATION: entered as {JSON.stringify(p.entered ?? '')}{p.reason ? ` (${p.reason})` : ''}{p.nearest ? `; nearest verified: ${p.nearest}` : ''}
        </div>
      )}
      {p.linked && <div className="text-emerald-300">Address {p.linked.how} as {p.linked.address} by {p.linked.by}</div>}
      {parts.length > 0 && <div className="text-slate-200">{parts.join(', ')}</div>}
      {p.detail && <div className="text-slate-400">Access: {p.detail}</div>}
    </div>
  )
}

/** The flags that apply to the incident now (and possible matches near an unverified location). */
export function IncidentFlags({ inc }: { inc: Incident }) {
  const set = useFlags(emptyDraft(), inc)
  return <FlagsBox set={set} />
}

/** The approved resources for the incident, for the dispatcher: the same guidance the call-taker saw,
 * opened beside the incident without leaving it. */
export function DispatcherResources({ view, inc }: { view: View; inc: Incident }) {
  const [open, setOpen] = useState(false)
  const res = useResources(view, { ...emptyDraft(), type: inc.type, modifiers: inc.modifiers ?? [] }, inc, undefined, open)
  if (!view.entry?.guidance) return null
  return (
    <section className="rounded border border-slate-800">
      <button type="button" className="flex w-full items-center gap-1.5 px-2 py-1 text-left text-xs font-semibold text-slate-200" onClick={() => setOpen(!open)} aria-expanded={open}>
        {open ? <ChevronDown size={12} aria-hidden /> : <ChevronRight size={12} aria-hidden />} <BookOpen size={12} aria-hidden /> Resources for this incident
        {res.count > 0 && <span className="chip ml-auto h-4 bg-slate-700 px-1 text-[10px]">{res.count}</span>}
      </button>
      {open && <div className="border-t border-slate-800 p-2"><ResourcesTab res={res} inc={inc} /></div>}
    </section>
  )
}
