import { act, screen, waitFor, within } from '@testing-library/react'
import type { Session } from '../../api/types.ts'
import { task } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, fakeServer, json, narrow } from '../../test-server.ts'
import { areas, monday, tasksServer } from './test-tasks.ts'

afterEach(() => vi.unstubAllGlobals())

const nav = () => within(screen.getByRole('navigation')).getAllByRole('link')
const links = () => nav().map((a) => a.getAttribute('href'))
const counted = () => nav().map((a) => a.querySelector('.cnt')?.textContent ?? '')

// The real modules, with notes' inbox holding two captures and tasks holding one to sort.
function server(modules: string[], tasks = [task(1, 5, { title: 'to sort' }), task(2, 5, { title: 'soon', list: 'soon' })]) {
  if (!modules.includes('tasks')) {
    const session: Session = { user: { name: 'ada' }, modules, version: '1.2.3', timezone: 'UTC', last_backup: null }
    return fakeServer((c) => (c.path === '/api/session' ? json(session) : c.path.startsWith('/api/areas') ? json({ areas }) : c.path.startsWith('/api/notes?inbox=true') ? json({ notes: [], total: 2 }) : undefined))
  }
  return tasksServer(tasks, monday, (c) => (c.path.startsWith('/api/notes?inbox=true') ? json({ notes: [], total: 2 }) : undefined), modules)
}

test('with tasks and notes on, a person lands on Tasks, which heads the side bar with what waits to be sorted', async () => {
  narrow(false)
  const calls = server(['tasks', 'notes'])
  const { where } = await open('/')
  // Tasks opens its first task beside the list on desktop.
  await waitFor(() => expect(where()).toBe('/tasks/1'))
  expect(links().slice(0, 6)).toEqual(['/tasks', '/inbox', '/notes', '/ideas', '/trash', '/search'])
  expect(counted().slice(0, 2)).toEqual(['1', '2'])
  expect(calls.map((c) => c.path)).toContain('/api/tasks?unsorted=true&limit=1')
  // Capture stays notes'.
  expect(within(screen.getByRole('navigation')).getByRole('button')).toBeInTheDocument()
})

test('with tasks and notes on, the phone has Tasks, Inbox, Capture, Notes and More, and Projects is a row of More', async () => {
  narrow(true)
  server(['tasks', 'notes'])
  await open('/more')
  expect(links()).toEqual(['/tasks', '/inbox', '/capture', '/notes', '/more'])
  const rows = within(screen.getByRole('main')).getAllByRole('link').map((a) => a.getAttribute('href'))
  expect(rows).toEqual(['/ideas', '/trash', '/projects', '/search', '/settings'])
})

test('without tasks the shell is as it was', async () => {
  narrow(true)
  server(['notes'])
  const phone = await open('/')
  expect(phone.where()).toBe('/inbox')
  expect(links()).toEqual(['/inbox', '/notes', '/capture', '/projects', '/more'])
  phone.unmount()

  narrow(false)
  server(['notes'])
  await open('/')
  expect(links()).not.toContain('/tasks')
  expect(links().slice(0, 5)).toEqual(['/inbox', '/notes', '/ideas', '/trash', '/search'])
})

test('with tasks alone a person lands on Tasks, with no action, and the phone keeps Projects as a tab', async () => {
  narrow(true)
  server(['tasks'])
  const { where } = await open('/')
  expect(where()).toBe('/tasks')
  expect(links()).toEqual(['/tasks', '/projects', '/more'])
  expect(counted()[0]).toBe('1')
})

test('the count beside Tasks is read again on a change of a task, and is gone when nothing waits', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    narrow(true)
    const tasks = [task(1, 5, { title: 'to sort' })]
    server(['tasks'], tasks)
    await open('/more')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    expect(counted()[0]).toBe('1')
    Object.assign(tasks[0], { list: 'soon', state: 'open' })
    act(() => es.send({ module: 'tasks', kind: 'task', id: 1, area: 1 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(counted()[0]).toBe('')
  } finally {
    vi.useRealTimers()
  }
})
