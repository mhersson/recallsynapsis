import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Area, Session } from '../../api/types.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, fakeServer, json, narrow, type Call } from '../../test-server.ts'

// A stand-in module whose section is a region named by the id of the area it is shown in.
vi.mock('../../modules/registry.ts', async () => {
  const { createElement } = await import('react')
  const section = ({ area }: { area: Area }) => createElement('section', { 'aria-label': String(area.id) })
  return { modules: [{ name: 'things', nav: [], tabs: [], more: [], routes: [], section }] }
})

afterEach(() => vi.unstubAllGlobals())

const session = (modules: string[]): Session => ({ user: { name: 'ada' }, modules, version: '1.2.3', timezone: 'UTC', last_backup: null })
const area = (id: number, kind: Area['kind'], name: string, archived = false): Area => ({
  id,
  kind,
  name,
  slug: name,
  brief: 'what it is',
  brief_revision: 3,
  brief_changed_at: '2026-11-20T10:00:00Z',
  brief_changed_by: 'web',
  archived,
})

// A server that keeps the areas and applies each PATCH to them, refusing a brief changed from another revision than the
// area's; answer, when given, answers first.
function server(areas: Area[], modules: string[] = [], answer: (c: Call) => Response | undefined = () => undefined) {
  return fakeServer((c) => {
    const a = answer(c)
    if (a) return a
    if (c.path === '/api/session') return json(session(modules))
    if (c.path.startsWith('/api/areas?')) return json({ areas })
    if (c.path === '/api/areas') return json({ areas: areas.filter((x) => !x.archived) })
    const changed = areas.find((x) => c.method === 'PATCH' && c.path === `/api/areas/${x.id}`)
    if (!changed) return
    const { brief_revision: from, ...body } = c.body as Partial<Area>
    if (from !== undefined && from !== changed.brief_revision) return json({ error: 'moved on: conflict' }, 409)
    return json(Object.assign(changed, body, from !== undefined && { brief_revision: from + 1 }))
  })
}

const main = () => within(screen.getByRole('main'))
const buttons = () => main().queryAllByRole('button')
const head = () => within(screen.getByRole('main').querySelector<HTMLElement>('.page-head')!).queryAllByRole('button')
const form = () => screen.getByRole('main').querySelector('form')
const fields = () => main().getAllByRole('textbox')
// Save submits the form, and Cancel is a plain button beside it.
const save = () => within(form()!).getAllByRole('button').find((b) => b.getAttribute('type') !== 'button')!
const cancel = () => within(form()!).getByRole('button', { name: (_, b) => b.getAttribute('type') === 'button' })
const patches = (calls: Call[]) => calls.filter((c) => c.method === 'PATCH').map((c) => [c.path, c.body])

test('a project is archived with a plain button and restored the same way', async () => {
  narrow(false)
  const calls = server([area(1, 'personal', 'Personal'), area(2, 'project', 'garden')])
  await open('/areas/2')
  const [, archive] = buttons()
  await userEvent.click(archive)
  expect(patches(calls)).toEqual([['/api/areas/2', { archived: true }]])
  expect(head()).toHaveLength(1)

  await userEvent.click(buttons()[0])
  expect(patches(calls)[1]).toEqual(['/api/areas/2', { archived: false }])
  expect(head()).toHaveLength(2)
})

test('the personal area keeps its name: it cannot be archived, and editing sends only its brief, from the revision it opened with', async () => {
  narrow(false)
  const calls = server([area(1, 'personal', 'Personal')])
  await open('/areas/1')
  expect(head()).toHaveLength(1)
  await userEvent.click(head()[0])
  expect(fields()).toHaveLength(1)
  await userEvent.clear(fields()[0])
  await userEvent.type(fields()[0], 'mine')
  await userEvent.click(save())
  expect(patches(calls)).toEqual([['/api/areas/1', { brief: 'mine', brief_revision: 3 }]])
  expect(form()).toBeNull()
})

test('a project is renamed in place, saving what did not change sends nothing, and Cancel sends nothing and asks nothing', async () => {
  narrow(false)
  const calls = server([area(1, 'personal', 'Personal'), area(2, 'project', 'garden')])
  await open('/areas/2')
  await userEvent.click(head()[0])
  await userEvent.type(fields()[1], ' {enter}')
  await userEvent.click(save())
  expect(patches(calls)).toEqual([])
  expect(form()).toBeNull()

  await userEvent.click(head()[0])
  await userEvent.type(fields()[1], ' and more')
  await userEvent.click(cancel())
  expect(form()).toBeNull()
  expect(screen.queryByRole('dialog')).toBeNull()

  await userEvent.click(head()[0])
  await userEvent.clear(fields()[0])
  await userEvent.type(fields()[0], 'allotment')
  await userEvent.click(save())
  expect(patches(calls)).toEqual([['/api/areas/2', { name: 'allotment' }]])
  expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('allotment')
})

test('Cancel sits before Save on desktop, and Save above Cancel on a phone', async () => {
  narrow(false)
  server([area(1, 'personal', 'Personal'), area(2, 'project', 'garden')])
  const desk = await open('/areas/2')
  await userEvent.click(head()[0])
  expect(within(form()!).getAllByRole('button')).toEqual([cancel(), save()])
  desk.unmount()

  narrow(true)
  server([area(1, 'personal', 'Personal'), area(2, 'project', 'garden')])
  await open('/areas/2')
  await userEvent.click(within(screen.getByRole('main').querySelector<HTMLElement>('.area-foot')!).getAllByRole('button')[0])
  expect(within(form()!).getAllByRole('button')).toEqual([save(), cancel()])
})

test('a name that is taken keeps the edit open and says so', async () => {
  narrow(false)
  server([area(1, 'personal', 'Personal'), area(2, 'project', 'garden')], [], (c) => (c.method === 'PATCH' ? json({ error: 'taken' }, 409) : undefined))
  await open('/areas/2')
  await userEvent.click(head()[0])
  await userEvent.type(fields()[0], 'x')
  await userEvent.click(save())
  expect(main().getByRole('alert')).not.toHaveTextContent('taken')
  expect(fields()).toHaveLength(2)
})

test('a name taken while the brief stands is told as a taken name, with the brief sent along or not, and the brief keeps its revision', async () => {
  narrow(false)
  const calls = server([area(1, 'personal', 'Personal'), area(2, 'project', 'garden'), area(3, 'project', 'shed')], [], (c) =>
    c.method === 'PATCH' && (c.body as Area).name === 'shed' ? json({ error: 'taken: conflict' }, 409) : undefined,
  )
  await open('/areas/2')
  await userEvent.click(head()[0])
  await userEvent.clear(fields()[0])
  await userEvent.type(fields()[0], 'shed')
  await userEvent.click(save())
  const alone = (await main().findByRole('alert')).textContent

  await userEvent.type(fields()[1], ' still')
  await userEvent.click(save())
  await waitFor(() => expect(calls.at(-1)!.path).toBe('/api/areas?archived=true'))
  expect(main().getByRole('alert').textContent).toBe(alone)
  await userEvent.click(save())
  expect(patches(calls).slice(1)).toEqual([
    ['/api/areas/2', { name: 'shed', brief: 'what it is still', brief_revision: 3 }],
    ['/api/areas/2', { name: 'shed', brief: 'what it is still', brief_revision: 3 }],
  ])
})

test('a brief over the cap is refused by the server, and the form keeps it with the server\'s line above Save', async () => {
  narrow(false)
  const line = 'a brief is at most 4000 characters: invalid'
  const calls = server([area(1, 'personal', 'Personal'), area(2, 'project', 'garden')], [], (c) =>
    c.method === 'PATCH' && [...(c.body as Area).brief].length > 4000 ? json({ error: line }, 400) : undefined,
  )
  await open('/areas/2')
  await userEvent.click(head()[0])
  const long = 'x'.repeat(4001)
  fireEvent.change(fields()[1], { target: { value: long } })
  await userEvent.click(save())
  expect(patches(calls)).toEqual([['/api/areas/2', { brief: long, brief_revision: 3 }]])
  const alert = main().getByRole('alert')
  expect(alert).toHaveTextContent(line)
  expect(alert.compareDocumentPosition(save()) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(fields()[1]).toHaveValue(long)
})

test('a brief saved elsewhere refuses the save here: the form keeps what was entered, and a second Save sends it from the revision that came in', async () => {
  narrow(false)
  const areas = [area(1, 'personal', 'Personal'), area(2, 'project', 'garden')]
  const calls = server(areas)
  await open('/areas/2')
  await userEvent.click(head()[0])
  Object.assign(areas[1], { brief: 'theirs', brief_revision: 4, brief_changed_by: 'aim' })
  await userEvent.type(fields()[1], ' still')
  await userEvent.click(save())
  expect(await main().findByRole('alert')).toBeInTheDocument()
  expect(fields()[1]).toHaveValue('what it is still')

  await userEvent.click(save())
  expect(patches(calls)).toEqual([
    ['/api/areas/2', { brief: 'what it is still', brief_revision: 3 }],
    ['/api/areas/2', { brief: 'what it is still', brief_revision: 4 }],
  ])
  expect(form()).toBeNull()
  expect(screen.getByRole('main').querySelector('.brief .md')).toHaveTextContent('what it is still')
})

test('after a save refused because the brief moved on, Cancel sends nothing and leaves the brief as it came in', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    narrow(false)
    const areas = [area(1, 'personal', 'Personal'), area(2, 'project', 'garden')]
    const calls = server(areas)
    await open('/areas/2')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    await userEvent.click(head()[0])
    Object.assign(areas[1], { brief: 'theirs', brief_revision: 4, brief_changed_by: 'aim' })
    act(() => es.send({ module: 'core', kind: 'area', id: 2, area: 2 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    await userEvent.type(fields()[1], ' still')
    await userEvent.click(save())
    expect(await main().findByRole('alert')).toBeInTheDocument()

    await userEvent.click(cancel())
    expect(patches(calls)).toHaveLength(1)
    expect(form()).toBeNull()
    expect(screen.getByRole('main').querySelector('.brief .md')).toHaveTextContent('theirs')
  } finally {
    vi.useRealTimers()
  }
})

test('leaving the form with a change asks first, and with none does not', async () => {
  narrow(false)
  server([area(1, 'personal', 'Personal'), area(2, 'project', 'garden')])
  const { where } = await open('/areas/2')
  const away = () => userEvent.click(within(screen.getByRole('navigation')).getAllByRole('link').find((a) => a.getAttribute('href') === '/areas/1')!)
  await userEvent.click(head()[0])
  await userEvent.type(fields()[1], ' still')
  await away()
  const asked = within(screen.getByRole('dialog'))
  await userEvent.click(asked.getAllByRole('button')[0])
  expect(where()).toBe('/areas/2')
  expect(fields()[1]).toHaveValue('what it is still')

  await userEvent.click(cancel())
  await userEvent.click(head()[0])
  await away()
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(where()).toBe('/areas/1')
})

test('each enabled module shows its section for the area, and none shows while it is off', async () => {
  narrow(false)
  server([area(1, 'personal', 'Personal'), area(2, 'project', 'garden')], ['things'])
  const on = await open('/areas/2')
  expect(main().getByRole('region', { name: '2' })).toBeInTheDocument()
  on.unmount()

  server([area(1, 'personal', 'Personal'), area(2, 'project', 'garden')])
  await open('/areas/2')
  expect(main().queryByRole('region')).toBeNull()
})

test('going to another area ends an edit', async () => {
  narrow(false)
  server([area(1, 'personal', 'Personal'), area(2, 'project', 'garden')])
  await open('/areas/2')
  await userEvent.click(buttons()[0])
  await userEvent.click(within(screen.getByRole('navigation')).getAllByRole('link').find((a) => a.getAttribute('href') === '/areas/1')!)
  expect(main().queryByRole('textbox')).toBeNull()
  expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('Personal')
})

test('an area that does not exist leads to the Projects list', async () => {
  narrow(true)
  server([area(1, 'personal', 'Personal')])
  const { where } = await open('/areas/9')
  expect(where()).toBe('/projects')
})

test('a change made elsewhere shows without a reload', async () => {
  vi.useFakeTimers()
  try {
    narrow(true)
    const areas = [area(1, 'personal', 'Personal'), area(2, 'project', 'garden')]
    server(areas)
    await open('/areas/2')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    areas[1].name = 'allotment'
    act(() => es.send({ module: 'core', kind: 'area', id: 2, area: 2 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('allotment')
  } finally {
    vi.useRealTimers()
  }
})

test('an edit sends only what was changed in it, so a rename made elsewhere meanwhile stands, and no read replaces the form', async () => {
  vi.useFakeTimers()
  try {
    narrow(false)
    const areas = [area(1, 'personal', 'Personal'), area(2, 'project', 'garden')]
    const calls = server(areas)
    await open('/areas/2')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    fireEvent.click(head()[0])
    fireEvent.change(fields()[1], { target: { value: 'beds' } })

    areas[1].name = 'allotment'
    act(() => es.send({ module: 'core', kind: 'area', id: 2, area: 2 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(fields().map((f) => (f as HTMLInputElement).value)).toEqual(['garden', 'beds'])
    await act(async () => fireEvent.click(save()))
    expect(patches(calls)).toEqual([['/api/areas/2', { brief: 'beds', brief_revision: 3 }]])
    expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('allotment')
  } finally {
    vi.useRealTimers()
  }
})
