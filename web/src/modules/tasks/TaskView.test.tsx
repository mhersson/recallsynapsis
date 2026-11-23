import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { changeTask, task } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, json, narrow, type Call } from '../../test-server.ts'
import type { Task } from './api.ts'
import { monday, tasksServer } from './test-tasks.ts'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

const view = () => screen.getByRole('main').querySelector<HTMLElement>('.task-read')!
const inView = () => within(view())
const head = () => within(view().querySelector<HTMLElement>('.task-head')!)
const ring = () => head().getAllByRole('button')[0]
const shortcuts = () => [...view().querySelectorAll<HTMLButtonElement>('.scs button')]
const dates = () => [...view().querySelectorAll<HTMLInputElement>('input[type=date]')]
const time = () => view().querySelector<HTMLInputElement>('input[type=time]')!
const lists = () => inView().getAllByRole('radio')
const entries = () => view().querySelectorAll('.lg')
const rows = () => [...screen.getByRole('main').querySelectorAll<HTMLElement>('.task-row')]
const rowOf = (title: string) => rows().find((r) => r.querySelector('.t1')?.textContent === title)!
const reads = (calls: Call[]) => calls.filter((c) => c.method === 'GET' && c.path.startsWith('/api/tasks?limit=')).length
const writes = (calls: Call[]) => calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path, c.body])
// Edit, in the head of a task on desktop and at its foot on a phone; while editing, Save and Cancel.
const editButton = () => [...view().querySelectorAll<HTMLElement>('.task-head .btn, .task-foot .btn')][0]
const editActs = () => within(view().querySelector<HTMLElement>('.task-edit .acts')!).getAllByRole('button')
// A date or time field changed and left.
const enter = (field: HTMLInputElement, value: string) => {
  fireEvent.change(field, { target: { value } })
  fireEvent.blur(field)
}

const tasks = (): Task[] => [
  task(1, 60, { title: 'to sort', said: 'Remember the light in the garage.' }),
  task(2, 30, { title: 'planned', planned_from: '2026-11-23', notes: 'first notes' }),
  task(3, 10, { title: 'in the garden', area: 'garden', planned_from: '2026-11-28', planned_to: '2026-11-29' }),
]

test('on desktop a view opens its first task beside the list, and a row picked opens its own, both named in the address', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  const { where } = await open('/tasks')
  await waitFor(() => expect(where()).toBe('/tasks/1'))
  expect(await head().findByRole('heading', { level: 1 })).toHaveTextContent('to sort')
  // What the owner said when a client made it is in the log.
  expect(entries()).toHaveLength(1)
  expect(entries()[0]).toHaveTextContent('Remember the light in the garage.')

  await userEvent.click(within(rowOf('planned')).getByRole('link'))
  await waitFor(() => expect(where()).toBe('/tasks/2'))
  await waitFor(() => expect(head().getByRole('heading', { level: 1 })).toHaveTextContent('planned'))
  expect(rowOf('planned').querySelector('a')).toHaveAttribute('aria-current', 'true')
  expect(writes(calls)).toEqual([])
})

test('on a phone a task picked opens as a page of its own, whose crumb leads back to the view', async () => {
  narrow(true)
  tasksServer(tasks())
  const { where } = await open('/tasks?view=week')
  expect(screen.getByRole('main').querySelector('.task-read')).toBeNull()
  await userEvent.click(within(rowOf('in the garden')).getByRole('link'))
  await waitFor(() => expect(where()).toBe('/tasks/3?view=week'))
  expect(rows()).toEqual([])
  await waitFor(() => expect(head().getByRole('heading', { level: 1 })).toHaveTextContent('in the garden'))
  await userEvent.click(within(screen.getByRole('main').querySelector<HTMLElement>('.crumb')!).getByRole('link'))
  await waitFor(() => expect(where()).toBe('/tasks?view=week'))
  expect(rows()).toHaveLength(1)
})

test('the ring of the open task marks it done, which strikes its row in place, and opens it again', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  await open('/tasks/2')
  await waitFor(() => expect(ring()).toHaveAttribute('aria-pressed', 'false'))
  await userEvent.click(ring())
  await waitFor(() => expect(ring()).toHaveAttribute('aria-pressed', 'true'))
  expect(rowOf('planned')).toHaveClass('done')
  expect(entries()).toHaveLength(2)
  await userEvent.click(ring())
  await waitFor(() => expect(ring()).toHaveAttribute('aria-pressed', 'false'))
  expect(rowOf('planned')).not.toHaveClass('done')
  expect(writes(calls)).toEqual([
    ['PATCH', '/api/tasks/2', { done: true }],
    ['PATCH', '/api/tasks/2', { done: false }],
  ])
})

test('a shortcut sends the dates it names and shows as chosen, and Clear the plan sends none', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  await open('/tasks/1')
  const chosen = () => shortcuts().map((b) => b.hasAttribute('aria-current'))
  await waitFor(() => expect(chosen()).toEqual([false, false, false, false]))
  await userEvent.click(shortcuts()[2])
  await waitFor(() => expect(chosen()).toEqual([false, false, true, false]))
  expect(dates().map((d) => d.value)).toEqual(['2026-11-28', '2026-11-29', ''])
  // The row leaves Today for the weekend, and comes back with Today, without a reload.
  expect(rowOf('to sort')).toBeUndefined()
  await userEvent.click(shortcuts()[0])
  await waitFor(() => expect(chosen()).toEqual([true, false, false, false]))
  expect(rowOf('to sort')).toBeDefined()
  expect(reads(calls)).toBe(1)
  await userEvent.click(shortcuts()[3])
  await waitFor(() => expect(chosen()).toEqual([false, false, false, true]))
  // Clear the plan is the one link-like button among the dates.
  await userEvent.click(within(view().querySelector<HTMLElement>('.dts')!).getByRole('button'))
  await waitFor(() => expect(dates()[0]).toHaveValue(''))
  expect(writes(calls)).toEqual([
    ['PATCH', '/api/tasks/1', { planned_from: '2026-11-28', planned_to: '2026-11-29' }],
    ['PATCH', '/api/tasks/1', { planned_from: '2026-11-23' }],
    ['PATCH', '/api/tasks/1', { planned_from: '2026-11-30', planned_to: '2026-12-06' }],
    ['PATCH', '/api/tasks/1', { planned_from: '' }],
  ])
})

test('the date and time fields, the deadline, the list and the area each send only what they change as they change', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  await open('/tasks/3')
  await waitFor(() => expect(dates()[0]).toHaveValue('2026-11-28'))
  // Each change answered is an entry of the log, after the making.
  const answered = (n: number) => waitFor(() => expect(entries()).toHaveLength(n + 1))
  // A span's from moved earlier keeps the to; a field typed into sends nothing until a whole date is in it.
  enter(dates()[0], '2026-11-27')
  await answered(1)
  enter(dates()[1], '0202-11-30')
  expect(dates()[1]).toHaveValue('2026-11-29')
  enter(dates()[1], '2026-11-30')
  await answered(2)
  enter(time(), '09:30')
  await answered(3)
  enter(dates()[2], '2026-12-01')
  await answered(4)
  await userEvent.click(view().querySelector<HTMLElement>('.tf .acts button')!)
  await waitFor(() => expect(dates()[2]).toHaveValue(''))
  await userEvent.click(lists()[2])
  await waitFor(() => expect(lists()[2]).toHaveAttribute('aria-checked', 'true'))
  await userEvent.selectOptions(inView().getByRole('combobox'), 'personal')
  await waitFor(() => expect(inView().getByRole('combobox')).toHaveValue('personal'))
  expect(writes(calls)).toEqual([
    ['PATCH', '/api/tasks/3', { planned_from: '2026-11-27', planned_to: '2026-11-29' }],
    ['PATCH', '/api/tasks/3', { planned_to: '2026-11-30' }],
    ['PATCH', '/api/tasks/3', { planned_time: '09:30' }],
    ['PATCH', '/api/tasks/3', { deadline: '2026-12-01' }],
    ['PATCH', '/api/tasks/3', { deadline: '' }],
    ['PATCH', '/api/tasks/3', { list: 'someday' }],
    ['PATCH', '/api/tasks/3', { area: 'personal' }],
  ])
  expect(entries()).toHaveLength(8)
})

test('a one-day plan moved by its from stays one day, and a refused date shows the server’s line and the task’s value again', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  await open('/tasks/2')
  await waitFor(() => expect(dates()[0]).toHaveValue('2026-11-23'))
  enter(dates()[0], '2026-11-21')
  await waitFor(() => expect(dates()[1]).toHaveValue('2026-11-21'))
  enter(dates()[1], '2026-11-20')
  expect(await inView().findByRole('alert')).toBeInTheDocument()
  await waitFor(() => expect(dates()[1]).toHaveValue('2026-11-21'))
  // The refused value, given again, is sent again.
  enter(dates()[1], '2026-11-20')
  await waitFor(() => expect(writes(calls)).toHaveLength(3))
  expect(writes(calls)).toEqual([
    ['PATCH', '/api/tasks/2', { planned_from: '2026-11-21' }],
    ['PATCH', '/api/tasks/2', { planned_to: '2026-11-20' }],
    ['PATCH', '/api/tasks/2', { planned_to: '2026-11-20' }],
  ])
})

test('turning a phone while a task is edited keeps what was typed', async () => {
  const turn = narrow(true)
  tasksServer(tasks())
  await open('/tasks/2')
  await userEvent.click(await waitFor(editButton))
  await userEvent.type(view().querySelector('textarea')!, ' typed')
  act(() => turn(false))
  expect(view().querySelector('textarea')).toHaveValue('first notes typed')
  expect(screen.getByRole('main').querySelector('.panes > .box .task-read')).not.toBeNull()
})

test('the answer to a change of a task no longer open leaves the one open now as it is', async () => {
  narrow(false)
  const kept = tasks()
  const held: (() => void)[] = []
  tasksServer(kept, monday, (c) => {
    if (c.method !== 'PATCH') return
    const t = kept.find((x) => c.path === `/api/tasks/${x.id}`)!
    return new Promise((answer) => held.push(() => answer(json(Object.assign(t, changeTask(t, c.body as Record<string, unknown>))))))
  })
  const { where } = await open('/tasks/2')
  await waitFor(() => expect(ring()).toHaveAttribute('aria-pressed', 'false'))
  await userEvent.click(ring())
  await userEvent.click(within(rowOf('to sort')).getByRole('link'))
  await waitFor(() => expect(where()).toBe('/tasks/1'))
  await waitFor(() => expect(head().getByRole('heading', { level: 1 })).toHaveTextContent('to sort'))
  await act(async () => held.splice(0).forEach((answer) => answer()))
  expect(head().getByRole('heading', { level: 1 })).toHaveTextContent('to sort')
})

test('a date or time field saves nothing as it changes, saves on Enter without leaving it, and sends a value once', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  await open('/tasks/2')
  await waitFor(() => expect(dates()).toHaveLength(3))
  fireEvent.change(dates()[2], { target: { value: '2026-12-02' } })
  fireEvent.change(time(), { target: { value: '09:00' } })
  expect(writes(calls)).toEqual([])
  fireEvent.keyDown(dates()[2], { key: 'Enter' })
  fireEvent.blur(dates()[2])
  fireEvent.blur(time())
  expect(writes(calls)).toEqual([
    ['PATCH', '/api/tasks/2', { deadline: '2026-12-02' }],
    ['PATCH', '/api/tasks/2', { planned_time: '09:00' }],
  ])
})

test('among tasks made in one call, one given a list from its page keeps its place before the list is read again', async () => {
  narrow(false)
  const made = '2026-11-23T08:00:00Z'
  const calls = tasksServer(['a', 'b', 'c'].map((title, i) => task(i + 1, 0, { title, planned_from: '2026-11-23', created_at: made })))
  await open('/tasks/2')
  await waitFor(() => expect(lists()[0]).toHaveAttribute('aria-checked', 'true'))
  await userEvent.click(lists()[1])
  await waitFor(() => expect(lists()[1]).toHaveAttribute('aria-checked', 'true'))
  expect(rows().map((r) => r.querySelector('.t1')!.textContent)).toEqual(['a', 'b', 'c'])
  expect(reads(calls)).toBe(1)
})

test('Save sends the title and the notes alone, as they differ from where the editing began, and Cancel sends nothing', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  await open('/tasks/2')
  await userEvent.click(await waitFor(editButton))
  const title = head().getByRole('textbox')
  await userEvent.clear(title)
  await userEvent.type(title, 'renamed')
  await userEvent.type(view().querySelector('textarea')!, ' and more')
  await userEvent.click(editActs()[0])
  await waitFor(() => expect(head().getByRole('heading', { level: 1 })).toHaveTextContent('renamed'))
  expect(view().querySelector('.md')).toHaveTextContent('first notes and more')

  await userEvent.click(editButton())
  await userEvent.type(view().querySelector('textarea')!, '!')
  await userEvent.click(editActs()[0])
  await waitFor(() => expect(view().querySelector('textarea')).toBeNull())
  await userEvent.click(editButton())
  await userEvent.type(view().querySelector('textarea')!, ' dropped')
  await userEvent.click(editActs()[1])
  expect(view().querySelector('.md')).toHaveTextContent('first notes and more!')
  expect(writes(calls)).toEqual([
    ['PATCH', '/api/tasks/2', { title: 'renamed', notes: 'first notes and more' }],
    ['PATCH', '/api/tasks/2', { notes: 'first notes and more!' }],
  ])
})

test('leaving a task with typed text asks first, and a change of the area chosen asks nothing', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  const { where } = await open('/tasks/2')
  await userEvent.click(await waitFor(editButton))
  await userEvent.type(view().querySelector('textarea')!, ' typed')
  await userEvent.selectOptions(within(screen.getByRole('main').querySelector<HTMLElement>('.page-head')!).getByRole('combobox'), 'personal')
  await waitFor(() => expect(where()).toBe('/tasks/2?area=personal'))
  expect(screen.queryByRole('dialog')).toBeNull()

  await userEvent.click(within(rowOf('to sort')).getByRole('link'))
  const asked = await screen.findByRole('dialog')
  await userEvent.click(within(asked).getAllByRole('button')[0])
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(where()).toBe('/tasks/2?area=personal')
  expect(view().querySelector('textarea')).toHaveValue('first notes typed')

  await userEvent.click(within(rowOf('to sort')).getByRole('link'))
  await userEvent.click(within(await screen.findByRole('dialog')).getAllByRole('button')[1])
  await waitFor(() => expect(where()).toBe('/tasks/1?area=personal'))
  expect(writes(calls)).toEqual([])
})

test('a change made elsewhere shows without a reload and never replaces a text being edited, and a refusal shows the server’s line', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  narrow(false)
  const kept = tasks()
  tasksServer(kept, monday, (c) => (c.method === 'PATCH' && c.path === '/api/tasks/2' && (c.body as { list?: string }).list ? json({ error: 'the line the server gave' }, 400) : undefined))
  await open('/tasks/2')
  const es = FakeEventSource.all[0]
  await act(async () => es.open())
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  const elsewhere = async (t: Partial<Task>) => {
    Object.assign(kept[1], t)
    act(() => es.send({ module: 'tasks', kind: 'task', id: 2, area: 1 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
  }
  await elsewhere({ deadline: '2026-12-24' })
  expect(dates()[2]).toHaveValue('2026-12-24')

  await user.click(editButton())
  await user.type(view().querySelector('textarea')!, ' mine')
  await elsewhere({ notes: 'from a client', title: 'retitled' })
  expect(view().querySelector('textarea')).toHaveValue('first notes mine')
  expect(rowOf('retitled')).toBeDefined()

  await user.click(lists()[1])
  expect(await inView().findByRole('alert')).toHaveTextContent('the line the server gave')
  expect(lists()[0]).toHaveAttribute('aria-checked', 'true')
})
