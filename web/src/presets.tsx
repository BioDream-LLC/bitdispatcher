import { useCallback, useEffect, useRef, useState } from 'react'
import { Lock, LockOpen, Save, Undo2, Building } from 'lucide-react'
import { toast } from 'sonner'
import { api, errText } from './api'
import { can, useStore } from './store'
import { useWorkspace, type Workspace } from './workspace'
import type { PresetOut } from './types'

// Saved duty presets: a person's workspace for each duty is kept with their account on the server, so it
// loads by itself at any console when they sign in for that duty. It stays locked in normal work; a change
// is Edit preset, arrange, Save changes (which locks it again). A trainee works the agency's assigned
// preset and has no layout controls; a protected preset is changed only by administrators.

export type PresetMeta = {
  loaded: boolean
  source: PresetOut['source']
  mayEdit: boolean
  why?: string
  trainee: boolean
  editing: boolean
  edit: () => void
  save: () => Promise<void>
  cancel: () => void
  saveAssigned: () => Promise<void>
}

export function usePresetWorkspace(user: string, duty: string): [Workspace, (w: Workspace) => void, PresetMeta] {
  const [ws, setLocal] = useWorkspace(user, duty)
  const [out, setOut] = useState<PresetOut | null>(null)
  const [editing, setEditing] = useState(false)
  const saved = useRef<Workspace | null>(null)
  const me = useStore((s) => s.me)

  const adopt = useCallback((o: PresetOut) => {
    setOut(o)
    const w = o.preset?.workspace as Workspace | undefined
    if (w && typeof w === 'object' && 'root' in w) {
      const locked = { ...w, locked: true }
      saved.current = locked
      setLocal(locked)
    } else {
      saved.current = null
    }
  }, [user, duty])

  useEffect(() => {
    if (!user || !duty) return
    let live = true
    api.get<PresetOut>(`/me/preset?duty=${encodeURIComponent(duty)}`).then((o) => live && adopt(o)).catch(() => {})
    return () => { live = false }
  }, [user, duty])

  const mayEdit = !!out?.mayEdit
  // In normal work the layout stays locked: unlocking is Edit preset, and only for those who may.
  const set = (w: Workspace) => {
    if (!editing) {
      // Any change to a locked preset starts editing it (for those who may), never a silent change.
      if (!mayEdit) {
        toast.error(out?.why || 'This layout is locked')
        return
      }
      setEditing(true)
      w = { ...w, locked: false }
    }
    setLocal(w)
  }
  const meta: PresetMeta = {
    loaded: !!out,
    source: out?.source ?? '',
    mayEdit,
    why: out?.why,
    trainee: !!out?.trainee,
    editing,
    edit: () => {
      if (!mayEdit) return void toast.error(out?.why || 'This layout is locked')
      setEditing(true)
      setLocal({ ...ws, locked: false })
    },
    save: async () => {
      const w = { ...ws, locked: true }
      try {
        await api.put('/me/preset', { duty, workspace: w, monitors: monitors() })
        saved.current = w
        setLocal(w)
        setEditing(false)
        setOut((o) => (o ? { ...o, source: 'own' } : o))
        toast.success('Preset saved and locked: it opens at any console you sign in to for this duty')
      } catch (e) {
        toast.error(errText(e))
      }
    },
    cancel: () => {
      setEditing(false)
      setLocal(saved.current ?? { ...ws, locked: true })
    },
    saveAssigned: async () => {
      if (!can.admin(me)) return
      try {
        await api.put(`/admin/presets/assigned/${encodeURIComponent(duty)}`, { workspace: { ...ws, locked: true }, monitors: monitors() })
        toast.success(`Saved as the agency's preset for ${duty}: trainees and people without their own start from it`)
      } catch (e) {
        toast.error(errText(e))
      }
    },
  }
  return [ws, set, meta]
}

function monitors() {
  return [{ width: window.screen.width, height: window.screen.height }]
}

/** The preset controls in the status bar: locked (Edit preset), or editing (Save changes, Cancel). */
export function PresetControls({ meta }: { meta: PresetMeta }) {
  const me = useStore((s) => s.me)
  if (!meta.editing) {
    return (
      <button type="button" className="btn btn-ghost px-1.5" onClick={meta.edit} disabled={!meta.mayEdit}
        aria-label={meta.mayEdit ? 'Layout locked: edit it' : `Layout locked: ${meta.why ?? 'not yours to change'}`}
        title={meta.mayEdit ? `Your ${meta.source === 'own' ? 'saved' : meta.source === 'assigned' ? "agency's" : 'standard'} duty preset is locked. Click to edit it, then Save & lock.` : meta.why}>
        <Lock size={14} aria-hidden />
      </button>
    )
  }
  return (
    <span className="flex items-center gap-0.5 rounded bg-amber-500/15 px-1">
      <LockOpen size={12} className="text-amber-300" aria-hidden />
      <span className="text-[10px] font-semibold text-amber-200">Editing layout</span>
      <button type="button" className="btn h-6 gap-1 px-1.5 text-[11px]" onClick={meta.save} aria-label="Save changes and lock"><Save size={12} aria-hidden />Save &amp; lock</button>
      <button type="button" className="btn btn-ghost h-6 px-1.5 text-[11px]" onClick={meta.cancel} aria-label="Cancel the changes"><Undo2 size={12} aria-hidden /></button>
      {can.admin(me) && <button type="button" className="btn btn-ghost h-6 px-1.5 text-[11px]" onClick={meta.saveAssigned} aria-label="Save as the agency's layout for this duty" title="Save as the agency's preset for this duty: trainees and people without their own start from it"><Building size={12} aria-hidden /></button>}
    </span>
  )
}
