import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { BookOpen, FlaskConical, Plus, Save, Sparkles, Trash2 } from 'lucide-react'
import { api, errText } from '../api'
import { cx } from '../util'
import type { GuidanceDoc, GuidanceResult, View } from '../types'
import { Page, Table } from './Admin'
import { Field, Modal } from '../ui'

const KINDS: [string, string][] = [
  ['policy', 'Policy (mandatory)'],
  ['guide', 'Guide'],
  ['prompt', 'Prompts'],
  ['checklist', 'Checklist'],
]
const kindName = (k: string) => KINDS.find(([c]) => c === k)?.[1] ?? k
const blank = (): GuidanceDoc => ({ id: '', title: '', kind: 'policy', version: '1', effective: '', approved: false, text: '' })
// A date input gives YYYY-MM-DD; the API wants RFC3339 at midnight, local.
const toRFC = (date: string) => (date ? new Date(`${date}T00:00:00`).toISOString() : '')
const toDate = (iso?: string) => (iso ? new Date(iso).toISOString().slice(0, 10) : '')

type Store = { items: GuidanceDoc[]; examples: GuidanceDoc[] }

/** Approved guidance: the agency's policies, call-taking guides, response guides and checklists. The
 * call-taker's Resources tab shows the matching passages with citations; only approved text is shown. */
export default function GuidanceAdmin({ view }: { view: View }) {
  const now = Date.now()
  const [store, setStore] = useState<Store>({ items: [], examples: [] })
  const [d, setD] = useState<(GuidanceDoc & { isNew: boolean }) | null>(null)
  const [type, setType] = useState('')
  const [tryMods, setTryMods] = useState<string[]>([])
  const [tryText, setTryText] = useState('')
  const [res, setRes] = useState<GuidanceResult | null>(null)
  const modifiers = view.entry?.modifiers ?? view.settings.modifiers ?? []
  const load = () => api.get<Store>('/admin/guidance').then(setStore).catch((e) => toast.error(errText(e)))
  useEffect(() => {
    load()
  }, [])
  const open = (g?: GuidanceDoc) => setD(g ? { ...g, isNew: false } : { ...blank(), isNew: true })
  const save = async () => {
    if (!d) return
    try {
      await api.put(`/admin/guidance/${encodeURIComponent(d.id.trim())}`, {
        id: d.id.trim(), title: d.title, kind: d.kind, version: d.version, effective: toRFC(toDate(d.effective)) || d.effective,
        expires: d.expires ? toRFC(toDate(d.expires)) : undefined, supersedes: d.supersedes, source: d.source,
        approved: d.approved, text: d.text, callTypes: d.callTypes, modifiers: d.modifiers, words: d.words, always: d.always,
      })
      toast.success(`${d.title || d.id} saved`)
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const remove = async (id: string) => {
    if (!confirm('Remove this document?')) return
    try {
      await api.del(`/admin/guidance/${encodeURIComponent(id)}`)
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const addExamples = async () => {
    try {
      for (const ex of store.examples) await api.put(`/admin/guidance/${encodeURIComponent(ex.id)}`, ex)
      toast.success('Examples added as drafts')
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const toggle = (key: 'callTypes' | 'modifiers', code: string) =>
    d && setD({ ...d, [key]: d[key]?.includes(code) ? d[key]!.filter((x) => x !== code) : [...(d[key] ?? []), code] })
  const runMatch = async () => {
    const p = new URLSearchParams()
    if (type) p.set('type', type)
    tryMods.forEach((m) => p.append('modifier', m))
    if (tryText) p.set('text', tryText)
    try {
      setRes(await api.get<GuidanceResult>(`/guidance/match?${p}`))
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const statusChip = (g: GuidanceDoc) => {
    const outdated = g.expires && new Date(g.expires).getTime() < now
    if (outdated) return <span className="chip bg-slate-600/30 text-slate-300">outdated</span>
    return g.approved ? <span className="chip bg-emerald-500/20 text-emerald-200">approved</span> : <span className="chip bg-amber-500/20 text-amber-200">draft</span>
  }
  return (
    <Page
      title="Approved guidance"
      intro="The agency's policies, call-taking guides, response guides and checklists. The call-taker's Resources tab shows the matching passages with citations; nothing is invented, only approved text is shown."
      actions={
        <>
          {store.examples.length > 0 && <button className="btn" onClick={addExamples}><Sparkles size={14} aria-hidden /> Add the examples</button>}
          <button className="btn btn-go" onClick={() => open()}><Plus size={14} aria-hidden /> Add a document</button>
        </>
      }
    >
      <div className="grid gap-4 lg:grid-cols-[1fr_22rem]">
        <Table head={['Title', 'Kind', 'Status', 'Version', 'Effective']} empty={store.items.length === 0 ? <p className="p-3 text-sm text-slate-500">No documents yet. Add the examples to see how it works.</p> : undefined}>
          {store.items.map((g) => (
            <tr key={g.id} className="cursor-pointer hover:bg-slate-800/50" onClick={() => open(g)}>
              <td className="px-3 py-1.5 text-slate-100">{g.title || g.id}</td>
              <td className="px-3 text-slate-400">{kindName(g.kind)}</td>
              <td className="px-3">{statusChip(g)}</td>
              <td className="px-3 font-mono text-xs text-slate-400">{g.version}</td>
              <td className="px-3 text-xs text-slate-400">{toDate(g.effective)}</td>
            </tr>
          ))}
        </Table>

        <section className="panel space-y-2 rounded-lg p-3">
          <h2 className="flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400"><FlaskConical size={12} aria-hidden /> Try it</h2>
          <select className="input text-sm" value={type} onChange={(e) => setType(e.target.value)} aria-label="Call type">
            <option value="">Any call type</option>
            {view.callTypes.map((c) => <option key={c.code} value={c.code}>{c.code} · {c.name}</option>)}
          </select>
          {modifiers.length > 0 && (
            <div className="flex flex-wrap gap-1">
              {modifiers.map((m) => (
                <button key={m.code} type="button" aria-pressed={tryMods.includes(m.code)} onClick={() => setTryMods((x) => x.includes(m.code) ? x.filter((y) => y !== m.code) : [...x, m.code])}
                  className={cx('chip cursor-pointer', tryMods.includes(m.code) ? 'bg-amber-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700')}>{m.name}</button>
              ))}
            </div>
          )}
          <textarea className="input min-h-16 text-sm" placeholder="Words from the call…" value={tryText} onChange={(e) => setTryText(e.target.value)} aria-label="Call text" />
          <button className="btn w-full" onClick={runMatch}>Find passages</button>
          {res && (
            <div className="space-y-2 text-xs">
              {res.matches.length === 0 ? (
                <p className="text-slate-500">{res.note || 'No matching passage.'}</p>
              ) : (
                res.matches.map((m, i) => (
                  <div key={i} className="rounded border border-slate-700 p-2">
                    <div className="flex items-center gap-1">
                      <span className="font-semibold text-slate-100">{m.title}</span>
                      <span className="chip bg-slate-700/40 text-slate-300">{kindName(m.kind)}</span>
                      <span className="ml-auto font-mono text-[10px] text-slate-500">v{m.version}</span>
                    </div>
                    <div className="text-slate-400">{m.heading}</div>
                    {m.why.length > 0 && <div className="text-[10px] text-slate-500">why: {m.why.join(', ')}</div>}
                    <p className="mt-1 whitespace-pre-wrap text-slate-300">{m.text}</p>
                  </div>
                ))
              )}
            </div>
          )}
        </section>
      </div>

      {d && (
        <Modal
          open
          xl
          onClose={() => setD(null)}
          title={d.isNew ? 'New document' : d.title || d.id}
          icon={<BookOpen size={16} className="text-emerald-300" aria-hidden />}
          footer={
            <>
              {!d.isNew && <button className="btn mr-auto text-rose-300" onClick={() => remove(d.id)}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => setD(null)}>Cancel</button>
              <button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save</button>
            </>
          }
        >
          <div className="grid gap-3 sm:grid-cols-4">
            <Field label="ID" htmlFor="gid"><input id="gid" className="input font-mono" disabled={!d.isNew} value={d.id} onChange={(e) => setD({ ...d, id: e.target.value })} /></Field>
            <div className="sm:col-span-2"><Field label="Title" htmlFor="gt"><input id="gt" className="input" value={d.title} onChange={(e) => setD({ ...d, title: e.target.value })} /></Field></div>
            <Field label="Kind" htmlFor="gk"><select id="gk" className="input" value={d.kind} onChange={(e) => setD({ ...d, kind: e.target.value })}>{KINDS.map(([c, n]) => <option key={c} value={c}>{n}</option>)}</select></Field>
            <Field label="Version" htmlFor="gv"><input id="gv" className="input font-mono" value={d.version} onChange={(e) => setD({ ...d, version: e.target.value })} /></Field>
            <Field label="Effective date" htmlFor="ge"><input id="ge" type="date" className="input" value={toDate(d.effective)} onChange={(e) => setD({ ...d, effective: e.target.value })} /></Field>
            <Field label="Expires" htmlFor="gx" hint="optional"><input id="gx" type="date" className="input" value={toDate(d.expires)} onChange={(e) => setD({ ...d, expires: e.target.value })} /></Field>
            <Field label="Supersedes" htmlFor="gs" hint="optional: the id it replaces"><input id="gs" className="input font-mono" value={d.supersedes ?? ''} onChange={(e) => setD({ ...d, supersedes: e.target.value })} /></Field>
            <div className="sm:col-span-4"><Field label="Source" htmlFor="gsrc" hint="where the full approved copy is"><input id="gsrc" className="input" value={d.source ?? ''} onChange={(e) => setD({ ...d, source: e.target.value })} /></Field></div>
            <div className="sm:col-span-4">
              <div className="label">Call types</div>
              <div className="flex max-h-20 flex-wrap gap-1 overflow-y-auto">
                {view.callTypes.map((c) => (
                  <button key={c.code} type="button" aria-pressed={d.callTypes?.includes(c.code)} onClick={() => toggle('callTypes', c.code)}
                    className={cx('chip cursor-pointer', d.callTypes?.includes(c.code) ? 'bg-indigo-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700')} title={c.name}>{c.code}</button>
                ))}
              </div>
            </div>
            {modifiers.length > 0 && (
              <div className="sm:col-span-4">
                <div className="label">Modifiers</div>
                <div className="flex flex-wrap gap-1">
                  {modifiers.map((m) => (
                    <button key={m.code} type="button" aria-pressed={d.modifiers?.includes(m.code)} onClick={() => toggle('modifiers', m.code)}
                      className={cx('chip cursor-pointer', d.modifiers?.includes(m.code) ? 'bg-amber-600 text-white' : 'bg-slate-800 text-slate-300 hover:bg-slate-700')} title={m.name}>{m.name}</button>
                  ))}
                </div>
              </div>
            )}
            <div className="sm:col-span-4"><Field label="Words" htmlFor="gw" hint="comma separated"><input id="gw" className="input" value={(d.words ?? []).join(', ')} onChange={(e) => setD({ ...d, words: e.target.value.split(',').map((w) => w.trim()).filter(Boolean) })} /></Field></div>
            <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!d.always} onChange={(e) => setD({ ...d, always: e.target.checked })} /> Always shown</label>
            <label className="flex items-center gap-2 text-sm text-slate-300 sm:col-span-3"><input type="checkbox" checked={d.approved} onChange={(e) => setD({ ...d, approved: e.target.checked })} /> Approved for use: shown to call-takers</label>
            <div className="sm:col-span-4">
              <Field label="Text" htmlFor="gtx" hint="Markdown with #/##/### headings. Headings divide it into passages. Under a heading, a line like “Applies: CARD, CHEST; modifiers: WEAPONS; words: not breathing” narrows that passage.">
                <textarea id="gtx" className="input min-h-48 font-mono text-xs" value={d.text} onChange={(e) => setD({ ...d, text: e.target.value })} />
              </Field>
            </div>
          </div>
        </Modal>
      )}
    </Page>
  )
}
