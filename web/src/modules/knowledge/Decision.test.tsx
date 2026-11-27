import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Group } from '../../api/types.ts'
import { note } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, fakeServer, json, narrow, type Call } from '../../test-server.ts'
import type { Full } from './api.ts'
import { areas, decision, linked, replace, row, session } from './test-knowledge.ts'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

// A server that keeps the decisions given, as the knowledge routes answer them; answer, when given, answers first.
function server(decisions: Full[], answer: (c: Call) => Response | undefined = () => undefined) {
  return fakeServer((c) => {
    const a = answer(c)
    if (a) return a
    if (c.path === '/api/session') return json(session())
    if (c.path.startsWith('/api/areas')) return json({ areas })
    const area = /^\/api\/knowledge\?area=(.+)$/.exec(c.path)?.[1]
    if (area) return json({ decisions: decisions.filter((d) => d.area === area).map(row), today: '2026-11-27' })
    const d = decisions.find((x) => c.path === `/api/knowledge/${x.id}`)
    if (d) return json(d)
    if (c.path.startsWith('/api/notes')) return json({ notes: [], total: 0 })
    if (c.path.startsWith('/api/tasks')) return json({ tasks: [], total: 0, now: { date: '2026-11-23', weekday: 'Monday', time: '10:00', timezone: 'UTC', week_start: 'monday' } })
  })
}

const main = () => within(screen.getByRole('main'))
const reads = (calls: Call[]) => calls.filter((c) => c.path.startsWith('/api/knowledge?'))
// The rows that lead to another decision, as [where each leads, its mark].
const leads = () => [...screen.getByRole('main').querySelectorAll('.kpart .lrow')].map((r) => [r.querySelector('a')!.getAttribute('href'), r.querySelector('.mk')!.className])

test('a decision reads at its own address, and its way back opens its project\'s page on Decisions', async () => {
  narrow(false)
  server([decision(7, '2026-11-26')])
  const { where } = await open('/decisions/7')
  await waitFor(() => expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('decision 7'))
  const back = main().getAllByRole('link').find((a) => a.getAttribute('href') === '/areas/2?tab=decisions')!
  await userEvent.click(back)
  expect(where()).toBe('/areas/2?tab=decisions')
})

test('a replaced decision shows what replaced it first, with the mark of its state, and a standing one leads to what it replaces', async () => {
  narrow(false)
  const [first, second, third] = [decision(5, '2026-11-20'), decision(6, '2026-11-22'), decision(7, '2026-11-26')]
  replace(second, first)
  replace(third, second)
  const calls = server([first, second, third])
  const page = await open('/decisions/5')
  await waitFor(() => expect(leads()).toEqual([['/decisions/6', 'mk m-decision d-replaced']]))
  expect(screen.getByRole('main').querySelector('.kpart')!.querySelector('.lrow')).not.toBeNull()
  page.unmount()

  calls.length = 0
  await open('/decisions/7')
  await waitFor(() => expect(leads()).toEqual([['/decisions/6', 'mk m-decision d-replaced']]))
  expect(screen.getByRole('main').querySelector('.decision-meta .mk')).not.toHaveClass('d-replaced')
  expect(reads(calls)).toEqual([])
})

test('Linked from lists each note and task that links to the decision, with its mark, and is absent when none does', async () => {
  narrow(false)
  const linking = decision(7, '2026-11-26', { linked_from: [linked('notes:3', 'note', 'filed'), linked('tasks:4', 'task', 'done')] })
  server([linking, decision(8, '2026-11-26')])
  const page = await open('/decisions/7')
  await waitFor(() => expect(screen.getByRole('main').querySelectorAll('.sect .lrow')).toHaveLength(2))
  expect([...screen.getByRole('main').querySelectorAll('.sect .lrow')].map((r) => [r.querySelector('a')!.getAttribute('href'), r.querySelector('.mk')!.className])).toEqual([
    ['/notes/3', 'mk m-note'],
    ['/tasks/4', 'mk m-task t-done'],
  ])
  page.unmount()

  await open('/decisions/8')
  await waitFor(() => expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('decision 8'))
  expect(screen.getByRole('main').querySelector('.sect')).toBeNull()
})

test.each([
  ['https://github.com/x/y/pull/41', true],
  ['http://example.test/a', true],
  ['The spec, section 2', false],
  ['https://example.test/a and the review', false],
  ['javascript:alert(1)', false],
])('the source %s is a link: %s', async (source, link) => {
  narrow(false)
  server([decision(7, '2026-11-26', { source })])
  await open('/decisions/7')
  await waitFor(() => expect(screen.getByRole('main').querySelector('.ksource')).not.toBeNull())
  const a = screen.getByRole('main').querySelector('.ksource a')
  if (!link) return expect(a).toBeNull()
  expect(a).toHaveAttribute('href', source)
  expect(a).toHaveAttribute('target', '_blank')
})

test('a decision mended or replaced elsewhere shows without a reload, and one deleted elsewhere says it is gone', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  narrow(false)
  const d = decision(7, '2026-11-26')
  const decisions = [d]
  server(decisions)
  await open('/decisions/7')
  const es = FakeEventSource.all[0]
  await act(async () => es.open())
  await waitFor(() => expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('decision 7'))

  d.decision = 'mended'
  act(() => es.send({ module: 'knowledge', kind: 'decision', id: 7, area: 2 }))
  await act(() => vi.advanceTimersByTimeAsync(2000))
  await waitFor(() => expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('mended'))

  // A decision added as replacing this one is all the stream names.
  const newer = decision(9, '2026-11-27')
  replace(newer, d)
  decisions.push(newer)
  act(() => es.send({ module: 'knowledge', kind: 'decision', id: 9, area: 2 }))
  await act(() => vi.advanceTimersByTimeAsync(2000))
  await waitFor(() => expect(screen.getByRole('main').querySelector('.decision-meta .mk')).toHaveClass('d-replaced'))
  expect(leads()).toEqual([['/decisions/9', 'mk m-decision']])

  decisions.length = 0
  act(() => es.send({ module: 'knowledge', kind: 'decision', id: 7, area: 2 }))
  await act(() => vi.advanceTimersByTimeAsync(2000))
  await waitFor(() => expect(main().getByRole('alert')).toBeInTheDocument())
  expect(main().queryByRole('heading', { level: 1 })).toBeNull()
})

test('the side bar marks the decision\'s project as current, and nothing else', async () => {
  narrow(false)
  server([decision(7, '2026-11-26')])
  await open('/decisions/7')
  const side = () => screen.getAllByRole('navigation')[0]
  await waitFor(() => expect(within(side()).getAllByRole('link').find((a) => a.getAttribute('href') === '/areas/2')).toHaveClass('on'))
  expect(within(side()).getAllByRole('link').filter((a) => a.classList.contains('on'))).toHaveLength(1)
})

test('a hit of a decision in Search shows its mark and its state, and opens its page', async () => {
  narrow(false)
  const hit = (id: number, state: string) => ({ ref: `knowledge:${id}`, kind: 'decision', title: `decision ${id}`, snippet: 'why', area: 'garden', state, changed_at: '2026-11-26T10:00:00Z' })
  const groups: Group[] = [{ module: 'knowledge', hits: [hit(7, 'stands'), hit(5, 'replaced')] }]
  server([], (c) => (c.path.startsWith('/api/search') ? json({ groups }) : undefined))
  const { where } = await open('/search?q=tasks')
  await waitFor(() => expect(main().getAllByRole('link').filter((a) => a.classList.contains('row'))).toHaveLength(2))
  const hits = main().getAllByRole('link').filter((a) => a.classList.contains('row'))
  expect(hits.map((h) => [h.getAttribute('href'), h.querySelector('.mk')!.className, h.querySelector('.t2')!.textContent!.includes(groups[0].hits[hits.indexOf(h)].state)])).toEqual([
    ['/decisions/7', 'mk m-decision', true],
    ['/decisions/5', 'mk m-decision d-replaced', true],
  ])
  await userEvent.click(hits[0])
  expect(where()).toBe('/decisions/7')
})

test('a note\'s link to a decision shows its mark and leads to it, tagged when it is replaced; with the module off it is unavailable', async () => {
  narrow(false)
  const links = [linked('knowledge:7', 'decision', 'stands'), linked('knowledge:5', 'decision', 'replaced')]
  const answer = (n: ReturnType<typeof note>) => (c: Call) =>
    c.path === '/api/notes/1' ? json(n) : c.path === '/api/notes/1/revisions' ? json({ revisions: [] }) : undefined
  server([], answer(note(1, 10, { title: 'saving', links })))
  const on = await open('/notes/1')
  const rows = () => [...screen.getByRole('main').querySelectorAll('.sect .lrow')]
  await waitFor(() => expect(rows()).toHaveLength(2))
  expect(rows().map((r) => [r.querySelector('a')!.getAttribute('href'), r.querySelector('.mk')!.className, !!r.querySelector('.tag')])).toEqual([
    ['/decisions/7', 'mk m-decision', false],
    ['/decisions/5', 'mk m-decision d-replaced', true],
  ])
  on.unmount()

  // The server resolves a link to a module switched off as unavailable.
  fakeServer((c) => {
    if (c.path === '/api/session') return json(session(['notes']))
    if (c.path.startsWith('/api/areas')) return json({ areas })
    if (c.path === '/api/notes/1') return json(note(1, 10, { title: 'saving', links: links.map((l) => ({ ...l, available: false })) }))
    if (c.path === '/api/notes/1/revisions') return json({ revisions: [] })
    if (c.path.startsWith('/api/notes')) return json({ notes: [], total: 0 })
  })
  await open('/notes/1')
  await waitFor(() => expect(rows()).toHaveLength(2))
  expect(rows().every((r) => !r.querySelector('a') && r.querySelector('.tag'))).toBe(true)
})
