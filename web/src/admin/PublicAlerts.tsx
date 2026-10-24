import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { Copy, ListPlus, Plus, Radio, Save, Send, Server, Trash2, Users } from 'lucide-react'
import { api, errText } from '../api'
import { Field, Modal } from '../ui'
import { cx } from '../util'
import { can, useStore } from '../store'
import type { AlertingSettings, AlertList, CAPReceiver, Subscriber, View } from '../types'
import { Page, Table } from './Admin'

// The agency's time zones offered, so CAP times are written with a US offset.
const ZONES = ['America/New_York', 'America/Chicago', 'America/Denver', 'America/Phoenix', 'America/Los_Angeles', 'America/Anchorage', 'Pacific/Honolulu', 'America/Puerto_Rico']

// Toggle a code in and out of a list on the draft.
const toggle = (list: string[] | undefined, code: string) => {
  const l = list ?? []
  return l.includes(code) ? l.filter((x) => x !== code) : [...l, code]
}

// A set token comes back as '••••'; leaving it means "keep", '-' clears it.
const MASK = '••••'

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success('Copied')
  } catch {
    toast.error('Could not copy')
  }
}

type RxDraft = CAPReceiver & { isNew: boolean }

/** PublicAlerts is how public alerts are written and where they go: the agency's CAP sender and the
 * counties it covers, the public feed, the systems each alert is sent to (CAP receivers), and the lists
 * of people who asked for alerts. Supervisors see the settings read-only; only an administrator saves them. */
export default function PublicAlerts({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const admin = can.admin(me)
  const gateways = view.roadmap?.gateways ?? []
  const gatewayName = (id: string) => gateways.find((g) => g.id === id)?.name ?? id

  const [settings, setSettings] = useState<AlertingSettings | null>(null)
  const [geocodes, setGeocodes] = useState('')
  const [rx, setRx] = useState<RxDraft | null>(null)
  const [lists, setLists] = useState<AlertList[]>([])
  const [list, setList] = useState<AlertList | null>(null)
  const [listNew, setListNew] = useState(false)
  const [importMode, setImportMode] = useState<'add' | 'replace'>('add')

  const loadSettings = async () => {
    const s = await api.get<AlertingSettings>('/admin/alerting')
    setSettings(s)
    setGeocodes((s.geocodes ?? []).join(', '))
  }
  const loadLists = async () => {
    const r = await api.get<{ items: AlertList[] }>('/admin/alertlists')
    setLists(r.items ?? [])
  }
  useEffect(() => {
    loadSettings().catch((e) => toast.error(errText(e)))
    loadLists().catch((e) => toast.error(errText(e)))
  }, [])

  const set = (p: Partial<AlertingSettings>) => setSettings((s) => (s ? { ...s, ...p } : s))

  // The settings payload the PUT takes: the editable fields plus the receivers, with the typed county codes.
  const settingsBody = (s: AlertingSettings, receivers: CAPReceiver[]) => ({
    sender: s.sender.trim(),
    senderName: s.senderName.trim(),
    jurisdiction: s.jurisdiction.trim(),
    geocodes: geocodes.split(',').map((x) => x.trim()).filter(Boolean),
    feed: s.feed,
    twoPerson: s.twoPerson,
    timeZone: (s.timeZone ?? '').trim(),
    web: (s.web ?? '').trim(),
    contact: (s.contact ?? '').trim(),
    receivers,
  })

  const saveSettings = async () => {
    if (!settings) return
    try {
      const updated = await api.put<AlertingSettings>('/admin/alerting', settingsBody(settings, settings.receivers))
      setSettings(updated)
      setGeocodes((updated.geocodes ?? []).join(', '))
      toast.success('Public alert settings saved')
    } catch (e) {
      toast.error(errText(e))
    }
  }

  // Saving a receiver PUTs the whole settings with that receiver added or changed.
  const saveReceiver = async () => {
    if (!settings || !rx) return
    const { isNew, ...r } = rx
    const trimmed: CAPReceiver = { ...r, name: r.name.trim(), url: r.url.trim() }
    const receivers = isNew ? [...settings.receivers, trimmed] : settings.receivers.map((x) => (x.id === trimmed.id ? trimmed : x))
    try {
      const updated = await api.put<AlertingSettings>('/admin/alerting', settingsBody(settings, receivers))
      setSettings(updated)
      setGeocodes((updated.geocodes ?? []).join(', '))
      toast.success(`${trimmed.name || 'Receiver'} saved`)
      setRx(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const removeReceiver = async (id: string) => {
    if (!settings) return
    if (!confirm('Remove this CAP receiver?')) return
    const receivers = settings.receivers.filter((x) => x.id !== id)
    try {
      const updated = await api.put<AlertingSettings>('/admin/alerting', settingsBody(settings, receivers))
      setSettings(updated)
      setGeocodes((updated.geocodes ?? []).join(', '))
      setRx(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const testReceiver = async (id: string) => {
    try {
      const r = await api.post<{ ok: boolean; detail: string }>('/admin/alerting/test', { receiver: id })
      if (r.ok) toast.success(`Test accepted${r.detail ? `: ${r.detail}` : ''}`)
      else toast.error(`Test failed${r.detail ? `: ${r.detail}` : ''}`)
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const blankList = (): AlertList => ({ id: '', name: '', gateways: [], people: [], note: '' })
  const openList = (l?: AlertList) => {
    const base = l ?? blankList()
    setListNew(!l)
    setImportMode('add')
    setList({ ...base, gateways: [...(base.gateways ?? [])], people: (base.people ?? []).map((p) => ({ ...p })) })
  }
  const saveList = async () => {
    if (!list) return
    try {
      const { id } = await api.put<{ id: string }>('/admin/alertlists', { ...list, name: list.name.trim() })
      toast.success(`${list.name.trim() || 'List'} saved`)
      await loadLists()
      // Keep the editor open on a now-saved list so its people can be imported.
      setList({ ...list, id })
      setListNew(false)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const removeList = async (id: string) => {
    if (!confirm('Remove this subscriber list?')) return
    try {
      await api.del(`/admin/alertlists/${encodeURIComponent(id)}`)
      await loadLists()
      setList(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }

  // People editor on the open list.
  const setPerson = (i: number, p: Partial<Subscriber>) => setList((l) => (l ? { ...l, people: l.people.map((x, j) => (j === i ? { ...x, ...p } : x)) } : l))
  const addPerson = () => setList((l) => (l ? { ...l, people: [...l.people, { name: '', phone: '', email: '', topic: '', lang: 'en' }] } : l))
  const removePerson = (i: number) => setList((l) => (l ? { ...l, people: l.people.filter((_, j) => j !== i) } : l))

  // Import a CSV into the saved list, like the state file download does: a raw fetch with the CSRF header.
  const importCsv = async (file: File) => {
    if (!list || !list.id) return
    try {
      const text = await file.text()
      const res = await fetch(`./api/admin/alertlists/${encodeURIComponent(list.id)}/import?mode=${importMode}`, {
        method: 'POST',
        headers: { 'X-BitDispatcher': '1', 'Content-Type': 'text/csv' },
        body: text,
        credentials: 'same-origin',
      })
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).message ?? res.statusText)
      const r = (await res.json()) as { imported: number; total: number }
      toast.success(`Imported ${r.imported} — ${r.total} on the list`)
      await loadLists()
      const fresh = (await api.get<{ items: AlertList[] }>('/admin/alertlists')).items.find((x) => x.id === list.id)
      if (fresh) setList({ ...fresh, gateways: [...fresh.gateways], people: fresh.people.map((p) => ({ ...p })) })
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const xmlFeed = useMemo(() => (settings ? settings.feedUrl.replace(/\.atom$/, '.xml') : ''), [settings])

  if (!settings) return <p className="text-sm text-slate-500">Loading…</p>

  return (
    <Page
      title="Public alerts"
      intro="How public alerts are written and where they go: the agency's CAP sender, the counties it covers, the public feed that sirens, broadcasters' EAS encoders and apps can read, systems each alert is sent to (CAP receivers), and the lists of people who asked for alerts. Alerts are written from the console by supervisors (the megaphone button)."
    >
      {settings.ready.length > 0 && (
        <div className="panel mb-4 rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-200">
          <p className="font-semibold">Before an alert can go:</p>
          <ul className="mt-1 list-disc space-y-0.5 pl-5">
            {settings.ready.map((r, i) => <li key={i}>{r}</li>)}
          </ul>
        </div>
      )}

      {/* 1. Settings */}
      <section className="panel space-y-3 rounded-lg p-4" aria-labelledby="alerting-h">
        <h2 id="alerting-h" className="flex items-center gap-2 font-semibold text-slate-100"><Radio size={15} className="text-sky-300" aria-hidden /> Settings</h2>
        {!admin && <p className="text-xs text-amber-300">You can see these settings. An administrator sets them.</p>}
        <div className="grid gap-3 md:grid-cols-2">
          <Field label="Sender" htmlFor="pa-sender" hint="An address the agency owns, like alerts@county.gov">
            <input id="pa-sender" className="input font-mono" disabled={!admin} value={settings.sender} onChange={(e) => set({ sender: e.target.value })} placeholder="alerts@county.gov" />
          </Field>
          <Field label="Sender name" htmlFor="pa-sender-name">
            <input id="pa-sender-name" className="input" disabled={!admin} value={settings.senderName} onChange={(e) => set({ senderName: e.target.value })} placeholder="Example County Emergency Communications" />
          </Field>
          <Field label="Jurisdiction" htmlFor="pa-jurisdiction" hint="The area named when an alert goes to everyone">
            <input id="pa-jurisdiction" className="input" disabled={!admin} value={settings.jurisdiction} onChange={(e) => set({ jurisdiction: e.target.value })} placeholder="Example County" />
          </Field>
          <Field label="County codes" htmlFor="pa-geocodes" hint="SAME codes: 0, the state FIPS and the county FIPS, e.g. 051770 for Roanoke city">
            <input id="pa-geocodes" className="input font-mono" disabled={!admin} value={geocodes} onChange={(e) => setGeocodes(e.target.value)} placeholder="051770, 051161" />
          </Field>
          <Field label="Time zone" htmlFor="pa-tz" hint="A name like America/New_York, so CAP times carry a US offset">
            <input id="pa-tz" className="input font-mono" disabled={!admin} list="pa-tz-list" value={settings.timeZone ?? ''} onChange={(e) => set({ timeZone: e.target.value })} placeholder="America/New_York" />
            <datalist id="pa-tz-list">
              {ZONES.map((z) => <option key={z} value={z} />)}
            </datalist>
          </Field>
          <Field label="Web page" htmlFor="pa-web" hint="Optional: a page the public can read for more">
            <input id="pa-web" className="input font-mono" disabled={!admin} value={settings.web ?? ''} onChange={(e) => set({ web: e.target.value })} placeholder="https://county.gov/alerts" />
          </Field>
          <Field label="Contact" htmlFor="pa-contact" hint="Optional: who to reach about an alert">
            <input id="pa-contact" className="input" disabled={!admin} value={settings.contact ?? ''} onChange={(e) => set({ contact: e.target.value })} placeholder="Dispatch, (540) 555-0101" />
          </Field>
        </div>

        <label className="flex items-center gap-2 text-sm text-slate-300">
          <input type="checkbox" disabled={!admin} checked={settings.twoPerson} onChange={(e) => set({ twoPerson: e.target.checked })} /> A second supervisor approves each alert before it goes (recommended)
        </label>
        <label className="flex items-center gap-2 text-sm text-slate-300">
          <input type="checkbox" disabled={!admin} checked={settings.feed} onChange={(e) => set({ feed: e.target.checked })} /> Publish the public alert feed
        </label>

        {settings.feed && (
          <div className="space-y-2 rounded-md border border-slate-800 p-3">
            <Field label="Public feed (Atom)" htmlFor="pa-feed-atom" hint="Links to each CAP message; for apps and readers">
              <div className="flex items-center gap-2">
                <input id="pa-feed-atom" className="input font-mono" readOnly value={settings.feedUrl} />
                <button type="button" className="btn" aria-label="Copy the Atom feed address" onClick={() => copy(settings.feedUrl)}><Copy size={13} aria-hidden /> Copy</button>
              </div>
            </Field>
            <Field label="IPAWS-style feed" htmlFor="pa-feed-xml" hint="IPAWS-style feed with the CAP messages inside, for EAS encoders">
              <div className="flex items-center gap-2">
                <input id="pa-feed-xml" className="input font-mono" readOnly value={xmlFeed} />
                <button type="button" className="btn" aria-label="Copy the IPAWS-style feed address" onClick={() => copy(xmlFeed)}><Copy size={13} aria-hidden /> Copy</button>
              </div>
            </Field>
          </div>
        )}

        {admin && (
          <div className="flex justify-end">
            <button className="btn btn-go" onClick={saveSettings}><Save size={14} aria-hidden /> Save settings</button>
          </div>
        )}
      </section>

      {/* 2. CAP receivers */}
      <section className="panel mt-4 space-y-3 rounded-lg p-4" aria-labelledby="rx-h">
        <div className="flex items-center gap-2">
          <h2 id="rx-h" className="flex items-center gap-2 font-semibold text-slate-100"><Server size={15} className="text-sky-300" aria-hidden /> CAP receivers</h2>
          {admin && <button className="btn ml-auto" onClick={() => setRx({ id: '', name: '', url: '', token: '', off: false, isNew: true })}><Plus size={13} aria-hidden /> Add a receiver</button>}
        </div>
        <p className="text-xs text-slate-400">Systems each alert's CAP message is sent to, like a state aggregator or an EAS encoder.</p>
        <Table head={['Name', 'Address', 'On', '']} empty={settings.receivers.length === 0 ? <p className="p-3 text-sm text-slate-500">No CAP receivers yet.</p> : undefined}>
          {settings.receivers.map((r) => (
            <tr key={r.id} className="hover:bg-slate-800/50">
              <td className="px-3 py-1.5 text-slate-100">{r.name || r.id}</td>
              <td className="px-3 font-mono text-slate-400">{r.url}</td>
              <td className="px-3"><span className={cx('chip', r.off ? 'bg-slate-800 text-slate-400' : 'bg-emerald-500/20 text-emerald-200')}>{r.off ? 'Off' : 'On'}</span></td>
              <td className="px-3">
                <div className="flex justify-end gap-2">
                  <button className="btn" onClick={() => testReceiver(r.id)}><Send size={13} aria-hidden /> Send a test</button>
                  {admin && <button className="btn" onClick={() => setRx({ ...r, token: MASK, isNew: false })}>Edit</button>}
                </div>
              </td>
            </tr>
          ))}
        </Table>
      </section>

      {/* 3. Subscriber lists */}
      <section className="panel mt-4 space-y-3 rounded-lg p-4" aria-labelledby="lists-h">
        <div className="flex items-center gap-2">
          <h2 id="lists-h" className="flex items-center gap-2 font-semibold text-slate-100"><Users size={15} className="text-sky-300" aria-hidden /> Subscriber lists</h2>
          <button className="btn ml-auto" disabled={gateways.length === 0} onClick={() => openList()}><Plus size={13} aria-hidden /> Add a list</button>
        </div>
        <p className="text-xs text-slate-400">People who asked for alerts. An alert goes to everyone on its lists who has no place given, and to those with a place only when it is inside the alert's area.</p>
        {gateways.length === 0 && (
          <div className="rounded-lg border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-amber-200">
            No paging gateway yet. <Link to="/admin/integrations" className="underline">Set up a paging gateway first</Link> so an alert has a way to reach people.
          </div>
        )}
        <Table head={['Name', 'How reached', 'People', 'Note']} empty={lists.length === 0 ? <p className="p-3 text-sm text-slate-500">No subscriber lists yet.</p> : undefined}>
          {lists.map((l) => (
            <tr key={l.id} className="cursor-pointer hover:bg-slate-800/50" onClick={() => openList(l)}>
              <td className="px-3 py-1.5 text-slate-100">{l.name || l.id}</td>
              <td className="px-3 text-slate-400">{l.gateways?.length ? l.gateways.map(gatewayName).join(', ') : '—'}</td>
              <td className="px-3 text-slate-400">{l.people?.length ?? 0}</td>
              <td className="px-3 text-slate-400">{l.note || '—'}</td>
            </tr>
          ))}
        </Table>
      </section>

      {/* Receiver editor */}
      {rx && (
        <Modal
          open
          onClose={() => setRx(null)}
          title={rx.isNew ? 'New CAP receiver' : rx.name || rx.id}
          icon={<Server size={16} className="text-sky-300" aria-hidden />}
          footer={
            <>
              {!rx.isNew && <button className="btn mr-auto text-rose-300" onClick={() => removeReceiver(rx.id)}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => setRx(null)}>Cancel</button>
              <button className="btn btn-go" onClick={saveReceiver} disabled={!rx.name.trim() || !rx.url.trim()}><Save size={13} aria-hidden /> Save</button>
            </>
          }
        >
          <div className="space-y-4">
            <Field label="Name" htmlFor="rx-name"><input id="rx-name" className="input" value={rx.name} onChange={(e) => setRx({ ...rx, name: e.target.value })} placeholder="State CAP aggregator" /></Field>
            <Field label="URL" htmlFor="rx-url"><input id="rx-url" className="input font-mono" value={rx.url} onChange={(e) => setRx({ ...rx, url: e.target.value })} placeholder="https://…" /></Field>
            <Field label="Token" htmlFor="rx-token" hint="Sent as Authorization: Bearer. Leave as dots to keep it, type - to clear it">
              <input id="rx-token" type="password" autoComplete="new-password" className="input font-mono" value={rx.token ?? ''} onChange={(e) => setRx({ ...rx, token: e.target.value })} />
            </Field>
            <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!rx.off} onChange={(e) => setRx({ ...rx, off: e.target.checked })} /> Off (do not send to this receiver)</label>
          </div>
        </Modal>
      )}

      {/* List editor */}
      {list && (
        <Modal
          open
          onClose={() => setList(null)}
          xl
          title={listNew ? 'New subscriber list' : list.name || list.id}
          icon={<Users size={16} className="text-sky-300" aria-hidden />}
          footer={
            <>
              {!listNew && <button className="btn mr-auto text-rose-300" onClick={() => removeList(list.id)}><Trash2 size={13} aria-hidden /> Remove</button>}
              <button className="btn" onClick={() => setList(null)}>Cancel</button>
              <button className="btn btn-go" onClick={saveList} disabled={!list.name.trim()}><Save size={13} aria-hidden /> Save</button>
            </>
          }
        >
          <div className="space-y-4">
            <Field label="Name" htmlFor="list-name"><input id="list-name" className="input" value={list.name} onChange={(e) => setList({ ...list, name: e.target.value })} placeholder="Riverside flood watch" /></Field>

            <fieldset>
              <legend className="label">How they are reached</legend>
              <div className="flex flex-wrap gap-1">
                {gateways.map((g) => (
                  <button key={g.id} type="button" aria-pressed={list.gateways.includes(g.id)} className={cx('chip', list.gateways.includes(g.id) ? 'bg-sky-600 text-white' : 'bg-slate-800 text-slate-300')} onClick={() => setList({ ...list, gateways: toggle(list.gateways, g.id) })}>{g.name}</button>
                ))}
                {gateways.length === 0 && <span className="text-xs text-slate-500">No gateways set up.</span>}
              </div>
            </fieldset>

            <Field label="Note" htmlFor="list-note"><input id="list-note" className="input" value={list.note ?? ''} onChange={(e) => setList({ ...list, note: e.target.value })} placeholder="Shown with the list" /></Field>

            <fieldset className="space-y-2">
              <legend className="label">People</legend>
              <div className="overflow-x-auto rounded border border-slate-800">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-slate-800 text-left text-[11px] uppercase tracking-wider text-slate-500">
                      <th className="px-2 py-1.5 font-semibold">Name</th>
                      <th className="px-2 font-semibold">Phone</th>
                      <th className="px-2 font-semibold">Email</th>
                      <th className="px-2 font-semibold">ntfy topic</th>
                      <th className="px-2 font-semibold">Lang</th>
                      <th className="px-2 font-semibold">Lat</th>
                      <th className="px-2 font-semibold">Lon</th>
                      <th className="px-2" />
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-800">
                    {list.people.slice(0, 200).map((p, i) => (
                      <tr key={i}>
                        <td className="px-2 py-1"><input className="input h-7 text-xs" aria-label={`Name, row ${i + 1}`} value={p.name ?? ''} onChange={(e) => setPerson(i, { name: e.target.value })} /></td>
                        <td className="px-2"><input className="input h-7 font-mono text-xs" aria-label={`Phone, row ${i + 1}`} value={p.phone ?? ''} onChange={(e) => setPerson(i, { phone: e.target.value })} placeholder="(540) 555-0101" /></td>
                        <td className="px-2"><input className="input h-7 font-mono text-xs" aria-label={`Email, row ${i + 1}`} value={p.email ?? ''} onChange={(e) => setPerson(i, { email: e.target.value })} /></td>
                        <td className="px-2"><input className="input h-7 font-mono text-xs" aria-label={`ntfy topic, row ${i + 1}`} value={p.topic ?? ''} onChange={(e) => setPerson(i, { topic: e.target.value })} /></td>
                        <td className="px-2">
                          <select className="input h-7 text-xs" aria-label={`Language, row ${i + 1}`} value={p.lang ?? 'en'} onChange={(e) => setPerson(i, { lang: e.target.value })}>
                            <option value="en">en</option>
                            <option value="es">es</option>
                          </select>
                        </td>
                        <td className="px-2"><input className="input h-7 font-mono text-xs" aria-label={`Latitude, row ${i + 1}`} value={p.lat ?? ''} onChange={(e) => setPerson(i, { lat: e.target.value === '' ? undefined : Number(e.target.value) })} /></td>
                        <td className="px-2"><input className="input h-7 font-mono text-xs" aria-label={`Longitude, row ${i + 1}`} value={p.lon ?? ''} onChange={(e) => setPerson(i, { lon: e.target.value === '' ? undefined : Number(e.target.value) })} /></td>
                        <td className="px-2"><button className="btn btn-ghost px-1.5" aria-label={`Remove row ${i + 1}`} onClick={() => removePerson(i)}><Trash2 size={13} aria-hidden /></button></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {list.people.length > 200 && <p className="text-xs text-slate-500">Showing the first 200 of {list.people.length}. Import a CSV to replace the whole list.</p>}
              <button className="btn" onClick={addPerson}><Plus size={13} aria-hidden /> Add a person</button>
            </fieldset>

            <fieldset className="space-y-2 rounded-md border border-slate-800 p-3">
              <legend className="label flex items-center gap-1"><ListPlus size={13} aria-hidden /> Import a CSV</legend>
              <p className="text-xs text-slate-500">The first row names the columns: name, phone, email, topic, lang, lat, lon. Phones like (540) 555-0101 become +15405550101.</p>
              {listNew ? (
                <p className="text-xs text-amber-300">Save the list first, then a CSV can be imported into it.</p>
              ) : (
                <div className="flex flex-wrap items-end gap-2">
                  <Field label="How to apply it" htmlFor="list-import-mode">
                    <select id="list-import-mode" className="input" value={importMode} onChange={(e) => setImportMode(e.target.value as 'add' | 'replace')}>
                      <option value="add">Add to the list</option>
                      <option value="replace">Replace the list</option>
                    </select>
                  </Field>
                  <Field label="CSV file" htmlFor="list-import-file">
                    <input id="list-import-file" type="file" accept=".csv,text/csv" className="input" onChange={(e) => { const f = e.target.files?.[0]; if (f) importCsv(f); e.target.value = '' }} />
                  </Field>
                </div>
              )}
            </fieldset>
          </div>
        </Modal>
      )}
    </Page>
  )
}
