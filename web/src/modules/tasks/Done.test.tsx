import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { changeTask, task } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { json, narrow, type Call } from '../../test-server.ts'
import type { Task } from './api.ts'
import { monday, tasksServer } from './test-tasks.ts'

afterEach(() => vi.unstubAllGlobals())

const page = () => screen.getByRole('main')
// Each day shown, in order, with the titles under it.
const days = () => [...page().querySelectorAll('[data-day]')].map((d) => [d.getAttribute('data-day'), [...d.querySelectorAll('.t1')].map((t) => t.textContent)])
const rowOf = (title: string) => [...page().querySelectorAll<HTMLElement>('.task-row')].find((r) => r.querySelector('.t1')?.textContent === title)!
const ringOf = (title: string) => within(rowOf(title)).getAllByRole('button')[0]
const asked = (calls: Call[]) => calls.filter((c) => c.method === 'GET' && c.path.includes('done=true')).map((c) => c.path)
const writes = (calls: Call[]) => calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path, c.body])

// Done on the Monday of the tests, the day before and the Saturday, in the server's time zone, and one still open.
const tasks = (): Task[] => [
  task(1, 90, { title: 'sat', done_at: '2026-11-21T15:40:00Z' }),
  task(2, 80, { title: 'sun', done_at: '2026-11-22T19:30:00Z', area: 'garden' }),
  task(3, 70, { title: 'mon early', done_at: '2026-11-23T07:00:00Z' }),
  task(4, 60, { title: 'mon late', done_at: '2026-11-23T09:10:00Z', planned_from: '2026-11-23' }),
  task(5, 50, { title: 'open', list: 'soon' }),
]

test('Done lists what was done, the newest first, under the day it was done, and narrows to the area chosen', async () => {
  narrow(true)
  const calls = tasksServer(tasks())
  await open('/tasks?view=done')
  await waitFor(() =>
    expect(days()).toEqual([
      ['2026-11-23', ['mon late', 'mon early']],
      ['2026-11-22', ['sun']],
      ['2026-11-21', ['sat']],
    ]),
  )
  expect(asked(calls)).toEqual(['/api/tasks?done=true&limit=50'])
  expect(rowOf('sat')).toHaveClass('done')
  // Done has no add line.
  expect(page().querySelector('.addl')).toBeNull()

  await userEvent.selectOptions(within(page()).getByRole('combobox'), 'garden')
  await waitFor(() => expect(days()).toEqual([['2026-11-22', ['sun']]]))
  expect(asked(calls).at(-1)).toBe('/api/tasks?done=true&area=garden&limit=50')
  expect(writes(calls)).toEqual([])
})

test('a list with more says so and shows 50 more, and with nothing done Done says so', async () => {
  narrow(true)
  const many = Array.from({ length: 60 }, (_, i) => task(i + 1, 60, { title: `done ${i + 1}`, done_at: `2026-11-${String(1 + (i % 22)).padStart(2, '0')}T10:00:00Z` }))
  const calls = tasksServer(many)
  const view = await open('/tasks?view=done')
  await waitFor(() => expect(page().querySelectorAll('.task-row')).toHaveLength(50))
  await userEvent.click(within(page().querySelector<HTMLElement>('.list-end')!).getByRole('button'))
  await waitFor(() => expect(page().querySelectorAll('.task-row')).toHaveLength(60))
  expect(asked(calls)).toEqual(['/api/tasks?done=true&limit=50', '/api/tasks?done=true&limit=100'])
  expect(page().querySelector('.list-end')).toBeNull()
  view.unmount()

  tasksServer([task(1, 10, { title: 'open', list: 'soon' })])
  await open('/tasks?view=done')
  await waitFor(() => expect(page().querySelector('.empty')).not.toBeNull())
  expect(page().querySelectorAll('.task-row')).toHaveLength(0)
})

test('Done carries no number, in the row or under More, though it holds done tasks, and is called by its name alone', async () => {
  const bounds = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    const width = this.matches('.views') ? 360 : this.matches('details') ? 46 : 74
    return new DOMRect(0, 0, width, 44)
  })
  try {
    narrow(true)
    tasksServer(tasks())
    const inRow = await open('/tasks?view=done')
    await waitFor(() => expect(days()).toHaveLength(3))
    const current = page().querySelector<HTMLElement>('.views > a[aria-current]')!
    expect(current).not.toHaveTextContent(/\d/)
    expect(current).not.toHaveAttribute('aria-label')
    inRow.unmount()

    const calls = tasksServer(tasks())
    await open('/tasks')
    await waitFor(() => expect(page().querySelector('.view-menu a[href="/tasks?view=done"]')).not.toBeNull())
    const listed = page().querySelector<HTMLElement>('.view-menu a[href="/tasks?view=done"]')!
    expect(listed).not.toHaveTextContent(/\d/)
    expect(listed).not.toHaveAttribute('aria-label')
    expect(asked(calls)).toEqual([])
  } finally {
    bounds.mockRestore()
  }
})

test('Done shown again after a task was done in another view opens that task first, never what it held before', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  const { where } = await open('/tasks?view=done')
  await waitFor(() => expect(where()).toBe('/tasks/4?view=done'))
  const views = () => within(page().querySelector<HTMLElement>('.views')!).getAllByRole('link')
  await userEvent.click(views()[0])
  await waitFor(() => expect(where()).toBe('/tasks/5'))
  await userEvent.click(ringOf('open'))
  await waitFor(() => expect(rowOf('open')).toBeUndefined())
  expect(asked(calls)).toEqual(['/api/tasks?done=true&limit=50'])
  await userEvent.click(views()[5])
  await waitFor(() => expect(where()).toBe('/tasks/5?view=done'))
  expect(rowOf('open')).toHaveClass('done')
})

test('a read of Done that fails says so on Done alone', async () => {
  narrow(true)
  tasksServer(tasks(), monday, (c) => (c.path.includes('done=true') ? json({ error: 'the line the server gave' }, 400) : undefined))
  await open('/tasks?view=done')
  expect(await within(page()).findByRole('alert')).toBeInTheDocument()
  await userEvent.click(within(page().querySelector<HTMLElement>('.views')!).getAllByRole('link')[0])
  await waitFor(() => expect(page().querySelector('[data-group]')).not.toBeNull())
  expect(within(page()).queryByRole('alert')).toBeNull()
})

test('a row ticked in a view of the open is not carried into Done, though its answer lands there, and Done lists it once done', async () => {
  narrow(true)
  const kept = tasks()
  const held: (() => void)[] = []
  tasksServer(kept, monday, (c) => {
    if (c.method !== 'PATCH') return
    const t = kept.find((x) => c.path === `/api/tasks/${x.id}`)!
    return new Promise((answer) => held.push(() => answer(json({ ...Object.assign(t, changeTask(t, c.body as Record<string, unknown>)), now: monday }))))
  })
  await open('/tasks?view=soon')
  await userEvent.click(ringOf('open'))
  await userEvent.click(within(page().querySelector<HTMLElement>('.views')!).getAllByRole('link')[5])
  await waitFor(() => expect(days()).toHaveLength(3))
  await act(async () => held.splice(0).forEach((answer) => answer()))
  await waitFor(() => expect(days().flatMap(([, titles]) => titles)).toContain('open'))
  expect(days().flatMap(([, titles]) => titles).filter((t) => t === 'open')).toHaveLength(1)
  expect(days().every(([day]) => String(day).startsWith('2026'))).toBe(true)
})
