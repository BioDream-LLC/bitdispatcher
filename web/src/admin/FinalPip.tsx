// PSC PIP connection: the outbound connection to a PSC PIP system. It sends operational metadata and
// source references only, by event class, to the PIP modules (CEDE, ARC, ROC, RESET). This page turns it
// on, sets the endpoint and token, chooses the classes, and previews exactly what a period would send.

import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { PlugZap, Save, Send } from 'lucide-react'
import { api, errText, ApiError } from '../api'
import { Field } from '../ui'
import { Page } from './Admin'
import type { ShiftPeriodInfo } from './FinalShared'

// What GET /api/admin/pip returns (internal/api/final.go pipRoutes).
type PipConfig = {
  on: boolean
  url: string
  tokenSet: boolean
  classes: string[]
  identified: boolean
  aggregate: boolean
  saltSet: boolean
  allClasses: string[]
}

// Each event class, the module it reaches and a sentence on what it carries.
const CLASS_INFO: Record<string, { label: string; module: string; note: string }> = {
  'period.summary': { label: 'Period summary', module: 'CEDE', note: 'Objective counts for one operational period: demand and exposure context.' },
  'location.exception': { label: 'Location exception', module: 'CEDE', note: 'A forced or estimated location, as operational context. Never a performance judgement.' },
  'critical.incident': { label: 'Critical incident', module: 'CEDE, and RESET when qualifying', note: 'A critical incident as demand and exposure; it reaches RESET as well when it is marked qualifying.' },
  'after.action': { label: 'After action', module: 'ARC', note: 'Sent only when a supervisor opens an after-action or readiness workflow.' },
  'recovery.effect': { label: 'Recovery effect', module: 'ROC', note: 'Recovery context: breaks, overtime, consecutive duty.' },
  'support.offer': { label: 'Support offer', module: 'RESET', note: 'That a support offer was opened or linked. An operational action only.' },
  'correction': { label: 'Correction', module: 'the module of the class it corrects', note: 'A versioned correction of an earlier event, routed like the class it corrects.' },
}

const BOUNDARY = 'bitdispatcher sends operational metadata and source references only. No caller, patient or victim details, no narratives, no wellness or clinical content, and no scores. PSC PIP\'s own logic stays in PSC PIP. Not tested against a real PSC PIP system.'

/** PipAdmin edits the PSC PIP connection and previews a period's payload. */
export function PipAdmin() {
  const [c, setC] = useState<PipConfig | null>(null)
  const [token, setToken] = useState('')
  const [salt, setSalt] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api.get<PipConfig>('/admin/pip').then(setC).catch((e) => toast.error(errText(e)))
  }, [])

  if (!c) return <Page title="PSC PIP connection" intro="Loading."><div /></Page>

  const set = (p: Partial<PipConfig>) => setC({ ...c, ...p })
  const toggleClass = (cls: string) => {
    const has = c.classes.includes(cls)
    set({ classes: has ? c.classes.filter((x) => x !== cls) : [...c.classes, cls] })
  }

  const save = async () => {
    setBusy(true)
    try {
      await api.put('/admin/pip', { on: c.on, url: c.url.trim(), token, classes: c.classes, identified: c.identified, aggregate: c.aggregate, salt })
      const fresh = await api.get<PipConfig>('/admin/pip')
      setC(fresh)
      setToken('')
      setSalt('')
      toast.success('PSC PIP connection saved')
    } catch (e) {
      if (e instanceof ApiError && e.status === 422) toast.error(e.message)
      else toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <Page
      title="PSC PIP connection"
      intro="The outbound connection to a PSC PIP system. It sends operational metadata and source references by event class to the PIP modules."
      actions={<button className="btn btn-go" disabled={busy} onClick={save}><Save size={14} aria-hidden /> Save</button>}
    >
      <div className="space-y-4">
        <div className="rounded-md border border-slate-700 bg-slate-800/50 p-3 text-xs text-slate-300">{BOUNDARY}</div>

        <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="pip-conn-h">
          <h2 id="pip-conn-h" className="flex items-center gap-2 font-semibold text-slate-100"><PlugZap size={15} className="text-sky-300" aria-hidden /> Connection</h2>
          <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={c.on} onChange={(e) => set({ on: e.target.checked })} /> Connection on</label>
          <Field label="URL" htmlFor="pip-url" hint="An https endpoint for the PSC PIP system.">
            <input id="pip-url" type="url" className="input font-mono" value={c.url} onChange={(e) => set({ url: e.target.value })} placeholder="https://pip.example.org/ingest" />
          </Field>
          <Field label={`Bearer token ${c.tokenSet ? '(set)' : ''}`} htmlFor="pip-token" hint="Empty keeps the stored token.">
            <input id="pip-token" type="password" className="input font-mono" value={token} onChange={(e) => setToken(e.target.value)} placeholder={c.tokenSet ? '••••••••' : ''} autoComplete="new-password" />
          </Field>
        </section>

        <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="pip-classes-h">
          <h2 id="pip-classes-h" className="font-semibold text-slate-100">Event classes</h2>
          <p className="text-xs text-slate-400">Choose which classes may cross. A class not ticked is dropped before anything is built or sent.</p>
          <div className="space-y-2">
            {c.allClasses.map((cls) => {
              const info = CLASS_INFO[cls]
              return (
                <label key={cls} className="flex items-start gap-2 rounded-md border border-slate-800 p-2 text-sm text-slate-300">
                  <input type="checkbox" className="mt-1" aria-label={info?.label ?? cls} checked={c.classes.includes(cls)} onChange={() => toggleClass(cls)} />
                  <span>
                    <span className="font-semibold text-slate-100">{info?.label ?? cls}</span> <span className="font-mono text-xs text-slate-500">{cls}</span>
                    <span className="ml-2 chip bg-slate-800 text-slate-300">{info?.module ?? ''}</span>
                    <span className="block text-xs text-slate-500">{info?.note ?? ''}</span>
                  </span>
                </label>
              )
            })}
          </div>
        </section>

        <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="pip-people-h">
          <h2 id="pip-people-h" className="font-semibold text-slate-100">People and aggregation</h2>
          <div>
            <span className="label">How people are named</span>
            <div className="flex flex-col gap-2">
              <label className="flex items-center gap-2 text-sm text-slate-300"><input type="radio" name="pip-id" checked={!c.identified} onChange={() => set({ identified: false })} /> Pseudonymous (a salted code stands for each person)</label>
              <label className="flex items-center gap-2 text-sm text-slate-300"><input type="radio" name="pip-id" checked={c.identified} onChange={() => set({ identified: true })} /> Identified (the person's username is sent)</label>
            </div>
          </div>
          <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={c.aggregate} onChange={(e) => set({ aggregate: e.target.checked })} /> Aggregate only (counts without any per-person reference)</label>
          <Field label={`Salt ${c.saltSet ? '(set)' : ''}`} htmlFor="pip-salt" hint="For the pseudonymous codes. Empty keeps the stored salt; one is generated when none is set.">
            <input id="pip-salt" type="password" className="input font-mono" value={salt} onChange={(e) => setSalt(e.target.value)} placeholder={c.saltSet ? '••••••••' : 'generated when empty'} autoComplete="new-password" />
          </Field>
        </section>

        <PreviewSection />
      </div>
    </Page>
  )
}

// ---- preview ----

function PreviewSection() {
  const [periods, setPeriods] = useState<ShiftPeriodInfo[]>([])
  const [period, setPeriod] = useState('')
  const [out, setOut] = useState<{ event: unknown; wouldSend: boolean } | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api.get<{ periods: ShiftPeriodInfo[] }>('/shift/periods')
      .then((r) => {
        const ps = r.periods ?? []
        setPeriods(ps)
        if (ps[0]?.id) setPeriod(ps[0].id)
      })
      .catch((e) => toast.error(errText(e)))
  }, [])

  const run = async () => {
    if (!period) return
    setBusy(true)
    try {
      const r = await api.get<{ event: unknown; wouldSend: boolean }>(`/admin/pip/preview?period=${encodeURIComponent(period)}`)
      setOut(r)
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="pip-preview-h">
      <h2 id="pip-preview-h" className="font-semibold text-slate-100">Preview</h2>
      <p className="text-xs text-slate-400">What would cross for an operational period, shown exactly as it would be sent. Nothing is sent from here.</p>
      <div className="flex flex-wrap items-end gap-2">
        <Field label="Period" htmlFor="pip-period">
          <select id="pip-period" className="input h-8 text-sm" value={period} onChange={(e) => setPeriod(e.target.value)}>
            {periods.map((p) => <option key={p.id} value={p.id}>{p.name || p.id}</option>)}
          </select>
        </Field>
        <button className="btn btn-go" disabled={busy || !period} onClick={run}><Send size={13} aria-hidden /> Preview</button>
      </div>
      {out && (
        <div className="space-y-2">
          <p className="text-xs text-slate-400">{out.wouldSend ? 'This would be sent now (the connection is on and this class is allowed).' : 'This would not be sent now (the connection is off, or this class is not allowed).'}</p>
          <pre className="max-h-96 overflow-auto rounded-md border border-slate-800 bg-slate-950 p-3 font-mono text-xs text-slate-200">{JSON.stringify(out.event, null, 2)}</pre>
        </div>
      )}
    </section>
  )
}
