import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { changeTask, task } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, json, narrow, type Call } from '../../test-server.ts'
import type { Now, Task } from './api.ts'
import { editButton, form, monday, taskView, tasksServer } from './test-tasks.ts'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

const page = () => screen.getByRole('main')
// The titles in each group shown, by the group's key.
const shown = () => Object.fromEntries([...page().querySelectorAll('[data-group]')].map((g) => [g.getAttribute('data-group'), [...g.querySelectorAll('.t1')].map((t) => t.textContent)]))
const rowOf = (title: string) => [...page().querySelectorAll<HTMLElement>('.task-row')].find((r) => r.querySelector('.t1')?.textContent === title)!
// Edits the task open, picks what pick finds in it, and saves.
async function sortOpen(pick: (view: HTMLElement) => HTMLElement) {
  await userEvent.click(await waitFor(editButton))
  await userEvent.click(pick(taskView()))
  await userEvent.click(form().save)
}
const ringOf = (title: string) => within(rowOf(title)).getAllByRole('button')[0]
const viewLinks = () => [...page().querySelectorAll<HTMLAnchorElement>('.views > a')]
const counts = () => viewLinks().map((a) => a.querySelector('.count')?.textContent ?? '')
// The number each view's spoken name carries, or '' when it is called by its name alone.
const spoken = () => viewLinks().map((a) => a.getAttribute('aria-label')?.match(/\d+/)?.[0] ?? '')
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
  task(6, 10, { title: 'soon', list: 'soon' }),
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
  expect(shown()).toEqual({ scheduled: ['overdue', 'last weekend', 'today', 'the weekend'], unscheduled: ['to sort', 'soon', 'someday'] })
  expect(viewLinks()[0]).toHaveAttribute('aria-current', 'page')
  await userEvent.click(viewLinks()[1])
  await waitFor(() => expect(where()).toBe('/tasks?view=today'))
  expect(shown()).toEqual({ sort: ['to sort'], left: ['last weekend'], late: ['overdue'], today: ['today'] })
  // What was left keeps the dates it had, and a deadline passed is in the danger look.
  expect(rowOf('last weekend').querySelector('.t2')).toHaveTextContent('Sat-Sun 21-22 Nov')
  expect(rowOf('overdue').querySelector('.late')).not.toBeNull()
  expect(counts()).toEqual(['7', '4', '1', '1', '1', ''])
  expect(spoken()).toEqual(counts())

  await userEvent.click(viewLinks()[2])
  await waitFor(() => expect(where()).toBe('/tasks?view=week'))
  expect(shown()).toEqual({ week: ['the weekend'] })
  await userEvent.click(viewLinks()[3])
  await waitFor(() => expect(shown()).toEqual({ soon: ['soon'] }))
  await userEvent.click(viewLinks()[4])
  await waitFor(() => expect(shown()).toEqual({ someday: ['someday'] }))
  expect(reads(calls)).toBe(1)
  expect(calls.filter((c) => c.path.includes('done=true'))).toEqual([])
})

test('on that Sunday the weekend is on Today, with its span on the row', async () => {
  narrow(false)
  tasksServer(week(), sunday)
  await open('/tasks?view=today')
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
  expect(counts()).toEqual(['1', '', '1', '', '', ''])
  await userEvent.selectOptions(area, '')
  await waitFor(() => expect(counts()).toEqual(['7', '4', '1', '1', '1', '']))
  view.unmount()

  const calls = tasksServer(week())
  await open('/tasks?view=week&area=garden')
  expect(within(page()).getByRole('combobox')).toHaveValue('garden')
  expect(viewLinks()[2]).toHaveAttribute('aria-current', 'page')
  expect(shown()).toEqual({ week: ['the weekend'] })
  expect(reads(calls)).toBe(1)
})

test.each([
  ['/tasks', {}, 'unscheduled'],
  ['/tasks?view=today', { planned_from: '2026-11-23' }, 'today'],
  ['/tasks?view=soon', { list: 'soon' }, 'soon'],
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

test('a view picked from More moves into the row and the menu dismisses with Escape', async () => {
  const bounds = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    const width = this.matches('.views') ? 360 : this.matches('details') ? 46 : 74
    return new DOMRect(0, 0, width, 44)
  })
  try {
    narrow(true)
    tasksServer(week())
    const { where } = await open('/tasks')
    const bar = within(page().querySelector<HTMLElement>('.views')!)
    const more = bar.getByText('More')
    await userEvent.click(more)
    await userEvent.keyboard('{Escape}')
    expect(more).toHaveFocus()
    expect(more.closest('details')).not.toHaveAttribute('open')
    await userEvent.click(more)
    await userEvent.click(within(more.closest('details')!).getByRole('link', { name: /^Someday/ }))
    await waitFor(() => expect(where()).toBe('/tasks?view=someday'))
    expect(bar.getByRole('link', { name: /^Someday/, current: 'page' })).toBeVisible()
    expect(more.closest('details')).not.toHaveAttribute('open')
  } finally {
    bounds.mockRestore()
  }
})

test('a row carries no button but its ring', async () => {
  narrow(false)
  tasksServer(week())
  await open('/tasks')
  // The ring is the one button that is pressed or not.
  const buttons = [...page().querySelectorAll<HTMLElement>('.task-row')].map((r) => within(r).getAllByRole('button').map((b) => b.hasAttribute('aria-pressed')))
  expect(buttons).toEqual(week().map(() => [true]))
})

test('a task to sort or left from earlier is planned for today through its page, and then stands where it belongs without a reload', async () => {
  narrow(false)
  const calls = tasksServer(week())
  await open('/tasks/1?view=today')
  await sortOpen((v) => v.querySelector<HTMLElement>('.scs button')!)
  await waitFor(() => expect(shown()).toEqual({ left: ['last weekend'], late: ['overdue'], today: ['today', 'to sort'] }))
  await userEvent.click(within(rowOf('last weekend')).getByRole('link'))
  await waitFor(() => expect(page().querySelector('.task-read h1')).toHaveTextContent('last weekend'))
  await sortOpen((v) => v.querySelector<HTMLElement>('.scs button')!)
  await waitFor(() => expect(shown()).toEqual({ late: ['overdue'], today: ['today', 'to sort', 'last weekend'] }))
  expect(writes(calls)).toEqual([
    ['PATCH', '/api/tasks/1', { planned_from: '2026-11-23' }],
    ['PATCH', '/api/tasks/2', { planned_from: '2026-11-23' }],
  ])
  expect(reads(calls)).toBe(1)
})

test.each([
  ['soon', 1],
  ['someday', 2],
] as const)('a task to sort put on %s on its page goes there', async (to, n) => {
  narrow(false)
  const calls = tasksServer(week())
  await open('/tasks/1?view=today')
  await sortOpen((v) => within(v).getAllByRole('radio')[n])
  await waitFor(() => expect(shown().sort).toBeUndefined())
  expect(writes(calls)).toEqual([['PATCH', '/api/tasks/1', { list: to }]])
  await userEvent.click(viewLinks()[n + 2])
  await waitFor(() => expect(viewLinks()[n + 2]).toHaveAttribute('aria-current', 'page'))
  expect(shown()[to]).toContain('to sort')
})

test('Agenda counts overlapping dates and lists once, a completion moves the row to Done, and Done carries no count', async () => {
  narrow(true)
  const tasks = week()
  tasks[4].list = 'soon'
  tasks.push(task(8, 0, { title: 'finished', done_at: '2026-11-22T10:00:00Z' }))
  const calls = tasksServer(tasks)
  await open('/tasks')
  expect(counts()).toEqual(['7', '4', '1', '2', '1', ''])
  expect(shown().scheduled).toEqual(['overdue', 'last weekend', 'today', 'the weekend'])
  expect(shown().unscheduled).toEqual(['to sort', 'soon', 'someday'])
  await userEvent.click(ringOf('the weekend'))
  await waitFor(() => expect(rowOf('the weekend')).toBeUndefined())
  expect(counts()).toEqual(['6', '4', '', '1', '1', ''])
  expect(spoken()).toEqual(counts())
  expect(viewLinks()[2]).not.toHaveTextContent(/\d/)
  await userEvent.click(viewLinks()[5])
  await waitFor(() => expect(rowOf('the weekend')).toHaveClass('done'))
  await userEvent.click(ringOf('the weekend'))
  await waitFor(() => expect(rowOf('the weekend')).toBeUndefined())
  await userEvent.click(viewLinks()[0])
  await waitFor(() => expect(rowOf('the weekend')).toBeDefined())
  expect(counts()).toEqual(['7', '4', '1', '2', '1', ''])
  expect(writes(calls)).toEqual([['PATCH', '/api/tasks/5', { done: true }], ['PATCH', '/api/tasks/5', { done: false }]])
})

test('Agenda includes open tasks and area counts beyond the first page', async () => {
  narrow(true)
  const many = Array.from({ length: 501 }, (_, i) => task(i + 1, 0, { title: String(i), list: 'soon', area: i === 500 ? 'garden' : 'personal' }))
  const calls = tasksServer(many)
  await open('/tasks?area=garden')
  await waitFor(() => expect(shown().unscheduled).toEqual(['500']))
  expect(counts()).toEqual(['1', '', '', '1', '', ''])
  expect(reads(calls)).toBe(2)
})

test('two tasks marked done before either answer lands both leave the open list', async () => {
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
  await waitFor(() => expect(shown().scheduled).toEqual(['last weekend', 'the weekend']))
  expect(counts()[0]).toBe('5')
})

test('a change made elsewhere shows without a reload, and a refusal shows the server’s line', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  narrow(false)
  const tasks = week()
  const calls = tasksServer(tasks, monday, (c) => (c.method === 'PATCH' ? json({ error: 'the line the server gave' }, 400) : undefined))
  await open('/tasks?view=soon')
  const es = FakeEventSource.all[0]
  await act(async () => es.open())
  tasks.push(task(8, 0, { title: 'made by a client', list: 'soon' }))
  act(() => es.send({ module: 'tasks', kind: 'task', id: 8, area: 1 }))
  await act(() => vi.advanceTimersByTimeAsync(2000))
  expect(shown().soon).toEqual(['soon', 'made by a client'])

  await userEvent.setup({ advanceTimers: vi.advanceTimersByTime }).click(ringOf('soon'))
  expect(await within(page()).findByRole('alert')).toHaveTextContent('the line the server gave')
  expect(ringOf('soon')).toHaveAttribute('aria-pressed', 'false')
  expect(reads(calls)).toBe(2)
})

test('a list that cannot be read says why, though the task open beside it is read', async () => {
  narrow(false)
  tasksServer(week(), monday, (c) => (c.path.startsWith('/api/tasks?limit=') ? json({ error: 'down' }, 502) : undefined))
  await open('/tasks/4')
  await waitFor(editButton)
  expect(await within(page()).findByRole('alert')).toBeInTheDocument()
})

test('with no task at all a view says so, and with nothing for today Today offers Soon', async () => {
  narrow(true)
  const none = tasksServer([])
  const view = await open('/tasks')
  expect(page().querySelector('.empty')).not.toBeNull()
  expect(page().querySelectorAll('[data-group]')).toHaveLength(0)
  expect(writes(none)).toEqual([])
  view.unmount()

  tasksServer([task(6, 10, { title: 'soon', list: 'soon' })])
  const { where } = await open('/tasks?view=today')
  expect(shown()).toEqual({ today: [] })
  await userEvent.click(within(page().querySelector<HTMLElement>('[data-group=today]')!).getByRole('link'))
  await waitFor(() => expect(where()).toBe('/tasks?view=soon'))
})
