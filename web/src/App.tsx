import { useEffect, useRef, useState } from 'react'
import { Navigate, Route, Routes, useLocation, useMatch, useNavigate, useSearchParams } from 'react-router'
import { api, onRedirect, onUnauthorized } from './api/client.ts'
import { useEvents } from './api/stream.ts'
import type { Session } from './api/types.ts'
import AccessEnded from './components/AccessEnded.tsx'
import Dialog from './components/Dialog.tsx'
import PageHead from './components/PageHead.tsx'
import PhoneTabs from './components/PhoneTabs.tsx'
import SideBar from './components/SideBar.tsx'
import { back, useClose } from './components/useClose.ts'
import { useNarrow } from './components/useNarrow.ts'
import { modules, type Action } from './modules/registry.ts'
import Login from './pages/Login/Login.tsx'
import More from './pages/More/More.tsx'
import AreaPage from './pages/Projects/AreaPage.tsx'
import Projects from './pages/Projects/Projects.tsx'
import Search from './pages/Search/Search.tsx'
import Settings from './pages/Settings/Settings.tsx'
import { ActionContext, AreaShownContext, ModulesContext, SessionContext } from './session.ts'
import './App.css'

// The page of a module's action, as a phone opens it.
function ActionPage({ action, home }: { action: Action; home: string }) {
  const close = useClose(home)
  const [params] = useSearchParams()
  return (
    <div className="page">
      <PageHead title={action.label} />
      <action.form area={Number(params.get('area')) || undefined} onClose={close} />
    </div>
  )
}

function Frame() {
  const narrow = useNarrow()
  const navigate = useNavigate()
  const [session, setSession] = useState<Session | null>(null)
  const { live } = useEvents()
  const area = Number(useMatch('/areas/:id')?.params.id) || undefined
  // The open dialog of the action keeps the area it was opened with.
  const [acting, setActing] = useState<{ area?: number }>()
  const [shown, setShown] = useState<number>()
  // Read whenever the stream opens or drops: a restart may have switched a module, and an ended session answers 401, which opens sign-in.
  useEffect(() => {
    api<Session>('GET', '/session').then(setSession, () => {})
  }, [live])

  if (!session) return null
  // The modules of the registry that the server names as switched on, in the registry's order.
  const on = modules.filter((m) => session.modules.includes(m.name))
  const home = on.find((m) => m.home)?.home ?? '/projects'
  const action = on.find((m) => m.action)?.action
  const opener = action && {
    label: action.label,
    open: (at: number) => (narrow ? void navigate(`${action.to}?area=${at}`, { state: back }) : setActing({ area: at })),
  }

  return (
    <SessionContext value={session}>
      <ModulesContext value={on}>
        <ActionContext value={opener ?? null}>
          <AreaShownContext value={setShown}>
            <div className={narrow ? 'frame narrow' : 'frame'}>
              {!narrow && <SideBar modules={on} action={action} onAction={() => setActing({ area })} live={live} area={shown} />}
              <main className="main">
                <Routes>
                  <Route path="/projects" element={<Projects />} />
                  <Route path="/areas/new" element={<Projects making />} />
                  <Route path="/areas/:id" element={<AreaPage />} />
                  <Route path="/search" element={<Search />} />
                  <Route path="/settings" element={<Settings />} />
                  <Route path="/more" element={<More />} />
                  {on.flatMap((m) => m.routes).map((r) => (
                    <Route key={r.path} path={r.path} element={r.element} />
                  ))}
                  {action && <Route path={action.to} element={<ActionPage action={action} home={home} />} />}
                  <Route path="*" element={<Navigate to={home} replace />} />
                </Routes>
              </main>
              {narrow && <PhoneTabs modules={on} action={action} area={area} />}
              {acting && action && (
                <Dialog title={action.label} onClose={() => setActing(undefined)}>
                  <action.form dialog area={acting.area} onClose={() => setActing(undefined)} />
                </Dialog>
              )}
            </div>
          </AreaShownContext>
        </ActionContext>
      </ModulesContext>
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
