import { lazy, Suspense, useEffect } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { Toaster } from 'sonner'
import * as Tooltip from '@radix-ui/react-tooltip'
import { checkMe, connect, disconnect, useStore } from './store'
import Login from './Login'
import Console from './Console'
import Admin from './admin/Admin'
import Mdt from './mdt/Mdt'
import { Help } from './Help'
import Rms from './rms/Rms'

const Board = lazy(() => import('./console/Board'))

export default function App() {
  const me = useStore((s) => s.me)
  const checked = useStore((s) => s.meChecked)
  useEffect(() => {
    checkMe()
  }, [])
  useEffect(() => {
    if (me) connect()
    return () => disconnect()
  }, [me])
  if (!checked) return <div className="grid h-full place-items-center text-sm text-slate-500">Starting…</div>
  return (
    <Tooltip.Provider delayDuration={300}>
      <Toaster theme="dark" position="bottom-right" richColors closeButton toastOptions={{ className: 'text-sm' }} />
      {me?.role === 'field' ? (
        <Routes>
          <Route path="/rms" element={<Rms />} />
          <Route path="/rms/:number" element={<Rms />} />
          <Route path="/help" element={<Help />} />
          <Route path="*" element={<Mdt />} />
        </Routes>
      ) : me ? (
        <Routes>
          <Route path="/mdt" element={<Mdt />} />
          <Route path="/help" element={<Help />} />
          <Route path="/rms" element={<Rms />} />
          <Route path="/rms/:number" element={<Rms />} />
          <Route path="/" element={<Console key="full" />} />
          <Route path="/screen/calls" element={<Console key="calls" screen="calls" />} />
          <Route path="/screen/units" element={<Console key="units" screen="units" />} />
          <Route path="/screen/stations" element={<Console key="stations" screen="stations" />} />
          <Route path="/screen/police" element={<Console key="police" screen="police" />} />
          <Route path="/screen/map" element={<Console key="map" screen="map" />} />
          <Route path="/screen/incident" element={<Console key="incident" screen="incident" />} />
          <Route path="/board" element={<Suspense fallback={<div className="grid h-full place-items-center text-sm text-slate-500">Loading the board…</div>}><Board /></Suspense>} />
          <Route path="/admin/*" element={<Admin />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      ) : (
        <Login />
      )}
    </Tooltip.Provider>
  )
}
