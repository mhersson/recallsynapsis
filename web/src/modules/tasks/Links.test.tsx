import { act, screen, waitFor, within } from '@testing-library/react'
import type { Linked, Session } from '../../api/types.ts'
import { note } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, fakeServer, json, narrow } from '../../test-server.ts'
import { areas } from './test-tasks.ts'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

const session: Session = { user: { name: 'ada' }, modules: ['notes', 'tasks'], version: '1.2.3', timezone: 'UTC', last_backup: null }
const linked = (id: number, state: string): Linked => ({ ref: `tasks:${id}`, kind: 'task', state, title: `task ${id}`, available: true })
const rows = () => [...screen.getByRole('main').querySelectorAll<HTMLElement>('.sect .lrow')]

test('a note shows the tasks it links to and those that link to it with their rings, each a link to its task, and a ring follows its task', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  narrow(false)
  const n = note(1, 10, { title: 'shelves', links: [linked(5, 'unsorted')], linked_from: [linked(6, 'open')] })
  fakeServer((c) => {
    if (c.path === '/api/session') return json(session)
    if (c.path.startsWith('/api/areas')) return json({ areas })
    if (c.path === '/api/notes/1') return json(n)
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
