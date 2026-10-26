import { useEffect, useState } from 'react'
import { api } from '../api'
import { cx } from '../util'
import type { Code, CodeList, Codes } from './types'

/** useDebounced returns v after it has been still for ms (for live checks and match lookups). */
export function useDebounced<T>(v: T, ms = 400): T {
  const [x, setX] = useState(v)
  useEffect(() => {
    const t = setTimeout(() => setX(v), ms)
    return () => clearTimeout(t)
  }, [v, ms])
  return x
}

let codesCache: Codes | null = null
let codesPromise: Promise<Codes> | null = null

/** useCodes loads the agency's code lists once and shares them across the tabs. */
export function useCodes(): Codes | null {
  const [codes, setCodes] = useState<Codes | null>(codesCache)
  useEffect(() => {
    if (codesCache) {
      setCodes(codesCache)
      return
    }
    let live = true
    codesPromise ??= api.get<Codes>('/codes').then((c) => {
      codesCache = c
      return c
    })
    codesPromise.then((c) => live && setCodes(c)).catch(() => live && setCodes(null))
    return () => {
      live = false
    }
  }, [])
  return codes
}

/** listOf finds one code list by id. */
export function listOf(codes: Codes | null, id: string): CodeList | undefined {
  return codes?.lists.find((l) => l.id === id)
}

/** codeLabel is 'CODE label', or the raw code when it is not on the list. */
export function codeLabel(list: CodeList | undefined, code?: string): string {
  if (!code) return ''
  const c = list?.codes.find((x) => x.code.toUpperCase() === code.toUpperCase())
  return c ? `${c.code} ${c.label}` : code
}

/** CodeSelect is a labelled select over a code list, each option read as 'CODE label'. */
export function CodeSelect({ id, label, list, value, onChange, span2, autoFocus }: {
  id: string
  label: string
  list: CodeList | undefined
  value?: string
  onChange: (v: string) => void
  span2?: boolean
  autoFocus?: boolean
}) {
  const codes = list?.codes ?? []
  const known = !value || codes.some((c) => c.code.toUpperCase() === value.toUpperCase())
  return (
    <label className={cx('flex flex-col gap-0.5 text-[11px] text-slate-400', span2 && 'col-span-2')} htmlFor={id}>
      {label}
      <select id={id} className="input h-7 text-xs" value={value ?? ''} onChange={(e) => onChange(e.target.value)} autoFocus={autoFocus}>
        <option value="">{list ? `Choose ${list.name.toLowerCase()}` : '—'}</option>
        {!known && value ? <option value={value}>{value} (not on the list)</option> : null}
        {codes.map((c: Code) => (
          <option key={c.code} value={c.code}>
            {c.code} {c.label}
          </option>
        ))}
      </select>
    </label>
  )
}
