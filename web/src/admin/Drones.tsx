import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Plane, Plus, Save, Trash2 } from 'lucide-react'
import { api, errText } from '../api'
import { Field } from '../ui'
import { cx } from '../util'
import { Page } from './Admin'

type DroneConf = { id: string; name: string; mavlink: string; signingKey?: string; systemId?: number; video?: string; altM: number; minBattery: number; approval: boolean; pilots?: string[]; record: boolean; off?: boolean }
type Telemetry = { connected: boolean; autopilot?: string; armed: boolean; havePos: boolean; lat: number; lon: number; altRel: number; battery: number; landed?: string }
type DronesView = { drones: DroneConf[]; telemetry: Record<string, Telemetry> }

const blank = (n: number): DroneConf => ({ id: `D${n}`, name: '', mavlink: 'udp-listen:0.0.0.0:14550', altM: 60, minBattery: 40, approval: true, record: true })

/** Drones bitdispatcher flies itself, over MAVLink, with their pictures relayed and recorded. */
export default function Drones() {
  const [d, setD] = useState<DroneConf[] | null>(null)
  const [tel, setTel] = useState<Record<string, Telemetry>>({})
  const load = () => api.get<DronesView>('/admin/drones').then((x) => { setD(x.drones); setTel(x.telemetry) }).catch((e) => toast.error(errText(e)))
  useEffect(() => { load() }, [])
  useEffect(() => {
    const t = setInterval(() => api.get<DronesView>('/admin/drones').then((x) => setTel(x.telemetry)).catch(() => {}), 2000)
    return () => clearInterval(t)
  }, [])
  if (!d) return <p className="text-sm text-slate-500">Loading…</p>
  const up = (i: number, p: Partial<DroneConf>) => setD(d.map((x, j) => (j === i ? { ...x, ...p } : x)))
  const save = async () => {
    try {
      const x = await api.put<DronesView>('/admin/drones', { drones: d })
      setD(x.drones)
      setTel(x.telemetry)
      toast.success('Saved')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Page
      title="Drones"
      intro="Drones bitdispatcher flies itself. It talks to each drone's autopilot (PX4 or ArduPilot) over MAVLink and takes its camera's picture over RTSP. A dispatcher asks for a drone on an incident; its remote pilot approves the launch; bitdispatcher flies it there, shows the live picture on the incident, records it for the incident with checkpoints in the ledger, and brings it home when told. The pilot keeps the drone's own controller and can take over at any time. Flying for an agency needs a Part 107 remote pilot, Remote ID, and an FAA waiver to fly beyond the pilot's sight."
      actions={<>
        <button className="btn" onClick={() => setD([...d, blank(d.length + 1)])}><Plus size={14} aria-hidden /> Add a drone</button>
        <button className="btn btn-go" onClick={save}><Save size={14} aria-hidden /> Save</button>
      </>}
    >
      {d.length === 0 && <p className="text-sm text-slate-400">No drones yet. To try this without a drone, run <span className="font-mono">dronesim</span> on this server and add a drone with the MAVLink address <span className="font-mono">udp-listen:127.0.0.1:14550</span> and the video <span className="font-mono">rtsp://127.0.0.1:8654/camera</span>.</p>}
      <div className="grid gap-4 xl:grid-cols-2">
        {d.map((c, i) => {
          const t = tel[c.id]
          return (
            <section key={i} aria-label={`Drone ${c.id}`} className="panel space-y-2 rounded-lg p-3">
              <h2 className="flex items-center gap-2 text-sm font-semibold text-slate-100">
                <Plane size={15} aria-hidden /> {c.name || c.id}
                <span className={cx('chip ml-1', t?.connected ? 'bg-emerald-500/20 text-emerald-200' : 'bg-slate-700 text-slate-300')}>
                  {c.off ? 'out of service' : t?.connected ? `connected${t.autopilot ? ', ' + t.autopilot : ''}` : 'not heard'}
                </span>
                {t?.connected && <span className="font-mono text-[11px] text-slate-400">{t.havePos ? `${t.lat.toFixed(5)}, ${t.lon.toFixed(5)}` : 'no position fix'}{t.battery >= 0 ? ` · battery ${t.battery}%` : ''}{t.landed ? ` · ${t.landed}` : ''}</span>}
                <button className="btn btn-ghost ml-auto h-7 px-1.5" aria-label={`Remove ${c.id}`} onClick={() => setD(d.filter((_, j) => j !== i))}><Trash2 size={14} aria-hidden /></button>
              </h2>
              <div className="grid gap-2 sm:grid-cols-2">
                <Field label="Id" htmlFor={`di${i}`} hint="Short, on the map: D1"><input id={`di${i}`} className="input font-mono uppercase" value={c.id} onChange={(e) => up(i, { id: e.target.value.toUpperCase() })} /></Field>
                <Field label="Name" htmlFor={`dn${i}`}><input id={`dn${i}`} className="input" value={c.name} onChange={(e) => up(i, { name: e.target.value })} placeholder="North station drone" /></Field>
              </div>
              <Field label="MAVLink address" htmlFor={`dm${i}`} hint="udp-listen:0.0.0.0:14550 waits for the drone or its radio to send; also udp:host:port, tcp:host:port, serial:/dev/ttyUSB0:57600">
                <input id={`dm${i}`} className="input font-mono" value={c.mavlink} onChange={(e) => up(i, { mavlink: e.target.value })} />
              </Field>
              <div className="grid gap-2 sm:grid-cols-[1fr_8rem]">
                <Field label="MAVLink 2 signing key" htmlFor={`dk${i}`} hint="64 hexadecimal digits, the same as set on the drone; without one, anyone who can reach the address could command the drone">
                  <input id={`dk${i}`} className="input font-mono" value={c.signingKey ?? ''} onChange={(e) => up(i, { signingKey: e.target.value })} />
                </Field>
                <Field label="System id" htmlFor={`ds${i}`} hint="0: first heard"><input id={`ds${i}`} type="number" min={0} max={255} className="input" value={c.systemId ?? 0} onChange={(e) => up(i, { systemId: Number(e.target.value) })} /></Field>
              </div>
              <Field label="Camera (RTSP)" htmlFor={`dv${i}`} hint="H.264 over RTSP, such as rtsp://user:password@camera:554/stream; empty for none">
                <input id={`dv${i}`} className="input font-mono" value={c.video ?? ''} onChange={(e) => up(i, { video: e.target.value })} />
              </Field>
              <div className="grid gap-2 sm:grid-cols-2">
                <Field label="Flying height, metres" htmlFor={`da${i}`} hint="Above the take-off point; 120 m is 400 feet"><input id={`da${i}`} type="number" min={10} max={120} className="input" value={c.altM} onChange={(e) => up(i, { altM: Number(e.target.value) })} /></Field>
                <Field label="Lowest battery to send it, %" htmlFor={`db${i}`}><input id={`db${i}`} type="number" min={10} max={95} className="input" value={c.minBattery} onChange={(e) => up(i, { minBattery: Number(e.target.value) })} /></Field>
              </div>
              <Field label="Remote pilots" htmlFor={`dp${i}`} hint="Usernames, separated by commas, who approve its launches; empty: supervisors">
                <input id={`dp${i}`} className="input font-mono" value={(c.pilots ?? []).join(', ')} onChange={(e) => up(i, { pilots: e.target.value.split(',').map((s) => s.trim()).filter(Boolean) })} />
              </Field>
              <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={c.approval} onChange={(e) => up(i, { approval: e.target.checked })} /> A remote pilot approves each launch (launches by the launch rule always wait for one)</label>
              <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={c.record} onChange={(e) => up(i, { record: e.target.checked })} /> Record the picture for the incident</label>
              <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={!!c.off} onChange={(e) => up(i, { off: e.target.checked })} /> Out of service</label>
            </section>
          )
        })}
      </div>
    </Page>
  )
}
