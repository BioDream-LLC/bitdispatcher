import { useEffect, useState } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import { toast } from 'sonner'
import { ShieldAlert, X } from 'lucide-react'
import { api, errText } from '../api'
import { cx } from '../util'
import type { Meta } from './Rms'

type Answer = { text: string; hit: boolean; simulated: boolean; at: string; message: string }

/** An NCIC inquiry: the fields for the kind, the purpose and reason CJIS requires, and the answer. */
export default function NcicPanel({ open, onOpenChange, meta, reason = '', seed }: { open: boolean; onOpenChange: (o: boolean) => void; meta: Meta; reason?: string; seed?: { key: string; fields: Record<string, string> } }) {
  const [key, setKey] = useState('QV')
  const [fields, setFields] = useState<Record<string, string>>({})
  const [purpose, setPurpose] = useState('C')
  const [why, setWhy] = useState(reason)
  const [busy, setBusy] = useState(false)
  const [answers, setAnswers] = useState<Answer[]>([])
  useEffect(() => {
    if (open) {
      setWhy(reason)
      if (seed) { setKey(seed.key); setFields(seed.fields) }
    }
  }, [open, reason, seed])
  const k = meta.ncic.kinds.find((x) => x.key === key)!
  const send = async () => {
    setBusy(true)
    try {
      const a = await api.post<Answer>('/ncic/query', { key, fields, purpose, reason: why })
      setAnswers((l) => [a, ...l].slice(0, 10))
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/60" />
        <Dialog.Content className="fixed left-1/2 top-10 z-50 max-h-[86vh] w-[min(52rem,96vw)] -translate-x-1/2 overflow-y-auto rounded-lg border border-slate-700 bg-slate-900 p-4 shadow-2xl"
          onKeyDown={(e) => e.key === 'Enter' && !e.shiftKey && (e.target as HTMLElement).tagName === 'INPUT' && send()}>
          <Dialog.Title className="flex items-center gap-2 font-semibold text-slate-100"><ShieldAlert size={16} className="text-sky-300" aria-hidden /> NCIC inquiry
            {meta.ncic.mode === 'sim' && <span className="chip bg-violet-600/30 text-violet-200">training simulator</span>}
            <span className="ml-auto font-mono text-xs text-slate-500">ORI {meta.ncic.ori || 'not set'}</span>
            <Dialog.Close className="btn h-7 w-7 justify-center p-0" aria-label="Close"><X size={14} /></Dialog.Close></Dialog.Title>
          <Dialog.Description className="mb-2 text-xs text-slate-500">Every inquiry is logged with who, the purpose and the reason; the terms only as a fingerprint, the answer not at all. A hit is a lead until the entering agency confirms it.</Dialog.Description>
          <div className="mb-2 flex gap-1" role="group" aria-label="Kind">
            {meta.ncic.kinds.map((x) => <button key={x.key} className={cx('btn text-xs', key === x.key && 'ring-1 ring-sky-400')} onClick={() => { setKey(x.key); setFields({}) }}>{x.key} {x.name}</button>)}
          </div>
          <div className="grid gap-2 md:grid-cols-3">
            {k.fields.map((f, i) => (
              <label key={f} className="text-[11px] text-slate-400">{meta.ncic.fields[f]} <span className="font-mono text-slate-500">{f}</span>
                <input className="input mt-0.5 font-mono uppercase" autoFocus={i === 0} value={fields[f] ?? ''} onChange={(e) => setFields({ ...fields, [f]: e.target.value.toUpperCase() })} />
              </label>
            ))}
          </div>
          <div className="mt-2 grid gap-2 md:grid-cols-[14rem_1fr_auto]">
            <select className="input text-xs" value={purpose} onChange={(e) => setPurpose(e.target.value)} aria-label="Purpose">
              {Object.entries(meta.ncic.purposes).map(([c, n]) => <option key={c} value={c}>{c}: {n}</option>)}
            </select>
            <input className="input text-xs" placeholder="Reason: incident number or what it is for" value={why} onChange={(e) => setWhy(e.target.value)} aria-label="Reason" />
            <button className="btn btn-go" disabled={busy} onClick={send}>Send ↵</button>
          </div>
          <p className="mt-1 text-[10px] text-slate-500">Needs {k.need.map((n) => n.join(' + ')).join(' or ')}.</p>
          <div className="mt-3 space-y-2">
            {answers.map((a, i) => (
              <div key={i} className={cx('rounded border p-2', a.hit ? 'border-rose-600 bg-rose-950/40' : 'border-slate-700 bg-slate-950')}>
                <div className="mb-1 flex items-center gap-2 text-[11px]">
                  {a.hit ? <b className="text-rose-300">RECORD RETURNED: CONFIRM BEFORE ACTING</b> : <span className="text-emerald-300">no record</span>}
                  <span className="font-mono text-slate-500">{a.message}</span>
                  <span className="ml-auto text-slate-500">{new Date(a.at).toLocaleTimeString()}</span>
                </div>
                <pre className="whitespace-pre-wrap font-mono text-xs text-slate-200">{a.text}</pre>
              </div>
            ))}
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
