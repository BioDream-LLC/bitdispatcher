import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { BookUser, Phone as PhoneIcon, Plus, Save, Trash2, X } from 'lucide-react'
import { api, errText } from '../api'
import { Field, Modal } from '../ui'
import { cx, hhmm } from '../util'
import type { DirEntry, OnCallList, OnCallShift, Phone, View } from '../types'
import { Page, Table } from './Admin'

type Dir = {
  items: DirEntry[]
  onCall: { list: OnCallList; now?: DirEntry; until?: string; backup: boolean }[]
  categories: string[]
}
type EntryDraft = DirEntry & { isNew: boolean; tagsText: string }
type ListDraft = OnCallList & { isNew: boolean }

// A datetime-local value is a local wall-clock string; the server wants ISO. These convert each way.
const toISO = (v: string) => (v ? new Date(v).toISOString() : '')
function toLocalInput(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** Contacts directory and on-call lists: who to call, their numbers, and who is on call now. */
export default function Directory({ view }: { view: View }) {
  const [dir, setDir] = useState<Dir>({ items: [], onCall: [], categories: [] })
  const [q, setQ] = useState('')
  const [entry, setEntry] = useState<EntryDraft | null>(null)
  const [list, setList] = useState<ListDraft | null>(null)

  const load = async (query: string) => {
    const r = await api.get<Dir>(`/directory?q=${encodeURIComponent(query)}`)
    setDir({ items: r.items ?? [], onCall: r.onCall ?? [], categories: r.categories ?? [] })
  }
  // Debounce the search by 250ms.
  useEffect(() => {
    const t = setTimeout(() => { load(q).catch((e) => toast.error(errText(e))) }, 250)
    return () => clearTimeout(t)
  }, [q])

  // ---- directory entries ----
  const openEntry = (e?: DirEntry) => {
    setEntry(e
      ? { ...e, phones: e.phones ?? [], types: e.types ?? [], tags: e.tags ?? [], tagsText: (e.tags ?? []).join(', '), isNew: false }
      : { id: '', name: '', phones: [{ label: '', number: '' }], types: [], tags: [], tagsText: '', isNew: true })
  }
  const saveEntry = async () => {
    if (!entry) return
    if (!entry.name.trim()) { toast.error('A name is needed'); return }
    const body: DirEntry = {
      ...entry,
      name: entry.name.trim(),
      phones: entry.phones.filter((p) => p.number.trim()),
      tags: entry.tagsText.split(',').map((t) => t.trim()).filter(Boolean),
    }
    try {
      await api.put<{ id: string }>('/directory', body)
      toast.success(`${body.name} saved`)
      setEntry(null)
      load(q)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const removeEntry = async (id: string) => {
    if (!confirm('Remove this contact?')) return
    try {
      await api.del(`/directory/${encodeURIComponent(id)}`)
      setEntry(null)
      load(q)
    } catch (e) {
      toast.error(errText(e))
    }
  }

  // ---- on-call lists ----
  const openList = (l?: OnCallList) => {
    setList(l
      ? { ...l, types: l.types ?? [], shifts: l.shifts ?? [], isNew: false }
      : { id: '', name: '', types: [], shifts: [], isNew: true })
  }
  const saveList = async () => {
    if (!list) return
    if (!list.name.trim()) { toast.error('A name is needed'); return }
    try {
      await api.put<{ id: string }>('/oncall', { ...list, name: list.name.trim() })
      toast.success(`${list.name.trim()} saved`)
      setList(null)
      load(q)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const removeList = async (id: string) => {
    if (!confirm('Remove this on-call list?')) return
    try {
      await api.del(`/oncall/${encodeURIComponent(id)}`)
      setList(null)
      load(q)
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const callTypeCodes = view.callTypes.map((c) => c.code)
  const entryName = (id?: string) => dir.items.find((x) => x.id === id)?.name ?? (id || '')

  return (
    <Page
      title="Contacts directory and on-call lists"
      intro="Who to call and how to reach them, and which contact is on call now for each list. An incident of a given type lists the contacts on its call-out list to call."
      actions={<button className="btn btn-go" onClick={() => openEntry()}><Plus size={14} aria-hidden /> Add a contact</button>}
    >
      <div className="mb-3">
        <label className="sr-only" htmlFor="dsearch">Search contacts</label>
        <input id="dsearch" className="input max-w-md" placeholder="Search by name, org, number…" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>

      <Table head={['Name', 'Org', 'Category', 'Numbers', 'Call types']} empty={dir.items.length === 0 ? <p className="p-3 text-sm text-slate-500">No contacts{q ? ' match that search' : ' yet'}.</p> : undefined}>
        {dir.items.map((e) => (
          <tr key={e.id} className="hover:bg-slate-800/50">
            <td className="cursor-pointer px-3 py-1.5 text-slate-100" onClick={() => openEntry(e)}>{e.name}</td>
            <td className="cursor-pointer px-3 text-slate-400" onClick={() => openEntry(e)}>{e.org || '—'}</td>
            <td className="cursor-pointer px-3 text-slate-400" onClick={() => openEntry(e)}>{e.category || '—'}</td>
            <td className="px-3">
              <div className="flex flex-wrap gap-x-3 gap-y-1 text-sm">
                {(e.phones ?? []).length ? (e.phones ?? []).map((p, i) => (
                  <a key={i} href={`tel:${p.number}`} className="font-mono text-sky-300 hover:underline">
                    {p.label ? <span className="text-slate-500">{p.label}: </span> : null}{p.number}
                  </a>
                )) : <span className="text-slate-500">—</span>}
              </div>
            </td>
            <td className="px-3">
              <div className="flex flex-wrap gap-1">
                {(e.types ?? []).length ? (e.types ?? []).map((t) => <span key={t} className="chip bg-slate-800 font-mono text-slate-300">{t}</span>) : <span className="text-slate-500">—</span>}
              </div>
            </td>
          </tr>
        ))}
      </Table>

      <div className="mt-6 flex items-center gap-3">
        <h2 className="font-semibold text-slate-100">On-call lists</h2>
        <button className="btn" onClick={() => openList()}><Plus size={13} aria-hidden /> Add a list</button>
      </div>
      <div className="mt-2 grid gap-3 sm:grid-cols-2">
        {dir.onCall.length === 0 && <p className="text-sm text-slate-500">No on-call lists yet.</p>}
        {dir.onCall.map((o) => (
          <button key={o.list.id} type="button" className="panel rounded-lg p-3 text-left hover:border-slate-600" onClick={() => openList(o.list)}>
            <div className="font-semibold text-slate-100">{o.list.name}</div>
            <div className="mt-1 text-sm text-slate-300">
              {o.now ? (
                <>
                  On call: {o.now.name}
                  {o.now.phones?.[0] && <span className="ml-1 font-mono text-sky-300">{o.now.phones[0].number}</span>}
                  <span className="ml-1 text-xs text-slate-500">{o.until ? `until ${hhmm(o.until)}` : ''}{o.backup ? ' (backup)' : ''}</span>
                </>
              ) : (
                <span className="text-amber-300">{o.backup ? 'backup: no shift covers now' : 'no one on call now'}</span>
              )}
            </div>
            {(o.list.types ?? []).length > 0 && (
              <div className="mt-2 flex flex-wrap gap-1">
                {(o.list.types ?? []).map((t) => <span key={t} className="chip bg-slate-800 font-mono text-slate-400">{t}</span>)}
              </div>
            )}
          </button>
        ))}
      </div>

      {entry && (
        <Modal
          open
          wide
          onClose={() => setEntry(null)}
          title={entry.isNew ? 'New contact' : entry.name}
          icon={<BookUser size={16} className="text-sky-300" aria-hidden />}
          footer={
            <>
              {!entry.isNew && <button className="btn mr-auto text-rose-300" onClick={() => removeEntry(entry.id)}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => setEntry(null)}>Cancel</button>
              <button className="btn btn-go" onClick={saveEntry}><Save size={13} aria-hidden /> Save</button>
            </>
          }
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Name" htmlFor="en"><input id="en" className="input" value={entry.name} onChange={(e) => setEntry({ ...entry, name: e.target.value })} /></Field>
            <Field label="Organisation" htmlFor="eo"><input id="eo" className="input" value={entry.org ?? ''} onChange={(e) => setEntry({ ...entry, org: e.target.value || undefined })} /></Field>
            <Field label="Category" htmlFor="ec">
              <input id="ec" className="input" list="dir-cats" value={entry.category ?? ''} onChange={(e) => setEntry({ ...entry, category: e.target.value || undefined })} />
            </Field>
            <datalist id="dir-cats">{dir.categories.map((c) => <option key={c} value={c} />)}</datalist>
            <Field label="Email" htmlFor="ee"><input id="ee" type="email" className="input" value={entry.email ?? ''} onChange={(e) => setEntry({ ...entry, email: e.target.value || undefined })} /></Field>
            <div className="sm:col-span-2">
              <span className="label">Phone numbers</span>
              <ul className="space-y-1">
                {entry.phones.map((p, i) => (
                  <li key={i} className="flex items-center gap-2">
                    <input aria-label={`Number ${i + 1} label`} className="input w-36" placeholder="label (desk, cell…)" value={p.label ?? ''} onChange={(e) => setEntry({ ...entry, phones: entry.phones.map((x, j) => (j === i ? { ...x, label: e.target.value } : x)) })} />
                    <input aria-label={`Number ${i + 1}`} className="input font-mono" placeholder="+15405550100" value={p.number} onChange={(e) => setEntry({ ...entry, phones: entry.phones.map((x, j) => (j === i ? { ...x, number: e.target.value } : x)) })} />
                    <button className="btn btn-ghost px-1.5" aria-label={`Remove number ${i + 1}`} onClick={() => setEntry({ ...entry, phones: entry.phones.filter((_, j) => j !== i) })}><X size={13} aria-hidden /></button>
                  </li>
                ))}
              </ul>
              <button className="btn mt-2" onClick={() => setEntry({ ...entry, phones: [...entry.phones, { label: '', number: '' } as Phone] })}><Plus size={13} aria-hidden /> Add a number</button>
            </div>
            <div className="sm:col-span-2">
              <Field label="Address" htmlFor="ea"><input id="ea" className="input" value={entry.address ?? ''} onChange={(e) => setEntry({ ...entry, address: e.target.value || undefined })} /></Field>
            </div>
            <div className="sm:col-span-2">
              <Field label="Notes" htmlFor="enotes"><input id="enotes" className="input" value={entry.notes ?? ''} onChange={(e) => setEntry({ ...entry, notes: e.target.value || undefined })} /></Field>
            </div>
            <div className="sm:col-span-2">
              <Field label="Tags" htmlFor="etags" hint="Separated by commas."><input id="etags" className="input" value={entry.tagsText} onChange={(e) => setEntry({ ...entry, tagsText: e.target.value })} /></Field>
            </div>
            <div className="sm:col-span-2">
              <span className="label">Call types to call this contact on</span>
              <p className="mb-1 text-xs text-slate-500">Incidents of these types list this contact to call.</p>
              <div className="flex flex-wrap gap-1.5">
                {callTypeCodes.map((code) => {
                  const on = (entry.types ?? []).includes(code)
                  return <button key={code} type="button" className={cx('chip cursor-pointer font-mono', on ? 'bg-sky-500/20 text-sky-200' : 'bg-slate-800 text-slate-400 hover:bg-slate-700')} aria-pressed={on} onClick={() => setEntry({ ...entry, types: on ? (entry.types ?? []).filter((x) => x !== code) : [...(entry.types ?? []), code] })}>{code}</button>
                })}
                {callTypeCodes.length === 0 && <span className="text-xs text-slate-500">No call types configured.</span>}
              </div>
            </div>
          </div>
        </Modal>
      )}

      {list && (
        <Modal
          open
          wide
          onClose={() => setList(null)}
          title={list.isNew ? 'New on-call list' : list.name}
          icon={<PhoneIcon size={16} className="text-emerald-300" aria-hidden />}
          footer={
            <>
              {!list.isNew && <button className="btn mr-auto text-rose-300" onClick={() => removeList(list.id)}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => setList(null)}>Cancel</button>
              <button className="btn btn-go" onClick={saveList}><Save size={13} aria-hidden /> Save</button>
            </>
          }
        >
          <div className="space-y-3">
            <Field label="Name" htmlFor="ln"><input id="ln" className="input" value={list.name} onChange={(e) => setList({ ...list, name: e.target.value })} /></Field>
            <div>
              <span className="label">Call types</span>
              <div className="flex flex-wrap gap-1.5">
                {callTypeCodes.map((code) => {
                  const on = (list.types ?? []).includes(code)
                  return <button key={code} type="button" className={cx('chip cursor-pointer font-mono', on ? 'bg-emerald-500/20 text-emerald-200' : 'bg-slate-800 text-slate-400 hover:bg-slate-700')} aria-pressed={on} onClick={() => setList({ ...list, types: on ? (list.types ?? []).filter((x) => x !== code) : [...(list.types ?? []), code] })}>{code}</button>
                })}
                {callTypeCodes.length === 0 && <span className="text-xs text-slate-500">No call types configured.</span>}
              </div>
            </div>
            <Field label="Backup contact" htmlFor="lb" hint="Called when no shift covers the moment.">
              <select id="lb" className="input" value={list.backup ?? ''} onChange={(e) => setList({ ...list, backup: e.target.value || undefined })}>
                <option value="">— none —</option>
                {dir.items.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
              </select>
            </Field>
            <Field label="Notes" htmlFor="lnotes"><input id="lnotes" className="input" value={list.notes ?? ''} onChange={(e) => setList({ ...list, notes: e.target.value || undefined })} /></Field>
            <div>
              <span className="label">Shifts</span>
              <ul className="space-y-1">
                {list.shifts.map((s, i) => (
                  <li key={i} className="flex flex-wrap items-center gap-2">
                    <select aria-label={`Shift ${i + 1} contact`} className="input w-48" value={s.entry} onChange={(e) => setList({ ...list, shifts: list.shifts.map((x, j) => (j === i ? { ...x, entry: e.target.value } : x)) })}>
                      <option value="">— pick a contact —</option>
                      {dir.items.map((x) => <option key={x.id} value={x.id}>{x.name}</option>)}
                    </select>
                    <input aria-label={`Shift ${i + 1} from`} type="datetime-local" className="input w-52" value={toLocalInput(s.from)} onChange={(e) => setList({ ...list, shifts: list.shifts.map((x, j) => (j === i ? { ...x, from: toISO(e.target.value) } : x)) })} />
                    <span className="text-slate-500">to</span>
                    <input aria-label={`Shift ${i + 1} to`} type="datetime-local" className="input w-52" value={toLocalInput(s.to)} onChange={(e) => setList({ ...list, shifts: list.shifts.map((x, j) => (j === i ? { ...x, to: toISO(e.target.value) } : x)) })} />
                    <button className="btn btn-ghost px-1.5" aria-label={`Remove shift ${i + 1}`} onClick={() => setList({ ...list, shifts: list.shifts.filter((_, j) => j !== i) })}><X size={13} aria-hidden /></button>
                  </li>
                ))}
              </ul>
              {list.shifts.length === 0 && <p className="text-xs text-slate-500">No shifts yet. Add one so someone is on call.</p>}
              <button className="btn mt-2" onClick={() => setList({ ...list, shifts: [...list.shifts, { entry: '', from: '', to: '' } as OnCallShift] })}><Plus size={13} aria-hidden /> Add a shift</button>
            </div>
            {list.backup && <p className="text-xs text-slate-500">Backup: {entryName(list.backup)}</p>}
          </div>
        </Modal>
      )}
    </Page>
  )
}
