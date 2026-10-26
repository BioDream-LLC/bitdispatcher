import { useState } from 'react'
import { Keyboard } from 'lucide-react'
import { api } from '../api'
import { cx } from '../util'
import type { KeyTest, KeyTestsResponse } from './types'
import { Empty, Panel, dateTime, usePoll } from './common'

/** KeyTests is tab 7: each workstation's latest key test, so a laptop that sends F2 to its volume
 * control shows up here, with the combination that works, instead of at the first emergency call. */
export default function KeyTests() {
  const [items, setItems] = useState<KeyTest[]>([])
  usePoll(async () => {
    const r = await api.get<KeyTestsResponse>('/admin/keytests')
    setItems(r.items ?? [])
  }, [])

  return (
    <div className="space-y-3">
      <p className="text-sm text-slate-400">The latest key test for each position and duty. A failed key lists what arrived instead and the combination to use.</p>
      <Panel icon={<Keyboard size={16} className="text-sky-300" aria-hidden />} title={`Workstation key tests (${items.length})`}>
        {items.length === 0 ? (
          <Empty>No key tests recorded yet.</Empty>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-xs uppercase tracking-wider text-slate-500">
                  <th className="px-2 py-1">Console</th>
                  <th className="px-2 py-1">Duty</th>
                  <th className="px-2 py-1">Profile</th>
                  <th className="px-2 py-1">Browser</th>
                  <th className="px-2 py-1">Passed</th>
                  <th className="px-2 py-1">Results</th>
                  <th className="px-2 py-1">When / by</th>
                </tr>
              </thead>
              <tbody>
                {items.map((t) => {
                  const failed = t.results.filter((r) => !r.ok)
                  return (
                    <tr key={t.console + '|' + t.duty} className="border-t border-slate-800 align-top">
                      <td className="px-2 py-1.5 font-mono">{t.console}</td>
                      <td className="px-2 py-1.5">{t.duty || '—'}</td>
                      <td className="px-2 py-1.5">{t.profile || '—'}</td>
                      <td className="px-2 py-1.5 text-xs text-slate-400">{t.browser || '—'}{t.platform ? ` · ${t.platform}` : ''}</td>
                      <td className="px-2 py-1.5">
                        <span className={cx('chip', t.passed ? 'bg-emerald-500/20 text-emerald-200' : 'bg-rose-500/20 text-rose-200')}>
                          {t.passed ? 'passed' : `${failed.length} failed`}
                        </span>
                      </td>
                      <td className="px-2 py-1.5 text-xs">
                        {t.results.length === 0 ? (
                          '—'
                        ) : failed.length === 0 ? (
                          <span className="text-slate-400">all {t.results.length} keys reach bitdispatcher</span>
                        ) : (
                          <ul className="space-y-0.5">
                            {failed.map((r, i) => (
                              <li key={i} className="text-rose-200">
                                <span className="font-mono">{r.key}</span> ({r.action}){r.got ? `, arrived as ${r.got}` : ''}{r.fix ? ` — use ${r.fix}` : ''}
                              </li>
                            ))}
                          </ul>
                        )}
                      </td>
                      <td className="px-2 py-1.5 text-xs text-slate-400">{dateTime(t.at)}{t.by ? ` · ${t.by}` : ''}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </Panel>
    </div>
  )
}
