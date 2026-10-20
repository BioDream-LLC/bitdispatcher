import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { CheckCircle2, CircleDashed, Copy, Fingerprint, Images, KeyRound, PlugZap, Save, Server, XCircle } from 'lucide-react'
import { api, errText } from '../api'
import { Field } from '../ui'
import { ago, cx, useNow } from '../util'
import type { View, ViewsConfig } from '../types'
import { Page } from './Admin'

type IV = { bitvoiceUrl: string; bitvoiceFingerprint: string; bitvoiceTokenSet: boolean; eidoTokenSet: boolean; radioSecretSet: boolean; aliListen: string }

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
              <p className="text-xs text-slate-400">The event stream (ringing and answered 911 calls, radio, delivery failures), calls in progress, radio positions, and the caller actions (rebid, text, video). Make an API token in BitVoice under Administrators › API tokens; viewer is enough to read, administrator is needed to acknowledge radio emergencies on its console.</p>
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
      <Connections view={view} />
      <ViewsSettings />
    </Page>
  )
}

type XV = { hookTokenSet: boolean; droneUrl: string; droneTokenSet: boolean; droneAuto: boolean; modelUrl: string; modelKeySet: boolean; modelName: string; modelOn: boolean; foiaModel?: boolean }

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
  const fill = (v: XV) => {
    setX(v)
    setDroneUrl(v.droneUrl)
    setDroneAuto(v.droneAuto)
    setModelUrl(v.modelUrl)
    setModelName(v.modelName)
    setModelOn(v.modelOn)
    setFoiaModel(!!v.foiaModel)
  }
  useEffect(() => {
    api.get<XV>('/admin/connections').then(fill).catch((e) => toast.error(errText(e)))
  }, [])
  const save = async () => {
    try {
      fill(await api.put<XV>('/admin/connections', { hookToken: hook, droneUrl, droneToken, droneAuto, modelUrl, modelKey, modelName, modelOn, foiaModel }))
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
          {['transcript', 'acn', 'drone', 'radio-text'].map((h) => <li key={h}><Copyable text={`${base}/${h}`} /></li>)}
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
        <h2 className="flex items-center gap-2 font-semibold text-slate-100"><Server size={15} aria-hidden /> Drone as first responder</h2>
        <p className="text-xs text-slate-400">bitdispatcher posts a launch request (incident, type, priority, position, summary) to the drone system's address; the drone system reports back to the drone hook. With the launch rule on, incidents of call types marked for a drone (Call types) ask by themselves, once each.</p>
        <Field label="Drone system's launch address" htmlFor="du"><input id="du" className="input font-mono" value={droneUrl} onChange={(e) => setDroneUrl(e.target.value)} placeholder="https://dfr.example/api/launch" /></Field>
        <Field label={`Token sent with it ${x.droneTokenSet ? '(set)' : ''}`} htmlFor="dt"><input id="dt" className="input font-mono" value={droneToken} onChange={(e) => setDroneToken(e.target.value)} placeholder={x.droneTokenSet ? '••••••••' : ''} /></Field>
        <label className="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" checked={droneAuto} onChange={(e) => setDroneAuto(e.target.checked)} /> Launch rule: ask for a drone for call types marked for one</label>
        <p className="text-xs text-slate-500">{view.health.drone ? 'A drone system is configured: incidents show Send a drone.' : 'No drone system configured.'}</p>
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
  const [test, setTest] = useState<Record<string, string> | null>(null)
  useEffect(() => {
    api.get<ViewsConfig>('/admin/views').then(setC).catch((e) => toast.error(errText(e)))
  }, [])
  if (!c) return null
  const set = (p: Partial<ViewsConfig>) => setC({ ...c, ...p })
  const save = async () => {
    try {
      setC(await api.put<ViewsConfig>('/admin/views', { ...c, eagleviewSecret: secret, mapillaryToken: token }))
      setSecret('')
      setToken('')
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
