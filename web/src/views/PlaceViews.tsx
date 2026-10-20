import { useEffect, useMemo, useRef, useState } from 'react'
import { Camera, Compass, ExternalLink, Images, Maximize2, Trash2 } from 'lucide-react'
import { api, ApiError } from '../api'
import { can, useStore } from '../store'
import { Modal } from '../ui'
import { cx } from '../util'
import type { PlaceView, PlaceViews as Found, Premise, PremisePhoto } from '../types'

/** A picture the viewer can show: a provider's view or one of the agency's pre-plan photos. */
type Shot = { key: string; src: string; group: 'oblique' | 'street' | 'preplan' | 'ortho'; facing?: string; title: string; detail: string; credit: string }

const SIDE: Record<string, string> = { N: 'south', E: 'west', S: 'north', W: 'east' }
const WORD: Record<string, string> = { N: 'north', E: 'east', S: 'south', W: 'west' }

const month = (iso?: string) => (iso ? new Date(iso).toLocaleDateString(undefined, { year: 'numeric', month: 'short' }) : 'date not given')

function shotOf(v: PlaceView): Shot {
  const src = `./api/views/picture/${v.ref}`
  if (v.kind === 'oblique') {
    return { key: v.ref, src, group: 'oblique', facing: v.facing, title: `Looking ${WORD[v.facing ?? ''] ?? '?'}`,
      detail: `shows the ${SIDE[v.facing ?? ''] ?? ''} side · ${v.provider} · ${month(v.captured)}`, credit: v.attribution }
  }
  if (v.kind === 'ortho') return { key: v.ref, src, group: 'ortho', title: 'From above', detail: `${v.provider} · ${month(v.captured)}`, credit: v.attribution }
  return { key: v.ref, src, group: 'street', facing: v.facing, title: `Street${v.facing ? `, looking ${WORD[v.facing]}` : ''}`,
    detail: `${v.distance} m away · ${v.provider} · ${month(v.captured)}`, credit: `${v.attribution}${v.license ? ` · ${v.license}` : ''}` }
}

function preplanOf(p: Premise, ph: PremisePhoto): Shot {
  return { key: `${p.id}/${ph.id}`, src: `./api/premises/${encodeURIComponent(p.id)}/photos/${ph.id}`, group: 'preplan', facing: ph.facing,
    title: ph.caption || 'Pre-plan photo', detail: `${p.name || p.address}${ph.facing ? ` · from the ${WORD[ph.facing]}` : ''} · added ${new Date(ph.added).toLocaleDateString()} by ${ph.by}`,
    credit: 'Agency pre-plan' }
}

/** usePlaceViews looks the place up once per place (rounded to about a metre). */
export function usePlaceViews(lat?: number, lon?: number, on = true) {
  const [found, setFound] = useState<Found | null>(null)
  const [err, setErr] = useState('')
  const key = lat != null && lon != null ? `${lat.toFixed(5)},${lon.toFixed(5)}` : ''
  useEffect(() => {
    setFound(null)
    setErr('')
    if (!on || !key) return
    let live = true
    api.get<Found>(`/views?lat=${lat}&lon=${lon}`).then((f) => live && setFound(f)).catch((e) => live && setErr(e instanceof Error ? e.message : String(e)))
    return () => { live = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, on])
  return { found, err, loading: on && !!key && !found && !err }
}

/** PlaceViews shows a place from every side: the four oblique aerial photos as a compass (arrow keys turn
 * it), street photos that look at the place, and the agency's own pre-plan photos. Each says where it came
 * from and when it was taken, so an old picture is never taken for today. */
export default function PlaceViews({ lat, lon, premises = [], big, onChanged }: { lat?: number; lon?: number; premises?: Premise[]; big?: boolean; onChanged?: () => void }) {
  const { found, err, loading } = usePlaceViews(lat, lon)
  const me = useStore((s) => s.me)
  const shots = useMemo(() => {
    const out: Shot[] = (found?.views ?? []).map(shotOf)
    for (const p of premises) for (const ph of p.photos ?? []) out.push(preplanOf(p, ph))
    const rank = { oblique: 0, preplan: 1, street: 2, ortho: 3 }
    return out.sort((a, b) => rank[a.group] - rank[b.group])
  }, [found, premises])
  const [sel, setSel] = useState('')
  const [full, setFull] = useState(false)
  const cur = shots.find((s) => s.key === sel) ?? shots[0]
  const obliques = shots.filter((s) => s.group === 'oblique')
  const box = useRef<HTMLDivElement>(null)

  // Arrow keys turn the compass when the viewer has focus.
  const turn = (d: number) => {
    if (!obliques.length) return
    const i = Math.max(0, obliques.findIndex((s) => s.key === cur?.key))
    setSel(obliques[(i + d + obliques.length) % obliques.length].key)
  }

  if (lat == null || lon == null) return <p className="text-xs text-slate-500">Views appear once the place has a position.</p>
  const problems = Object.entries(found?.problems ?? {})
  return (
    <div ref={box} className="space-y-2 outline-none" tabIndex={0} aria-label="Views of the place"
      onKeyDown={(e) => { if (e.key === 'ArrowRight') { e.preventDefault(); turn(1) } else if (e.key === 'ArrowLeft') { e.preventDefault(); turn(-1) } }}>
      <div className="flex flex-wrap items-center gap-1.5">
        {(['N', 'E', 'S', 'W'] as const).map((f) => {
          const s = obliques.find((o) => o.facing === f)
          return (
            <button key={f} type="button" disabled={!s} onClick={() => s && setSel(s.key)} aria-pressed={!!s && cur?.key === s.key}
              title={s ? `${s.title}: ${s.detail}` : `No oblique photo looking ${WORD[f]}`}
              className={cx('btn h-7 w-9 justify-center px-0 font-mono font-bold', s && cur?.key === s.key ? 'bg-sky-600 text-white' : '', !s && 'opacity-40')}>{f}</button>
          )
        })}
        {obliques.length > 0 && <span className="text-[11px] text-slate-500"><Compass size={11} className="inline" aria-hidden /> ← → to turn</span>}
        {found?.google && <a className="btn btn-ghost ml-auto h-7 text-xs" href={found.google} target="_blank" rel="noreferrer noopener"><ExternalLink size={12} aria-hidden /> Google Street View</a>}
      </div>

      {loading && <div className={cx('animate-pulse rounded-md bg-slate-800', big ? 'h-[52vh]' : 'h-48')} aria-label="Looking the place up" />}
      {err && <p role="alert" className="text-xs text-rose-300">{err}</p>}
      {cur && (
        <figure className="relative overflow-hidden rounded-md border border-slate-800 bg-black">
          <img key={cur.src} src={cur.src} alt={`${cur.title}, ${cur.detail}`} className={cx('w-full object-contain', big ? 'h-[52vh]' : 'h-56')} />
          <figcaption className="absolute inset-x-0 bottom-0 flex items-end gap-2 bg-gradient-to-t from-black/90 to-transparent px-2 pb-1.5 pt-6 text-xs">
            <div className="min-w-0">
              <div className="font-semibold text-white">{cur.title}</div>
              <div className="truncate text-slate-300">{cur.detail}</div>
            </div>
            <span className="ml-auto shrink-0 text-[10px] text-slate-400">{cur.credit}</span>
            <button type="button" className="btn btn-ghost h-6 shrink-0 px-1" onClick={() => setFull(true)} aria-label="Full size"><Maximize2 size={13} aria-hidden /></button>
          </figcaption>
        </figure>
      )}
      {shots.length > 1 && (
        <ul className="flex gap-1.5 overflow-x-auto pb-1" aria-label="Pictures">
          {shots.map((s) => (
            <li key={s.key} className="shrink-0">
              <button type="button" onClick={() => setSel(s.key)} aria-pressed={cur?.key === s.key} title={`${s.title}: ${s.detail}`}
                className={cx('relative block h-14 w-20 overflow-hidden rounded border', cur?.key === s.key ? 'border-sky-400' : 'border-slate-700')}>
                <img src={s.src} alt="" loading="lazy" className="h-full w-full object-cover" />
                <span className="absolute left-0.5 top-0.5 rounded bg-black/70 px-1 text-[9px] font-semibold uppercase text-slate-100">{s.group === 'preplan' ? 'pre-plan' : s.group === 'oblique' ? s.facing : s.group}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {found && !shots.length && (
        <p className="text-xs text-slate-500">
          {found.views.length === 0 && !problems.length && !premisesHaveAny(premises) && (can.admin(me)
            ? <>No pictures of this place. Pictures from the side come from the providers in <a className="underline" href="#/admin/integrations">Admin › Integrations › Views</a>, and pre-plan photos from each premise.</>
            : 'No pictures of this place.')}
        </p>
      )}
      {problems.map(([p, why]) => <p key={p} className="text-xs text-amber-300">{p}: {why}</p>)}
      <PreplanAdd premises={premises} onChanged={onChanged} />
      {cur?.group === 'preplan' && can.supervise(me) && <PreplanRemove shot={cur} onChanged={onChanged} />}

      <Modal open={full && !!cur} onClose={() => setFull(false)} title={cur ? `${cur.title} · ${cur.detail}` : ''} xl>
        {cur && <img src={cur.src} alt={`${cur.title}, ${cur.detail}`} className="mx-auto max-h-[66vh] w-auto" />}
        {cur && <p className="mt-2 text-right text-[11px] text-slate-500">{cur.credit}</p>}
      </Modal>
    </div>
  )
}

const premisesHaveAny = (ps: Premise[]) => ps.some((p) => (p.photos?.length ?? 0) > 0)

/** PreplanAdd takes a photo with the device's camera (or a file) for a premise at the place. */
function PreplanAdd({ premises, onChanged }: { premises: Premise[]; onChanged?: () => void }) {
  const me = useStore((s) => s.me)
  const [pid, setPid] = useState(premises[0]?.id ?? '')
  const [caption, setCaption] = useState('')
  const [facing, setFacing] = useState('')
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState('')
  const file = useRef<HTMLInputElement>(null)
  useEffect(() => { if (!premises.some((p) => p.id === pid)) setPid(premises[0]?.id ?? '') }, [premises, pid])
  if (!premises.length || !me || (me.role !== 'field' && !can.dispatch(me))) return null
  const send = async (f: File) => {
    setBusy(true)
    setMsg('')
    try {
      const q = new URLSearchParams({ caption, facing })
      const r = await fetch(`./api/premises/${encodeURIComponent(pid)}/photos?${q}`, { method: 'POST', body: f, headers: { 'X-BitDispatcher': '1' }, credentials: 'same-origin' })
      if (!r.ok) throw new ApiError(r.status, ((await r.json().catch(() => ({}))) as { message?: string }).message ?? r.statusText)
      setCaption('')
      setMsg('Added.')
      onChanged?.()
    } catch (e) {
      setMsg(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
      if (file.current) file.current.value = ''
    }
  }
  return (
    <details className="rounded border border-slate-800 px-2 py-1 text-xs">
      <summary className="cursor-pointer text-slate-300"><Camera size={12} className="inline" aria-hidden /> Add a pre-plan photo</summary>
      <div className="mt-1.5 flex flex-wrap items-end gap-1.5">
        {premises.length > 1 && (
          <label className="flex flex-col"><span className="label">Premise</span>
            <select className="input h-7 text-xs" value={pid} onChange={(e) => setPid(e.target.value)}>{premises.map((p) => <option key={p.id} value={p.id}>{p.name || p.address}</option>)}</select>
          </label>
        )}
        <label className="flex min-w-40 flex-1 flex-col"><span className="label">Caption</span>
          <input className="input h-7 text-xs" value={caption} maxLength={200} onChange={(e) => setCaption(e.target.value)} placeholder="Back door, Knox box, gas shut-off…" />
        </label>
        <label className="flex flex-col"><span className="label">Taken from</span>
          <select className="input h-7 text-xs" value={facing} onChange={(e) => setFacing(e.target.value)}>
            <option value="">—</option><option value="N">the north</option><option value="E">the east</option><option value="S">the south</option><option value="W">the west</option>
          </select>
        </label>
        <input ref={file} type="file" accept="image/jpeg,image/png,image/webp" capture="environment" className="hidden" onChange={(e) => e.target.files?.[0] && send(e.target.files[0])} />
        <button type="button" className="btn h-7 text-xs" disabled={busy || !pid} onClick={() => file.current?.click()}><Images size={12} aria-hidden /> {busy ? 'Sending…' : 'Take or choose a photo'}</button>
      </div>
      {msg && <p role="status" className="mt-1 text-slate-400">{msg}</p>}
    </details>
  )
}

function PreplanRemove({ shot, onChanged }: { shot: Shot; onChanged?: () => void }) {
  const [busy, setBusy] = useState(false)
  const [pid, ph] = shot.key.split('/')
  return (
    <button type="button" className="btn btn-ghost h-7 text-xs text-rose-300" disabled={busy}
      onClick={async () => {
        if (!confirm(`Remove the pre-plan photo "${shot.title}"?`)) return
        setBusy(true)
        try { await api.del(`/premises/${encodeURIComponent(pid)}/photos/${ph}`); onChanged?.() } finally { setBusy(false) }
      }}>
      <Trash2 size={12} aria-hidden /> Remove this pre-plan photo
    </button>
  )
}

/** ViewsButton opens the views of a place in a large window. */
export function ViewsButton({ lat, lon, premises, label = 'Views', className }: { lat?: number; lon?: number; premises?: Premise[]; label?: string; className?: string }) {
  const [open, setOpen] = useState(false)
  if (lat == null || lon == null) return null
  return (
    <>
      <button type="button" className={cx('btn h-6 px-1.5 text-[11px]', className)} onClick={() => setOpen(true)} title="Pictures of the place from every side">
        <Images size={12} aria-hidden /> {label}
      </button>
      <Modal open={open} onClose={() => setOpen(false)} title="Views of the place" icon={<Images size={16} aria-hidden />} xl>
        {open && <PlaceViews lat={lat} lon={lon} premises={premises} big />}
      </Modal>
    </>
  )
}
