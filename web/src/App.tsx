import { useEffect, useRef, useState } from 'react'
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router'
import { api, onRedirect, onUnauthorized } from './api/client.ts'
import { useEvents } from './api/stream.ts'
import type { Session } from './api/types.ts'
import AccessEnded from './components/AccessEnded.tsx'
import Login from './pages/Login/Login.tsx'
import Placeholder from './pages/Placeholder.tsx'
import { SessionContext } from './session.ts'
import './App.css'

function Frame() {
  const [session, setSession] = useState<Session | null>(null)
  const { live } = useEvents()
  // Read whenever the stream opens or drops: a restart may have switched a module, and an ended session answers 401, which opens sign-in.
  useEffect(() => {
    api<Session>('GET', '/session').then(setSession, () => {})
  }, [live])

  if (!session) return null

  return (
    <SessionContext value={session}>
      <div className="frame">
        <main className="main">
          <Routes>
            <Route path="/" element={<Placeholder title="Projects" />} />
            <Route path="/areas/new" element={<Placeholder title="New project" />} />
            <Route path="/areas/:id" element={<Placeholder title="Project" />} />
            <Route path="/search" element={<Placeholder title="Search" />} />
            <Route path="/settings" element={<Placeholder title="Settings" />} />
            <Route path="/more" element={<Placeholder title="More" />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </main>
      </div>
    </SessionContext>
  )
}

export default function App() {
  const navigate = useNavigate()
  const location = useLocation()
  const here = useRef(location)
  useEffect(() => {
    here.current = location
  })
  // A 401 anywhere opens sign-in, which returns to the page the person was on.
  useEffect(() => {
    onUnauthorized(() => {
      const { pathname, search } = here.current
      if (pathname !== '/login') navigate('/login', { replace: true, state: { from: pathname + search } })
    })
  }, [navigate])
  // The API redirects only a path that is not clean, which the app never asks for, so a redirect is taken as access
  // to the page having ended; only a reload clears it.
  const [ended, setEnded] = useState(false)
  useEffect(() => onRedirect(() => setEnded(true)), [])

  return (
    <>
      {ended && <AccessEnded />}
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="*" element={<Frame />} />
      </Routes>
    </>
  )
}
