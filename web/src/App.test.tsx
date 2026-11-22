import { useEffect } from 'react'
import { act, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, useLocation } from 'react-router'
import type { Session } from './api/types.ts'
import App from './App.tsx'
import { FakeEventSource, fakeServer, json, redirected } from './test-server.ts'

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
  expect(here).toBe('/')
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
