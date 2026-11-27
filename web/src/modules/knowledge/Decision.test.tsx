import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Group } from '../../api/types.ts'
import { note } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, fakeServer, json, narrow, type Call } from '../../test-server.ts'
import { areas, decision, keeper, linked, replace, session } from './test-knowledge.ts'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

const main = () => within(screen.getByRole('main'))
const reads = (calls: Call[]) => calls.filter((c) => c.path.startsWith('/api/knowledge?'))
// The rows that lead to another decision, as [where each leads, its mark].
const leads = () => [...screen.getByRole('main').querySelectorAll('.kpart .lrow')].map((r) => [r.querySelector('a')!.getAttribute('href'), r.querySelector('.mk')!.className])

test('a decision reads at its own address, and its way back opens its project\'s page on Decisions', async () => {
  narrow(false)
  keeper([decision(7, '2026-11-26')])
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
  const calls = keeper([first, second, third])
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
  keeper([linking, decision(8, '2026-11-26')])
  const page = await open('/decisions/7')
  await waitFor(() => expect(screen.getByRole('main').querySelectorAll('.sect .lrow')).toHaveLength(2))
  expect([...screen.getByRole('main').querySelectorAll('.sect .lrow')].map((r) => [r.querySelector('a')!.getAttribute('href'), r.querySelector('.mk')!.className])).toEqual([
    ['/notes/3', 'mk m-note'],
    ['/tasks/4', 'mk m-task t-done'],
  ])
  page.unmount()

  await open('/decisions/8')
  await waitFor(() => expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('decision 8'))
  expect(screen.getByRole('main').querySelector('section.sect')).toBeNull()
})

test.each([
  ['https://github.com/x/y/pull/41', true],
  ['http://example.test/a', true],
  ['The spec, section 2', false],
  ['https://example.test/a and the review', false],
  ['javascript:alert(1)', false],
])('the source %s is a link: %s', async (source, link) => {
  narrow(false)
  keeper([decision(7, '2026-11-26', { source })])
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
  keeper(decisions)
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
  keeper([decision(7, '2026-11-26')])
  await open('/decisions/7')
  const side = () => screen.getAllByRole('navigation')[0]
  await waitFor(() => expect(within(side()).getAllByRole('link').find((a) => a.getAttribute('href') === '/areas/2')).toHaveClass('on'))
  expect(within(side()).getAllByRole('link').filter((a) => a.classList.contains('on'))).toHaveLength(1)
})

test('a hit of a decision in Search shows its mark and its state, and opens its page', async () => {
  narrow(false)
  const hit = (id: number, state: string) => ({ ref: `knowledge:${id}`, kind: 'decision', title: `decision ${id}`, snippet: 'why', area: 'garden', state, changed_at: '2026-11-26T10:00:00Z' })
  const groups: Group[] = [{ module: 'knowledge', hits: [hit(7, 'stands'), hit(5, 'replaced')] }]
  keeper([], (c) => (c.path.startsWith('/api/search') ? json({ groups }) : undefined))
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
  keeper([], answer(note(1, 10, { title: 'saving', links })))
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

test('Edit, Replace and Delete sit at the head of a decision on desktop and at its foot on a phone', async () => {
  const acts = (scope: string) => [...screen.getByRole('main').querySelectorAll(`${scope} :is(button, a.btn)`)]
  narrow(false)
  keeper([decision(7, '2026-11-26')])
  const desk = await open('/decisions/7')
  await waitFor(() => expect(acts('.page-head')).toHaveLength(3))
  expect(acts('.kfoot')).toHaveLength(0)
  desk.unmount()

  narrow(true)
  keeper([decision(7, '2026-11-26')])
  await open('/decisions/7')
  await waitFor(() => expect(acts('.kfoot')).toHaveLength(3))
  expect(acts('.page-head')).toHaveLength(0)
})

test('Delete asks first, with Keep it holding the focus and Escape keeping the decision; deleting opens the project on Decisions, where the one it replaced stands again', async () => {
  narrow(false)
  const [seven, six, five] = [decision(7, '2026-11-26'), decision(6, '2026-11-22'), decision(5, '2026-11-20')]
  replace(seven, five)
  const calls = keeper([seven, six, five])
  const { where } = await open('/decisions/7')
  const remove = () => userEvent.click(within(screen.getByRole('main').querySelector<HTMLElement>('.page-head')!).getAllByRole('button').at(-1)!)
  const writes = () => calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path])
  await waitFor(() => expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('decision 7'))
  await remove()
  // The browser gives a modal dialog's first control the focus.
  const [keep] = within(screen.getByRole('dialog')).getAllByRole('button')
  await userEvent.click(keep)
  expect(screen.queryByRole('dialog')).toBeNull()

  await remove()
  fireEvent(screen.getByRole('dialog'), new Event('cancel', { cancelable: true }))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(writes()).toEqual([])

  await remove()
  await userEvent.click(within(screen.getByRole('dialog')).getAllByRole('button')[1])
  expect(writes()).toEqual([['DELETE', '/api/knowledge/7']])
  await waitFor(() => expect(where()).toBe('/areas/2?tab=decisions'))
  const standing = () => [...screen.getByRole('main').querySelectorAll('.decisions > table .l a')].map((a) => a.getAttribute('href'))
  await waitFor(() => expect(standing()).toEqual(['/decisions/6', '/decisions/5']))
})

test('a delete the server refuses keeps the decision, with the server\'s line', async () => {
  narrow(false)
  keeper([decision(7, '2026-11-26')], (c) => (c.method === 'DELETE' ? json({ error: 'no' }, 500) : undefined))
  const { where } = await open('/decisions/7')
  await waitFor(() => expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('decision 7'))
  await userEvent.click(within(screen.getByRole('main').querySelector<HTMLElement>('.page-head')!).getAllByRole('button').at(-1)!)
  await userEvent.click(within(screen.getByRole('dialog')).getAllByRole('button')[1])
  expect(await main().findByRole('alert')).toBeInTheDocument()
  expect(where()).toBe('/decisions/7')
})

test('History lists each revision once unfolded, the newest first; Read shows that wording, offers no restore, and leads back to the decision', async () => {
  narrow(false)
  const [seven, five] = [decision(7, '2026-11-26', { revision: 3 }), decision(5, '2026-11-20')]
  replace(seven, five)
  const entry = (revision: number, said = '') => ({ revision, at: `2026-11-2${revision}T10:00:00Z`, by: 'aim', said })
  // The second wording replaced a decision since deleted, the first one that stands.
  const wording = (revision: number, replaces: string) => ({ ...entry(revision), decision: `wording ${revision}`, why: 'why then', source: '', decided: '2026-11-25', replaces })
  const calls = keeper([seven, five], (c) => {
    if (c.path === '/api/knowledge/7/revisions') return json({ revisions: [entry(3), entry(2), entry(1, 'record it')] })
    const n = /^\/api\/knowledge\/7\/revisions\/(\d)$/.exec(c.path)?.[1]
    if (n) return json(wording(Number(n), n === '2' ? 'knowledge:4' : 'knowledge:5'))
  })
  await open('/decisions/7')
  await waitFor(() => expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('decision 7'))
  const history = () => [...screen.getByRole('main').querySelectorAll<HTMLDetailsElement>('details.sect')].at(-1)!
  expect(calls.filter((c) => c.path.includes('/revisions'))).toEqual([])
  await userEvent.click(history().querySelector('summary')!)
  await waitFor(() => expect(history().querySelectorAll('.hrow')).toHaveLength(3))
  const rows = () => [...history().querySelectorAll('.hrow')]
  expect(rows().map((r) => [r.querySelector('.n')!.textContent!.replace(/\D/g, ''), !!r.querySelector('button'), !!r.querySelector('.said')])).toEqual([
    ['3', false, false],
    ['2', true, false],
    ['1', true, true],
  ])

  const frame = () => screen.getByRole('main').querySelector<HTMLElement>('.revision')!
  await userEvent.click(rows()[1].querySelector('button')!)
  await waitFor(() => expect(frame().querySelector('.rt')).toHaveTextContent('wording 2'))
  expect(within(frame()).getAllByRole('button')).toHaveLength(1)
  await waitFor(() => expect(frame().querySelector('.lrow .tag')).not.toBeNull())
  expect(frame().querySelector('.lrow a')).toBeNull()
  await userEvent.click(within(frame()).getByRole('button'))

  await waitFor(() => expect(history().open).toBe(true))
  await waitFor(() => expect(rows()).toHaveLength(3))
  await userEvent.click(rows()[2].querySelector('button')!)
  await waitFor(() => expect(frame().querySelector('.lrow a')).toHaveAttribute('href', '/decisions/5'))
  expect(calls.filter((c) => c.method !== 'GET')).toEqual([])
})

test('nothing is sent to the server but by Save and by Delete for good', async () => {
  narrow(false)
  const [seven, five] = [decision(7, '2026-11-26', { revision: 2 }), decision(5, '2026-11-20')]
  replace(seven, five)
  const entry = { revision: 1, at: '2026-11-25T10:00:00Z', by: 'aim', said: '' }
  const calls = keeper([seven, five], (c) =>
    c.path === '/api/knowledge/7/revisions' ? json({ revisions: [entry] }) : c.path === '/api/knowledge/7/revisions/1' ? json({ ...entry, decision: 'first', why: 'then', source: '', decided: '2026-11-25', replaces: '' }) : undefined,
  )
  await open('/decisions/7')
  await waitFor(() => expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('decision 7'))
  const head = () => within(screen.getByRole('main').querySelector<HTMLElement>('.page-head')!)
  const form = () => screen.getByRole('main').querySelector('form')!
  await userEvent.click([...screen.getByRole('main').querySelectorAll('summary')].at(-1)!)
  await waitFor(() => expect(screen.getByRole('main').querySelector('.hrow button')).not.toBeNull())
  await userEvent.click(screen.getByRole('main').querySelector<HTMLElement>('.hrow button')!)
  await waitFor(() => expect(screen.getByRole('main').querySelector('.rev .rt')).not.toBeNull())
  await userEvent.click(within(screen.getByRole('main').querySelector<HTMLElement>('.revision')!).getByRole('button'))

  await userEvent.click(head().getAllByRole('button')[0])
  await userEvent.type(within(form()).getAllByRole('textbox')[1], ' more')
  fireEvent.change(form().querySelector('input[type="date"]')!, { target: { value: '2026-11-01' } })
  await userEvent.selectOptions(within(form()).getByRole('combobox'), '')
  await userEvent.click(within(form()).getByRole('button', { name: (_, b) => b.getAttribute('type') === 'button' }))

  await userEvent.click(head().getAllByRole('button').at(-1)!)
  await userEvent.click(within(screen.getByRole('dialog')).getAllByRole('button')[0])

  await userEvent.click(head().getAllByRole('link')[0])
  await waitFor(() => expect(form()).not.toBeNull())
  await userEvent.type(within(form()).getAllByRole('textbox')[0], 'newer')
  await userEvent.click(within(form()).getByRole('button', { name: (_, b) => b.getAttribute('type') === 'button' }))
  await waitFor(() => expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('decision 7'))
  expect(calls.filter((c) => c.method !== 'GET')).toEqual([])
})
