import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ListChecks, Plus, Save, Trash2 } from 'lucide-react'
import { api, errText } from '../api'
import { cx } from '../util'
import type { Determinant, Protocol, View } from '../types'
import { Page, Table } from './Admin'
import { Field, Modal } from '../ui'

type Q = Protocol['questions'][number]
const blank = (): Protocol => ({ id: '', name: '', version: '1', approved: false, callTypes: [], start: 'q1', questions: [{ id: 'q1', text: '', answers: [{ label: 'Yes' }, { label: 'No' }] }], determinants: [] })
const nextQ = (qs: Q[]) => { let n = qs.length + 1; while (qs.some((q) => q.id === `q${n}`)) n++; return `q${n}` }

/** Protocols: the agency's call-taking question sets. Each answer leads to the next question or ends at
 * a determinant, which can set the priority and the call type and gives the instructions for the caller.
 * Only approved protocols are offered on calls. Licensed protocol software reports through a hook instead. */
export default function Protocols({ view }: { view: View }) {
  const [list, setList] = useState<Protocol[]>([])
  const [d, setD] = useState<(Protocol & { isNew: boolean }) | null>(null)
  const load = () => api.get<Protocol[]>('/admin/protocols').then(setList).catch((e) => toast.error(errText(e)))
  useEffect(() => { load() }, [])
  const save = async () => {
    if (!d) return
    const { isNew: _, ...p } = d
    try {
      await api.put('/admin/protocols', p)
      toast.success('Saved')
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const setQ = (i: number, q: Partial<Q>) => d && setD({ ...d, questions: d.questions.map((x, j) => (j === i ? { ...x, ...q } : x)) })
  const setDet = (i: number, x: Partial<Determinant>) => d && setD({ ...d, determinants: d.determinants.map((y, j) => (j === i ? { ...y, ...x } : y)) })
  return (
    <Page title="Call-taking protocols" intro="Your agency's question sets, approved by your medical director or chief: the call-taker answers one question at a time and ends at a determinant that sets the priority and gives the caller's instructions. Protocol software you license (ProQA and others) can report its determinant instead (docs/PROTOCOLS.md)."
      actions={<>
        <button className="btn" onClick={async () => setD({ ...(await api.get<Protocol>('/admin/protocols/example')), isNew: true })}>Start from the example</button>
        <button className="btn btn-go" onClick={() => setD({ ...blank(), isNew: true })}><Plus size={14} aria-hidden /> New protocol</button>
      </>}>
      <Table head={['Protocol', 'Version', 'Call types', 'Questions', 'Status', '']} empty={!list.length && 'No protocols yet.'}>
        {list.map((p) => (
          <tr key={p.id} className="border-t border-slate-800">
            <td className="px-3 py-2"><div className="font-semibold text-slate-100">{p.name}</div><div className="font-mono text-[11px] text-slate-500">{p.id}</div></td>
            <td className="px-3 py-2 font-mono text-xs">{p.version}</td>
            <td className="px-3 py-2 text-xs">{p.callTypes.length ? p.callTypes.join(', ') : 'all'}</td>
            <td className="px-3 py-2 text-xs">{p.questions.length} · {p.determinants.length} determinants</td>
            <td className="px-3 py-2"><span className={cx('chip', p.approved ? 'bg-emerald-700 text-white' : 'bg-slate-700 text-slate-200')}>{p.approved ? 'approved' : 'draft'}</span></td>
            <td className="px-3 py-2 text-right">
              <button className="btn btn-ghost h-7 text-xs" onClick={() => setD({ ...p, isNew: false })}>Change</button>
              <button className="btn btn-ghost h-7 text-xs" aria-label={`Remove ${p.name}`} onClick={async () => { if (confirm(`Remove ${p.name}?`)) { await api.del(`/admin/protocols/${encodeURIComponent(p.id)}`).catch((e) => toast.error(errText(e))); load() } }}><Trash2 size={13} aria-hidden /></button>
            </td>
          </tr>
        ))}
      </Table>
      <Modal open={!!d} onClose={() => setD(null)} xl title={d?.isNew ? 'New protocol' : d?.name} icon={<ListChecks size={16} aria-hidden />}
        footer={<><button className="btn btn-ghost" onClick={() => setD(null)}>Cancel</button><button className="btn btn-go" onClick={save}><Save size={14} aria-hidden /> Save</button></>}>
        {d && (
          <div className="space-y-4">
            <div className="grid gap-2 md:grid-cols-4">
              <Field label="ID" htmlFor="p-id"><input id="p-id" className="input font-mono" value={d.id} disabled={!d.isNew} onChange={(e) => setD({ ...d, id: e.target.value })} placeholder="BREATHING" /></Field>
              <Field label="Name" htmlFor="p-nm"><input id="p-nm" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} /></Field>
              <Field label="Version" htmlFor="p-v"><input id="p-v" className="input font-mono" value={d.version} onChange={(e) => setD({ ...d, version: e.target.value })} /></Field>
              <label className="mt-6 flex items-center gap-2 text-sm"><input type="checkbox" checked={d.approved} onChange={(e) => setD({ ...d, approved: e.target.checked })} /> Approved for calls</label>
            </div>
            <fieldset>
              <legend className="label">Offered for these call types (none: all)</legend>
              <div className="flex flex-wrap gap-1">
                {view.callTypes.map((c) => {
                  const on = d.callTypes.includes(c.code)
                  return <button key={c.code} type="button" aria-pressed={on} className={cx('chip', on ? 'bg-sky-600 text-white' : 'bg-slate-800 text-slate-300')}
                    onClick={() => setD({ ...d, callTypes: on ? d.callTypes.filter((x) => x !== c.code) : [...d.callTypes, c.code] })}>{c.code}</button>
                })}
              </div>
            </fieldset>
            <section aria-label="Questions" className="space-y-2">
              <h3 className="text-sm font-semibold text-slate-200">Questions <span className="font-normal text-slate-500">(the first is {d.start})</span></h3>
              {d.questions.map((q, i) => (
                <div key={i} className="rounded border border-slate-800 p-2">
                  <div className="flex gap-2">
                    <span className="mt-1.5 font-mono text-xs text-slate-400">{q.id}</span>
                    <input className="input flex-1" value={q.text} onChange={(e) => setQ(i, { text: e.target.value })} placeholder="The question, as the call-taker asks it" aria-label={`Question ${q.id}`} />
                    {d.start !== q.id && <button className="btn btn-ghost h-8 text-xs" onClick={() => setD({ ...d, start: q.id })}>Make first</button>}
                    <button className="btn btn-ghost h-8" aria-label={`Remove ${q.id}`} onClick={() => setD({ ...d, questions: d.questions.filter((_, j) => j !== i) })}><Trash2 size={13} aria-hidden /></button>
                  </div>
                  <ul className="mt-1 space-y-1 pl-8">
                    {q.answers.map((a, k) => (
                      <li key={k} className="flex items-center gap-2">
                        <input className="input h-7 w-40 text-xs" value={a.label} onChange={(e) => setQ(i, { answers: q.answers.map((x, m) => (m === k ? { ...x, label: e.target.value } : x)) })} aria-label="Answer" />
                        <span className="text-xs text-slate-500">→</span>
                        <select className="input h-7 w-auto text-xs" aria-label="Then" value={a.next ? `q:${a.next}` : a.determinant ? `d:${a.determinant}` : ''}
                          onChange={(e) => { const [k2, v] = e.target.value.split(/:(.*)/); setQ(i, { answers: q.answers.map((x, m) => (m === k ? { label: x.label, ...(k2 === 'q' ? { next: v } : k2 === 'd' ? { determinant: v } : {}) } : x)) }) }}>
                          <option value="">choose…</option>
                          <optgroup label="Next question">{d.questions.filter((x) => x.id !== q.id).map((x) => <option key={x.id} value={`q:${x.id}`}>{x.id}: {x.text.slice(0, 40)}</option>)}</optgroup>
                          <optgroup label="Determinant">{d.determinants.map((x) => <option key={x.code} value={`d:${x.code}`}>{x.code}: {x.text.slice(0, 40)}</option>)}</optgroup>
                        </select>
                        <button className="btn btn-ghost h-7" aria-label="Remove the answer" onClick={() => setQ(i, { answers: q.answers.filter((_, m) => m !== k) })}><Trash2 size={12} aria-hidden /></button>
                      </li>
                    ))}
                    <li><button className="btn btn-ghost h-6 text-xs" onClick={() => setQ(i, { answers: [...q.answers, { label: '' }] })}><Plus size={12} aria-hidden /> Answer</button></li>
                  </ul>
                </div>
              ))}
              <button className="btn h-7 text-xs" onClick={() => setD({ ...d, questions: [...d.questions, { id: nextQ(d.questions), text: '', answers: [{ label: 'Yes' }, { label: 'No' }] }] })}><Plus size={12} aria-hidden /> Question</button>
            </section>
            <section aria-label="Determinants" className="space-y-2">
              <h3 className="text-sm font-semibold text-slate-200">Determinants</h3>
              {d.determinants.map((x, i) => (
                <div key={i} className="grid gap-2 rounded border border-slate-800 p-2 md:grid-cols-[8rem_1fr_6rem_8rem_auto]">
                  <input className="input font-mono" value={x.code} onChange={(e) => setDet(i, { code: e.target.value })} placeholder="Code" aria-label="Code" />
                  <input className="input" value={x.text} onChange={(e) => setDet(i, { text: e.target.value })} placeholder="What it means" aria-label="Meaning" />
                  <select className="input" value={x.priority ?? 0} onChange={(e) => setDet(i, { priority: Number(e.target.value) || undefined })} aria-label="Priority">
                    <option value={0}>keep P</option>{[1, 2, 3, 4, 5].map((n) => <option key={n} value={n}>P{n}</option>)}
                  </select>
                  <select className="input" value={x.callType ?? ''} onChange={(e) => setDet(i, { callType: e.target.value || undefined })} aria-label="Call type">
                    <option value="">keep type</option>{view.callTypes.map((c) => <option key={c.code} value={c.code}>{c.code}</option>)}
                  </select>
                  <button className="btn btn-ghost" aria-label={`Remove ${x.code}`} onClick={() => setD({ ...d, determinants: d.determinants.filter((_, j) => j !== i) })}><Trash2 size={13} aria-hidden /></button>
                  <textarea className="input md:col-span-5" rows={2} value={(x.instructions ?? []).join('\n')} onChange={(e) => setDet(i, { instructions: e.target.value.split('\n').filter((l) => l.trim()) })} placeholder="Instructions for the caller, one per line" aria-label="Instructions" />
                </div>
              ))}
              <button className="btn h-7 text-xs" onClick={() => setD({ ...d, determinants: [...d.determinants, { code: '', text: '' }] })}><Plus size={12} aria-hidden /> Determinant</button>
            </section>
          </div>
        )}
      </Modal>
    </Page>
  )
}
