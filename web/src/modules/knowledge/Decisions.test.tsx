import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { open } from '../../test-app.tsx'
import { FakeEventSource, fakeServer, json, narrow, type Call } from '../../test-server.ts'
import type { Full } from './api.ts'
import { areas, decision, replace, row, session } from './test-knowledge.ts'

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

// A server with the modules given on that keeps the decisions given, as the knowledge routes answer them.
function server(decisions: Full[], modules?: string[]) {
  return fakeServer((c) => {
    if (c.path === '/api/session') return json(session(modules))
    if (c.path.startsWith('/api/areas')) return json({ areas })
    const area = /^\/api\/knowledge\?area=(.+)$/.exec(c.path)?.[1]
    if (area) return json({ decisions: decisions.filter((d) => d.area === area).map(row), today: '2026-11-27' })
    const d = decisions.find((x) => c.path === `/api/knowledge/${x.id}`)
    if (d) return json(d)
    if (c.path.startsWith('/api/notes')) return json({ notes: [], total: 0 })
    if (c.path.startsWith('/api/tasks')) return json({ tasks: [], total: 0, now: { date: '2026-11-23', weekday: 'Monday', time: '10:00', timezone: 'UTC', week_start: 'monday' } })
  })
}

// Two that stand in the garden, the newer replacing a third, newest first as the server lists them.
function garden(): Full[] {
  const [older, newer, other] = [decision(5, '2026-11-20'), decision(7, '2026-11-26', { source: 'https://example.test/pull/41' }), decision(6, '2026-11-22')]
  replace(newer, older)
  return [newer, other, older]
}

const main = () => screen.getByRole('main')
const tabs = () => within(main()).queryByRole('navigation')
const knowledge = (calls: Call[]) => calls.filter((c) => c.path.startsWith('/api/knowledge'))
// Where each decision of a table or a list of blocks leads.
const rows = (scope: ParentNode) => [...scope.querySelectorAll(':is(.l > div, .kline) > a')].map((a) => a.getAttribute('href'))
const standing = () => main().querySelector<HTMLElement>('.decisions')!.firstElementChild!
const fold = () => main().querySelector<HTMLDetailsElement>('.kfold')!

test('with knowledge on a project shows Activity, holding the page as it was, and Decisions; the personal area shows none, and with knowledge off no page shows them and none is read', async () => {
  narrow(false)
  const on = server(garden())
  const project = await open('/areas/2')
  expect(within(tabs()!).getAllByRole('link').map((a) => a.getAttribute('href'))).toEqual(['/areas/2', '/areas/2?tab=decisions'])
  expect(main().querySelector('.area-parts')).not.toBeNull()
  project.unmount()

  on.length = 0
  const personal = await open('/areas/1')
  expect(tabs()).toBeNull()
  expect(knowledge(on)).toEqual([])
  personal.unmount()

  const off = server(garden(), ['notes', 'tasks'])
  await open('/areas/2?tab=decisions')
  expect(tabs()).toBeNull()
  expect(main().querySelector('.area-parts')).not.toBeNull()
  expect(knowledge(off)).toEqual([])
})

test('Decisions carries the number of those that stand, and none when none does', async () => {
  narrow(false)
  const calls = server(garden())
  const counted = await open('/areas/2')
  await waitFor(() => expect(tabs()!.querySelector('.dim')).toHaveTextContent('2'))
  counted.unmount()

  await open('/areas/3')
  await waitFor(() => expect(knowledge(calls).map((c) => c.path)).toContain('/api/knowledge?area=shed'))
  await act(async () => {})
  expect(tabs()!.querySelector('.dim')).toBeEmptyDOMElement()
})

test('the table lists those that stand, the newest first, and the replaced beneath under their count, folded', async () => {
  narrow(false)
  server(garden())
  await open('/areas/2?tab=decisions')
  await waitFor(() => expect(rows(standing())).toEqual(['/decisions/7', '/decisions/6']))
  expect(standing().tagName).toBe('TABLE')
  expect(fold().open).toBe(false)
  expect(fold().querySelector('summary .dim')).toHaveTextContent('1')
  expect(rows(fold())).toEqual(['/decisions/5'])
})

test('on a phone each decision is a block', async () => {
  narrow(true)
  server(garden())
  await open('/areas/2?tab=decisions')
  await waitFor(() => expect(rows(standing())).toEqual(['/decisions/7', '/decisions/6']))
  expect(main().querySelector('table')).toBeNull()
  expect(rows(fold())).toEqual(['/decisions/5'])
})

test('a project with no decision says so', async () => {
  narrow(false)
  server(garden())
  await open('/areas/3?tab=decisions')
  await waitFor(() => expect(main().querySelector('.empty')).not.toBeNull())
  expect(main().querySelector('table')).toBeNull()
})

test('a click on a row opens its decision, but on a link in it or as a selection ends, and the decision\'s way back opens the project on Decisions', async () => {
  narrow(false)
  server(garden())
  const { where } = await open('/areas/2?tab=decisions')
  await waitFor(() => expect(rows(standing())).toHaveLength(2))
  const first = standing().querySelector('tbody tr')!
  await userEvent.click(first.querySelector<HTMLAnchorElement>('a[target="_blank"]')!)
  expect(where()).toBe('/areas/2?tab=decisions')
  // A why selected to be copied is not a click to open.
  getSelection()!.selectAllChildren(first.querySelector('td > .md')!)
  fireEvent.click(first.querySelector('td > .md')!)
  expect(where()).toBe('/areas/2?tab=decisions')
  getSelection()!.removeAllRanges()

  await userEvent.click(first.querySelector('td > .md')!)
  expect(where()).toBe('/decisions/7')
  await waitFor(() => expect(within(main()).getByRole('heading', { level: 1 })).toHaveTextContent('decision 7'))
  await userEvent.click(within(main()).getAllByRole('link').find((a) => a.getAttribute('href') === '/areas/2?tab=decisions')!)
  expect(where()).toBe('/areas/2?tab=decisions')
  await waitFor(() => expect(rows(standing())).toHaveLength(2))
})

test('a decision added, mended or deleted elsewhere shows on the tab without a reload, with the number of those that stand', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  narrow(false)
  const decisions = garden()
  const [seven, six, five] = decisions
  server(decisions)
  await open('/areas/2?tab=decisions')
  const es = FakeEventSource.all[0]
  await act(async () => es.open())
  await waitFor(() => expect(rows(standing())).toEqual(['/decisions/7', '/decisions/6']))
  const told = async (id: number) => {
    act(() => es.send({ module: 'knowledge', kind: 'decision', id, area: 2 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
  }

  decisions.unshift(decision(8, '2026-11-27'))
  await told(8)
  await waitFor(() => expect(rows(standing())).toEqual(['/decisions/8', '/decisions/7', '/decisions/6']))
  expect(tabs()!.querySelector('.dim')).toHaveTextContent('3')

  six.decision = 'mended'
  await told(6)
  await waitFor(() => expect(standing().querySelector('a[href="/decisions/6"]')).toHaveTextContent('mended'))

  // Deleting the decision that replaced another makes the other stand again, though the stream names only the one deleted.
  decisions.splice(decisions.indexOf(seven), 1)
  Object.assign(five, { state: 'stands', replaced_by: null })
  await told(7)
  await waitFor(() => expect(rows(standing())).toEqual(['/decisions/8', '/decisions/6', '/decisions/5']))
  expect(main().querySelector('.kfold')).toBeNull()
})

test('on a phone a decision\'s page keeps More current in the tab bar', async () => {
  narrow(true)
  server(garden())
  await open('/decisions/7')
  const bar = screen.getAllByRole('navigation').at(-1)!
  expect(within(bar).getAllByRole('link').find((a) => a.getAttribute('aria-current'))).toHaveAttribute('href', '/more')
})
