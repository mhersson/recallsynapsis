import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { full, history, listed, note, refs, version } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, json, narrow, type Call } from '../../test-server.ts'
import type { Note, Version } from './api.ts'
import { notesServer } from './test-notes.ts'

afterEach(() => vi.unstubAllGlobals())

// A note at revision 3, whose first revision a client captured with what the owner said, and whose second had another text.
const three = () =>
  note(1, 5, { title: 'now', body: 'the text now', revision: 3, said: 'keep this', tags: ['a', 'b'], links: refs('notes:2') })
const earlier = (n: Note): Version[] => history(n).map((v) => (v.revision === 2 ? { ...v, body: 'the text then', tags: ['a'], links: [], source: 'aim' } : v))

// A server that keeps the note's revisions as the server does, and restores one as a new revision that names it;
// answer, when given, answers first.
function server(n: Note, answer: (c: Call) => Response | Promise<Response> | undefined = () => undefined) {
  const notes = [n, note(2, 9, { title: 'other', body: 'x' })]
  const kept = earlier(n)
  const calls = notesServer(notes, (c) => {
    const a = answer(c)
    if (a) return a
    if (c.path === '/api/notes/1/revisions') return json(listed(kept))
    const [, r, restore] = /^\/api\/notes\/1\/revisions\/(\d+)(\/restore)?$/.exec(c.path) ?? []
    const v = kept.find((x) => x.revision === Number(r))
    if (!v) return
    if (!restore) return json(v)
    Object.assign(n, { title: v.title, body: v.body, tags: v.tags, links: refs(...v.links), revision: n.revision + 1 })
    kept.push({ ...version(n, n.revision), restored_from: v.revision })
    return json(full(notes, n))
  })
  return { calls, notes }
}

const histories = (calls: Call[]) => calls.filter((c) => c.path === '/api/notes/1/revisions').length
const section = () => [...screen.getByRole('main').querySelectorAll<HTMLElement>('.sect')].at(-1)!
const rows = () => [...section().querySelectorAll<HTMLElement>('.hrow')]
const main = () => screen.getByRole('main')

test('the history lists the revisions the newest first, marks the current, says what the owner said, and reads any other', async () => {
  narrow(false)
  server(three())
  await open('/notes/1')
  await waitFor(() => expect(rows()).toHaveLength(3))
  expect(rows().map((r) => within(r).queryAllByRole('button').length)).toEqual([0, 1, 1])
  expect(rows()[0].querySelector('.tag')).not.toBeNull()
  expect(rows().map((r) => !!r.querySelector('.said'))).toEqual([false, false, true])
})

test('Read opens a revision in a frame apart from the current text, and Back returns to the note as it is', async () => {
  narrow(false)
  const { calls } = server(three())
  await open('/notes/1')
  await userEvent.click(await waitFor(() => within(rows()[1]).getByRole('button')))
  expect(calls.map((c) => c.path)).toContain('/api/notes/1/revisions/2')
  const frame = await waitFor(() => main().querySelector<HTMLElement>('.rev')!)
  expect(frame.querySelector('.md')).toHaveTextContent('the text then')
  expect(main().querySelector('.note-read')).toBeNull()

  const [, back] = within(main().querySelector<HTMLElement>('.revision')!).getAllByRole('button')
  await userEvent.click(back)
  expect(main().querySelector('.rev')).toBeNull()
  expect(main().querySelector('.note-read .md')).toHaveTextContent('the text now')
  expect(calls.filter((c) => c.method !== 'GET')).toEqual([])
})

test('restoring a revision asks the server for it, the note then reads as it did, and the history shows the restore', async () => {
  narrow(false)
  const { calls } = server(three())
  await open('/notes/1')
  await userEvent.click(await waitFor(() => within(rows()[1]).getByRole('button')))
  const before = histories(calls)
  await userEvent.click(await waitFor(() => within(main().querySelector<HTMLElement>('.revision')!).getAllByRole('button')[0]))
  expect(calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path])).toEqual([['POST', '/api/notes/1/revisions/2/restore']])
  await waitFor(() => expect(main().querySelector('.note-read .md')).toHaveTextContent('the text then'))
  expect(main().querySelector('.notice.restored')).not.toBeNull()
  await waitFor(() => expect(rows()).toHaveLength(4))
  expect(histories(calls)).toBe(before + 1)
  expect(rows()[0]).toHaveClass('fresh')
  expect(rows()[0].querySelector('.tag')).not.toBeNull()
})

test('a restore the server refuses shows its line and keeps the revision open', async () => {
  narrow(true)
  server(three(), (c) => (c.method === 'POST' ? json({ error: 'the note is in the trash' }, 409) : undefined))
  await open('/notes/1')
  await userEvent.click(await waitFor(() => section().querySelector('summary')!))
  await userEvent.click(await waitFor(() => within(rows()[1]).getByRole('button')))
  const revision = await waitFor(() => main().querySelector<HTMLElement>('.revision')!)
  await userEvent.click(within(revision).getAllByRole('button')[0])
  expect(await within(revision).findByRole('alert')).toBeInTheDocument()
  expect(main().querySelector('.rev')).not.toBeNull()
})

test('the history is read again when the note moves on elsewhere, and only then', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    narrow(false)
    const n = three()
    const { calls } = server(n)
    await open('/notes/1')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    await waitFor(() => expect(rows()).toHaveLength(3))
    const before = histories(calls)
    const told = async () => {
      act(() => es.send({ module: 'notes', kind: 'note', id: 1, area: 1 }))
      await act(() => vi.advanceTimersByTimeAsync(2000))
    }
    // A file attached elsewhere writes no revision.
    await told()
    expect(histories(calls)).toBe(before)
    n.revision = 4
    await told()
    expect(histories(calls)).toBe(before + 1)
  } finally {
    vi.useRealTimers()
  }
})

test('a note in the trash has its revisions read, and none restored', async () => {
  narrow(false)
  server({ ...three(), trashed_at: new Date().toISOString() })
  await open('/notes/1')
  await userEvent.click(await waitFor(() => within(rows()[1]).getByRole('button')))
  const revision = await waitFor(() => main().querySelector<HTMLElement>('.revision')!)
  await waitFor(() => expect(revision.querySelector('.rev')).not.toBeNull())
  expect(within(revision).getAllByRole('button')).toHaveLength(1)
})
