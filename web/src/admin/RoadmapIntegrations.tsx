import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Plus, Radio, Save, Send, Stethoscope, Trash2 } from 'lucide-react'
import { api, errText } from '../api'
import { Field } from '../ui'
import type { Gateway, GatewayKind, MemberBrief } from '../types'

type PagingConfig = { gateways: Gateway[]; publicUrl: string; kinds: GatewayKind[] }

// A row the administrator is editing: every Gateway text field as a string so the form is simple.
type GwRow = {
  id: string
  name: string
  kind: string
  url: string
  token: string
  account: string
  from: string
  smtpHost: string
  smtpPort: string
  smtpUser: string
  smtpPassword: string
  smtpStartTls: boolean
  to: string
  saved: boolean
}

const toRow = (g: Gateway): GwRow => ({
  id: g.id,
  name: g.name,
  kind: g.kind,
  url: g.url ?? '',
  token: g.token ?? '',
  account: g.account ?? '',
  from: g.from ?? '',
  smtpHost: g.smtpHost ?? '',
  smtpPort: g.smtpPort != null ? String(g.smtpPort) : '',
  smtpUser: g.smtpUser ?? '',
  smtpPassword: g.smtpPassword ?? '',
  smtpStartTls: !!g.smtpStartTls,
  to: g.to ?? '',
  saved: true,
})

const blankRow = (kind: string): GwRow => ({ id: '', name: '', kind, url: '', token: '', account: '', from: '', smtpHost: '', smtpPort: '', smtpUser: '', smtpPassword: '', smtpStartTls: false, to: '', saved: false })

// A set secret comes back as '••••'; leaving it means "keep". Blank it out in the form so the person
// starts from the placeholder, and only send a field the person actually typed into.
const isMasked = (v: string) => v === '••••'

function rowToGateway(r: GwRow): Gateway {
  const g: Gateway = { id: r.id.trim(), name: r.name.trim(), kind: r.kind }
  if (r.url.trim()) g.url = r.url.trim()
  if (r.account.trim()) g.account = r.account.trim()
  if (r.from.trim()) g.from = r.from.trim()
  if (r.smtpHost.trim()) g.smtpHost = r.smtpHost.trim()
  if (r.smtpPort.trim()) g.smtpPort = Number(r.smtpPort)
  if (r.smtpUser.trim()) g.smtpUser = r.smtpUser.trim()
  if (r.to.trim()) g.to = r.to.trim()
  g.smtpStartTls = r.smtpStartTls
  // Secrets: empty keeps the stored value (don't send), a typed value replaces it, '-' clears it.
  if (r.token && !isMasked(r.token)) g.token = r.token
  if (r.smtpPassword && !isMasked(r.smtpPassword)) g.smtpPassword = r.smtpPassword
  return g
}

// Each Gateway field name to its label and whether it is a secret, so a kind shows only what it uses.
const FIELD_LABEL: Record<string, string> = {
  url: 'Address',
  token: 'Token',
  account: 'Account',
  from: 'From',
  smtpHost: 'Mail server',
  smtpPort: 'Port',
  smtpUser: 'Mail user',
  smtpPassword: 'Mail password',
  smtpStartTls: 'Use STARTTLS',
  to: 'Send to',
}

/** PagingSettings sets up the paging gateways (how a page actually goes out: a webhook, Twilio,
 * ntfy, or email), the public address each person's answer link opens, and a test page. */
export function PagingSettings() {
  const [c, setC] = useState<PagingConfig | null>(null)
  const [rows, setRows] = useState<GwRow[]>([])
  const [publicUrl, setPublicUrl] = useState('')
  const [members, setMembers] = useState<MemberBrief[]>([])
  const [testGw, setTestGw] = useState('')
  const [testMember, setTestMember] = useState('')
  const [testResults, setTestResults] = useState<{ ok: boolean; to: string; detail: string }[] | null>(null)
  const load = async () => {
    const r = await api.get<PagingConfig>('/admin/paging')
    setC(r)
    setRows(r.gateways.map(toRow))
    setPublicUrl(r.publicUrl)
    if (r.gateways[0]) setTestGw((g) => g || r.gateways[0].id)
  }
  useEffect(() => {
    load().catch((e) => toast.error(errText(e)))
    api.get<{ members: MemberBrief[] }>('/roster').then((r) => setMembers(r.members ?? [])).catch(() => {})
  }, [])
  if (!c) return null
  const kinds = c.kinds
  const kindOf = (name: string) => kinds.find((k) => k.kind === name || k.name === name)
  const setRow = (i: number, p: Partial<GwRow>) => setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...p } : r)))
  const addRow = () => setRows((rs) => [...rs, blankRow(kinds[0]?.kind ?? '')])
  const removeRow = (i: number) => setRows((rs) => rs.filter((_, j) => j !== i))
  const save = async () => {
    try {
      const gateways = rows.map(rowToGateway)
      await api.put('/admin/paging', { gateways, publicUrl: publicUrl.trim() })
      toast.success('Paging saved')
      await load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const sendTest = async () => {
    if (!testGw || !testMember) return
    try {
      const r = await api.post<{ results: { ok: boolean; to: string; detail: string }[] }>('/admin/paging/test', { gateway: testGw, member: testMember })
      setTestResults(r.results ?? [])
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const activeMembers = members.filter((m) => !m.inactive)
  return (
    <section className="panel mt-4 space-y-3 rounded-lg p-4" aria-labelledby="paging-h">
      <h2 id="paging-h" className="flex items-center gap-2 font-semibold text-slate-100"><Radio size={15} className="text-sky-300" aria-hidden /> Paging gateways</h2>
      <p className="text-xs text-slate-400">How a page leaves bitdispatcher: a gateway is one way out, used by the paging groups. Each person's page carries a link to answer — responding, straight to the scene, or not available — and that link opens at the address below.</p>
      <Field label="Address responders open" htmlFor="paging-public" hint="The link in each page opens here. Use https, e.g. https://cad.example.gov">
        <input id="paging-public" className="input font-mono" value={publicUrl} onChange={(e) => setPublicUrl(e.target.value)} placeholder="https://cad.example.gov" />
      </Field>
      <div className="space-y-3">
        {rows.length === 0 && <p className="text-xs text-slate-500">No gateways yet. Add one so paging groups have a way out.</p>}
        {rows.map((r, i) => {
          const k = kindOf(r.kind)
          const uses = new Set(k?.fields ?? [])
          return (
            <div key={i} className="space-y-3 rounded-md border border-slate-800 p-3">
              <div className="grid gap-3 sm:grid-cols-3">
                <Field label="ID" htmlFor={`gw-id-${i}`} hint={r.saved ? 'Fixed once saved' : 'A short name, e.g. twilio'}>
                  <input id={`gw-id-${i}`} className="input font-mono" disabled={r.saved} value={r.id} onChange={(e) => setRow(i, { id: e.target.value })} placeholder="twilio" />
                </Field>
                <Field label="Name" htmlFor={`gw-name-${i}`}>
                  <input id={`gw-name-${i}`} className="input" value={r.name} onChange={(e) => setRow(i, { name: e.target.value })} placeholder="Text messages" />
                </Field>
                <Field label="Kind" htmlFor={`gw-kind-${i}`}>
                  <select id={`gw-kind-${i}`} className="input" value={r.kind} onChange={(e) => setRow(i, { kind: e.target.value })}>
                    {kinds.map((kk) => <option key={kk.kind} value={kk.kind}>{kk.name}</option>)}
                  </select>
                </Field>
              </div>
              {k?.note && <p className="text-xs text-slate-500">{k.note}</p>}
              <div className="grid gap-3 sm:grid-cols-2">
                {uses.has('url') && <Field label={FIELD_LABEL.url} htmlFor={`gw-url-${i}`}><input id={`gw-url-${i}`} className="input font-mono" value={r.url} onChange={(e) => setRow(i, { url: e.target.value })} placeholder="https://…" /></Field>}
                {uses.has('token') && <Field label={FIELD_LABEL.token} htmlFor={`gw-token-${i}`} hint={isMasked(r.token) ? 'set — leave as is to keep, - to clear' : 'The gateway secret'}><input id={`gw-token-${i}`} type="password" autoComplete="new-password" className="input font-mono" value={r.token} onChange={(e) => setRow(i, { token: e.target.value })} /></Field>}
                {uses.has('account') && <Field label={FIELD_LABEL.account} htmlFor={`gw-account-${i}`}><input id={`gw-account-${i}`} className="input font-mono" value={r.account} onChange={(e) => setRow(i, { account: e.target.value })} /></Field>}
                {uses.has('from') && <Field label={FIELD_LABEL.from} htmlFor={`gw-from-${i}`}><input id={`gw-from-${i}`} className="input font-mono" value={r.from} onChange={(e) => setRow(i, { from: e.target.value })} placeholder="+15405550100" /></Field>}
                {uses.has('smtpHost') && <Field label={FIELD_LABEL.smtpHost} htmlFor={`gw-host-${i}`}><input id={`gw-host-${i}`} className="input font-mono" value={r.smtpHost} onChange={(e) => setRow(i, { smtpHost: e.target.value })} placeholder="mail.example.gov" /></Field>}
                {uses.has('smtpPort') && <Field label={FIELD_LABEL.smtpPort} htmlFor={`gw-port-${i}`}><input id={`gw-port-${i}`} className="input font-mono" value={r.smtpPort} onChange={(e) => setRow(i, { smtpPort: e.target.value.replace(/[^0-9]/g, '') })} placeholder="587" /></Field>}
                {uses.has('smtpUser') && <Field label={FIELD_LABEL.smtpUser} htmlFor={`gw-user-${i}`}><input id={`gw-user-${i}`} className="input font-mono" value={r.smtpUser} onChange={(e) => setRow(i, { smtpUser: e.target.value })} autoComplete="off" /></Field>}
                {uses.has('smtpPassword') && <Field label={FIELD_LABEL.smtpPassword} htmlFor={`gw-pw-${i}`} hint={isMasked(r.smtpPassword) ? 'set — leave as is to keep, - to clear' : undefined}><input id={`gw-pw-${i}`} type="password" autoComplete="new-password" className="input font-mono" value={r.smtpPassword} onChange={(e) => setRow(i, { smtpPassword: e.target.value })} /></Field>}
                {uses.has('to') && <Field label={FIELD_LABEL.to} htmlFor={`gw-to-${i}`}><input id={`gw-to-${i}`} className="input font-mono" value={r.to} onChange={(e) => setRow(i, { to: e.target.value })} placeholder="pager@example.gov" /></Field>}
              </div>
              {uses.has('smtpStartTls') && (
                <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={r.smtpStartTls} onChange={(e) => setRow(i, { smtpStartTls: e.target.checked })} /> {FIELD_LABEL.smtpStartTls}</label>
              )}
              <div className="flex justify-end">
                <button className="btn text-rose-300" onClick={() => removeRow(i)}><Trash2 size={13} aria-hidden /> Remove</button>
              </div>
            </div>
          )
        })}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <button className="btn" onClick={addRow}><Plus size={13} aria-hidden /> Add a gateway</button>
        <button className="btn btn-go ml-auto" onClick={save}><Save size={14} aria-hidden /> Save paging</button>
      </div>
      <div className="space-y-2 rounded-md border border-slate-800 p-3">
        <h3 className="text-sm font-semibold text-slate-200">Send a test page</h3>
        <div className="flex flex-wrap items-end gap-2">
          <Field label="Gateway" htmlFor="paging-test-gw">
            <select id="paging-test-gw" className="input" value={testGw} onChange={(e) => setTestGw(e.target.value)}>
              <option value="">Choose a gateway</option>
              {rows.filter((r) => r.saved).map((r) => <option key={r.id} value={r.id}>{r.name || r.id}</option>)}
            </select>
          </Field>
          <Field label="Person" htmlFor="paging-test-member">
            <select id="paging-test-member" className="input" value={testMember} onChange={(e) => setTestMember(e.target.value)}>
              <option value="">Choose a person</option>
              {activeMembers.map((m) => <option key={m.id} value={m.id}>{m.name}{m.station ? ` · ${m.station}` : ''}</option>)}
            </select>
          </Field>
          <button className="btn" disabled={!testGw || !testMember} onClick={sendTest}><Send size={13} aria-hidden /> Send a test page</button>
        </div>
        {testResults && (
          <ul className="space-y-0.5 text-xs" aria-label="Test results">
            {testResults.map((t, i) => <li key={i} className={t.ok ? 'text-emerald-300' : 'text-rose-300'}>{t.ok ? 'ok' : 'failed'} · {t.to}{t.detail ? ` · ${t.detail}` : ''}</li>)}
            {testResults.length === 0 && <li className="text-slate-500">No result.</li>}
          </ul>
        )}
        <p className="text-xs text-slate-500">Tested against stand-ins (webhook, Twilio, ntfy, email). IamResponding and Active911 take dispatch by email in their documented formats; not certified by either.</p>
      </div>
    </section>
  )
}

type NEMSISConfig = { on: boolean; url: string; tokenSet: boolean; agency: string; agencyStateId: string; state: string; auto: boolean; reasons: { code: string; name: string }[]; version: string }

/** NEMSISSettings sends each EMS unit's dispatch, times and scene to the patient-care report system as
 * a NEMSIS record; the crew completes the rest in their ePCR. */
export function NEMSISSettings() {
  const [c, setC] = useState<NEMSISConfig | null>(null)
  const [token, setToken] = useState('')
  useEffect(() => {
    api.get<NEMSISConfig>('/admin/nemsis').then(setC).catch((e) => toast.error(errText(e)))
  }, [])
  if (!c) return null
  const set = (p: Partial<NEMSISConfig>) => setC({ ...c, ...p })
  const save = async () => {
    try {
      const body = { on: c.on, url: c.url.trim(), token, agency: c.agency.trim(), agencyStateId: c.agencyStateId.trim(), state: c.state.trim().toUpperCase(), auto: c.auto }
      setC(await api.put<NEMSISConfig>('/admin/nemsis', body))
      setToken('')
      toast.success('NEMSIS saved')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section className="panel mt-4 space-y-3 rounded-lg p-4" aria-labelledby="nemsis-h">
      <h2 id="nemsis-h" className="flex items-center gap-2 font-semibold text-slate-100"><Stethoscope size={15} className="text-rose-300" aria-hidden /> NEMSIS: EMS responses to the patient-care report</h2>
      <p className="text-xs text-slate-400">Send each EMS unit's response to the patient-care report (ePCR) system as a NEMSIS record. The record is NEMSIS version {c.version} (dispatch, times and scene only; the crew completes the rest in their ePCR). Tested against a stand-in, not a vendor's system.</p>
      <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={c.on} onChange={(e) => set({ on: e.target.checked })} /> Send EMS responses to the patient-care report system</label>
      <div className="grid gap-3 md:grid-cols-2">
        <Field label="Import endpoint" htmlFor="nemsis-url" hint="The https import endpoint of the ePCR system">
          <input id="nemsis-url" className="input font-mono" value={c.url} onChange={(e) => set({ url: e.target.value })} placeholder="https://epcr.example.gov/import" />
        </Field>
        <Field label="Token" htmlFor="nemsis-token" hint={c.tokenSet ? 'set — leave empty to keep, - to clear' : 'not set'}>
          <input id="nemsis-token" type="password" autoComplete="new-password" className="input font-mono" value={token} onChange={(e) => setToken(e.target.value)} placeholder={c.tokenSet ? '••••••••' : ''} />
        </Field>
        <Field label="Agency number (dAgency.02)" htmlFor="nemsis-agency">
          <input id="nemsis-agency" className="input font-mono" value={c.agency} onChange={(e) => set({ agency: e.target.value })} />
        </Field>
        <Field label="Agency state ID (dAgency.01)" htmlFor="nemsis-agency-state">
          <input id="nemsis-agency-state" className="input font-mono" value={c.agencyStateId} onChange={(e) => set({ agencyStateId: e.target.value })} />
        </Field>
        <Field label="State" htmlFor="nemsis-state" hint="Two letters">
          <input id="nemsis-state" className="input font-mono uppercase" maxLength={2} value={c.state} onChange={(e) => set({ state: e.target.value })} placeholder="VA" />
        </Field>
      </div>
      <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={c.auto} onChange={(e) => set({ auto: e.target.checked })} /> Send each EMS unit's response when it clears</label>
      <div className="flex justify-end">
        <button className="btn btn-go" onClick={save}><Save size={14} aria-hidden /> Save NEMSIS</button>
      </div>
    </section>
  )
}
