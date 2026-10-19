import type { ReactNode } from 'react'
import * as Dialog from '@radix-ui/react-dialog'
import * as Tooltip from '@radix-ui/react-tooltip'
import { X } from 'lucide-react'
import { cx, prio, statusMeta } from './util'

export function Logo({ size = 22 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden>
      <defs>
        <linearGradient id="lg" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#f43f5e" />
          <stop offset="1" stopColor="#f97316" />
        </linearGradient>
      </defs>
      <path d="M16 3l11 4.5v8.2C27 23 22 27.4 16 29 10 27.4 5 23 5 15.7V7.5z" fill="url(#lg)" />
      <path d="M11 16h10M16 11v10" stroke="#0b0f17" strokeWidth="3" strokeLinecap="round" />
    </svg>
  )
}

export function Tip({ text, children, side = 'top' }: { text: ReactNode; children: ReactNode; side?: 'top' | 'bottom' | 'left' | 'right' }) {
  return (
    <Tooltip.Root>
      <Tooltip.Trigger asChild>{children}</Tooltip.Trigger>
      <Tooltip.Portal>
        <Tooltip.Content
          side={side}
          sideOffset={4}
          className="z-50 max-w-xs rounded-md border border-slate-700 bg-slate-900 px-2 py-1.5 text-xs text-slate-200 shadow-xl"
        >
          {text}
        </Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
  )
}

export function Modal({ open, onClose, title, icon, children, footer, wide, xl }: {
  open: boolean; onClose: () => void; title: ReactNode; icon?: ReactNode; children: ReactNode; footer?: ReactNode; wide?: boolean; xl?: boolean
}) {
  return (
    <Dialog.Root open={open} onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-40 bg-black/60 backdrop-blur-[2px]" />
        <Dialog.Content
          className={cx('panel fixed left-1/2 top-[8vh] z-50 max-h-[84vh] w-[94vw] -translate-x-1/2 overflow-hidden rounded-xl shadow-2xl', xl ? 'max-w-6xl' : wide ? 'max-w-3xl' : 'max-w-lg')}
          aria-describedby={undefined}
        >
          <div className="flex items-center gap-2 border-b border-slate-800 px-4 py-3">
            {icon}
            <Dialog.Title className="font-semibold text-slate-50">{title}</Dialog.Title>
            <Dialog.Close className="btn btn-ghost ml-auto px-1.5" aria-label="Close">
              <X size={16} aria-hidden />
            </Dialog.Close>
          </div>
          <div className={cx('overflow-y-auto p-4', xl ? 'max-h-[72vh]' : 'max-h-[66vh]')}>{children}</div>
          {footer && <div className="flex items-center justify-end gap-2 border-t border-slate-800 px-4 py-3">{footer}</div>}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

export function StatusChip({ status, big }: { status: string; big?: boolean }) {
  const m = statusMeta(status)
  return (
    <span className={cx('chip font-mono', m.bg, m.fg, big && 'h-5 px-1.5 text-xs')} title={m.label}>
      {m.code}
    </span>
  )
}

export function Priority({ p, onClick }: { p: number; onClick?: () => void }) {
  const m = prio(p)
  const cls = cx('chip font-mono', m.bg, m.fg)
  return onClick ? (
    <button type="button" className={cx(cls, 'cursor-pointer hover:brightness-110')} onClick={onClick} title={`Priority ${p}: change`}>
      P{p}
    </button>
  ) : (
    <span className={cls} title={`Priority ${p}`}>P{p}</span>
  )
}

export function Field({ label, htmlFor, hint, error, children }: { label: string; htmlFor: string; hint?: ReactNode; error?: string; children: ReactNode }) {
  return (
    <div>
      <label className="label" htmlFor={htmlFor}>{label}</label>
      {children}
      {error ? <p className="mt-1 text-xs text-rose-300">{error}</p> : hint ? <p className="mt-1 text-xs text-slate-500">{hint}</p> : null}
    </div>
  )
}
