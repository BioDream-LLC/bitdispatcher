import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { ArrowRightLeft, Building, FileDown, Printer, RefreshCw, Wifi, WifiOff } from 'lucide-react'
import { api, errText } from '../api'
import { can, useStore } from '../store'
import { ago, cx, useNow } from '../util'
import type { ContinuityInfo } from './types'

const fmt = (t?: string) => (t ? new Date(t).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : '')

/** Continuity is what keeps the centre working through an outage: the links and what each one down means,
 * the move to the backup site with its checklist, the continuity pack, and the way to overflow. */
export default function Continuity() {
  const now = useNow()
  const me = useStore((s) => s.me)
  const view = useStore((s) => s.view)
  const sup = can.supervise(me) || me?.duty === 'supervisor'
  const [info, setInfo] = useState<ContinuityInfo | null>(null)
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const load = useCallback(() => {
    api.get<ContinuityInfo>('/continuity').then(setInfo).catch((e) => toast.error(errText(e)))
  }, [])
  useEffect(() => {
    load()
    const t = setInterval(load, 15000)
    return () => clearInterval(t)
  }, [load, view?.seq])

  const act = async (f: () => Promise<unknown>, ok: string) => {
    setBusy(true)
    try {
      await f()
      toast.success(ok)
      load()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }

  if (!info) return <div className="grid h-full place-items-center text-sm text-slate-500">Loading continuity…</div>
  const h = info.health
  const r = info.relocation?.active ? info.relocation : null
  const steps = info.steps ?? []
  const inetDown = h.internet === 'down'
  return (
    <div className="flex h-full flex-col overflow-hidden">
      <header className="panel-head flex items-center gap-3 px-4 py-2">
        <a href="#/" className="btn btn-ghost h-6 px-1.5 text-xs">← Dispatch</a>
        <h1 className="text-sm font-semibold text-slate-100">Continuity</h1>
        <span className="text-xs text-slate-500">Links, the backup site and overflow. Every action here is recorded as you.</span>
        <Link to="/overflow" className="btn ml-auto h-7 text-xs"><ArrowRightLeft size={13} aria-hidden /> Overflow to a neighbour</Link>
      </header>
      <div className="grid flex-1 gap-4 overflow-auto p-4 lg:grid-cols-2">
        <section className="panel space-y-2 rounded-lg p-4" aria-labelledby="links-h">
          <h2 id="links-h" className="flex items-center gap-2 font-semibold text-slate-100">{inetDown ? <WifiOff size={15} className="text-rose-300" aria-hidden /> : <Wifi size={15} className="text-emerald-300" aria-hidden />} Links</h2>
          {inetDown ? (
            <div className="rounded border border-rose-800 bg-rose-950/40 p-2 text-sm text-rose-100">
              <p className="font-semibold">The internet is down.</p>
              <p className="mt-1 text-xs">These keep working on this server: call entry, dispatch, unit status, the recorded street map, routing and addresses, the ledger, printing.</p>
              <p className="mt-1 text-xs">These wait for the internet: online map tiles, Street View and aerial photos not recorded, traffic, map updates, neighbouring centres and overflow, NERIS, PSC PIP, and BitVoice or paging if they are reached over the internet.</p>
            </div>
          ) : h.internet === 'unknown' ? (
            <p className="text-xs text-slate-400">No internet check is set (Admin › Continuity), so bitdispatcher cannot say whether the internet is up. The links below are checked anyway.</p>
          ) : (
            <p className="text-xs text-emerald-300">The internet is reachable.</p>
          )}
          <ul className="divide-y divide-slate-800 text-sm">
            {info.links.length === 0 && <li className="py-1 text-xs text-slate-500">No link has been checked yet.</li>}
            {info.links.map((l) => (
              <li key={l.name} className="flex items-baseline gap-2 py-1">
                <span className={cx('h-2 w-2 shrink-0 rounded-full', l.up ? 'bg-emerald-400' : 'bg-rose-400 live-dot')} aria-hidden />
                <span className="font-medium text-slate-200">{l.name}</span>
                <span className={cx('text-xs', l.up ? 'text-slate-500' : 'text-rose-300')}>{l.up ? 'up' : 'down'} for {ago(l.since, now)}</span>
                {l.error && <span className="truncate text-xs text-slate-500" title={l.error}>{l.error}</span>}
              </li>
            ))}
          </ul>
          {info.outages.length > 0 && (
            <details className="text-xs text-slate-400">
              <summary className="cursor-pointer">Outages ({info.outages.length})</summary>
              <ul className="mt-1 space-y-0.5">
                {[...info.outages].reverse().map((o, i) => (
                  <li key={i}>{o.name}: {fmt(o.from)} to {o.to ? fmt(o.to) : 'now'}{o.error ? ` · ${o.error}` : ''}</li>
                ))}
              </ul>
            </details>
          )}
          {info.sites && info.sites.length > 0 && (
            <p className="text-xs text-slate-400">This is site {info.site}. Other sites: {info.sites.map((x) => `${x.name} ${x.ok ? 'copying' : 'not copying'}`).join(', ')}. Each site holds every site's records, so the backup site can take over at once.</p>
          )}
        </section>

        <section className="panel space-y-2 rounded-lg p-4" aria-labelledby="reloc-h">
          <h2 id="reloc-h" className="flex items-center gap-2 font-semibold text-slate-100"><Building size={15} aria-hidden /> Backup site</h2>
          {r ? (
            <div className="rounded border border-amber-700 bg-amber-950/30 p-2 text-sm text-amber-100">
              Working from the backup site{r.site ? ` (site ${r.site})` : ''} since {fmt(r.since)}, declared by {r.by}.{r.note ? ` ${r.note}` : ''}
            </div>
          ) : (
            <p className="text-xs text-slate-400">When the centre has to leave its building, a supervisor declares it here at the backup site. Every console then shows it, and the checklist below is worked and recorded.</p>
          )}
          {steps.length > 0 && (
            <ol className="space-y-1 text-sm">
              {steps.map((s, i) => (
                <li key={i}>
                  <label className="flex items-start gap-2">
                    <input type="checkbox" disabled={!r || !sup || busy} checked={!!r?.done.includes(i)}
                      onChange={(e) => act(() => api.post(`/continuity/relocation/steps/${i}`, { done: e.target.checked }), e.target.checked ? 'Step done' : 'Step undone')} />
                    <span className={cx(r?.done.includes(i) && 'text-slate-500 line-through')}>{i + 1}. {s}</span>
                  </label>
                </li>
              ))}
            </ol>
          )}
          {steps.length === 0 && <p className="text-xs text-slate-500">No checklist is set (Admin › Continuity).</p>}
          {sup && (
            <div className="flex flex-wrap items-center gap-2">
              {!r && <input className="input h-8 flex-1 text-sm" placeholder="Why, for the record (optional)" value={note} onChange={(e) => setNote(e.target.value)} aria-label="Why the centre is moving" />}
              <button className={cx('btn h-8 text-xs', r ? '' : 'btn-danger')} disabled={busy}
                onClick={() => {
                  if (!r && !confirm('Declare that the centre is working from the backup site? Every console will show it.')) return
                  act(() => api.post('/continuity/relocation', { active: !r, note }), r ? 'Back in the building' : 'Declared: working from the backup site').then(() => setNote(''))
                }}>
                {r ? 'We are back in our building' : 'We are working from the backup site'}
              </button>
            </div>
          )}
        </section>

        <section className="panel space-y-2 rounded-lg p-4" aria-labelledby="pack-h">
          <h2 id="pack-h" className="flex items-center gap-2 font-semibold text-slate-100"><FileDown size={15} aria-hidden /> Continuity pack</h2>
          <p className="text-xs text-slate-400">A printable page of the open incidents, the units, the checklist, the contacts and blank log lines, for working on paper if the CAD cannot be reached. It holds callers' names and numbers: keep printed copies as you keep records.</p>
          <p className="text-xs text-slate-400">
            {info.packMinutes ? `Written every ${info.packMinutes} minutes to the folders set in Admin › Continuity.` : 'Not written on a schedule (Admin › Continuity).'}
            {info.packAt && ` Last written ${ago(info.packAt, now)} ago.`}
          </p>
          {(info.packErrors ?? []).map((e) => <p key={e} className="text-xs text-rose-300">Could not write: {e}</p>)}
          <div className="flex gap-2">
            <a className="btn h-8 text-xs" href="/api/continuity/pack" target="_blank" rel="noopener"><Printer size={13} aria-hidden /> Open to print</a>
            {sup && <button className="btn h-8 text-xs" disabled={busy} onClick={() => act(() => api.post('/continuity/pack/write'), 'Continuity pack written')}><RefreshCw size={13} aria-hidden /> Write it now</button>}
          </div>
        </section>

        <section className="panel space-y-2 rounded-lg p-4" aria-labelledby="ovf-h">
          <h2 id="ovf-h" className="flex items-center gap-2 font-semibold text-slate-100"><ArrowRightLeft size={15} aria-hidden /> Overflow</h2>
          <p className="text-xs text-slate-400">When the centre is overwhelmed or out of its building, a neighbouring centre that runs bitdispatcher can take its calls: each call becomes an incident here, and the neighbour sees only the calls it took. The 911 calls themselves reach the neighbour by the phone network's alternate routing, which the 911 authority sets up.</p>
          {(h.overflow ?? []).length === 0 ? <p className="text-xs text-slate-500">No overflow now.</p> : (
            <ul className="text-sm">
              {(h.overflow ?? []).map((o) => <li key={o.direction + o.partner}>{o.direction === 'in' ? `We take calls for ${o.name}` : `${o.name} takes our calls`} ({o.status}) until {fmt(o.until)}</li>)}
            </ul>
          )}
          <Link to="/overflow" className="btn h-8 text-xs">Open overflow</Link>
        </section>

        {h.mapAuto && (
          <section className="panel space-y-1 rounded-lg p-4 text-xs text-slate-400" aria-labelledby="map-h">
            <h2 id="map-h" className="text-sm font-semibold text-slate-100">Map updates</h2>
            <p>Automatic updates: {h.mapAuto.mode}.{h.mapAuto.next && ` Next ${fmt(h.mapAuto.next)}.`}{h.mapAuto.last && ` Last ${fmt(h.mapAuto.last)}: ${h.mapAuto.result}.`}</p>
            {h.mapAuto.error && h.mapAuto.result === 'failed' && <p className="text-rose-300">{h.mapAuto.error}</p>}
          </section>
        )}
        {(info.contacts ?? []).length > 0 && (
          <section className="panel space-y-1 rounded-lg p-4" aria-labelledby="contacts-h">
            <h2 id="contacts-h" className="text-sm font-semibold text-slate-100">Contacts</h2>
            <ul className="text-sm text-slate-300">{(info.contacts ?? []).map((c) => <li key={c}>{c}</li>)}</ul>
          </section>
        )}
      </div>
    </div>
  )
}
