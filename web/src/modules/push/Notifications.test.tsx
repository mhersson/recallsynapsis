import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { open } from '../../test-app.tsx'
import { FakeEventSource, fakePush, fakeServer, json, narrow, type Call } from '../../test-server.ts'
import { ModulesContext } from '../../session.ts'
import { push } from './index.tsx'
import Notifications from './Notifications.tsx'
import { look, turnOn, type Browser } from './push.ts'

afterEach(() => vi.unstubAllGlobals())

const ID = 'push_browser'
const today = (h: number) => new Date(new Date().setHours(h, 0, 0, 0)).toISOString()

// A server with the modules given switched on, which keeps the caller's browsers by id; a new one is 7. failed is how the
// last message to a browser failed; answer, when given, answers first.
function server(browsers: number[], { modules = ['push'], failed, answer }: { modules?: string[]; failed?: Partial<Browser>; answer?: (c: Call) => Response | Promise<Response> | undefined } = {}) {
  return fakeServer((c) => {
    const a = answer?.(c)
    if (a) return a
    const url = new URL(c.path, 'http://x')
    if (c.path === '/api/session') return json({ user: { name: 'ada' }, modules, version: '1.2.3', timezone: 'UTC', last_backup: null })
    if (c.path.startsWith('/api/areas')) return json({ areas: [] })
    if (c.path === '/api/tokens') return json({ tokens: [] })
    if (c.path === '/api/auth/logout') return new Response(null, { status: 204 })
    if (url.pathname === '/api/push' && c.method === 'GET') {
      const id = Number(url.searchParams.get('browser'))
      return json({ key: '-_8', on: browsers.length, browser: browsers.includes(id) ? { id, failed_at: null, failure: '', ...failed } : null })
    }
    if (url.pathname === '/api/push' && c.method === 'PUT') {
      browsers.push(7)
      return json({ id: 7 })
    }
    if (url.pathname === '/api/push' && c.method === 'DELETE') {
      browsers.length = 0
      return new Response(null, { status: 204 })
    }
    const gone = browsers.indexOf(Number(/^\/api\/push\/(\d+)$/.exec(url.pathname)?.[1]))
    if (c.method === 'DELETE' && gone >= 0) {
      browsers.splice(gone, 1)
      return new Response(null, { status: 204 })
    }
  })
}

const part = () => within(document.getElementById('notifications')!)
const buttons = () => part().queryAllByRole('button')
const sent = (calls: Call[]) => calls.filter((c) => c.method !== 'GET' && c.path !== '/api/auth/logout').map((c) => [c.method, c.path])
// What the page asked of the server last about this browser.
const asked = (calls: Call[]) => calls.filter((c) => c.method === 'GET' && c.path.startsWith('/api/push')).at(-1)?.path
// In how many browsers the part says notifications are on.
const count = () => Number(/\d+/.exec(document.querySelector('#notifications .push-count')!.textContent!)?.[0] ?? 0)

beforeEach(() => narrow(false))

test('Turn on asks inside the click, subscribes with the key, registers the browser and keeps its id', async () => {
  const browser = fakePush()
  const calls = server([2])
  await open('/settings')
  await waitFor(() => expect(buttons()).toHaveLength(2))
  expect(count()).toBe(1)

  fireEvent.click(buttons()[0])
  // The permission is asked before anything is awaited, so it stays inside the click.
  expect(browser.requestPermission).toHaveBeenCalledTimes(1)
  await waitFor(() => expect(asked(calls)).toBe('/api/push?browser=7'))
  expect(browser.register).toHaveBeenCalledWith('/sw.js')
  expect(browser.subscribe).toHaveBeenCalledWith({ userVisibleOnly: true, applicationServerKey: new Uint8Array([251, 255]) })
  expect(calls.find((c) => c.method === 'PUT')!.body).toEqual({ endpoint: 'https://push.example/s1', keys: { p256dh: 'P', auth: 'A' } })
  expect(localStorage.getItem(ID)).toBe('7')
  expect(count()).toBe(2)

  // The browser's button now turns it off.
  await userEvent.click(buttons()[0])
  await waitFor(() => expect(sent(calls).at(-1)).toEqual(['DELETE', '/api/push/7']))
  expect(browser.unsubscribe).toHaveBeenCalledTimes(1)
  expect(localStorage.getItem(ID)).toBeNull()
})

test('a look while Turn on waits for the server runs after it, and finds the browser on', async () => {
  const browser = fakePush()
  const browsers: number[] = []
  let release = () => {}
  const held = new Promise<void>((r) => (release = r))
  const calls = server(browsers, {
    answer: (c) =>
      c.method === 'PUT'
        ? held.then(() => {
            browsers.push(7)
            return json({ id: 7 })
          })
        : undefined,
  })
  const on = turnOn('-_8')
  await waitFor(() => expect(calls.some((c) => c.method === 'PUT')).toBe(true))
  // As the stream tells of the browser before its PUT is answered.
  const seen = look()
  release()
  await on
  await expect(seen).resolves.toMatchObject({ state: 'on' })
  expect(browser.unsubscribe).not.toHaveBeenCalled()
  expect(localStorage.getItem(ID)).toBe('7')
})

test('the page never registers by itself, and with no browser on there is nothing to turn off everywhere', async () => {
  const browser = fakePush()
  const calls = server([])
  await open('/settings')
  await waitFor(() => expect(buttons()).toHaveLength(1))
  expect(count()).toBe(0)
  expect(browser.register).not.toHaveBeenCalled()
  expect(sent(calls)).toEqual([])
  expect(asked(calls)).toBe('/api/push')
})

test('a refused permission subscribes to nothing, sends nothing, and leaves no button', async () => {
  const browser = fakePush({ answer: 'denied' })
  const calls = server([])
  await open('/settings')
  await waitFor(() => expect(buttons()).toHaveLength(1))
  await userEvent.click(buttons()[0])
  await waitFor(() => expect(buttons()).toHaveLength(0))
  expect(browser.subscribe).not.toHaveBeenCalled()
  expect(sent(calls)).toEqual([])
  expect(localStorage.getItem(ID)).toBeNull()
})

test('a registration the server refuses shows its line and leaves the browser off', async () => {
  const browser = fakePush()
  const calls = server([], { answer: (c) => (c.method === 'PUT' ? json({ error: 'the endpoint is no https URL' }, 400) : undefined) })
  await open('/settings')
  await waitFor(() => expect(buttons()).toHaveLength(1))
  await userEvent.click(buttons()[0])
  expect(await part().findByRole('alert')).toBeInTheDocument()
  expect(browser.subscribed()).toBe(false)
  expect(localStorage.getItem(ID)).toBeNull()
  expect(asked(calls)).toBe('/api/push')
})

test('a browser whose kept id the server no longer knows unsubscribes and shows off', async () => {
  const browser = fakePush({ subscribed: true, permission: 'granted' })
  localStorage.setItem(ID, '5')
  const calls = server([2])
  await open('/settings')
  await waitFor(() => expect(browser.unsubscribe).toHaveBeenCalled())
  expect(localStorage.getItem(ID)).toBeNull()
  expect(sent(calls)).toEqual([])
  // Its button turns it on again.
  await userEvent.click(buttons()[0])
  expect(browser.requestPermission).toHaveBeenCalled()
})

test('a subscription with no kept id is unsubscribed, and never registered by the page', async () => {
  const browser = fakePush({ subscribed: true, permission: 'granted' })
  const calls = server([2])
  await open('/settings')
  await waitFor(() => expect(browser.unsubscribe).toHaveBeenCalled())
  expect(sent(calls)).toEqual([])
})

test('a kept id with no subscription is removed at the server, a 404 counting as done', async () => {
  for (const known of [true, false]) {
    fakePush({ permission: 'granted' })
    localStorage.setItem(ID, '5')
    const calls = server(known ? [5, 2] : [2])
    const view = await open('/settings')
    await waitFor(() => expect(asked(calls)).toBe('/api/push'))
    expect(sent(calls)).toEqual([['DELETE', '/api/push/5']])
    expect(localStorage.getItem(ID)).toBeNull()
    view.unmount()
  }
})

test('Turn off everywhere asks first with Keep them holding the focus, then turns every browser off', async () => {
  const browser = fakePush({ subscribed: true, permission: 'granted' })
  localStorage.setItem(ID, '5')
  const calls = server([5, 2, 3])
  await open('/settings')
  await waitFor(() => expect(asked(calls)).toBe('/api/push?browser=5'))

  await userEvent.click(buttons()[1])
  // The browser gives a modal dialog's first control the focus.
  const [keep] = within(screen.getByRole('dialog')).getAllByRole('button')
  await userEvent.click(keep)
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(sent(calls)).toEqual([])

  await userEvent.click(buttons()[1])
  await userEvent.click(within(screen.getByRole('dialog')).getAllByRole('button')[1])
  // This browser shows off, and with no browser on there is nothing more to turn off everywhere.
  await waitFor(() => expect(buttons()).toHaveLength(1))
  expect(sent(calls)).toEqual([['DELETE', '/api/push']])
  expect(browser.unsubscribe).toHaveBeenCalled()
  expect(localStorage.getItem(ID)).toBeNull()
})

test('an open Settings follows a change made in another browser without a reload', async () => {
  const browser = fakePush({ subscribed: true, permission: 'granted' })
  localStorage.setItem(ID, '5')
  const browsers = [5, 2]
  server(browsers)
  await open('/settings')
  await waitFor(() => expect(buttons()).toHaveLength(2))

  browsers.length = 0
  await act(async () => FakeEventSource.all[0].send({ module: 'push', kind: 'browser', id: 0, area: 0 }))
  await waitFor(() => expect(buttons()).toHaveLength(1), { timeout: 3000 })
  expect(browser.unsubscribe).toHaveBeenCalled()
  expect(localStorage.getItem(ID)).toBeNull()
})

test.each([
  ['an iPhone Safari tab', { supported: false, standalone: false }, 'home'],
  ['an iPhone Home Screen app with no Web Push', { supported: false, standalone: true }, 'none'],
  ['a refused permission', { permission: 'denied' as const }, 'refused'],
  ['no Web Push', { supported: false }, 'none'],
  ['a page not on https', { secure: false }, 'none'],
])('%s shows no button for this browser, and the others can still be turned off', async (_, options, state) => {
  fakePush(options)
  const calls = server([2, 3])
  await expect(look()).resolves.toMatchObject({ state })
  await open('/settings')
  await waitFor(() => expect(buttons()).toHaveLength(1))
  await userEvent.click(buttons()[0])
  expect(screen.getByRole('dialog')).toBeInTheDocument()
  expect(asked(calls)).toBe('/api/push')
})

test('a failure shows with the yellow mark, the time and the host', async () => {
  fakePush({ subscribed: true, permission: 'granted' })
  localStorage.setItem(ID, '5')
  server([5], { failed: { failed_at: today(14), failure: 'web.push.apple.com was not reached' } })
  await open('/settings')
  await waitFor(() => expect(document.querySelector('#notifications .mk.wait')).not.toBeNull())
  const line = document.querySelector('#notifications .mk.wait')!.parentElement
  expect(line).toHaveTextContent('14:00')
  expect(line).toHaveTextContent('web.push.apple.com')
})

test('signing out turns this browser off before the session ends, and ends it when that fails', async () => {
  for (const fails of [false, true]) {
    const browser = fakePush({ subscribed: true, permission: 'granted' })
    localStorage.setItem(ID, '5')
    const calls = server([5], { answer: (c) => (fails && c.path === '/api/push/5' ? json({ error: 'broken' }, 500) : undefined) })
    const view = await open('/settings')
    await waitFor(() => expect(asked(calls)).toBe('/api/push?browser=5'))
    await userEvent.click(within(document.getElementById('session')!).getByRole('button'))
    await waitFor(() => expect(view.where()).toBe('/login'))
    const order = calls.filter((c) => c.method !== 'GET').map((c) => c.path)
    expect(order).toEqual(['/api/push/5', '/api/auth/logout'])
    expect(browser.unsubscribe).toHaveBeenCalled()
    view.unmount()
  }
})

test('signing out removes this browser at the server when the browser fails to unsubscribe', async () => {
  const browser = fakePush({ subscribed: true, permission: 'granted' })
  browser.unsubscribe.mockRejectedValue(new DOMException('failed', 'AbortError'))
  localStorage.setItem(ID, '5')
  const calls = server([5])
  const view = await open('/settings')
  await waitFor(() => expect(asked(calls)).toBe('/api/push?browser=5'))
  await userEvent.click(within(document.getElementById('session')!).getByRole('button'))
  await waitFor(() => expect(view.where()).toBe('/login'))
  expect(sent(calls)).toEqual([['DELETE', '/api/push/5']])
})

test('a permission prompt left unanswered holds up no sign-out', async () => {
  const browser = fakePush()
  browser.requestPermission.mockReturnValue(new Promise(() => {}))
  server([])
  const view = await open('/settings')
  await waitFor(() => expect(buttons()).toHaveLength(1))
  await userEvent.click(buttons()[0])
  await userEvent.click(within(document.getElementById('session')!).getByRole('button'))
  await waitFor(() => expect(view.where()).toBe('/login'))
})

test('with push off Settings has no part Notifications and asks no push route', async () => {
  fakePush({ subscribed: true, permission: 'granted' })
  localStorage.setItem(ID, '5')
  const calls = server([5], { modules: [] })
  const view = await open('/settings')
  expect(document.getElementById('notifications')).toBeNull()
  await userEvent.click(within(document.getElementById('session')!).getByRole('button'))
  await waitFor(() => expect(view.where()).toBe('/login'))
  expect(calls.some((c) => c.path.startsWith('/api/push'))).toBe(false)
})

test('Notifications sits after Backups, on desktop and as a folding part on a phone', async () => {
  fakePush()
  server([])
  const view = await open('/settings')
  const anchors = within(screen.getByRole('navigation', { name: 'Sections' })).getAllByRole('link')
  expect(anchors.map((a) => a.getAttribute('href'))).toEqual(['#tokens', '#backups', '#notifications', '#browser', '#session'])
  view.unmount()

  narrow(true)
  await open('/settings')
  const parts = within(screen.getByRole('main')).getAllByRole('group')
  expect(parts).toHaveLength(5)
  await waitFor(() => expect(within(parts[2]).getAllByRole('button')).toHaveLength(1))
})

test('a browser that keeps nothing shows off and breaks nothing', async () => {
  const refuse = () => {
    throw new DOMException('denied', 'SecurityError')
  }
  vi.stubGlobal('localStorage', { getItem: refuse, setItem: refuse, removeItem: refuse })
  const browser = fakePush({ subscribed: true, permission: 'granted' })
  const calls = server([2])
  await open('/settings')
  await waitFor(() => expect(browser.unsubscribe).toHaveBeenCalled())
  expect(buttons()).toHaveLength(2)
  expect(sent(calls)).toEqual([])
})

test("each module's kinds show under the browser's lines, and when the browser cannot be read", async () => {
  for (const readable of [true, false]) {
    fakePush()
    server([], { answer: () => (readable ? undefined : json({ error: 'broken' }, 500)) })
    const view = render(
      <ModulesContext value={[{ ...push, notify: () => <input type="checkbox" /> }]}>
        <Notifications />
      </ModulesContext>,
    )
    const kinds = screen.getByRole('checkbox')
    // Once the browser is read, its button shows, or the line that says why it could not be.
    expect(await screen.findByRole(readable ? 'button' : 'alert')).toBeInTheDocument()
    expect(kinds).toBeInTheDocument()
    view.unmount()
  }
})
