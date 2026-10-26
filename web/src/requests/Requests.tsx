import { Queue } from './Queue'

/** The full-page NCIC and state request queue: every DMV, VCIC, NCIC, Nlets and other-state transaction
 * and every hit confirmation, filtered. Metadata only, never a return's text. */
export default function Requests() {
  return (
    <div className="h-full overflow-y-auto bg-slate-950 p-4">
      <Queue />
    </div>
  )
}
