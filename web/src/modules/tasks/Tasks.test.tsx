import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { changeTask, task } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, json, narrow, type Call } from '../../test-server.ts'
import type { Now, Task } from './api.ts'
import { monday, tasksServer } from './test-tasks.ts'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

const page = () => screen.getByRole('main')
// The titles in each group shown, by the group's key.
const shown = () => Object.fromEntries([...page().querySelectorAll('[data-group]')].map((g) => [g.getAttribute('data-group'), [...g.querySelectorAll('.t1')].map((t) => t.textContent)]))
const rowOf = (title: string) => [...page().querySelectorAll<HTMLElement>('.task-row')].find((r) => r.querySelector('.t1')?.textContent === title)!
const quick = (title: string) => within(rowOf(title).querySelector<HTMLElement>('.qa')!).getAllByRole('button')
const ringOf = (title: string) => within(rowOf(title)).getAllByRole('button')[0]
const viewLinks = () => [...page().querySelectorAll<HTMLAnchorElement>('.views a')]
const counts = () => viewLinks().map((a) => a.querySelector('.dim')?.textContent?.trim() ?? '')
const reads = (calls: Call[]) => calls.filter((c) => c.method === 'GET' && c.path.startsWith('/api/tasks?limit=')).length
const writes = (calls: Call[]) => calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path, c.body])
const sunday: Now = { ...monday, date: '2026-11-29', weekday: 'Sunday' }

// Around the week of Monday 23 November 2026.
const week = (): Task[] => [
  task(1, 60, { title: 'to sort', source: 'portal' }),
  task(2, 50, { title: 'last weekend', planned_from: '2026-11-21', planned_to: '2026-11-22' }),
  task(3, 40, { title: 'overdue', deadline: '2026-11-20' }),
  task(4, 30, { title: 'today', planned_from: '2026-11-23', planned_time: '14:00' }),
  task(5, 20, { title: 'the weekend', planned_from: '2026-11-28', planned_to: '2026-11-29', area: 'garden' }),
  task(6, 10, { title: 'next', list: 'next' }),
  task(7, 5, { title: 'someday', list: 'someday' }),
]

test('each view shows the open tasks its question gives by the server’s today, never by the browser’s clock', async () => {
  // The browser's clock says June; the server says Monday 23 November.
  vi.useFakeTimers({ toFake: ['Date'], now: new Date('2026-06-01T12:00:00Z') })
  narrow(true)
  const calls = tasksServer(week())
  const { where } = await open('/')
  expect(where()).toBe('/tasks')
  expect(calls.filter((c) => c.path.startsWith('/api/tasks?limit='))).toHaveLength(1)
  expect(calls.find((c) => c.path.startsWith('/api/tasks?limit='))!.path).toBe('/api/tasks?limit=500')
  expect(shown()).toEqual({ sort: ['to sort'], left: ['last weekend'], late: ['overdue'], today: ['today'] })
  // What was left keeps the dates it had, and a deadline passed is in the danger look.
  expect(rowOf('last weekend').querySelector('.t2')).toHaveTextContent('Sat-Sun 21-22 Nov')
  expect(rowOf('overdue').querySelector('.late')).not.toBeNull()
  expect(counts()).toEqual(['4', '1', '1', '1'])

  await userEvent.click(viewLinks()[1])
  await waitFor(() => expect(where()).toBe('/tasks?view=week'))
  expect(shown()).toEqual({ week: ['the weekend'] })
  await userEvent.click(viewLinks()[2])
  await waitFor(() => expect(shown()).toEqual({ next: ['next'] }))
  await userEvent.click(viewLinks()[3])
  await waitFor(() => expect(shown()).toEqual({ someday: ['someday'] }))
  expect(reads(calls)).toBe(1)
})

test('on that Sunday the weekend is on Today, with its span on the row', async () => {
  narrow(false)
  tasksServer(week(), sunday)
  await open('/tasks')
  expect(shown().today).toEqual(['the weekend'])
  expect(rowOf('the weekend').querySelector('.t2')).toHaveTextContent('Sat-Sun 28-29 Nov')
})

test('the area chosen narrows every view and count and is kept in the address with the view, so a reload keeps them', async () => {
  narrow(true)
  tasksServer(week())
  const view = await open('/tasks?view=week')
  const area = within(page()).getByRole('combobox')
  await userEvent.selectOptions(area, 'garden')
  await waitFor(() => expect(view.where()).toBe('/tasks?view=week&area=garden'))
  expect(counts()).toEqual(['', '1', '', ''])
  await userEvent.selectOptions(area, '')
  await waitFor(() => expect(counts()).toEqual(['4', '1', '1', '1']))
  view.unmount()

  const calls = tasksServer(week())
  await open('/tasks?view=week&area=garden')
  expect(within(page()).getByRole('combobox')).toHaveValue('garden')
  expect(viewLinks()[1]).toHaveAttribute('aria-current', 'page')
  expect(shown()).toEqual({ week: ['the weekend'] })
  expect(reads(calls)).toBe(1)
})

test.each([
  ['/tasks', { planned_from: '2026-11-23' }, 'today'],
  ['/tasks?view=next', { list: 'next' }, 'next'],
  ['/tasks?view=someday&area=garden', { list: 'someday', area: 'garden' }, 'someday'],
])('the add line on %s sends one task with %j, which then stands in its group without a reload and opens beside it', async (path, fields, group) => {
  narrow(false)
  const calls = tasksServer(week())
  const { where } = await open(path)
  const line = within(page()).getByRole('textbox')
  await userEvent.type(line, '  a new one {Enter}')
  expect(writes(calls)).toEqual([['POST', '/api/tasks', { tasks: [{ title: 'a new one', ...fields }] }]])
  await waitFor(() => expect(shown()[group]).toContain('a new one'))
  await waitFor(() => expect(where()).toBe(path.replace('/tasks', '/tasks/8')))
  expect(line).toHaveValue('')
  expect(line).toHaveFocus()
  expect(reads(calls)).toBe(1)
})

test('a quick button sends only what it changes, and the row then stands where it belongs without a reload', async () => {
  narrow(false)
  const calls = tasksServer(week())
  await open('/tasks')
  await userEvent.click(quick('to sort')[0])
  await waitFor(() => expect(shown()).toEqual({ left: ['last weekend'], late: ['overdue'], today: ['today', 'to sort'] }))
  await userEvent.click(quick('last weekend')[0])
  await waitFor(() => expect(shown()).toEqual({ late: ['overdue'], today: ['today', 'to sort', 'last weekend'] }))
  expect(writes(calls)).toEqual([
    ['PATCH', '/api/tasks/1', { planned_from: '2026-11-23' }],
    ['PATCH', '/api/tasks/2', { planned_from: '2026-11-23' }],
  ])
  expect(reads(calls)).toBe(1)
})

test.each([
  [1, { list: 'next' }, 'next'],
  [2, { list: 'someday' }, 'someday'],
])('the quick button %i of a task to sort sends %j and takes it to %s', async (n, fields, to) => {
  narrow(true)
  const calls = tasksServer(week())
  const view = await open('/tasks')
  await userEvent.click(quick('to sort')[n])
  await waitFor(() => expect(shown().sort).toBeUndefined())
  expect(writes(calls)).toEqual([['PATCH', '/api/tasks/1', fields]])
  await userEvent.click(viewLinks()[n + 1])
  await waitFor(() => expect(view.where()).toBe(`/tasks?view=${to}`))
  expect(shown()[to]).toContain('to sort')
})

test('among tasks made in one call, one marked done or opened again keeps its place before the list is read again', async () => {
  narrow(true)
  const made = '2026-11-23T08:00:00Z'
  const calls = tasksServer(['a', 'b', 'c'].map((title, i) => task(i + 1, 0, { title, planned_from: '2026-11-23', created_at: made })))
  await open('/tasks')
  expect(shown().today).toEqual(['a', 'b', 'c'])
  await userEvent.click(ringOf('b'))
  await waitFor(() => expect(ringOf('b')).toHaveAttribute('aria-pressed', 'true'))
  expect(shown().today).toEqual(['a', 'b', 'c'])
  await userEvent.click(ringOf('b'))
  await waitFor(() => expect(ringOf('b')).toHaveAttribute('aria-pressed', 'false'))
  expect(shown().today).toEqual(['a', 'b', 'c'])
  expect(reads(calls)).toBe(1)
})

test('a task marked done stays in place, struck through, until the view is left, and a second tap opens it again', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  narrow(false)
  const calls = tasksServer(week())
  await open('/tasks')
  const es = FakeEventSource.all[0]
  await act(async () => es.open())
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

  await user.click(ringOf('overdue'))
  await waitFor(() => expect(ringOf('overdue')).toHaveAttribute('aria-pressed', 'true'))
  expect(rowOf('overdue')).toHaveClass('done')
  // The server has it done at once, so the list read again lacks it; the row stays where it stood.
  act(() => es.send({ module: 'tasks', kind: 'task', id: 3, area: 1 }))
  await act(() => vi.advanceTimersByTimeAsync(2000))
  expect(reads(calls)).toBe(2)
  expect(shown()).toEqual({ sort: ['to sort'], left: ['last weekend'], late: ['overdue'], today: ['today'] })
  expect(counts()[0]).toBe('4')

  await user.click(ringOf('overdue'))
  await waitFor(() => expect(ringOf('overdue')).toHaveAttribute('aria-pressed', 'false'))
  await user.click(ringOf('overdue'))
  await waitFor(() => expect(ringOf('overdue')).toHaveAttribute('aria-pressed', 'true'))
  expect(writes(calls)).toEqual([
    ['PATCH', '/api/tasks/3', { done: true }],
    ['PATCH', '/api/tasks/3', { done: false }],
    ['PATCH', '/api/tasks/3', { done: true }],
  ])

  await user.click(viewLinks()[1])
  await user.click(viewLinks()[0])
  await waitFor(() => expect(shown().late).toBeUndefined())
  expect(counts()[0]).toBe('3')
})

test('two tasks marked done before either answer lands both stay struck through in place', async () => {
  narrow(true)
  const tasks = week()
  const held: (() => void)[] = []
  tasksServer(tasks, monday, (c) => {
    if (c.method !== 'PATCH') return
    const t = tasks.find((x) => c.path === `/api/tasks/${x.id}`)!
    return new Promise((answer) => held.push(() => answer(json(Object.assign(t, changeTask(t, c.body as Record<string, unknown>))))))
  })
  await open('/tasks')
  await userEvent.click(ringOf('overdue'))
  await userEvent.click(ringOf('today'))
  await act(async () => held.splice(0).forEach((answer) => answer()))
  await waitFor(() => expect([ringOf('overdue'), ringOf('today')].map((r) => r.getAttribute('aria-pressed'))).toEqual(['true', 'true']))
  expect(shown()).toEqual({ sort: ['to sort'], left: ['last weekend'], late: ['overdue'], today: ['today'] })
})

test('a change made elsewhere shows without a reload, and a refusal shows the server’s line', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  narrow(false)
  const tasks = week()
  const calls = tasksServer(tasks, monday, (c) => (c.method === 'PATCH' ? json({ error: 'the line the server gave' }, 400) : undefined))
  await open('/tasks?view=next')
  const es = FakeEventSource.all[0]
  await act(async () => es.open())
  tasks.push(task(8, 0, { title: 'made by a client', list: 'next' }))
  act(() => es.send({ module: 'tasks', kind: 'task', id: 8, area: 1 }))
  await act(() => vi.advanceTimersByTimeAsync(2000))
  expect(shown().next).toEqual(['next', 'made by a client'])

  await userEvent.setup({ advanceTimers: vi.advanceTimersByTime }).click(ringOf('next'))
  expect(await within(page()).findByRole('alert')).toHaveTextContent('the line the server gave')
  expect(ringOf('next')).toHaveAttribute('aria-pressed', 'false')
  expect(reads(calls)).toBe(2)
})

test('with no task at all a view says so, and with nothing for today Today offers Next', async () => {
  narrow(true)
  const none = tasksServer([])
  const view = await open('/tasks')
  expect(page().querySelector('.empty')).not.toBeNull()
  expect(page().querySelectorAll('[data-group]')).toHaveLength(0)
  expect(writes(none)).toEqual([])
  view.unmount()

  tasksServer([task(6, 10, { title: 'next', list: 'next' })])
  const { where } = await open('/tasks')
  expect(shown()).toEqual({ today: [] })
  await userEvent.click(within(page().querySelector<HTMLElement>('[data-group=today]')!).getByRole('link'))
  await waitFor(() => expect(where()).toBe('/tasks?view=next'))
})
