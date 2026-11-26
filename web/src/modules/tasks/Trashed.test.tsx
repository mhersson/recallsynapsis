import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { task } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, narrow, type Call } from '../../test-server.ts'
import type { Task } from './api.ts'
import { editButton, taskView, tasksServer, trashButton } from './test-tasks.ts'

afterEach(() => vi.unstubAllGlobals())

const page = () => screen.getByRole('main')
const titles = () => [...page().querySelectorAll('.task-row .t1')].map((t) => t.textContent)
const rowOf = (title: string) => [...page().querySelectorAll<HTMLElement>('.task-row')].find((r) => r.querySelector('.t1')?.textContent === title)!
const writes = (calls: Call[]) => calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path])
const dialog = () => within(screen.getByRole('dialog'))
const ago = (hours: number) => new Date(Date.now() - hours * 3_600_000).toISOString()

// Two in the trash, the done one trashed last, and one outside it.
const tasks = (): Task[] => [
  task(1, 90, { title: 'planned', planned_from: '2026-11-24', trashed_at: ago(5) }),
  task(2, 80, { title: 'done', done_at: ago(30), trashed_at: ago(2) }),
  task(3, 70, { title: 'open', list: 'soon' }),
]

test('Trashed lists the whole trash, the most recently trashed first, and a trashed task is read and never changed', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  const { where } = await open('/tasks')
  await userEvent.click(within(page().querySelector<HTMLElement>('.page-head')!).getByRole('link'))
  await waitFor(() => expect(titles()).toEqual(['done', 'planned']))
  expect(calls.filter((c) => c.path.includes('trashed=true')).map((c) => c.path)).toEqual(['/api/tasks?trashed=true&limit=50'])
  // A row of the trash has its mark, and no ring to change it.
  expect(within(rowOf('done')).queryAllByRole('button')).toEqual([])
  expect(rowOf('done').querySelector('.mk')).toHaveClass('m-task', 't-done')

  // On desktop the newest opens beside the list, with Restore and Delete for good alone.
  await waitFor(() => expect(where()).toBe('/tasks/trashed/2'))
  await waitFor(() => expect(within(taskView()).getAllByRole('button')).toHaveLength(2))
  expect(taskView().querySelector('.chk, input, textarea, select')).toBeNull()
  expect(writes(calls)).toEqual([])
})

test('Restore puts a task back where it was, and the trash no longer lists it', async () => {
  narrow(true)
  const calls = tasksServer(tasks())
  const { where } = await open('/tasks/trashed')
  await userEvent.click(within(rowOf('planned')).getByRole('link'))
  await waitFor(() => expect(where()).toBe('/tasks/trashed/1'))
  const [restore] = await waitFor(() => within(taskView()).getAllByRole('button'))
  await userEvent.click(restore)
  await waitFor(() => expect(where()).toBe('/tasks/trashed'))
  await waitFor(() => expect(titles()).toEqual(['done']))
  expect(page().querySelector('.notice a')).toHaveAttribute('href', '/tasks/1')
  expect(writes(calls)).toEqual([['POST', '/api/tasks/1/restore']])

  await userEvent.click(page().querySelector<HTMLElement>('.notice a')!)
  await waitFor(() => expect(where()).toBe('/tasks/1'))
  await userEvent.click(within(page().querySelector<HTMLElement>('.crumb')!).getByRole('link'))
  await userEvent.click(await waitFor(() => within(page().querySelector<HTMLElement>('.views')!).getAllByRole('link')[2]))
  await waitFor(() => expect(page().querySelector('[data-group=week] .t1')).toHaveTextContent('planned'))
})

test('deleting for good asks first, with keeping it first and plain, and Keep it sends nothing', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  const { where } = await open('/tasks/trashed/1')
  const [, remove] = await waitFor(() => within(taskView()).getAllByRole('button'))
  await userEvent.click(remove)
  // The browser gives the first control the focus.
  const [keep, sure] = dialog().getAllByRole('button')
  expect([keep, sure].map((b) => b.classList.contains('danger'))).toEqual([false, true])
  await userEvent.click(keep)
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(writes(calls)).toEqual([])

  await userEvent.click(remove)
  await userEvent.click(dialog().getAllByRole('button')[1])
  await waitFor(() => expect(where()).toBe('/tasks/trashed'))
  await waitFor(() => expect(titles()).toEqual(['done']))
  expect(writes(calls)).toEqual([['DELETE', '/api/tasks/1']])
})

test('Empty the trash asks first and says how many tasks go, and then the trash is empty', async () => {
  narrow(true)
  const calls = tasksServer(tasks())
  await open('/tasks/trashed')
  await waitFor(() => expect(titles()).toHaveLength(2))
  // On a phone it stands at the foot.
  await userEvent.click(within(page().querySelector<HTMLElement>('.task-foot')!).getByRole('button'))
  expect(screen.getByRole('dialog').querySelector('.note')!.textContent).toMatch(/\b2\b/)
  const [keep, empty] = dialog().getAllByRole('button')
  expect([keep, empty].map((b) => b.classList.contains('danger'))).toEqual([false, true])
  await userEvent.click(empty)
  await waitFor(() => expect(page().querySelector('.empty')).not.toBeNull())
  expect(titles()).toEqual([])
  expect(writes(calls)).toEqual([['DELETE', '/api/tasks/trash']])
})

test('Trash on a task’s page moves it to the trash at once, asking nothing, and the view goes on without it', async () => {
  narrow(false)
  const calls = tasksServer([task(4, 60, { title: 'first', list: 'soon' }), task(5, 50, { title: 'second', list: 'soon' })])
  const { where } = await open('/tasks/4?view=soon')
  await userEvent.click(await waitFor(trashButton))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(writes(calls)).toEqual([['POST', '/api/tasks/4/trash']])
  // On desktop the view opens its first task again.
  await waitFor(() => expect(where()).toBe('/tasks/5?view=soon'))
  expect(titles()).toEqual(['second'])
  await waitFor(editButton)
})

test('on a phone Trash sits under Edit at the foot of a task, and the page goes back to the list', async () => {
  narrow(true)
  tasksServer([task(4, 60, { title: 'first', list: 'soon' }), task(5, 50, { title: 'second', list: 'soon' })])
  const { where } = await open('/tasks?view=soon')
  await userEvent.click(within(rowOf('first')).getByRole('link'))
  await waitFor(() => expect(where()).toBe('/tasks/4?view=soon'))
  const foot = await waitFor(() => taskView().querySelector<HTMLElement>('.task-foot')!)
  expect(within(foot).getAllByRole('button')).toEqual([editButton(), trashButton()])
  await userEvent.click(trashButton())
  await waitFor(() => expect(where()).toBe('/tasks?view=soon'))
  await waitFor(() => expect(titles()).toEqual(['second']))
})

test('a task open in the trash and deleted elsewhere closes to say so, and leaves the list', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    narrow(false)
    const kept = tasks()
    tasksServer(kept)
    await open('/tasks/trashed/1')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    await waitFor(() => expect(within(taskView()).getAllByRole('button')).toHaveLength(2))
    // Another browser empties the trash.
    kept.splice(0, 2)
    act(() => es.send({ module: 'tasks', kind: 'trash', id: 0, area: 0 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    await waitFor(() => expect(titles()).toEqual([]))
    expect(page().querySelector('.task-read')).toBeNull()
    expect(within(page().querySelector<HTMLElement>('.box')!).getByRole('alert')).toBeInTheDocument()
  } finally {
    vi.useRealTimers()
  }
})

test('a task trashed elsewhere while it is open in Tasks is read only there, and Restore puts it back in its view', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  await open('/tasks/1?view=week')
  const [restore] = await waitFor(() => within(taskView()).getAllByRole('button'))
  expect(within(taskView()).getAllByRole('button')).toHaveLength(1)
  await userEvent.click(restore)
  await waitFor(() => expect(titles()).toEqual(['planned']))
  await waitFor(editButton)
  expect(writes(calls)).toEqual([['POST', '/api/tasks/1/restore']])
})
