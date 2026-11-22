import { act, fireEvent, screen, within } from '@testing-library/react'
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
const area = (id: number, kind: Area['kind'], name: string, archived = false): Area => ({ id, kind, name, slug: name, description: 'what it is', archived })

// A server that keeps the areas and applies each PATCH to them; answer, when given, answers first.
function server(areas: Area[], modules: string[] = [], answer: (c: Call) => Response | undefined = () => undefined) {
  return fakeServer((c) => {
    const a = answer(c)
    if (a) return a
    if (c.path === '/api/session') return json(session(modules))
    if (c.path.startsWith('/api/areas?')) return json({ areas })
    if (c.path === '/api/areas') return json({ areas: areas.filter((x) => !x.archived) })
    const changed = areas.find((x) => c.method === 'PATCH' && c.path === `/api/areas/${x.id}`)
    if (changed) return json(Object.assign(changed, c.body))
  })
}

const main = () => within(screen.getByRole('main'))
const buttons = () => main().queryAllByRole('button')
const patches = (calls: Call[]) => calls.filter((c) => c.method === 'PATCH').map((c) => [c.path, c.body])

test('a project is archived with a plain button and restored the same way', async () => {
  narrow(false)
  const calls = server([area(1, 'personal', 'Personal'), area(2, 'project', 'garden')])
  await open('/areas/2')
  const [, archive] = buttons()
  await userEvent.click(archive)
  expect(patches(calls)).toEqual([['/api/areas/2', { archived: true }]])
  expect(buttons()).toHaveLength(1)

  await userEvent.click(buttons()[0])
  expect(patches(calls)[1]).toEqual(['/api/areas/2', { archived: false }])
  expect(buttons()).toHaveLength(2)
})

test('the personal area keeps its name: it cannot be archived, and editing sends only its description', async () => {
  narrow(false)
  const calls = server([area(1, 'personal', 'Personal')])
  await open('/areas/1')
  expect(buttons()).toHaveLength(1)
  await userEvent.click(buttons()[0])
  const fields = main().getAllByRole('textbox')
  expect(fields).toHaveLength(1)
  await userEvent.clear(fields[0])
  await userEvent.type(fields[0], 'mine')
  await userEvent.click(buttons()[0])
  expect(patches(calls)).toEqual([['/api/areas/1', { description: 'mine' }]])
  expect(main().queryByRole('textbox')).toBeNull()
})

test('a project is renamed in place, and saving what did not change sends nothing', async () => {
  narrow(false)
  const calls = server([area(1, 'personal', 'Personal'), area(2, 'project', 'garden')])
  await open('/areas/2')
  await userEvent.click(buttons()[0])
  await userEvent.click(main().getAllByRole('button')[0])
  expect(patches(calls)).toEqual([])
  expect(main().queryByRole('textbox')).toBeNull()

  await userEvent.click(buttons()[0])
  const name = main().getAllByRole('textbox')[0]
  await userEvent.clear(name)
  await userEvent.type(name, 'allotment')
  await userEvent.click(buttons()[0])
  expect(patches(calls)).toEqual([['/api/areas/2', { name: 'allotment' }]])
  expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('allotment')
})

test('a name that is taken keeps the edit open and says so', async () => {
  narrow(false)
  server([area(1, 'personal', 'Personal'), area(2, 'project', 'garden')], [], (c) => (c.method === 'PATCH' ? json({ error: 'taken' }, 409) : undefined))
  await open('/areas/2')
  await userEvent.click(buttons()[0])
  await userEvent.type(main().getAllByRole('textbox')[0], 'x')
  await userEvent.click(buttons()[0])
  expect(main().getByRole('alert')).toBeInTheDocument()
  expect(main().getAllByRole('textbox')).toHaveLength(2)
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

test('an edit sends only what was changed in it, so a rename made elsewhere meanwhile stands', async () => {
  vi.useFakeTimers()
  try {
    narrow(false)
    const areas = [area(1, 'personal', 'Personal'), area(2, 'project', 'garden')]
    const calls = server(areas)
    await open('/areas/2')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    fireEvent.click(buttons()[0])

    areas[1].name = 'allotment'
    act(() => es.send({ module: 'core', kind: 'area', id: 2, area: 2 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    fireEvent.change(main().getAllByRole('textbox')[1], { target: { value: 'beds' } })
    await act(async () => fireEvent.click(buttons()[0]))
    expect(patches(calls)).toEqual([['/api/areas/2', { description: 'beds' }]])
    expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('allotment')
  } finally {
    vi.useRealTimers()
  }
})
