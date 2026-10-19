import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Copy, Fingerprint, Network, Plus, Save, Trash2 } from 'lucide-react'
import { api, errText } from '../api'
import { Field } from '../ui'
import { ago, cx, useNow } from '../util'
import { Page } from './Admin'

type Peer = { name: string; url: string; token: string; fingerprint: string }
type Health = { name: string; url: string; ok: boolean; last?: string; error?: string; copied: number }
type SitesView = { name: string; prefix: string; token: string; fingerprint: string; peers: Peer[]; health: Health[]; fixed: boolean }

const copy = (s: string) => navigator.clipboard.writeText(s).then(() => toast.success('Copied'))

/** Several sites, active-active: each keeps working alone and catches up when the link returns. */
export default function Sites() {
  const now = useNow()
  const [v, setV] = useState<SitesView | null>(null)
  const [d, setD] = useState<SitesView | null>(null)
  const load = () => api.get<SitesView>('/admin/sites').then((x) => { setV(x); setD(x) }).catch((e) => toast.error(errText(e)))
  useEffect(() => { load() }, [])
  useEffect(() => {
    const t = setInterval(() => api.get<SitesView>('/admin/sites').then((x) => setV((cur) => (cur ? { ...cur, health: x.health } : x))).catch(() => {}), 3000)
    return () => clearInterval(t)
  }, [])
  if (!v || !d) return <p className="text-sm text-slate-500">Loading…</p>
  const save = async () => {
    try {
      const x = await api.put<SitesView>('/admin/sites', d)
      setV(x)
      setD(x)
      toast.success('Saved')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const upPeer = (i: number, p: Partial<Peer>) => setD({ ...d, peers: d.peers.map((x, j) => (j === i ? { ...x, ...p } : x)) })
  const read = async (i: number) => {
    try {
      const r = await api.post<{ fingerprint: string }>('/admin/sites/fingerprint', { url: d.peers[i].url })
      upPeer(i, { fingerprint: r.fingerprint })
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Page
      title="Sites"
      intro="Run bitdispatcher at two or more sites, all live at once. Each site writes its own ledger and keeps a verified copy of the others'; every site applies all events in the same order, so they show the same picture. A site cut off from the others keeps dispatching on its own and catches up when the link returns. Each site numbers its own incidents under its own prefix."
      actions={<button className="btn btn-go" onClick={save}><Save size={14} aria-hidden /> Save</button>}
    >
      <div className="grid gap-4 lg:grid-cols-[22rem_1fr]">
        <section className="panel space-y-2 rounded-lg p-3">
          <h2 className="flex items-center gap-2 text-sm font-semibold text-slate-100"><Network size={15} aria-hidden /> This site</h2>
          <Field label="Name" htmlFor="sn" hint={v.fixed ? 'In use in the ledger: it cannot change' : 'lower-case: north, backup, county-b'}>
            <input id="sn" className="input font-mono" disabled={v.fixed} value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} />
          </Field>
          <Field label="Incident number prefix" htmlFor="sp" hint="One to three letters: N gives N26-000001">
            <input id="sp" className="input font-mono uppercase" disabled={v.fixed} value={d.prefix} onChange={(e) => setD({ ...d, prefix: e.target.value.toUpperCase() })} />
          </Field>
          {v.token && (
            <>
              <div className="text-[11px] text-slate-400">Give the other sites this site's token and certificate:</div>
              <div className="flex items-center gap-1"><code className="truncate rounded bg-slate-950 px-1 text-[11px] text-amber-200">{v.token}</code><button className="btn btn-ghost px-1" onClick={() => copy(v.token)} aria-label="Copy the token"><Copy size={12} /></button></div>
              <div className="flex items-center gap-1"><code className="truncate rounded bg-slate-950 px-1 text-[10px] text-slate-300">{v.fingerprint}</code><button className="btn btn-ghost px-1" onClick={() => copy(v.fingerprint)} aria-label="Copy the fingerprint"><Copy size={12} /></button></div>
            </>
          )}
          {!v.name && <p className="text-[11px] text-slate-500">Save a name first; this site's token is made then.</p>}
        </section>
        <section className="panel space-y-2 rounded-lg p-3">
          <h2 className="flex items-center text-sm font-semibold text-slate-100">Other sites
            <button className="btn ml-auto h-7 text-xs" disabled={!v.name} onClick={() => setD({ ...d, peers: [...d.peers, { name: '', url: 'https://', token: '', fingerprint: '' }] })}><Plus size={12} aria-hidden /> Add a site</button></h2>
          {d.peers.length === 0 && <p className="text-sm text-slate-500">One site on its own. Add the other sites to run active-active.</p>}
          {d.peers.map((p, i) => {
            const h = v.health.find((x) => x.name === p.name)
            return (
              <div key={i} className="grid gap-1 rounded border border-slate-800 p-2 md:grid-cols-[8rem_1fr_12rem]">
                <input className="input font-mono" placeholder="name" value={p.name} onChange={(e) => upPeer(i, { name: e.target.value.toLowerCase() })} aria-label="Site name" />
                <input className="input font-mono text-xs" placeholder="https://site-b:9443" value={p.url} onChange={(e) => upPeer(i, { url: e.target.value.trim() })} aria-label="Address" />
                <input className="input font-mono text-xs" placeholder="its token" value={p.token} onChange={(e) => upPeer(i, { token: e.target.value.trim() })} aria-label="Its token" />
                <div className="flex gap-1 md:col-span-3">
                  <input className="input font-mono text-[10px]" placeholder="its certificate SHA-256" value={p.fingerprint} onChange={(e) => upPeer(i, { fingerprint: e.target.value.trim() })} aria-label="Its certificate fingerprint" />
                  <button className="btn text-xs" onClick={() => read(i)}><Fingerprint size={12} aria-hidden /> Read it</button>
                  <button className="btn btn-ghost px-1.5" onClick={() => setD({ ...d, peers: d.peers.filter((_, j) => j !== i) })} aria-label={`Remove ${p.name}`}><Trash2 size={12} /></button>
                </div>
                {h && (
                  <div className={cx('text-xs md:col-span-3', h.ok ? 'text-emerald-300' : 'text-rose-300')}>
                    {h.ok ? `Linked · last copy ${h.last ? ago(h.last, now) + ' ago' : '—'} · ${h.copied.toLocaleString()} events copied` : `Not linked: ${h.error ?? 'waiting'}`}
                  </div>
                )}
              </div>
            )
          })}
        </section>
      </div>
    </Page>
  )
}
