import { lazy, Suspense, useEffect } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { Toaster } from 'sonner'
import * as Tooltip from '@radix-ui/react-tooltip'
import { checkMe, connect, disconnect, store, useStore } from './store'
import { LockGuard } from './Lock'
import Login from './Login'
import Console from './Console'
import Admin from './admin/Admin'
import Mdt from './mdt/Mdt'
import { Help } from './Help'
import Rms from './rms/Rms'

const Board = lazy(() => import('./console/Board'))
const Review = lazy(() => import('./records/Review'))
const Records = lazy(() => import('./records/Records'))
const Taker = lazy(() => import('./taker/Taker'))
const Station = lazy(() => import('./Station'))

const wait = (what: string) => <div className="grid h-full place-items-center text-sm text-slate-500">Loading {what}…</div>

/** Home is the workspace of the duty signed in: call entry for a call-taker, the console with its own
 * discipline for a police, fire or EMS dispatcher, administration or records for those duties. */
function Home() {
  const duty = useStore((s) => s.me?.duty)
  if (duty === 'calltaker') return <Suspense fallback={wait('call entry')}><Taker /></Suspense>
  if (duty === 'admin') return <Navigate to="/admin" replace />
  if (duty === 'records') return <Navigate to="/records" replace />
  return <Console key="full" />
}

export default function App() {
  const me = useStore((s) => s.me)
  const checked = useStore((s) => s.meChecked)
  useEffect(() => {
    checkMe()
  }, [])
  const signedIn = !!me
  useEffect(() => {
    if (signedIn) connect()
    return () => disconnect()
  }, [signedIn])
  // A police, fire or EMS dispatcher's screens start with their own discipline.
  const duty = me?.duty
  useEffect(() => {
    if ((duty === 'police' || duty === 'fire' || duty === 'ems') && !sessionStorage.getItem('bd.lens')) store.setLens([duty])
  }, [duty])
  if (!checked) return <div className="grid h-full place-items-center text-sm text-slate-500">Starting…</div>
  return (
    <Tooltip.Provider delayDuration={300}>
      <Toaster theme="dark" position="bottom-right" richColors closeButton toastOptions={{ className: 'text-sm' }} />
      <LockGuard />
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
          <Route path="/" element={<Home />} />
          <Route path="/console" element={<Console key="full" />} />
          <Route path="/taker" element={<Suspense fallback={wait('call entry')}><Taker /></Suspense>} />
          <Route path="/screen/calls" element={<Console key="calls" screen="calls" />} />
          <Route path="/screen/units" element={<Console key="units" screen="units" />} />
          <Route path="/screen/stations" element={<Console key="stations" screen="stations" />} />
          <Route path="/screen/police" element={<Console key="police" screen="police" />} />
          <Route path="/screen/map" element={<Console key="map" screen="map" />} />
          <Route path="/screen/incident" element={<Console key="incident" screen="incident" />} />
          <Route path="/board" element={<Suspense fallback={<div className="grid h-full place-items-center text-sm text-slate-500">Loading the board…</div>}><Board /></Suspense>} />
          <Route path="/review/:number" element={<Suspense fallback={<div className="grid h-full place-items-center text-sm text-slate-500">Loading the review…</div>}><Review /></Suspense>} />
          <Route path="/records" element={<Suspense fallback={<div className="grid h-full place-items-center text-sm text-slate-500">Loading records…</div>}><Records /></Suspense>} />
          <Route path="/records/:id" element={<Suspense fallback={<div className="grid h-full place-items-center text-sm text-slate-500">Loading records…</div>}><Records /></Suspense>} />
          <Route path="/admin/*" element={<Admin />} />
          <Route path="/station" element={<Suspense fallback={wait('the station screen')}><Station /></Suspense>} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      ) : (
        <Login />
      )}
    </Tooltip.Provider>
  )
}
