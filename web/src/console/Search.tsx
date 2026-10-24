// Search across the CAD: incidents, messages and BOLOs (comms), people, vehicles, attachments and units.
// A search never links records and it does not change anything — it only finds and says why each result
// matched. Every search is written down. internal/cad/find.go does the finding.
import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { AlertTriangle, Ban, Car, FileText, Link2Off, MessageSquare, Paperclip, Radio, Search, User } from 'lucide-react'
import { api, errText } from '../api'
import { store } from '../store'
import { Field, Modal } from '../ui'
import { cx } from '../util'
import type { AgencyView, FindQuery, FindResults, Found, View } from '../types'
import { UnitRecall } from './UnitRecall'

// The record types a search can cover, in the order results are shown.
const GROUPS = ['incidents', 'comms', 'people', 'vehicles', 'attachments', 'units'] as const
type Group = (typeof GROUPS)[number]

const GROUP_LABEL: Record<Group, string> = {
  incidents: 'Incidents',
  comms: 'Messages and BOLOs',
  people: 'People',
  vehicles: 'Vehicles',
  attachments: 'Attachments',
  units: 'Units',
}
const GROUP_ICON: Record<Group, typeof Search> = {
  incidents: FileText,
  comms: MessageSquare,
  people: User,
  vehicles: Car,
  attachments: Paperclip,
  units: Radio,
}

// The record types the dispatcher can tick. 'Messages and BOLOs' searches the 'comms' group.
const TYPES: { value: Group; label: string }[] = [
  { value: 'incidents', label: 'Incidents' },
  { value: 'comms', label: 'Messages and BOLOs' },
  { value: 'people', label: 'People' },
  { value: 'vehicles', label: 'Vehicles' },
  { value: 'attachments', label: 'Attachments' },
  { value: 'units', label: 'Units' },
]

const STATUSES = [
  { value: '', label: 'Any status' },
  { value: 'open', label: 'Open' },
  { value: 'closed', label: 'Closed' },
  { value: 'active', label: 'Active' },
  { value: 'expired', label: 'Expired' },
  { value: 'cancelled', label: 'Cancelled' },
]

const strengthLabel: Record<Found['strength'], string> = { exact: 'Exact', partial: 'Partial', similar: 'Similar' }

// A BOLO or message that is no longer current must never read as if it is: expired, cancelled and updated
// statuses are shown in capitals with an icon so they stand out.
type StatusShown = { text: string; caps: boolean }
function shownStatus(status?: string): StatusShown | null {
  if (!status) return null
  const s = status.toLowerCase()
  if (s === 'expired' || s === 'cancelled' || s === 'updated') return { text: s.toUpperCase(), caps: true }
  return { text: s, caps: false }
}

// A date for the ISO from/to the query wants: the start of the 'from' day, the end of the 'to' day.
function dayStart(d: string): string | undefined {
  if (!d) return undefined
  const t = new Date(d + 'T00:00:00')
  return isNaN(t.getTime()) ? undefined : t.toISOString()
}
function dayEnd(d: string): string | undefined {
  if (!d) return undefined
  const t = new Date(d + 'T23:59:59.999')
  return isNaN(t.getTime()) ? undefined : t.toISOString()
}

/** The status bar's search button: opens the search dialog. Other code can open it with the window event
 * 'bd:search' (its detail, if any, pre-fills the box). */
export function SearchButton({ view }: { view: View }) {
  const [open, setOpen] = useState(false)
  const [initial, setInitial] = useState<string | undefined>(undefined)
  useEffect(() => {
    const on = (e: Event) => {
      setInitial((e as CustomEvent<string>).detail || undefined)
      setOpen(true)
    }
    window.addEventListener('bd:search', on)
    return () => window.removeEventListener('bd:search', on)
  }, [])
  return (
    <>
      <button className="btn btn-ghost gap-1 px-1.5" onClick={() => { setInitial(undefined); setOpen(true) }} aria-label="Search" title="Search">
        <Search size={14} aria-hidden />
      </button>
      {open && <SearchDialog view={view} initial={initial} onClose={() => setOpen(false)} />}
    </>
  )
}

/** Search: a box, filters, and results grouped by record type. Each result says why it matched and whether
 * it is a confirmed match or only possible. Opening a result goes to that record; it never links records. */
export function SearchDialog({ view, onClose, initial }: { view: View; onClose: () => void; initial?: string }) {
  const [q, setQ] = useState(initial ?? '')
  const [types, setTypes] = useState<Group[]>([])
  const [agency, setAgency] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [status, setStatus] = useState('')
  const [fuzzy, setFuzzy] = useState(false)
  const [allAgencies, setAllAgencies] = useState(false)
  const [agencies, setAgencies] = useState<{ code: string; name: string }[]>([])
  const [results, setResults] = useState<FindResults | null>(null)
  const [busy, setBusy] = useState(false)
  const [recallUnit, setRecallUnit] = useState<string | null>(null)

  useEffect(() => {
    api.get<AgencyView>('/agencies').then((r) => setAgencies((r.agencies ?? []).map((a) => ({ code: a.code, name: a.name })))).catch(() => {})
  }, [])

  const toggleType = (t: Group) => setTypes((cur) => (cur.includes(t) ? cur.filter((x) => x !== t) : [...cur, t]))

  const run = async () => {
    const body: FindQuery = {
      q: q.trim(),
      types: types.length ? types : undefined,
      agency: agency || undefined,
      from: dayStart(from),
      to: dayEnd(to),
      status: status || undefined,
      allAgencies: allAgencies || undefined,
      fuzzy: fuzzy || undefined,
    }
    if (!body.q) {
      toast.error('Type something to search for.')
      return
    }
    setBusy(true)
    try {
      const r = await api.post<FindResults>('/search', body)
      setResults(r)
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }

  // Where the search looked, in words: the agencies it covered, or every agency.
  const scopeText = useMemo(() => {
    const sc = results?.scope ?? []
    if (!sc.length || allAgencies) return 'every agency'
    return sc.join(', ')
  }, [results, allAgencies])

  // Opening a result: record that it was opened, then go to the record. A search never links records.
  const openResult = async (f: Found) => {
    try {
      await api.post('/search/opened', { q: results?.query.q ?? q, group: f.group, ref: f.ref, field: f.field })
    } catch {
      // The search is still useful if the note did not save; opening the record matters more.
    }
    if (f.group === 'incidents' || f.incident) {
      const number = f.group === 'incidents' ? f.ref : f.incident!
      store.select(number)
      onClose()
      return
    }
    if (f.group === 'comms') {
      window.dispatchEvent(new CustomEvent('bd:open-comm', { detail: f.ref }))
      onClose()
      return
    }
    if (f.group === 'units') {
      setRecallUnit(f.ref)
      return
    }
  }

  return (
    <>
      <Modal open onClose={onClose} xl title="Search" icon={<Search size={16} className="text-sky-300" aria-hidden />}
        footer={<button className="btn" onClick={onClose}>Close</button>}>
        <div className="grid gap-4 lg:grid-cols-[22rem_1fr]">
          {/* The box and the filters. */}
          <form
            className="space-y-3"
            onSubmit={(e) => { e.preventDefault(); run() }}
          >
            <Field label="Search for" htmlFor="searchq" hint="Plate, name, phone, VIN, incident number, words…">
              <input
                id="searchq"
                className="input"
                value={q}
                autoFocus
                onChange={(e) => setQ(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); run() } }}
                placeholder="e.g. ABC123, Smith, 555-0100"
              />
            </Field>

            <fieldset className="rounded border border-slate-800 p-2">
              <legend className="px-1 text-[11px] uppercase tracking-wider text-slate-500">Record types</legend>
              <div className="grid grid-cols-2 gap-1">
                {TYPES.map((t) => (
                  <label key={t.value} className="flex items-center gap-2 text-sm text-slate-300">
                    <input type="checkbox" checked={types.includes(t.value)} onChange={() => toggleType(t.value)} />
                    {t.label}
                  </label>
                ))}
              </div>
              <p className="mt-1 text-[11px] text-slate-500">None ticked searches all types.</p>
            </fieldset>

            <Field label="Agency" htmlFor="searchagency">
              <select id="searchagency" className="input" value={agency} onChange={(e) => setAgency(e.target.value)}>
                <option value="">All agencies</option>
                {agencies.map((a) => <option key={a.code} value={a.code}>{a.name} ({a.code})</option>)}
              </select>
            </Field>

            <div className="grid grid-cols-2 gap-2">
              <Field label="From" htmlFor="searchfrom"><input id="searchfrom" type="date" className="input" value={from} onChange={(e) => setFrom(e.target.value)} /></Field>
              <Field label="To" htmlFor="searchto"><input id="searchto" type="date" className="input" value={to} onChange={(e) => setTo(e.target.value)} /></Field>
            </div>

            <Field label="Status" htmlFor="searchstatus">
              <select id="searchstatus" className="input" value={status} onChange={(e) => setStatus(e.target.value)}>
                {STATUSES.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}
              </select>
            </Field>

            <label className="flex items-center gap-2 text-sm text-slate-300">
              <input type="checkbox" checked={fuzzy} onChange={(e) => setFuzzy(e.target.checked)} /> Similar spellings
            </label>
            <label className="flex items-center gap-2 text-sm text-slate-300">
              <input type="checkbox" checked={allAgencies} onChange={(e) => setAllAgencies(e.target.checked)} /> Beyond my agencies
            </label>

            <button className="btn btn-go w-full" type="submit" disabled={busy}>
              <Search size={14} aria-hidden /> {busy ? 'Searching…' : 'Search'}
            </button>

            <p className="text-[11px] leading-relaxed text-slate-500">
              Results say why they matched. A search never links records to each other. Every search is recorded.
            </p>
          </form>

          {/* The results. */}
          <div aria-label="Search results" aria-live="polite">
            {!results ? (
              <p className="text-sm text-slate-500">Type what you are looking for and press Enter.</p>
            ) : (
              <>
                <p className="mb-2 text-xs text-slate-400">Searched: {scopeText}</p>
                <Results results={results} onOpen={openResult} />
              </>
            )}
          </div>
        </div>
      </Modal>
      {recallUnit && <UnitRecall view={view} unit={recallUnit} onClose={() => setRecallUnit(null)} />}
    </>
  )
}

function Results({ results, onOpen }: { results: FindResults; onOpen: (f: Found) => void }) {
  const total = GROUPS.reduce((n, g) => n + (results.counts[g] ?? (results.groups[g]?.length ?? 0)), 0)
  if (total === 0) return <p className="text-sm text-slate-500">Nothing found. Try fewer filters, or tick “Similar spellings”.</p>
  return (
    <div className="space-y-4">
      {GROUPS.map((g) => {
        const found = results.groups[g] ?? []
        const count = results.counts[g] ?? found.length
        if (found.length === 0) return null
        const Icon = GROUP_ICON[g]
        return (
          <section key={g} aria-label={`${GROUP_LABEL[g]} results`}>
            <h3 className="mb-1 flex items-center gap-1.5 text-[11px] font-bold uppercase tracking-wider text-slate-300">
              <Icon size={13} aria-hidden /> {GROUP_LABEL[g]} ({count})
            </h3>
            <ul className="space-y-1.5">
              {found.map((f, i) => <li key={`${g}-${f.ref}-${i}`}><ResultRow f={f} onOpen={onOpen} /></li>)}
            </ul>
          </section>
        )
      })}
    </div>
  )
}

function ResultRow({ f, onOpen }: { f: Found; onOpen: (f: Found) => void }) {
  const st = shownStatus(f.status)
  return (
    <button
      type="button"
      onClick={() => onOpen(f)}
      className="block w-full rounded border border-slate-800 bg-slate-900/40 p-2 text-left hover:border-sky-700 hover:bg-slate-900"
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium text-slate-100">{f.title}</span>
        {f.agency && <span className="chip bg-slate-800 text-slate-300">{f.agency}</span>}
        {st && (
          <span className={cx('chip', st.caps ? 'bg-amber-500/25 text-amber-100' : 'bg-slate-800 text-slate-300')}>
            {st.caps && <AlertTriangle size={11} className="mr-0.5 inline" aria-hidden />}
            {st.text}
          </span>
        )}
      </div>
      {f.detail && <div className="mt-0.5 text-xs text-slate-400">{f.detail}</div>}
      <div className="mt-1 flex flex-wrap items-center gap-2 text-[11px]">
        <span className="chip bg-sky-500/20 text-sky-200">{strengthLabel[f.strength]}</span>
        <span className="text-slate-400">{f.why}</span>
        {f.confirmed ? (
          <span className="text-emerald-300">Confirmed</span>
        ) : (
          <span className="flex items-center gap-1 text-slate-400"><Link2Off size={11} aria-hidden /> Possible match — not linked</span>
        )}
        {f.limited && <span className="flex items-center gap-1 text-slate-500"><Ban size={11} aria-hidden /> Restricted: details withheld</span>}
      </div>
    </button>
  )
}
