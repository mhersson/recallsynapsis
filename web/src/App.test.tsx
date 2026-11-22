import { useEffect } from 'react'
import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router'
import type { Session } from './api/types.ts'
import App from './App.tsx'
import { FakeEventSource, fakeServer, json, narrow, redirected } from './test-server.ts'

// Stand-in modules, each on while the session names it: plain adds nothing, and things has a home and a page that shows
// the signed-in user's name. The factory is hoisted above the imports, so it makes its elements without JSX.
vi.mock('./modules/registry.ts', async () => {
  const { createElement } = await import('react')
  const { useSession } = await import('./session.ts')
  const Things = () => createElement('h1', null, useSession().user.name)
  return {
    modules: [
      { name: 'plain', nav: [], tabs: [], more: [], routes: [] },
      {
        name: 'things',
        home: '/things',
        nav: [{ to: '/things', label: 'Things', icon: null }],
        tabs: [{ to: '/things', label: 'Things', icon: null }],
        more: [{ to: '/odds', label: 'Odds', note: 'Odd things', icon: null }],
        routes: [
          { path: '/things', element: createElement(Things) },
          { path: '/odds', element: createElement(Things) },
        ],
      },
    ],
  }
})

afterEach(() => vi.unstubAllGlobals())

let here = ''
function Here() {
  const { pathname, search } = useLocation()
  useEffect(() => {
    here = pathname + search
  })
  return null
}

async function show(path = '/') {
  const view = render(
    <MemoryRouter initialEntries={[path]}>
      <App />
      <Here />
    </MemoryRouter>,
  )
  await act(async () => {})
  return view
}

const session: Session = { user: { name: 'ada' }, modules: [], version: '1.2.3', timezone: 'UTC', last_backup: null }
const isSession = (c: { path: string }) => c.path === '/api/session'
const password = () => document.querySelector<HTMLInputElement>('input[type=password]')

async function signIn() {
  const user = userEvent.setup()
  await user.type(screen.getByRole('textbox'), 'ada')
  await user.type(password()!, 'pw')
  await user.click(screen.getByRole('button'))
}

// A server whose session is on while signedIn is true, and that fails while down; signing in turns the session on.
function server() {
  const state = { signedIn: false, down: false }
  const calls = fakeServer((c) => {
    if (c.path === '/api/auth/login') {
      state.signedIn = true
      return new Response(null, { status: 204 })
    }
    if (isSession(c)) return state.down ? json({ error: 'down' }, 502) : state.signedIn ? json(session) : json({ error: 'sign in' }, 401)
  })
  return { state, calls }
}

test('signing in lands on the page that was asked for', async () => {
  const { calls } = server()
  await show('/search?q=garden')
  expect(here).toBe('/login')
  expect(password()).not.toBeNull()
  expect(screen.queryByRole('main')).toBeNull()

  await signIn()
  expect(await screen.findByRole('main')).toBeInTheDocument()
  expect(here).toBe('/search?q=garden')
  expect(password()).toBeNull()
  expect(calls.filter(isSession)).toHaveLength(2)
})

test('a session that ends leads back to sign-in once the stream drops, and signing in returns to the page', async () => {
  const { state } = server()
  state.signedIn = true
  await show('/settings')
  const es = FakeEventSource.all[0]
  await act(async () => es.open())
  expect(screen.getByRole('main')).toBeInTheDocument()

  // The server refuses the ended session's stream, and the browser closes it.
  state.signedIn = false
  await act(async () => es.fail(true))
  expect(here).toBe('/login')
  expect(password()).not.toBeNull()

  await signIn()
  expect(await screen.findByRole('main')).toBeInTheDocument()
  expect(here).toBe('/settings')
})

test('a session that could not be read is read again when the stream opens', async () => {
  const { state } = server()
  state.signedIn = true
  state.down = true
  await show('/settings')
  expect(screen.queryByRole('main')).toBeNull()
  expect(here).toBe('/settings')

  state.down = false
  await act(async () => FakeEventSource.all[0].open())
  expect(screen.getByRole('main')).toBeInTheDocument()
})

test('a path that is no page goes to the Projects list', async () => {
  server().state.signedIn = true
  await show('/no/such/page')
  expect(screen.getByRole('heading', { level: 1 })).toBeInTheDocument()
  expect(here).toBe('/projects')
})

test('a redirect to another origin asks for a reload', async () => {
  fakeServer(() => redirected())
  await show('/')
  const notice = screen.getByRole('alert')
  expect(within(notice).getAllByRole('button')).toHaveLength(1)
  expect(password()).toBeNull()
})

test('a network failure shows no notice and opens no sign-in', async () => {
  vi.stubGlobal('EventSource', FakeEventSource)
  const failing = vi.fn(() => Promise.reject(new TypeError('Failed to fetch')))
  vi.stubGlobal('fetch', failing)
  await show('/search')
  expect(failing).toHaveBeenCalled()
  expect(screen.queryByRole('alert')).toBeNull()
  expect(here).toBe('/search')
})

const areas = [
  { id: 1, kind: 'personal', name: 'Personal', slug: 'personal', description: '', archived: false },
  { id: 2, kind: 'project', name: 'garden', slug: 'garden', description: '', archived: false },
]
const isAreas = (c: { path: string }) => c.path === '/api/areas'
const links = () => screen.getAllByRole('link').map((a) => a.getAttribute('href'))
const current = () => screen.getAllByRole('link').filter((a) => a.getAttribute('aria-current') === 'page').map((a) => a.getAttribute('href'))

// A signed-in server with the given modules on.
function signedIn(modules: string[]) {
  const s = { ...session, modules }
  const calls = fakeServer((c) => {
    if (isSession(c)) return json(s)
    if (isAreas(c)) return json({ areas })
  })
  return { s, calls }
}

test('with no module on, the side bar holds Search, the areas and Settings, and a module page is absent', async () => {
  narrow(false)
  signedIn([])
  await show('/things')
  expect(links()).toEqual(['/search', '/projects', '/areas/new', '/areas/1', '/areas/2', '/settings'])
  expect(here).toBe('/projects')
  expect(current()).toEqual(['/projects'])
})

test('with a module on, the shell shows its entries and its pages, which read the session', async () => {
  narrow(false)
  signedIn(['things'])
  await show('/things')
  expect(links()).toEqual(['/things', '/search', '/projects', '/areas/new', '/areas/1', '/areas/2', '/settings'])
  expect(current()).toEqual(['/things'])
  expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent(session.user.name)
})

test('a phone has the tabs of the modules that are on, then Projects and More', async () => {
  narrow(true)
  signedIn([])
  const none = await show()
  expect(links()).toEqual(['/projects', '/more'])
  none.unmount()

  signedIn(['things'])
  await show()
  expect(links()).toEqual(['/things', '/projects', '/more'])
})

test.each([
  ['/projects', '/projects'],
  ['/areas/2', '/projects'],
  ['/more', '/more'],
  ['/search', '/more'],
  ['/settings', '/more'],
  ['/odds', '/more'],
  ['/things', '/things'],
])('on a phone, %s keeps the tab %s current', async (path, tab) => {
  narrow(true)
  signedIn(['things'])
  await show(path)
  expect(current()).toEqual([tab])
})

test('a change to an area reads the areas again, and a change to anything else does not', async () => {
  vi.useFakeTimers()
  try {
    narrow(false)
    const { calls } = signedIn([])
    await show()
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    expect(calls.filter(isAreas)).toHaveLength(1)
    const sessions = calls.filter(isSession).length
    act(() => es.send({ module: 'notes', kind: 'area', id: 9, area: 1 }))
    act(() => es.send({ module: 'core', kind: 'note', id: 9, area: 1 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(calls.filter(isAreas)).toHaveLength(1)

    areas.push({ id: 3, kind: 'project', name: 'shed', slug: 'shed', description: '', archived: false })
    act(() => es.send({ module: 'core', kind: 'area', id: 3, area: 3 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(calls.filter(isAreas)).toHaveLength(2)
    expect(links()).toContain('/areas/3')
    expect(calls.filter(isSession)).toHaveLength(sessions)
  } finally {
    areas.pop()
    vi.useRealTimers()
  }
})

test('a module switched off while the page is open leaves the shell once the stream drops', async () => {
  narrow(false)
  const { s } = signedIn(['things'])
  await show('/things')
  expect(links()).toContain('/things')

  const es = FakeEventSource.all[0]
  await act(async () => es.open())
  s.modules = []
  await act(async () => es.fail())
  expect(links()).not.toContain('/things')
  expect(here).toBe('/projects')
})

test.each([
  [[], '/projects'],
  [['plain'], '/projects'],
  [['plain', 'things'], '/things'],
])('with the modules %j on, a person lands on %s', async (on, landing) => {
  signedIn(on)
  await show('/')
  expect(here).toBe(landing)
})

test('the Areas heading of the side bar stays current while a project is made', async () => {
  narrow(false)
  signedIn([])
  await show('/areas/new')
  expect(current()).toEqual(['/projects'])
})

test('the side bar says whether the stream is live', async () => {
  narrow(false)
  signedIn([])
  await show()
  const mark = screen.getByRole('status')
  expect(mark).toHaveAttribute('data-live', 'false')
  await act(async () => FakeEventSource.all[0].open())
  expect(mark).toHaveAttribute('data-live', 'true')
  await act(async () => FakeEventSource.all[0].fail())
  expect(mark).toHaveAttribute('data-live', 'false')
})
