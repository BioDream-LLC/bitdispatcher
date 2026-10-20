import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Plus, Trash2, UserCog } from 'lucide-react'
import { api, errText } from '../api'
import { useStore } from '../store'
import { Field, Modal } from '../ui'
import { ago, useNow } from '../util'
import type { Session } from '../types'
import { Page, Table } from './Admin'

type U = { username: string; name: string; role: string; disabled?: boolean; demo?: boolean; created: string; lastIn?: string; duties?: string[]; mfa?: boolean }
type Draft = { username: string; name: string; role: string; password: string; disabled: boolean; demo: boolean; isNew: boolean; duties: string[]; mfa: boolean }

// The duties a person can hold, in the order the sign-in page offers them; their role follows from them.
const DUTIES: [string, string][] = [
  ['calltaker', 'Call-taker'], ['police', 'Police dispatcher'], ['fire', 'Fire dispatcher'], ['ems', 'EMS dispatcher'], ['dispatcher', 'Dispatcher (all disciplines)'],
  ['supervisor', 'Supervisor'], ['admin', 'Administrator'], ['records', 'Records'], ['field', 'Field crew (mobile terminal)'], ['viewer', 'Viewer'],
]

const ROLES = [
  ['dispatcher', 'Dispatcher: works incidents and units'],
  ['supervisor', 'Supervisor: also reopens incidents and reads the ledger'],
  ['admin', 'Administrator: also changes configuration and users'],
  ['field', 'Field crew: a mobile terminal; signs in with the unit as the position'],
  ['viewer', 'Viewer: sees everything, changes nothing'],
]

export default function Users() {
  const now = useNow()
  const me = useStore((s) => s.me)
  const [items, setItems] = useState<U[]>([])
  const [sessions, setSessions] = useState<Session[]>([])
  const [d, setD] = useState<Draft | null>(null)
  const [shown, setShown] = useState('')
  const load = async () => {
    const r = await api.get<{ items: U[]; sessions: Session[] }>('/admin/users')
    setItems(r.items)
    setSessions(r.sessions)
  }
  useEffect(() => {
    load().catch((e) => toast.error(errText(e)))
  }, [])
  const save = async (generate: boolean) => {
    if (!d) return
    try {
      const name = encodeURIComponent(d.username.trim().toLowerCase())
      const role = roleFor(d.duties) || d.role
      const r = await api.put<{ password?: string }>(`/admin/users/${name}`, { name: d.name, role, password: d.password, disabled: d.disabled, demo: d.demo, generate })
      if (d.username !== me?.username && d.duties.length) await api.put(`/admin/users/${name}/duties`, { duties: d.duties })
      if (r.password) setShown(`${d.username}: ${r.password}`)
      toast.success(`${d.username} saved`)
      setD(null)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Page
      title="Users and positions"
      intro="Who can sign in, and the duties each holds. At sign-in a person chooses the duty they are working; what they may do follows it, and the workstation gives the position. Every action is recorded with the person, the duty and the position."
      actions={<button className="btn btn-go" onClick={() => setD({ username: '', name: '', role: 'dispatcher', password: '', disabled: false, demo: false, isNew: true, duties: ['calltaker', 'dispatcher'], mfa: false })}><Plus size={14} aria-hidden /> Add user</button>}
    >
      {shown && (
        <div role="status" className="mb-3 rounded-lg border border-emerald-800 bg-emerald-950/40 p-3 text-sm text-emerald-200">
          Give this password to the person now; it is not shown again: <span className="font-mono text-amber-300">{shown}</span>
          <button className="btn ml-3" onClick={() => setShown('')}>Done</button>
        </div>
      )}
      <Table head={['Username', 'Name', 'Duties', 'MFA', 'Last sign-in', 'State']}>
        {items.map((u) => (
          <tr key={u.username} className="cursor-pointer hover:bg-slate-800/50" onClick={() => setD({ username: u.username, name: u.name, role: u.role, password: '', disabled: !!u.disabled, demo: !!u.demo, isNew: false, duties: u.duties ?? [], mfa: !!u.mfa })}>
            <td className="px-3 py-1.5 font-mono text-slate-100">{u.username}{u.username === me?.username && <span className="ml-1 text-xs text-slate-500">(you)</span>}{u.demo && <span className="chip ml-2 bg-sky-500/20 px-1.5 text-[10px] font-bold uppercase text-sky-200" title="Offered on the sign-in page: Sign in as the demo user">demo</span>}</td>
            <td className="px-3 text-slate-200">{u.name}</td>
            <td className="px-3 text-xs text-slate-300">{(u.duties ?? [u.role]).map((x) => DUTIES.find(([k]) => k === x)?.[1] ?? x).join(', ')}</td>
            <td className="px-3 text-xs">{u.mfa ? <span className="text-emerald-300">on</span> : <span className="text-slate-500">—</span>}</td>
            <td className="px-3 text-xs text-slate-400">{u.lastIn ? `${ago(u.lastIn, now)} ago` : 'never'}</td>
            <td className="px-3 text-xs">{u.disabled ? <span className="text-rose-300">disabled</span> : <span className="text-emerald-300">active</span>}</td>
          </tr>
        ))}
      </Table>
      <h2 className="mb-2 mt-6 font-semibold text-slate-100">Signed in now</h2>
      <Table head={['Position', 'User', 'Duty', 'From', 'Since', 'Last active']}>
        {sessions.map((s) => (
          <tr key={s.username + s.created}>
            <td className="px-3 py-1.5 font-mono text-slate-100">{s.console || '—'}</td>
            <td className="px-3 text-slate-200">{s.name || s.username}</td>
            <td className="px-3 text-slate-300">{DUTIES.find(([k]) => k === s.duty)?.[1] ?? s.role}{s.locked && <span className="chip ml-1 bg-amber-500/20 text-amber-200">locked</span>}{s.workstation && <span className="ml-1 text-[10px] text-emerald-300">enrolled</span>}</td>
            <td className="px-3 font-mono text-xs text-slate-400">{s.from}</td>
            <td className="px-3 text-xs text-slate-400">{ago(s.created, now)} ago</td>
            <td className="px-3 text-xs text-slate-400">{ago(s.last, now)} ago</td>
          </tr>
        ))}
      </Table>
      {d && (
        <Modal
          open
          onClose={() => setD(null)}
          title={d.isNew ? 'New user' : d.username}
          icon={<UserCog size={16} className="text-fuchsia-300" aria-hidden />}
          footer={
            <>
              {!d.isNew && d.username !== me?.username && (
                <button className="btn mr-auto text-rose-300" onClick={async () => { if (!confirm(`Remove ${d.username}?`)) return; try { await api.del(`/admin/users/${d.username}`); setD(null); load() } catch (e) { toast.error(errText(e)) } }}>
                  <Trash2 size={13} aria-hidden /> Remove
                </button>
              )}
              <button className="btn" onClick={() => setD(null)}>Cancel</button>
              <button className="btn" onClick={() => save(true)}>{d.isNew ? 'Save with a generated password' : 'Generate a new password'}</button>
              <button className="btn btn-go" disabled={d.isNew && !d.password} onClick={() => save(false)}>Save</button>
            </>
          }
        >
          <div className="space-y-3">
            <Field label="Username" htmlFor="un"><input id="un" className="input font-mono" disabled={!d.isNew} value={d.username} onChange={(e) => setD({ ...d, username: e.target.value })} /></Field>
            <Field label="Name" htmlFor="nm"><input id="nm" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} /></Field>
            <fieldset disabled={d.username === me?.username}>
              <legend className="label">Duties (qualifications)</legend>
              <div className="grid grid-cols-2 gap-1">
                {DUTIES.map(([k, l]) => (
                  <label key={k} className="flex items-center gap-1.5 text-sm text-slate-300">
                    <input type="checkbox" checked={d.duties.includes(k)} onChange={(e) => setD({ ...d, duties: e.target.checked ? [...d.duties.filter((x) => (k === 'field' ? false : x !== 'field')), k] : d.duties.filter((x) => x !== k) })} /> {l}
                  </label>
                ))}
              </div>
              <p className="mt-1 text-xs text-slate-500">{d.username === me?.username ? 'Another administrator changes your own duties.' : `Rights: ${ROLES.find(([v]) => v === (roleFor(d.duties) || d.role))?.[1] ?? '—'}. A supervisor keeps supervisory rights when covering a position; an administrator works as one only when signed in as Administrator.`}</p>
            </fieldset>
            {!d.isNew && (
              <div className="flex items-center gap-2 text-sm text-slate-300">
                One-time codes: {d.mfa ? <span className="text-emerald-300">set up</span> : <span className="text-slate-500">not set up</span>}
                {d.mfa && <button type="button" className="btn h-7 text-xs" onClick={async () => { try { await api.post(`/admin/users/${d.username}/reset-mfa`); toast.success('They set up their authenticator again at their next sign-in'); setD({ ...d, mfa: false }); load() } catch (e) { toast.error(errText(e)) } }}>Reset (lost phone)</button>}
              </div>
            )}
            <Field label={d.isNew ? 'Password' : 'New password'} htmlFor="pw" hint="At least 12 characters. Leave empty to keep the current one.">
              <input id="pw" type="password" autoComplete="new-password" className="input" value={d.password} onChange={(e) => setD({ ...d, password: e.target.value })} />
            </Field>
            {d.role !== 'admin' && d.role !== 'field' && (
              <label className="flex items-start gap-2 text-sm text-slate-300">
                <input type="checkbox" className="mt-1" checked={d.demo} onChange={(e) => setD({ ...d, demo: e.target.checked })} />
                <span>Public demonstration account<span className="block text-xs text-slate-500">The sign-in page offers <b>Sign in as the demo user</b>, with no password. For a public demonstration only: anyone who can reach this server can then work as this user. One account at most.</span></span>
              </label>
            )}
            {!d.isNew && d.username !== me?.username && (
              <label className="flex items-center gap-2 text-sm text-slate-300">
                <input type="checkbox" checked={d.disabled} onChange={(e) => setD({ ...d, disabled: e.target.checked })} /> Disabled (signs them out at once)
              </label>
            )}
          </div>
        </Modal>
      )}
    </Page>
  )
}

/** roleFor is the least role holding every duty, as the server works it out. */
function roleFor(duties: string[]): string {
  const rank: Record<string, number> = { viewer: 1, records: 1, field: 1, calltaker: 2, police: 2, fire: 2, ems: 2, dispatcher: 2, supervisor: 3, admin: 4 }
  const role: Record<string, string> = { viewer: 'viewer', records: 'viewer', field: 'field', calltaker: 'dispatcher', police: 'dispatcher', fire: 'dispatcher', ems: 'dispatcher', dispatcher: 'dispatcher', supervisor: 'supervisor', admin: 'admin' }
  let best = ''
  for (const d of duties) if (!best || rank[d] > rank[best] || (best === 'field' && d !== 'field')) best = d
  return best ? role[best] : ''
}
