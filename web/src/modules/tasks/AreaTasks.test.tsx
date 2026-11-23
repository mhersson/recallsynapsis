import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { task } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { json, narrow, type Call } from '../../test-server.ts'
import type { Task } from './api.ts'
import { monday, tasksServer } from './test-tasks.ts'

afterEach(() => vi.unstubAllGlobals())

const part = () => screen.getByRole('main').querySelector<HTMLElement>('.area-tasks')!
// The titles in each group of the project's tasks, in order.
const shown = () => [...part().querySelectorAll('[data-group]')].map((g) => [g.getAttribute('data-group'), [...g.querySelectorAll('.t1')].map((t) => t.textContent)])
const rowOf = (title: string) => [...part().querySelectorAll<HTMLElement>('.task-row')].find((r) => r.querySelector('.t1')?.textContent === title)!
const writes = (calls: Call[]) => calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path, c.body])

// The project garden's tasks, one in each of four groups, and one of the personal area.
const tasks = (): Task[] => [
  task(1, 60, { title: 'sort', area: 'garden' }),
  task(2, 50, { title: 'today', area: 'garden', planned_from: '2026-11-23' }),
  task(3, 40, { title: 'weekend', area: 'garden', planned_from: '2026-11-28', planned_to: '2026-11-29', list: 'soon' }),
  task(4, 30, { title: 'soon', area: 'garden', list: 'soon' }),
  task(5, 20, { title: 'personal', planned_from: '2026-11-23' }),
]

// Tasks and notes on, with no note in the project.
const server = (ts: Task[]) => tasksServer(ts, monday, (c) => (c.path.startsWith('/api/notes') ? json({ notes: [], total: 0 }) : undefined), ['tasks', 'notes'])

test('a project’s page shows its tasks alone, each once, grouped as the views group them, first of the modules’ sections', async () => {
  narrow(false)
  const calls = server(tasks())
  await open('/areas/2')
  await waitFor(() =>
    expect(shown()).toEqual([
      ['sort', ['sort']],
      ['today', ['today']],
      ['week', ['weekend']],
      ['soon', ['soon']],
    ]),
  )
  expect(screen.getByRole('main').querySelector('.area-parts')!.firstElementChild).toBe(part())
  expect(within(rowOf('today')).getByRole('link')).toHaveAttribute('href', '/tasks/2?area=garden')
  expect(part().querySelector('.afoot a')).toHaveAttribute('href', '/tasks?view=done&area=garden')
  expect(writes(calls)).toEqual([])
})

test('the add line adds a task to sort to the project, and a ring marks a task done in place, struck through', async () => {
  narrow(false)
  const calls = server(tasks())
  await open('/areas/2')
  await userEvent.type(await waitFor(() => within(part()).getByRole('textbox')), 'new one{Enter}')
  await waitFor(() => expect(shown()[0]).toEqual(['sort', ['sort', 'new one']]))
  await userEvent.click(within(rowOf('today')).getAllByRole('button')[0])
  await waitFor(() => expect(rowOf('today')).toHaveClass('done'))
  expect(shown()[1]).toEqual(['today', ['today']])
  expect(writes(calls)).toEqual([
    ['POST', '/api/tasks', { tasks: [{ title: 'new one', area: 'garden' }] }],
    ['PATCH', '/api/tasks/2', { done: true }],
  ])
})

test('on a phone the project’s tasks are open and the notes and ideas folded', async () => {
  narrow(true)
  server(tasks())
  await open('/areas/2')
  await waitFor(() => expect(shown()).toHaveLength(4))
  const folds = [...screen.getByRole('main').querySelectorAll<HTMLDetailsElement>('.area-parts details')]
  expect(folds.map((d) => [d === part(), d.open])).toEqual([
    [true, true],
    [false, false],
    [false, false],
  ])
})
