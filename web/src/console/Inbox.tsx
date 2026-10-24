// The operational communications inbox: messages, BOLOs, officer-safety and administrative notices, like
// email inside the CAD. Four priorities, each with its own tone and whether it must be acknowledged.
// internal/cad/comms.go and internal/api/comms.go do the work on the server.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { toast } from 'sonner'
import { AlertTriangle, Check, Mail, Paperclip, Pencil, Reply, Send, VolumeX, X } from 'lucide-react'
import { api, errText } from '../api'
import { can, store, useStore } from '../store'
import { Field, Modal } from '../ui'
import { ago, cx, hhmm, useNow } from '../util'
import type { AgencyView, Comm, CommDraft, CommFile, CommKind, CommList, CommPriority, CommView, PriorityRule, Receipt, Session, View } from '../types'
import { playTone, soundReady, unlockSound } from '../tones'

// ---------- priorities: label and look (colour is never the only signal) ----------

const PRIORITIES: CommPriority[] = ['urgent', 'high', 'medium', 'low']
const prioLabel: Record<CommPriority, string> = { urgent: 'URGENT', high: 'HIGH', medium: 'MEDIUM', low: 'LOW' }
const prioClass: Record<CommPriority, string> = {
  urgent: 'bg-rose-500/25 text-rose-100 border-rose-500/50',
  high: 'bg-orange-500/20 text-orange-200 border-orange-500/40',
  medium: 'bg-amber-400/20 text-amber-200 border-amber-400/40',
  low: 'bg-slate-700 text-slate-300 border-slate-600',
}

/** A small label that always carries the word, so it reads without colour. */
function PriorityTag({ p }: { p: CommPriority }) {
  return <span className={cx('chip border font-semibold', prioClass[p])}>{prioLabel[p]}</span>
}

const stateLabel: Record<string, string> = { active: 'active', updated: 'updated', expired: 'expired', cancelled: 'cancelled', scheduled: 'scheduled' }

const ruleFor = (rules: PriorityRule[], p: CommPriority): PriorityRule | undefined => rules.find((r) => r.level === p)

// ---------- recipient tokens (To / CC) ----------

const GROUPS = ['ALL', 'ALL-FIRE', 'ALL-EMS', 'ALL-POLICE', 'DISPATCH']
const ROLES = ['@role:supervisor', '@role:dispatcher']
const DUTIES = ['@duty:calltaker', '@duty:police', '@duty:fire', '@duty:ems', '@duty:dispatcher']

// tokenLabel turns a recipient token into readable English for its chip.
function tokenLabel(t: string): string {
  if (t === 'ALL') return 'Everyone'
  if (t === 'ALL-FIRE') return 'All fire'
  if (t === 'ALL-EMS') return 'All EMS'
  if (t === 'ALL-POLICE') return 'All police'
  if (t === 'DISPATCH') return 'Dispatch'
  const m = /^@(\w+):(.+)$/.exec(t)
  if (m) {
    const [, k, v] = m
    if (k === 'agency') return `Agency ${v}`
    if (k === 'role') return `Role: ${v}`
    if (k === 'duty') return `On duty: ${v}`
    if (k === 'user') return `User ${v}`
    if (k === 'position') return `Position ${v}`
    if (k === 'incident') return `Incident ${v}`
    return t
  }
  return `Unit ${t}`
}

// ---------- the status-bar button ----------

/** The status bar's mail: unread messages and BOLOs, their tones, and an interruptive banner for the urgent ones. */
export function InboxButton({ view }: { view: View }) {
  const seq = useStore((s) => s.view)?.seq
  const [open, setOpen] = useState(false)
  const [openId, setOpenId] = useState<string | undefined>(undefined)
  const [compose, setCompose] = useState<Partial<Comm> | undefined>(undefined)
  const [list, setList] = useState<CommList | null>(null)
  const [silenced, setSilenced] = useState<Set<CommPriority>>(new Set())
  const seen = useRef<Set<string>>(new Set()) // items we have already sounded a first tone for
  const timers = useRef<Record<string, number>>({}) // per-item repeat timers

  const load = useCallback(() => {
    api.get<CommList>('/comms?folder=inbox').then(setList).catch((e) => toast.error(errText(e)))
  }, [])

  // Poll every 10 s and whenever the shared state's seq changes.
  useEffect(() => {
    load()
    const t = window.setInterval(load, 10000)
    return () => window.clearInterval(t)
  }, [load])
  useEffect(() => {
    load()
  }, [seq, load])

  // Open the dialog on a specific item, or the composer, from other parts of the console.
  useEffect(() => {
    const onOpen = (e: Event) => {
      setOpenId((e as CustomEvent<string>).detail)
      setCompose(undefined)
      setOpen(true)
    }
    const onCompose = (e: Event) => {
      setCompose((e as CustomEvent<Partial<Comm>>).detail ?? {})
      setOpenId(undefined)
      setOpen(true)
    }
    window.addEventListener('bd:open-comm', onOpen as EventListener)
    window.addEventListener('bd:compose', onCompose as EventListener)
    return () => {
      window.removeEventListener('bd:open-comm', onOpen as EventListener)
      window.removeEventListener('bd:compose', onCompose as EventListener)
    }
  }, [])

  const items = list?.items ?? []
  const rules = list?.priorities ?? []
  const unread = items.filter((i) => !i.openedByMe && i.state !== 'cancelled' && i.state !== 'expired')
  const toAck = items.filter((i) => i.needsAck && !i.ackedByMe)

  // Sound tones: a first tone for anything new and unread or needing an ack; then repeat until acknowledged.
  useEffect(() => {
    const live = new Set(items.map((i) => i.id))
    // Drop timers for items no longer present.
    for (const id of Object.keys(timers.current)) {
      if (!live.has(id)) {
        window.clearInterval(timers.current[id])
        delete timers.current[id]
      }
    }
    // New items sound once together: the tone of the highest priority among them, not one over another.
    const order: CommPriority[] = ['urgent', 'high', 'medium', 'low']
    let first: PriorityRule | undefined
    for (const it of items) {
      const rule = ruleFor(rules, it.priority)
      if (!rule || rule.tone === 'none') continue
      const needs = (it.needsAck && !it.ackedByMe) || (!it.openedByMe && it.state !== 'cancelled' && it.state !== 'expired')
      const quiet = rule.silenceable && silenced.has(it.priority)
      if (needs && !seen.current.has(it.id)) {
        seen.current.add(it.id)
        if (!quiet && (!first || order.indexOf(rule.level) < order.indexOf(first.level))) first = rule
      }
      // Repeat until acknowledged, when the rule asks for it.
      const wantRepeat = needs && (rule.repeat ?? 0) > 0 && !quiet
      if (wantRepeat && !timers.current[it.id]) {
        timers.current[it.id] = window.setInterval(() => playTone(rule.tone), (rule.repeat ?? 0) * 1000)
      }
      if ((!needs || quiet) && timers.current[it.id]) {
        window.clearInterval(timers.current[it.id])
        delete timers.current[it.id]
      }
    }
    if (first) playTone(first.tone)
  }, [items, rules, silenced])

  // Browsers keep a page silent until a person has used it: sound starts on the first key or click.
  useEffect(() => unlockSound(), [])
  const [soundOn, setSoundOn] = useState(soundReady())
  useEffect(() => {
    const on = () => setSoundOn(soundReady())
    window.addEventListener('bd:sound', on)
    const t = window.setInterval(on, 2000)
    return () => { window.removeEventListener('bd:sound', on); window.clearInterval(t) }
  }, [])

  // Stop every repeat timer on unmount.
  useEffect(() => () => { for (const id of Object.keys(timers.current)) window.clearInterval(timers.current[id]) }, [])

  const ack = async (id: string) => {
    try {
      await api.post(`/comms/${encodeURIComponent(id)}/ack`)
      load()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const openItem = (id: string) => {
    setOpenId(id)
    setCompose(undefined)
    setOpen(true)
  }

  // The interruptive items: urgent or high, not yet acknowledged, still in force.
  const interrupting = items.filter(
    (i) => (i.priority === 'urgent' || i.priority === 'high') && i.needsAck && !i.ackedByMe && i.state !== 'cancelled' && i.state !== 'expired',
  )

  const ariaLabel = `Messages and BOLOs: ${unread.length} unread${toAck.length ? `, ${toAck.length} to acknowledge` : ''}`

  return (
    <>
      <button
        className={cx('btn btn-ghost gap-1 px-1.5', toAck.length > 0 && 'text-rose-300')}
        onClick={() => { setOpenId(undefined); setCompose(undefined); setOpen(true) }}
        aria-label={ariaLabel}
        title="Messages and BOLOs"
      >
        <Mail size={14} aria-hidden />
        {unread.length > 0 && <span className="chip bg-sky-500/25 text-sky-100">{unread.length}</span>}
        {toAck.length > 0 && <span className="chip animate-pulse bg-rose-500/30 text-rose-100">{toAck.length} to ack</span>}
      </button>

      {interrupting.length > 0 && (
        <UrgentBanner items={interrupting} rules={rules} silenced={silenced} soundOn={soundOn} onSilence={(p) => setSilenced((s) => new Set(s).add(p))} onOpen={openItem} onAck={ack} />
      )}

      {open && (
        <InboxDialog
          view={view}
          compose={compose}
          openId={openId}
          onClose={() => { setOpen(false); setOpenId(undefined); setCompose(undefined); load() }}
        />
      )}
    </>
  )
}

// An interruptive banner for urgent/high communications that still need acknowledging.
function UrgentBanner({ items, rules, silenced, soundOn, onSilence, onOpen, onAck }: {
  items: CommView[]; rules: PriorityRule[]; silenced: Set<CommPriority>; soundOn: boolean; onSilence: (p: CommPriority) => void; onOpen: (id: string) => void; onAck: (id: string) => void
}) {
  return (
    <div role="alert" aria-label="Urgent communication" className="fixed inset-x-0 top-10 z-[60] mx-auto w-[min(96vw,56rem)] space-y-1 px-2">
      {!soundOn && items.some((it) => ruleFor(rules, it.priority)?.tone !== 'none') && (
        <button className="btn w-full justify-center border-amber-400 bg-amber-500/20 text-xs font-bold text-amber-100" onClick={() => playTone('tick')}>
          <VolumeX size={13} aria-hidden /> The browser is keeping this console silent: click here to turn the alert sounds on
        </button>
      )}
      {items.map((it) => {
        const rule = ruleFor(rules, it.priority)
        return (
          <div key={it.id} className={cx('flex items-center gap-2 rounded-b border-x border-b px-3 py-2 text-sm shadow-xl', prioClass[it.priority])}>
            <AlertTriangle size={16} aria-hidden />
            <PriorityTag p={it.priority} />
            <div className="min-w-0 flex-1">
              <div className="truncate font-semibold text-slate-50">{it.subject}</div>
              <div className="truncate text-xs text-slate-300">from {it.fromName || it.from}{it.fromDuty ? ` · ${it.fromDuty}` : ''}{it.fromAgency ? ` · ${it.fromAgency}` : ''}</div>
            </div>
            <button className="btn h-7 text-xs" onClick={() => onOpen(it.id)}>Open</button>
            <button className="btn btn-go h-7 text-xs" onClick={() => onAck(it.id)}><Check size={13} aria-hidden /> Acknowledge</button>
            {rule?.silenceable && !silenced.has(it.priority) && (
              <button className="btn btn-ghost h-7 px-1.5 text-xs" onClick={() => onSilence(it.priority)} aria-label={`Silence ${prioLabel[it.priority]} tone`} title="Silence the tone (it still needs acknowledging)"><VolumeX size={13} aria-hidden /></button>
            )}
          </div>
        )
      })}
    </div>
  )
}

// ---------- the dialog: folders, list, detail, composer ----------

type Folder = 'inbox' | 'drafts' | 'sent' | 'bolos' | 'archived'
const FOLDERS: { id: Folder; name: string }[] = [
  { id: 'inbox', name: 'Inbox' },
  { id: 'drafts', name: 'Drafts' },
  { id: 'sent', name: 'Sent' },
  { id: 'bolos', name: 'Active BOLOs' },
  { id: 'archived', name: 'Archived' },
]

type Detail = { versions: CommView[]; files: CommFile[] }

/** The messages and BOLOs window: folders, a list, a detail pane, and the composer. */
export function InboxDialog({ view, onClose, compose, openId }: { view: View; onClose: () => void; compose?: Partial<Comm>; openId?: string }) {
  const me = useStore((s) => s.me)
  const [folder, setFolder] = useState<Folder>('inbox')
  const [list, setList] = useState<CommList | null>(null)
  const [sel, setSel] = useState<string | undefined>(openId)
  const [detail, setDetail] = useState<Detail | null>(null)
  const [composing, setComposing] = useState<Partial<Comm> | null>(compose ?? null)
  const opened = useRef<Set<string>>(new Set())

  const load = useCallback((f: Folder) => {
    api.get<CommList>(`/comms?folder=${f}`).then(setList).catch((e) => toast.error(errText(e)))
  }, [])
  useEffect(() => { load(folder) }, [folder, load])

  // Open a specific item when asked (the bd:open-comm event, or a banner).
  useEffect(() => { if (openId) { setSel(openId); setComposing(null) } }, [openId])

  // Load the detail of the selected item and mark it opened the first time.
  useEffect(() => {
    if (!sel) { setDetail(null); return }
    let live = true
    api.get<Detail>(`/comms/${encodeURIComponent(sel)}`).then((d) => { if (live) setDetail(d) }).catch((e) => toast.error(errText(e)))
    if (!opened.current.has(sel)) {
      opened.current.add(sel)
      api.post(`/comms/${encodeURIComponent(sel)}/open`).then(() => load(folder)).catch(() => {})
    }
    return () => { live = false }
  }, [sel, folder, load])

  const kinds = list?.kinds ?? []
  const rows: CommView[] = list?.items ?? []
  const drafts: CommDraft[] = list?.drafts ?? []

  const refresh = () => load(folder)
  const ack = async (id: string) => {
    try { await api.post(`/comms/${encodeURIComponent(id)}/ack`); refresh(); if (sel === id) api.get<Detail>(`/comms/${encodeURIComponent(id)}`).then(setDetail).catch(() => {}) } catch (e) { toast.error(errText(e)) }
  }

  const title = 'Messages and BOLOs'
  return (
    <Modal open onClose={onClose} xl title={title} icon={<Mail size={16} className="text-sky-300" aria-hidden />}
      footer={<><button className="btn btn-go mr-auto" onClick={() => { setComposing({}); setSel(undefined) }}><Pencil size={13} aria-hidden /> Compose</button><button className="btn" onClick={onClose}>Close</button></>}>
      {composing ? (
        <Composer view={view} kinds={kinds} initial={composing} onClose={() => setComposing(null)} onDone={() => { setComposing(null); setFolder('sent'); load('sent') }} />
      ) : (
        <>
          <div role="tablist" aria-label="Folders" className="mb-3 flex flex-wrap gap-1 border-b border-slate-800 pb-2">
            {FOLDERS.map((f) => (
              <button
                key={f.id}
                role="tab"
                aria-selected={folder === f.id}
                className={cx('btn h-7 text-xs', folder === f.id ? 'border-sky-400 bg-sky-500/15 text-sky-100' : 'btn-ghost')}
                onClick={() => { setFolder(f.id); setSel(undefined) }}
              >
                {f.name}
              </button>
            ))}
          </div>

          <div className="grid gap-3 lg:grid-cols-[minmax(0,22rem)_1fr]">
            <div className="max-h-[56vh] overflow-y-auto" role="listbox" aria-label={FOLDERS.find((f) => f.id === folder)?.name}>
              {folder === 'drafts' ? (
                drafts.length === 0 ? <p className="p-2 text-sm text-slate-500">No drafts.</p> : (
                  <ul className="divide-y divide-slate-800">
                    {drafts.map((d) => (
                      <li key={d.id}>
                        <button className="w-full px-2 py-1.5 text-left hover:bg-slate-800/60" onClick={() => setComposing({ ...d.comm, id: d.id })}
                          aria-label={`Draft: ${d.comm.subject || 'untitled'}`}>
                          <div className="truncate text-sm text-slate-100">{d.comm.subject || '(no subject)'}</div>
                          <div className="text-xs text-slate-500">saved {ago(d.updated, Date.now())} ago</div>
                        </button>
                      </li>
                    ))}
                  </ul>
                )
              ) : rows.length === 0 ? (
                <p className="p-2 text-sm text-slate-500">Nothing here.</p>
              ) : (
                <ul className="divide-y divide-slate-800">
                  {rows.map((it) => {
                    const kind = kinds.find((k) => k.code === it.kind)
                    const name = it.fromName || it.from
                    return (
                      <li key={it.id} role="option" aria-selected={sel === it.id}>
                        <button
                          className={cx('w-full px-2 py-1.5 text-left hover:bg-slate-800/60', sel === it.id && 'bg-slate-800')}
                          onClick={() => { setSel(it.id); setComposing(null) }}
                          aria-label={`${prioLabel[it.priority]} ${kind?.name ?? it.kind}: ${it.subject}, from ${name}, ${stateLabel[it.state] ?? it.state}${!it.openedByMe ? ', unread' : ''}${it.needsAck && !it.ackedByMe ? ', needs acknowledgement' : ''}`}
                        >
                          <div className="flex items-center gap-1.5">
                            <PriorityTag p={it.priority} />
                            <span className="text-[10px] uppercase text-slate-400">{kind?.name ?? it.kind}</span>
                            {!it.openedByMe && <span className="ml-auto chip bg-sky-500/25 text-sky-100">new</span>}
                          </div>
                          <div className={cx('truncate text-sm', it.openedByMe ? 'text-slate-200' : 'font-semibold text-slate-50')}>{it.subject}</div>
                          <div className="flex items-center gap-1 truncate text-xs text-slate-500">
                            <span className="truncate">{name}{it.fromDuty ? ` · ${it.fromDuty}` : ''}{it.fromAgency ? ` · ${it.fromAgency}` : ''}</span>
                            <span className="ml-auto whitespace-nowrap font-mono">{hhmm(it.issued)}</span>
                          </div>
                          <div className="flex items-center gap-2 text-[10px] text-slate-500">
                            <span>{stateLabel[it.state] ?? it.state}</span>
                            {it.openedByMe && <span className="text-slate-400">opened</span>}
                            {it.ackedByMe && <span className="text-emerald-300">acknowledged</span>}
                          </div>
                        </button>
                      </li>
                    )
                  })}
                </ul>
              )}
            </div>

            <div className="max-h-[56vh] overflow-y-auto">
              {sel ? (
                <CommDetail id={sel} detail={detail} rows={rows} me={me} onAck={ack}
                  onUpdate={(c) => { setComposing(c); setSel(undefined) }}
                  onReply={(c) => { setComposing(c); setSel(undefined) }}
                  onCancel={refresh} onClose={() => setSel(undefined)} />
              ) : (
                <p className="p-2 text-sm text-slate-500">Select a message to read it.</p>
              )}
            </div>
          </div>
        </>
      )}
    </Modal>
  )
}

// A receipt line: who, their duty, their console/unit, and when.
function receiptLine(user: string, r: Receipt): string {
  const parts = [user]
  if (r.duty) parts.push(r.duty)
  if (r.console) parts.push(r.console)
  else if (r.unit) parts.push(r.unit)
  return `${parts.join(' · ')} at ${hhmm(r.at)}`
}

// ---------- the detail pane ----------

function CommDetail({ id, detail, rows, me, onAck, onUpdate, onReply, onCancel, onClose }: {
  id: string; detail: Detail | null; rows: CommView[]; me: Session | null
  onAck: (id: string) => void; onUpdate: (c: Partial<Comm>) => void; onReply: (c: Partial<Comm>) => void; onCancel: () => void; onClose: () => void
}) {
  const now = useNow()
  if (!detail) return <p className="p-2 text-sm text-slate-500">Loading…</p>
  const versions = detail.versions
  const cur = versions[versions.length - 1]
  const row = rows.find((r) => r.id === id)
  if (!cur) return <p className="p-2 text-sm text-slate-500">Not found.</p>
  const needsAck = row?.needsAck && !row.ackedByMe
  const supervisor = can.supervise(me)

  const cancel = async () => {
    const why = prompt('Why cancel this? The people it reached are told.')
    if (why == null) return
    try {
      await api.post('/comms', { of: id, cancelled: true, subject: cur.subject, why })
      toast.success('Cancelled')
      onCancel()
    } catch (e) {
      toast.error(errText(e))
    }
  }

  const reply = () => onReply({ to: [`@user:${cur.from}`], subject: cur.subject.startsWith('Re: ') ? cur.subject : `Re: ${cur.subject}`, kind: cur.kind, priority: cur.priority })
  const update = () => onUpdate({ of: id, kind: cur.kind, priority: cur.priority, subject: cur.subject, body: cur.body, to: cur.to, cc: cur.cc, incident: cur.incident, related: cur.related, plate: cur.plate, plateState: cur.plateState, vehicle: cur.vehicle, person: cur.person, description: cur.description, agency: cur.agency, effective: cur.effective, expires: cur.expires, noExpiry: cur.noExpiry })

  const isBolo = !!(cur.plate || cur.vehicle || cur.person || cur.description)
  const openers = Object.entries(cur.opened ?? {})
  const ackers = Object.entries(cur.acked ?? {})
  const showReceipts = cur.mine || supervisor

  return (
    <section className="space-y-3 p-1 text-sm" aria-label="Message detail">
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <PriorityTag p={cur.priority} />
            <span className="chip bg-slate-800 text-slate-300">{stateLabel[cur.state] ?? cur.state}</span>
            {cur.openedByMe && <span className="text-xs text-slate-400">opened</span>}
            {cur.ackedByMe && <span className="text-xs text-emerald-300">acknowledged</span>}
          </div>
          <h3 className="mt-1 text-base font-semibold text-slate-50">{cur.subject}</h3>
          <div className="text-xs text-slate-400">
            from {cur.fromName || cur.from}{cur.fromDuty ? ` · ${cur.fromDuty}` : ''}{cur.fromAgency ? ` · ${cur.fromAgency}` : ''} · {hhmm(cur.issued)} ({ago(cur.issued, now)} ago)
          </div>
        </div>
        <button className="btn btn-ghost h-6 px-1" onClick={onClose} aria-label="Close message"><X size={13} aria-hidden /></button>
      </div>

      <div className="flex flex-wrap items-center gap-2 border-y border-slate-800 py-2">
        {needsAck && <button className="btn btn-go h-7 text-xs" onClick={() => onAck(id)}><Check size={13} aria-hidden /> Acknowledge</button>}
        <button className="btn h-7 text-xs" onClick={reply}><Reply size={13} aria-hidden /> Reply</button>
        {cur.mine && cur.state !== 'cancelled' && <button className="btn h-7 text-xs" onClick={update}><Pencil size={13} aria-hidden /> Update</button>}
        {cur.mine && cur.state !== 'cancelled' && <button className="btn h-7 text-xs text-rose-300" onClick={cancel}><X size={13} aria-hidden /> Cancel</button>}
      </div>

      <div className="space-y-1 text-xs text-slate-400">
        <div><span className="text-slate-500">To:</span> {cur.to.map(tokenLabel).join(', ') || '—'}</div>
        {cur.cc && cur.cc.length > 0 && <div><span className="text-slate-500">CC:</span> {cur.cc.map(tokenLabel).join(', ')}</div>}
        {cur.incident && (
          <div>
            <span className="text-slate-500">Incident:</span>{' '}
            <button className="font-mono text-sky-300 underline" onClick={() => store.select(cur.incident!)}>{cur.incident}</button>
          </div>
        )}
        {cur.related && cur.related.length > 0 && (
          <div><span className="text-slate-500">Related:</span> {cur.related.map((n) => <button key={n} className="mr-1 font-mono text-sky-300 underline" onClick={() => store.select(n)}>{n}</button>)}</div>
        )}
        {(cur.effective || cur.expires || cur.noExpiry) && (
          <div><span className="text-slate-500">In force:</span> {cur.effective ? `from ${hhmm(cur.effective)}` : 'now'} {cur.noExpiry ? '· no expiry' : cur.expires ? `· until ${hhmm(cur.expires)}` : ''}</div>
        )}
      </div>

      {isBolo && (
        <div className="rounded border border-slate-800 bg-slate-900/50 p-2 text-xs">
          <div className="mb-1 font-semibold uppercase tracking-wider text-slate-400">BOLO</div>
          <dl className="grid grid-cols-[6rem_1fr] gap-x-2 gap-y-0.5">
            {cur.plate && <><dt className="text-slate-500">Plate</dt><dd className="text-slate-200">{cur.plate}{cur.plateState ? ` (${cur.plateState})` : ''}</dd></>}
            {cur.vehicle && <><dt className="text-slate-500">Vehicle</dt><dd className="text-slate-200">{cur.vehicle}</dd></>}
            {cur.person && <><dt className="text-slate-500">Person</dt><dd className="text-slate-200">{cur.person}</dd></>}
            {cur.description && <><dt className="text-slate-500">Describe</dt><dd className="whitespace-pre-wrap text-slate-200">{cur.description}</dd></>}
          </dl>
        </div>
      )}

      <div className="whitespace-pre-wrap text-slate-100">{cur.body}</div>

      {detail.files.length > 0 && (
        <div>
          <div className="mb-1 text-xs font-semibold uppercase tracking-wider text-slate-400">Attachments</div>
          <ul className="flex flex-wrap gap-2">
            {detail.files.filter((f) => !f.removed).map((f) => (
              <li key={f.id}>
                <a className="flex items-center gap-1 rounded border border-slate-800 p-1 text-xs text-sky-300 underline" href={`/api/comms/files/${encodeURIComponent(f.id)}`} target="_blank" rel="noreferrer">
                  {f.type.startsWith('image/')
                    ? <img src={`/api/comms/files/${encodeURIComponent(f.id)}`} alt={f.name} className="h-16 w-16 rounded object-cover" />
                    : <Paperclip size={12} aria-hidden />}
                  <span>{f.name}</span>
                </a>
              </li>
            ))}
          </ul>
        </div>
      )}

      {showReceipts && (openers.length > 0 || ackers.length > 0) && (
        <div className="grid gap-2 border-t border-slate-800 pt-2 text-xs sm:grid-cols-2">
          <div>
            <div className="font-semibold text-slate-400">Opened by</div>
            {openers.length === 0 ? <div className="text-slate-500">no one yet</div> : <ul className="text-slate-300">{openers.map(([u, r]) => <li key={u}>{receiptLine(u, r)}</li>)}</ul>}
          </div>
          <div>
            <div className="font-semibold text-slate-400">Acknowledged by</div>
            {ackers.length === 0 ? <div className="text-slate-500">no one yet</div> : <ul className="text-slate-300">{ackers.map(([u, r]) => <li key={u}>{receiptLine(u, r)}</li>)}</ul>}
          </div>
        </div>
      )}

      {versions.length > 1 && (
        <div className="border-t border-slate-800 pt-2">
          <div className="mb-1 text-xs font-semibold uppercase tracking-wider text-slate-400">History</div>
          <ol className="space-y-2">
            {versions.map((v, i) => (
              <li key={i} className="rounded border border-slate-800 p-2 text-xs">
                <div className="flex flex-wrap items-center gap-1.5 text-slate-400">
                  <span className="font-mono">{hhmm(v.issued)}</span>
                  <span>by {v.fromName || v.from}</span>
                  <PriorityTag p={v.priority} />
                  {i === versions.length - 1 && <span className="chip bg-slate-800 text-slate-300">current</span>}
                </div>
                {v.why && <div className="mt-0.5 italic text-slate-400">why: {v.why}</div>}
                <div className="mt-0.5 whitespace-pre-wrap text-slate-200">{v.body}</div>
              </li>
            ))}
          </ol>
        </div>
      )}
    </section>
  )
}

// ---------- the composer ----------

type Attachment = CommFile

function Composer({ view, kinds, initial, onClose, onDone }: {
  view: View; kinds: CommKind[]; initial: Partial<Comm>; onClose: () => void; onDone: () => void
}) {
  const firstKind = kinds[0]?.code ?? ''
  const [draftId, setDraftId] = useState<string | undefined>(initial.id)
  const isUpdate = !!initial.of
  const [kind, setKind] = useState(initial.kind ?? firstKind)
  const kindDef = kinds.find((k) => k.code === kind)
  const [priority, setPriority] = useState<CommPriority>(initial.priority ?? kindDef?.priority ?? 'medium')
  const [subject, setSubject] = useState(initial.subject ?? '')
  const [body, setBody] = useState(initial.body ?? '')
  const [to, setTo] = useState<string[]>(initial.to ?? [])
  const [cc, setCc] = useState<string[]>(initial.cc ?? [])
  const [agency, setAgency] = useState(initial.agency ?? '')
  const [incident, setIncident] = useState(initial.incident ?? '')
  const [related, setRelated] = useState<string[]>(initial.related ?? [])
  const [plate, setPlate] = useState(initial.plate ?? '')
  const [plateState, setPlateState] = useState(initial.plateState ?? '')
  const [vehicle, setVehicle] = useState(initial.vehicle ?? '')
  const [person, setPerson] = useState(initial.person ?? '')
  const [description, setDescription] = useState(initial.description ?? '')
  const [effective, setEffective] = useState(toLocal(initial.effective))
  const [noExpiry, setNoExpiry] = useState<boolean>(initial.noExpiry ?? false)
  const [expires, setExpires] = useState(toLocal(initial.expires))
  const [why, setWhy] = useState(initial.why ?? '')
  const [files, setFiles] = useState<Attachment[]>([])
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const [agencies, setAgencies] = useState<AgencyView['agencies']>([])
  useEffect(() => {
    api.get<AgencyView>('/agencies').then((r) => setAgencies(r.agencies ?? [])).catch(() => {})
  }, [])

  // When the kind changes, follow its default priority (unless the user already picked).
  const pickKind = (code: string) => {
    setKind(code)
    const def = kinds.find((k) => k.code === code)
    if (def?.priority) setPriority(def.priority)
  }

  const isBolo = /bolo|officer|safety/i.test(kind) || !!(plate || vehicle || person || description)

  // Recipient suggestions for the datalists.
  const suggestions = useMemo(() => {
    const units = view.units.map((u) => u.id)
    const incs = view.incidents.filter((i) => i.status !== 'closed').map((i) => `@incident:${i.number}`)
    const ags = agencies.map((a) => `@agency:${a.code}`)
    return [...GROUPS, ...units, ...ags, ...ROLES, ...DUTIES, ...incs]
  }, [view.units, view.incidents, agencies])

  const openIncidents = view.incidents.filter((i) => i.status !== 'closed')

  const upload = async (list: FileList) => {
    for (const f of Array.from(list)) {
      try {
        const r = await fetch('/api/comms/files?name=' + encodeURIComponent(f.name), { method: 'POST', body: f, headers: { 'X-BitDispatcher': '1' }, credentials: 'same-origin' })
        if (!r.ok) throw new Error((await r.text()) || r.statusText)
        const cf = (await r.json()) as CommFile
        setFiles((xs) => [...xs, cf])
      } catch (e) {
        toast.error(`${f.name}: ${errText(e)}`)
      }
    }
  }

  const commBody = (): Partial<Comm> => ({
    of: initial.of,
    kind,
    priority,
    subject,
    body,
    to,
    cc: cc.length ? cc : undefined,
    agency: agency || undefined,
    incident: incident || undefined,
    related: related.length ? related : undefined,
    plate: isBolo ? plate || undefined : undefined,
    plateState: isBolo ? plateState || undefined : undefined,
    vehicle: isBolo ? vehicle || undefined : undefined,
    person: isBolo ? person || undefined : undefined,
    description: isBolo ? description || undefined : undefined,
    effective: fromLocal(effective),
    expires: noExpiry ? undefined : fromLocal(expires),
    noExpiry: noExpiry || undefined,
    why: isUpdate ? why : undefined,
    files: files.length ? files.map((f) => f.id) : undefined,
  })

  const saveDraft = async () => {
    setBusy(true); setErr('')
    try {
      const r = await api.put<{ id: string }>('/comms/drafts', { id: draftId, comm: commBody() })
      if (r?.id) setDraftId(r.id)
      toast.success('Draft saved')
    } catch (e) {
      setErr(errText(e))
    } finally {
      setBusy(false)
    }
  }

  const send = async () => {
    if (isUpdate && !why.trim()) { setErr('Say why you are updating this.'); return }
    setBusy(true); setErr('')
    try {
      const path = draftId && !isUpdate ? `/comms?draft=${encodeURIComponent(draftId)}` : '/comms'
      await api.post(path, commBody())
      toast.success(isUpdate ? 'Update sent' : 'Sent')
      onDone()
    } catch (e) {
      setErr(errText(e))
    } finally {
      setBusy(false)
    }
  }

  const boloNoTo = isBolo && to.length === 0

  return (
    <div className="space-y-3">
      <div className="flex items-center gap-2">
        <h3 className="text-sm font-semibold text-slate-100">{isUpdate ? 'Update a communication' : 'Compose'}</h3>
        <button className="btn btn-ghost ml-auto h-6 px-1" onClick={onClose} aria-label="Back to the list"><X size={13} aria-hidden /></button>
      </div>

      {isUpdate && <p className="rounded border border-sky-700 bg-sky-950/30 p-2 text-xs text-sky-200">This updates an earlier communication. Everyone who has it gets the new version.</p>}

      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Kind" htmlFor="cm-kind">
          <select id="cm-kind" className="input" value={kind} onChange={(e) => pickKind(e.target.value)}>
            {kinds.map((k) => <option key={k.code} value={k.code}>{k.name}</option>)}
          </select>
        </Field>
        <Field label="Priority" htmlFor="cm-prio">
          <select id="cm-prio" className="input" value={priority} onChange={(e) => setPriority(e.target.value as CommPriority)}>
            {PRIORITIES.map((p) => <option key={p} value={p}>{prioLabel[p]}</option>)}
          </select>
        </Field>
      </div>

      <Field label="Subject" htmlFor="cm-subject"><input id="cm-subject" className="input" value={subject} onChange={(e) => setSubject(e.target.value)} /></Field>

      <Field label="Message" htmlFor="cm-body" hint="The essential warning goes in the text, not only in an attachment.">
        <textarea id="cm-body" rows={5} className="input" value={body} onChange={(e) => setBody(e.target.value)} />
      </Field>

      <TokenInput id="cm-to" label="To" value={to} onChange={setTo} suggestions={suggestions} />
      <TokenInput id="cm-cc" label="CC" value={cc} onChange={setCc} suggestions={suggestions} />

      {boloNoTo && <p className="rounded border border-amber-500/40 bg-amber-500/10 p-2 text-xs text-amber-200"><AlertTriangle size={12} className="mr-1 inline" aria-hidden />With no recipients, this BOLO goes to every CAD position and terminal of the owning agency.</p>}

      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Owning agency" htmlFor="cm-agency" hint="Empty = your working agency.">
          <select id="cm-agency" className="input" value={agency} onChange={(e) => setAgency(e.target.value)}>
            <option value="">My working agency</option>
            {agencies.map((a) => <option key={a.code} value={a.code}>{a.name} ({a.code})</option>)}
          </select>
        </Field>
        <Field label="Incident" htmlFor="cm-incident">
          <select id="cm-incident" className="input" value={incident} onChange={(e) => setIncident(e.target.value)}>
            <option value="">None</option>
            {openIncidents.map((i) => <option key={i.number} value={i.number}>{i.number} — {i.type}</option>)}
          </select>
        </Field>
      </div>

      <TokenInput id="cm-related" label="Related incidents" value={related} onChange={setRelated} suggestions={openIncidents.map((i) => i.number)} placeholder="Incident number" />

      {isBolo && (
        <fieldset className="space-y-2 rounded border border-slate-800 p-2">
          <legend className="px-1 text-xs font-semibold uppercase tracking-wider text-slate-400">BOLO / officer safety</legend>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Plate" htmlFor="cm-plate"><input id="cm-plate" className="input" value={plate} onChange={(e) => setPlate(e.target.value)} /></Field>
            <Field label="Plate state" htmlFor="cm-platestate"><input id="cm-platestate" className="input" value={plateState} onChange={(e) => setPlateState(e.target.value)} /></Field>
          </div>
          <Field label="Vehicle" htmlFor="cm-vehicle"><input id="cm-vehicle" className="input" value={vehicle} onChange={(e) => setVehicle(e.target.value)} /></Field>
          <Field label="Person" htmlFor="cm-person"><input id="cm-person" className="input" value={person} onChange={(e) => setPerson(e.target.value)} /></Field>
          <Field label="Description" htmlFor="cm-description"><textarea id="cm-description" rows={2} className="input" value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
        </fieldset>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Effective" htmlFor="cm-effective"><input id="cm-effective" type="datetime-local" className="input" value={effective} onChange={(e) => setEffective(e.target.value)} /></Field>
        <div>
          <Field label="Expires" htmlFor="cm-expires"><input id="cm-expires" type="datetime-local" className="input" value={expires} disabled={noExpiry} onChange={(e) => setExpires(e.target.value)} /></Field>
          <label className="mt-1 flex items-center gap-2 text-xs text-slate-300"><input type="checkbox" checked={noExpiry} onChange={(e) => setNoExpiry(e.target.checked)} /> No expiration</label>
        </div>
      </div>

      {isUpdate && <Field label="Why updating" htmlFor="cm-why" hint="Required when updating."><input id="cm-why" className="input" value={why} onChange={(e) => setWhy(e.target.value)} /></Field>}

      <div>
        <label className="label" htmlFor="cm-files">Attachments</label>
        <input id="cm-files" type="file" multiple className="input" onChange={(e) => { if (e.target.files) upload(e.target.files); e.target.value = '' }} />
        {files.length > 0 && (
          <ul className="mt-2 space-y-1 text-xs">
            {files.map((f) => (
              <li key={f.id} className="flex items-center gap-2 rounded border border-slate-800 p-1">
                <Paperclip size={12} aria-hidden />
                <span className="truncate text-slate-200">{f.name}</span>
                <span className="text-slate-500">{Math.round(f.size / 1024)} KB · {f.type || 'file'}</span>
                <button className="btn btn-ghost ml-auto h-5 px-1" onClick={() => setFiles((xs) => xs.filter((x) => x.id !== f.id))} aria-label={`Remove ${f.name}`}><X size={12} aria-hidden /></button>
              </li>
            ))}
          </ul>
        )}
      </div>

      {err && <p className="rounded border border-rose-700 bg-rose-950/30 p-2 text-xs text-rose-200" role="alert">{err}</p>}

      <div className="flex items-center gap-2 border-t border-slate-800 pt-2">
        <button className="btn" disabled={busy} onClick={saveDraft}>Save draft</button>
        <button className="btn btn-go ml-auto" disabled={busy} onClick={send}><Send size={13} aria-hidden /> Send</button>
      </div>
    </div>
  )
}

// ---------- a token input with suggestions and removable chips ----------

function TokenInput({ id, label, value, onChange, suggestions, placeholder }: {
  id: string; label: string; value: string[]; onChange: (v: string[]) => void; suggestions: string[]; placeholder?: string
}) {
  const [text, setText] = useState('')
  const add = (raw: string) => {
    const t = raw.trim()
    if (!t) return
    if (!value.includes(t)) onChange([...value, t])
    setText('')
  }
  const remove = (t: string) => onChange(value.filter((x) => x !== t))
  return (
    <div>
      <label className="label" htmlFor={id}>{label}</label>
      {value.length > 0 && (
        <ul className="mb-1 flex flex-wrap gap-1" aria-label={`${label} recipients`}>
          {value.map((t) => (
            <li key={t}>
              <span className="chip bg-slate-800 text-slate-200">
                {tokenLabel(t)}
                <button type="button" className="ml-1 text-slate-400 hover:text-slate-100" onClick={() => remove(t)} aria-label={`Remove ${tokenLabel(t)}`}>×</button>
              </span>
            </li>
          ))}
        </ul>
      )}
      <input
        id={id}
        className="input"
        list={`${id}-opts`}
        value={text}
        placeholder={placeholder ?? 'Add a recipient and press Enter'}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ',') { e.preventDefault(); add(text) } }}
        onBlur={() => add(text)}
      />
      <datalist id={`${id}-opts`}>{suggestions.map((sug) => <option key={sug} value={sug}>{tokenLabel(sug)}</option>)}</datalist>
    </div>
  )
}

// ---------- datetime-local helpers ----------

// toLocal turns an ISO instant into the value a datetime-local input wants (local time, no zone).
function toLocal(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// fromLocal turns a datetime-local value back into an ISO instant.
function fromLocal(local: string): string | undefined {
  if (!local) return undefined
  const d = new Date(local)
  if (isNaN(d.getTime())) return undefined
  return d.toISOString()
}
