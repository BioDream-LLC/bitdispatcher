import { useEffect, useState } from 'react'
import { BellRing, Check, MapPin, X } from 'lucide-react'
import { Logo } from './ui'
import { cx } from './util'

type Info = { id: string; at: string; subject: string; text: string; address?: string; name: string; agency?: string; answer?: { answer: string; eta?: number; at: string } | null }

/** The page a paged responder opens from the link in their text or email: no sign-in, the link is the key.
 * They answer responding (to the station), straight to the scene, or not available, with when they will
 * arrive; the dispatcher sees the answer on the incident at once. */
export default function PageReply() {
  const [, , id, token] = window.location.hash.split('/') // #/r/<page>/<token>
  const [info, setInfo] = useState<Info | null>(null)
  const [err, setErr] = useState('')
  const [eta, setEta] = useState(10)
  const [sent, setSent] = useState('')
  useEffect(() => {
    fetch(`./api/page-reply/${encodeURIComponent(id ?? '')}?t=${encodeURIComponent(token ?? '')}`, { cache: 'no-store' })
      .then(async (r) => (r.ok ? setInfo(await r.json()) : setErr((await r.json().catch(() => ({}))).message ?? 'This page link is not valid')))
      .catch(() => setErr('The dispatch centre is not answering. Call in by radio or phone.'))
  }, [id, token])
  const answer = async (a: 'responding' | 'scene' | 'unavailable') => {
    const r = await fetch(`./api/page-reply/${encodeURIComponent(id)}`, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-BitDispatcher': '1' }, body: JSON.stringify({ t: token, answer: a, eta }) })
    if (r.ok || r.status === 204) setSent(a)
    else setErr((await r.json().catch(() => ({}))).message ?? 'Not sent')
  }
  return (
    <main className="mx-auto flex min-h-full max-w-md flex-col gap-4 bg-slate-950 p-4 text-slate-100">
      <header className="flex items-center gap-2"><Logo size={22} /> <span className="font-semibold">{info?.agency || 'Dispatch'}</span></header>
      {err && <p role="alert" className="rounded border border-rose-600 bg-rose-950/40 p-3 text-sm">{err}</p>}
      {!info && !err && <p className="text-sm text-slate-400">Loading the page…</p>}
      {info && (
        <>
          <section className="rounded-lg border border-rose-700/60 bg-rose-950/20 p-3">
            <div className="flex items-center gap-2 text-xs uppercase tracking-wider text-rose-300"><BellRing size={14} aria-hidden /> Page {info.id} · {new Date(info.at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</div>
            <h1 className="mt-1 text-xl font-black">{info.subject}</h1>
            {info.address && <a className="mt-1 flex items-center gap-1 text-sky-300 underline" href={`https://www.openstreetmap.org/search?query=${encodeURIComponent(info.address)}`} target="_blank" rel="noreferrer"><MapPin size={14} aria-hidden /> {info.address}</a>}
            {info.text && <p className="mt-2 text-sm text-slate-200">{info.text}</p>}
          </section>
          <p className="text-sm text-slate-400">For {info.name}.{info.answer && !sent ? ` You answered: ${label(info.answer.answer)}.` : ''}</p>
          {sent ? (
            <p role="status" className="flex items-center gap-2 rounded border border-emerald-600 bg-emerald-950/40 p-3 text-lg font-bold"><Check aria-hidden /> Sent: {label(sent)}{sent !== 'unavailable' ? `, about ${eta} min` : ''}</p>
          ) : (
            <>
              <label className="text-sm" htmlFor="eta">I will be there in about <b>{eta}</b> minutes</label>
              <input id="eta" type="range" min={1} max={60} value={eta} onChange={(e) => setEta(Number(e.target.value))} className="w-full" />
              <div className="grid gap-2">
                <button className="rounded-lg bg-emerald-600 py-4 text-lg font-black text-white" onClick={() => answer('responding')}>Responding to the station</button>
                <button className="rounded-lg bg-sky-600 py-4 text-lg font-black text-white" onClick={() => answer('scene')}>Going straight to the scene</button>
                <button className={cx('flex items-center justify-center gap-2 rounded-lg border border-slate-600 py-3 text-base font-bold text-slate-200')} onClick={() => answer('unavailable')}><X size={16} aria-hidden /> Not available</button>
              </div>
            </>
          )}
          <p className="text-xs text-slate-500">Answering here tells the dispatcher at once. It does not replace the radio: report in as your agency requires.</p>
        </>
      )}
    </main>
  )
}

function label(a: string) {
  return a === 'responding' ? 'responding to the station' : a === 'scene' ? 'going to the scene' : 'not available'
}
