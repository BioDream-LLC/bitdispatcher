import { useState } from 'react'
import { toast } from 'sonner'
import { Building2, Plus, Trash2 } from 'lucide-react'
import { api, ApiError, errText } from '../api'
import AddressInput from '../AddressInput'
import { Field, Modal } from '../ui'
import type { View } from '../types'
import { Page, Table } from './Admin'

type Draft = { id: string; name: string; address: string; lat: string; lon: string; isNew: boolean }

export default function Stations({ view }: { view: View }) {
  const [d, setD] = useState<Draft | null>(null)
  const [err, setErr] = useState<ApiError | null>(null)
  const open = (id?: string) => {
    const s = view.stations.find((x) => x.id === id)
    setErr(null)
    setD(s ? { id: s.id, name: s.name, address: s.address ?? '', lat: String(s.lat), lon: String(s.lon), isNew: false } : { id: '', name: '', address: '', lat: '', lon: '', isNew: true })
  }
  const save = async () => {
    if (!d) return
    try {
      await api.put(`/admin/stations/${encodeURIComponent(d.id.trim().toUpperCase())}`, { name: d.name, address: d.address, lat: Number(d.lat), lon: Number(d.lon) })
      toast.success(`Station ${d.id.toUpperCase()} saved`)
      setD(null)
    } catch (e) {
      setErr(e instanceof ApiError ? e : new ApiError(0, errText(e)))
    }
  }
  return (
    <Page
      title="Stations"
      intro="Where units are based. A unit without a current AVL position is placed at its station for the recommendation, and the map says so; the Situation panel shows which stations have no available unit."
      actions={<button className="btn btn-go" onClick={() => open()}><Plus size={14} aria-hidden /> Add station</button>}
    >
      <Table head={['ID', 'Name', 'Address', 'Position', 'Units']}>
        {view.stations.map((s) => {
          const here = view.units.filter((u) => u.station === s.id)
          return (
            <tr key={s.id} className="cursor-pointer hover:bg-slate-800/50" onClick={() => open(s.id)}>
              <td className="px-3 py-1.5 font-mono font-bold text-slate-100">{s.id}</td>
              <td className="px-3 text-slate-200">{s.name}</td>
              <td className="px-3 text-slate-400">{s.address}</td>
              <td className="px-3 font-mono text-xs text-slate-400">{s.lat.toFixed(5)}, {s.lon.toFixed(5)}</td>
              <td className="px-3 font-mono text-xs text-slate-300">{here.map((u) => u.id).join(' ')}</td>
            </tr>
          )
        })}
      </Table>
      {d && (
        <Modal
          open
          onClose={() => setD(null)}
          title={d.isNew ? 'New station' : `Station ${d.id}`}
          icon={<Building2 size={16} className="text-sky-300" aria-hidden />}
          footer={
            <>
              {!d.isNew && (
                <button className="btn mr-auto text-rose-300" onClick={async () => { if (!confirm(`Remove station ${d.id}?`)) return; try { await api.del(`/admin/stations/${d.id}`); setD(null) } catch (e) { toast.error(errText(e)) } }}>
                  <Trash2 size={13} aria-hidden /> Remove
                </button>
              )}
              <button className="btn" onClick={() => setD(null)}>Cancel</button>
              <button className="btn btn-go" onClick={save}>Save</button>
            </>
          }
        >
          <div className="grid gap-3 sm:grid-cols-2">
            {err && <p className="sm:col-span-2 rounded bg-rose-950/50 px-2 py-1 text-sm text-rose-200">{err.message}</p>}
            <Field label="ID" htmlFor="sid"><input id="sid" className="input font-mono uppercase" disabled={!d.isNew} value={d.id} onChange={(e) => setD({ ...d, id: e.target.value })} /></Field>
            <Field label="Name" htmlFor="sn"><input id="sn" className="input" value={d.name} onChange={(e) => setD({ ...d, name: e.target.value })} /></Field>
            <div className="sm:col-span-2"><Field label="Address" htmlFor="sa" hint="Pick it from the recorded map and the position fills in"><AddressInput id="sa" value={d.address} onChange={(v) => setD((x) => x && { ...x, address: v })} onPick={(r) => setD((x) => x && { ...x, lat: r.lat.toFixed(6), lon: r.lon.toFixed(6) })} placeholder="3401 Williamson Rd" /></Field></div>
            <Field label="Latitude" htmlFor="slat" error={err?.field === 'lat' ? err.message : undefined}><input id="slat" className="input font-mono" value={d.lat} onChange={(e) => setD({ ...d, lat: e.target.value })} /></Field>
            <Field label="Longitude" htmlFor="slon"><input id="slon" className="input font-mono" value={d.lon} onChange={(e) => setD({ ...d, lon: e.target.value })} /></Field>
          </div>
        </Modal>
      )}
    </Page>
  )
}
