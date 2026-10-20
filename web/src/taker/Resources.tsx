import { useEffect, useMemo, useRef, useState } from 'react'
import { AlertTriangle, BookOpen, ExternalLink, Search } from 'lucide-react'
import { api } from '../api'
import { Modal } from '../ui'
import { cx } from '../util'
import type { Call, GuidanceResult, Incident, Passage, View } from '../types'
import type { Draft } from './draft'

export type Resources = { result: GuidanceResult | null; count: number; fresh: boolean }

const KIND: Record<Passage['kind'], { label: string; cls: string }> = {
  policy: { label: 'Policy (mandatory)', cls: 'bg-rose-500/20 text-rose-200' },
  checklist: { label: 'Checklist', cls: 'bg-violet-500/20 text-violet-200' },
  guide: { label: 'Guide', cls: 'bg-sky-500/20 text-sky-200' },
  prompt: { label: 'Suggested prompts', cls: 'bg-slate-700 text-slate-200' },
}

const keyOf = (p: Passage) => `${p.doc}@${p.version}#${p.anchor}`

/** useResources finds the approved guidance for the call as entered: the call type, modifiers and what was
 * said. A new passage lights the tab; opening it records what was shown, by version, on the incident. */
export function useResources(view: View | null, d: Draft, inc: Incident | undefined, call: Call | undefined, open: boolean): Resources {
  const [result, setResult] = useState<GuidanceResult | null>(null)
  const seen = useRef(new Set<string>())
  const [fresh, setFresh] = useState(false)
  const type = inc?.type ?? d.type
  const mods = inc?.modifiers ?? d.modifiers
  const words = [...d.notes.map((n) => n.text), ...(call?.transcript ?? []).map((t) => t.english || t.text)].join(' ').slice(-1500)
  const key = JSON.stringify([type, mods, words.length > 0 ? words.slice(-300) : '', inc?.number, view?.entry?.guidance])
  useEffect(() => {
    if (!view?.entry?.guidance || !type) return setResult(null)
    const t = setTimeout(() => {
      const q = new URLSearchParams({ type, text: words })
      mods.forEach((m) => q.append('modifier', m))
      api.get<GuidanceResult>(`/guidance/match?${q}`).then((r) => {
        setResult(r)
        if (r.matches.some((p) => !seen.current.has(keyOf(p)))) setFresh(true)
      }).catch(() => setResult(null))
    }, 500)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key])
  // Opening the tab: they are seen; on an incident, recorded as shown.
  useEffect(() => {
    if (!open || !result) return
    result.matches.forEach((p) => seen.current.add(keyOf(p)))
    setFresh(false)
    if (inc && result.matches.length) {
      api.post(`/incidents/${inc.number}/guidance/shown`, { shown: result.matches.filter((p) => p.status === 'current').map((p) => ({ doc: p.doc, version: p.version, anchor: p.anchor, heading: p.heading })) }).catch(() => {})
    }
  }, [open, result, inc])
  return { result, count: result?.matches.filter((p) => p.status === 'current').length ?? 0, fresh }
}

/** Resources: the passages of the agency's approved policies, guides and checklists that apply to this call,
 * each with its document, section, version and date, and the whole document a click away. */
export default function ResourcesTab({ res }: { res: Resources; inc?: Incident }) {
  const [q, setQ] = useState('')
  const [doc, setDoc] = useState<{ id: string; anchor: string } | null>(null)
  const list = useMemo(() => (res.result?.matches ?? []).filter((p) => !q.trim() || (p.title + ' ' + p.heading + ' ' + p.text).toLowerCase().includes(q.toLowerCase())), [res.result, q])
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <h2 className="flex items-center gap-1.5 text-sm font-semibold text-slate-100"><BookOpen size={14} aria-hidden /> Resources</h2>
        <span className="text-[11px] text-slate-500">approved text only, as written; follows the call type, modifiers and what is said</span>
        <div className="relative ml-auto w-56">
          <Search size={12} className="absolute left-2 top-2 text-slate-500" aria-hidden />
          <input className="input h-7 pl-6 text-xs" aria-label="Search the resources" placeholder="Search these passages" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
      </div>
      {!res.result && <p className="text-xs text-slate-500">Choose the call type: the approved guidance for it appears here.</p>}
      {res.result?.noMatch && <p className="rounded-md border border-slate-700 px-2 py-1.5 text-xs text-slate-300">{res.result.note || 'No approved guidance found for this call type.'}</p>}
      <ul className="space-y-2">
        {list.map((p) => (
          <li key={keyOf(p)} className={cx('panel rounded-lg p-2', p.status === 'outdated' && 'opacity-70')}>
            <div className="flex flex-wrap items-center gap-1.5 text-xs">
              <span className={cx('chip', KIND[p.kind]?.cls)}>{KIND[p.kind]?.label ?? p.kind}</span>
              <b className="text-slate-100">{p.title}</b> <span className="text-slate-400">› {p.path.join(' › ')}</span>
              <span className="ml-auto font-mono text-[10px] text-slate-500">{p.doc} v{p.version} · effective {new Date(p.effective).toLocaleDateString()}</span>
            </div>
            {p.status === 'outdated' && <p className="mt-1 flex items-center gap-1 text-[11px] font-semibold text-amber-300"><AlertTriangle size={11} aria-hidden /> Outdated: expired or replaced. Check the current version.</p>}
            {p.conflict && <p className="mt-1 flex items-center gap-1 text-[11px] font-semibold text-amber-300"><AlertTriangle size={11} aria-hidden /> Another policy ({p.conflict}) also applies: check which governs.</p>}
            <div className="mt-1 whitespace-pre-wrap text-sm leading-relaxed text-slate-200">{p.text}</div>
            <div className="mt-1 flex items-center gap-2 text-[10px] text-slate-500">
              <span>why: {p.why.join(', ')}</span>
              {p.source && <span>source: {p.source}</span>}
              <button className="btn btn-ghost ml-auto h-5 px-1 text-[10px]" onClick={() => setDoc({ id: p.doc, anchor: p.anchor })}><ExternalLink size={10} aria-hidden /> The whole document</button>
            </div>
          </li>
        ))}
      </ul>
      {doc && <DocModal id={doc.id} anchor={doc.anchor} onClose={() => setDoc(null)} />}
    </div>
  )
}

type Full = { doc: { id: string; title: string; version: string; effective: string; source?: string; kind: string }; sections: { heading: string; anchor: string; path: string[]; text: string }[] }

function DocModal({ id, anchor, onClose }: { id: string; anchor: string; onClose: () => void }) {
  const [full, setFull] = useState<Full | null>(null)
  useEffect(() => {
    api.get<Full>(`/guidance/${encodeURIComponent(id)}`).then(setFull).catch(() => setFull(null))
  }, [id])
  useEffect(() => {
    if (full) setTimeout(() => document.getElementById('g-' + anchor)?.scrollIntoView({ block: 'start' }), 50)
  }, [full, anchor])
  return (
    <Modal open onClose={onClose} xl title={full ? `${full.doc.title} — ${full.doc.id} v${full.doc.version}` : 'Loading…'} icon={<BookOpen size={16} className="text-sky-300" aria-hidden />}>
      {full && (
        <div className="space-y-3 text-sm">
          <p className="text-xs text-slate-500">Effective {new Date(full.doc.effective).toLocaleDateString()}{full.doc.source ? ` · approved copy: ${full.doc.source}` : ''}</p>
          {full.sections.map((s) => (
            <section key={s.anchor} id={'g-' + s.anchor} className={cx('rounded p-2', s.anchor === anchor && 'bg-sky-950/40 ring-1 ring-sky-700')}>
              <h3 className="font-semibold text-slate-100">{s.path.join(' › ')}</h3>
              <p className="mt-1 whitespace-pre-wrap text-slate-300">{s.text}</p>
            </section>
          ))}
        </div>
      )}
    </Modal>
  )
}
