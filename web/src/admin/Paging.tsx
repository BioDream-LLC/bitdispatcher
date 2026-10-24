import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { Hand, Plus, Radio, Save, Trash2 } from 'lucide-react'
import { api, errText } from '../api'
import { Field, Modal } from '../ui'
import { cx } from '../util'
import type { PageGroup, View } from '../types'
import { Page, Table } from './Admin'

const blank = (): PageGroup => ({ id: '', name: '', members: [], gateways: [], units: [], stations: [], types: [], note: '', off: false })

type Draft = PageGroup & { isNew: boolean }

// Toggle a code in and out of a list on the draft.
const toggle = (list: string[] | undefined, code: string) => {
  const l = list ?? []
  return l.includes(code) ? l.filter((x) => x !== code) : [...l, code]
}

/** Paging groups: who is paged together and how. A group can be paged by hand or automatically when
 * one of its units or stations is dispatched, or a call type it watches gets units. */
export default function Paging({ view }: { view: View }) {
  const roadmap = view.roadmap
  const groups = roadmap?.pageGroups ?? []
  const gateways = roadmap?.gateways ?? []
  const members = roadmap?.members ?? []
  const pages = roadmap?.pages ?? []
  const [d, setD] = useState<Draft | null>(null)
  const [search, setSearch] = useState('')
  const gatewayName = (id: string) => gateways.find((g) => g.id === id)?.name ?? id
  const open = (g?: PageGroup) => {
    const base = g ?? blank()
    setSearch('')
    setD({ ...base, members: [...(base.members ?? [])], gateways: [...(base.gateways ?? [])], units: [...(base.units ?? [])], stations: [...(base.stations ?? [])], types: [...(base.types ?? [])], isNew: !g })
  }
  const save = async () => {
    if (!d) return
    try {
      const { isNew, ...g } = d
      await api.put<{ id: string }>('/admin/pagegroups', { ...g, name: g.name.trim() })
      toast.success(`${d.name.trim() || 'Group'} saved`)
      setD(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const remove = async (id: string) => {
    if (!confirm('Remove this paging group?')) return
    try {
      await api.del(`/admin/pagegroups/${encodeURIComponent(id)}`)
      setD(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const autoSummary = (g: PageGroup) => {
    const parts: string[] = []
    if (g.units?.length) parts.push(`${g.units.length} unit${g.units.length === 1 ? '' : 's'}`)
    if (g.stations?.length) parts.push(`${g.stations.length} station${g.stations.length === 1 ? '' : 's'}`)
    if (g.types?.length) parts.push(`${g.types.length} call type${g.types.length === 1 ? '' : 's'}`)
    return parts.length ? parts.join(', ') : 'by hand only'
  }
  const shownMembers = useMemo(() => {
    const active = members.filter((m) => !m.inactive)
    const q = search.trim().toLowerCase()
    if (!q) return active
    return active.filter((m) => m.name.toLowerCase().includes(q) || (m.station ?? '').toLowerCase().includes(q) || (m.quals ?? []).some((x) => x.toLowerCase().includes(q)))
  }, [members, search])
  const clock = (iso: string) => new Date(iso).toLocaleString()
  return (
    <Page
      title="Paging groups"
      intro="Who is paged together and how. A group can be paged by hand (the PAGE command or the incident's Page button) and automatically when one of its units or stations is dispatched, or an incident of one of its call types gets units. Each person gets a link to answer responding, straight to the scene, or not available."
      actions={<button className="btn btn-go" onClick={() => open()} disabled={gateways.length === 0}><Plus size={14} aria-hidden /> Add a group</button>}
    >
      {gateways.length === 0 && (
        <div className="panel mb-4 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-200">
          No paging gateway yet. <Link to="/admin/integrations" className="underline">Set up a paging gateway first</Link> so a page has a way out.
        </div>
      )}

      <Table head={['Name', 'People', 'Gateways', 'Automatic', 'On']} empty={groups.length === 0 ? <p className="p-3 text-sm text-slate-500">No paging groups yet.</p> : undefined}>
        {groups.map((g) => (
          <tr key={g.id} className="cursor-pointer hover:bg-slate-800/50" onClick={() => open(g)}>
            <td className="px-3 py-1.5 text-slate-100">{g.name || g.id}</td>
            <td className="px-3 text-slate-400">{g.members?.length ?? 0}</td>
            <td className="px-3 text-slate-400">{g.gateways?.length ? g.gateways.map(gatewayName).join(', ') : '—'}</td>
            <td className="px-3 text-slate-400">{autoSummary(g)}</td>
            <td className="px-3"><span className={cx('chip', g.off ? 'bg-slate-800 text-slate-400' : 'bg-emerald-500/20 text-emerald-200')}>{g.off ? 'Off' : 'On'}</span></td>
          </tr>
        ))}
      </Table>

      <h2 className="mt-6 mb-2 text-sm font-semibold text-slate-200">Recent pages</h2>
      <Table head={['Time', 'Incident', 'Subject', 'How', 'People', 'Delivered', 'Answers']} empty={pages.length === 0 ? <p className="p-3 text-sm text-slate-500">No pages yet.</p> : undefined}>
        {pages.map((p) => {
          const ok = p.results.filter((r) => r.ok).length
          const failed = p.results.length - ok
          const answers = Object.keys(p.replies ?? {}).length
          return (
            <tr key={p.id} className="hover:bg-slate-800/50">
              <td className="px-3 py-1.5 text-slate-400">{clock(p.at)}</td>
              <td className="px-3 font-mono text-slate-300">{p.incident ?? '—'}</td>
              <td className="px-3 text-slate-200">{p.subject}</td>
              <td className="px-3 text-slate-400">{p.auto ? 'Automatic' : 'By hand'}</td>
              <td className="px-3 text-slate-400">{p.to?.length ?? 0}</td>
              <td className="px-3"><span className="text-emerald-300">{ok} ok</span>{failed > 0 && <span className="text-rose-300"> · {failed} failed</span>}</td>
              <td className="px-3 text-slate-400">{answers}</td>
            </tr>
          )
        })}
      </Table>

      {d && (
        <Modal
          open
          onClose={() => setD(null)}
          wide
          title={d.isNew ? 'New paging group' : d.name || d.id}
          icon={<Radio size={16} className="text-sky-300" aria-hidden />}
          footer={
            <>
              {!d.isNew && <button className="btn mr-auto text-rose-300" onClick={() => remove(d.id)}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => setD(null)}>Cancel</button>
              <button className="btn btn-go" onClick={save} disabled={!d.name.trim()}><Save size={13} aria-hidden /> Save</button>
            </>
          }
        >
          <div className="space-y-4">
            <Field label="Name" htmlFor="pg-name"><input id="pg-name" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} placeholder="Station 1 volunteers" /></Field>

            <fieldset>
              <legend className="label">People</legend>
              <input className="input mb-2" placeholder="Search by name, station or qualification" value={search} onChange={(e) => setSearch(e.target.value)} aria-label="Search people" />
              <div className="max-h-56 space-y-0.5 overflow-y-auto rounded border border-slate-800 p-2">
                {shownMembers.length === 0 && <p className="text-xs text-slate-500">No one matches.</p>}
                {shownMembers.map((m) => (
                  <label key={m.id} className="flex items-center gap-2 rounded px-1 py-0.5 text-sm text-slate-300 hover:bg-slate-800/60">
                    <input type="checkbox" checked={d.members.includes(m.id)} onChange={() => setD({ ...d, members: toggle(d.members, m.id) })} />
                    <span className="text-slate-100">{m.name}</span>
                    {m.station && <span className="text-xs text-slate-500">{m.station}</span>}
                    {m.quals?.length ? <span className="text-xs text-slate-500">{m.quals.join(', ')}</span> : null}
                    {m.volunteer && <span className="chip bg-sky-500/20 text-sky-200">volunteer</span>}
                  </label>
                ))}
              </div>
              <p className="mt-1 text-xs text-slate-500">{d.members.length} chosen.</p>
            </fieldset>

            <fieldset>
              <legend className="label">Gateways</legend>
              <div className="flex flex-wrap gap-1">
                {gateways.map((g) => (
                  <button key={g.id} type="button" aria-pressed={d.gateways.includes(g.id)} className={cx('chip', d.gateways.includes(g.id) ? 'bg-sky-600 text-white' : 'bg-slate-800 text-slate-300')} onClick={() => setD({ ...d, gateways: toggle(d.gateways, g.id) })}>{g.name}</button>
                ))}
                {gateways.length === 0 && <span className="text-xs text-slate-500">No gateways set up.</span>}
              </div>
            </fieldset>

            <fieldset className="space-y-2">
              <legend className="label">Automatic</legend>
              <p className="text-xs text-slate-500">Page this group by itself when one of these is dispatched, or an incident of one of these call types gets units.</p>
              <div>
                <p className="mb-1 text-xs text-slate-400">Units</p>
                <div className="flex flex-wrap gap-1">
                  {view.units.map((u) => (
                    <button key={u.id} type="button" aria-pressed={(d.units ?? []).includes(u.id)} className={cx('chip font-mono', (d.units ?? []).includes(u.id) ? 'bg-emerald-600 text-white' : 'bg-slate-800 text-slate-300')} onClick={() => setD({ ...d, units: toggle(d.units, u.id) })}>{u.id}</button>
                  ))}
                </div>
              </div>
              <div>
                <p className="mb-1 text-xs text-slate-400">Stations</p>
                <div className="flex flex-wrap gap-1">
                  {view.stations.map((s) => (
                    <button key={s.id} type="button" aria-pressed={(d.stations ?? []).includes(s.id)} className={cx('chip font-mono', (d.stations ?? []).includes(s.id) ? 'bg-emerald-600 text-white' : 'bg-slate-800 text-slate-300')} onClick={() => setD({ ...d, stations: toggle(d.stations, s.id) })}>{s.id}</button>
                  ))}
                </div>
              </div>
              <div>
                <p className="mb-1 text-xs text-slate-400">Call types</p>
                <div className="flex flex-wrap gap-1">
                  {view.callTypes.map((t) => (
                    <button key={t.code} type="button" aria-pressed={(d.types ?? []).includes(t.code)} className={cx('chip font-mono', (d.types ?? []).includes(t.code) ? 'bg-emerald-600 text-white' : 'bg-slate-800 text-slate-300')} onClick={() => setD({ ...d, types: toggle(d.types, t.code) })}>{t.code}</button>
                  ))}
                </div>
              </div>
            </fieldset>

            <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!d.off} onChange={(e) => setD({ ...d, off: e.target.checked })} /> <Hand size={13} aria-hidden /> Paged by hand only (turn automatic off)</label>

            <Field label="Note" htmlFor="pg-note"><input id="pg-note" className="input" value={d.note ?? ''} onChange={(e) => setD({ ...d, note: e.target.value })} placeholder="Shown with the group" /></Field>
          </div>
        </Modal>
      )}
    </Page>
  )
}
