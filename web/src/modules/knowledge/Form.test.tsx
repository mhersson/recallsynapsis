import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { open } from '../../test-app.tsx'
import { json, narrow, type Call } from '../../test-server.ts'
import { TODAY, areas, decision, keeper } from './test-knowledge.ts'

afterEach(() => vi.unstubAllGlobals())

const main = () => within(screen.getByRole('main'))
const form = () => screen.getByRole('main').querySelector('form')
// The line, the why and the source, the day and Replaces.
const texts = () => within(form()!).getAllByRole('textbox')
const day = () => form()!.querySelector('input[type="date"]')!
const replaces = () => within(form()!).getByRole('combobox')
// Save submits the form, and Cancel is a plain button beside it.
const save = () => within(form()!).getAllByRole('button').find((b) => b.getAttribute('type') !== 'button')!
const cancel = () => within(form()!).getByRole('button', { name: (_, b) => b.getAttribute('type') === 'button' })
const writes = (calls: Call[]) => calls.filter((c) => c.method !== 'GET').map((c) => [c.method, c.path, c.body])
const add = () => main().queryAllByRole('link').find((a) => a.getAttribute('href')?.startsWith('/decisions/new'))
const side = () => within(screen.getAllByRole('navigation')[0])
// The rows that lead to another decision, as [where each leads, its mark].
const leads = () => [...screen.getByRole('main').querySelectorAll('.kpart .lrow')].map((r) => [r.querySelector('a')!.getAttribute('href'), r.querySelector('.mk')!.className])

test('Add a decision sends a package of one for the project, with the day shown and what Replaces names, and then shows the decision, which replaces it', async () => {
  narrow(false)
  const calls = keeper([decision(7, '2026-11-26'), decision(6, '2026-11-22')])
  const { where, back } = await open('/areas/2?tab=decisions')
  await waitFor(() => expect(add()).toBeDefined())
  await userEvent.click(add()!)
  await waitFor(() => expect(form()).not.toBeNull())
  expect(day()).toHaveValue(TODAY)
  expect(within(replaces()).getAllByRole('option').map((o) => o.getAttribute('value'))).toEqual(['', 'knowledge:7', 'knowledge:6'])
  expect(replaces()).toHaveValue('')

  await userEvent.type(texts()[0], 'mulch the beds')
  await userEvent.type(texts()[1], 'it keeps the water')
  await userEvent.selectOptions(replaces(), 'knowledge:6')
  await userEvent.click(save())
  expect(writes(calls)).toEqual([
    ['POST', '/api/knowledge', { area: 'garden', add: [{ decision: 'mulch the beds', why: 'it keeps the water', source: '', decided: TODAY, replaces: 'knowledge:6' }] }],
  ])
  await waitFor(() => expect(where()).toBe('/decisions/8'))
  await waitFor(() => expect(main().getByRole('heading', { level: 1 })).toHaveTextContent('mulch the beds'))
  expect(leads()).toEqual([['/decisions/6', 'mk m-decision d-replaced']])

  // The form is left behind: a step back leads to the tab.
  await back()
  expect(where()).toBe('/areas/2?tab=decisions')
})

test('a project with none offers Add a decision and says so, and an archived one takes no decision there', async () => {
  narrow(false)
  const empty = () => screen.getByRole('main').querySelector('.decisions .empty')
  keeper([])
  const page = await open('/areas/3?tab=decisions')
  await waitFor(() => expect(empty()).not.toBeNull())
  expect(add()).toBeDefined()
  page.unmount()

  const archived = areas.map((a) => (a.id === 3 ? { ...a, archived: true } : a))
  keeper([], (c) => (c.path.startsWith('/api/areas') ? json({ areas: archived }) : undefined))
  await open('/areas/3?tab=decisions')
  await waitFor(() => expect(empty()).not.toBeNull())
  expect(add()).toBeUndefined()
})

test('an empty why is refused by the server: the form keeps what was entered with the server\'s line above Save, and a second Save sends it again', async () => {
  narrow(false)
  const calls = keeper([decision(7, '2026-11-26')])
  await open('/decisions/new?area=garden')
  await waitFor(() => expect(form()).not.toBeNull())
  // A line takes no line break: Enter saves.
  await userEvent.type(texts()[0], 'mulch the beds{Enter}')
  const alert = await main().findByRole('alert')
  expect(alert.compareDocumentPosition(save()) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  expect(texts()[0]).toHaveValue('mulch the beds')

  await userEvent.click(save())
  const sent = { area: 'garden', add: [{ decision: 'mulch the beds', why: '', source: '', decided: TODAY }] }
  expect(writes(calls)).toEqual([
    ['POST', '/api/knowledge', sent],
    ['POST', '/api/knowledge', sent],
  ])
  expect(main().getByRole('alert')).toBeInTheDocument()
})

test('leaving the form with a change asks first, and with none does not, and Cancel sends nothing and asks nothing', async () => {
  narrow(false)
  const calls = keeper([decision(7, '2026-11-26')])
  const { where } = await open('/areas/2?tab=decisions')
  const away = () => userEvent.click(side().getAllByRole('link').find((a) => a.getAttribute('href') === '/areas/1')!)
  await waitFor(() => expect(add()).toBeDefined())
  await userEvent.click(add()!)
  await waitFor(() => expect(form()).not.toBeNull())
  await userEvent.type(texts()[1], 'because')
  await away()
  await userEvent.click(within(screen.getByRole('dialog')).getAllByRole('button')[0])
  expect(where()).toBe('/decisions/new?area=garden')
  expect(texts()[1]).toHaveValue('because')

  await userEvent.click(cancel())
  expect(screen.queryByRole('dialog')).toBeNull()
  await waitFor(() => expect(where()).toBe('/areas/2?tab=decisions'))

  await waitFor(() => expect(add()).toBeDefined())
  await userEvent.click(add()!)
  await waitFor(() => expect(form()).not.toBeNull())
  await away()
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(where()).toBe('/areas/1')
  expect(writes(calls)).toEqual([])
})

test('Cancel sits before Save on desktop, and Save above Cancel on a phone', async () => {
  narrow(false)
  keeper([decision(7, '2026-11-26')])
  const desk = await open('/decisions/new?area=garden')
  await waitFor(() => expect(form()).not.toBeNull())
  expect(within(form()!).getAllByRole('button')).toEqual([cancel(), save()])
  desk.unmount()

  narrow(true)
  keeper([decision(7, '2026-11-26')])
  await open('/decisions/new?area=garden')
  await waitFor(() => expect(form()).not.toBeNull())
  expect(within(form()!).getAllByRole('button')).toEqual([save(), cancel()])
})
