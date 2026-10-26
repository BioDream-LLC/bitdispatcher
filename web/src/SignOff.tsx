import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { FileSearch } from 'lucide-react'
import { Modal } from './ui'

type Req = { id: string; type: string; category: string; subject?: string; status: string; due?: string; incident?: string }

/** Shown when signing off is refused: the open NCIC or state requests the person owns, each to finish or
 * hand to another dispatcher (or a queue) first, so none is silently left behind at shift change. */
export function SignOffBlocked() {
  const [reqs, setReqs] = useState<Req[] | null>(null)
  const [msg, setMsg] = useState('')
  useEffect(() => {
    const on = (e: Event) => {
      const d = (e as CustomEvent).detail as { message?: string; requests?: Req[] } | undefined
      setMsg(d?.message ?? '')
      setReqs(d?.requests ?? [])
    }
    window.addEventListener('bd:signoff-blocked', on)
    return () => window.removeEventListener('bd:signoff-blocked', on)
  }, [])
  if (!reqs) return null
  return (
    <Modal open onClose={() => setReqs(null)} title="Finish or hand over your requests first" icon={<FileSearch size={16} aria-hidden />}
      footer={<Link to="/requests?mine=1" className="btn btn-go" onClick={() => setReqs(null)}>Open my requests</Link>}>
      <p className="mb-2 text-sm text-slate-300">{msg}</p>
      <ul className="space-y-1 text-sm">
        {reqs.map((r) => (
          <li key={r.id} className="rounded border border-slate-700 px-2 py-1">
            <span className="font-mono font-bold">{r.id}</span> {r.type} {r.category} {r.subject} <span className="text-slate-400">· {r.status}{r.incident ? ` · ${r.incident}` : ''}{r.due ? ` · due ${new Date(r.due).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}` : ''}</span>
          </li>
        ))}
      </ul>
      <p className="mt-2 text-xs text-slate-400">Hand each one to the dispatcher taking over (they accept it) or to a queue, with a note of what is outstanding.</p>
    </Modal>
  )
}
