import { useRef, useState } from 'react'
import { toast } from 'sonner'
import { FileText, Paperclip, Trash2, Upload, X } from 'lucide-react'
import { api, ApiError, errText } from './api'
import { can, useStore } from './store'
import type { Incident, IncidentFile } from './types'

const src = (n: string, f: IncidentFile) => `./api/incidents/${encodeURIComponent(n)}/files/${f.id}`
const size = (b: number) => (b > 1 << 20 ? `${(b / (1 << 20)).toFixed(1)} MB` : `${Math.max(1, Math.round(b / 1024))} KB`)

/** IncidentFiles shows an incident's attachments (pictures as thumbnails, others as links) and adds
 * them: a dispatcher's upload, a crew's camera on the terminal. Each file's SHA-256 is in the ledger; a
 * supervisor removes one with why, and it stays in the record, hidden. */
export default function IncidentFiles({ inc, big, onChanged }: { inc: Incident; big?: boolean; onChanged?: () => void }) {
  const me = useStore((s) => s.me)
  const [busy, setBusy] = useState(false)
  const [caption, setCaption] = useState('')
  const [open, setOpen] = useState<IncidentFile | null>(null)
  const input = useRef<HTMLInputElement>(null)
  const files = (inc.files ?? []).filter((f) => !f.hidden)
  const mayAdd = !!me && (me.role === 'field' || can.dispatch(me))
  if (!files.length && !mayAdd) return null
  const send = async (list: FileList | null) => {
    if (!list?.length) return
    setBusy(true)
    try {
      for (const f of Array.from(list)) {
        const q = new URLSearchParams({ name: f.name, caption })
        const r = await fetch(`./api/incidents/${encodeURIComponent(inc.number)}/files?${q}`, { method: 'POST', body: f, headers: { 'X-BitDispatcher': '1' }, credentials: 'same-origin' })
        if (!r.ok) throw new ApiError(r.status, ((await r.json().catch(() => ({}))) as { message?: string }).message ?? r.statusText)
      }
      setCaption('')
      toast.success(list.length > 1 ? `${list.length} files added` : 'File added')
      onChanged?.()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
      if (input.current) input.current.value = ''
    }
  }
  const remove = async (f: IncidentFile) => {
    const why = prompt(`Remove ${f.name}? It stays in the record, hidden. Why?`)
    if (!why) return
    await api.del(`/incidents/${inc.number}/files/${f.id}`, { why }).catch((e) => toast.error(errText(e)))
    onChanged?.()
  }
  return (
    <section aria-label="Files" className="space-y-1.5">
      <div className="flex items-center gap-1.5 text-xs text-slate-400">
        <Paperclip size={12} aria-hidden /> Files {files.length > 0 && <span className="text-slate-500">· {files.length}</span>}
        {mayAdd && (
          <span className="ml-auto flex items-center gap-1">
            {!big && <input className="input h-6 w-40 text-[11px]" value={caption} onChange={(e) => setCaption(e.target.value)} placeholder="Caption (optional)" aria-label="Caption" />}
            <button className={big ? 'btn h-10 px-4 text-base' : 'btn h-6 px-2 text-[11px]'} disabled={busy} onClick={() => input.current?.click()}>
              <Upload size={big ? 16 : 12} aria-hidden /> {busy ? 'Sending…' : big ? 'Photo or file' : 'Add'}
            </button>
            <input ref={input} type="file" multiple hidden accept="image/*,application/pdf,text/plain,audio/*,video/*" onChange={(e) => send(e.target.files)} aria-label="Choose files" />
          </span>
        )}
      </div>
      {files.length > 0 && (
        <ul className="flex flex-wrap gap-1.5">
          {files.map((f) => (
            <li key={f.id} className="group relative">
              {f.type.startsWith('image/') ? (
                <button className="block overflow-hidden rounded border border-slate-700" onClick={() => setOpen(f)} title={`${f.name}${f.caption ? ` · ${f.caption}` : ''} · ${f.unit || f.by}`}>
                  <img src={src(inc.number, f)} alt={f.caption || f.name} className={big ? 'h-24 w-32 object-cover' : 'h-14 w-20 object-cover'} loading="lazy" />
                </button>
              ) : (
                <a className="flex h-14 max-w-48 items-center gap-1.5 rounded border border-slate-700 px-2 text-[11px] text-slate-200 hover:bg-slate-800" href={src(inc.number, f)} target="_blank" rel="noopener" title={f.caption}>
                  <FileText size={16} className="shrink-0 text-slate-400" aria-hidden /><span className="truncate">{f.name}</span><span className="shrink-0 text-slate-500">{size(f.size)}</span>
                </a>
              )}
              {can.supervise(me) && (
                <button className="absolute right-0.5 top-0.5 hidden rounded bg-slate-900/90 p-0.5 text-rose-300 group-hover:block" aria-label={`Remove ${f.name}`} onClick={() => remove(f)}><Trash2 size={11} aria-hidden /></button>
              )}
            </li>
          ))}
        </ul>
      )}
      {open && (
        <div role="dialog" aria-label={open.name} className="fixed inset-0 z-50 grid place-items-center bg-black/85 p-4" onClick={() => setOpen(null)}>
          <figure className="max-h-full max-w-full" onClick={(e) => e.stopPropagation()}>
            <img src={src(inc.number, open)} alt={open.caption || open.name} className="max-h-[85vh] max-w-full rounded" />
            <figcaption className="mt-1 flex items-center gap-2 text-sm text-slate-200">
              {open.caption || open.name} <span className="text-slate-400">· {open.unit || open.by} · {new Date(open.added).toLocaleString()}</span>
              <button className="btn btn-ghost ml-auto h-7" onClick={() => setOpen(null)} aria-label="Close"><X size={14} aria-hidden /></button>
            </figcaption>
          </figure>
        </div>
      )}
    </section>
  )
}
