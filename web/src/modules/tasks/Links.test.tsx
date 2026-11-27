import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Hit, Linked, Session } from '../../api/types.ts'
import { answerTasks, note, refs, task } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, fakeServer, json, narrow, type Call } from '../../test-server.ts'
import type { Task } from './api.ts'
import { areas, monday, taskView } from './test-tasks.ts'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

const session: Session = { user: { name: 'ada' }, modules: ['notes', 'tasks'], version: '1.2.3', timezone: 'UTC', last_backup: null }
const linked = (id: number, state: string): Linked => ({ ref: `tasks:${id}`, kind: 'task', state, title: `task ${id}`, available: true })
const rows = () => [...screen.getByRole('main').querySelectorAll<HTMLElement>('.sect .lrow')]
const section = () => screen.getByRole('main').querySelector<HTMLElement>('.sect')!
const writes = (calls: Call[]) => calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path, c.body])
const entries = () => taskView().querySelectorAll('.lg')

// A server with notes and tasks on that keeps the tasks given, and the notes given to resolve their links; answer, when
// given, answers first.
function server(tasks: Task[], notes = [note(7, 10, { title: 'measurements' }), note(8, 10, { kind: 'idea', title: 'workshop', links: refs('tasks:1') })], answer: (c: Call) => Response | undefined = () => undefined) {
  return fakeServer((c) => {
    const a = answer(c)
    if (a) return a
    if (c.path === '/api/session') return json(session)
    if (c.path.startsWith('/api/areas')) return json({ areas })
    if (!c.path.startsWith('/api/tasks')) return
    const [status, body] = answerTasks(tasks, c.method, new URL(c.path, 'http://here'), (c.body ?? {}) as Record<string, unknown>, monday, () => {}, notes)
    return status === 204 ? new Response(null, { status }) : json(body, status)
  })
}

// A task that links to a note, a done task and a note gone for good, and is linked from an idea and a task to sort.
const linking = (): Task[] => [
  task(1, 60, { title: 'shelves', list: 'soon', links: refs('notes:7', 'tasks:2', 'notes:99') }),
  task(2, 50, { title: 'brackets', done_at: '2026-11-20T10:00:00Z' }),
  task(3, 40, { title: 'tidy', links: refs('tasks:1') }),
]

test('a note shows the tasks it links to and those that link to it with their rings, each a link to its task, and a ring follows its task', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  narrow(false)
  const n = note(1, 10, { title: 'shelves', links: [linked(5, 'unsorted')], linked_from: [linked(6, 'open')] })
  fakeServer((c) => {
    if (c.path === '/api/session') return json(session)
    if (c.path.startsWith('/api/areas')) return json({ areas })
    if (c.path === '/api/notes/1') return json(n)
    if (c.path === '/api/notes/1/revisions') return json({ revisions: [] })
    if (c.path.startsWith('/api/notes')) return json({ notes: [], total: 0 })
    if (c.path.startsWith('/api/tasks')) return json({ tasks: [], total: 0, now: { date: '2026-11-23', weekday: 'Monday', time: '10:00', timezone: 'UTC', week_start: 'monday' } })
  })
  await open('/notes/1')
  const es = FakeEventSource.all[0]
  await act(async () => es.open())
  await waitFor(() => expect(rows()).toHaveLength(2))
  expect(rows().map((r) => [within(r).getByRole('link').getAttribute('href'), r.querySelector('.mk')!.className])).toEqual([
    ['/tasks/5', 'mk m-task t-sort'],
    ['/tasks/6', 'mk m-task'],
  ])

  // The task is marked done elsewhere: the note is read again, and the ring is filled.
  n.links = [linked(5, 'done')]
  act(() => es.send({ module: 'tasks', kind: 'task', id: 5, area: 1 }))
  await act(() => vi.advanceTimersByTimeAsync(2000))
  await waitFor(() => expect(rows()[0].querySelector('.mk')).toHaveClass('t-done'))
})

test('a task shows what it links to and what links to it, each a link to its page with its mark, and a link to what is gone leads nowhere', async () => {
  narrow(false)
  const calls = server(linking())
  await open('/tasks/1')
  await waitFor(() => expect(rows()).toHaveLength(5))
  expect(rows().map((r) => [within(r).queryByRole('link')?.getAttribute('href') ?? null, r.querySelector('.mk')!.className])).toEqual([
    ['/notes/7', 'mk m-note'],
    ['/tasks/2', 'mk m-task t-done'],
    [null, 'mk'],
    ['/notes/8', 'mk m-idea'],
    ['/tasks/3', 'mk m-task t-sort'],
  ])
  // Only what the task links to can be removed here.
  expect(rows().map((r) => within(r).queryAllByRole('button').length)).toEqual([1, 1, 1, 0, 0])
  expect(writes(calls)).toEqual([])
})

test('the x removes a link at once, as a change of the task that its log tells of', async () => {
  narrow(false)
  const calls = server(linking())
  await open('/tasks/1')
  await waitFor(() => expect(rows()).toHaveLength(5))
  expect(entries()).toHaveLength(1)
  await userEvent.click(within(rows()[1]).getByRole('button'))
  expect(writes(calls)).toEqual([['PATCH', '/api/tasks/1', { links: ['notes:7', 'notes:99'] }]])
  await waitFor(() => expect(rows()).toHaveLength(4))
  expect(entries()).toHaveLength(2)
})

test('Add a link beside the heading finds items through search, leaves out the task itself, shows what is linked already, and links the one picked', async () => {
  narrow(false)
  const tasks = linking()
  const hit = (ref: string, kind: string, state: string): Hit => ({ ref, kind, title: ref, snippet: '', area: 'personal', state, changed_at: tasks[0].changed_at })
  const calls = server(tasks, undefined, (c) =>
    c.path.startsWith('/api/search?')
      ? json({ groups: [{ module: 'notes', hits: [hit('notes:7', 'note', 'filed')] }, { module: 'tasks', hits: [hit('tasks:1', 'task', 'open'), hit('tasks:3', 'task', 'unsorted')] }] })
      : undefined,
  )
  await open('/tasks/1')
  await userEvent.click(await waitFor(() => section().querySelector<HTMLElement>('.sect-head .btn')!))
  await userEvent.type(within(section()).getByRole('searchbox'), 'sh')
  const finder = () => within(section().querySelector<HTMLElement>('.finder')!)
  // The task itself is left out, and the tasks come before the notes.
  await waitFor(() => expect(finder().getAllByRole('button').map((b) => b.querySelector('.t1')?.textContent ?? '')).toEqual(['tasks:3', 'notes:7', '']))
  const [tidy, linkedAlready] = finder().getAllByRole('button')
  expect(linkedAlready).toBeDisabled()
  await userEvent.click(tidy)
  expect(writes(calls)).toEqual([['PATCH', '/api/tasks/1', { links: ['notes:7', 'tasks:2', 'notes:99', 'tasks:3'] }]])
  await waitFor(() => expect(section().querySelector('.finder')).toBeNull())
  expect(rows()).toHaveLength(6)
})

test('a task in the trash shows its links, and neither removes nor adds one', async () => {
  narrow(false)
  const tasks = linking()
  tasks[0].trashed_at = new Date().toISOString()
  server(tasks)
  await open('/tasks/trashed/1')
  await waitFor(() => expect(rows()).toHaveLength(5))
  expect(within(section()).queryAllByRole('button')).toEqual([])
})

test('on a phone the links and the log are folded, and Add a link opens the finder within the links', async () => {
  narrow(true)
  server(linking())
  await open('/tasks/1')
  const [links, log] = await waitFor(() => {
    const folds = [...taskView().querySelectorAll<HTMLDetailsElement>('details.sect')]
    expect(folds).toHaveLength(2)
    return folds
  })
  expect([links.open, log.open]).toEqual([false, false])
  await userEvent.click(links.querySelector('summary')!)
  await userEvent.click(within(links).getAllByRole('button').at(0)!)
  expect(within(links).getByRole('searchbox')).toHaveFocus()
})
