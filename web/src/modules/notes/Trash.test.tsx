import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { note } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, json, narrow, type Call } from '../../test-server.ts'
import type { Note } from './api.ts'
import { notesServer } from './test-notes.ts'

afterEach(() => vi.unstubAllGlobals())

// A note trashed minutes ago, and one outside the trash.
const trashed = (id: number, minutes: number, n: Partial<Note> = {}) =>
  note(id, minutes + 60, { title: `note ${id}`, body: `body ${id}`, trashed_at: new Date(Date.now() - minutes * 60_000).toISOString(), ...n })
const filed = (id: number) => note(id, 1, { title: `note ${id}`, body: `body ${id}` })

// A server that keeps the notes and restores, deletes and empties as the server does; answer, when given, answers first.
function server(notes: Note[], answer: (c: Call) => Response | Promise<Response> | undefined = () => undefined) {
  return notesServer(notes, (c) => {
    const a = answer(c)
    if (a) return a
    if (c.method === 'DELETE' && c.path === '/api/notes/trash') {
      notes.splice(0, notes.length, ...notes.filter((n) => !n.trashed_at))
      return new Response(null, { status: 204 })
    }
    const n = notes.find((x) => c.path === `/api/notes/${x.id}` || c.path === `/api/notes/${x.id}/restore`)
    if (!n || c.method === 'GET') return
    if (c.method === 'POST') return json(Object.assign(n, { trashed_at: null }))
    notes.splice(notes.indexOf(n), 1)
    return new Response(null, { status: 204 })
  })
}

const main = () => screen.getByRole('main')
const rows = () => [...main().querySelectorAll<HTMLAnchorElement>('a.row')]
const hrefs = () => rows().map((r) => r.getAttribute('href'))
const tools = () => [...main().querySelectorAll<HTMLElement>('.note-read > .page-head .btn, .note-foot .btn')]
const writes = (calls: Call[]) => calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path])
const dialog = () => within(screen.getByRole('dialog')).getAllByRole('button')

test('Trash follows Ideas in the side bar and the idea bank in More', async () => {
  narrow(false)
  server([])
  const view = await open('/inbox')
  const nav = within(screen.getByRole('navigation')).getAllByRole('link').map((a) => a.getAttribute('href'))
  expect(nav.slice(0, 5)).toEqual(['/inbox', '/notes', '/ideas', '/trash', '/search'])
  view.unmount()

  narrow(true)
  server([])
  await open('/more')
  expect(within(main()).getAllByRole('link').map((a) => a.getAttribute('href'))).toEqual(['/ideas', '/trash', '/search', '/settings'])
})

test('Trash asks for the trash, lists it the most recently trashed first, and on desktop opens the newest beside it, read only', async () => {
  narrow(false)
  const calls = server([trashed(1, 30), trashed(2, 5), filed(3)])
  const { where } = await open('/trash')
  expect(calls.filter((c) => c.path.includes('trashed')).map((c) => c.path)).toEqual(['/api/notes?trashed=true'])
  expect(hrefs()).toEqual(['/trash/2', '/trash/1'])
  await waitFor(() => expect(where()).toBe('/trash/2'))
  expect(await within(main().querySelector<HTMLElement>('.note-read')!).findByRole('heading', { level: 1 })).toHaveTextContent('note 2')
  expect(main().querySelectorAll('.note-read textarea, .note-read input')).toHaveLength(0)
  expect(tools()).toHaveLength(2)
})

test('a trash with more says so and shows 50 more on asking', async () => {
  narrow(true)
  const calls = server(Array.from({ length: 60 }, (_, i) => trashed(i + 1, i + 1)))
  await open('/trash')
  await waitFor(() => expect(rows()).toHaveLength(50))
  await userEvent.click(within(main().querySelector<HTMLElement>('.list-end')!).getByRole('button'))
  await waitFor(() => expect(rows()).toHaveLength(60))
  expect(calls.filter((c) => c.path.includes('trashed')).map((c) => c.path)).toEqual(['/api/notes?trashed=true', '/api/notes?trashed=true&limit=100'])
  expect(main().querySelector('.list-end')).toBeNull()
})

test('Restore puts the note back: it leaves the trash, the page leads to it, and no other note opens unasked', async () => {
  narrow(false)
  const calls = server([trashed(1, 30), trashed(2, 5)])
  const { where } = await open('/trash')
  await waitFor(() => expect(tools()).toHaveLength(2))
  await userEvent.click(tools()[0])
  expect(writes(calls)).toEqual([['POST', '/api/notes/2/restore']])
  await waitFor(() => expect(where()).toBe('/trash'))
  await waitFor(() => expect(hrefs()).toEqual(['/trash/1']))
  expect(within(main().querySelector<HTMLElement>('.notice')!).getByRole('link')).toHaveAttribute('href', '/notes/2')
  expect(main().querySelector('.note-read')).toBeNull()
})

test('Delete for good asks first: keeping it sends nothing, and deleting it deletes it and closes it', async () => {
  narrow(false)
  const calls = server([trashed(1, 30), trashed(2, 5)])
  const { where } = await open('/trash/1')
  await waitFor(() => expect(tools()).toHaveLength(2))
  await userEvent.click(tools()[1])
  await userEvent.click(dialog()[0])
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(writes(calls)).toEqual([])

  await userEvent.click(tools()[1])
  await userEvent.click(dialog()[1])
  expect(writes(calls)).toEqual([['DELETE', '/api/notes/1']])
  await waitFor(() => expect(where()).toBe('/trash'))
  await waitFor(() => expect(hrefs()).toEqual(['/trash/2']))
  expect(main().querySelector('.note-read')).toBeNull()
})

test('a delete the server refuses shows its line and keeps the note open', async () => {
  narrow(false)
  server([trashed(1, 5)], (c) => (c.method === 'DELETE' ? json({ error: 'note 1 is not in the trash' }, 409) : undefined))
  const { where } = await open('/trash/1')
  await waitFor(() => expect(tools()).toHaveLength(2))
  await userEvent.click(tools()[1])
  await userEvent.click(dialog()[1])
  expect(await within(main().querySelector<HTMLElement>('.note-read')!).findByRole('alert')).toBeInTheDocument()
  expect(where()).toBe('/trash/1')
})

test('Empty the trash asks first, then empties it, and the trash says it is empty and offers it no more', async () => {
  narrow(true)
  const calls = server([trashed(1, 30), trashed(2, 5), filed(3)])
  await open('/trash')
  const empty = () => main().querySelector<HTMLElement>('.note-foot .btn')
  await waitFor(() => expect(empty()).not.toBeNull())
  await userEvent.click(empty()!)
  await userEvent.click(dialog()[0])
  expect(writes(calls)).toEqual([])

  await userEvent.click(empty()!)
  await userEvent.click(dialog()[1])
  expect(writes(calls)).toEqual([['DELETE', '/api/notes/trash']])
  await waitFor(() => expect(rows()).toEqual([]))
  expect(main().querySelector('.empty')).not.toBeNull()
  expect(empty()).toBeNull()
})

test('on a phone a trashed note opens as a page, and Restore goes back to the trash', async () => {
  narrow(true)
  const calls = server([trashed(1, 30), trashed(2, 5)])
  const { where } = await open('/trash')
  await userEvent.click(rows()[1])
  await waitFor(() => expect(where()).toBe('/trash/1'))
  expect(rows()).toEqual([])
  await waitFor(() => expect(tools()).toHaveLength(2))
  await userEvent.click(tools()[0])
  expect(writes(calls)).toEqual([['POST', '/api/notes/1/restore']])
  await waitFor(() => expect(where()).toBe('/trash'))
  await waitFor(() => expect(hrefs()).toEqual(['/trash/2']))
  expect(main().querySelector('.notice')).not.toBeNull()
})

test('the trash emptied elsewhere is read again', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    narrow(true)
    const notes = [trashed(1, 30)]
    server(notes)
    await open('/trash')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    expect(rows()).toHaveLength(1)
    notes.splice(0)
    act(() => es.send({ module: 'notes', kind: 'trash', id: 0, area: 0 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(rows()).toEqual([])
  } finally {
    vi.useRealTimers()
  }
})

test('a read of a trashed note asked before Restore, answered after it, does not put it back in the trash', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    narrow(false)
    const notes = [trashed(1, 5)]
    let hold = false
    let late: () => void = () => {}
    server(notes, (c) => {
      if (!hold || c.method !== 'GET' || c.path !== '/api/notes/1') return
      hold = false
      const then = { ...notes[0] }
      return new Promise<Response>((r) => (late = () => r(json(then))))
    })
    await open('/notes/1')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    await waitFor(() => expect(tools()).toHaveLength(2))
    hold = true
    act(() => es.send({ module: 'notes', kind: 'note', id: 9, area: 1 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    await userEvent.setup({ advanceTimers: vi.advanceTimersByTime }).click(tools()[0])
    await waitFor(() => expect(main().querySelector('.note-read .notice')).toBeNull())
    await act(async () => late())
    expect(main().querySelector('.note-read .notice')).toBeNull()
  } finally {
    vi.useRealTimers()
  }
})

test('a note open beside the trash that is deleted elsewhere closes to say so', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    narrow(false)
    const notes = [trashed(1, 5)]
    server(notes)
    await open('/trash/1')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    await waitFor(() => expect(tools()).toHaveLength(2))
    notes.splice(0)
    act(() => es.send({ module: 'notes', kind: 'trash', id: 0, area: 0 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(tools()).toEqual([])
    expect(main().querySelector('[role=alert]')).not.toBeNull()
  } finally {
    vi.useRealTimers()
  }
})
