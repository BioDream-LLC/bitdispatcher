import { useState } from 'react'
import CodeMirror from '@uiw/react-codemirror'
import { javascript } from '@codemirror/lang-javascript'
import { StreamLanguage } from '@codemirror/language'
import { lua } from '@codemirror/legacy-modes/mode/lua'
import { Play } from 'lucide-react'
import { api, errText } from '../api'
import { cx } from '../util'

export type ScriptRule = { id: string; kind: string; name: string; enabled: boolean; severity: string; explain: string; lang?: string; hook?: string; source?: string; timeoutMs?: number }
type Try = { on: string; input: unknown; value: unknown; log?: string[]; ms: number; error?: string }

/** A script rule's code, with Try: run once on the newest real incident, recommendation or report. */
export default function ScriptEditor({ rule, onChange, disabled }: { rule: ScriptRule; onChange: (source: string) => void; disabled: boolean }) {
  const [t, setT] = useState<Try | null>(null)
  const [busy, setBusy] = useState(false)
  const [showIn, setShowIn] = useState(false)
  const run = async () => {
    setBusy(true)
    try {
      setT(await api.post<Try>('/admin/advisory/try', { rule }))
    } catch (e) {
      setT({ on: '', input: null, value: null, ms: 0, error: errText(e) })
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="mt-1 space-y-1">
      <div className="overflow-hidden rounded border border-slate-700 text-[12px]">
        <CodeMirror value={rule.source ?? ''} theme="dark" height="auto" minHeight="6rem" maxHeight="24rem" editable={!disabled} basicSetup={{ foldGutter: false, highlightActiveLine: false }}
          extensions={[rule.lang === 'lua' ? StreamLanguage.define(lua) : javascript()]} onChange={onChange} aria-label={`Code of ${rule.name}`} />
      </div>
      <div className="flex items-center gap-2 text-xs">
        <button className="btn h-7 text-xs" disabled={busy} onClick={run}><Play size={12} aria-hidden /> Try it</button>
        <span className="text-slate-500">runs once on the newest {rule.hook === 'report' ? 'report' : 'open incident'}, unsaved code included; nothing is changed</span>
      </div>
      {t && (
        <div className={cx('rounded border p-2 font-mono text-[11px]', t.error ? 'border-rose-700 bg-rose-950/30 text-rose-200' : 'border-slate-700 bg-slate-950 text-slate-200')}>
          <div className="mb-1 flex gap-2 font-sans text-slate-400">{t.on && <span>on {t.on}</span>}<span>{t.ms} ms</span>
            {t.input != null && <button className="underline" onClick={() => setShowIn(!showIn)}>{showIn ? 'hide' : 'show'} the input</button>}</div>
          {t.error ? <div>{t.error}</div> : <pre className="whitespace-pre-wrap">{t.value === null || t.value === undefined ? '(nothing: no suggestion)' : JSON.stringify(t.value, null, 2)}</pre>}
          {t.log?.map((l, i) => <div key={i} className="text-slate-400">log: {l}</div>)}
          {showIn && <pre className="mt-1 max-h-48 overflow-auto whitespace-pre-wrap text-slate-500">{JSON.stringify(t.input, null, 2)}</pre>}
        </div>
      )}
    </div>
  )
}
