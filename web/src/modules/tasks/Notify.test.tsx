import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { KINDS } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, fakePush, fakeServer, json, narrow, type Call } from '../../test-server.ts'
import type { Kinds } from './api.ts'

afterEach(() => vi.unstubAllGlobals())

beforeEach(() => {
  narrow(false)
  fakePush()
})

// A server with the modules given switched on, tasks and push unless told, that keeps the kinds it is given; answer,
// when given, answers first.
function server(kinds: Kinds = { ...KINDS }, { modules = ['tasks', 'push'], answer }: { modules?: string[]; answer?: (c: Call) => Response | undefined } = {}) {
  return fakeServer((c) => {
    const a = answer?.(c)
    if (a) return a
    if (c.path === '/api/session') return json({ user: { name: 'ada' }, modules, version: '1.2.3', timezone: 'UTC', last_backup: null })
    if (c.path.startsWith('/api/areas')) return json({ areas: [] })
    if (c.path === '/api/tokens') return json({ tokens: [] })
    if (c.path.startsWith('/api/push')) return json({ key: '-_8', on: 0, browser: null })
    if (c.path === '/api/tasks/notify') {
      if (c.method === 'PATCH') Object.assign(kinds, c.body)
      return json(kinds)
    }
  })
}

// Settings holds no other switch and no other time field.
const switches = () => screen.getAllByRole('switch')
const on = () => switches().map((s) => s.getAttribute('aria-checked') === 'true')
const hours = () => [...document.querySelectorAll<HTMLInputElement>('input[type=time]')]
const type = (i: number, value: string) => fireEvent.change(hours()[i], { target: { value } })
const sent = (calls: Call[]) => calls.filter((c) => c.method === 'PATCH').map((c) => c.body)
const acts = () => document.querySelector<HTMLElement>('.notify > .acts')
// Cancel and Save, in the order shown; Save is the primary one.
function form() {
  const buttons = within(acts()!).getAllByRole('button')
  return { order: buttons.map((b) => (b.classList.contains('primary') ? 'save' : 'cancel')), save: buttons.find((b) => b.classList.contains('primary'))!, cancel: buttons.find((b) => !b.classList.contains('primary'))! }
}

test('the kinds show what the server holds, each switch and hour named, and an hour dimmed while its kind is off', async () => {
  const calls = server({ ...KINDS, untimed: true, untimed_at: '08:45' })
  await open('/settings')
  await waitFor(() => expect(switches()).toHaveLength(4))
  expect(screen.getAllByRole('switch', { name: /\S/ })).toHaveLength(4)
  expect(on()).toEqual([true, true, false, true])
  expect(hours().map((h) => h.value)).toEqual(['07:00', '18:00', '08:45'])
  expect(hours().every((h) => h.labels?.length === 1)).toBe(true)
  expect(hours().map((h) => h.classList.contains('off'))).toEqual([false, true, false])
  expect(acts()).toBeNull()
  expect(sent(calls)).toEqual([])
})

test('a switch flipped sends its one field at once and shows what the server answers', async () => {
  const kinds = { ...KINDS }
  // The server answers with what another browser changed meanwhile too.
  const calls = server(kinds, { answer: (c) => (c.method === 'PATCH' ? json(Object.assign(kinds, c.body, { untimed: true })) : undefined) })
  await open('/settings')
  await waitFor(() => expect(switches()).toHaveLength(4))
  await userEvent.click(switches()[2])
  await waitFor(() => expect(on()).toEqual([true, true, true, true]))
  expect(sent(calls)).toEqual([{ deadline: true }])
  expect(hours()[1]).not.toHaveClass('off')

  await userEvent.click(switches()[0])
  await waitFor(() => expect(on()[0]).toBe(false))
  expect(sent(calls)).toEqual([{ deadline: true }, { start: false }])
})

test.each([false, true])('an hour changed brings up Cancel and Save, Save sending the hours that differ, on a phone %s', async (phone) => {
  narrow(phone)
  const calls = server()
  await open('/settings')
  await waitFor(() => expect(hours()).toHaveLength(3))
  type(0, '06:30')
  type(2, '08:15')
  // On desktop Cancel then Save, and on a phone Save above Cancel.
  expect(form().order).toEqual(phone ? ['save', 'cancel'] : ['cancel', 'save'])
  await userEvent.click(form().save)
  await waitFor(() => expect(acts()).toBeNull())
  expect(sent(calls)).toEqual([{ morning_at: '06:30', untimed_at: '08:15' }])
  expect(hours().map((h) => h.value)).toEqual(['06:30', '18:00', '08:15'])
})

test('Cancel puts the hours back, and an hour typed back as it was needs no Save', async () => {
  const calls = server()
  await open('/settings')
  await waitFor(() => expect(hours()).toHaveLength(3))
  type(1, '19:00')
  type(1, '18:00')
  expect(acts()).toBeNull()

  type(0, '06:30')
  type(1, '19:00')
  await userEvent.click(form().cancel)
  expect(acts()).toBeNull()
  expect(hours().map((h) => h.value)).toEqual(['07:00', '18:00', '09:00'])
  expect(sent(calls)).toEqual([])
})

test('Save is quiet while an hour is empty or half typed', async () => {
  server()
  await open('/settings')
  await waitFor(() => expect(hours()).toHaveLength(3))
  type(0, '')
  expect(form().save).toBeDisabled()
  type(0, '06:30')
  expect(form().save).toBeEnabled()
})

test('an hour can be set while its kind is off, and a switch flipped then sends its field alone and keeps the hour', async () => {
  const calls = server()
  await open('/settings')
  await waitFor(() => expect(hours()).toHaveLength(3))
  expect(hours()[1]).toBeEnabled()
  type(1, '17:30')
  await userEvent.click(switches()[2])
  await waitFor(() => expect(on()[2]).toBe(true))
  expect(sent(calls)).toEqual([{ deadline: true }])
  expect(hours()[1]).toHaveValue('17:30')
  await userEvent.click(form().save)
  await waitFor(() => expect(acts()).toBeNull())
  expect(sent(calls)).toEqual([{ deadline: true }, { deadline_at: '17:30' }])
})

test('a refused change shows its line, and the control shows what the server holds', async () => {
  const calls = server(undefined, { answer: (c) => (c.method === 'PATCH' ? json({ error: 'internal error' }, 500) : undefined) })
  await open('/settings')
  await waitFor(() => expect(switches()).toHaveLength(4))
  await userEvent.click(switches()[0])
  expect(await screen.findByRole('alert')).toBeInTheDocument()
  expect(on()).toEqual([true, true, false, false])

  // A refused Save keeps the hour as typed, to be sent again.
  type(0, '06:30')
  await userEvent.click(form().save)
  await waitFor(() => expect(sent(calls)).toHaveLength(2))
  expect(screen.getByRole('alert')).toBeInTheDocument()
  expect(hours()[0]).toHaveValue('06:30')
  expect(form().save).toBeEnabled()
})

test('a change made in another browser shows without a reload, and an hour being changed here stays as typed', async () => {
  const kinds = { ...KINDS }
  server(kinds)
  await open('/settings')
  await waitFor(() => expect(hours()).toHaveLength(3))
  type(0, '06:30')

  Object.assign(kinds, { untimed: true, morning_at: '08:00', untimed_at: '10:00' })
  await act(async () => FakeEventSource.all[0].send({ module: 'tasks', kind: 'notify', id: 0, area: 0 }))
  await waitFor(() => expect(on()).toEqual([true, true, false, true]), { timeout: 3000 })
  expect(hours().map((h) => h.value)).toEqual(['06:30', '18:00', '10:00'])
  // Cancel now puts back what the server holds.
  await userEvent.click(form().cancel)
  expect(hours()[0]).toHaveValue('08:00')
})

test('leaving with an hour changed asks first, outside the part, which a phone may have folded', async () => {
  narrow(true)
  server()
  const view = await open('/settings')
  await waitFor(() => expect(hours()).toHaveLength(3))
  type(0, '06:30')
  const tasks = document.querySelector<HTMLElement>('a[href="/tasks"]')!

  await userEvent.click(tasks)
  expect(screen.getByRole('dialog').closest('details')).toBeNull()
  const [keep] = within(screen.getByRole('dialog')).getAllByRole('button')
  await userEvent.click(keep)
  expect(view.where()).toBe('/settings')
  expect(hours()[0]).toHaveValue('06:30')

  await userEvent.click(tasks)
  await userEvent.click(within(screen.getByRole('dialog')).getAllByRole('button')[1])
  await waitFor(() => expect(view.where()).toBe('/tasks'))
})

test('with push off, or tasks off, the kinds are not shown and not asked for', async () => {
  for (const modules of [['tasks'], ['push']]) {
    const calls = server(undefined, { modules })
    const view = await open('/settings')
    await waitFor(() => expect(document.getElementById('session')).not.toBeNull())
    expect(document.querySelectorAll('[role=switch]')).toHaveLength(0)
    expect(calls.some((c) => c.path === '/api/tasks/notify')).toBe(false)
    view.unmount()
  }
})
