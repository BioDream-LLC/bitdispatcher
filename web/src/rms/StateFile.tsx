import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Download, Landmark } from 'lucide-react'
import { api, errText } from '../api'

type Preview = { summary: { incidents: number; arrests: number; zero: boolean; segments: number; skipped?: string[] }; reports: string[]; ori: string; due: string }

const lastMonth = () => {
  const d = new Date()
  d.setDate(1)
  d.setMonth(d.getMonth() - 1)
  return d.toISOString().slice(0, 7)
}

/** The month's file for the state's crime repository: approved reports, in the state's format, one click. */
export default function StateFile() {
  const [month, setMonth] = useState(lastMonth)
  const [p, setP] = useState<Preview | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => {
    setErr('')
    api.get<Preview>(`/rms/ibr?month=${month}`).then(setP).catch((e) => { setP(null); setErr(errText(e)) })
  }, [month])
  const download = async () => {
    try {
      const res = await fetch(`./api/rms/ibr?month=${month}`, { method: 'POST', headers: { 'X-BitDispatcher': '1' }, credentials: 'same-origin' })
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).message ?? res.statusText)
      const url = URL.createObjectURL(await res.blob())
      const a = document.createElement('a')
      a.href = url
      a.download = /filename="([^"]+)"/.exec(res.headers.get('Content-Disposition') ?? '')?.[1] ?? `ibr-${month}.txt`
      a.click()
      URL.revokeObjectURL(url)
      toast.success('Made the state file: upload it to the Virginia Crime Repository')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section className="space-y-1 border-t border-slate-800 p-2 text-xs" aria-label="State reporting">
      <h2 className="flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400"><Landmark size={12} aria-hidden /> State IBR file</h2>
      <div className="flex gap-1">
        <input type="month" className="input h-7 text-xs" value={month} onChange={(e) => setMonth(e.target.value)} aria-label="Month" />
        <button className="btn btn-go h-7 text-xs" disabled={!p} onClick={download}><Download size={12} aria-hidden /> File</button>
      </div>
      {err && <p className="text-amber-300">{err}</p>}
      {p && (
        <p className="text-slate-400">
          {p.summary.zero ? 'Nothing approved: a zero report' : `${p.summary.incidents} incidents · ${p.summary.arrests} arrests`} for {p.ori} · due {p.due}
          {(p.summary.skipped?.length ?? 0) > 0 && <span className="block text-amber-300">Left out: {p.summary.skipped!.join('; ')}</span>}
        </p>
      )}
    </section>
  )
}
