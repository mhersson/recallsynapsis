import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { open } from '../../test-app.tsx'
import { FakeEventSource, json, narrow, type Call } from '../../test-server.ts'
import { capture, notesServer } from './test-notes.ts'

afterEach(() => vi.unstubAllGlobals())

const rows = () => [...document.querySelectorAll<HTMLElement>('.pick')]
const strips = () => [...document.querySelectorAll<HTMLElement>('.strip')]
const box = () => within(document.querySelector<HTMLElement>('.filing')!)
// The box's buttons end with File and Trash, after the tags.
const file = () => box().getAllByRole('button').at(-2)!
const trash = () => box().getAllByRole('button').at(-1)!
const nav = () => within(screen.getByRole('navigation')).getAllByRole('link')
const count = () => nav().find((a) => a.getAttribute('href') === '/inbox')?.querySelector('.cnt')
const writes = (calls: Call[]) => calls.filter((c) => c.method !== 'GET')

test.each([false, true])('with notes on, a person lands on the Inbox, whose entry shows how many wait (phone: %s)', async (phone) => {
  narrow(phone)
  const calls = notesServer([capture(1, 5), capture(2, 9)])
  const { where } = await open('/')
  expect(where()).toBe('/inbox')
  expect(calls.some((c) => c.path === '/api/notes?inbox=true&limit=1')).toBe(true)
  expect(count()).toHaveTextContent('2')
})

test('without notes the shell has no Inbox, and a person lands on the Projects list', async () => {
  narrow(false)
  const calls = notesServer([capture(1, 5)], undefined, [])
  const { where } = await open('/inbox')
  expect(where()).toBe('/projects')
  expect(nav().map((a) => a.getAttribute('href'))).not.toContain('/inbox')
  expect(calls.some((c) => c.path.startsWith('/api/notes'))).toBe(false)
})

test('the Inbox lists the captures, the newest first and open for filing, and opens another when it is chosen', async () => {
  narrow(false)
  const calls = notesServer([capture(1, 30, { title: 'older' }), capture(2, 5, { said: 'keep this' })])
  await open('/inbox')
  // A row shows the capture's title, else the start of its text.
  expect(rows().map((r) => r.textContent)).toEqual([expect.stringContaining('capture 2'), expect.stringContaining('older')])
  expect(rows()[0]).toHaveAttribute('aria-current', 'true')
  // The box shows the capture as read in full, with what the owner said.
  expect(calls.some((c) => c.path === '/api/notes/2')).toBe(true)
  expect(box().getByText('keep this')).toBeInTheDocument()

  await userEvent.click(rows()[1])
  expect(rows()[1]).toHaveAttribute('aria-current', 'true')
  await waitFor(() => expect(box().getByText('capture 1')).toBeInTheDocument())
  expect(writes(calls)).toEqual([])
})

test('filing sends the area, the kind and the tags with the revision it was read at, and the capture leaves the list', async () => {
  narrow(false)
  const notes = [capture(1, 30), capture(2, 5, { revision: 3, tags: ['nas'] })]
  const calls = notesServer(notes, (c) => {
    if (c.method !== 'PATCH') return
    Object.assign(notes[1], { inbox: false, revision: 4 })
    return json(notes[1])
  })
  await open('/inbox')
  const [area, kind] = box().getAllByRole('radiogroup')
  await userEvent.click(within(area).getAllByRole('radio')[1])
  await userEvent.click(within(kind).getAllByRole('radio')[1])
  // A tag being typed is taken as it is when File is pressed.
  await userEvent.type(box().getByRole('textbox'), '#Backup')
  await userEvent.click(file())

  const patch = writes(calls)[0]
  expect([patch.method, patch.path, patch.body]).toEqual(['PATCH', '/api/notes/2', { revision: 3, area: 'garden', kind: 'idea', tags: ['nas', 'backup'], inbox: false }])
  await waitFor(() => expect(rows()).toHaveLength(1))
  expect(rows()[0]).toHaveAttribute('aria-current', 'true')
})

test('a filing the server refuses reads the capture again and shows it as it now is, and filing it again names that revision, what was chosen and what changed', async () => {
  narrow(false)
  const notes = [capture(1, 5)]
  let refuse = true
  const calls = notesServer(notes, (c) => {
    if (c.method !== 'PATCH') return
    if (refuse) {
      refuse = false
      Object.assign(notes[0], { revision: 2, body: 'capture 1, and more', tags: ['nas'] })
      return json({ error: 'moved on' }, 409)
    }
    notes[0].inbox = false
    return json(notes[0])
  })
  await open('/inbox')
  await userEvent.click(within(box().getAllByRole('radiogroup')[0]).getAllByRole('radio')[1])
  await userEvent.click(file())
  await waitFor(() => expect(box().getByText('capture 1, and more')).toBeInTheDocument())
  expect(box().getByRole('alert')).toBeInTheDocument()

  await userEvent.click(file())
  const sent = writes(calls).map((c) => c.body as { revision: number; area: string; tags: string[] })
  expect(sent.map((b) => [b.revision, b.area, b.tags])).toEqual([
    [1, 'garden', []],
    [2, 'garden', ['nas']],
  ])
  await waitFor(() => expect(rows()).toHaveLength(0))
})

test('Trash sends the capture to the trash, and it leaves the list', async () => {
  narrow(false)
  const notes = [capture(1, 30), capture(2, 5)]
  const calls = notesServer(notes, (c) => {
    if (c.method !== 'POST') return
    notes[1].trashed_at = new Date().toISOString()
    return json(notes[1])
  })
  await open('/inbox')
  await userEvent.click(trash())
  expect(writes(calls).map((c) => [c.method, c.path])).toEqual([['POST', '/api/notes/2/trash']])
  await waitFor(() => expect(rows()).toHaveLength(1))
})

test('a capture made elsewhere appears in an open Inbox, the count follows, and the capture open for filing stays open', async () => {
  vi.useFakeTimers()
  try {
    narrow(false)
    const notes = [capture(1, 30)]
    notesServer(notes)
    await open('/inbox')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    expect(rows()).toHaveLength(1)
    expect(count()).toHaveTextContent('1')

    notes.push(capture(2, 0))
    act(() => es.send({ module: 'notes', kind: 'note', id: 2, area: 1 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(rows()).toHaveLength(2)
    expect(rows()[1]).toHaveAttribute('aria-current', 'true')
    expect(count()).toHaveTextContent('2')
  } finally {
    vi.useRealTimers()
  }
})

test('on a phone each capture is a strip, File on one opens it for filing in place, and its area is a select', async () => {
  narrow(true)
  const notes = [capture(1, 30), capture(2, 5)]
  const calls = notesServer(notes, (c) => (c.method === 'PATCH' ? json(notes[0]) : undefined))
  await open('/inbox')
  expect(strips()).toHaveLength(2)
  expect(strips()[0].querySelector('.filing')).not.toBeNull()

  await userEvent.click(within(strips()[1]).getByRole('button'))
  await waitFor(() => expect(strips()[1].querySelector('.filing')).not.toBeNull())
  expect(strips()[0].querySelector('.filing')).toBeNull()
  await userEvent.selectOptions(box().getByRole('combobox'), 'garden')
  await userEvent.click(file())
  expect(writes(calls).map((c) => [c.path, c.body])).toEqual([['/api/notes/1', { revision: 1, area: 'garden', kind: 'note', tags: [], inbox: false }]])
})

test('an empty inbox has nothing to list and nothing to file', async () => {
  narrow(false)
  notesServer([])
  await open('/inbox')
  expect(screen.getByRole('main').querySelector('.empty')).not.toBeNull()
  expect(rows()).toEqual([])
  expect(document.querySelector('.filing')).toBeNull()
  expect(count()).toBeEmptyDOMElement()
})
