import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Area, Session } from '../../api/types.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, fakeServer, json, narrow, type Call } from '../../test-server.ts'

afterEach(() => vi.unstubAllGlobals())

const session: Session = { user: { name: 'ada' }, modules: [], version: '1.2.3', timezone: 'UTC', last_backup: null }
const area = (id: number, kind: Area['kind'], name: string, archived = false): Area => ({ id, kind, name, slug: name, brief: '', brief_revision: 0, brief_changed_at: null, brief_changed_by: '', archived })

// A server that keeps the areas it is given; answer, when given, answers first.
function server(areas: Area[], answer: (c: Call) => Response | undefined = () => undefined) {
  return fakeServer((c) => {
    const a = answer(c)
    if (a) return a
    if (c.path === '/api/session') return json(session)
    if (c.path === '/api/areas?archived=true') return json({ areas })
    if (c.path === '/api/areas') return json({ areas: areas.filter((x) => !x.archived) })
  })
}

const main = () => within(screen.getByRole('main'))
const newProject = () => main().getAllByRole('link').find((a) => a.getAttribute('href') === '/areas/new')!
const reads = (calls: Call[]) => calls.filter((c) => c.method === 'GET' && c.path === '/api/areas?archived=true').length

test('the list holds the personal area, then the projects, then the archived ones, each with Restore', async () => {
  narrow(false)
  const calls = server([area(1, 'personal', 'Personal'), area(3, 'project', 'cavu'), area(6, 'project', 'homelander', true), area(4, 'project', 'portal')])
  await open('/projects')
  expect(reads(calls)).toBe(1)
  expect(main().getAllByRole('link').map((a) => a.getAttribute('href'))).toEqual(['/areas/new', '/areas/1', '/areas/3', '/areas/4', '/areas/6'])
  expect(main().getAllByRole('button')).toHaveLength(1)
})

test('Restore brings an archived project back into the list', async () => {
  narrow(false)
  const areas = [area(1, 'personal', 'Personal'), area(6, 'project', 'homelander', true)]
  const calls = server(areas, (c) => {
    if (c.method !== 'PATCH') return
    areas[1].archived = false
    return json(areas[1])
  })
  await open('/projects')
  await userEvent.click(main().getByRole('button'))
  const patch = calls.find((c) => c.method === 'PATCH')!
  expect([patch.path, patch.body]).toEqual(['/api/areas/6', { archived: false }])
  expect(patch.headers.get('X-Recallsynapsis-CSRF')).toBeTruthy()
  expect(reads(calls)).toBe(2)
  expect(main().queryByRole('button')).toBeNull()
})

test('on desktop a project is made in a dialog over the list, which then holds it', async () => {
  narrow(false)
  const areas = [area(1, 'personal', 'Personal')]
  const calls = server(areas, (c) => {
    if (c.method !== 'POST') return
    areas.push(area(2, 'project', 'garden'))
    return json(areas[1], 201)
  })
  const { where } = await open('/projects')
  await userEvent.click(newProject())
  const dialog = within(screen.getByRole('dialog'))
  expect(where()).toBe('/areas/new')
  const [name, brief] = dialog.getAllByRole('textbox')
  await userEvent.type(name, ' garden ')
  await userEvent.type(brief, 'Beds and the greenhouse.')
  await userEvent.click(dialog.getAllByRole('button')[1])

  const post = calls.find((c) => c.method === 'POST')!
  expect([post.path, post.body]).toEqual(['/api/areas', { name: 'garden', brief: 'Beds and the greenhouse.' }])
  await waitFor(() => expect(where()).toBe('/projects'))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(main().getAllByRole('link').map((a) => a.getAttribute('href'))).toContain('/areas/2')
})

test('a name that is taken keeps the form open and says so', async () => {
  narrow(false)
  server([area(1, 'personal', 'Personal'), area(3, 'project', 'cavu')], (c) => (c.method === 'POST' ? json({ error: 'taken' }, 409) : undefined))
  const { where } = await open('/areas/new')
  const dialog = within(screen.getByRole('dialog'))
  await userEvent.type(dialog.getAllByRole('textbox')[0], 'Cavu')
  await userEvent.click(dialog.getAllByRole('button')[1])
  expect(dialog.getByRole('alert')).not.toHaveTextContent('taken')
  expect(where()).toBe('/areas/new')
})

test('Create waits for a name, and Cancel and Escape each close the dialog without a request', async () => {
  narrow(false)
  const calls = server([area(1, 'personal', 'Personal')])
  const { where } = await open('/areas/new')
  const [cancel, create] = within(screen.getByRole('dialog')).getAllByRole('button')
  expect(create).toBeDisabled()
  await userEvent.click(cancel)
  expect(where()).toBe('/projects')

  await userEvent.click(newProject())
  fireEvent(screen.getByRole('dialog'), new Event('cancel'))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(where()).toBe('/projects')
  expect(calls.filter((c) => c.method !== 'GET')).toEqual([])
})

test('the form opened from the side bar closes back to the page it was opened on', async () => {
  narrow(false)
  server([area(1, 'personal', 'Personal')])
  const { where } = await open('/areas/1')
  await userEvent.click(within(screen.getByRole('navigation')).getAllByRole('link').find((a) => a.getAttribute('href') === '/areas/new')!)
  await userEvent.click(within(screen.getByRole('dialog')).getAllByRole('button')[0])
  expect(where()).toBe('/areas/1')
})

test('the form opened from the list closes back to it, so Back leaves the list', async () => {
  narrow(false)
  server([area(1, 'personal', 'Personal')])
  const { where, back } = await open('/areas/1')
  await userEvent.click(within(screen.getByRole('navigation')).getAllByRole('link').find((a) => a.getAttribute('href') === '/projects')!)
  await userEvent.click(newProject())
  await userEvent.click(within(screen.getByRole('dialog')).getAllByRole('button')[0])
  expect(where()).toBe('/projects')
  await back()
  expect(where()).toBe('/areas/1')
})

test('the form opened by its address after a sign-in closes to the Projects list', async () => {
  narrow(false)
  let signedIn = false
  server([area(1, 'personal', 'Personal')], (c) => {
    if (c.path === '/api/auth/login') {
      signedIn = true
      return new Response(null, { status: 204 })
    }
    if (c.path === '/api/session' && !signedIn) return json({ error: 'sign in' }, 401)
  })
  const { where } = await open('/areas/new')
  expect(where()).toBe('/login')
  await userEvent.type(screen.getByRole('textbox'), 'ada')
  await userEvent.type(document.querySelector<HTMLInputElement>('input[type=password]')!, 'pw')
  await userEvent.click(screen.getByRole('button'))
  await userEvent.click(within(await screen.findByRole('dialog')).getAllByRole('button')[0])
  expect(where()).toBe('/projects')
  expect(screen.queryByRole('dialog')).toBeNull()
})

test('on a phone a project is made on a page of its own', async () => {
  narrow(true)
  const calls = server([area(1, 'personal', 'Personal')], (c) => (c.method === 'POST' ? json(area(2, 'project', 'garden'), 201) : undefined))
  const { where } = await open('/areas/new')
  expect(screen.queryByRole('dialog')).toBeNull()
  await userEvent.type(main().getAllByRole('textbox')[0], 'garden')
  await userEvent.click(main().getAllByRole('button')[0])
  expect(calls.some((c) => c.method === 'POST' && c.path === '/api/areas')).toBe(true)
  await waitFor(() => expect(where()).toBe('/projects'))
})

test('a project made in another browser appears without a reload', async () => {
  vi.useFakeTimers()
  try {
    narrow(false)
    const areas = [area(1, 'personal', 'Personal')]
    const calls = server(areas)
    await open('/projects')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    const before = reads(calls)

    areas.push(area(2, 'project', 'garden'))
    act(() => es.send({ module: 'core', kind: 'area', id: 2, area: 2 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(reads(calls)).toBe(before + 1)
    expect(main().getAllByRole('link').map((a) => a.getAttribute('href'))).toContain('/areas/2')
  } finally {
    vi.useRealTimers()
  }
})
