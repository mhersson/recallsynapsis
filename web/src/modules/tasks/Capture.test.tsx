import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { open } from '../../test-app.tsx'
import { json, narrow, type Call } from '../../test-server.ts'
import type { Now, Task } from './api.ts'
import { monday, tasksServer } from './test-tasks.ts'

afterEach(() => vi.unstubAllGlobals())

const dialog = () => within(screen.getByRole('dialog'))
const kinds = () => dialog().getAllByRole('radio')
const shortcuts = () => [...document.querySelectorAll<HTMLElement>('.when .sc')]
const writes = (calls: Call[]) => calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path, c.body])

// A server with notes and the modules given on, on the day now gives, that keeps the tasks made and the notes captured.
function server(now: Now = monday, modules = ['notes', 'tasks']) {
  const tasks: Task[] = []
  return tasksServer(
    tasks,
    now,
    (c) => {
      if (c.method === 'POST' && c.path === '/api/notes') return json({}, 201)
      if (c.path.startsWith('/api/notes')) return json({ notes: [], total: 0 })
    },
    modules,
  )
}

async function capture(text: string) {
  await userEvent.click(within(screen.getByRole('navigation')).getByRole('button'))
  await userEvent.type(dialog().getByRole('textbox'), text)
}

test.each([
  [['notes'], 2, ['', 'personal', 'garden']],
  [['notes', 'tasks'], 3, ['', 'personal', 'garden']],
])('with %j on Capture offers %i kinds, and a note goes to the inbox', async (modules, n, goes) => {
  narrow(false)
  const calls = server(monday, modules)
  await open('/projects')
  await capture('Beds')
  await waitFor(() => expect(dialog().getAllByRole('option').map((o) => o.getAttribute('value'))).toEqual(goes))
  expect(kinds()).toHaveLength(n)
  expect(shortcuts()).toHaveLength(0)
  await userEvent.click(dialog().getAllByRole('button').at(-1)!)
  expect(writes(calls)).toEqual([['POST', '/api/notes', { body: 'Beds', kind: 'note', inbox: true }]])
})

test('Capture with Task goes to an area, keeps what was typed, and sends the first line as the title, the rest as the notes and the dates of the shortcut chosen', async () => {
  narrow(false)
  const calls = server()
  await open('/projects')
  await capture('  Clean the filter{Enter}in the ventilation{Enter}{Enter}by the stairs  ')
  await userEvent.click(kinds()[2])
  // Goes to lists the areas and not the Inbox, the personal area chosen.
  await waitFor(() => expect(shortcuts()).toHaveLength(4))
  expect(dialog().getAllByRole('option').map((o) => o.getAttribute('value'))).toEqual(['personal', 'garden'])
  expect(dialog().getByRole('combobox')).toHaveValue('personal')
  expect(calls.some((c) => c.path === '/api/tasks?limit=1')).toBe(true)

  // What was typed stays while the kind is changed, and so does the shortcut chosen.
  await userEvent.click(shortcuts()[2])
  await userEvent.click(kinds()[1])
  expect(dialog().getByRole('combobox')).toHaveValue('')
  await userEvent.click(kinds()[2])
  expect(dialog().getByRole('textbox')).toHaveValue('  Clean the filter\nin the ventilation\n\nby the stairs  ')
  await waitFor(() => expect(shortcuts()[2]).toHaveAttribute('aria-current', 'true'))

  await userEvent.selectOptions(dialog().getByRole('combobox'), 'garden')
  await userEvent.click(dialog().getAllByRole('button').at(-1)!)
  expect(writes(calls)).toEqual([
    ['POST', '/api/tasks', { tasks: [{ title: 'Clean the filter', notes: 'in the ventilation\n\nby the stairs', area: 'garden', planned_from: '2026-11-28', planned_to: '2026-11-29' }] }],
  ])
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
})

test('with no shortcut chosen, or the one chosen picked again, Capture with Task sends a task to sort, and Weekend on a Sunday is that day, by the server’s today', async () => {
  narrow(false)
  // A Sunday, whose weekend is that day alone.
  const calls = server({ ...monday, date: '2026-12-06', weekday: 'Sunday' })
  await open('/projects')
  await capture('Wash the car')
  await userEvent.click(kinds()[2])
  await waitFor(() => expect(shortcuts()).toHaveLength(4))
  await userEvent.click(shortcuts()[2])
  expect(shortcuts()[2]).toHaveAttribute('aria-current', 'true')
  await userEvent.click(shortcuts()[2])
  expect(shortcuts()[2]).not.toHaveAttribute('aria-current')
  await userEvent.click(dialog().getAllByRole('button').at(-1)!)
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  await capture('Vacuum it')
  await userEvent.click(kinds()[2])
  await waitFor(() => expect(shortcuts()).toHaveLength(4))
  await userEvent.click(shortcuts()[2])
  await userEvent.click(dialog().getAllByRole('button').at(-1)!)
  expect(writes(calls)).toEqual([
    ['POST', '/api/tasks', { tasks: [{ title: 'Wash the car', area: 'personal' }] }],
    ['POST', '/api/tasks', { tasks: [{ title: 'Vacuum it', area: 'personal', planned_from: '2026-12-06', planned_to: '2026-12-06' }] }],
  ])
})

test('a task the server refuses keeps the dialog open with what was typed, and says why', async () => {
  narrow(false)
  const calls = server()
  await open('/projects')
  await capture('x'.repeat(201))
  await userEvent.click(kinds()[2])
  await userEvent.click(dialog().getAllByRole('button').at(-1)!)
  expect(await dialog().findByRole('alert')).toBeInTheDocument()
  expect(dialog().getByRole('textbox')).toHaveValue('x'.repeat(201))
  expect(writes(calls)).toHaveLength(1)
})

test('on a phone Capture with Task is a page, opened from a project with the project chosen, which goes back once the task is sent', async () => {
  narrow(true)
  const calls = server()
  const { where } = await open('/areas/2')
  await userEvent.click(within(screen.getByRole('navigation')).getAllByRole('link').find((a) => a.getAttribute('href') === '/capture?area=2')!)
  const main = within(screen.getByRole('main'))
  await userEvent.type(main.getByRole('textbox'), 'Plant the bulbs')
  await userEvent.click(main.getAllByRole('radio')[2])
  await waitFor(() => expect(main.getByRole('combobox')).toHaveValue('garden'))
  await waitFor(() => expect(shortcuts()).toHaveLength(4))
  await userEvent.click(shortcuts()[1])
  await userEvent.click(main.getAllByRole('button').at(-1)!)
  expect(writes(calls)).toEqual([['POST', '/api/tasks', { tasks: [{ title: 'Plant the bulbs', area: 'garden', planned_from: '2026-11-24', planned_to: '2026-11-24' }] }]])
  await waitFor(() => expect(where()).toBe('/areas/2'))
})
