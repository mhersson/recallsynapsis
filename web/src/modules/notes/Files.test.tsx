import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { full, note } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, json, narrow, type Call } from '../../test-server.ts'
import type { Attached, Note } from './api.ts'
import { notesServer } from './test-notes.ts'

afterEach(() => vi.unstubAllGlobals())

const filed = (id: number, n: Partial<Note> = {}) => note(id, id, { title: `note ${id}`, body: `body ${id}`, ...n })
const png: Attached = { id: 7, name: 'board.png', type: 'image/png', size: 2048 }
const svg: Attached = { id: 8, name: 'logo.svg', type: 'image/svg+xml', size: 300 }
const pdf: Attached = { id: 9, name: 'plan.pdf', type: 'application/pdf', size: 86_016 }

// A server that keeps the notes, attaches and removes files as the server does, and answers an upload with answer when
// it gives one, else at once.
function server(notes: Note[], answer: (c: Call) => Response | Promise<Response> | undefined = () => undefined) {
  return notesServer(notes, (c) => {
    const a = answer(c)
    if (a) return a
    const n = notes.find((x) => c.path.startsWith(`/api/notes/${x.id}/files`))
    if (!n) return
    if (c.method === 'POST') {
      const f = c.body as File
      const at = { id: 100 + n.files.length, name: new URL(c.path, 'http://here').searchParams.get('name')!, type: f.type, size: f.size }
      n.files.push(at)
      return json(at, 201)
    }
    if (c.method === 'DELETE') {
      n.files = n.files.filter((f) => c.path !== `/api/notes/${n.id}/files/${f.id}`)
      return new Response(null, { status: 204 })
    }
  })
}

const sections = () => [...screen.getByRole('main').querySelectorAll<HTMLElement>('.sect')]
const files = () => sections().find((s) => s.querySelector('.tiles'))!
const tiles = () => [...files().querySelectorAll<HTMLElement>('.tile')]
const picker = () => files().querySelector<HTMLInputElement>('input[type=file]:not([capture])')!
const uploads = (calls: Call[]) => calls.filter((c) => c.method === 'POST' && c.path.includes('/files'))
const reads = (calls: Call[], id: number) => calls.filter((c) => c.method === 'GET' && c.path === `/api/notes/${id}`).length
const file = (name: string, bytes: number, type = 'text/plain') => new File([new Uint8Array(bytes)], name, { type })

test('a file of one of the four image types shows as an image, and any other, SVG among them, as a download of its address', async () => {
  narrow(false)
  server([filed(1, { files: [png, svg, pdf] })])
  await open('/notes/1')
  await waitFor(() => expect(tiles()).toHaveLength(3))
  expect([...files().querySelectorAll('img')].map((i) => i.getAttribute('src'))).toEqual(['/api/notes/1/files/7'])
  const downloads = [...files().querySelectorAll('a[download]')].map((a) => a.getAttribute('href'))
  expect(downloads).toEqual(['/api/notes/1/files/8', '/api/notes/1/files/9'])
})

test('a file chosen is sent as the body with its type, the picker is emptied so it can be chosen again, and the note reads again', async () => {
  narrow(false)
  const calls = server([filed(1)])
  await open('/notes/1')
  await waitFor(() => expect(files()).toBeDefined())
  const before = reads(calls, 1)
  const chosen = file('a b.txt', 5)
  await userEvent.upload(picker(), chosen)
  const [sent] = uploads(calls)
  expect(sent.path).toBe('/api/notes/1/files?name=a+b.txt')
  expect(sent.body).toBe(chosen)
  expect(sent.headers.get('Content-Type')).toBe('text/plain')
  expect(picker().value).toBe('')
  await waitFor(() => expect(tiles()).toHaveLength(1))
  expect(reads(calls, 1)).toBe(before + 1)
})

test('a file over the cap is refused before anything is sent, and a file the server refuses shows its line', async () => {
  narrow(false)
  const calls = server([filed(1)], (c) => {
    if (c.path === '/api/notes/limits') return json({ file_max_bytes: 10 })
    if (c.method === 'POST') return json({ error: 'the upload ended before the file did' }, 400)
  })
  await open('/notes/1')
  await waitFor(() => expect(files().querySelector('.note')).not.toBeNull())
  await userEvent.upload(picker(), file('big.bin', 11))
  expect(within(files()).getByRole('alert')).toBeInTheDocument()
  expect(uploads(calls)).toEqual([])

  await userEvent.upload(picker(), file('small.bin', 10))
  expect(uploads(calls)).toHaveLength(1)
  await waitFor(() => expect(within(files()).getByRole('alert')).toBeInTheDocument())
  expect(tiles()).toEqual([])
})

test('an upload under way shows as a tile and is counted, and leaving the note ends it', async () => {
  narrow(false)
  const calls = server([filed(1), filed(2)], (c) => (c.method === 'POST' ? new Promise<Response>(() => {}) : undefined))
  const { where } = await open('/notes/1')
  await waitFor(() => expect(files()).toBeDefined())
  expect(files().querySelector('h2')).not.toHaveTextContent(/\d/)
  await userEvent.upload(picker(), [file('a.txt', 3), file('b.txt', 4)])
  expect(files().querySelectorAll('.tile.up')).toHaveLength(2)
  expect(files().querySelector('.count')).toHaveTextContent('2')
  expect(files().querySelector('.fline')).not.toBeNull()
  const sent = uploads(calls)
  expect(sent.map((c) => c.signal!.aborted)).toEqual([false, false])

  await userEvent.click(screen.getByRole('main').querySelector<HTMLElement>('a.row[href="/notes/2"]')!)
  await waitFor(() => expect(where()).toBe('/notes/2'))
  expect(sent.map((c) => c.signal!.aborted)).toEqual([true, true])
})

test('a read of the note asked before a file was attached, and answered after one asked since, never hides the file', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  try {
    narrow(false)
    const notes = [filed(1)]
    let hold = false
    let late: () => void = () => {}
    server(notes, (c) => {
      if (!hold || c.method !== 'GET' || c.path !== '/api/notes/1') return
      hold = false
      const then = { ...full(notes, notes[0]), files: [...notes[0].files] }
      return new Promise<Response>((r) => (late = () => r(json(then))))
    })
    await open('/notes/1')
    const es = FakeEventSource.all[0]
    await act(async () => es.open())
    hold = true
    act(() => es.send({ module: 'notes', kind: 'note', id: 1, area: 1 }))
    await act(() => vi.advanceTimersByTimeAsync(2000))
    await userEvent.setup({ advanceTimers: vi.advanceTimersByTime }).upload(picker(), file('a.txt', 3))
    await waitFor(() => expect(tiles()).toHaveLength(1))
    await act(async () => late())
    expect(tiles()).toHaveLength(1)
  } finally {
    vi.useRealTimers()
  }
})

test('an upload goes on while the note is edited, and its end never replaces what is typed', async () => {
  narrow(false)
  let answer: () => void = () => {}
  const notes = [filed(1)]
  const calls = server(notes, (c) => {
    if (c.method !== 'POST') return
    return new Promise<Response>((r) => (answer = () => r(json(notes[0].files[notes[0].files.push(png) - 1], 201))))
  })
  await open('/notes/1')
  await waitFor(() => expect(files()).toBeDefined())
  await userEvent.upload(picker(), file('board.png', 3, 'image/png'))
  await userEvent.click(screen.getByRole('main').querySelector<HTMLElement>('.note-read > .page-head .btn')!)
  const text = () => screen.getByRole('main').querySelector('textarea')!
  fireEvent.change(text(), { target: { value: 'mine' } })
  const before = reads(calls, 1)
  await act(async () => answer())
  await waitFor(() => expect(reads(calls, 1)).toBe(before + 1))
  expect(uploads(calls)[0].signal!.aborted).toBe(false)
  expect(text()).toHaveValue('mine')
})

test('on desktop a file dropped on the note and an image pasted are attached, and a paste of no image is the page’s own', async () => {
  narrow(false)
  const calls = server([filed(1)])
  await open('/notes/1')
  const read = await waitFor(() => screen.getByRole('main').querySelector<HTMLElement>('.note-read')!)
  const dropped = file('notes.md', 4)
  fireEvent.dragOver(read, { dataTransfer: { types: ['Files'], files: [dropped] } })
  fireEvent.drop(read, { dataTransfer: { types: ['Files'], files: [dropped] } })
  const pasted = file('image.png', 4, 'image/png')
  fireEvent.paste(document, { clipboardData: { files: [pasted], types: ['Files'] } })
  const text = fireEvent.paste(document, { clipboardData: { files: [], types: ['text/plain'] } })
  // Cells copied from a spreadsheet, pasted in a field, are the text they hold.
  const cells = fireEvent.paste(read.querySelector('input')!, { clipboardData: { files: [file('cells.png', 4, 'image/png')], types: ['text/plain', 'Files'] } })
  expect(uploads(calls).map((c) => c.body)).toEqual([dropped, pasted])
  expect([text, cells]).toEqual([true, true])

  // An image pasted into a dialog over the note, such as Capture, is the dialog's.
  await userEvent.click(screen.getByRole('navigation').querySelector<HTMLElement>('.side-act')!)
  fireEvent.paste(within(screen.getByRole('dialog')).getByRole('textbox'), { clipboardData: { files: [file('shot.png', 4, 'image/png')], types: ['Files'] } })
  expect(uploads(calls)).toHaveLength(2)
})

test('on a phone Take a photo opens the camera at once, and Choose a file the phone’s own picker', async () => {
  narrow(true)
  server([filed(1)])
  await open('/notes/1')
  const clicked = vi.spyOn(HTMLInputElement.prototype, 'click').mockImplementation(function (this: HTMLInputElement) {})
  try {
    const [camera, chooser] = await waitFor(() => within(files()).getAllByRole('button'))
    await userEvent.click(camera)
    await userEvent.click(chooser)
    expect(clicked.mock.contexts.map((i) => [(i as HTMLInputElement).accept, (i as HTMLInputElement).getAttribute('capture')])).toEqual([
      ['image/*', 'environment'],
      ['', null],
    ])
  } finally {
    clicked.mockRestore()
  }
})

test('removing a file asks first, every time: keeping it sends nothing, and Remove removes it', async () => {
  narrow(false)
  const calls = server([filed(1, { files: [png, pdf] })])
  await open('/notes/1')
  const remove = async (i: number) => userEvent.click(within(await waitFor(() => tiles()[i])).getAllByRole('button').at(-1)!)
  await remove(1)
  await userEvent.click(within(screen.getByRole('dialog')).getAllByRole('button')[0])
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(calls.filter((c) => c.method === 'DELETE')).toEqual([])

  await remove(1)
  await userEvent.click(within(screen.getByRole('dialog')).getAllByRole('button')[1])
  expect(calls.filter((c) => c.method === 'DELETE').map((c) => c.path)).toEqual(['/api/notes/1/files/9'])
  await waitFor(() => expect(tiles()).toHaveLength(1))
})

test('an image opened fills the view with a way to download it, and closes', async () => {
  narrow(false)
  server([filed(1, { files: [png] })])
  await open('/notes/1')
  await userEvent.click(await waitFor(() => within(tiles()[0]).getAllByRole('button')[0]))
  const viewer = within(screen.getByRole('dialog'))
  expect(screen.getByRole('dialog').querySelector('img')).toHaveAttribute('src', '/api/notes/1/files/7')
  expect(viewer.getByRole('link')).toHaveAttribute('href', '/api/notes/1/files/7')
  expect(viewer.getByRole('link')).toHaveAttribute('download')
  await userEvent.click(viewer.getByRole('button'))
  expect(screen.queryByRole('dialog')).toBeNull()
})

test('a note in the trash shows its files, attaches and removes none, and takes no pasted image', async () => {
  narrow(false)
  const calls = server([filed(1, { files: [png, pdf], trashed_at: new Date().toISOString() })])
  await open('/notes/1')
  await waitFor(() => expect(tiles()).toHaveLength(2))
  expect(tiles().map((t) => within(t).queryAllByRole('button').length)).toEqual([1, 0])
  expect(files().querySelector('input')).toBeNull()
  fireEvent.paste(document, { clipboardData: { files: [file('image.png', 4, 'image/png')] } })
  expect(uploads(calls)).toEqual([])
})
