import worker from '../../../public/sw.js?raw'

// The service worker, run against a recording of the browser's side.
function load() {
  const listeners: Record<string, (e: unknown) => void> = {}
  const self = {
    location: { origin: 'https://recall.example' },
    registration: { showNotification: vi.fn(() => Promise.resolve()) },
    clients: { matchAll: vi.fn(), openWindow: vi.fn(() => Promise.resolve()), claim: vi.fn(() => Promise.resolve()) },
    addEventListener: (type: string, f: (e: unknown) => void) => void (listeners[type] = f),
  }
  new Function('self', worker)(self)
  return { self, listeners }
}

const message = { title: 'Ringe snekkeren', body: '14:00 · Hytta', tag: 'tasks-12', url: '/tasks/12' }

test('every push shows its notification with its tag, the icon and its path, the same tag again as well', () => {
  const { self, listeners } = load()
  const waited: Promise<unknown>[] = []
  for (let i = 0; i < 2; i++) listeners.push({ data: { json: () => message }, waitUntil: (p: Promise<unknown>) => waited.push(p) })
  expect(self.registration.showNotification).toHaveBeenCalledTimes(2)
  expect(self.registration.showNotification).toHaveBeenCalledWith(message.title, {
    body: message.body,
    tag: message.tag,
    renotify: true,
    icon: '/icon-192.png',
    data: { url: message.url },
  })
  expect(waited).toHaveLength(2)
})

test('a push with no tag is shown without asking to renotify, which the browser refuses then', () => {
  const { self, listeners } = load()
  listeners.push({ data: { json: () => ({ ...message, tag: '' }) }, waitUntil: () => {} })
  expect(self.registration.showNotification).toHaveBeenCalledWith(message.title, expect.objectContaining({ tag: '', renotify: false }))
})

test('a click closes the notification and takes an open window to its path', async () => {
  const { self, listeners } = load()
  const win = { focus: vi.fn(() => Promise.resolve()), navigate: vi.fn(() => Promise.resolve()) }
  self.clients.matchAll.mockResolvedValue([win])
  const close = vi.fn()
  let waited: Promise<unknown> = Promise.resolve()
  listeners.notificationclick({ notification: { close, data: { url: '/tasks/12' } }, waitUntil: (p: Promise<unknown>) => (waited = p) })
  await waited
  expect(close).toHaveBeenCalled()
  expect(win.focus).toHaveBeenCalled()
  expect(win.navigate).toHaveBeenCalledWith('https://recall.example/tasks/12')
  expect(self.clients.openWindow).not.toHaveBeenCalled()
})

test('a click with no window open opens one at its path', async () => {
  const { self, listeners } = load()
  self.clients.matchAll.mockResolvedValue([])
  let waited: Promise<unknown> = Promise.resolve()
  listeners.notificationclick({ notification: { close: vi.fn(), data: { url: '/tasks' } }, waitUntil: (p: Promise<unknown>) => (waited = p) })
  await waited
  expect(self.clients.openWindow).toHaveBeenCalledWith('https://recall.example/tasks')
})

test('the worker claims the open pages when it activates', async () => {
  const { self, listeners } = load()
  let waited: Promise<unknown> = Promise.resolve()
  listeners.activate({ waitUntil: (p: Promise<unknown>) => (waited = p) })
  await waited
  expect(self.clients.claim).toHaveBeenCalled()
})
