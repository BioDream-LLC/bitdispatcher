import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Camera as CameraIcon, Copy, ExternalLink, LocateFixed, ShieldAlert, Siren } from 'lucide-react'
import { api, errText } from '../api'
import { can, useStore } from '../store'
import { Field, Modal } from '../ui'
import { ago, cx } from '../util'
import type { Call, CameraNear, Incident, View } from '../types'

/** LocationLinkBox sends the caller a link that shares their phone's location with the call. */
export function LocationLinkBox({ call, view, now }: { call: Call; view: View; now: number }) {
  const me = useStore((s) => s.me)
  const [busy, setBusy] = useState(false)
  const [made, setMade] = useState<{ url: string; sent: string } | null>(null)
  const links = view.next?.locLinks[call.id] ?? []
  const live = links.find((l) => !l.ended && Date.parse(l.expires) > now)
  if (!can.dispatch(me) || call.state === 'ended') return null
  const textOK = view.health.bitvoice.configured && (call.sources.includes('bitvoice') || call.sources.includes('eido'))
  const send = async () => {
    setBusy(true)
    try {
      const r = await api.post<{ url: string; sent: string; public: boolean }>(`/calls/${encodeURIComponent(call.id)}/location-link`, { send: textOK })
      setMade(r)
      if (!r.public) toast.warning('No public address is set (Admin › Paging groups): the link uses this console’s address, which the caller’s phone may not reach')
      toast.success(`Location link ${r.sent}`)
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  const end = async () => {
    if (!live) return
    try {
      await api.post(`/location-links/${live.id}/end`)
      setMade(null)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const shared = call.location?.source === 'caller-link'
  return (
    <div className="flex flex-wrap items-center gap-1 text-[11px]">
      <button className="btn h-6 px-2 py-0 text-[11px]" disabled={busy} onClick={send} title="The caller opens it and presses Share: their phone's own location comes to this call, again as they move">
        <LocateFixed size={12} aria-hidden /> {live ? 'Send another location link' : 'Send location link'}
      </button>
      {live && (
        <span className="text-slate-400">
          link {live.sent ?? 'made'} {ago(live.at, now)} ago · ends in {Math.max(0, Math.round((Date.parse(live.expires) - now) / 60000))} min
          <button className="ml-1 text-sky-300 hover:underline" onClick={end}>end it</button>
        </span>
      )}
      {shared && <span className="chip bg-emerald-500/20 text-emerald-200">shared by the caller</span>}
      {made && !made.sent.startsWith('texted') && (
        <span className="flex items-center gap-1 rounded bg-slate-900 px-1 font-mono text-[10px] text-slate-200">
          {made.url}
          <button aria-label="Copy the link" onClick={() => navigator.clipboard?.writeText(made.url)}><Copy size={10} aria-hidden /></button>
        </span>
      )}
    </div>
  )
}

/** CamerasBox lists the agency's cameras near the incident; opening one asks why and records it. */
export function CamerasBox({ inc, view }: { inc: Incident; view: View }) {
  const me = useStore((s) => s.me)
  const [items, setItems] = useState<CameraNear[] | null>(null)
  const [open, setOpen] = useState<CameraNear | null>(null)
  const on = !!view.next?.cameras
  const lat = inc.location?.lat
  const lon = inc.location?.lon
  useEffect(() => {
    if (!on || lat == null || lon == null || inc.status === 'closed') return setItems(null)
    let live = true
    api.get<{ items: CameraNear[] }>(`/cameras/near?incident=${encodeURIComponent(inc.number)}`).then((r) => live && setItems(r.items)).catch(() => live && setItems(null))
    return () => {
      live = false
    }
  }, [on, lat, lon, inc.number, inc.status])
  if (!on || !can.dispatch(me) || !items?.length) return null
  return (
    <section aria-label="Cameras near" className="rounded-md border border-slate-800 p-2 text-xs">
      <div className="mb-1 flex items-center gap-1 font-semibold text-slate-200"><CameraIcon size={12} aria-hidden /> Cameras near <span className="font-normal text-slate-500">· opening one is recorded with why</span></div>
      <ul className="space-y-0.5">
        {items.slice(0, 6).map((c) => (
          <li key={c.id} className="flex items-center gap-1">
            <button className="text-left text-sky-300 hover:underline" onClick={() => setOpen(c)}>{c.name}</button>
            <span className="text-[10px] text-slate-500">{c.owner ? c.owner + ' · ' : ''}{Math.round(c.metres)} m{c.facing === true ? ' · looks this way' : c.facing === false ? ' · looks away' : ''}</span>
          </li>
        ))}
      </ul>
      <CameraDialog cam={open} incident={inc.number} onClose={() => setOpen(null)} />
    </section>
  )
}

function CameraDialog({ cam, incident, onClose }: { cam: CameraNear | null; incident: string; onClose: () => void }) {
  const [purpose, setPurpose] = useState('')
  const [opened, setOpened] = useState<{ live: string; still: boolean } | null>(null)
  const [tick, setTick] = useState(0)
  useEffect(() => {
    setOpened(null)
    setPurpose('')
  }, [cam])
  useEffect(() => {
    if (!opened?.still) return
    const t = setInterval(() => setTick((x) => x + 1), 5000)
    return () => clearInterval(t)
  }, [opened])
  const go = async () => {
    if (!cam) return
    try {
      setOpened(await api.post<{ live: string; still: boolean }>(`/cameras/${cam.id}/open`, { incident, purpose }))
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Modal open={!!cam} onClose={onClose} title={cam?.name ?? ''} icon={<CameraIcon size={16} aria-hidden />} wide>
      {cam && !opened && (
        <div className="space-y-3 text-sm">
          <p className="text-xs text-slate-400">Cameras are for the call in front of you. Opening this one is recorded: who, when and why. bitdispatcher keeps no video.</p>
          <Field label={`Why (for ${incident}, or a reason)`} htmlFor="cam-why"><input id="cam-why" className="input" value={purpose} onChange={(e) => setPurpose(e.target.value)} placeholder={`incident ${incident}`} maxLength={300} /></Field>
          <button className="btn btn-go" onClick={go}>Open the camera</button>
        </div>
      )}
      {cam && opened && (
        <div className="space-y-2">
          {opened.still && <img src={`./api/cameras/${cam.id}/still?t=${tick}`} alt={`What ${cam.name} sees now`} className="w-full rounded bg-black" />}
          {opened.live && <a className="btn" href={opened.live} target="_blank" rel="noreferrer noopener"><ExternalLink size={13} aria-hidden /> Open its live view (in the camera system)</a>}
          <p className="text-[11px] text-slate-500">The picture refreshes every 5 seconds while this is open.</p>
        </div>
      )}
    </Modal>
  )
}

const ALARM_EVENTS = ['burglary', 'holdup', 'panic', 'fire', 'smoke', 'carbon monoxide', 'medical', 'duress', 'other']

/** PhonedAlarm is the alarm card a call-taker fills while an alarm company's operator is on the phone. */
export function PhonedAlarm({ open, onClose }: { open: boolean; onClose: () => void }) {
  const blank = { sender: '', operator: '', callback: '', id: '', event: 'burglary', zone: '', premise: '', address: '', permit: '', account: '', keyholder: '', verified: false, detail: '' }
  const [a, setA] = useState(blank)
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    if (open) setA(blank)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])
  const set = (k: keyof typeof blank) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => setA({ ...a, [k]: e.target.type === 'checkbox' ? (e.target as HTMLInputElement).checked : e.target.value })
  const save = async () => {
    setBusy(true)
    try {
      await api.post('/alarms/phoned', a)
      toast.success('Alarm in the queue, with its permit and false-alarm checks')
      onClose()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  const input = (k: keyof typeof blank, label: string, ph = '') => (
    <Field label={label} htmlFor={'pa-' + k}><input id={'pa-' + k} className="input" value={a[k] as string} onChange={set(k)} placeholder={ph} /></Field>
  )
  return (
    <Modal open={open} onClose={onClose} title="Alarm company on the phone" icon={<Siren size={16} className="text-amber-300" aria-hidden />} wide
      footer={<><button className="btn" onClick={onClose}>Cancel</button><button className="btn btn-go" disabled={busy || !a.sender.trim() || !a.address.trim() || !a.event} onClick={save}>Put the alarm in the queue</button></>}>
      <div className="grid grid-cols-2 gap-x-3 gap-y-2 text-sm">
        {input('sender', 'Alarm company', 'Acme Monitoring')}
        {input('operator', 'Operator', 'name or number')}
        {input('callback', 'Their callback number')}
        {input('id', 'Their event number')}
        <Field label="Alarm" htmlFor="pa-event">
          <select id="pa-event" className="input" value={a.event} onChange={set('event')}>{ALARM_EVENTS.map((x) => <option key={x}>{x}</option>)}</select>
        </Field>
        {input('zone', 'Zone or sensor', '3 rear door')}
        {input('premise', 'Premise', 'Main Street Pharmacy')}
        {input('address', 'Address')}
        {input('permit', 'Permit number')}
        {input('account', 'Account')}
        {input('keyholder', 'Keyholder and how to reach them')}
        {input('detail', 'Anything else')}
        <label className={cx('col-span-2 flex items-center gap-2 text-xs text-slate-300')}><input type="checkbox" checked={a.verified} onChange={set('verified')} /> <ShieldAlert size={12} aria-hidden /> Verified by the company (video, audio or a person on site)</label>
      </div>
    </Modal>
  )
}
