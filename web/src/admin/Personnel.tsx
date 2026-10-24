import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Plus, Save, Trash2, Users as UsersIcon, X } from 'lucide-react'
import { api, errText } from '../api'
import { Field, Modal } from '../ui'
import { cx } from '../util'
import type { CapNeed, Member, View } from '../types'
import { Page, Table } from './Admin'

type Roster = { members: (Member & { riding?: string })[]; qualifications: string[]; capNeeds: CapNeed[] }
type Draft = Member & { isNew: boolean }

const blank = (): Member => ({ id: '', name: '', quals: [] })

/** Personnel and crews: the roster of responders, what each is qualified for, and what a crew must have. */
export default function Personnel({ view }: { view: View }) {
  const [roster, setRoster] = useState<Roster>({ members: [], qualifications: [], capNeeds: [] })
  const [d, setD] = useState<Draft | null>(null)
  const [newQual, setNewQual] = useState('')
  const load = async () => {
    const r = await api.get<Roster>('/roster')
    setRoster({ members: r.members ?? [], qualifications: r.qualifications ?? [], capNeeds: r.capNeeds ?? [] })
  }
  useEffect(() => {
    load().catch((e) => toast.error(errText(e)))
  }, [])

  const open = (m?: Member) => {
    setNewQual('')
    setD(m ? { ...m, quals: m.quals ?? [], isNew: false } : { ...blank(), isNew: true })
  }
  const toggleQual = (q: string) => {
    if (!d) return
    const has = (d.quals ?? []).includes(q)
    setD({ ...d, quals: has ? (d.quals ?? []).filter((x) => x !== q) : [...(d.quals ?? []), q] })
  }
  const addQual = () => {
    if (!d) return
    const q = newQual.trim().toLowerCase()
    if (q && !(d.quals ?? []).includes(q)) setD({ ...d, quals: [...(d.quals ?? []), q] })
    setNewQual('')
  }
  const save = async () => {
    if (!d) return
    const id = d.id.trim()
    if (!id) { toast.error('A number (ID) is needed'); return }
    try {
      await api.put('/admin/members', { ...d, id })
      toast.success(`${d.name || id} saved`)
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const remove = async (id: string) => {
    if (!confirm('Remove this person from the roster?')) return
    try {
      await api.del(`/admin/members/${encodeURIComponent(id)}`)
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const importCsv = async (text: string): Promise<{ saved: number; problems: string[] }> => {
    const res = await fetch('./api/admin/members/import', {
      method: 'POST',
      headers: { 'X-BitDispatcher': '1', 'Content-Type': 'text/csv' },
      body: text,
      credentials: 'same-origin',
    })
    const data = (await res.json().catch(() => ({}))) as { saved?: number; problems?: string[]; message?: string }
    if (!res.ok) throw new Error(data.message || `Import failed (${res.status})`)
    return { saved: data.saved ?? 0, problems: data.problems ?? [] }
  }

  const stationName = (id?: string) => view.stations.find((s) => s.id === id)?.name ?? (id || '')
  // The qualifications a person might toggle: the configured list plus any already on this person.
  const qualOptions = d ? [...new Set([...roster.qualifications, ...(d.quals ?? [])])] : roster.qualifications

  return (
    <Page
      title="Personnel and crews"
      intro="Everyone who rides, and what each is qualified for. The roster decides who can fill a seat and whether a crew meets what its unit must carry. Pages reach people by text through the number or push topic here."
      actions={<>
        <label className="btn cursor-pointer" title="A CSV with a header row: id, name, rank, qualifications (separated by ;), station, phone, email, volunteer (yes/no)">
          Import from CSV
          <input type="file" accept=".csv,text/csv" className="sr-only" onChange={async (e) => {
            const f = e.target.files?.[0]
            e.target.value = ''
            if (!f) return
            try {
              const { saved, problems } = await importCsv(await f.text())
              if (problems.length) toast.error(`${saved} saved. ${problems.length} problem${problems.length === 1 ? '' : 's'}: ${problems.join('; ')}`)
              else toast.success(`${saved} saved`)
              load()
            } catch (err) {
              toast.error(errText(err))
            }
          }} />
        </label>
        <button className="btn btn-go" onClick={() => open()}><Plus size={14} aria-hidden /> Add a person</button>
      </>}
    >
      <Table head={['Name', 'Number', 'Rank', 'Qualifications', 'Station', 'Volunteer', 'Riding', 'State']} empty={roster.members.length === 0 ? <p className="p-3 text-sm text-slate-500">No one on the roster yet. Add a person, or import a CSV.</p> : undefined}>
        {roster.members.map((m) => (
          <tr key={m.id} className="cursor-pointer hover:bg-slate-800/50" onClick={() => open(m)}>
            <td className="px-3 py-1.5 text-slate-100">{m.name || m.id}</td>
            <td className="px-3 font-mono text-slate-400">{m.id}</td>
            <td className="px-3 text-slate-400">{m.rank || '—'}</td>
            <td className="px-3">
              <div className="flex flex-wrap gap-1">
                {(m.quals ?? []).length ? (m.quals ?? []).map((q) => <span key={q} className="chip bg-slate-800 text-slate-300">{q}</span>) : <span className="text-slate-500">—</span>}
              </div>
            </td>
            <td className="px-3 text-slate-400">{stationName(m.station) || '—'}</td>
            <td className="px-3 text-slate-400">{m.volunteer ? 'Yes' : '—'}</td>
            <td className="px-3 font-mono text-slate-300">{m.riding || '—'}</td>
            <td className="px-3 text-xs">{m.inactive ? <span className="text-slate-500">inactive</span> : <span className="text-emerald-300">active</span>}</td>
          </tr>
        ))}
      </Table>

      <div className="panel mt-6 rounded-lg p-4">
        <h2 className="font-semibold text-slate-100">What a crew must have</h2>
        <p className="mt-1 text-sm text-slate-400">
          Applies only to units whose crew is recorded. Edit these in Admin › Settings.
        </p>
        {roster.capNeeds.length ? (
          <ul className="mt-3 space-y-1 text-sm text-slate-300">
            {roster.capNeeds.map((n, i) => (
              <li key={i}>
                <span className="font-mono text-slate-100">{n.capability}</span> needs {n.count} {n.qualification} aboard
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-3 text-sm text-slate-500">No crew rules set. Any recorded crew will do.</p>
        )}
      </div>

      {d && (
        <Modal
          open
          wide
          onClose={() => setD(null)}
          title={d.isNew ? 'New person' : d.name || d.id}
          icon={<UsersIcon size={16} className="text-sky-300" aria-hidden />}
          footer={
            <>
              {!d.isNew && <button className="btn mr-auto text-rose-300" onClick={() => remove(d.id)}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => setD(null)}>Cancel</button>
              <button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save</button>
            </>
          }
        >
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Number (ID)" htmlFor="mid"><input id="mid" className="input font-mono" disabled={!d.isNew} value={d.id} onChange={(e) => setD({ ...d, id: e.target.value })} /></Field>
            <Field label="Name" htmlFor="mn"><input id="mn" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} /></Field>
            <Field label="Rank" htmlFor="mr"><input id="mr" className="input" value={d.rank ?? ''} onChange={(e) => setD({ ...d, rank: e.target.value || undefined })} /></Field>
            <Field label="Station" htmlFor="ms">
              <select id="ms" className="input" value={d.station ?? ''} onChange={(e) => setD({ ...d, station: e.target.value || undefined })}>
                <option value="">— none —</option>
                {view.stations.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
              </select>
            </Field>
            <div className="sm:col-span-2">
              <span className="label">Qualifications</span>
              <div className="flex flex-wrap gap-1.5">
                {qualOptions.map((q) => {
                  const on = (d.quals ?? []).includes(q)
                  return (
                    <button
                      key={q}
                      type="button"
                      className={cx('chip cursor-pointer', on ? 'bg-sky-500/20 text-sky-200' : 'bg-slate-800 text-slate-400 hover:bg-slate-700')}
                      aria-pressed={on}
                      onClick={() => toggleQual(q)}
                    >
                      {q}
                    </button>
                  )
                })}
                {qualOptions.length === 0 && <span className="text-xs text-slate-500">No qualifications configured yet; add one below.</span>}
              </div>
              <div className="mt-2 flex gap-2">
                <input
                  aria-label="Add another qualification"
                  className="input font-mono"
                  placeholder="add another, e.g. paramedic"
                  value={newQual}
                  onChange={(e) => setNewQual(e.target.value)}
                  onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addQual() } }}
                />
                <button type="button" className="btn" onClick={addQual}><Plus size={13} aria-hidden /> Add</button>
              </div>
            </div>
            <Field label="Phone" htmlFor="mp" hint="International form, e.g. +15405550100. Used for pages by text."><input id="mp" className="input font-mono" value={d.phone ?? ''} onChange={(e) => setD({ ...d, phone: e.target.value || undefined })} /></Field>
            <Field label="Email" htmlFor="me"><input id="me" type="email" className="input" value={d.email ?? ''} onChange={(e) => setD({ ...d, email: e.target.value || undefined })} /></Field>
            <div className="sm:col-span-2">
              <Field label="Push topic" htmlFor="mt" hint="An ntfy topic for pages to this person's phone.">
                <input id="mt" className="input font-mono" value={d.topic ?? ''} onChange={(e) => setD({ ...d, topic: e.target.value || undefined })} />
              </Field>
            </div>
            <label className="flex items-center gap-2 text-sm text-slate-300">
              <input type="checkbox" checked={!!d.volunteer} onChange={(e) => setD({ ...d, volunteer: e.target.checked })} /> Volunteer
            </label>
            <label className="flex items-center gap-2 text-sm text-slate-300">
              <input type="checkbox" checked={!!d.inactive} onChange={(e) => setD({ ...d, inactive: e.target.checked })} /> Inactive (off the roster for now)
            </label>
            <div className="sm:col-span-2">
              <Field label="Note" htmlFor="mnote"><input id="mnote" className="input" value={d.note ?? ''} onChange={(e) => setD({ ...d, note: e.target.value || undefined })} /></Field>
            </div>
          </div>
          {!d.isNew && (d.quals ?? []).length > 0 && (
            <p className="mt-3 flex items-center gap-1 text-xs text-slate-500">
              <X size={11} aria-hidden className="opacity-0" />Tap a qualification above to turn it off.
            </p>
          )}
        </Modal>
      )}
    </Page>
  )
}
