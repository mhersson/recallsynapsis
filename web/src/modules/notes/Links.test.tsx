import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Hit } from '../../api/types.ts'
import { full, note, refs } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, json, narrow, type Call } from '../../test-server.ts'
import type { Note } from './api.ts'
import { capture, notesServer } from './test-notes.ts'

afterEach(() => vi.unstubAllGlobals())

const filed = (id: number, n: Partial<Note> = {}) => note(id, id, { title: `note ${id}`, body: `body ${id}`, ...n })

// A server that keeps the notes and changes a note's links from the revision a change names, as the server does;
// answer, when given, answers first.
function server(notes: Note[], answer: (c: Call) => Response | Promise<Response> | undefined = () => undefined) {
  return notesServer(notes, (c) => {
    const a = answer(c)
    if (a || c.method !== 'PATCH') return a
    const n = notes.find((x) => c.path === `/api/notes/${x.id}`)!
    const { revision, links } = c.body as { revision: number; links: string[] }
    if (revision !== n.revision) return json({ error: 'conflict' }, 409)
    return json(full(notes, Object.assign(n, { links: refs(...links), revision: n.revision + 1 })))
  })
}

const hit = (n: Note, snippet = n.body): Hit => ({ ref: n.ref, kind: n.kind, title: n.title || n.body, snippet, area: n.area, state: n.inbox ? 'inbox' : 'filed', changed_at: n.changed_at })
const section = () => screen.getByRole('main').querySelector<HTMLElement>('.sect')!
const rows = () => [...section().querySelectorAll<HTMLElement>('.lrow')]
const writes = (calls: Call[]) => calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path, c.body])
const finder = () => within(section().querySelector<HTMLElement>('.finder')!)

test('a note shows what it links to and what links to it, each a link to its page with its mark, and a link to what is gone leads nowhere', async () => {
  narrow(false)
  server([filed(1, { links: refs('notes:2', 'notes:9') }), filed(2, { kind: 'idea' }), capture(3, 1, { links: refs('notes:1') })])
  await open('/notes/1')
  await waitFor(() => expect(rows()).toHaveLength(3))
  const [to, gone, from] = rows()
  expect(within(to).getByRole('link')).toHaveAttribute('href', '/notes/2')
  expect(to.querySelector('.mk')).toHaveClass('m-idea')
  expect(within(from).getByRole('link')).toHaveAttribute('href', '/notes/3')
  expect(from.querySelector('.mk')).toHaveClass('wait')
  expect(within(gone).queryByRole('link')).toBeNull()
  expect(gone.querySelector('.mk')!.className).toBe('mk')
  // Only what the note links to can be removed here.
  expect([to, gone, from].map((r) => within(r).queryAllByRole('button').length)).toEqual([1, 1, 0])
})

test('the x removes a link at once, as a change of the note with its revision, and a refused change shows why and reads the note again', async () => {
  narrow(false)
  const notes = [filed(1, { revision: 4, links: refs('notes:2', 'notes:9') }), filed(2)]
  let refuse = false
  const calls = server(notes, (c) => (refuse && c.method === 'PATCH' ? json({ error: 'note 1 is at revision 6' }, 409) : undefined))
  await open('/notes/1')
  await waitFor(() => expect(rows()).toHaveLength(2))
  await userEvent.click(within(rows()[1]).getByRole('button'))
  expect(writes(calls)).toEqual([['PATCH', '/api/notes/1', { revision: 4, links: ['notes:2'] }]])
  await waitFor(() => expect(rows()).toHaveLength(1))

  refuse = true
  const reads = calls.filter((c) => c.method === 'GET' && c.path === '/api/notes/1').length
  await userEvent.click(within(rows()[0]).getByRole('button'))
  expect(await within(section()).findByRole('alert')).toBeInTheDocument()
  expect(calls.filter((c) => c.method === 'GET' && c.path === '/api/notes/1').length).toBe(reads + 1)
  expect(rows()).toHaveLength(1)
})

test('while a change is sent no other can be asked, and its answer, come after a newer read of the note, is dropped', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    narrow(false)
    const notes = [filed(1, { revision: 4, links: refs('notes:2', 'notes:3') }), filed(2), filed(3)]
    let answer: () => void = () => {}
    server(notes, (c) =>
      c.method === 'PATCH' ? new Promise<Response>((r) => (answer = () => r(json(full(notes, { ...notes[0], revision: 5, links: refs('notes:3') }))))) : undefined,
    )
    await open('/notes/1')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    await waitFor(() => expect(rows()).toHaveLength(2))
    await userEvent.setup({ advanceTimers: vi.advanceTimersByTime }).click(within(rows()[0]).getByRole('button'))
    expect(within(rows()[1]).getByRole('button')).toBeDisabled()

    Object.assign(notes[0], { revision: 6, links: [] })
    act(() => es.send({ module: 'notes', kind: 'note', id: 1, area: 1 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(rows()).toEqual([])
    await act(async () => answer())
    expect(rows()).toEqual([])
  } finally {
    vi.useRealTimers()
  }
})

test('Add a link finds items through search, leaves out the note itself, shows what is linked already, and links the one picked', async () => {
  narrow(false)
  const notes = [filed(1, { revision: 2, links: refs('notes:2') }), filed(2), note(3, 3, { body: 'short' }), filed(4, { kind: 'idea' })]
  const calls = server(notes, (c) =>
    c.path.startsWith('/api/search?') ? json({ groups: [{ module: 'notes', hits: [hit(notes[0]), hit(notes[1], 'around the word'), hit(notes[2]), hit(notes[3])] }] }) : undefined,
  )
  await open('/notes/1')
  await userEvent.click(await waitFor(() => section().querySelector<HTMLElement>('.sect-head .btn')!))
  await userEvent.type(finder().getByRole('searchbox'), 'no te')
  await waitFor(() => expect(finder().getAllByRole('button')).toHaveLength(4))
  expect(calls.filter((c) => c.path.startsWith('/api/search')).map((c) => c.path)).toEqual(['/api/search?q=no+te'])
  const [linked, short, idea] = finder().getAllByRole('button')
  expect(linked).toBeDisabled()
  expect(short).toBeEnabled()
  // A short note with no title is its own snippet, shown once.
  expect([linked, short].map((b) => b.querySelectorAll('.t3').length)).toEqual([1, 0])
  expect(idea.querySelector('.mk')).toHaveClass('m-idea')

  await userEvent.click(idea)
  expect(writes(calls)).toEqual([['PATCH', '/api/notes/1', { revision: 2, links: ['notes:2', 'notes:4'] }]])
  await waitFor(() => expect(section().querySelector('.finder')).toBeNull())
  expect(rows().map((r) => within(r).getByRole('link').getAttribute('href'))).toEqual(['/notes/2', '/notes/4'])
})

test('the finder shows the answer to what is typed now, though an answer to what was typed before comes after it', async () => {
  narrow(false)
  const notes = [filed(1), filed(2), filed(3)]
  let first: () => void = () => {}
  server(notes, (c) => {
    if (c.path === '/api/search?q=n') return new Promise<Response>((r) => (first = () => r(json({ groups: [{ module: 'notes', hits: [hit(notes[1])] }] }))))
    if (c.path === '/api/search?q=no') return json({ groups: [{ module: 'notes', hits: [hit(notes[2])] }] })
  })
  await open('/notes/1')
  await userEvent.click(await waitFor(() => section().querySelector<HTMLElement>('.sect-head .btn')!))
  await userEvent.type(finder().getByRole('searchbox'), 'n')
  await new Promise((r) => setTimeout(r, 300))
  await userEvent.type(finder().getByRole('searchbox'), 'o')
  await waitFor(() => expect(finder().getAllByRole('button')).toHaveLength(2))
  await act(async () => first())
  const [picked] = finder().getAllByRole('button')
  expect(picked.querySelector('.t1')!.textContent).toBe(notes[2].title)
})

test('a note in the trash shows its links, and neither removes nor adds one', async () => {
  narrow(false)
  server([filed(1, { links: refs('notes:2'), trashed_at: new Date().toISOString() }), filed(2)])
  await open('/notes/1')
  await waitFor(() => expect(rows()).toHaveLength(1))
  expect(within(section()).queryAllByRole('button')).toEqual([])
})

test('on a phone the links are folded, and Add a link opens the finder within them', async () => {
  narrow(true)
  server([filed(1), filed(2)])
  await open('/notes/1')
  const fold = await waitFor(() => screen.getByRole('main').querySelector<HTMLDetailsElement>('details.sect')!)
  expect(fold.open).toBe(false)
  await userEvent.click(fold.querySelector('summary')!)
  await userEvent.click(within(fold).getByRole('button'))
  expect(within(fold).getByRole('searchbox')).toHaveFocus()
})
