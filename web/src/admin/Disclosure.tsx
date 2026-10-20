import { Fragment, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { FileLock2, FlaskConical, Plus, Save, Trash2, TriangleAlert } from 'lucide-react'
import { api, errText } from '../api'
import { cx } from '../util'
import type { FOIAProposal, FOIARule, View } from '../types'
import { Page, Table } from './Admin'
import { Field, Modal } from '../ui'

const KINDS: [FOIARule['kind'], string][] = [
  ['category', 'Category'],
  ['words', 'Words'],
  ['pattern', 'Pattern'],
]
const toDate = (iso?: string) => (iso ? new Date(iso).toISOString().slice(0, 10) : '')
const toRFC = (date: string) => (date ? new Date(`${date}T00:00:00`).toISOString() : '')
const blank = (): FOIARule => ({ id: '', name: '', kind: 'category', basis: '', explain: '', enabled: false })

type Store = { items: FOIARule[]; categories: string[]; samples: FOIARule[] }
type TryResult = { proposals: FOIAProposal[]; errors: string[] }

/** Disclosure rules for records requests (FOIA): the agency's legally reviewed rules. Nothing is
 * hardcoded as exempt; the system proposes, people decide. */
export default function Disclosure({ view: _view }: { view: View }) {
  const [store, setStore] = useState<Store>({ items: [], categories: [], samples: [] })
  const [d, setD] = useState<(FOIARule & { isNew: boolean }) | null>(null)
  const [text, setText] = useState('')
  const [res, setRes] = useState<TryResult | null>(null)
  const load = () => api.get<Store>('/admin/foia/rules').then(setStore).catch((e) => toast.error(errText(e)))
  useEffect(() => {
    load()
  }, [])
  const open = (r?: FOIARule) => setD(r ? { ...r, isNew: false } : { ...blank(), isNew: true })
  const save = async () => {
    if (!d) return
    if (!d.basis.trim()) {
      toast.error('A legal basis is required')
      return
    }
    try {
      await api.put(`/admin/foia/rules/${encodeURIComponent(d.id.trim())}`, {
        id: d.id.trim(), name: d.name, jurisdiction: d.jurisdiction, recordTypes: d.recordTypes, kind: d.kind,
        pattern: d.pattern, words: d.words, category: d.category, basis: d.basis, explain: d.explain,
        from: d.from ? toRFC(toDate(d.from)) : undefined, to: d.to ? toRFC(toDate(d.to)) : undefined, enabled: d.enabled,
      })
      toast.success(`${d.name || d.id} saved`)
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const remove = async (id: string) => {
    if (!confirm('Remove this rule?')) return
    try {
      await api.del(`/admin/foia/rules/${encodeURIComponent(id)}`)
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const addSamples = async () => {
    try {
      for (const s of store.samples) await api.put(`/admin/foia/rules/${encodeURIComponent(s.id)}`, { ...s, enabled: false })
      toast.success('Sample rules added, disabled')
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const runTry = async () => {
    try {
      setRes(await api.post<TryResult>('/admin/foia/try', { text }))
    } catch (e) {
      toast.error(errText(e))
    }
  }
  // Highlight the text with the proposed passages marked; amber for 'review', rose for 'high'.
  const highlighted = () => {
    if (!res || res.proposals.length === 0) return <span>{text}</span>
    const spans = [...res.proposals].sort((a, b) => a.start - b.start)
    const out: React.ReactNode[] = []
    let at = 0
    spans.forEach((p, i) => {
      if (p.start < at) return
      if (p.start > at) out.push(<Fragment key={`t${i}`}>{text.slice(at, p.start)}</Fragment>)
      out.push(
        <mark key={`m${i}`} className={cx('rounded px-0.5', p.confidence === 'high' ? 'bg-rose-500/40 text-rose-100' : 'bg-amber-500/40 text-amber-100')} title={`${p.rule}: ${p.basis}`}>
          {text.slice(p.start, p.end)}
        </mark>,
      )
      at = p.end
    })
    if (at < text.length) out.push(<Fragment key="tend">{text.slice(at)}</Fragment>)
    return out
  }
  return (
    <Page
      title="Disclosure rules"
      intro="For records requests (FOIA): the agency's legally reviewed rules. Nothing is hardcoded as exempt; the system proposes, people decide. Your counsel reviews every rule before it is enabled."
      actions={
        <>
          {store.samples.length > 0 && <button className="btn" onClick={addSamples}><Plus size={14} aria-hidden /> Add the sample rules (disabled)</button>}
          <button className="btn btn-go" onClick={() => open()}><Plus size={14} aria-hidden /> Add a rule</button>
        </>
      }
    >
      <div className="mb-4 flex items-start gap-2 rounded-lg border border-amber-800 bg-amber-950/30 p-3 text-sm text-amber-200">
        <TriangleAlert size={16} className="mt-0.5 shrink-0" aria-hidden />
        <span>Samples are examples, not legal advice: your counsel reviews every rule before it is enabled.</span>
      </div>

      <div className="grid gap-4 lg:grid-cols-[1fr_24rem]">
        <Table head={['Name', 'Kind', 'Basis', 'State']} empty={store.items.length === 0 ? <p className="p-3 text-sm text-slate-500">No rules yet.</p> : undefined}>
          {store.items.map((r) => (
            <tr key={r.id} className="cursor-pointer hover:bg-slate-800/50" onClick={() => open(r)}>
              <td className="px-3 py-1.5 text-slate-100">{r.name || r.id}</td>
              <td className="px-3 text-slate-400">{KINDS.find(([c]) => c === r.kind)?.[1] ?? r.kind}</td>
              <td className="max-w-xs truncate px-3 text-xs text-slate-400" title={r.basis}>{r.basis}</td>
              <td className="px-3 text-xs">{r.enabled ? <span className="text-emerald-300">enabled</span> : <span className="text-slate-500">disabled</span>}</td>
            </tr>
          ))}
        </Table>

        <section className="panel space-y-2 rounded-lg p-3">
          <h2 className="flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-slate-400"><FlaskConical size={12} aria-hidden /> Try it</h2>
          <textarea className="input min-h-24 text-sm" placeholder="Paste a record's text…" value={text} onChange={(e) => setText(e.target.value)} aria-label="Record text" />
          <button className="btn w-full" onClick={runTry}>Propose passages</button>
          {res && (
            <>
              <p className="whitespace-pre-wrap rounded border border-slate-700 bg-slate-950 p-2 text-xs text-slate-300">{highlighted()}</p>
              {res.errors.length > 0 && res.errors.map((er, i) => <p key={i} className="text-xs text-rose-300">{er}</p>)}
              <ul className="space-y-1 text-xs">
                {res.proposals.length === 0 && <li className="text-slate-500">Nothing proposed.</li>}
                {res.proposals.map((p) => (
                  <li key={p.id} className="rounded border border-slate-700 p-2">
                    <div className="flex items-center gap-1">
                      <span className={cx('chip', p.confidence === 'high' ? 'bg-rose-500/20 text-rose-200' : 'bg-amber-500/20 text-amber-200')}>{p.confidence}</span>
                      <span className="font-mono text-[10px] text-slate-500">{p.rule}</span>
                    </div>
                    <div className="mt-0.5 italic text-slate-300">“{p.text}”</div>
                    <div className="text-[10px] text-slate-500">{p.basis}</div>
                  </li>
                ))}
              </ul>
            </>
          )}
        </section>
      </div>

      {d && (
        <Modal
          open
          wide
          onClose={() => setD(null)}
          title={d.isNew ? 'New rule' : d.name || d.id}
          icon={<FileLock2 size={16} className="text-fuchsia-300" aria-hidden />}
          footer={
            <>
              {!d.isNew && <button className="btn mr-auto text-rose-300" onClick={() => remove(d.id)}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => setD(null)}>Cancel</button>
              <button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save</button>
            </>
          }
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="ID" htmlFor="rid"><input id="rid" className="input font-mono" disabled={!d.isNew} value={d.id} onChange={(e) => setD({ ...d, id: e.target.value })} /></Field>
            <Field label="Name" htmlFor="rn"><input id="rn" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} /></Field>
            <Field label="Jurisdiction" htmlFor="rj"><input id="rj" className="input" value={d.jurisdiction ?? ''} onChange={(e) => setD({ ...d, jurisdiction: e.target.value })} /></Field>
            <Field label="Record types" htmlFor="rrt" hint="comma separated"><input id="rrt" className="input" value={(d.recordTypes ?? []).join(', ')} onChange={(e) => setD({ ...d, recordTypes: e.target.value.split(',').map((x) => x.trim()).filter(Boolean) })} /></Field>
            <Field label="Kind" htmlFor="rk"><select id="rk" className="input" value={d.kind} onChange={(e) => setD({ ...d, kind: e.target.value as FOIARule['kind'] })}>{KINDS.map(([c, n]) => <option key={c} value={c}>{n}</option>)}</select></Field>
            {d.kind === 'category' && (
              <Field label="Category" htmlFor="rc">
                <select id="rc" className="input" value={d.category ?? ''} onChange={(e) => setD({ ...d, category: e.target.value })}>
                  <option value="">Choose a category…</option>
                  {store.categories.map((c) => <option key={c} value={c}>{c}</option>)}
                </select>
              </Field>
            )}
            {d.kind === 'words' && (
              <Field label="Words" htmlFor="rw" hint="comma separated"><input id="rw" className="input" value={(d.words ?? []).join(', ')} onChange={(e) => setD({ ...d, words: e.target.value.split(',').map((x) => x.trim()).filter(Boolean) })} /></Field>
            )}
            {d.kind === 'pattern' && (
              <Field label="Pattern" htmlFor="rp" hint="Go regular expression"><input id="rp" className="input font-mono" value={d.pattern ?? ''} onChange={(e) => setD({ ...d, pattern: e.target.value })} /></Field>
            )}
            <div className="sm:col-span-2"><Field label="Legal basis" htmlFor="rb" hint="required: the statute or rule"><input id="rb" className="input" value={d.basis} onChange={(e) => setD({ ...d, basis: e.target.value })} /></Field></div>
            <div className="sm:col-span-2"><Field label="Explanation" htmlFor="re"><textarea id="re" className="input min-h-14" value={d.explain} onChange={(e) => setD({ ...d, explain: e.target.value })} /></Field></div>
            <Field label="Effective from" htmlFor="rf"><input id="rf" type="date" className="input" value={toDate(d.from)} onChange={(e) => setD({ ...d, from: e.target.value })} /></Field>
            <Field label="Effective to" htmlFor="rt"><input id="rt" type="date" className="input" value={toDate(d.to)} onChange={(e) => setD({ ...d, to: e.target.value })} /></Field>
            <label className="flex items-center gap-2 text-sm text-slate-300 sm:col-span-2"><input type="checkbox" checked={d.enabled} onChange={(e) => setD({ ...d, enabled: e.target.checked })} /> Enabled (counsel has reviewed it)</label>
          </div>
        </Modal>
      )}
    </Page>
  )
}
