import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Download, LifeBuoy, Save } from 'lucide-react'
import { api, errText } from '../api'
import type { SetupView, View } from '../types'
import { Page } from './Admin'

const INSIDE = [
  ['system.json', 'version, operating system, uptime, the ledger\u2019s size and check, disk space'],
  ['setup.json, golive.json', 'the setup checklist and the last go-live check'],
  ['health.json', 'the phone system link, the other sites, the map, paging gateways (names only)'],
  ['counts.json', 'how many of each thing, and the kinds of events in the last 7 days'],
  ['settings.json, config.json', 'the settings, stations, units, call types and response plans'],
  ['integrations.json', 'the connections, with every token, password and key replaced by \u201c(set)\u201d'],
  ['log.txt', 'the server\u2019s recent warnings and errors'],
]

/** Support: who the agency asks for help, and a bundle to send them that holds no secrets and no
 * caller data. bitdispatcher sends nothing by itself. */
export default function Support({ view }: { view: View }) {
  const [text, setText] = useState(view.settings.next?.support ?? '')
  const [shown, setShown] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    api.get<SetupView>('/admin/setup').then((v) => setShown(v.support)).catch(() => {})
  }, [view.seq])
  const save = async () => {
    try {
      await api.put('/admin/settings', { ...view.settings, next: { ...view.settings.next, support: text.trim() } })
      toast.success('Saved')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const download = async () => {
    setBusy(true)
    try {
      const res = await fetch('./api/admin/support/bundle', { method: 'POST', headers: { 'X-BitDispatcher': '1' }, credentials: 'same-origin' })
      if (!res.ok) throw new Error((await res.json().catch(() => ({}))).message ?? res.statusText)
      const name = /filename="([^"]+)"/.exec(res.headers.get('Content-Disposition') ?? '')?.[1] ?? 'bitdispatcher-support.zip'
      const a = document.createElement('a')
      a.href = URL.createObjectURL(await res.blob())
      a.download = name
      a.click()
      setTimeout(() => URL.revokeObjectURL(a.href), 10000)
      toast.success('Bundle downloaded: read it before you send it')
    } catch (e) {
      toast.error(errText(e))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Page title="Support" intro="Who your agency asks for help, and a support bundle to send them. Support for bitdispatcher is remote: by e-mail, phone or video call, never on site. The people at your centre, your IT staff and your phone and radio vendors do anything that needs hands.">
      <div className="grid grid-cols-[1fr_24rem] gap-4">
        <section className="panel space-y-3 rounded-lg p-4 text-sm">
          <h2 className="flex items-center gap-2 font-semibold text-slate-100"><LifeBuoy size={14} aria-hidden /> Who to ask for help</h2>
          <p className="whitespace-pre-wrap rounded bg-slate-900 p-3 text-slate-200" data-testid="support-shown">{shown}</p>
          <label className="label" htmlFor="sup">Your agency&apos;s support contact</label>
          <textarea id="sup" rows={4} className="input" value={text} onChange={(e) => setText(e.target.value)} maxLength={2000}
            placeholder="County IT help desk, 540-555-0100, 24 hours. Support agreement: … (empty: community support on GitHub)" />
          <button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save</button>
        </section>
        <section className="panel space-y-3 rounded-lg p-4 text-sm">
          <h2 className="flex items-center gap-2 font-semibold text-slate-100"><Download size={14} aria-hidden /> Support bundle</h2>
          <p className="text-xs text-slate-400">One file with what a helper needs to find a problem without screen sharing. You download it; bitdispatcher sends nothing. The download is recorded in the ledger.</p>
          <ul className="space-y-1 text-xs">
            {INSIDE.map(([f, d]) => <li key={f}><code className="text-slate-200">{f}</code> <span className="text-slate-400">{d}</span></li>)}
          </ul>
          <p className="text-xs text-slate-300"><b>Left out:</b> calls, incidents, transcripts, recordings, caller names, numbers and locations, premise notes, usernames, passwords, tokens and keys. E-mail addresses, phone numbers and anything like a token are removed from every file.</p>
          <p className="text-xs text-amber-300">Read it before you send it: a name typed into a setting or a call type stays in.</p>
          <button className="btn btn-go" onClick={download} disabled={busy}><Download size={13} aria-hidden /> {busy ? 'Building…' : 'Download the bundle'}</button>
        </section>
      </div>
    </Page>
  )
}
