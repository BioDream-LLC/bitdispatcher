import { Queue } from './Queue'

/** RequestsTab is the request queue filtered to one incident, compact, for the call-taker's incident
 * workspace. */
export function RequestsTab({ incident }: { incident?: string }) {
  return (
    <div className="space-y-2">
      <Queue incident={incident} compact />
    </div>
  )
}

export default RequestsTab
