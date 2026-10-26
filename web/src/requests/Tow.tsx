import { useCallback, useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Phone, Truck } from 'lucide-react'
import { api, errText } from '../api'
import { Modal } from '../ui'
import { cx, hhmm, phone as fmtPhone } from '../util'
import type { IncidentTows, Tow, TowAsk, TowDetail } from './types'

const STEP_LABELS: Record<string, string> = {
  requested: 'Requested',
  accepted: 'Accepted',
  dispatched: 'Dispatched',
  enroute: 'En route',
  arrived: 'Arrived',
  hooked: 'Hooked',
  transport: 'Transport',
  completed: 'Completed',
  cancelled: 'Cancelled',
  refused: 'Refused',
  reassigned: 'Reassigned',
}

const RESULT_WORDS: Record<string, string> = {
  called: 'called, awaiting answer',
  accepted: 'accepted',
  declined: 'declined (lost its turn)',
  'no-answer': 'did not answer (lost its turn)',
  cancelled: 'cancelled',
  arrived: 'arrived',
}

const openTow = (t: Tow) => t.result === 'called' || t.result === 'accepted' || t.result === 'arrived'

/** TowDialog asks for and tracks a tow for one vehicle on an incident. Usable from the vehicle record and
 * from the SE.<unit> F10 command. If no vehicle is given it requires choosing one of the incident's
 * vehicles. */
export function TowDialog({ incident, vehicle, requester, open, onClose }: {
  incident: string
  vehicle?: string
  requester?: string
  open: boolean
  onClose: () => void
}) {
  const [data, setData] = useState<IncidentTows | null>(null)
  const [vehId, setVehId] = useState(vehicle ?? '')
  const [cls, setCls] = useState('')
  const [zone, setZone] = useState('')
  const [reqUnit, setReqUnit] = useState(requester ?? '')
  const [reason, setReason] = useState('')
  const [requestNo, setRequestNo] = useState('')
  const [kind, setKind] = useState<'rotation' | 'owner' | 'police'>('rotation')
  const [company, setCompany] = useState('')
  const [override, setOverride] = useState('')
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      const q = new URLSearchParams()
      if (cls) q.set('class', cls)
      if (zone) q.set('zone', zone)
      const r = await api.get<IncidentTows>(`/incidents/${incident}/tows?${q}`)
      setData(r)
      if (!cls && r.classes.length) setCls(r.classes[0])
    } catch (e) {
      toast.error(errText(e))
    }
  }, [incident, cls, zone])

  useEffect(() => {
    if (open) {
      setVehId(vehicle ?? '')
      setReqUnit(requester ?? '')
      load()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, vehicle, requester])

  useEffect(() => {
    if (open) load()
  }, [open, cls, zone, load])

  const vehicles = data?.vehicles.filter((v) => !v.removed) ?? []
  const mustChoose = vehicles.length > 0
  const chosen = vehId
  const tows: Tow[] = chosen ? data?.byVehicle[chosen.split(' ')[0]] ?? data?.byVehicle[chosen] ?? [] : []
  const currentOpen = tows.find(openTow)
  const rotation = data?.rotation ?? []
  const nextCompany = rotation[0]

  const ask = async () => {
    if (mustChoose && !chosen) {
      toast.error('Choose which vehicle on the call is towed')
      return
    }
    if (kind === 'police' && !override.trim()) {
      toast.error('Say why the rotation is not followed')
      return
    }
    setBusy(true)
    try {
      const body: TowAsk = {
        incident,
        vehicle: chosen.split(' ')[0] || chosen,
        requester: reqUnit.trim() || undefined,
        reason: reason.trim() || undefined,
        requestNo: requestNo.trim() || undefined,
        class: cls,
        zone: zone || undefined,
        kind,
        company: kind === 'rotation' ? undefined : company || undefined,
        override: kind === 'police' ? override.trim() : undefined,
      }
      const t = await api.post<Tow>('/tows/vehicle', body)
      toast.success(`Tow ${t.id}: ${t.name} called ${fmtPhone(t.phone)}`)
      setOverride('')
      await load()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }

  const answer = async (id: string, result: 'accepted' | 'declined' | 'no-answer', note?: string) => {
    try {
      await api.post(`/tows/${id}/answer`, { result, note: note ?? undefined })
      toast.success(`Recorded: ${RESULT_WORDS[result] ?? result}`)
      await load()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const step = async (id: string, s: string) => {
    try {
      await api.post(`/tows/${id}/detail`, { id, step: s } as TowDetail)
      toast.success(`${STEP_LABELS[s] ?? s} recorded`)
      await load()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  return (
    <Modal open={open} onClose={onClose} xl title="Tow" icon={<Truck size={16} aria-hidden />}>
      <div className="space-y-4 text-sm">
        {mustChoose && (
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
            Vehicle on the call
            <select className="input h-8 text-sm" value={chosen} onChange={(e) => setVehId(e.target.value)}>
              <option value="">Choose the vehicle to tow</option>
              {vehicles.map((v) => (
                <option key={v.id} value={v.id}>
                  {[v.id, v.state, v.plate, v.year, v.make, v.model].filter(Boolean).join(' ')}
                </option>
              ))}
            </select>
          </label>
        )}

        <section className="space-y-2 rounded-md border border-slate-800 p-3" aria-label="Ask for a tow">
          <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
            <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
              Requester unit
              <input className="input h-8 text-sm" value={reqUnit} onChange={(e) => setReqUnit(e.target.value)} placeholder="unit or officer" />
            </label>
            <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
              Request number
              <input className="input h-8 text-sm" value={requestNo} onChange={(e) => setRequestNo(e.target.value)} />
            </label>
            <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
              Class
              <select className="input h-8 text-sm" value={cls} onChange={(e) => setCls(e.target.value)}>
                {(data?.classes ?? []).map((c) => (
                  <option key={c} value={c}>
                    {c}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
              Zone
              <select className="input h-8 text-sm" value={zone} onChange={(e) => setZone(e.target.value)} disabled={!(data?.zones?.length)}>
                <option value="">{data?.zones?.length ? 'Any zone' : 'No zones'}</option>
                {(data?.zones ?? []).map((z) => (
                  <option key={z} value={z}>
                    {z}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
            Reason
            <input className="input h-8 text-sm" value={reason} onChange={(e) => setReason(e.target.value)} placeholder="abandoned, crash, recovery..." />
          </label>

          <fieldset>
            <legend className="mb-1 text-[11px] text-slate-400">How the company is chosen</legend>
            <div className="flex flex-wrap gap-1" role="radiogroup" aria-label="How the company is chosen">
              {([['rotation', 'Ask next company (rotation)'], ['owner', "Owner's choice"], ['police', "Officer's choice"]] as const).map(([v, lbl]) => (
                <button
                  key={v}
                  type="button"
                  role="radio"
                  aria-checked={kind === v}
                  className={cx('chip h-7 cursor-pointer px-2 text-xs', kind === v ? 'bg-sky-500 text-white' : 'bg-slate-800 text-slate-300')}
                  onClick={() => setKind(v)}
                >
                  {lbl}
                </button>
              ))}
            </div>
          </fieldset>

          {kind === 'rotation' ? (
            <div className="rounded border border-slate-800 bg-slate-900/60 p-2">
              <div className="mb-1 text-[11px] font-semibold text-slate-300">Rotation (next due at the top)</div>
              {rotation.length === 0 && <p className="text-[11px] text-slate-500">No company on the rotation tows {cls || 'this class'}{zone ? ' in ' + zone : ''}.</p>}
              <ul className="space-y-0.5">
                {rotation.map((t, i) => (
                  <li key={t.company.id} className={cx('flex items-center gap-2 rounded px-1.5 py-1 text-xs', i === 0 && 'bg-emerald-500/10')}>
                    {i === 0 && <span className="chip h-4 bg-emerald-500/30 px-1 text-[9px] text-emerald-100">NEXT</span>}
                    <span className="font-semibold text-slate-100">{t.company.name}</span>
                    <span className="font-mono text-slate-400">{fmtPhone(t.company.phone)}</span>
                    <span className="ml-auto text-[10px] text-slate-500">last turn {t.lastTurn ? hhmm(t.lastTurn) : 'never'}</span>
                  </li>
                ))}
              </ul>
            </div>
          ) : (
            <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
              Company
              <select className="input h-8 text-sm" value={company} onChange={(e) => setCompany(e.target.value)}>
                <option value="">Choose the company</option>
                {rotation.map((t) => (
                  <option key={t.company.id} value={t.company.id}>
                    {t.company.name} ({fmtPhone(t.company.phone)})
                  </option>
                ))}
              </select>
            </label>
          )}

          {kind === 'police' && (
            <label className="flex flex-col gap-0.5 text-[11px] text-slate-400">
              Override reason (why the rotation is not followed)
              <input className="input h-8 text-sm" value={override} onChange={(e) => setOverride(e.target.value)} />
            </label>
          )}

          <div className="flex items-center justify-end gap-2">
            {kind === 'rotation' && nextCompany && (
              <span className="text-[11px] text-slate-400">
                Next: <b className="text-slate-200">{nextCompany.company.name}</b> {fmtPhone(nextCompany.company.phone)}
              </span>
            )}
            <button className="btn btn-go h-8 text-sm" onClick={ask} disabled={busy}>
              <Phone size={13} aria-hidden /> {kind === 'rotation' ? 'Ask next company' : 'Call this company'}
            </button>
          </div>
        </section>

        {currentOpen && (
          <section className="space-y-2 rounded-md border border-amber-800 bg-amber-950/30 p-3" aria-label="Current tow">
            <div className="text-[11px] font-semibold text-amber-100">
              Current tow {currentOpen.id}: {currentOpen.name} {fmtPhone(currentOpen.phone)} — {RESULT_WORDS[currentOpen.result] ?? currentOpen.result}
            </div>
            {currentOpen.result === 'called' && (
              <div className="flex flex-wrap gap-1">
                <button className="btn btn-go h-7 text-xs" onClick={() => answer(currentOpen.id, 'accepted')}>
                  Accepted
                </button>
                <button
                  className="btn h-7 text-xs"
                  onClick={() => {
                    const why = prompt('Why did it decline? (optional)') ?? ''
                    answer(currentOpen.id, 'declined', why)
                  }}
                >
                  Declined
                </button>
                <button className="btn h-7 text-xs" onClick={() => answer(currentOpen.id, 'no-answer')}>
                  No answer
                </button>
                <span className="ml-1 self-center text-[10px] text-amber-200/80">A decline or no answer loses its turn; ask the next company above.</span>
              </div>
            )}
            {(currentOpen.result === 'accepted' || currentOpen.result === 'arrived') && (
              <>
                <div className="flex flex-wrap gap-1">
                  {['dispatched', 'enroute', 'arrived', 'hooked', 'transport', 'completed'].map((s) => (
                    <button key={s} className="btn h-7 text-xs" onClick={() => step(currentOpen.id, s)} title={`Record ${STEP_LABELS[s]} time`}>
                      {STEP_LABELS[s]}
                    </button>
                  ))}
                </div>
                <TowDetailForm tow={currentOpen} onSaved={load} />
              </>
            )}
          </section>
        )}

        {chosen && tows.length > 0 && (
          <section aria-label="Tow sequence">
            <h3 className="mb-1 text-[11px] font-bold uppercase tracking-wider text-slate-400">Tow sequence for {chosen.split(' ')[0]}</h3>
            <ul className="space-y-1">
              {tows.map((t) => (
                <li key={t.id} className="rounded border border-slate-800 bg-slate-900/60 px-2 py-1.5 text-xs">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-slate-300">{t.id}</span>
                    <span className="font-semibold text-slate-100">{t.name}</span>
                    <span className="font-mono text-slate-400">{fmtPhone(t.phone)}</span>
                    <span className="chip h-4 bg-slate-800 px-1 text-[10px] text-slate-300">{t.kind}</span>
                    <span className="ml-auto text-slate-400">{RESULT_WORDS[t.result] ?? t.result}</span>
                  </div>
                  <div className="mt-0.5 text-[10px] text-slate-500">
                    called {hhmm(t.at)}
                    {t.times
                      ? ' · ' +
                        Object.entries(t.times)
                          .map(([k, v]) => `${STEP_LABELS[k] ?? k} ${hhmm(v)}`)
                          .join(' · ')
                      : ''}
                  </div>
                  {(t.driver || t.truck || t.destination || t.impound || t.hold || t.release) && (
                    <div className="mt-0.5 text-[11px] text-slate-300">
                      {[t.driver && `driver ${t.driver}`, t.truck && `truck ${t.truck}`, t.destination && `to ${t.destination}`, t.impound && `impound ${t.impound}`, t.hold && `hold ${t.hold}`, t.release && `release ${t.release}`]
                        .filter(Boolean)
                        .join(' · ')}
                    </div>
                  )}
                  {t.skipped && t.skipped.length > 0 && <div className="mt-0.5 text-[10px] text-slate-500">passed over: {t.skipped.join('; ')}</div>}
                  {t.note && <div className="mt-0.5 text-[11px] text-slate-400">{t.note}</div>}
                </li>
              ))}
            </ul>
          </section>
        )}
      </div>
    </Modal>
  )
}

function TowDetailForm({ tow, onSaved }: { tow: Tow; onSaved: () => void }) {
  const [driver, setDriver] = useState(tow.driver ?? '')
  const [truck, setTruck] = useState(tow.truck ?? '')
  const [destination, setDestination] = useState(tow.destination ?? '')
  const [impound, setImpound] = useState(tow.impound ?? '')
  const [hold, setHold] = useState(tow.hold ?? '')
  const [release, setRelease] = useState(tow.release ?? '')
  const [saving, setSaving] = useState(false)
  const save = async () => {
    setSaving(true)
    try {
      await api.post(`/tows/${tow.id}/detail`, {
        id: tow.id,
        driver: driver.trim() || undefined,
        truck: truck.trim() || undefined,
        destination: destination.trim() || undefined,
        impound: impound.trim() || undefined,
        hold: hold.trim() || undefined,
        release: release.trim() || undefined,
      } as TowDetail)
      toast.success('Tow details saved')
      onSaved()
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setSaving(false)
    }
  }
  return (
    <div className="grid grid-cols-2 gap-2 lg:grid-cols-3">
      {[
        ['Driver', driver, setDriver],
        ['Truck', truck, setTruck],
        ['Destination / storage lot', destination, setDestination],
        ['Impound number', impound, setImpound],
        ['Hold', hold, setHold],
        ['Release restriction', release, setRelease],
      ].map(([label, val, set]) => (
        <label key={label as string} className="flex flex-col gap-0.5 text-[11px] text-slate-400">
          {label as string}
          <input className="input h-7 text-xs" value={val as string} onChange={(e) => (set as (v: string) => void)(e.target.value)} />
        </label>
      ))}
      <div className="col-span-2 flex justify-end lg:col-span-3">
        <button className="btn h-7 text-xs" onClick={save} disabled={saving}>
          Save tow details
        </button>
      </div>
    </div>
  )
}
