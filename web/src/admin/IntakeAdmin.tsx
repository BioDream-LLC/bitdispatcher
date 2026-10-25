import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { Bot, Copy, KeyRound, Plus, Save, Send, Trash2 } from 'lucide-react'
import { api, errText } from '../api'
import { Field } from '../ui'
import { cx } from '../util'
import type { IntakeQuestion, IntakeScript, View } from '../types'
import { Page } from './Admin'

type Turn = { say: string; done: boolean; transfer: boolean; reason?: string; ask?: string; kind?: string }

/** The non-emergency intake: the questions a voice or text bot on the non-emergency line asks, the words
 * that hand a caller to a person at once, the bot's token, and a place to try the conversation. */
export default function IntakeAdmin({ view }: { view: View }) {
  const [sc, setSc] = useState<IntakeScript | null>(null)
  const [tokenSet, setTokenSet] = useState(false)
  const [token, setToken] = useState('')
  const [words, setWords] = useState<string[]>([])
  const load = () => api.get<{ script: IntakeScript; tokenSet: boolean; emergencyWords: string[] }>('/admin/intake').then((r) => { setSc(r.script); setTokenSet(r.tokenSet); setWords(r.emergencyWords) }).catch((e) => toast.error(errText(e)))
  useEffect(() => {
    load()
  }, [])
  if (!sc) return null
  const qs = sc.questions ?? []
  const setQ = (i: number, q: Partial<IntakeQuestion>) => setSc({ ...sc, questions: qs.map((x, k) => (k === i ? { ...x, ...q } : x)) })
  const save = async () => {
    try {
      await api.put('/admin/settings', { ...view.settings, next: { ...view.settings.next, intake: sc } })
      toast.success('Intake saved')
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const makeToken = async (clear: boolean) => {
    try {
      const r = await api.put<{ token: string }>('/admin/intake/token', { clear })
      setToken(r.token)
      setTokenSet(!clear)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <Page title="Non-emergency intake" intro="A voice or text bot on the non-emergency line (BitVoice's AI receptionist, or another) asks these questions through bitdispatcher. The moment a caller says anything that sounds like an emergency, or answers yes to a danger question, the bot is told to hand them to a call-taker. A finished report waits in the non-emergency list for a person; nothing is dispatched by it."
      actions={<button className="btn btn-go" onClick={save}><Save size={13} aria-hidden /> Save</button>}>
      <div className="grid grid-cols-[1fr_22rem] gap-4">
        <div className="space-y-4">
          <section className="panel space-y-3 rounded-lg p-4 text-sm">
            <label className="flex items-center gap-2 text-slate-200"><input type="checkbox" checked={!!sc.on} onChange={(e) => setSc({ ...sc, on: e.target.checked })} /> Take reports from the intake bot</label>
            <Field label="Greeting" htmlFor="ig"><textarea id="ig" rows={2} className="input" value={sc.greeting ?? ''} onChange={(e) => setSc({ ...sc, greeting: e.target.value })} /></Field>
            <div>
              <div className="mb-1 text-xs font-semibold text-slate-300">Questions, in order</div>
              <ul className="space-y-1.5">
                {qs.map((q, i) => (
                  <li key={i} className="grid grid-cols-[6rem_1fr_7rem_auto] items-center gap-1.5">
                    <input className="input h-7 font-mono text-xs" value={q.key} onChange={(e) => setQ(i, { key: e.target.value })} aria-label="Key" />
                    <input className="input h-7 text-xs" value={q.ask} onChange={(e) => setQ(i, { ask: e.target.value })} aria-label="What to ask" />
                    <select className="input h-7 text-xs" value={q.kind} onChange={(e) => setQ(i, { kind: e.target.value as IntakeQuestion['kind'] })} aria-label="Kind of answer">
                      <option value="text">words</option><option value="address">an address</option><option value="phone">a phone number</option><option value="yesno">yes or no</option>
                    </select>
                    <span className="flex items-center gap-2 whitespace-nowrap text-[11px] text-slate-400">
                      <label><input type="checkbox" checked={!!q.required} onChange={(e) => setQ(i, { required: e.target.checked })} /> needed</label>
                      {q.kind === 'yesno' && <label title="Yes hands the caller to a person"><input type="checkbox" checked={!!q.danger} onChange={(e) => setQ(i, { danger: e.target.checked })} /> yes = hand over</label>}
                      <button aria-label="Remove the question" onClick={() => setSc({ ...sc, questions: qs.filter((_, k) => k !== i) })}><Trash2 size={11} aria-hidden /></button>
                    </span>
                  </li>
                ))}
              </ul>
              <button className="btn mt-1.5 h-6 px-2 py-0 text-[11px]" onClick={() => setSc({ ...sc, questions: [...qs, { key: 'q' + (qs.length + 1), ask: '', kind: 'text' }] })}><Plus size={11} aria-hidden /> Add a question</button>
              <p className="mt-1 text-[11px] text-slate-500">The keys <code>where</code>, <code>phone</code> and <code>name</code> fill the report's address (placed on the map when it is found exactly), callback number and caller.</p>
            </div>
            <Field label="Closing ({ref} is the report's number)" htmlFor="ic"><textarea id="ic" rows={2} className="input" value={sc.closing ?? ''} onChange={(e) => setSc({ ...sc, closing: e.target.value })} /></Field>
            <Field label="Said before handing over" htmlFor="it"><input id="it" className="input" value={sc.transfer ?? ''} onChange={(e) => setSc({ ...sc, transfer: e.target.value })} /></Field>
            <Field label="Words that hand the caller to a person" htmlFor="iw" hint="Separated by commas; empty uses the built-in list and the critical words. A word just after no, not or nobody does not count."><textarea id="iw" rows={3} className="input text-xs" value={(sc.emergencyWords ?? []).join(', ')} onChange={(e) => setSc({ ...sc, emergencyWords: e.target.value.split(',').map((x) => x.trim()).filter(Boolean) })} placeholder={words.join(', ')} /></Field>
          </section>
          <section className="panel space-y-2 rounded-lg p-4 text-sm">
            <h2 className="flex items-center gap-2 font-semibold text-slate-100"><KeyRound size={14} aria-hidden /> The bot's token</h2>
            <p className="text-xs text-slate-400">The bot starts a conversation with <code>POST /api/intake/sessions</code> and sends each thing the caller says to <code>POST /api/intake/sessions/&#123;id&#125;</code>, with this token as Bearer. Each answer says what to say next, and whether to hand the caller over (docs/INTAKE.md).</p>
            <div className="flex items-center gap-2">
              <button className="btn" onClick={() => makeToken(false)}>{tokenSet ? 'Make a new token' : 'Make the token'}</button>
              {tokenSet && <button className="btn btn-ghost" onClick={() => confirm('Disconnect the bot?') && makeToken(true)}>Disconnect</button>}
              <span className="text-xs text-slate-400">{tokenSet ? 'set' : 'not set: the bot cannot connect'}</span>
            </div>
            {token && <div className="text-xs"><span className="text-amber-300">Shown this once: </span><code className="break-all">{token}</code> <button aria-label="Copy the token" onClick={() => navigator.clipboard?.writeText(token)}><Copy size={11} aria-hidden /></button></div>}
          </section>
        </div>
        <TryIt />
      </div>
    </Page>
  )
}

/** TryIt runs the saved script as a caller would, making no report. */
function TryIt() {
  const [id, setId] = useState('')
  const [lines, setLines] = useState<{ who: 'bot' | 'you'; text: string; note?: string }[]>([])
  const [text, setText] = useState('')
  const [over, setOver] = useState(false)
  const start = async () => {
    try {
      const r = await api.post<{ session: string; turn: Turn }>('/admin/intake/try', { channel: 'text' })
      setId(r.session)
      setOver(false)
      setLines([{ who: 'bot', text: r.turn.say }])
    } catch (e) {
      toast.error(errText(e))
    }
  }
  const send = async () => {
    if (!text.trim() || !id) return
    const said = text
    setText('')
    setLines((l) => [...l, { who: 'you', text: said }])
    try {
      const r = await api.post<{ turn: Turn; ref?: string }>(`/admin/intake/try/${id}`, { text: said })
      setLines((l) => [...l, { who: 'bot', text: r.turn.say, note: r.turn.transfer ? `handed to a person: ${r.turn.reason}` : r.turn.done ? 'report finished (a try-out: nothing was made)' : undefined }])
      if (r.turn.done) setOver(true)
    } catch (e) {
      toast.error(errText(e))
    }
  }
  return (
    <section className="panel flex h-[32rem] flex-col rounded-lg p-3 text-sm" aria-label="Try the intake">
      <h2 className="mb-2 flex items-center gap-2 font-semibold text-slate-100"><Bot size={14} aria-hidden /> Try it (the saved script)</h2>
      <ul className="min-h-0 flex-1 space-y-1.5 overflow-y-auto">
        {lines.map((l, i) => (
          <li key={i} className={cx('rounded px-2 py-1 text-xs', l.who === 'bot' ? 'bg-slate-800 text-slate-100' : 'ml-6 bg-sky-900/50 text-sky-100')}>
            {l.text}{l.note && <div className="mt-0.5 text-[10px] font-semibold text-amber-300">{l.note}</div>}
          </li>
        ))}
      </ul>
      {!id || over ? <button className="btn mt-2" onClick={start}>{id ? 'Start again' : 'Start a conversation'}</button> : (
        <form className="mt-2 flex gap-1" onSubmit={(e) => { e.preventDefault(); send() }}>
          <input className="input h-8 text-xs" value={text} onChange={(e) => setText(e.target.value)} placeholder="Say something as the caller…" aria-label="As the caller" autoFocus />
          <button className="btn h-8" aria-label="Send"><Send size={12} aria-hidden /></button>
        </form>
      )}
    </section>
  )
}
