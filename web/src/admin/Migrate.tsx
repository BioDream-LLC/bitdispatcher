import { useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { CheckCircle2, CircleAlert, FileUp, Import } from 'lucide-react'
import { api, errText } from '../api'
import { cx } from '../util'
import { Page } from './Admin'

type File = { name: string; headers: string[]; sample: Record<string, string>[]; rows: number; kind: string; guess: Record<string, Record<string, string>>; kinds: Record<string, string[]>; need: Record<string, string[]> }
type Result = { rows: number; good: number; imported: number; located: number; problems: { row: number; message: string }[]; notes?: string[]; more?: number }

const KINDS: Record<string, [string, string]> = {
  premises: ['Premises and hazards', 'addresses (or positions) with hazards, notes and contacts'],
  history: ['Incident history', 'earlier incidents: number, date, place, type, disposition: they count as calls before at an address'],
  runcards: ['Call types and run cards', 'codes, names, priorities and what to send: "2 engine; ladder:1; als"'],
}
const LABEL: Record<string, string> = {
  address: 'Address', lat: 'Latitude', lon: 'Longitude', name: 'Name', hazards: 'Hazards (; between)', notes: 'Notes', contacts: 'Contacts (; between)', expires: 'Expires', id: 'Their id',
  number: 'Incident number', at: 'Date and time', type: 'Call type', disposition: 'Disposition', note: 'Narrative', code: 'Code', priority: 'Priority', discipline: 'Discipline', runcard: 'Run card',
}

/** Bringing an earlier CAD system's data over: upload its export, check the columns, import. */
export default function Migrate() {
  const [f, setF] = useState<File | null>(null)
  const [kind, setKind] = useState('premises')
  const [fields, setFields] = useState<Record<string, string>>({})
  const [res, setRes] = useState<Result | null>(null)
  const [busy, setBusy] = useState('')
  const check = async (file: File, k: string, fl: Record<string, string>, really = false) => {
    setBusy(really ? 'Importing…' : 'Checking every row…')
    try {
      const r = await api.post<Result>('/admin/migrate/import', { file: file.name, kind: k, fields: fl, check: !really })
      setRes(r)
      if (really) toast.success(`Imported ${r.imported.toLocaleString()}`)
    } catch (e) {
      setRes(null)
      toast.error(errText(e))
    } finally {
      setBusy('')
    }
  }
  const upload = async (file: globalThis.File) => {
    setBusy('Reading…')
    setRes(null)
    try {
      const r = await fetch(`./api/admin/migrate/upload?name=${encodeURIComponent(file.name)}`, { method: 'POST', body: file, headers: { 'X-BitDispatcher': '1' }, credentials: 'same-origin' })
      const body = await r.json()
      if (!r.ok) throw new Error(body.message ?? r.statusText)
      setF(body)
      setKind(body.kind)
      setFields(body.guess[body.kind])
      await check(body, body.kind, body.guess[body.kind]) // checked at once: one click less
    } catch (e) {
      toast.error(errText(e))
      setBusy('')
    }
  }
  const missing = f ? f.need[kind].filter((x) => !fields[x]) : []
  return (
    <Page title="Migration" intro="Bring an earlier CAD system's data over from the spreadsheets it exports (CSV or tab-separated): its premises and hazards, its run cards and its incident history. The columns are matched for you and every row is checked before anything is imported; importing the same file again replaces what it brought rather than doubling it. Every import is a ledger event.">
      <div className="grid gap-4 lg:grid-cols-[1fr_22rem]">
        <section className="panel space-y-3 rounded-lg p-3">
          <label className="flex cursor-pointer items-center justify-center gap-2 rounded-lg border-2 border-dashed border-slate-700 p-6 text-sm text-slate-300 hover:border-sky-500"
            onDragOver={(e) => e.preventDefault()} onDrop={(e) => { e.preventDefault(); const x = e.dataTransfer.files[0]; if (x) upload(x) }}>
            <FileUp size={18} aria-hidden /> {f ? `${f.name} · ${f.rows.toLocaleString()} rows: drop another, or click` : 'Drop the export here, or click to choose it'}
            <input type="file" accept=".csv,.tsv,.txt,text/csv" className="hidden" onChange={(e) => e.target.files?.[0] && upload(e.target.files[0])} />
          </label>
          {f && (
            <>
              <div className="flex flex-wrap gap-1" role="radiogroup" aria-label="What the file holds">
                {Object.entries(KINDS).map(([k, [n]]) => (
                  <button key={k} role="radio" aria-checked={kind === k} className={cx('btn text-xs', kind === k && 'border-sky-500 bg-sky-500/20 text-sky-100')}
                    onClick={() => { setKind(k); setFields(f.guess[k]); check(f, k, f.guess[k]) }}>{n}</button>
                ))}
              </div>
              <p className="text-xs text-slate-500">{KINDS[kind][1]}</p>
              <table className="w-full text-xs">
                <thead><tr className="text-left text-[10px] uppercase tracking-wider text-slate-500"><th className="py-1">Ours</th><th>Their column</th><th>First row</th></tr></thead>
                <tbody>
                  {f.kinds[kind].map((x) => (
                    <tr key={x} className="border-t border-slate-800">
                      <td className="py-1 pr-2 text-slate-300">{LABEL[x] ?? x}{f.need[kind].includes(x) && <span className="text-rose-300"> *</span>}</td>
                      <td className="pr-2">
                        <select className="input h-7 py-0 font-mono text-xs" value={fields[x] ?? ''} onChange={(e) => setFields({ ...fields, [x]: e.target.value })} aria-label={LABEL[x] ?? x}>
                          <option value="">—</option>
                          {f.headers.map((h) => <option key={h}>{h}</option>)}
                        </select>
                      </td>
                      <td className="max-w-64 truncate font-mono text-[11px] text-slate-400">{fields[x] ? f.sample[0]?.[fields[x]] : ''}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <div className="flex items-center gap-2">
                <button className="btn" disabled={!!busy || missing.length > 0} onClick={() => check(f, kind, fields)}>Check again</button>
                <button className="btn btn-go" disabled={!!busy || !res || res.good === 0 || missing.length > 0} onClick={() => check(f, kind, fields, true)}>
                  <Import size={13} aria-hidden /> Import {res ? res.good.toLocaleString() : ''} {res && res.good !== res.rows ? `of ${res.rows.toLocaleString()}` : ''}
                </button>
                {busy && <span className="text-xs text-slate-400">{busy}</span>}
                {missing.length > 0 && <span className="text-xs text-amber-300">Match {missing.map((x) => LABEL[x] ?? x).join(', ')}</span>}
              </div>
            </>
          )}
        </section>
        <aside className="space-y-3">
          {res && (
            <section className="panel space-y-1 rounded-lg p-3 text-xs">
              <div className={cx('flex items-center gap-1 text-sm', res.problems.length ? 'text-amber-200' : 'text-emerald-300')}>
                {res.problems.length ? <CircleAlert size={14} aria-hidden /> : <CheckCircle2 size={14} aria-hidden />}
                {res.imported ? `${res.imported.toLocaleString()} imported` : `${res.good.toLocaleString()} of ${res.rows.toLocaleString()} rows ready`}
              </div>
              {res.located > 0 && <p className="text-slate-400">{res.located.toLocaleString()} placed by finding their address on the map.</p>}
              {res.notes?.map((n) => <p key={n} className="text-amber-200">{n}</p>)}
              {res.problems.length > 0 && (
                <ul className="max-h-80 space-y-0.5 overflow-y-auto text-slate-300">
                  {res.problems.map((p, i) => <li key={i}><span className="font-mono text-slate-500">row {p.row}</span> {p.message}</li>)}
                  {(res.more ?? 0) > 0 && <li className="text-slate-500">and {res.more} more</li>}
                </ul>
              )}
            </section>
          )}
          <section className="panel rounded-lg p-3 text-xs text-slate-400">
            Address points with latitude and longitude go to <Link to="/admin/gis" className="text-sky-300 underline">County GIS</Link>, which takes the same CSV files. Imported premises are edited in <Link to="/admin/premises" className="text-sky-300 underline">Premises</Link>.
          </section>
        </aside>
      </div>
    </Page>
  )
}
