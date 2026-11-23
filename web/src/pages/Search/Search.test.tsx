import { act, fireEvent, screen, within } from '@testing-library/react'
import type { Area, Group, Session } from '../../api/types.ts'
import { notes } from '../../modules/notes/index.tsx'
import { tasks } from '../../modules/tasks/index.tsx'
import { open } from '../../test-app.tsx'
import { FakeEventSource, fakeServer, json, narrow, type Call } from '../../test-server.ts'

beforeEach(() => vi.useFakeTimers())
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

const session = (modules: string[]): Session => ({ user: { name: 'ada' }, modules, version: '1.2.3', timezone: 'UTC', last_backup: null })
const area = (id: number, slug: string, archived = false): Area => ({ id, kind: 'project', name: slug.toUpperCase(), slug, description: '', archived })
const areas = [area(1, 'personal'), area(3, 'cavu'), area(6, 'homelander', true)]
const hit = (ref: string, state: string, kind = 'note') => ({ ref, kind, title: ref, snippet: '', area: 'cavu', state, changed_at: '2026-11-22T06:00:00Z' })

// A server with the notes module on unless told otherwise, whose search answers find(q, area).
function server(find: (q: string, area: string | null) => Group[], modules = ['notes']) {
  return fakeServer((c: Call) => {
    if (c.path === '/api/session') return json(session(modules))
    if (c.path.startsWith('/api/areas')) return json({ areas })
    if (c.path.startsWith('/api/search?')) {
      const p = new URLSearchParams(c.path.split('?')[1])
      return json({ groups: find(p.get('q')!, p.get('area')) })
    }
  })
}

const searches = (calls: Call[]) => calls.filter((c) => c.path.startsWith('/api/search')).map((c) => c.path)
const main = () => within(screen.getByRole('main'))
// user-event waits on a timer between steps, which fake timers hold, so these tests fire the events themselves.
const type = (text: string) => fireEvent.change(main().getByRole('searchbox'), { target: { value: text } })
const pause = () => act(() => vi.advanceTimersByTimeAsync(250))

test('typing asks once it pauses, and the hits show grouped by module', async () => {
  narrow(false)
  const calls = server(() => [
    { module: 'notes', hits: [hit('notes:1', 'inbox'), hit('notes:2', 'filed')] },
    { module: 'tasks', hits: [hit('tasks:5', 'open')] },
  ])
  const { where } = await open('/search')
  expect(searches(calls)).toEqual([])
  type('g')
  type('ga')
  type('gar')
  await pause()
  expect(searches(calls)).toEqual(['/api/search?q=gar'])
  expect(where()).toBe('/search?q=gar')

  const groups = main().getAllByRole('heading', { level: 2 })
  expect(groups.map((g) => g.textContent)).toEqual(['notes', 'tasks'])
  const first = groups[0].parentElement!
  expect(within(first).getByText('notes:1').parentElement).toHaveTextContent('CAVU')
})

test('a hit says its state as the module it names words it, and a hit of a module that gives no item says the state as it came', async () => {
  narrow(false)
  server(() => [
    { module: 'notes', hits: [hit('notes:1', 'inbox'), hit('notes:2', 'filed')] },
    { module: 'tasks', hits: [hit('tasks:5', 'open')] },
  ])
  await open('/search?q=gar')
  await pause()
  const details = [...screen.getByRole('main').querySelectorAll('.row .t2')].map((d) => [...d.children].map((s) => s.textContent))
  const inbox = notes.item!('notes:1', 'note', 'inbox').state
  expect(inbox).toBeTruthy()
  expect(details).toEqual([
    ['CAVU', 'note', inbox, expect.any(String)],
    ['CAVU', 'note', expect.any(String)],
    ['CAVU', 'note', 'open', expect.any(String)],
  ])
})

test('a hit is a link to its item with its mark, as the module it names gives them, and a hit of a module that gives none is no link', async () => {
  narrow(false)
  server(() => [
    { module: 'notes', hits: [hit('notes:1', 'inbox'), hit('notes:2', 'filed'), hit('notes:3', 'filed', 'idea')] },
    { module: 'tasks', hits: [hit('tasks:5', 'open')] },
  ])
  const { where } = await open('/search?q=gar')
  await pause()
  const rows = [...screen.getByRole('main').querySelectorAll('.row')]
  expect(rows.map((r) => [r.getAttribute('href'), r.querySelector('.mk')!.className])).toEqual([
    ['/notes/1', 'mk wait'],
    ['/notes/2', 'mk m-note'],
    ['/notes/3', 'mk m-idea'],
    [null, 'mk'],
  ])
  fireEvent.click(rows[2])
  await pause()
  expect(where()).toBe('/notes/3')
})

test('with tasks on its hits come before the notes’, though the server gives them after, each a link to its task with its ring and its state', async () => {
  narrow(false)
  server(
    () => [
      { module: 'notes', hits: [hit('notes:1', 'filed')] },
      { module: 'tasks', hits: [hit('tasks:5', 'open', 'task'), hit('tasks:6', 'unsorted', 'task'), hit('tasks:7', 'done', 'task')] },
    ],
    ['notes', 'tasks'],
  )
  const { where } = await open('/search?q=gar')
  await pause()
  expect(main().getAllByRole('heading', { level: 2 }).map((g) => g.textContent)).toEqual(['tasks', 'notes'])
  const rows = [...screen.getByRole('main').querySelectorAll('.row')]
  expect(rows.map((r) => [r.getAttribute('href'), r.querySelector('.mk')!.className])).toEqual([
    ['/tasks/5', 'mk m-task'],
    ['/tasks/6', 'mk m-task t-sort'],
    ['/tasks/7', 'mk m-task t-done'],
    ['/notes/1', 'mk m-note'],
  ])
  // The ring tells an open task; to sort and done are said too.
  const said = (state: string) => tasks.item!('tasks:1', 'task', state).state
  expect([said('unsorted'), said('done')].every(Boolean)).toBe(true)
  const details = rows.slice(0, 3).map((r) => [...r.querySelector('.t2')!.children].map((s) => s.textContent))
  expect(details).toEqual([
    ['CAVU', 'task', expect.any(String)],
    ['CAVU', 'task', said('unsorted'), expect.any(String)],
    ['CAVU', 'task', said('done'), expect.any(String)],
  ])
  fireEvent.click(rows[1])
  await pause()
  expect(where()).toBe('/tasks/6')
})

test('the area dropdown offers the areas that are not archived, and narrows the search to one', async () => {
  narrow(false)
  const calls = server(() => [])
  await open('/search?q=port')
  const select = main().getByRole('combobox')
  expect([...select.querySelectorAll('option')].map((o) => o.value)).toEqual(['', 'personal', 'cavu'])
  fireEvent.change(select, { target: { value: 'cavu' } })
  await pause()
  expect(searches(calls).at(-1)).toBe('/api/search?q=port&area=cavu')
})

test('nothing found in one area offers every area', async () => {
  narrow(true)
  const calls = server((_, area) => (area ? [] : [{ module: 'notes', hits: [hit('notes:9', 'filed')] }]))
  const { where } = await open('/search?q=garasjeport&area=cavu')
  await pause()
  fireEvent.click(main().getByRole('button'))
  await pause()
  expect(searches(calls)).toEqual(['/api/search?q=garasjeport&area=cavu', '/api/search?q=garasjeport'])
  expect(main().getByRole('combobox')).toHaveValue('')
  expect(main().getByRole('heading', { level: 2 })).toBeInTheDocument()
  expect(where()).toBe('/search?q=garasjeport')
})

test('an answer that comes after a newer question is never shown', async () => {
  narrow(false)
  let slow: (r: Response) => void = () => {}
  const calls = fakeServer((c) => {
    if (c.path === '/api/session') return json(session(['notes']))
    if (c.path.startsWith('/api/areas')) return json({ areas })
    if (c.path === '/api/search?q=a') return new Promise<Response>((r) => (slow = r))
    if (c.path.startsWith('/api/search')) return json({ groups: [] })
  })
  await open('/search')
  type('a')
  await pause()
  type('ab')
  await pause()
  await act(async () => slow(json({ groups: [{ module: 'notes', hits: [hit('notes:1', 'filed')] }] })))
  expect(searches(calls)).toEqual(['/api/search?q=a', '/api/search?q=ab'])
  expect(main().queryByRole('heading', { level: 2 })).toBeNull()
})

test('spaces around the words are not searched for, and a refused search says why', async () => {
  narrow(false)
  const calls = fakeServer((c) => {
    if (c.path === '/api/session') return json(session(['notes']))
    if (c.path.startsWith('/api/areas')) return json({ areas })
    if (c.path.startsWith('/api/search')) return json({ error: 'down' }, 502)
  })
  await open('/search')
  type(' port ')
  await pause()
  expect(searches(calls)).toEqual(['/api/search?q=port'])
  expect(main().getByRole('alert')).toBeInTheDocument()
})

test('with no module on there is nothing to search, and typing asks nothing', async () => {
  narrow(false)
  const calls = server(() => [], [])
  await open('/search')
  type('garden')
  await pause()
  expect(searches(calls)).toEqual([])
})

test('a change made by a module asks again, and a change to an area reads the areas alone', async () => {
  narrow(false)
  const calls = server(() => [])
  await open('/search?q=port')
  await pause()
  const es = FakeEventSource.all[0]
  await act(async () => es.open())
  const reads = () => calls.filter((c) => c.path.startsWith('/api/areas?')).length
  const before = [searches(calls).length, reads()]

  act(() => es.send({ module: 'core', kind: 'area', id: 3, area: 3 }))
  await act(() => vi.advanceTimersByTimeAsync(2000))
  await pause()
  expect([searches(calls).length, reads()]).toEqual([before[0], before[1] + 1])

  act(() => es.send({ module: 'notes', kind: 'note', id: 9, area: 3 }))
  await act(() => vi.advanceTimersByTimeAsync(2000))
  await pause()
  expect([searches(calls).length, reads()]).toEqual([before[0] + 1, before[1] + 1])
})
