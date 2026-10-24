import { useEffect, useState } from 'react'
import { api } from './api'
import type { ViewDef } from './types'

// The supervisory views the agency has given this person (the server decides; this only hides what
// would be refused). Fetched once per sign-in.
let cache: { key: string; ids: Set<string> } | null = null
let pending: Promise<Set<string>> | null = null

export function useViews(key: string | undefined): Set<string> | null {
  const [ids, setIds] = useState<Set<string> | null>(cache && cache.key === key ? cache.ids : null)
  useEffect(() => {
    if (!key) return
    if (cache?.key === key) {
      setIds(cache.ids)
      return
    }
    pending ??= api.get<{ items: ViewDef[] }>('/me/views').then((r) => new Set(r.items.map((v) => v.id))).catch(() => new Set<string>())
    pending.then((s) => {
      cache = { key, ids: s }
      pending = null
      setIds(s)
    })
  }, [key])
  return ids
}

/** The admin pages that are supervisory views, by their view ID. */
export const VIEW_OF_PAGE: Record<string, string> = { reports: 'operations', quality: 'quality', ledger: 'ledger' }
