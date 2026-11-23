import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { listNotes, note } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, json, narrow, type Call } from '../../test-server.ts'
import type { Note, Row } from './api.ts'
import { capture, notesServer } from './test-notes.ts'

afterEach(() => vi.unstubAllGlobals())

// A filed note of the personal area, changed minutes ago.
const filed = (id: number, minutes: number, n: Partial<Note> = {}) => note(id, minutes, { title: `note ${id}`, body: `body ${id}`, ...n })

// A server that keeps the notes, applies a change made from a note's revision one revision on, refuses one made from
// another or of a note in the trash, and trashes a note, as the server does; answer, when given, answers first.
function server(notes: Note[], answer: (c: Call) => Response | Promise<Response> | undefined = () => undefined) {
  return notesServer(notes, (c) => {
    const a = answer(c)
    if (a || c.method === 'GET') return a
    const n = notes.find((x) => c.path.startsWith(`/api/notes/${x.id}`))!
    if (c.path.endsWith('/trash')) return json(Object.assign(n, { trashed_at: new Date().toISOString() }))
    if (c.path.endsWith('/restore')) return json(Object.assign(n, { trashed_at: null }))
    const { revision, ...change } = c.body as { revision: number }
    if (n.trashed_at || revision !== n.revision) return json({ error: 'conflict' }, 409)
    return json(Object.assign(n, change, { revision: n.revision + 1 }))
  })
}

const main = () => within(screen.getByRole('main'))
const rows = () => [...screen.getByRole('main').querySelectorAll<HTMLAnchorElement>('a.row')]
const open1 = () => rows().find((r) => r.getAttribute('aria-current'))
const note1 = () => within(screen.getByRole('main').querySelector<HTMLElement>('.note-read, .note-edit')!)
const selects = () => main().getAllByRole('combobox') as HTMLSelectElement[]
const lists = (calls: Call[]) => calls.filter((c) => c.path.startsWith('/api/notes?inbox=false')).map((c) => c.path)
const writes = (calls: Call[]) => calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path, c.body])
// Edit and Trash, in the head of a note on desktop and at its foot on a phone.
const tools = () => [...screen.getByRole('main').querySelectorAll<HTMLElement>('.note-read > .page-head .btn, .note-foot .btn')]
// The editor's buttons come last on the note, the save first here: Save and Cancel, or, after a refused save, Save mine
// over it and Give up mine. On desktop the save stands last, at the right, and on a phone first.
const acts = () => {
  const [a, b] = note1().getAllByRole('button').slice(-2)
  return window.matchMedia('(max-width: 720px)').matches ? [a, b] : [b, a]
}

test('the Notes list asks for the filed notes, and opens the newest beside them, or the one picked', async () => {
  narrow(false)
  const calls = server([filed(1, 30), filed(2, 5)])
  const { where } = await open('/notes')
  expect(lists(calls)).toEqual(['/api/notes?inbox=false'])
  expect(rows().map((r) => r.getAttribute('href'))).toEqual(['/notes/2', '/notes/1'])
  expect(open1()).toBe(rows()[0])
  expect(note1().getByRole('heading', { level: 1 })).toHaveTextContent('note 2')

  await userEvent.click(rows()[1])
  await waitFor(() => expect(where()).toBe('/notes/1'))
  expect(open1()).toBe(rows()[1])
  await waitFor(() => expect(note1().getByRole('heading', { level: 1 })).toHaveTextContent('note 1'))
  expect(writes(calls)).toEqual([])
})

test('the dropdowns ask the server for what they choose, and keep it in the address, so a reload asks the same', async () => {
  narrow(false)
  const notes = [filed(1, 30, { area: 'garden', kind: 'idea', tags: ['nas'] }), filed(2, 5)]
  const calls = server(notes)
  const view = await open('/notes')
  const [area, kind, tag] = selects()
  expect([...area.options].map((o) => o.value)).toEqual(['', 'personal', 'garden'])
  expect([...tag.options].map((o) => o.value)).toEqual(['', 'nas'])

  await userEvent.selectOptions(area, 'garden')
  await userEvent.selectOptions(kind, 'idea')
  await userEvent.selectOptions(tag, 'nas')
  // The newest note, open when the filters were chosen, stays open.
  await waitFor(() => expect(view.where()).toBe('/notes/2?area=garden&kind=idea&tag=nas'))
  expect(lists(calls).at(-1)).toBe('/api/notes?inbox=false&area=garden&kind=idea&tag=nas')
  await waitFor(() => expect(rows().map((r) => r.getAttribute('href'))).toEqual(['/notes/1?area=garden&kind=idea&tag=nas']))
  view.unmount()

  const again = server(notes)
  await open('/notes?area=garden&kind=idea&tag=nas')
  expect(lists(again)).toEqual(['/api/notes?inbox=false&area=garden&kind=idea&tag=nas'])
  expect(selects().map((s) => s.value)).toEqual(['garden', 'idea', 'nas'])
})

test('no note under the filters offers to clear them', async () => {
  narrow(false)
  const calls = server([filed(1, 5)])
  const { where } = await open('/notes?kind=idea')
  expect(rows()).toEqual([])
  expect(screen.getByRole('main').querySelector('.box')).toBeNull()
  await userEvent.click(main().getByRole('button'))
  await waitFor(() => expect(where()).toBe('/notes/1'))
  expect(lists(calls).at(-1)).toBe('/api/notes?inbox=false')
  await waitFor(() => expect(rows()).toHaveLength(1))
})

test('a list with more says so and shows 50 more on asking, up to the 500 a list gives', async () => {
  narrow(false)
  const many: Row[] = Array.from({ length: 50 }, (_, i) => filed(i + 1, i + 1))
  const calls = server([], (c) => (c.path.startsWith('/api/notes?') ? json({ notes: many, total: 900 }) : undefined))
  await open('/notes')
  const more = () => screen.getByRole('main').querySelector<HTMLElement>('.list-end')
  for (let i = 0; i < 9; i++) await userEvent.click(within(more()!).getByRole('button'))
  expect(lists(calls)).toEqual(['/api/notes?inbox=false', ...[100, 150, 200, 250, 300, 350, 400, 450, 500].map((n) => `/api/notes?inbox=false&limit=${n}`)])
  expect(more()).not.toBeNull()
  expect(within(more()!).queryByRole('button')).toBeNull()
})

test('on a phone a note picked opens as a page of its own, whose crumb leads back to the list as it was filtered', async () => {
  narrow(true)
  const calls = server([filed(1, 30), filed(2, 5, { kind: 'idea' })])
  const { where } = await open('/notes?kind=idea')
  expect(open1()).toBeUndefined()
  expect(screen.getByRole('main').querySelector('.note-read')).toBeNull()

  await userEvent.click(rows()[0])
  await waitFor(() => expect(where()).toBe('/notes/2?kind=idea'))
  expect(rows()).toEqual([])
  expect(await main().findByRole('heading', { level: 1 })).toHaveTextContent('note 2')
  expect(calls.map((c) => c.path)).toContain('/api/notes/2')

  await userEvent.click(main().getByRole('link'))
  await waitFor(() => expect(where()).toBe('/notes?kind=idea'))
  expect(rows()).toHaveLength(1)
})

test('the body is rendered as Markdown, never as HTML', async () => {
  narrow(false)
  server([filed(1, 5, { body: '# Head\n\nIt is <b>bold</b> and **strong**.' })])
  await open('/notes/1')
  const md = screen.getByRole('main').querySelector('.note-read .md')!
  expect(md.querySelector('h2')).toHaveTextContent('Head')
  expect(md.querySelector('strong')).toHaveTextContent('strong')
  expect(md.querySelector('b')).toBeNull()
  expect(md).toHaveTextContent('<b>bold</b>')
})

test('Save sends the title, the text and the tags with the revision the editing began from, and the note then reads as saved', async () => {
  narrow(false)
  const notes = [filed(1, 5, { revision: 3, tags: ['nas'] })]
  const calls = server(notes)
  await open('/notes/1')
  await userEvent.click(tools()[0])
  const [title, text, tags] = note1().getAllByRole('textbox')
  expect(text).toHaveFocus()
  fireEvent.change(title, { target: { value: 'renamed' } })
  fireEvent.change(text, { target: { value: 'new text' } })
  await userEvent.type(tags, 'backup{Enter}')
  await userEvent.click(acts()[0])
  expect(writes(calls)).toEqual([['PATCH', '/api/notes/1', { revision: 3, title: 'renamed', body: 'new text', tags: ['nas', 'backup'] }]])
  await waitFor(() => expect(note1().getByRole('heading', { level: 1 })).toHaveTextContent('renamed'))
  expect(screen.getByRole('main').querySelector('textarea')).toBeNull()
})

test('the editor moves a note to another area and changes its kind, and Save sends only those', async () => {
  narrow(false)
  const calls = server([filed(1, 5, { revision: 2 })])
  await open('/notes/1')
  await userEvent.click(tools()[0])
  // The areas are the personal area and garden, and the kinds a note and an idea, each in that order.
  const [area, kind] = note1().getAllByRole('radiogroup').map((g) => within(g).getAllByRole('radio'))
  expect(area[0]).toHaveAttribute('aria-checked', 'true')
  await userEvent.click(area[1])
  await userEvent.click(kind[1])
  await userEvent.click(acts()[0])
  expect(writes(calls)).toEqual([['PATCH', '/api/notes/1', { revision: 2, area: 'garden', kind: 'idea' }]])
  await waitFor(() => expect(screen.getByRole('main').querySelector('.note-edit')).toBeNull())
})

test('on a phone the editor chooses the area from a dropdown, and a change of the area alone makes leaving ask first', async () => {
  narrow(true)
  const calls = server([filed(1, 30), capture(2, 5)])
  const { where } = await open('/notes/2')
  await userEvent.click(tools()[0])
  await userEvent.selectOptions(note1().getByRole('combobox'), 'garden')
  await userEvent.click(main().getAllByRole('link')[0])
  expect(screen.getByRole('dialog')).toBeInTheDocument()
  expect(where()).toBe('/notes/2')
  await userEvent.click(within(screen.getByRole('dialog')).getAllByRole('button')[0])
  await userEvent.click(acts()[0])
  // A capture moved to an area is filed by the server.
  expect(writes(calls)).toEqual([['PATCH', '/api/notes/2', { revision: 1, area: 'garden' }]])
})

test('a change made elsewhere shows without a reload, and never replaces a text being edited; its save is refused, keeps what was typed, shows the note as it now is, and saves over it on asking', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    narrow(false)
    const notes = [filed(1, 5, { revision: 3 })]
    const calls = server(notes)
    await open('/notes/1')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const elsewhere = async (n: Partial<Note>) => {
      Object.assign(notes[0], n)
      act(() => es.send({ module: 'notes', kind: 'note', id: 1, area: 1 }))
      await act(() => vi.advanceTimersByTimeAsync(2000))
    }
    await elsewhere({ revision: 4, body: 'from a client' })
    expect(screen.getByRole('main').querySelector('.md')).toHaveTextContent('from a client')

    await user.click(tools()[0])
    const text = () => screen.getByRole('main').querySelector('textarea')!
    fireEvent.change(text(), { target: { value: 'mine' } })
    await elsewhere({ revision: 5, body: 'theirs', source: 'portal' })
    expect(text()).toHaveValue('mine')

    await user.click(acts()[0])
    await waitFor(() => expect(screen.getByRole('main').querySelector('.notice')).not.toBeNull())
    expect(text()).toHaveValue('mine')
    expect(screen.getByRole('main').querySelector('.now')).toBeNull()
    await user.click(within(screen.getByRole('main').querySelector<HTMLElement>('.notice')!).getByRole('button'))
    expect(screen.getByRole('main').querySelector('.now')).toHaveTextContent('theirs')

    await user.click(acts()[0])
    // Only what was changed here is sent.
    expect(writes(calls).map((w) => w[2])).toEqual([
      { revision: 4, body: 'mine' },
      { revision: 5, body: 'mine' },
    ])
    await waitFor(() => expect(screen.getByRole('main').querySelector('.md')).toHaveTextContent('mine'))
  } finally {
    vi.useRealTimers()
  }
})

test('giving up what was typed after a refused save shows the note as it now is', async () => {
  narrow(true)
  const notes = [filed(1, 5)]
  const calls = server(notes, (c) => {
    if (c.method !== 'PATCH') return
    Object.assign(notes[0], { revision: 2, body: 'theirs' })
    return json({ error: 'conflict' }, 409)
  })
  await open('/notes/1')
  await userEvent.click(tools()[0])
  fireEvent.change(screen.getByRole('main').querySelector('textarea')!, { target: { value: 'mine' } })
  await userEvent.click(acts()[0])
  await waitFor(() => expect(screen.getByRole('main').querySelector('.notice')).not.toBeNull())
  // Leaving after the refusal asks too; keeping on editing keeps the refusal as it was.
  await userEvent.click(main().getAllByRole('link')[0])
  await userEvent.click(within(screen.getByRole('dialog')).getAllByRole('button')[0])
  expect(screen.getByRole('main').querySelector('.notice')).not.toBeNull()
  await userEvent.click(acts()[1])
  expect(screen.getByRole('main').querySelector('.md')).toHaveTextContent('theirs')
  expect(writes(calls)).toHaveLength(1)
})

test('with text typed, leaving the note asks first: keeping on editing loses nothing, and giving it up goes on', async () => {
  narrow(false)
  server([filed(1, 30), filed(2, 5)])
  const { where } = await open('/notes/2')
  await userEvent.click(tools()[0])
  fireEvent.change(screen.getByRole('main').querySelector('textarea')!, { target: { value: 'mine' } })
  const unload = new Event('beforeunload', { cancelable: true })
  window.dispatchEvent(unload)
  expect(unload.defaultPrevented).toBe(true)

  const ask = () => within(screen.getByRole('dialog')).getAllByRole('button')
  await userEvent.click(rows()[1])
  expect(ask()).toHaveLength(2)
  expect(where()).toBe('/notes/2')
  expect(screen.getByRole('main').querySelector('textarea')).toHaveValue('mine')
  await userEvent.click(ask()[0])
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(where()).toBe('/notes/2')
  expect(screen.getByRole('main').querySelector('textarea')).toHaveValue('mine')

  await userEvent.click(rows()[1])
  await userEvent.click(ask()[1])
  await waitFor(() => expect(where()).toBe('/notes/1'))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(screen.getByRole('main').querySelector('textarea')).toBeNull()
  const after = new Event('beforeunload', { cancelable: true })
  window.dispatchEvent(after)
  expect(after.defaultPrevented).toBe(false)
})

test('a save that lands while leaving is asked lets the way out go on', async () => {
  narrow(false)
  const notes = [filed(1, 30), filed(2, 5)]
  let answer: () => void = () => {}
  server(notes, (c) => (c.method === 'PATCH' ? new Promise<Response>((r) => (answer = () => r(json(Object.assign(notes[1], { body: 'mine', revision: 2 }))))) : undefined))
  const { where } = await open('/notes/2')
  await userEvent.click(tools()[0])
  fireEvent.change(screen.getByRole('main').querySelector('textarea')!, { target: { value: 'mine' } })
  await userEvent.click(acts()[0])
  await userEvent.click(rows()[1])
  expect(screen.getByRole('dialog')).toBeInTheDocument()
  await act(async () => answer())
  await waitFor(() => expect(where()).toBe('/notes/1'))
  expect(screen.queryByRole('dialog')).toBeNull()
})

test('with nothing typed leaving the note asks nothing, and nor does a change of the filters, which keeps it open', async () => {
  narrow(false)
  server([filed(1, 30), filed(2, 5)])
  const { where } = await open('/notes/2')
  await userEvent.click(tools()[0])
  const unload = new Event('beforeunload', { cancelable: true })
  window.dispatchEvent(unload)
  expect(unload.defaultPrevented).toBe(false)
  await userEvent.click(rows()[1])
  await waitFor(() => expect(where()).toBe('/notes/1'))
  expect(screen.queryByRole('dialog')).toBeNull()

  await userEvent.click(tools()[0])
  fireEvent.change(screen.getByRole('main').querySelector('textarea')!, { target: { value: 'mine' } })
  await userEvent.selectOptions(selects()[1], 'note')
  await waitFor(() => expect(where()).toBe('/notes/1?kind=note'))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(screen.getByRole('main').querySelector('textarea')).toHaveValue('mine')
})

test('a save refused because the note was trashed meanwhile says so, and offers no save over it', async () => {
  narrow(false)
  const notes = [filed(1, 5)]
  server(notes, (c) => {
    if (c.method !== 'PATCH') return
    notes[0].trashed_at = new Date().toISOString()
    return json({ error: 'conflict' }, 409)
  })
  await open('/notes/1')
  await userEvent.click(tools()[0])
  fireEvent.change(screen.getByRole('main').querySelector('textarea')!, { target: { value: 'mine' } })
  await userEvent.click(acts()[0])
  // The page says it in its own line, not in the server's.
  expect(await note1().findByRole('alert')).not.toHaveTextContent('conflict')
  expect(screen.getByRole('main').querySelector('.notice')).toBeNull()
  expect(screen.getByRole('main').querySelector('textarea')).not.toBeNull()
})

test('a tag added or removed where the note is read is a change of the note with its revision', async () => {
  narrow(false)
  const calls = server([filed(1, 5, { revision: 2, tags: ['nas'] })])
  await open('/notes/1')
  const chips = () => [...screen.getByRole('main').querySelectorAll<HTMLElement>('.note-tags button.tag')]
  await userEvent.type(note1().getByRole('textbox'), 'backup{Enter}')
  await waitFor(() => expect(chips()).toHaveLength(2))
  await userEvent.click(chips()[0])
  expect(writes(calls)).toEqual([
    ['PATCH', '/api/notes/1', { revision: 2, tags: ['nas', 'backup'] }],
    ['PATCH', '/api/notes/1', { revision: 3, tags: ['backup'] }],
  ])
})

test('Trash sends the note to the trash, it leaves the list, and the newest opens in its place', async () => {
  narrow(false)
  const calls = server([filed(1, 30), filed(2, 5)])
  await open('/notes')
  await userEvent.click(tools()[1])
  expect(writes(calls)).toEqual([['POST', '/api/notes/2/trash', undefined]])
  await waitFor(() => expect(rows().map((r) => r.getAttribute('href'))).toEqual(['/notes/1']))
  await waitFor(() => expect(note1().getByRole('heading', { level: 1 })).toHaveTextContent('note 1'))
})

test('after Trash the newest of what is left opens, though the list answers only after the note has closed', async () => {
  narrow(false)
  const notes = [filed(1, 30), filed(2, 5)]
  const held: (() => void)[] = []
  let hold = false
  server(notes, (c) => {
    if (!hold || !c.path.startsWith('/api/notes?')) return
    return new Promise<Response>((r) => held.push(() => r(json(listNotes(notes, new URL(c.path, 'http://here').searchParams)))))
  })
  const { where } = await open('/notes')
  await waitFor(() => expect(where()).toBe('/notes/2'))
  hold = true
  await userEvent.click(tools()[1])
  await waitFor(() => expect(where()).toBe('/notes/1'))
  await act(async () => held.forEach((answer) => answer()))
  expect(rows().map((r) => r.getAttribute('href'))).toEqual(['/notes/1'])
  expect(where()).toBe('/notes/1')
})

test('Trash of the note opened unasked, after a click on its own row, still opens the newest of what is left', async () => {
  narrow(false)
  server([filed(1, 30), filed(2, 5)])
  const { where } = await open('/notes')
  await waitFor(() => expect(where()).toBe('/notes/2'))
  await userEvent.click(rows()[0])
  await userEvent.click(tools()[1])
  await waitFor(() => expect(where()).toBe('/notes/1'))
})

test('Trash on a phone goes back to the list as it was filtered', async () => {
  narrow(true)
  const calls = server([filed(1, 30), filed(2, 5, { kind: 'idea' })])
  const { where } = await open('/notes?kind=idea')
  await userEvent.click(rows()[0])
  await waitFor(() => expect(tools()).toHaveLength(2))
  await userEvent.click(tools()[1])
  expect(writes(calls)).toEqual([['POST', '/api/notes/2/trash', undefined]])
  await waitFor(() => expect(where()).toBe('/notes?kind=idea'))
})

test('the newest note, opened unasked on desktop, stays open while it is edited, as a newer one comes and as it leaves the list', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    narrow(false)
    const notes = [filed(1, 30), filed(2, 5)]
    server(notes)
    await open('/notes')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    await userEvent.setup({ advanceTimers: vi.advanceTimersByTime }).click(tools()[0])
    fireEvent.change(screen.getByRole('main').querySelector('textarea')!, { target: { value: 'mine' } })

    notes.push(filed(3, 0))
    act(() => es.send({ module: 'notes', kind: 'note', id: 3, area: 1 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(rows().map((r) => r.getAttribute('href'))).toEqual(['/notes/3', '/notes/2', '/notes/1'])
    expect(open1()).toBe(rows()[1])
    expect(screen.getByRole('main').querySelector('textarea')).toHaveValue('mine')

    notes[1].trashed_at = new Date().toISOString()
    act(() => es.send({ module: 'notes', kind: 'note', id: 2, area: 1 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(rows().map((r) => r.getAttribute('href'))).toEqual(['/notes/3', '/notes/1'])
    expect(screen.getByRole('main').querySelector('textarea')).toHaveValue('mine')
  } finally {
    vi.useRealTimers()
  }
})

test('turning a phone while a note is edited keeps what is typed', async () => {
  const turn = narrow(true)
  server([filed(1, 5)])
  await open('/notes/1')
  await userEvent.click(tools()[0])
  fireEvent.change(screen.getByRole('main').querySelector('textarea')!, { target: { value: 'mine' } })
  act(() => turn(false))
  await waitFor(() => expect(rows()).toHaveLength(1))
  expect(screen.getByRole('main').querySelector('textarea')).toHaveValue('mine')
  act(() => turn(true))
  expect(rows()).toEqual([])
  expect(screen.getByRole('main').querySelector('textarea')).toHaveValue('mine')
})

test('the tags take no change while one is being sent', async () => {
  narrow(false)
  let answer: (r: Response) => void = () => {}
  const notes = [filed(1, 5, { tags: ['nas'] })]
  server(notes, (c) => (c.method === 'PATCH' ? new Promise<Response>((r) => (answer = r)) : undefined))
  await open('/notes/1')
  const chip = () => screen.getByRole('main').querySelector<HTMLButtonElement>('.note-tags button.tag')!
  await userEvent.type(note1().getByRole('textbox'), 'backup{Enter}')
  expect(chip()).toBeDisabled()
  expect(note1().getByRole('textbox')).toHaveAttribute('readonly')
  await act(async () => answer(json(Object.assign(notes[0], { revision: 2, tags: ['backup', 'nas'] }))))
  expect(chip()).toBeEnabled()
  expect(note1().getByRole('textbox')).not.toHaveAttribute('readonly')
})

test('a list that cannot be read says why, and keeps the dropdowns to choose again', async () => {
  narrow(false)
  server([filed(1, 5)], (c) => (c.path.includes('area=gone') ? json({ error: 'no such area' }, 404) : undefined))
  const { where } = await open('/notes?area=gone')
  expect(main().getByRole('alert')).toBeInTheDocument()
  expect(selects()).toHaveLength(3)
  await userEvent.click(within(main().getByRole('alert')).getByRole('button'))
  await waitFor(() => expect(where()).toBe('/notes/1'))
  await waitFor(() => expect(rows()).toHaveLength(1))
  expect(main().queryByRole('alert')).toBeNull()
})

test('a trashed note opened by its address is read only, and Restore there puts it back to be changed', async () => {
  narrow(true)
  const calls = server([filed(1, 5, { tags: ['nas'], trashed_at: new Date().toISOString() })])
  await open('/notes/1')
  expect(await main().findByRole('heading', { level: 1 })).toHaveTextContent('note 1')
  expect(main().queryAllByRole('textbox')).toEqual([])
  expect(tools()).toHaveLength(2)
  await userEvent.click(tools()[0])
  expect(writes(calls)).toEqual([['POST', '/api/notes/1/restore', undefined]])
  await waitFor(() => expect(main().getAllByRole('textbox')).toHaveLength(1))
})

test('the list reads again on a change of a note made elsewhere, and of an area, since a renamed project changes the slug of its notes', async () => {
  vi.useFakeTimers()
  try {
    narrow(false)
    const notes = [filed(1, 5)]
    const calls = server(notes)
    await open('/notes')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    notes.push(filed(2, 0))
    act(() => es.send({ module: 'notes', kind: 'note', id: 2, area: 1 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(rows()).toHaveLength(2)

    const before = lists(calls).length
    act(() => es.send({ module: 'core', kind: 'area', id: 2, area: 2 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(lists(calls)).toHaveLength(before + 1)
  } finally {
    vi.useRealTimers()
  }
})

test('the idea bank asks for the filed ideas, groups them by area, and filters by area and tag alone', async () => {
  narrow(false)
  const calls = server([filed(1, 30, { kind: 'idea' }), filed(2, 20, { kind: 'idea', area: 'garden', tags: ['nas'] }), filed(3, 10), filed(4, 5, { kind: 'idea' })])
  const { where } = await open('/ideas')
  expect(within(screen.getByRole('navigation')).getAllByRole('link').map((a) => a.getAttribute('href'))).toContain('/ideas')
  expect(lists(calls)).toEqual(['/api/notes?inbox=false&kind=idea'])
  const groups = main().getAllByRole('heading', { level: 2 })
  expect(groups.map((g) => within(g.parentElement!).getAllByRole('link').map((a) => a.getAttribute('href')))).toEqual([['/ideas/4', '/ideas/1'], ['/ideas/2']])
  // No idea opens until one is picked.
  expect(open1()).toBeUndefined()
  expect(screen.getByRole('main').querySelector('.note-read')).toBeNull()

  expect(selects()).toHaveLength(2)
  await userEvent.selectOptions(selects()[0], 'garden')
  await waitFor(() => expect(where()).toBe('/ideas?area=garden'))
  expect(lists(calls).at(-1)).toBe('/api/notes?inbox=false&area=garden&kind=idea')
  await waitFor(() => expect(rows()).toHaveLength(1))

  await userEvent.click(rows()[0])
  await waitFor(() => expect(where()).toBe('/ideas/2?area=garden'))
  expect(open1()).toBe(rows()[0])
  expect(await note1().findByRole('heading', { level: 1 })).toHaveTextContent('note 2')
})

test('the idea bank reads its tag from the address, and going there from Notes opens no idea until one is picked', async () => {
  narrow(false)
  const calls = server([filed(1, 30, { kind: 'idea', tags: ['nas'] }), filed(2, 5)])
  const view = await open('/ideas?tag=nas')
  expect(lists(calls)).toEqual(['/api/notes?inbox=false&kind=idea&tag=nas'])
  expect(selects().map((s) => s.value)).toEqual(['', 'nas'])
  view.unmount()

  server([filed(1, 5, { kind: 'idea' })])
  const { where } = await open('/notes')
  expect(open1()).toBe(rows()[0])
  await userEvent.click(within(screen.getByRole('navigation')).getAllByRole('link').find((a) => a.getAttribute('href') === '/ideas')!)
  await waitFor(() => expect(where()).toBe('/ideas'))
  await waitFor(() => expect(rows()).toHaveLength(1))
  expect(open1()).toBeUndefined()
  expect(screen.getByRole('main').querySelector('.note-read')).toBeNull()
})

test('on a phone the idea bank is a row of More, and an idea opens as a page whose crumb leads back to the bank', async () => {
  narrow(true)
  server([filed(1, 30, { kind: 'idea' })])
  const { where } = await open('/more')
  await userEvent.click(main().getAllByRole('link')[0])
  await waitFor(() => expect(where()).toBe('/ideas'))
  await userEvent.click(rows()[0])
  await waitFor(() => expect(where()).toBe('/ideas/1'))
  await userEvent.click(await main().findByRole('link'))
  await waitFor(() => expect(where()).toBe('/ideas'))
})

test('a project lists its notes and its ideas, each leading to the list it was picked from, and Capture here opens Capture over the page with the project chosen', async () => {
  narrow(false)
  const calls = server([filed(1, 30, { area: 'garden' }), filed(2, 20, { area: 'garden', kind: 'idea' }), filed(3, 10)])
  const { where } = await open('/areas/2')
  expect(lists(calls).sort()).toEqual(['/api/notes?inbox=false&area=garden&kind=idea', '/api/notes?inbox=false&area=garden&kind=note'])
  expect(rows().map((r) => r.getAttribute('href'))).toEqual(['/notes/1?area=garden&kind=note', '/ideas/2?area=garden'])

  await userEvent.click(main().getAllByRole('button')[0])
  expect(within(screen.getByRole('dialog')).getByRole('combobox')).toHaveValue('garden')
  expect(where()).toBe('/areas/2')
})

test('on a phone Capture here opens the page of Capture with the project chosen', async () => {
  narrow(true)
  server([])
  const { where } = await open('/areas/2')
  await userEvent.click(within(screen.getByRole('main').querySelector<HTMLElement>('.area-foot')!).getAllByRole('button')[0])
  await waitFor(() => expect(where()).toBe('/capture?area=2'))
  await waitFor(() => expect(main().getByRole('combobox')).toHaveValue('garden'))
})

test('an archived project has no Capture here', async () => {
  narrow(false)
  notesServer([], (c) =>
    c.path.startsWith('/api/areas?') ? json({ areas: [{ id: 2, kind: 'project', name: 'garden', slug: 'garden', description: '', archived: true }] }) : undefined,
  )
  await open('/areas/2')
  expect(screen.getByRole('main').querySelectorAll('.page-head .btn')).toHaveLength(1)
})

test('an area whose notes cannot be read says why', async () => {
  narrow(false)
  server([], (c) => (c.path.startsWith('/api/notes?') ? json({ error: 'down' }, 502) : undefined))
  await open('/areas/2')
  expect(main().getByRole('alert')).toBeInTheDocument()
})

test('the personal area lists its notes and ideas, and has no Capture here', async () => {
  narrow(false)
  const calls = server([filed(1, 30)])
  await open('/areas/1')
  expect(lists(calls)).toHaveLength(2)
  expect(rows().map((r) => r.getAttribute('href'))).toEqual(['/notes/1?area=personal&kind=note'])
  expect(screen.getByRole('main').querySelectorAll('.page-head .btn')).toHaveLength(1)
})
