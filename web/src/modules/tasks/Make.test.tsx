import { act, fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { answerTasks, full, note } from '../../dev/fixtures.ts'
import { open } from '../../test-app.tsx'
import { json, narrow, type Call } from '../../test-server.ts'
import type { Note } from '../notes/api.ts'
import { notesServer } from '../notes/test-notes.ts'
import type { Task } from './api.ts'
import { monday } from './test-tasks.ts'

afterEach(() => vi.unstubAllGlobals())

const main = () => screen.getByRole('main')
const dialog = () => within(screen.getByRole('dialog'))
// Edit, the makers and Trash, in the head of a note on desktop and at its foot on a phone.
const tools = () => [...main().querySelectorAll<HTMLElement>('.note-read > .page-head .btn, .note-foot .btn')]
// The title and the notes of the form of Make a task, in its dialog or on its page.
const titleField = () => document.querySelector<HTMLInputElement>('.task-make input.title')!
const notesField = () => document.querySelector<HTMLTextAreaElement>('.task-make textarea')!
const writes = (calls: Call[]) => calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path, c.body])
const linkedFrom = () => [...main().querySelectorAll<HTMLElement>('.sect .lrow')].filter((r) => !r.querySelector('button'))
const put = (input: HTMLElement, value: string) => fireEvent.change(input, { target: { value } })

// A server with the modules given on that keeps the notes and the tasks, resolves the links of each among both, trashes a
// note, and answers the tasks routes as the server does, on a Monday; answer, when given, answers first.
function server(notes: Note[], tasks: Task[] = [], answer: (c: Call) => Response | undefined = () => undefined, modules = ['notes', 'tasks']) {
  return notesServer(
    notes,
    (c) => {
      const a = answer(c)
      if (a) return a
      const url = new URL(c.path, 'http://here')
      if (url.pathname.startsWith('/api/tasks')) {
        const [status, body] = answerTasks(tasks, c.method, url, (c.body ?? {}) as Record<string, unknown>, monday, () => {}, notes)
        return status === 204 ? new Response(null, { status }) : json(body, status)
      }
      const n = notes.find((x) => url.pathname.startsWith(`/api/notes/${x.id}`))
      if (n && c.method === 'POST' && url.pathname.endsWith('/trash')) return json(Object.assign(n, { trashed_at: new Date().toISOString() }))
      if (n && c.method === 'GET' && url.pathname === `/api/notes/${n.id}`) return json(full(notes, n, tasks))
    },
    modules,
  )
}

const shelves = () => note(7, 30, { area: 'garden', title: 'Shelves', body: 'Left wall 240 cm.' })

test.each([
  [['notes'], false, 2],
  [['notes', 'tasks'], false, 3],
  [['notes'], true, 2],
  [['notes', 'tasks'], true, 3],
])('with %j on and a phone %s, a note’s page offers %i tools', async (modules, phone, n) => {
  narrow(phone)
  server([shelves()], [], undefined, modules)
  await open('/notes/7')
  await waitFor(() => expect(tools()).toHaveLength(n))
})

test('Make a task opens a dialog with the note’s title, its area and the link back, and Save sends what was chosen; the note stays and lists the task with its ring', async () => {
  narrow(false)
  const tasks: Task[] = []
  const calls = server([shelves()], tasks)
  const { where } = await open('/notes/7')
  await userEvent.click(await waitFor(() => tools()[1]))
  expect(titleField()).toHaveValue('Shelves')
  expect(dialog().getByRole('combobox')).toHaveValue('garden')
  // The shortcuts come once the server's today is read.
  const weekend = await waitFor(() => [...screen.getByRole('dialog').querySelectorAll<HTMLElement>('.sc')][2])
  await userEvent.type(titleField(), ' up')
  await userEvent.click(weekend)
  put(screen.getByRole('dialog').querySelectorAll<HTMLInputElement>('input[type=date]')[2], '2026-12-04')
  await userEvent.click(dialog().getAllByRole('radio')[1])
  await userEvent.type(notesField(), 'brackets first')
  await userEvent.selectOptions(dialog().getByRole('combobox'), 'personal')
  expect(writes(calls)).toEqual([])

  await userEvent.click(dialog().getAllByRole('button').at(-1)!)
  expect(writes(calls)).toEqual([
    [
      'POST',
      '/api/tasks',
      {
        tasks: [
          {
            title: 'Shelves up',
            notes: 'brackets first',
            area: 'personal',
            planned_from: '2026-11-28',
            planned_to: '2026-11-29',
            deadline: '2026-12-04',
            list: 'soon',
            links: ['notes:7'],
          },
        ],
      },
    ],
  ])
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  expect(where()).toBe('/notes/7')
  await waitFor(() => expect(linkedFrom()).toHaveLength(1))
  expect(within(linkedFrom()[0]).getByRole('link')).toHaveAttribute('href', `/tasks/${tasks[0].id}`)
  expect(linkedFrom()[0].querySelector('.mk')).toHaveClass('m-task')
  // The notice leads to the task too.
  expect(within(main().querySelector<HTMLElement>('.notice')!).getByRole('link')).toHaveAttribute('href', `/tasks/${tasks[0].id}`)
})

test('a note with no title starts the task with the first line of its text, and with nothing chosen it waits to be sorted', async () => {
  narrow(false)
  const calls = server([note(8, 30, { kind: 'idea', body: 'Paint the fence\n\nwhite, this time' })])
  await open('/notes/8')
  await userEvent.click(await waitFor(() => tools()[1]))
  expect(titleField()).toHaveValue('Paint the fence')
  await userEvent.click(dialog().getAllByRole('button').at(-1)!)
  expect(writes(calls)).toEqual([['POST', '/api/tasks', { tasks: [{ title: 'Paint the fence', area: 'personal', links: ['notes:8'] }] }]])
  await waitFor(() => expect(linkedFrom()[0]?.querySelector('.mk')).toHaveClass('t-sort'))
})

test('Cancel and Escape close the form with something entered, asking nothing and sending nothing, and a task the server refuses keeps the form as entered and says why', async () => {
  narrow(false)
  const calls = server([shelves()], [], (c) => (c.method === 'POST' && c.path === '/api/tasks' ? json({ error: 'refused' }, 400) : undefined))
  await open('/notes/7')
  await userEvent.click(await waitFor(() => tools()[1]))
  await userEvent.type(notesField(), 'gone')
  await userEvent.click(dialog().getAllByRole('button').at(-2)!)
  expect(screen.queryByRole('dialog')).toBeNull()
  await userEvent.click(tools()[1])
  await userEvent.type(notesField(), 'gone')
  fireEvent(screen.getByRole('dialog'), new Event('cancel'))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(writes(calls)).toEqual([])

  await userEvent.click(tools()[1])
  await userEvent.type(notesField(), 'by hand')
  await userEvent.click(dialog().getAllByRole('button').at(-1)!)
  expect(await dialog().findByRole('alert')).toBeInTheDocument()
  expect(notesField()).toHaveValue('by hand')
  expect(writes(calls)).toHaveLength(1)
})

test('leaving the note with something entered in the dialog asks first, and Keep editing keeps it', async () => {
  narrow(false)
  server([shelves()])
  const { where } = await open('/notes/7')
  await userEvent.click(await waitFor(() => tools()[1]))
  await userEvent.type(notesField(), 'by hand')
  await userEvent.click(within(screen.getByRole('navigation')).getAllByRole('link').find((a) => a.getAttribute('href') === '/inbox')!)
  const asked = await waitFor(() => {
    const open = screen.getAllByRole('dialog')
    expect(open).toHaveLength(2)
    return within(open[1])
  })
  await userEvent.click(asked.getAllByRole('button')[0])
  expect(where()).toBe('/notes/7')
  expect(notesField()).toHaveValue('by hand')
})

test('turning a phone keeps Make a task where it opened, with what was entered', async () => {
  const turn = narrow(true)
  server([shelves()])
  await open('/notes/7')
  await userEvent.click(await waitFor(() => tools()[1]))
  await userEvent.type(notesField(), 'by hand')
  await act(async () => turn(false))
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(notesField()).toHaveValue('by hand')
})

test('on a phone the note’s links unfold once a task is made, to show it among what links to the note', async () => {
  narrow(true)
  server([shelves()])
  await open('/notes/7')
  await userEvent.click(await waitFor(() => tools()[1]))
  await userEvent.click(within(main().querySelector<HTMLElement>('.task-make > .acts')!).getAllByRole('button')[0])
  await waitFor(() => expect(linkedFrom()).toHaveLength(1))
  expect(main().querySelector<HTMLDetailsElement>('details.sect')!.open).toBe(true)
})

test('on a phone Make a task is the note’s page, Save above Cancel; leaving it with something entered asks first, and Save goes back to the note', async () => {
  narrow(true)
  const tasks: Task[] = []
  const calls = server([shelves()], tasks)
  const { where, back } = await open('/notes/7')
  const notesTab = () => within(screen.getByRole('navigation')).getAllByRole('link').find((a) => a.getAttribute('href') === '/notes')!
  await userEvent.click(await waitFor(() => tools()[1]))
  expect(screen.queryByRole('dialog')).toBeNull()
  // With nothing entered leaving asks nothing.
  await userEvent.click(notesTab())
  await waitFor(() => expect(where()).toBe('/notes'))
  await back()
  await userEvent.click(await waitFor(() => tools()[1]))
  await userEvent.type(notesField(), 'by hand')
  await userEvent.click(notesTab())
  // Keep editing stays, with what was entered.
  await userEvent.click(dialog().getAllByRole('button')[0])
  expect(where()).toBe('/notes/7')
  expect(notesField()).toHaveValue('by hand')

  const [save, cancel] = within(main().querySelector<HTMLElement>('.task-make > .acts')!).getAllByRole('button')
  expect(cancel).toBeEnabled()
  await userEvent.click(save)
  expect(writes(calls)).toEqual([['POST', '/api/tasks', { tasks: [{ title: 'Shelves', notes: 'by hand', area: 'garden', links: ['notes:7'] }] }]])
  await waitFor(() => expect(tools()).toHaveLength(3))
  expect(where()).toBe('/notes/7')
})
