// Public alerts on the console: what is in force, writing an alert (with its phone text, the area and who
// it goes to), checking it as the CAP message it makes, approving another supervisor's, updating and
// cancelling. internal/cad/publicalerts.go and internal/api/publicalerts.go do the work.
import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { AlertTriangle, Check, FileCode, Megaphone, Pencil, Send, X } from 'lucide-react'
import { api, errText } from '../api'
import { can, useStore } from '../store'
import { Field, Modal } from '../ui'
import { cx } from '../util'
import type { AlertingSettings, AlertList, AlertText, Incident, PublicAlert, PublicAlertBrief, View } from '../types'

type Status = 'Actual' | 'Exercise' | 'Test'

/** What a kind of alert usually says, in English and Spanish, to start from. */
const templates: Record<string, { response: string; en: AlertText; es: AlertText }> = {
  SPW: { response: 'Shelter', en: { headline: 'Shelter in place', instruction: 'Go inside now. Close windows and doors. Turn off fans, heating and air conditioning. Stay inside until told it is safe.', short: 'Go inside now, close windows and doors, turn off fans and AC.' },
    es: { headline: 'Refúgiese en un lugar cerrado', instruction: 'Entre ahora. Cierre puertas y ventanas. Apague ventiladores, calefacción y aire acondicionado. Quédese adentro hasta que le digan que es seguro.', short: 'Entre ahora, cierre puertas y ventanas, apague ventiladores y aire.' } },
  EVI: { response: 'Evacuate', en: { headline: 'Evacuate now', instruction: 'Leave the area now. Take medicines, pets and phone chargers. Do not return until told it is safe.', short: 'Leave the area now. Do not return until told it is safe.' },
    es: { headline: 'Evacúe ahora', instruction: 'Salga del área ahora. Lleve medicinas, mascotas y cargadores. No regrese hasta que le digan que es seguro.', short: 'Salga del área ahora. No regrese hasta que sea seguro.' } },
  LEW: { response: 'Avoid', en: { headline: 'Police activity: avoid the area', instruction: 'Stay inside, lock doors and keep away from windows. Avoid the area. Call 911 to report anything suspicious.', short: 'Police activity. Stay inside, lock doors, avoid the area.' },
    es: { headline: 'Actividad policial: evite el área', instruction: 'Quédese adentro, cierre las puertas con llave y aléjese de las ventanas. Evite el área.', short: 'Actividad policial. Quédese adentro y evite el área.' } },
  HMW: { response: 'Avoid', en: { headline: 'Hazardous materials: avoid the area', instruction: 'Avoid the area. If you are nearby, go inside and close windows and doors.', short: 'Hazardous spill. Avoid the area; nearby, go inside and close windows.' },
    es: { headline: 'Materiales peligrosos: evite el área', instruction: 'Evite el área. Si está cerca, entre y cierre puertas y ventanas.', short: 'Derrame peligroso. Evite el área; si está cerca, entre.' } },
  FRW: { response: 'Prepare', en: { headline: 'Fire warning', instruction: 'Be ready to leave at once. Follow the directions of firefighters and police.', short: 'Fire nearby. Be ready to leave at once. Follow responders.' },
    es: { headline: 'Aviso de incendio', instruction: 'Esté listo para salir de inmediato. Siga las instrucciones de bomberos y policía.', short: 'Incendio cerca. Esté listo para salir de inmediato.' } },
  CAE: { response: 'Monitor', en: { headline: 'Child abduction emergency', description: 'Describe the child, the suspect and the vehicle.', instruction: 'If you see them, call 911. Do not approach.', short: 'Child abduction. Look for (vehicle, plate). Call 911 if seen.' },
    es: { headline: 'Emergencia por secuestro de menor', instruction: 'Si los ve, llame al 911. No se acerque.', short: 'Secuestro de menor. Si lo ve, llame al 911.' } },
  TOE: { response: 'None', en: { headline: '911 is not working', instruction: 'For an emergency call the number below, or go to the nearest fire or police station.', short: '911 is down. For emergencies call (number).' },
    es: { headline: 'El 911 no funciona', instruction: 'Para una emergencia llame al número indicado o vaya a la estación de bomberos o policía más cercana.', short: 'El 911 no funciona. Para emergencias llame al (número).' } },
  CEM: { response: 'Monitor', en: { headline: 'Civil emergency', instruction: 'Follow the directions of local officials.', short: 'Civil emergency. Follow the directions of local officials.' },
    es: { headline: 'Emergencia civil', instruction: 'Siga las instrucciones de las autoridades locales.', short: 'Emergencia civil. Siga las instrucciones de las autoridades.' } },
  DMO: { response: 'None', en: { headline: 'This is a test of the alert system', instruction: 'No action is needed.', short: 'TEST of the alert system. No action is needed.' },
    es: { headline: 'Esta es una prueba del sistema de alertas', instruction: 'No hace falta hacer nada.', short: 'PRUEBA del sistema de alertas. No hace falta hacer nada.' } },
}

const hours = [1, 2, 4, 8, 12, 24]
const radii = [0.25, 0.5, 1, 2]
const channelNames: Record<string, string> = { EAS: 'Radio and TV (EAS)', NWEM: 'Weather radio', CMAS: 'Phones (Wireless Emergency Alerts)', PUBLIC: 'Public sites and apps' }

/** A circle of miles around a point, as [lon, lat] corners. */
function ring(lat: number, lon: number, miles: number): [number, number][] {
  const km = miles * 1.609344
  const out: [number, number][] = []
  for (let i = 0; i < 24; i++) {
    const a = (i / 24) * 2 * Math.PI
    out.push([+(lon + (km / (111.32 * Math.cos((lat * Math.PI) / 180))) * Math.sin(a)).toFixed(5), +(lat + (km / 110.574) * Math.cos(a)).toFixed(5)])
  }
  return out
}

const statusChip = (s: string) => (s === 'Actual' ? 'bg-rose-500/25 text-rose-100' : 'bg-amber-500/25 text-amber-100')
const stateChip: Record<string, string> = { pending: 'bg-amber-500/25 text-amber-100', issued: 'bg-emerald-500/20 text-emerald-200', rejected: 'bg-slate-700 text-slate-300', updated: 'bg-slate-700 text-slate-300', cancelled: 'bg-slate-700 text-slate-300' }
const when = (iso?: string) => (iso ? new Date(iso).toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }) : '')

/** The status bar's megaphone: alerts in force, and for supervisors the ones waiting for approval. */
export function PublicAlertsButton({ view }: { view: View }) {
  const me = useStore((s) => s.me)
  const [open, setOpen] = useState(false)
  const alerts = view.roadmap?.publicAlerts ?? []
  const live = alerts.filter((a) => a.state === 'issued')
  const waiting = alerts.filter((a) => a.state === 'pending' && a.by !== me?.username)
  if (!can.supervise(me) && live.length === 0) return null
  return (
    <>
      <button className={cx('btn btn-ghost gap-1 px-1.5', live.some((a) => a.status === 'Actual') && 'text-rose-300')} onClick={() => setOpen(true)}
        aria-label={`Public alerts: ${live.length} in force${waiting.length ? `, ${waiting.length} waiting for approval` : ''}`} title="Public alerts (mass notification)">
        <Megaphone size={14} aria-hidden />
        {live.length > 0 && <span className="chip bg-rose-500/25 text-rose-100">{live.length}</span>}
        {can.supervise(me) && waiting.length > 0 && <span className="chip animate-pulse bg-amber-500/30 text-amber-100">{waiting.length} to approve</span>}
      </button>
      {open && <PublicAlertsDialog view={view} onClose={() => setOpen(false)} />}
    </>
  )
}

/** On an incident: a supervisor can alert the public around it. */
export function PublicAlertBox({ inc, view }: { inc: Incident; view: View }) {
  const me = useStore((s) => s.me)
  const [open, setOpen] = useState(false)
  const mine = (view.roadmap?.publicAlerts ?? []).filter((a) => a.incident === inc.number)
  if (!can.supervise(me) && mine.length === 0) return null
  return (
    <section className="panel rounded border border-slate-800 p-2 text-xs" aria-label="Public alerts for this incident">
      <div className="flex items-center gap-2">
        <h3 className="flex items-center gap-1 text-[11px] font-bold uppercase tracking-wider text-rose-300"><Megaphone size={12} aria-hidden /> Public alert</h3>
        {mine.map((a) => <span key={a.id} className={cx('chip', statusChip(a.status))}>{a.state === 'pending' ? 'waiting: ' : ''}{a.eventName}</span>)}
        {can.supervise(me) && inc.status !== 'closed' && <button className="btn ml-auto h-6 text-xs" onClick={() => setOpen(true)}><Megaphone size={12} aria-hidden /> Alert the public</button>}
      </div>
      {open && <PublicAlertsDialog view={view} incident={inc} compose onClose={() => setOpen(false)} />}
    </section>
  )
}

type Draft = {
  msgType: 'Alert' | 'Update'; of?: string; status: Status; event: string; response: string
  text: AlertText; spanishOn: boolean; spanish: AlertText
  areaKind: 'same' | 'incident' | 'fence' | 'whole'; radius: number; fence: string; desc: string; area?: PublicAlert['area']
  hours: number; lists: string[]; receivers: string[]; block: string[]; incident?: string
}

/** Public alerts: the list, and writing one. */
export function PublicAlertsDialog({ view, onClose, incident, compose }: { view: View; onClose: () => void; incident?: Incident; compose?: boolean }) {
  const me = useStore((s) => s.me)
  const [items, setItems] = useState<PublicAlert[]>([])
  const [settings, setSettings] = useState<AlertingSettings | null>(null)
  const [lists, setLists] = useState<AlertList[]>([])
  const [d, setD] = useState<Draft | null>(null)
  const [ready, setReady] = useState(false)
  const load = () => api.get<{ items: PublicAlert[]; settings: AlertingSettings }>('/alerts').then((r) => { setItems(r.items); setSettings(r.settings) }).catch((e) => toast.error(errText(e)))
  useEffect(() => {
    Promise.all([load(), api.get<{ items: AlertList[] }>('/admin/alertlists').then((r) => setLists(r.items)).catch(() => {})]).then(() => setReady(true))
  }, [])
  // Opened from an incident: start writing at once, once the lists and receivers are known.
  useEffect(() => {
    if (ready && compose) start()
  }, [ready])
  const hasPoint = (i?: Incident) => i?.location?.lat != null && i?.location?.lon != null
  const start = (from?: PublicAlert) => {
    const ev = from?.event ?? (incident && /HAZ|GAS|CHEM/.test(incident.type) ? 'HMW' : 'SPW')
    const t = templates[ev] ?? templates.CEM
    const fence = view.roadmap?.geofences?.[0]?.id ?? ''
    setD(from ? {
      msgType: 'Update', of: from.id, status: from.status, event: from.event, response: from.response ?? '', text: { ...from.text }, spanishOn: !!from.spanish, spanish: from.spanish ? { ...from.spanish } : { ...t.es },
      areaKind: 'same', area: from.area, radius: 0.5, fence: from.area.fence ?? fence, desc: from.area.desc,
      hours: 2, lists: from.lists ?? [], receivers: from.receivers ?? [], block: from.block ?? [], incident: from.incident,
    } : {
      msgType: 'Alert', status: 'Actual', event: ev, response: t.response, text: { ...t.en }, spanishOn: false, spanish: { ...t.es },
      areaKind: hasPoint(incident) ? 'incident' : fence ? 'fence' : 'whole', radius: 0.5, fence, desc: '',
      hours: 2, lists: lists.map((l) => l.id), receivers: (settings?.receivers ?? []).filter((r) => !r.off).map((r) => r.id), block: [], incident: incident?.number,
    })
  }
  const act = async (path: string, body?: unknown, ok?: string) => {
    try {
      await api.post(path, body)
      if (ok) toast.success(ok)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const cancelAlert = (a: PublicAlert) => {
    if (!confirm(`Cancel "${a.text.headline}"? An all-clear goes to the same people and places.`)) return
    act('/alerts', { msgType: 'Cancel', of: a.id }, settings?.twoPerson ? 'The cancel waits for a second supervisor' : 'Cancelled: the all-clear is going out')
  }
  const now = Date.now()
  return (
    <Modal open onClose={onClose} xl title="Public alerts" icon={<Megaphone size={16} className="text-rose-300" aria-hidden />}
      footer={<>{can.supervise(me) && !d && <button className="btn btn-go mr-auto" onClick={() => start()}><Megaphone size={13} aria-hidden /> Write an alert</button>}<button className="btn" onClick={onClose}>Close</button></>}>
      {settings && settings.ready.length > 0 && (
        <div className="mb-3 rounded border border-amber-500/40 bg-amber-500/10 p-2 text-xs text-amber-200">
          {settings.ready.map((r) => <div key={r}><AlertTriangle size={12} className="mr-1 inline" aria-hidden />{r}</div>)}
        </div>
      )}
      {d ? (
        <Compose view={view} draft={d} setDraft={setD} settings={settings} lists={lists} incident={incident} onDone={() => { setD(null); load() }} />
      ) : (
        <table className="w-full text-sm">
          <thead><tr className="text-left text-[10px] uppercase text-slate-500"><th>State</th><th>Alert</th><th>Area</th><th>Sent</th><th>Until</th><th>Reached</th><th /></tr></thead>
          <tbody className="divide-y divide-slate-800">
            {items.map((a) => {
              const inForce = a.state === 'issued' && Date.parse(a.expires) > now
              const bad = a.results.filter((r) => !r.ok)
              return (
                <tr key={a.id} className={cx(!inForce && a.state !== 'pending' && 'opacity-60')}>
                  <td className="py-1"><span className={cx('chip', stateChip[a.state])}>{a.state === 'issued' && !inForce ? 'expired' : a.state}</span> <span className={cx('chip', statusChip(a.status))}>{a.status}</span></td>
                  <td>
                    <div className="text-slate-100">{a.msgType !== 'Alert' && <span className="mr-1 text-xs text-slate-400">{a.msgType}:</span>}{a.text.headline}</div>
                    <div className="text-xs text-slate-500">{a.eventName} ({a.event}) · {a.id} · by {a.by}{a.approvedBy && a.approvedBy !== a.by ? `, approved by ${a.approvedBy}` : ''}{a.rejected ? ` · ${a.rejected}` : ''}</div>
                  </td>
                  <td className="text-xs text-slate-400">{a.area.desc}</td>
                  <td className="font-mono text-xs">{when(a.sent)}</td>
                  <td className="font-mono text-xs">{when(a.expires)}</td>
                  <td className="text-xs" title={a.results.map((r) => `${r.gateway} ${r.to}: ${r.ok ? 'ok' : r.detail}`).join('\n')}>
                    {a.state === 'issued' || a.results.length > 0 ? <>{a.reached} people{a.results.some((r) => r.kind === 'cap') && `, ${a.results.filter((r) => r.kind === 'cap' && r.ok).length} systems`}{bad.length > 0 && <span className="ml-1 text-rose-300">{bad.length} failed</span>}</> : ''}
                  </td>
                  <td className="whitespace-nowrap text-right">
                    {a.state === 'pending' && can.supervise(me) && a.by !== me?.username && <>
                      <button className="btn btn-go h-6 text-xs" onClick={() => confirm(`Approve and send "${a.text.headline}" (${a.status}) to the public now?`) && act(`/alerts/${a.id}/approve`, undefined, 'Approved: the alert is going out')}><Check size={12} aria-hidden /> Approve</button>{' '}
                      <button className="btn h-6 text-xs" onClick={() => { const reason = prompt('Why not?') ?? ''; act(`/alerts/${a.id}/reject`, { reason }, 'Turned down') }}><X size={12} aria-hidden /> Reject</button>
                    </>}
                    {a.state === 'pending' && a.by === me?.username && <button className="btn h-6 text-xs" onClick={() => act(`/alerts/${a.id}/reject`, { reason: '' }, 'Withdrawn')}>Withdraw</button>}
                    {inForce && can.supervise(me) && a.msgType !== 'Cancel' && <>
                      <button className="btn h-6 text-xs" onClick={() => start(a)}><Pencil size={12} aria-hidden /> Update</button>{' '}
                      <button className="btn h-6 text-xs text-rose-300" onClick={() => cancelAlert(a)}><X size={12} aria-hidden /> Cancel</button>
                    </>}
                    {' '}<a className="btn btn-ghost h-6 px-1 text-xs" href={`./api/alerts/${a.id}/cap`} target="_blank" rel="noreferrer" aria-label={`The CAP message of ${a.id}`} title="The CAP message"><FileCode size={12} aria-hidden /></a>
                  </td>
                </tr>
              )
            })}
            {items.length === 0 && <tr><td colSpan={7} className="py-2 text-slate-500">No public alerts yet.</td></tr>}
          </tbody>
        </table>
      )}
    </Modal>
  )
}

function Counter({ text, max }: { text?: string; max: number }) {
  const n = [...(text ?? '')].length
  return <span className={cx('font-mono text-[10px]', n > max ? 'text-rose-300' : n > max - 10 ? 'text-amber-300' : 'text-slate-500')}>{n}/{max}</span>
}

function Compose({ view, draft: d, setDraft: setD, settings, lists, incident, onDone }: {
  view: View; draft: Draft; setDraft: (d: Draft | null) => void; settings: AlertingSettings | null; lists: AlertList[]; incident?: Incident; onDone: () => void
}) {
  const [check, setCheck] = useState<{ problems: string[]; cap: string; reach: number; twoPerson: boolean } | null>(null)
  const [confirming, setConfirming] = useState(false)
  const [busy, setBusy] = useState(false)
  const fences = view.roadmap?.geofences ?? []
  const events = settings?.events ?? []
  const loc = incident?.location
  const set = (p: Partial<Draft>) => { setCheck(null); setD({ ...d, ...p }) }
  const setText = (p: Partial<AlertText>) => set({ text: { ...d.text, ...p } })
  const setEs = (p: Partial<AlertText>) => set({ spanish: { ...d.spanish, ...p } })
  const autoDesc = useMemo(() => {
    if (d.areaKind === 'incident' && loc) return `Within ${d.radius} mile${d.radius === 1 ? '' : 's'} of ${loc.address ?? incident?.number}`
    if (d.areaKind === 'same') return d.area?.desc ?? ''
    if (d.areaKind === 'fence') return fences.find((f) => f.id === d.fence)?.name ?? ''
    return settings?.jurisdiction || 'The whole jurisdiction'
  }, [d.areaKind, d.area, d.radius, d.fence, loc, fences, settings, incident])
  const body = () => {
    const area = d.areaKind === 'same' && d.area ? { ...d.area, desc: d.desc || d.area.desc }
      : d.areaKind === 'incident' && loc?.lat != null && loc?.lon != null ? { desc: d.desc || autoDesc, polygon: ring(loc.lat, loc.lon, d.radius) }
      : d.areaKind === 'fence' ? { desc: d.desc || autoDesc, fence: d.fence } : { desc: '' }
    return {
      msgType: d.msgType, of: d.of, status: d.status, event: d.event, response: d.response, text: d.text, spanish: d.spanishOn ? d.spanish : undefined,
      area, whole: d.areaKind === 'whole' || (d.areaKind === 'same' && !d.area?.polygon?.length && !d.area?.fence), expires: new Date(Date.now() + d.hours * 3600000).toISOString(), lists: d.lists, receivers: d.receivers, block: d.block, incident: d.incident,
    }
  }
  const preview = async () => {
    try {
      const r = await api.post<{ problems: string[] | null; cap: string; reach: number; twoPerson: boolean }>('/alerts/check', body())
      setCheck({ ...r, problems: r.problems ?? [] })
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const send = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ id: string; state: string }>('/alerts', body())
      toast.success(r.state === 'pending' ? `${r.id} waits for a second supervisor to approve it` : `${r.id} is going out`)
      onDone()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
      setConfirming(false)
    }
  }
  const pickEvent = (code: string) => {
    const t = templates[code]
    if (!t || d.msgType === 'Update') return set({ event: code })
    set({ event: code, response: t.response, text: { ...t.en }, spanish: { ...t.es } })
  }
  const toggle = (k: 'lists' | 'receivers' | 'block', id: string) => set({ [k]: d[k].includes(id) ? d[k].filter((x) => x !== id) : [...d[k], id] } as Partial<Draft>)
  return (
    <div className="grid gap-3 lg:grid-cols-2">
      <div className="space-y-3">
        {d.msgType === 'Update' && <p className="rounded border border-sky-700 bg-sky-950/30 p-2 text-xs text-sky-200">An update to {d.of}: it replaces that alert for everyone who has it.</p>}
        <div className="flex flex-wrap gap-2" role="radiogroup" aria-label="Real, exercise or test">
          {(['Actual', 'Exercise', 'Test'] as Status[]).map((s) => (
            <button key={s} role="radio" aria-checked={d.status === s} disabled={d.msgType === 'Update'} onClick={() => set({ status: s })}
              className={cx('btn', d.status === s && (s === 'Actual' ? 'border-rose-400 bg-rose-500/20 text-rose-100' : 'border-amber-400 bg-amber-500/20 text-amber-100'))}>
              {s === 'Actual' ? 'Real alert' : s}
            </button>
          ))}
        </div>
        <Field label="Kind of alert" htmlFor="paev">
          <select id="paev" className="input" value={d.event} onChange={(e) => pickEvent(e.target.value)}>
            {events.map((e) => <option key={e.code} value={e.code}>{e.name} ({e.code})</option>)}
          </select>
        </Field>
        <Field label="Headline" htmlFor="pahead"><input id="pahead" className="input" value={d.text.headline} onChange={(e) => setText({ headline: e.target.value })} /></Field>
        <Field label="What is happening" htmlFor="padesc"><textarea id="padesc" rows={2} className="input" value={d.text.description ?? ''} onChange={(e) => setText({ description: e.target.value })} placeholder="A chlorine leak at the water plant on Main St." /></Field>
        <Field label="What to do" htmlFor="pains"><textarea id="pains" rows={2} className="input" value={d.text.instruction ?? ''} onChange={(e) => setText({ instruction: e.target.value })} /></Field>
        <Field label="Phone alert (Wireless Emergency Alerts)" htmlFor="pashort" hint={<><Counter text={d.text.short} max={90} /> · what phones show first, and what text messages and push notifications say</>}>
          <textarea id="pashort" rows={2} className="input" value={d.text.short ?? ''} onChange={(e) => setText({ short: e.target.value })} />
        </Field>
        <Field label="Longer phone alert (optional)" htmlFor="palong" hint={<Counter text={d.text.long} max={360} />}>
          <textarea id="palong" rows={2} className="input" value={d.text.long ?? ''} onChange={(e) => setText({ long: e.target.value })} />
        </Field>
        <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={d.spanishOn} onChange={(e) => set({ spanishOn: e.target.checked })} /> Also in Spanish</label>
        {d.spanishOn && (
          <div className="space-y-2 rounded border border-slate-800 p-2">
            <Field label="Titular" htmlFor="pashead"><input id="pashead" lang="es" className="input" value={d.spanish.headline} onChange={(e) => setEs({ headline: e.target.value })} /></Field>
            <Field label="Qué pasa" htmlFor="pasdesc"><textarea id="pasdesc" lang="es" rows={2} className="input" value={d.spanish.description ?? ''} onChange={(e) => setEs({ description: e.target.value })} /></Field>
            <Field label="Qué hacer" htmlFor="pasins"><textarea id="pasins" lang="es" rows={2} className="input" value={d.spanish.instruction ?? ''} onChange={(e) => setEs({ instruction: e.target.value })} /></Field>
            <Field label="Alerta al teléfono" htmlFor="passhort" hint={<Counter text={d.spanish.short} max={90} />}><textarea id="passhort" lang="es" rows={2} className="input" value={d.spanish.short ?? ''} onChange={(e) => setEs({ short: e.target.value })} /></Field>
          </div>
        )}
      </div>
      <div className="space-y-3">
        <div>
          <span className="label">Where</span>
          <div className="flex flex-wrap gap-2" role="radiogroup" aria-label="Where">
            {d.area && <button role="radio" aria-checked={d.areaKind === 'same'} className={cx('btn', d.areaKind === 'same' && 'border-sky-400 bg-sky-500/15')} onClick={() => set({ areaKind: 'same', desc: d.area?.desc ?? '' })}>The same area</button>}
            {loc?.lat != null && <button role="radio" aria-checked={d.areaKind === 'incident'} className={cx('btn', d.areaKind === 'incident' && 'border-sky-400 bg-sky-500/15')} onClick={() => set({ areaKind: 'incident', desc: '' })}>Around the incident</button>}
            {fences.length > 0 && <button role="radio" aria-checked={d.areaKind === 'fence'} className={cx('btn', d.areaKind === 'fence' && 'border-sky-400 bg-sky-500/15')} onClick={() => set({ areaKind: 'fence', desc: '' })}>An area on the map</button>}
            <button role="radio" aria-checked={d.areaKind === 'whole'} className={cx('btn', d.areaKind === 'whole' && 'border-sky-400 bg-sky-500/15')} onClick={() => set({ areaKind: 'whole', desc: '' })}>Everyone ({settings?.jurisdiction || 'the jurisdiction'})</button>
          </div>
        </div>
        {d.areaKind === 'incident' && (
          <Field label="How far around it" htmlFor="parad"><select id="parad" className="input w-40" value={d.radius} onChange={(e) => set({ radius: Number(e.target.value) })}>{radii.map((r) => <option key={r} value={r}>{r} mile{r === 1 ? '' : 's'}</option>)}</select></Field>
        )}
        {d.areaKind === 'fence' && (
          <Field label="Area" htmlFor="pafence"><select id="pafence" className="input" value={d.fence} onChange={(e) => set({ fence: e.target.value })}>{fences.map((f) => <option key={f.id} value={f.id}>{f.name}</option>)}</select></Field>
        )}
        {d.areaKind !== 'whole' && <Field label="The area in words" htmlFor="paadesc" hint="What people hear and read: streets, neighbourhoods, landmarks"><input id="paadesc" className="input" value={d.desc} placeholder={autoDesc} onChange={(e) => set({ desc: e.target.value })} /></Field>}
        <Field label="Expires after" htmlFor="pahours"><select id="pahours" className="input w-40" value={d.hours} onChange={(e) => set({ hours: Number(e.target.value) })}>{hours.map((h) => <option key={h} value={h}>{h} hour{h === 1 ? '' : 's'}</option>)}</select></Field>
        <div>
          <span className="label">Send to the people on</span>
          <div className="flex flex-wrap gap-1">
            {lists.map((l) => <button key={l.id} aria-pressed={d.lists.includes(l.id)} onClick={() => toggle('lists', l.id)} className={cx('chip cursor-pointer', d.lists.includes(l.id) ? 'bg-sky-500/30 text-sky-100' : 'bg-slate-800 text-slate-400')}>{l.name} ({l.people.length})</button>)}
            {lists.length === 0 && <span className="text-xs text-slate-500">No subscriber lists (Admin › Public alerts)</span>}
          </div>
        </div>
        <div>
          <span className="label">And to these systems (CAP)</span>
          <div className="flex flex-wrap gap-1">
            {(settings?.receivers ?? []).map((r) => <button key={r.id} aria-pressed={d.receivers.includes(r.id)} onClick={() => toggle('receivers', r.id)} className={cx('chip cursor-pointer', d.receivers.includes(r.id) ? 'bg-sky-500/30 text-sky-100' : 'bg-slate-800 text-slate-400')}>{r.name}</button>)}
            {(settings?.receivers ?? []).length === 0 && <span className="text-xs text-slate-500">None set up</span>}
          </div>
          {settings?.feed && <p className="mt-1 text-xs text-slate-500">It is also in the public alert feed while it is in force.</p>}
        </div>
        <details className="text-xs text-slate-400">
          <summary className="cursor-pointer">Keep it off some channels</summary>
          <div className="mt-1 flex flex-wrap gap-1">
            {(settings?.channels ?? []).map((c) => <button key={c} aria-pressed={d.block.includes(c)} onClick={() => toggle('block', c)} className={cx('chip cursor-pointer', d.block.includes(c) ? 'bg-amber-500/30 text-amber-100' : 'bg-slate-800 text-slate-400')}>{d.block.includes(c) ? 'not on ' : ''}{channelNames[c] ?? c}</button>)}
          </div>
          <p className="mt-1">For alert origination software that sends to IPAWS: the message asks it to leave these out.</p>
        </details>
        <div className="flex flex-wrap items-center gap-2 border-t border-slate-800 pt-2">
          <button className="btn" onClick={() => setD(null)}>Back</button>
          <button className="btn" onClick={preview}><FileCode size={13} aria-hidden /> Check</button>
          <button className={cx('btn ml-auto', d.status === 'Actual' ? 'btn-go' : 'border-amber-400 bg-amber-500/20 text-amber-100')} disabled={!check || check.problems.length > 0} onClick={() => setConfirming(true)} title={!check ? 'Check it first' : undefined}>
            <Send size={13} aria-hidden /> {check?.twoPerson ? 'Send for approval' : 'Send'}
          </button>
        </div>
        {check && (
          <div className="space-y-1 text-xs" aria-live="polite">
            {check.problems.length > 0
              ? <ul className="rounded border border-rose-700 bg-rose-950/30 p-2 text-rose-200">{check.problems.map((p) => <li key={p}>{p}</li>)}</ul>
              : <p className="rounded border border-emerald-700 bg-emerald-950/30 p-2 text-emerald-200">Ready: a valid CAP message for the IPAWS profile. It goes to {check.reach} {check.reach === 1 ? 'person' : 'people'} on the lists{d.receivers.length ? ` and ${d.receivers.length} system${d.receivers.length === 1 ? '' : 's'}` : ''}.{check.twoPerson && ' A second supervisor approves it first.'}</p>}
            <details><summary className="cursor-pointer text-slate-400">The CAP message</summary><pre className="max-h-64 overflow-auto rounded bg-slate-950 p-2 text-[10px] text-slate-300">{check.cap}</pre></details>
          </div>
        )}
      </div>
      {confirming && check && (
        <Modal open onClose={() => setConfirming(false)} title={d.status === 'Actual' ? 'Send a real public alert?' : `Send a ${d.status.toLowerCase()} alert?`}
          icon={<Megaphone size={16} className={d.status === 'Actual' ? 'text-rose-300' : 'text-amber-300'} aria-hidden />}
          footer={<><button className="btn" onClick={() => setConfirming(false)}>Not yet</button><button className={cx('btn', d.status === 'Actual' ? 'btn-go' : 'border-amber-400 bg-amber-500/20 text-amber-100')} disabled={busy} onClick={send}><Send size={13} aria-hidden /> {check.twoPerson ? 'Send for approval' : d.status === 'Actual' ? 'Send to the public' : `Send the ${d.status.toLowerCase()}`}</button></>}>
          <div className="space-y-2 text-sm">
            <p><span className={cx('chip', statusChip(d.status))}>{d.status === 'Actual' ? 'REAL' : d.status.toUpperCase()}</span> <b>{events.find((e) => e.code === d.event)?.name}</b></p>
            <p className="text-slate-100">{d.text.headline}</p>
            <p className="rounded bg-slate-900 p-2 font-mono text-xs">{d.text.short}</p>
            <p className="text-slate-400">{d.areaKind === 'whole' ? settings?.jurisdiction || 'The whole jurisdiction' : d.desc || autoDesc} · {check.reach} people · {d.receivers.length} systems · {d.hours} h</p>
          </div>
        </Modal>
      )}
    </div>
  )
}

/** The areas of alerts in force, for the map. */
export function alertAreas(alerts: PublicAlertBrief[] | undefined): GeoJSON.Feature[] {
  return (alerts ?? []).filter((a) => a.state === 'issued' && (a.area.polygon?.length ?? 0) >= 3).map((a) => ({
    type: 'Feature', properties: { name: a.headline, real: a.status === 'Actual' ? 1 : 0 }, geometry: { type: 'Polygon', coordinates: [[...a.area.polygon!, a.area.polygon![0]]] },
  }))
}
