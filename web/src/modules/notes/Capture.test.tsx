import { act, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { open } from '../../test-app.tsx'
import { FakeEventSource, json, narrow, type Call } from '../../test-server.ts'
import type { Note } from './api.ts'
import { capture, notesServer } from './test-notes.ts'

afterEach(() => vi.unstubAllGlobals())

const nav = () => within(screen.getByRole('navigation'))
const links = () => nav().getAllByRole('link').map((a) => a.getAttribute('href'))
const dialog = () => within(screen.getByRole('dialog'))
const posts = (calls: Call[]) => calls.filter((c) => c.method === 'POST').map((c) => [c.path, c.body])

// A server that keeps what is captured as a note in the inbox.
function server() {
  const notes: Note[] = []
  return notesServer(notes, (c) => {
    if (c.method !== 'POST') return
    const n = capture(notes.length + 1, 0, { body: (c.body as { body: string }).body })
    notes.push(n)
    return json(n, 201)
  })
}

test.each([
  [['notes'], 1, ['/inbox', '/capture', '/projects', '/more']],
  [[], 0, ['/projects', '/more']],
])('with the modules %j on, the side bar has %i action button, and a phone has the tabs %j', async (modules, buttons, tabs) => {
  notesServer([], undefined, modules)
  narrow(false)
  const desktop = await open('/')
  expect(nav().queryAllByRole('button')).toHaveLength(buttons)
  desktop.unmount()
  narrow(true)
  await open('/')
  expect(links()).toEqual(tabs)
})

test('Capture sends the text and the kind to the inbox, and the capture then shows in an open Inbox', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    narrow(false)
    const calls = server()
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    await open('/inbox')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    expect(document.querySelectorAll('.pick')).toHaveLength(0)

    await user.click(nav().getByRole('button'))
    await user.type(dialog().getByRole('textbox'), '  Ring the garage door company  ')
    await user.click(dialog().getAllByRole('radio')[1])
    await user.click(dialog().getAllByRole('button').at(-1)!)
    expect(posts(calls)).toEqual([['/api/notes', { body: 'Ring the garage door company', kind: 'idea', inbox: true }]])
    expect(screen.queryByRole('dialog')).toBeNull()

    // The server tells the stream of the capture, and the Inbox reads again.
    act(() => es.send({ module: 'notes', kind: 'note', id: 1, area: 1 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    expect(document.querySelectorAll('.pick')).toHaveLength(1)
  } finally {
    vi.useRealTimers()
  }
})

test.each([
  ['/areas/2', { area: 'garden' }],
  ['/areas/1', { inbox: true }],
])('Capture opened on %s sends the capture to %j', async (page, to) => {
  narrow(false)
  const calls = server()
  await open(page)
  await userEvent.click(nav().getByRole('button'))
  // The Inbox, then the areas once they are read.
  await waitFor(() => expect(dialog().getAllByRole('option')).toHaveLength(3))
  expect(dialog().getByRole('combobox')).toHaveValue('area' in to ? to.area : '')
  await userEvent.type(dialog().getByRole('textbox'), 'Beds')
  await userEvent.click(dialog().getAllByRole('button').at(-1)!)
  expect(posts(calls)).toEqual([['/api/notes', { body: 'Beds', kind: 'note', ...to }]])
})

test('Capture waits for a text, and Cancel and Escape close it without a request', async () => {
  narrow(false)
  const calls = server()
  await open('/inbox')
  await userEvent.click(nav().getByRole('button'))
  const [cancel, save] = dialog().getAllByRole('button')
  expect(save).toBeDisabled()
  await userEvent.type(dialog().getByRole('textbox'), ' ')
  expect(save).toBeDisabled()
  await userEvent.click(cancel)
  expect(screen.queryByRole('dialog')).toBeNull()

  await userEvent.click(nav().getByRole('button'))
  await userEvent.keyboard('{Escape}')
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(posts(calls)).toEqual([])
})

test('a capture the server refuses keeps the dialog open with what was typed, says why, and can be sent again', async () => {
  narrow(false)
  const calls = notesServer([], (c) => (c.method === 'POST' ? json({ error: 'refused' }, 400) : undefined))
  await open('/inbox')
  await userEvent.click(nav().getByRole('button'))
  await userEvent.type(dialog().getByRole('textbox'), 'Beds')
  await userEvent.click(dialog().getAllByRole('button').at(-1)!)
  expect(dialog().getByRole('alert')).toBeInTheDocument()
  expect(dialog().getByRole('textbox')).toHaveValue('Beds')
  await userEvent.click(dialog().getAllByRole('button').at(-1)!)
  expect(posts(calls)).toHaveLength(2)
})

test('on a phone Capture is a page, opened from a project with the project chosen, which goes back once it is sent', async () => {
  narrow(true)
  const calls = server()
  const { where } = await open('/areas/2')
  await userEvent.click(nav().getAllByRole('link').find((a) => a.getAttribute('href') === '/capture?area=2')!)
  expect(screen.queryByRole('dialog')).toBeNull()
  const main = within(screen.getByRole('main'))
  await waitFor(() => expect(main.getByRole('combobox')).toHaveValue('garden'))
  await userEvent.type(main.getByRole('textbox'), 'Beds')
  await userEvent.selectOptions(main.getByRole('combobox'), '')
  await userEvent.click(main.getByRole('button'))
  expect(posts(calls)).toEqual([['/api/notes', { body: 'Beds', kind: 'note', inbox: true }]])
  await waitFor(() => expect(where()).toBe('/areas/2'))
})
