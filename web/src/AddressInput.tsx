import { useEffect, useState } from 'react'
import { api } from './api'
import { cx } from './util'
import type { SearchResult } from './types'

/** An address field that suggests matches from the recorded map; picking one gives its position. */
export default function AddressInput({ id, value, onChange, onPick, placeholder }: {
  id: string; value: string; onChange: (v: string) => void; onPick: (r: SearchResult) => void; placeholder?: string
}) {
  const [res, setRes] = useState<SearchResult[]>([])
  const [open, setOpen] = useState(false)
  const [hi, setHi] = useState(0)
  useEffect(() => {
    if (!open || value.trim().length < 3) {
      setRes([])
      return
    }
    const t = setTimeout(() => api.get<SearchResult[]>(`/map/search?q=${encodeURIComponent(value)}&limit=6`).then((r) => { setRes(r ?? []); setHi(0) }).catch(() => setRes([])), 200)
    return () => clearTimeout(t)
  }, [value, open])
  const pick = (r: SearchResult) => {
    onChange(r.kind === 'address' || r.kind === 'intersection' ? r.label : `${r.label}${r.detail ? ', ' + r.detail : ''}`)
    onPick(r)
    setOpen(false)
  }
  return (
    <div className="relative">
      <input id={id} className="input" value={value} placeholder={placeholder} autoComplete="off" role="combobox" aria-expanded={open && res.length > 0} aria-controls={id + '-list'}
        onChange={(e) => { onChange(e.target.value); setOpen(true) }} onBlur={() => setTimeout(() => setOpen(false), 150)}
        onKeyDown={(e) => {
          if (!res.length) return
          if (e.key === 'ArrowDown') { e.preventDefault(); setHi((h) => Math.min(res.length - 1, h + 1)) }
          else if (e.key === 'ArrowUp') { e.preventDefault(); setHi((h) => Math.max(0, h - 1)) }
          else if (e.key === 'Enter') { e.preventDefault(); pick(res[hi]) }
          else if (e.key === 'Escape') setOpen(false)
        }} />
      {open && res.length > 0 && (
        <ul id={id + '-list'} role="listbox" className="absolute z-50 mt-1 w-full overflow-hidden rounded-md border border-slate-700 bg-slate-900 shadow-xl">
          {res.map((r, i) => (
            <li key={i} role="option" aria-selected={i === hi}>
              <button type="button" className={cx('flex w-full items-baseline gap-2 px-2 py-1 text-left text-sm', i === hi ? 'bg-slate-800' : 'hover:bg-slate-800')} onMouseDown={(e) => { e.preventDefault(); pick(r) }}>
                <span className="text-slate-100">{r.label}</span><span className="truncate text-xs text-slate-500">{r.detail}</span>
                {r.source === 'overture' && <span className="ml-auto text-[10px] text-violet-300">Overture</span>}{r.source === 'county' && <span className="ml-auto text-[10px] text-orange-300">county</span>}{r.exact && <span className="text-[10px] text-emerald-300">✓</span>}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
