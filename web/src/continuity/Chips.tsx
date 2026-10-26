import { Link } from 'react-router-dom'
import { ArrowRightLeft, Building, LifeBuoy, WifiOff } from 'lucide-react'
import { useStore, can } from '../store'
import { cx, useNow } from '../util'
import type { View } from '../types'
import { live } from './types'

/** ContinuityChips sit in the status bar of every console: the internet down, the centre at its backup
 * site, an overflow under way or asked for. Each opens the page to act on it. */
export function ContinuityChips({ view }: { view: View }) {
  const now = useNow()
  const me = useStore((s) => s.me)
  const c = view.health.continuity
  const sup = can.supervise(me) || me?.duty === 'supervisor'
  if (!c) return null
  const down = c.down ?? []
  const ovf = (c.overflow ?? []).filter((o) => live(o, now))
  const chip = 'inline-flex h-6 items-center gap-1 whitespace-nowrap rounded px-1.5 text-[11px] font-bold'
  return (
    <>
      {c.internet === 'down' && (
        <Link to="/continuity" className={cx(chip, 'bg-rose-500/25 text-rose-100')} title="The internet is down. Dispatch keeps working here; see what waits for it.">
          <WifiOff size={12} aria-hidden /> Internet down
        </Link>
      )}
      {c.internet !== 'down' && down.length > 0 && (
        <Link to="/continuity" className={cx(chip, 'bg-amber-500/15 text-amber-200')} title={down.map((l) => `${l.name}: ${l.error || 'down'}`).join('\n')}>
          <WifiOff size={12} aria-hidden /> {down.length} link{down.length === 1 ? '' : 's'} down
        </Link>
      )}
      {c.relocation?.active && (
        <Link to="/continuity" className={cx(chip, 'bg-amber-500/25 text-amber-100')} title={`Declared by ${c.relocation.by}${c.relocation.note ? ': ' + c.relocation.note : ''}`}>
          <Building size={12} aria-hidden /> At backup site
        </Link>
      )}
      {ovf.map((o) => {
        const asked = o.status === 'asked'
        const text = o.direction === 'in'
          ? asked ? `${o.name} asks for help` : `Taking calls for ${o.name}`
          : asked ? `Asked ${o.name} for help` : `${o.name} is taking our calls`
        return (
          <Link key={o.direction + o.partner} to="/overflow" className={cx(chip, o.direction === 'in' && asked ? 'flash text-white' : 'bg-sky-500/20 text-sky-100')}
            title={`Overflow until ${new Date(o.until).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`}>
            <ArrowRightLeft size={12} aria-hidden /> {text}
          </Link>
        )
      })}
      {(sup || ovf.length > 0) && (
        <Link to="/continuity" className="btn btn-ghost px-1.5" aria-label="Continuity: links, backup site, overflow" title="Continuity: links, backup site, overflow and the continuity pack">
          <LifeBuoy size={14} aria-hidden />
        </Link>
      )}
    </>
  )
}
