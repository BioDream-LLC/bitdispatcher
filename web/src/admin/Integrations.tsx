import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { CheckCircle2, CircleDashed, Copy, Fingerprint, Images, KeyRound, PlugZap, Save, Server, XCircle } from 'lucide-react'
import { api, errText } from '../api'
import { Field } from '../ui'
import { ago, cx, useNow } from '../util'
import type { NERISConfig, View, ViewsConfig } from '../types'
import { Page } from './Admin'
import { NEMSISSettings, PagingSettings } from './RoadmapIntegrations'

type MV = { id: string; name: string; url: string; fingerprint: string; tokenSet: boolean; eidoTokenSet: boolean; radioSecretSet: boolean }
type IV = { bitvoiceUrl: string; bitvoiceFingerprint: string; bitvoiceTokenSet: boolean; eidoTokenSet: boolean; radioSecretSet: boolean; aliListen: string; more?: MV[] }

function Copyable({ text }: { text: string }) {
  return (
    <button
      type="button"
      className="inline-flex max-w-full items-center gap-1 rounded bg-slate-950 px-1.5 py-0.5 font-mono text-xs text-amber-300 hover:bg-slate-800"
      onClick={() => navigator.clipboard?.writeText(text).then(() => toast.success('Copied'), () => toast.error('Copy blocked: select the text'))}
      title="Copy"
    >
      <span className="truncate">{text}</span> <Copy size={11} aria-hidden />
    </button>
  )
}

function Check({ ok, wait, children }: { ok: boolean; wait?: boolean; children: React.ReactNode }) {
  return (
    <li className="flex items-start gap-2 text-xs">
      {ok ? <CheckCircle2 size={14} className="mt-px shrink-0 text-emerald-400" aria-label="working" /> : wait ? <CircleDashed size={14} className="mt-px shrink-0 text-slate-500" aria-label="not seen yet" /> : <XCircle size={14} className="mt-px shrink-0 text-rose-400" aria-label="not working" />}
      <span className={ok ? 'text-slate-200' : 'text-slate-400'}>{children}</span>
    </li>
  )
}

export default function Integrations({ view }: { view: View }) {
  const now = useNow()
  const [iv, setIv] = useState<IV | null>(null)
  const [url, setUrl] = useState('')
  const [fp, setFp] = useState('')
  const [token, setToken] = useState('')
  const [eidoToken, setEidoToken] = useState('')
  const [radioSecret, setRadioSecret] = useState('')
  const [ali, setAli] = useState('')
  const [fpInfo, setFpInfo] = useState('')
  const load = async () => {
    const r = await api.get<IV>('/admin/integrations')
    setIv(r)
    setUrl(r.bitvoiceUrl)
    setFp(r.bitvoiceFingerprint)
    setAli(r.aliListen)
  }
  useEffect(() => {
    load().catch((e) => toast.error(errText(e)))
  }, [])
  const save = async () => {
    try {
      const r = await api.put<IV>('/admin/integrations', { bitvoiceUrl: url, bitvoiceFingerprint: fp, bitvoiceToken: token, eidoToken, radioSecret, aliListen: ali })
      setIv(r)
      setToken('')
      setEidoToken('')
      setRadioSecret('')
      toast.success('Integrations saved')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const gen = async (set: (s: string) => void) => {
    const r = await api.post<{ secret: string }>('/admin/integrations/secret')
    set(r.secret)
  }
  const h = view.health.bitvoice
  const here = location.origin
  const recent = (t?: string, mins = 60) => !!t && now - new Date(t).getTime() < mins * 60000
  return (
    <Page
      title="Integrations"
      intro="Call handling, radio and location feeds. With BitVoice, bitdispatcher uses every hook it offers a CAD; without it, ALI spill and EIDO from any standards-compliant system work the same way. Secrets are write-only: they are never shown again."
      actions={<button className="btn btn-go" onClick={save}><Save size={14} aria-hidden /> Save</button>}
    >
      {!iv ? (
        <p className="text-sm text-slate-500">Loading…</p>
      ) : (
        <div className="grid gap-4 lg:grid-cols-[3fr_2fr]">
          <div className="space-y-4">
            <section className="panel space-y-3 rounded-lg p-4">
              <h2 className="flex items-center gap-2 font-semibold text-slate-100"><Server size={16} className="text-sky-300" aria-hidden /> BitVoice: bitdispatcher reads</h2>
              <p className="text-xs text-slate-400">The event stream (ringing and answered 911 calls, radio, delivery failures), calls in progress, radio positions, and the caller actions (rebid, text, video). Make an API token in BitVoice under Administrators › API tokens; viewer is enough to read, administrator is needed to acknowledge radio emergencies on its console. Tick <b>Names the person it acts for</b> so BitVoice's records (who looked at a caller's photos and video, who took a text call) name the dispatcher, not the token.</p>
              <Field label="BitVoice address" htmlFor="bu"><input id="bu" className="input font-mono" placeholder="https://bitvoice.example:8443" value={url} onChange={(e) => setUrl(e.target.value.trim())} /></Field>
              <Field label="API token" htmlFor="bt" hint={iv.bitvoiceTokenSet ? 'Set. Leave empty to keep it; - clears it.' : 'Not set'}>
                <input id="bt" type="password" autoComplete="off" className="input font-mono" placeholder={iv.bitvoiceTokenSet ? '••••••••' : 'bv_…'} value={token} onChange={(e) => setToken(e.target.value)} />
              </Field>
              <Field label="Certificate pin (SHA-256)" htmlFor="bf" hint="For BitVoice's self-signed certificate. Compare with the fingerprint BitVoice shows under Settings › Certificate before saving. Empty: trust the system's authorities.">
                <div className="flex gap-2">
                  <input id="bf" className="input font-mono text-xs" value={fp} onChange={(e) => setFp(e.target.value.trim())} />
                  <button
                    className="btn shrink-0"
                    disabled={!url}
                    onClick={async () => {
                      try {
                        const r = await api.post<{ fingerprint: string; subject: string }>('/admin/integrations/bitvoice/fingerprint', { url })
                        setFp(r.fingerprint)
                        setFpInfo(r.subject)
                      } catch (e) {
                        toast.error(errText(e))
                      }
                    }}
                  >
                    <Fingerprint size={13} aria-hidden /> Read it
                  </button>
                </div>
              </Field>
              {fpInfo && <p className="text-xs text-amber-300">Read from {fpInfo}. Check it matches BitVoice before you save.</p>}
              <button
                className="btn"
                onClick={async () => {
                  try {
                    const r = await api.post<{ detail: string }>('/admin/integrations/bitvoice/test')
                    toast.success(r.detail)
                  } catch (e) {
                    toast.error(errText(e))
                  }
                }}
              >
                <PlugZap size={13} aria-hidden /> Test the saved connection
              </button>
            </section>
            <section className="panel space-y-3 rounded-lg p-4">
              <h2 className="flex items-center gap-2 font-semibold text-slate-100"><KeyRound size={16} className="text-amber-300" aria-hidden /> BitVoice: what it sends to bitdispatcher</h2>
              <p className="text-xs text-slate-400">Put these in BitVoice's settings. BitVoice checks certificates, so it must trust this server's certificate (or reach it by a name with a public certificate); until then the read path above still delivers every call.</p>
              <div className="space-y-1 text-xs">
                <div><span className="text-slate-500">911 › EIDO address (psap.eidoUrl): </span><Copyable text={`${here}/api/ng911/eido`} /></div>
                <div><span className="text-slate-500">Radio › CAD address (radio.cadUrl): </span><Copyable text={`${here}/api/radio/events`} /></div>
              </div>
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label="EIDO token (psap.eidoToken)" htmlFor="et" hint={iv.eidoTokenSet ? 'Set. New value replaces it.' : 'Not set: EIDO pushes are refused'}>
                  <div className="flex gap-1">
                    <input id="et" className="input font-mono text-xs" value={eidoToken} onChange={(e) => setEidoToken(e.target.value)} autoComplete="off" />
                    <button className="btn shrink-0" onClick={() => gen(setEidoToken)}>New</button>
                  </div>
                </Field>
                <Field label="Radio secret (radio.cadSecret)" htmlFor="rs" hint={iv.radioSecretSet ? 'Set. New value replaces it.' : 'Not set: radio pushes are refused'}>
                  <div className="flex gap-1">
                    <input id="rs" className="input font-mono text-xs" value={radioSecret} onChange={(e) => setRadioSecret(e.target.value)} autoComplete="off" />
                    <button className="btn shrink-0" onClick={() => gen(setRadioSecret)}>New</button>
                  </div>
                </Field>
              </div>
              {(eidoToken || radioSecret) && <p className="text-xs text-amber-300">Copy the new values into BitVoice now, then Save here: they are not shown again.</p>}
            </section>
            <section className="panel space-y-3 rounded-lg p-4">
              <h2 className="font-semibold text-slate-100">ALI spill (any call-handling system)</h2>
              <p className="text-xs text-slate-400">The legacy text block CAD systems already read, over TCP. In BitVoice: 911 › ALI address (psap.aliAddress) = this server's address and the port below. A spill for a call BitVoice is already sending joins that call.</p>
              <Field label="Listen on" htmlFor="al" hint={view.health.ali.error ? `Failed: ${view.health.ali.error}` : view.health.ali.listening ? `Listening on ${view.health.ali.listening}` : 'Off'}>
                <input id="al" className="input w-60 font-mono" placeholder="0.0.0.0:5001" value={ali} onChange={(e) => setAli(e.target.value.trim())} />
              </Field>
              <button className="btn" disabled={!view.health.ali.listening} onClick={async () => { try { toast.success((await api.post<{ detail: string }>('/admin/integrations/ali/test')).detail) } catch (e) { toast.error(errText(e)) } }}>
                <PlugZap size={13} aria-hidden /> Send a test spill
              </button>
            </section>
          </div>
          <section className="panel h-fit space-y-2 rounded-lg p-4">
            <h2 className="font-semibold text-slate-100">Every hook, live</h2>
            <ul className="space-y-1.5">
              <Check ok={h.connected}>Event stream {h.connected ? `connected ${ago(h.since, now)} ago · ${h.events} events` : h.configured ? `not connected${h.lastError ? `: ${h.lastError}` : ''}` : 'not configured'}</Check>
              <Check ok={h.connected} wait={!h.connected}>Ringing and answered 911 calls with location, class, text and video (call.started/updated/ended)</Check>
              <Check ok={recent(h.lastEido, 24 * 60)} wait>EIDO pulled on answer and on new caller media {h.lastEido ? `· last ${ago(h.lastEido, now)} ago` : '· none yet'}</Check>
              <Check ok={recent(h.lastPush, 24 * 60)} wait>EIDO pushed by BitVoice {h.lastPush ? `· last ${ago(h.lastPush, now)} ago` : iv.eidoTokenSet ? '· none yet' : '· token not set'}</Check>
              <Check ok={recent(h.lastRadio, 24 * 60)} wait>Radio: unit, emergency button, acknowledgement, transmission, GPS {h.lastRadio ? `· last ${ago(h.lastRadio, now)} ago` : '· none yet'}</Check>
              <Check ok={recent(h.lastRadioWebhook, 24 * 60)} wait>Radio events pushed by BitVoice {h.lastRadioWebhook ? `· last ${ago(h.lastRadioWebhook, now)} ago` : iv.radioSecretSet ? '· none yet' : '· secret not set'}</Check>
              <Check ok={h.failures.length === 0}>Delivery failures BitVoice reports (emergency.cad_failed, radio.cad_failed): {h.failures.length}</Check>
              <Check ok={!!view.health.ali.listening} wait={!ali}>ALI spill listener {view.health.ali.listening ? `· ${view.health.ali.count} spills` : '· off'}</Check>
              <Check ok={h.configured} wait>Caller actions: rebid, text (TTY, RTT, MSRP, SIP), video link, live video (WHEP), photos, radio-console acknowledgement</Check>
              <Check ok={h.connected} wait>BitVoice emergency mode shown on every console {h.emergencyMode ? '· ON now' : ''}</Check>
            </ul>
            {h.failures.length > 0 && (
              <div className="mt-2 space-y-0.5 border-t border-slate-800 pt-2 text-xs text-amber-300">
                {h.failures.map((f, i) => <div key={i} className={cx()}>{ago(f.at, now)} ago · {f.what}{f.call ? ` · ${f.call}` : ''}: {f.error}</div>)}
              </div>
            )}
          </section>
        </div>
      )}
      {iv && <MoreBitVoice view={view} list={iv.more ?? []} reload={() => load().catch((e) => toast.error(errText(e)))} />}
      <Connections view={view} />
      <ViewsSettings />
      <NERISSettings />
      <NEMSISSettings />
      <PagingSettings />
      <LISSettings />
      <Partners />
      <PublicFeedSettings view={view} />
    </Page>
  )
}

const blankMore = { id: '', name: '', url: '', fingerprint: '', token: '', eidoToken: '', radioSecret: '' }

// More BitVoice systems: others followed besides the main one (a neighbouring PSAP's, a second site's).
// Each has its own link and push secrets; its calls show its name, and what a dispatcher does with
// one goes back to it.
function MoreBitVoice({ view, list, reload }: { view: View; list: MV[]; reload: () => void }) {
  const now = useNow()
  const [f, setF] = useState<typeof blankMore | null>(null)
  const set = (k: keyof typeof blankMore) => (e: React.ChangeEvent<HTMLInputElement>) => setF((o) => (o ? { ...o, [k]: e.target.value } : o))
  const health = new Map((view.health.moreBitvoice ?? []).map((m) => [m.id, m.health]))
  const here = location.origin
  const save = async () => {
    if (!f) return
    try {
      const body = { name: f.name, url: f.url, fingerprint: f.fingerprint, token: f.token, eidoToken: f.eidoToken, radioSecret: f.radioSecret }
      if (f.id) await api.put(`/admin/integrations/more/${encodeURIComponent(f.id)}`, body)
      else await api.post('/admin/integrations/more', body)
      toast.success(`${f.name} saved`)
      setF(null)
      reload()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const secret = async (k: 'eidoToken' | 'radioSecret') => {
    const r = await api.post<{ secret: string }>('/admin/integrations/secret')
    setF((o) => (o ? { ...o, [k]: r.secret } : o))
  }
  return (
    <section className="panel mt-4 space-y-3 rounded-lg p-4" aria-labelledby="morebv">
      <div className="flex items-center gap-2">
        <h2 id="morebv" className="flex items-center gap-2 font-semibold text-slate-100"><Server size={16} className="text-sky-300" aria-hidden /> More BitVoice systems</h2>
        {!f && <button className="btn ml-auto" onClick={() => setF({ ...blankMore })}>Add a BitVoice</button>}
      </div>
      <p className="text-xs text-slate-400">Follow another BitVoice besides the main one above, such as a neighbouring PSAP's. It sends to the same addresses ({here}/api/ng911/eido and /api/radio/events) with its own EIDO token and radio secret.</p>
      {list.length === 0 && !f && <p className="text-xs text-slate-500">None.</p>}
      <ul className="space-y-2">
        {list.map((m) => {
          const h = health.get(m.id)
          return (
            <li key={m.id} className="flex flex-wrap items-center gap-2 rounded border border-slate-800 p-2 text-xs">
              {h?.connected ? <CheckCircle2 size={14} className="text-emerald-400" aria-label="connected" /> : <XCircle size={14} className="text-rose-400" aria-label="not connected" />}
              <span className="font-semibold text-slate-100">{m.name}</span>
              <span className="font-mono text-slate-400">{m.url}</span>
              <span className="text-slate-400">
                {h?.connected ? `connected ${ago(h.since, now)} ago · ${h.events} events` : h?.lastError ? `not connected: ${h.lastError}` : 'not connected'}
                {h?.lastPush ? ` · EIDO pushed ${ago(h.lastPush, now)} ago` : ''}
              </span>
              <span className="ml-auto flex gap-1">
                <button className="btn" onClick={async () => { try { toast.success((await api.post<{ detail: string }>(`/admin/integrations/more/${encodeURIComponent(m.id)}/test`)).detail) } catch (e) { toast.error(errText(e)) } }}><PlugZap size={13} aria-hidden /> Test</button>
                <button className="btn" onClick={() => setF({ ...blankMore, id: m.id, name: m.name, url: m.url, fingerprint: m.fingerprint })}>Edit</button>
                <button className="btn" onClick={async () => { if (!confirm(`Stop following ${m.name}?`)) return; try { await api.del(`/admin/integrations/more/${encodeURIComponent(m.id)}`); reload() } catch (e) { toast.error(errText(e)) } }}>Remove</button>
              </span>
            </li>
          )
        })}
      </ul>
      {f && (
        <div className="space-y-3 rounded border border-slate-700 p-3">
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Name" htmlFor="mbn"><input id="mbn" className="input" placeholder="Neighbour 911" value={f.name} onChange={set('name')} /></Field>
            <Field label="BitVoice address" htmlFor="mbu"><input id="mbu" className="input font-mono" placeholder="https://bitvoice.example" value={f.url} onChange={set('url')} /></Field>
            {(() => {
              const m = list.find((x) => x.id === f.id)
              return (
                <>
                  <Field label="API token" htmlFor="mbt" hint={m?.tokenSet ? 'Set. Leave empty to keep it.' : 'Not set'}><input id="mbt" type="password" autoComplete="off" className="input font-mono" placeholder="bv_…" value={f.token} onChange={set('token')} /></Field>
                  <Field label="Certificate pin (SHA-256)" htmlFor="mbf" hint="Only for a self-signed certificate; empty trusts the system's authorities.">
                    <div className="flex gap-1">
                      <input id="mbf" className="input font-mono text-xs" value={f.fingerprint} onChange={set('fingerprint')} />
                      <button className="btn shrink-0" disabled={!f.url} onClick={async () => { try { const r = await api.post<{ fingerprint: string }>('/admin/integrations/bitvoice/fingerprint', { url: f.url }); setF((o) => (o ? { ...o, fingerprint: r.fingerprint } : o)) } catch (e) { toast.error(errText(e)) } }}><Fingerprint size={13} aria-hidden /> Read it</button>
                    </div>
                  </Field>
                  <Field label="EIDO token it sends" htmlFor="mbe" hint={m?.eidoTokenSet ? 'Set. New value replaces it.' : 'Not set: its EIDO pushes are refused'}>
                    <div className="flex gap-1"><input id="mbe" className="input font-mono text-xs" autoComplete="off" value={f.eidoToken} onChange={set('eidoToken')} /><button className="btn shrink-0" onClick={() => secret('eidoToken')}>New</button></div>
                  </Field>
                  <Field label="Radio secret it sends" htmlFor="mbr" hint={m?.radioSecretSet ? 'Set. New value replaces it.' : 'Not set: its radio pushes are refused'}>
                    <div className="flex gap-1"><input id="mbr" className="input font-mono text-xs" autoComplete="off" value={f.radioSecret} onChange={set('radioSecret')} /><button className="btn shrink-0" onClick={() => secret('radioSecret')}>New</button></div>
                  </Field>
                </>
              )
            })()}
          </div>
          {(f.eidoToken || f.radioSecret) && <p className="text-xs text-amber-300">Copy the new values into that BitVoice now, then Save: they are not shown again.</p>}
          <div className="flex gap-2">
            <button className="btn btn-go" onClick={save}><Save size={14} aria-hidden /> Save</button>
            <button className="btn" onClick={() => setF(null)}>Cancel</button>
          </div>
        </div>
      )}
    </section>
  )
}

type XV = { hookTokenSet: boolean; droneUrl: string; droneTokenSet: boolean; droneAuto: boolean; modelUrl: string; modelKeySet: boolean; modelName: string; modelOn: boolean; foiaModel?: boolean; alertAnswerUrl?: string; alertAnswerTokenSet?: boolean; stationAlerts?: boolean; stationGatewayUrl?: string; stationGatewayTokenSet?: boolean; clamav?: string; ocr?: boolean }

// Connections are the wishlist's: the hooks other systems send to (transcripts, crash data, drones, radio
// words), the drone system, and the optional language model for translation.
function Connections({ view }: { view: View }) {
  const [x, setX] = useState<XV | null>(null)
  const [hook, setHook] = useState('')
  const [droneUrl, setDroneUrl] = useState('')
  const [droneToken, setDroneToken] = useState('')
  const [droneAuto, setDroneAuto] = useState(false)
  const [modelUrl, setModelUrl] = useState('')
  const [modelKey, setModelKey] = useState('')
  const [modelName, setModelName] = useState('')
  const [modelOn, setModelOn] = useState(false)
  const [foiaModel, setFoiaModel] = useState(false)
  const [answerUrl, setAnswerUrl] = useState('')
  const [answerToken, setAnswerToken] = useState('')
  const [stations, setStations] = useState(false)
  const [gwUrl, setGwUrl] = useState('')
  const [gwToken, setGwToken] = useState('')
  const [clam, setClam] = useState('')
  const [ocr, setOcr] = useState(false)
  const [ocrInfo, setOcrInfo] = useState<{ installed: boolean; version: string } | null>(null)
  const [clamTest, setClamTest] = useState<{ ok?: boolean; version?: string; testFile?: string; clean?: boolean; error?: string } | null>(null)
  const fill = (v: XV) => {
    setClam(v.clamav ?? '')
    setOcr(!!v.ocr)
    setX(v)
    setDroneUrl(v.droneUrl)
    setDroneAuto(v.droneAuto)
    setModelUrl(v.modelUrl)
    setModelName(v.modelName)
    setModelOn(v.modelOn)
    setFoiaModel(!!v.foiaModel)
    setAnswerUrl(v.alertAnswerUrl ?? '')
    setStations(!!v.stationAlerts)
    setGwUrl(v.stationGatewayUrl ?? '')
  }
  useEffect(() => {
    api.get<XV>('/admin/connections').then(fill).catch((e) => toast.error(errText(e)))
    api.get<{ installed: boolean; version: string }>('/admin/connections/ocr').then(setOcrInfo).catch(() => {})
  }, [])
  const testClam = async () => {
    setClamTest(null)
    try {
      setClamTest(await api.post('/admin/connections/clamav/test', { address: clam }))
    } catch (e) {
      setClamTest({ error: errText(e) })
    }
  }
  const save = async () => {
    try {
      fill(await api.put<XV>('/admin/connections', { hookToken: hook, droneUrl, droneToken, droneAuto, modelUrl, modelKey, modelName, modelOn, foiaModel, alertAnswerUrl: answerUrl, alertAnswerToken: answerToken, stationAlerts: stations, stationGatewayUrl: gwUrl, stationGatewayToken: gwToken, clamav: clam, ocr }))
      setAnswerToken('')
      setGwToken('')
      setHook('')
      setDroneToken('')
      setModelKey('')
      toast.success('Connections saved')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  if (!x) return null
  const base = `${location.origin}/api/hooks`
  return (
    <div className="mt-4 grid gap-4 lg:grid-cols-2">
      <section className="panel space-y-3 rounded-lg p-4">
        <h2 className="flex items-center gap-2 font-semibold text-slate-100"><KeyRound size={15} aria-hidden /> Hooks: transcripts, crash data, drones, radio words</h2>
        <p className="text-xs text-slate-400">Other systems post here with <span className="font-mono">Authorization: Bearer</span> and this token: a speech-to-text engine (call transcripts), a telematics provider (vehicle crash notifications), the drone system (status, position, live picture), a radio system's transcription. Without a token the hooks are off.</p>
        <ul className="space-y-1 text-xs">
          {['transcript', 'acn', 'drone', 'radio-text', 'location', 'alarm', 'lpr', 'gunshot', 'alert'].map((h) => <li key={h}><Copyable text={`${base}/${h}`} /></li>)}
        </ul>
        <Field label={`Hook token ${x.hookTokenSet ? '(set)' : '(not set: hooks off)'}`} htmlFor="ht" hint="Leave empty to keep it; - clears it">
          <div className="flex gap-1">
            <input id="ht" className="input font-mono" value={hook} onChange={(e) => setHook(e.target.value)} placeholder={x.hookTokenSet ? '••••••••' : ''} />
            <button type="button" className="btn" onClick={async () => setHook((await api.post<{ secret: string }>('/admin/integrations/secret')).secret)}>New</button>
          </div>
        </Field>
        {hook && hook !== '-' && <p className="text-xs text-amber-300">Copy it now and give it to the sending system: <Copyable text={hook} /></p>}
      </section>
      <section className="panel space-y-3 rounded-lg p-4">
        <h2 className="flex items-center gap-2 font-semibold text-slate-100"><Server size={15} aria-hidden /> Alarms, plate readers, gunshot detection</h2>
        <p className="text-xs text-slate-400">Alarm companies, plate readers and gunshot detection post to the alarm, lpr, gunshot and alert hooks above with the hook token (the contract is docs/ALERTS.md). Each arrives as a call in the queue, data only, for a dispatcher to make the incident from. Alarm companies are told what became of their alarms at this address: accepted, dispatched, on scene, closed.</p>
        <Field label="Answers to the alarm company" htmlFor="aau" hint="https:// address; empty: no answers"><input id="aau" className="input font-mono" value={answerUrl} onChange={(e) => setAnswerUrl(e.target.value)} placeholder="https://alarm-company.example/cad/status" /></Field>
        <Field label={`Token sent with them ${x.alertAnswerTokenSet ? '(set)' : ''}`} htmlFor="aat" hint="Leave empty to keep it; - clears it"><input id="aat" type="password" className="input font-mono" value={answerToken} onChange={(e) => setAnswerToken(e.target.value)} placeholder={x.alertAnswerTokenSet ? '••••••••' : ''} autoComplete="new-password" /></Field>
        <p className="text-xs text-slate-500">ASAP over Nlets, the national standard for alarms, needs the standard's schema (sold by APCO) and the state's message switch; it is not connected yet.</p>
      </section>
      <section className="panel space-y-3 rounded-lg p-4">
        <h2 className="flex items-center gap-2 font-semibold text-slate-100"><Server size={15} aria-hidden /> Station alerting</h2>
        <p className="text-xs text-slate-400">When units are dispatched, their stations are toned out: the station's screen (open <span className="font-mono">#/station?st=ST1</span> on a read-only account in the firehouse) sounds the tones and reads the call aloud, and an alerting controller at the address below gets each alert (docs/STATION-ALERTING.md). TONE on the command line tones out again.</p>
        <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={stations} onChange={(e) => setStations(e.target.checked)} /> Tone out stations when their units are dispatched</label>
        <Field label="Alerting controller's address (optional)" htmlFor="gwu" hint="https:// or http://127.0.0.1 for a program on this server"><input id="gwu" className="input font-mono" value={gwUrl} onChange={(e) => setGwUrl(e.target.value)} placeholder="https://station-gateway.example/alert" /></Field>
        <Field label={`Token sent with it ${x.stationGatewayTokenSet ? '(set)' : ''}`} htmlFor="gwt" hint="Leave empty to keep it; - clears it"><input id="gwt" type="password" className="input font-mono" value={gwToken} onChange={(e) => setGwToken(e.target.value)} placeholder={x.stationGatewayTokenSet ? '••••••••' : ''} autoComplete="new-password" /></Field>
        <p className="text-xs text-slate-500">Vendors' station controllers (such as Phoenix G2 or Locution) use their own protocols; a small adapter from this alert to theirs is the connection, and none is included yet.</p>
      </section>
      <section className="panel space-y-3 rounded-lg p-4">
        <h2 className="flex items-center gap-2 font-semibold text-slate-100"><Server size={15} aria-hidden /> Drone as first responder</h2>
        <p className="text-xs text-slate-400">bitdispatcher posts a launch request (incident, type, priority, position, summary) to the drone system's address; the drone system reports back to the drone hook. With the launch rule on, incidents of call types marked for a drone (Call types) ask by themselves, once each.</p>
        <Field label="Drone system's launch address" htmlFor="du"><input id="du" className="input font-mono" value={droneUrl} onChange={(e) => setDroneUrl(e.target.value)} placeholder="https://dfr.example/api/launch" /></Field>
        <Field label={`Token sent with it ${x.droneTokenSet ? '(set)' : ''}`} htmlFor="dt"><input id="dt" className="input font-mono" value={droneToken} onChange={(e) => setDroneToken(e.target.value)} placeholder={x.droneTokenSet ? '••••••••' : ''} /></Field>
        <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={droneAuto} onChange={(e) => setDroneAuto(e.target.checked)} /> Launch rule: ask for a drone for call types marked for one</label>
        <p className="text-xs text-slate-500">{view.health.drone ? 'A drone system is configured: incidents show Send a drone.' : 'No drone system configured.'}</p>
      </section>
      <section className="panel space-y-3 rounded-lg p-4 lg:col-span-2">
        <h2 className="flex items-center gap-2 font-semibold text-slate-100"><Server size={15} aria-hidden /> Attachments: virus scan and the words inside</h2>
        <p className="text-xs text-slate-400">Every file is checked by its content and kept with its hash. With a ClamAV daemon (clamd) on this server or your network, incident files and message attachments are also scanned before they are kept, and a file it reports is refused and the attempt recorded. If clamd cannot be reached, uploads are refused rather than kept unscanned.</p>
        <div className="flex items-end gap-2">
          <Field label="ClamAV daemon (clamd)" htmlFor="clam" hint="host:port (such as 127.0.0.1:3310) or the path of its socket; empty: not scanned">
            <input id="clam" className="input font-mono" value={clam} onChange={(e) => setClam(e.target.value)} placeholder="127.0.0.1:3310" />
          </Field>
          <button type="button" className="btn mb-5" onClick={testClam} disabled={!clam && !x.clamav}>Test</button>
        </div>
        {clamTest && (
          <p role="status" className={clamTest.ok ? 'text-xs text-emerald-300' : 'text-xs text-rose-300'}>
            {clamTest.ok ? `${clamTest.version}: a clean file passed and the standard test file was caught (${clamTest.testFile}).` : clamTest.error || `clamd answered (${clamTest.version ?? '?'}) but did not catch the standard test file: check its signatures are loaded.`}
          </p>
        )}
        <p className="text-xs text-slate-400">Search reads the words in plain-text files and in PDFs' text by itself. Words in pictures (a photo of a plate, a scanned page) need optical character recognition by the Tesseract program installed on this server; pictures are read on this server only.</p>
        <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={ocr} onChange={(e) => setOcr(e.target.checked)} disabled={!ocrInfo?.installed && !ocr} /> Read the words in pictures for search</label>
        <p className="text-xs text-slate-500">{ocrInfo?.installed ? `Installed: ${ocrInfo.version}. Turning it on also reads the pictures already stored.` : 'Tesseract is not installed on this server (on Debian or Ubuntu: apt install tesseract-ocr).'}</p>
        <div className="flex justify-end"><button className="btn btn-go" onClick={save}><Save size={14} aria-hidden /> Save connections</button></div>
      </section>
      <section className="panel space-y-3 rounded-lg p-4 lg:col-span-2">
        <h2 className="flex items-center gap-2 font-semibold text-slate-100"><PlugZap size={15} aria-hidden /> Translation by a language model (optional)</h2>
        <p className="text-xs text-slate-400">Without one, a built-in emergency phrasebook translates Spanish and English and says when it only has the gist. A model on an OpenAI-compatible chat API (one on this server, or a service your agency has an agreement with) translates the rest. <b className="text-amber-200">What callers say is sent to it</b>: switch it on only under your agency's policy.</p>
        <div className="grid gap-2 md:grid-cols-3">
          <Field label="Address" htmlFor="mu"><input id="mu" className="input font-mono" value={modelUrl} onChange={(e) => setModelUrl(e.target.value)} placeholder="http://127.0.0.1:11434/v1" /></Field>
          <Field label="Model" htmlFor="mn"><input id="mn" className="input font-mono" value={modelName} onChange={(e) => setModelName(e.target.value)} placeholder="llama3.1" /></Field>
          <Field label={`Key ${x.modelKeySet ? '(set)' : ''}`} htmlFor="mk"><input id="mk" type="password" className="input font-mono" value={modelKey} onChange={(e) => setModelKey(e.target.value)} placeholder={x.modelKeySet ? '••••••••' : 'none for a local model'} /></Field>
        </div>
        <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={modelOn} onChange={(e) => setModelOn(e.target.checked)} /> Use it for translation</label>
        <label className="flex items-start gap-2 text-sm text-slate-300"><input type="checkbox" className="mt-1" checked={foiaModel} onChange={(e) => setFoiaModel(e.target.checked)} /><span>Use it to suggest redactions in records requests<span className="block text-xs text-amber-300">The record's text is sent to the model. Its suggestions are proposals for a person to decide, marked as the model's.</span></span></label>
        <div className="flex justify-end"><button className="btn btn-go" onClick={save}><Save size={14} aria-hidden /> Save connections</button></div>
      </section>
    </div>
  )
}

// ViewsSettings: where pictures of a place from the side come from. Each provider is told where the
// incident is when it is looked up, so each is off until an administrator turns it on.
function ViewsSettings() {
  const [c, setC] = useState<ViewsConfig | null>(null)
  const [secret, setSecret] = useState('')
  const [token, setToken] = useState('')
  const [googleKey, setGoogleKey] = useState('')
  const [test, setTest] = useState<Record<string, string> | null>(null)
  useEffect(() => {
    api.get<ViewsConfig>('/admin/views').then(setC).catch((e) => toast.error(errText(e)))
  }, [])
  if (!c) return null
  // googleKeySet comes back from the server (GET returns it); the shared ViewsConfig type may not name it yet.
  const googleKeySet = !!(c as { googleKeySet?: boolean }).googleKeySet
  const set = (p: Partial<ViewsConfig>) => setC({ ...c, ...p })
  const save = async () => {
    try {
      setC(await api.put<ViewsConfig>('/admin/views', { ...c, eagleviewSecret: secret, mapillaryToken: token, googleKey }))
      setSecret('')
      setToken('')
      setGoogleKey('')
      toast.success('Views saved')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section className="panel mt-4 space-y-3 rounded-lg p-4" aria-labelledby="views-h">
      <h2 id="views-h" className="flex items-center gap-2 font-semibold text-slate-100"><Images size={15} className="text-sky-300" aria-hidden /> Views: the place from every side</h2>
      <p className="text-xs text-slate-400">
        Oblique aerial photos (the front, back and sides of a building) and street photos, shown on the incident, in the call-taker's Hazards tab and on the crews' terminals, beside the agency's own pre-plan photos of each premise.
        Pictures are fetched by this server, so keys never reach a browser. <b className="text-amber-200">Looking a place up tells the provider where the incident is</b>: turn a provider on only under your agency's agreement with it.
      </p>
      <div className="grid gap-4 lg:grid-cols-3">
        <div className="space-y-2 rounded-md border border-slate-800 p-3">
          <label className="flex items-center gap-2 text-sm font-semibold text-slate-100"><input type="checkbox" checked={c.eagleviewOn} onChange={(e) => set({ eagleviewOn: e.target.checked })} /> EagleView (oblique, your licence)</label>
          <p className="text-xs text-slate-500">An application on EagleView's developer portal with Imagery API access under your agency's licence: its client ID and secret.</p>
          <Field label="Client ID" htmlFor="evid"><input id="evid" className="input font-mono" value={c.eagleviewClientId} onChange={(e) => set({ eagleviewClientId: e.target.value })} autoComplete="off" /></Field>
          <Field label={`Client secret ${c.eagleviewSecretSet ? '(set)' : ''}`} htmlFor="evs" hint="Leave empty to keep it; - clears it">
            <input id="evs" type="password" className="input font-mono" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder={c.eagleviewSecretSet ? '••••••••' : ''} autoComplete="new-password" />
          </Field>
          <label className="flex items-center gap-2 text-xs text-slate-300"><input type="checkbox" checked={c.eagleviewSandbox} onChange={(e) => set({ eagleviewSandbox: e.target.checked })} /> EagleView's sandbox (a test area in Omaha)</label>
        </div>
        <div className="space-y-2 rounded-md border border-slate-800 p-3">
          <label className="flex items-center gap-2 text-sm font-semibold text-slate-100"><input type="checkbox" checked={c.panoramaxOn} onChange={(e) => set({ panoramaxOn: e.target.checked })} /> Panoramax (street, open)</label>
          <p className="text-xs text-slate-500">Open street-level photos (CC-BY-SA). Free, no key; coverage depends on who has photographed your streets.</p>
          <Field label="Instance" htmlFor="pxu" hint="Empty: the catalogue of every instance"><input id="pxu" className="input font-mono" value={c.panoramaxUrl} onChange={(e) => set({ panoramaxUrl: e.target.value })} placeholder="https://api.panoramax.xyz" /></Field>
        </div>
        <div className="space-y-2 rounded-md border border-slate-800 p-3">
          <label className="flex items-center gap-2 text-sm font-semibold text-slate-100"><input type="checkbox" checked={c.mapillaryOn} onChange={(e) => set({ mapillaryOn: e.target.checked })} /> Mapillary (street)</label>
          <p className="text-xs text-slate-500">Street-level photos (CC-BY-SA) with a free client token from Mapillary's developer dashboard.</p>
          <Field label={`Client token ${c.mapillaryTokenSet ? '(set)' : ''}`} htmlFor="mlt" hint="Leave empty to keep it; - clears it">
            <input id="mlt" type="password" className="input font-mono" value={token} onChange={(e) => setToken(e.target.value)} placeholder={c.mapillaryTokenSet ? '••••••••' : 'MLY|…'} autoComplete="new-password" />
          </Field>
          <label className="flex items-center gap-2 text-xs text-slate-300"><input type="checkbox" checked={c.googleLink} onChange={(e) => set({ googleLink: e.target.checked })} /> Offer a link to Google Street View (opens Google in a new tab; nothing is fetched here)</label>
          <Field label={`Google Maps key ${googleKeySet ? '(set)' : ''}`} htmlFor="gmk" hint="With a key restricted to this site, Street View docks in call entry and the server checks first whether Google has imagery there (Street View metadata and Maps Embed API). Without one, only the open street photos show.">
            <input id="gmk" type="password" className="input font-mono" value={googleKey} onChange={(e) => setGoogleKey(e.target.value)} placeholder={googleKeySet ? '••••••••' : 'AIza…'} autoComplete="new-password" />
          </Field>
        </div>
      </div>
      {test && <ul className="space-y-0.5 text-xs" aria-label="Test results">{Object.entries(test).map(([k, v]) => <li key={k}><b>{k}</b>: {v}</li>)}{!Object.keys(test).length && <li>No provider is on.</li>}</ul>}
      <div className="flex justify-end gap-2">
        <button className="btn" onClick={async () => { try { setTest((await api.post<{ results: Record<string, string> }>('/admin/views/test', {})).results) } catch (e) { toast.error(errText(e)) } }}>Test at the map's centre</button>
        <button className="btn btn-go" onClick={save}><Save size={14} aria-hidden /> Save views</button>
      </div>
    </section>
  )
}

// NERISSettings: fire and EMS incidents to NERIS, the national reporting system, as CAD records.
function NERISSettings() {
  const [c, setC] = useState<NERISConfig | null>(null)
  const [secret, setSecret] = useState('')
  const [test, setTest] = useState<string>('')
  useEffect(() => {
    api.get<NERISConfig>('/admin/neris').then(setC).catch((e) => toast.error(errText(e)))
  }, [])
  if (!c) return null
  const set = (p: Partial<NERISConfig>) => setC({ ...c, ...p })
  const save = async () => {
    try {
      setC(await api.put<NERISConfig>('/admin/neris', { ...c, secret }))
      setSecret('')
      toast.success('NERIS saved')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section className="panel mt-4 space-y-3 rounded-lg p-4" aria-labelledby="neris-h">
      <h2 id="neris-h" className="flex items-center gap-2 font-semibold text-slate-100"><Server size={15} className="text-orange-300" aria-hidden /> NERIS: fire and EMS reporting</h2>
      <p className="text-xs text-slate-400">NERIS replaced NFIRS in 2026. bitdispatcher sends each fire or EMS incident as a <b>CAD record</b>: when the call came in and was answered, each unit's dispatch, en route, on scene and clear times, the place and how it ended. NERIS keeps it as pending until the fire department completes the rest (the incident type, actions, fire and medical detail) in NERIS or its records system. Police units are left out.</p>
      <div className="grid gap-2 md:grid-cols-3">
        <Field label="Department's NERIS ID" htmlFor="nd" hint="FD and eight digits"><input id="nd" className="input font-mono uppercase" value={c.department} onChange={(e) => set({ department: e.target.value })} placeholder="FD51161234" /></Field>
        <Field label="Client ID" htmlFor="nc" hint="a CAD integration from NERIS"><input id="nc" className="input font-mono" value={c.clientId} onChange={(e) => set({ clientId: e.target.value })} autoComplete="off" /></Field>
        <Field label={`Client secret ${c.secretSet ? '(set)' : ''}`} htmlFor="ns" hint="Leave empty to keep it; - clears it"><input id="ns" type="password" className="input font-mono" value={secret} onChange={(e) => setSecret(e.target.value)} placeholder={c.secretSet ? '••••••••' : ''} autoComplete="new-password" /></Field>
        <Field label="Dispatch centre" htmlFor="ncc" hint="sent as the centre's ID"><input id="ncc" className="input" value={c.center} onChange={(e) => set({ center: e.target.value })} placeholder="Roanoke ECC" /></Field>
      </div>
      <div className="flex flex-wrap gap-4 text-sm text-slate-300">
        <label className="flex items-center gap-2"><input type="checkbox" checked={c.on} onChange={(e) => set({ on: e.target.checked })} /> On</label>
        <label className="flex items-center gap-2"><input type="checkbox" checked={c.auto} onChange={(e) => set({ auto: e.target.checked })} /> Send each fire or EMS incident when it closes</label>
        <label className="flex items-center gap-2"><input type="checkbox" checked={c.test} onChange={(e) => set({ test: e.target.checked })} /> NERIS's test environment</label>
      </div>
      {test && <pre className="max-h-40 overflow-auto whitespace-pre-wrap rounded bg-slate-950 p-2 text-[11px] text-slate-300">{test}</pre>}
      <div className="flex justify-end gap-2">
        <button className="btn" onClick={async () => {
          try { const r = await api.post<{ incident: string; ok: boolean; status: number; answer: string; error: string }>('/admin/neris/test', {}); setTest(`${r.incident}: ${r.ok ? 'NERIS accepts it' : r.error}\n${r.answer}`) } catch (e) { setTest(errText(e)) }
        }}>Check the last closed incident</button>
        <button className="btn btn-go" onClick={save}><Save size={14} aria-hidden /> Save NERIS</button>
      </div>
    </section>
  )
}

type PV = { id: string; name: string; url: string; outTokenSet: boolean; inTokenSet: boolean; receive: string }

// Partners: neighbouring agencies' CADs, for sending and receiving incidents (CAD-to-CAD, as EIDOs).
function Partners() {
  const [list, setList] = useState<PV[]>([])
  const [d, setD] = useState({ id: '', name: '', url: '', outToken: '', inToken: '' })
  const load = () => api.get<PV[]>('/admin/partners').then(setList).catch((e) => toast.error(errText(e)))
  useEffect(() => { load() }, [])
  const save = async () => {
    try {
      await api.put(`/admin/partners/${encodeURIComponent(d.id)}`, d)
      toast.success('Saved')
      setD({ id: '', name: '', url: '', outToken: '', inToken: '' })
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section className="panel mt-4 space-y-3 rounded-lg p-4" aria-labelledby="partners-h">
      <h2 id="partners-h" className="flex items-center gap-2 font-semibold text-slate-100"><Server size={15} className="text-violet-300" aria-hidden /> Neighbouring agencies (CAD-to-CAD)</h2>
      <p className="text-xs text-slate-400">Dispatchers send an incident to a neighbour's CAD (mutual aid, a call on their side of the line) as an EIDO, the NENA standard; what a neighbour sends arrives here as a call in the queue. Each neighbour has its own address and its own tokens (docs/CAD-TO-CAD.md).</p>
      <ul className="space-y-1 text-sm">
        {list.map((p) => (
          <li key={p.id} className="flex flex-wrap items-center gap-2 rounded border border-slate-800 px-2 py-1">
            <span className="font-mono font-bold text-slate-100">{p.id}</span><span className="text-slate-200">{p.name}</span>
            <span className="truncate font-mono text-[11px] text-slate-500">{p.url || 'no address: receive only'}</span>
            <span className="text-[11px] text-slate-400">they send to <Copyable text={location.origin + p.receive} /> {p.inTokenSet ? '' : '(no token yet: refused)'}</span>
            <button className="btn btn-ghost ml-auto h-6 text-xs" onClick={() => setD({ id: p.id, name: p.name, url: p.url, outToken: '', inToken: '' })}>Change</button>
            <button className="btn btn-ghost h-6 text-xs" onClick={async () => { if (confirm(`Remove ${p.name}?`)) { await api.del(`/admin/partners/${p.id}`).catch((e) => toast.error(errText(e))); load() } }}>Remove</button>
          </li>
        ))}
        {!list.length && <li className="text-xs text-slate-500">No neighbouring agencies yet.</li>}
      </ul>
      <div className="grid gap-2 md:grid-cols-5">
        <Field label="ID" htmlFor="pid"><input id="pid" className="input font-mono uppercase" value={d.id} onChange={(e) => setD({ ...d, id: e.target.value })} placeholder="FRANKLIN" /></Field>
        <Field label="Name" htmlFor="pnm"><input id="pnm" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} placeholder="Franklin County 911" /></Field>
        <Field label="Their CAD's EIDO address" htmlFor="purl"><input id="purl" className="input font-mono" value={d.url} onChange={(e) => setD({ ...d, url: e.target.value })} placeholder="https://cad.franklin.example/…" /></Field>
        <Field label="Token we send" htmlFor="pot" hint="empty keeps it; - clears"><input id="pot" type="password" className="input font-mono" value={d.outToken} onChange={(e) => setD({ ...d, outToken: e.target.value })} autoComplete="new-password" /></Field>
        <Field label="Token they send" htmlFor="pit" hint="empty keeps it; - clears"><input id="pit" type="password" className="input font-mono" value={d.inToken} onChange={(e) => setD({ ...d, inToken: e.target.value })} autoComplete="new-password" /></Field>
      </div>
      <div className="flex justify-end"><button className="btn btn-go" disabled={!d.id || !d.name} onClick={save}><Save size={14} aria-hidden /> Save the agency</button></div>
    </section>
  )
}

type PFV = { on: boolean; tokenSet: boolean; police: boolean; leave: string[]; cprTypes: string[]; pushUrl: string; pushTokenSet: boolean }

// PublicFeedSettings: the public incident feed, for the public and apps such as PulsePoint, without what
// identifies anyone (hundred-block addresses, points to about 100 m, no callers or notes).
function PublicFeedSettings({ view }: { view: View }) {
  const [c, setC] = useState<PFV | null>(null)
  const [tok, setTok] = useState('')
  const [pushTok, setPushTok] = useState('')
  const [preview, setPreview] = useState('')
  useEffect(() => { api.get<PFV>('/admin/public-feed').then(setC).catch(() => {}) }, [])
  if (!c) return null
  const save = async () => {
    try {
      setC(await api.put<PFV>('/admin/public-feed', { ...c, token: tok, pushToken: pushTok }))
      setTok('')
      setPushTok('')
      toast.success('Saved')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const pick = (k: 'leave' | 'cprTypes', code: string) => setC({ ...c, [k]: c[k].includes(code) ? c[k].filter((x) => x !== code) : [...c[k], code] })
  return (
    <section className="panel mt-4 space-y-3 rounded-lg p-4" aria-labelledby="pf-h">
      <h2 id="pf-h" className="flex items-center gap-2 font-semibold text-slate-100"><Server size={15} className="text-emerald-300" aria-hidden /> Public incident feed (PulsePoint and others)</h2>
      <p className="text-xs text-slate-400">Open incidents for the public, a website or an app: the type, the hundred block, a point to about 100 m, the units and their status. Never callers, callbacks, notes or house numbers. PulsePoint connects through its own partner interface; this is the documented feed a bridge reads (docs/PUBLIC-FEED.md).</p>
      <div className="flex flex-wrap gap-4 text-sm">
        <label className="flex items-center gap-2"><input type="checkbox" checked={c.on} onChange={(e) => setC({ ...c, on: e.target.checked })} /> Publish the feed at <Copyable text={location.origin + '/api/public/incidents'} /></label>
        <label className="flex items-center gap-2"><input type="checkbox" checked={c.police} onChange={(e) => setC({ ...c, police: e.target.checked })} /> Include police incidents</label>
      </div>
      {c.on && !c.tokenSet && !tok && <p className="rounded bg-amber-500/15 px-2 py-1 text-xs text-amber-200">Without a token anyone on the internet can read the feed. Set one to give it only to a partner.</p>}
      <fieldset>
        <legend className="label">Never show these call types</legend>
        <div className="flex flex-wrap gap-1">{view.callTypes.map((t) => <button key={t.code} type="button" aria-pressed={c.leave.includes(t.code)} className={cx('chip', c.leave.includes(t.code) ? 'bg-rose-700 text-white' : 'bg-slate-800 text-slate-300')} onClick={() => pick('leave', t.code)}>{t.code}</button>)}</div>
      </fieldset>
      <fieldset>
        <legend className="label">Ask for CPR-trained bystanders on these</legend>
        <div className="flex flex-wrap gap-1">{view.callTypes.filter((t) => t.discipline !== 'police').map((t) => <button key={t.code} type="button" aria-pressed={c.cprTypes.includes(t.code)} className={cx('chip', c.cprTypes.includes(t.code) ? 'bg-sky-600 text-white' : 'bg-slate-800 text-slate-300')} onClick={() => pick('cprTypes', t.code)}>{t.code}</button>)}</div>
      </fieldset>
      <div className="grid gap-2 md:grid-cols-3">
        <Field label="Readers' token" htmlFor="pf-tok" hint={c.tokenSet ? 'set: empty keeps it; - clears' : 'empty: anyone may read'}><input id="pf-tok" type="password" className="input font-mono" value={tok} onChange={(e) => setTok(e.target.value)} autoComplete="new-password" /></Field>
        <Field label="Push to (optional)" htmlFor="pf-push"><input id="pf-push" className="input font-mono" value={c.pushUrl} onChange={(e) => setC({ ...c, pushUrl: e.target.value })} placeholder="https://bridge.example/feed" /></Field>
        <Field label="Push token" htmlFor="pf-pt" hint={c.pushTokenSet ? 'set: empty keeps it; - clears' : undefined}><input id="pf-pt" type="password" className="input font-mono" value={pushTok} onChange={(e) => setPushTok(e.target.value)} autoComplete="new-password" /></Field>
      </div>
      <div className="flex justify-end gap-2">
        <button className="btn" onClick={async () => setPreview(JSON.stringify(await api.get('/admin/public-feed/preview'), null, 2))}>See what it shows</button>
        <button className="btn btn-go" onClick={save}><Save size={14} aria-hidden /> Save the feed</button>
      </div>
      {preview && <pre className="max-h-64 overflow-auto rounded bg-slate-950 p-2 text-[11px] text-slate-300">{preview}</pre>}
    </section>
  )
}

type LV = { on: boolean; name: string; url: string; tokenSet: boolean; user: string; passwordSet: boolean; auto: boolean }

// LISSettings: a location server asked by HELD for the caller's phone (RapidSOS's clearinghouse or a
// state NG9-1-1 LIS), with the credentials the provider gives the centre.
function LISSettings() {
  const [c, setC] = useState<LV | null>(null)
  const [tok, setTok] = useState('')
  const [pw, setPw] = useState('')
  const [num, setNum] = useState('')
  const [res, setRes] = useState('')
  useEffect(() => { api.get<LV>('/admin/lis').then(setC).catch(() => {}) }, [])
  if (!c) return null
  const save = async () => {
    try { setC(await api.put<LV>('/admin/lis', { ...c, token: tok, password: pw })); setTok(''); setPw(''); toast.success('Saved') } catch (e) { toast.error(errText(e)) }
  }
  return (
    <section className="panel mt-4 space-y-3 rounded-lg p-4" aria-labelledby="lis-h">
      <h2 id="lis-h" className="flex items-center gap-2 font-semibold text-slate-100"><Server size={15} className="text-sky-300" aria-hidden /> Supplemental location (RapidSOS, NG9-1-1 LIS)</h2>
      <p className="text-xs text-slate-400">bitdispatcher asks a location server where the caller's phone is, by its number, using HELD (the NG9-1-1 standard). The provider gives your centre the address and credentials. The answer is offered to the call-taker like any newer location (docs/SUPPLEMENTAL-LOCATION.md). Not yet tested against RapidSOS itself.</p>
      <div className="flex flex-wrap gap-4 text-sm">
        <label className="flex items-center gap-2"><input type="checkbox" checked={c.on} onChange={(e) => setC({ ...c, on: e.target.checked })} /> On</label>
        <label className="flex items-center gap-2"><input type="checkbox" checked={c.auto} onChange={(e) => setC({ ...c, auto: e.target.checked })} /> Ask for every new 911 call</label>
      </div>
      <div className="grid gap-2 md:grid-cols-5">
        <Field label="Name" htmlFor="lis-n"><input id="lis-n" className="input" value={c.name} onChange={(e) => setC({ ...c, name: e.target.value })} placeholder="RapidSOS" /></Field>
        <Field label="HELD address" htmlFor="lis-u"><input id="lis-u" className="input font-mono" value={c.url} onChange={(e) => setC({ ...c, url: e.target.value })} placeholder="https://…" /></Field>
        <Field label="Token (Bearer)" htmlFor="lis-t" hint={c.tokenSet ? 'set: empty keeps it; - clears' : undefined}><input id="lis-t" type="password" className="input font-mono" value={tok} onChange={(e) => setTok(e.target.value)} autoComplete="new-password" /></Field>
        <Field label="Or user" htmlFor="lis-us"><input id="lis-us" className="input font-mono" value={c.user} onChange={(e) => setC({ ...c, user: e.target.value })} autoComplete="off" /></Field>
        <Field label="Password" htmlFor="lis-p" hint={c.passwordSet ? 'set: empty keeps it; - clears' : undefined}><input id="lis-p" type="password" className="input font-mono" value={pw} onChange={(e) => setPw(e.target.value)} autoComplete="new-password" /></Field>
      </div>
      <div className="flex flex-wrap items-end justify-end gap-2">
        <Field label="Try a number" htmlFor="lis-num"><input id="lis-num" className="input font-mono" value={num} onChange={(e) => setNum(e.target.value)} placeholder="+15405550100" /></Field>
        <button className="btn" disabled={!num} onClick={async () => { const r = await api.post<{ ok: boolean; message?: string; location?: { lat?: number; lon?: number; radius?: number } }>('/admin/lis/test', { callback: num }); setRes(r.ok ? `Located: ${r.location?.lat}, ${r.location?.lon} ±${r.location?.radius ?? '?'} m` : `No: ${r.message}`) }}>Ask</button>
        <button className="btn btn-go" onClick={save}><Save size={14} aria-hidden /> Save</button>
      </div>
      {res && <p className="text-xs text-slate-300">{res}</p>}
    </section>
  )
}
