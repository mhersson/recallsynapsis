import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { changeTask, task } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, json, narrow, type Call } from '../../test-server.ts'
import type { Task } from './api.ts'
import { editButton, form, monday, taskView as view, tasksServer, trashButton } from './test-tasks.ts'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

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
const put = (field: HTMLInputElement, value: string) => fireEvent.change(field, { target: { value } })

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
  // Edit stands at the foot, and the head has the ring alone.
  expect(head().getAllByRole('button')).toEqual([ring()])
  expect(view().querySelector('.task-foot .btn')).not.toBeNull()
  await userEvent.click(within(screen.getByRole('main').querySelector<HTMLElement>('.crumb')!).getByRole('link'))
  await waitFor(() => expect(where()).toBe('/tasks?view=week'))
  expect(rows()).toHaveLength(1)
})

test('on a phone a task’s page reads the task alone, which gives the day, and the list is read once the page goes back to it', async () => {
  narrow(true)
  const calls = tasksServer(tasks())
  await open('/tasks/3')
  await waitFor(() => expect(head().getByRole('heading', { level: 1 })).toHaveTextContent('in the garden'))
  expect(reads(calls)).toBe(0)
  await userEvent.click(within(screen.getByRole('main').querySelector<HTMLElement>('.crumb')!).getByRole('link'))
  await waitFor(() => expect(rows()).toHaveLength(2))
  expect(reads(calls)).toBe(1)
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

test('a task read has no control but its ring, Edit, Trash and Add a link', async () => {
  narrow(false)
  tasksServer(tasks())
  await open('/tasks/3')
  await waitFor(editButton)
  expect(inView().getAllByRole('button')).toEqual([ring(), editButton(), trashButton(), view().querySelector('.sect-head .btn')])
  expect(view().querySelectorAll('input, select, textarea')).toHaveLength(0)
})

test('a shortcut, the dates, the time, the deadline, the list and the area change the form alone, and Save sends one change of what differs', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  await open('/tasks/3')
  await userEvent.click(await waitFor(editButton))
  const chosen = () => shortcuts().map((b) => b.hasAttribute('aria-current'))
  expect(chosen()).toEqual([false, false, true, false])
  await userEvent.type(head().getByRole('textbox'), ' too')
  await userEvent.type(view().querySelector('textarea')!, 'by hand')
  await userEvent.click(shortcuts()[3])
  expect(chosen()).toEqual([false, false, false, true])
  expect(dates().map((d) => d.value)).toEqual(['2026-11-30', '2026-12-06', ''])
  put(time(), '09:30')
  put(dates()[2], '2026-12-01')
  // The deadline's Clear is the one button beside it.
  await userEvent.click(view().querySelector<HTMLElement>('.tf .acts button')!)
  expect(dates()[2]).toHaveValue('')
  put(dates()[2], '2026-12-02')
  await userEvent.click(lists()[2])
  await userEvent.selectOptions(inView().getByRole('combobox'), 'personal')
  // Neither Enter nor leaving a field sends it.
  fireEvent.keyDown(dates()[2], { key: 'Enter' })
  fireEvent.blur(dates()[2])
  expect(writes(calls)).toEqual([])

  await userEvent.click(form().save)
  await waitFor(() => expect(view().querySelector('.task-edit')).toBeNull())
  expect(writes(calls)).toEqual([
    ['PATCH', '/api/tasks/3', { title: 'in the garden too', notes: 'by hand', planned_from: '2026-11-30', planned_to: '2026-12-06', planned_time: '09:30', deadline: '2026-12-02', list: 'someday', area: 'personal' }],
  ])
  expect(entries()).toHaveLength(2)
})

test('Clear the plan clears it whole in the form, and Save sends a from of none', async () => {
  narrow(false)
  const calls = tasksServer([task(2, 30, { title: 'planned', planned_from: '2026-11-23', planned_time: '14:00' })])
  await open('/tasks/2')
  await userEvent.click(await waitFor(editButton))
  await userEvent.click(within(view().querySelector<HTMLElement>('.dts')!).getByRole('button'))
  expect([...dates(), time()].map((f) => [f.value, f.disabled])).toEqual([
    ['', false],
    ['', true],
    ['', false],
    ['', true],
  ])
  expect(writes(calls)).toEqual([])
  await userEvent.click(form().save)
  await waitFor(() => expect(view().querySelector('.task-edit')).toBeNull())
  expect(writes(calls)).toEqual([['PATCH', '/api/tasks/2', { planned_from: '' }]])
})

test('a one-day plan moved by its from stays one day, a span keeps its to however its from is typed, and a to alone is sent alone', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  await open('/tasks/3')
  // Edits the open task: each from in turn, as a date typed a part at a time passes through, then the to and the time.
  const edit = async (froms: string[], to: string, time: string, shown: string) => {
    await userEvent.click(await waitFor(editButton))
    froms.forEach((f) => put(dates()[0], f))
    if (to) put(dates()[1], to)
    if (time) put(view().querySelector<HTMLInputElement>('input[type=time]')!, time)
    expect(dates()[1]).toHaveValue(shown)
    await userEvent.click(form().save)
    await waitFor(() => expect(view().querySelector('.task-edit')).toBeNull())
  }
  const pick = async (title: string) => {
    await userEvent.click(within(rowOf(title)).getByRole('link'))
    await waitFor(() => expect(head().getByRole('heading', { level: 1 })).toHaveTextContent(title))
  }
  await edit(['2026-12-27', '2026-11-27'], '', '', '2026-11-29')
  await edit([], '2026-11-30', '', '2026-11-30')
  await pick('planned')
  await edit(['2026-12-25', '2026-11-25'], '', '', '2026-11-25')
  await pick('to sort')
  await edit(['2026-11-24'], '', '08:00', '2026-11-24')
  expect(writes(calls)).toEqual([
    ['PATCH', '/api/tasks/3', { planned_from: '2026-11-27', planned_to: '2026-11-29' }],
    ['PATCH', '/api/tasks/3', { planned_to: '2026-11-30' }],
    ['PATCH', '/api/tasks/2', { planned_from: '2026-11-25' }],
    ['PATCH', '/api/tasks/1', { planned_from: '2026-11-24', planned_time: '08:00' }],
  ])
})

test('a save the server refuses keeps the form as entered and shows the server’s line, and a second Save sends it again', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  await open('/tasks/2')
  await userEvent.click(await waitFor(editButton))
  await userEvent.type(head().getByRole('textbox'), ' again')
  put(dates()[1], '2026-11-20')
  await userEvent.click(lists()[1])
  await userEvent.click(form().save)
  expect(await within(view().querySelector<HTMLElement>('.task-edit')!).findByRole('alert')).toBeInTheDocument()
  expect(head().getByRole('textbox')).toHaveValue('planned again')
  expect(dates().map((d) => d.value)).toEqual(['2026-11-23', '2026-11-20', ''])
  expect(lists()[1]).toHaveAttribute('aria-checked', 'true')
  await userEvent.click(form().save)
  await waitFor(() => expect(writes(calls)).toHaveLength(2))
  const change = { title: 'planned again', planned_to: '2026-11-20', list: 'soon' }
  expect(writes(calls)).toEqual([
    ['PATCH', '/api/tasks/2', change],
    ['PATCH', '/api/tasks/2', change],
  ])
  // Put back as it was, the form sends nothing, closes and leaves no refusal behind.
  await userEvent.clear(head().getByRole('textbox'))
  await userEvent.type(head().getByRole('textbox'), 'planned')
  put(dates()[1], '2026-11-23')
  await userEvent.click(lists()[0])
  await userEvent.click(form().save)
  expect(view().querySelector('.task-edit')).toBeNull()
  expect(inView().queryByRole('alert')).toBeNull()
  expect(writes(calls)).toHaveLength(2)
})

test('Save with nothing changed sends nothing and closes the form, and Cancel gives up a change without asking', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  await open('/tasks/2')
  await userEvent.click(await waitFor(editButton))
  await userEvent.click(form().save)
  expect(view().querySelector('.task-edit')).toBeNull()

  await userEvent.click(editButton())
  await userEvent.type(view().querySelector('textarea')!, ' dropped')
  await userEvent.click(shortcuts()[1])
  await userEvent.click(form().cancel)
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(view().querySelector('.task-edit')).toBeNull()
  // The form opens again on the task as it is.
  await userEvent.click(editButton())
  expect(view().querySelector('textarea')).toHaveValue('first notes')
  expect(dates()[0]).toHaveValue('2026-11-23')
  expect(writes(calls)).toEqual([])
})

test('the ring beside the title field marks the task done at once and the form keeps what was entered, so Save sends no done', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  await open('/tasks/2')
  await userEvent.click(await waitFor(editButton))
  await userEvent.type(head().getByRole('textbox'), ' first')
  await userEvent.click(shortcuts()[1])
  await userEvent.click(ring())
  await waitFor(() => expect(ring()).toHaveAttribute('aria-pressed', 'true'))
  expect(rowOf('planned')).toHaveClass('done')
  expect(head().getByRole('textbox')).toHaveValue('planned first')
  expect(dates()[0]).toHaveValue('2026-11-24')
  await userEvent.click(form().save)
  await waitFor(() => expect(view().querySelector('.task-edit')).toBeNull())
  expect(writes(calls)).toEqual([
    ['PATCH', '/api/tasks/2', { done: true }],
    ['PATCH', '/api/tasks/2', { title: 'planned first', planned_from: '2026-11-24' }],
  ])
})

test('turning a phone while a task is edited keeps what was typed, and Save on the phone sends it', async () => {
  const turn = narrow(true)
  const calls = tasksServer(tasks())
  await open('/tasks/2')
  await userEvent.click(await waitFor(editButton))
  await userEvent.type(view().querySelector('textarea')!, ' typed')
  await userEvent.click(shortcuts()[1])
  act(() => turn(false))
  expect(view().querySelector('textarea')).toHaveValue('first notes typed')
  expect(dates()[0]).toHaveValue('2026-11-24')
  expect(screen.getByRole('main').querySelector('.panes > .box .task-read')).not.toBeNull()
  act(() => turn(true))
  await userEvent.click(form().save)
  await waitFor(() => expect(view().querySelector('.task-edit')).toBeNull())
  expect(writes(calls)).toEqual([['PATCH', '/api/tasks/2', { notes: 'first notes typed', planned_from: '2026-11-24' }]])
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

test('among tasks made in one call, one given a list from its page keeps its place before the list is read again', async () => {
  narrow(false)
  const made = '2026-11-23T08:00:00Z'
  const calls = tasksServer(['a', 'b', 'c'].map((title, i) => task(i + 1, 0, { title, planned_from: '2026-11-23', created_at: made })))
  await open('/tasks/2')
  await userEvent.click(await waitFor(editButton))
  await userEvent.click(lists()[1])
  await userEvent.click(form().save)
  await waitFor(() => expect(view().querySelector('.task-edit')).toBeNull())
  expect(rows().map((r) => r.querySelector('.t1')!.textContent)).toEqual(['a', 'b', 'c'])
  expect(reads(calls)).toBe(1)
})

test('leaving a task asks first once a date alone was changed, and asks nothing with nothing changed or for a change of the area chosen', async () => {
  narrow(false)
  const calls = tasksServer(tasks())
  const { where } = await open('/tasks/2')
  await userEvent.click(await waitFor(editButton))
  await userEvent.click(within(rowOf('to sort')).getByRole('link'))
  await waitFor(() => expect(where()).toBe('/tasks/1'))
  expect(screen.queryByRole('dialog')).toBeNull()

  await waitFor(() => expect(head().getByRole('heading', { level: 1 })).toHaveTextContent('to sort'))
  await userEvent.click(editButton())
  put(dates()[0], '2026-11-24')
  await userEvent.selectOptions(within(screen.getByRole('main').querySelector<HTMLElement>('.page-head')!).getByRole('combobox'), 'personal')
  await waitFor(() => expect(where()).toBe('/tasks/1?area=personal'))
  expect(screen.queryByRole('dialog')).toBeNull()

  await userEvent.click(within(rowOf('planned')).getByRole('link'))
  await userEvent.click(within(await screen.findByRole('dialog')).getAllByRole('button')[0])
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(where()).toBe('/tasks/1?area=personal')
  expect(dates()[0]).toHaveValue('2026-11-24')

  await userEvent.click(within(rowOf('planned')).getByRole('link'))
  await userEvent.click(within(await screen.findByRole('dialog')).getAllByRole('button')[1])
  await waitFor(() => expect(where()).toBe('/tasks/2?area=personal'))
  expect(writes(calls)).toEqual([])
})

test('a change made elsewhere shows without a reload and never replaces the form, and Save leaves it standing', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  narrow(false)
  const kept = tasks()
  const calls = tasksServer(kept)
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
  await user.click(editButton())
  expect(dates()[2]).toHaveValue('2026-12-24')

  await user.type(view().querySelector('textarea')!, ' mine')
  await elsewhere({ notes: 'from a client', title: 'retitled', deadline: '2026-12-31' })
  expect(head().getByRole('textbox')).toHaveValue('planned')
  expect(view().querySelector('textarea')).toHaveValue('first notes mine')
  expect(dates()[2]).toHaveValue('2026-12-24')
  expect(rowOf('retitled')).toBeDefined()

  await user.click(form().save)
  await waitFor(() => expect(head().getByRole('heading', { level: 1 })).toHaveTextContent('retitled'))
  expect(writes(calls)).toEqual([['PATCH', '/api/tasks/2', { notes: 'first notes mine' }]])
  expect(kept[1].deadline).toBe('2026-12-31')
})
