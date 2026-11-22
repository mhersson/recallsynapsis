import { act, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Session, Token } from '../../api/types.ts'
import { open } from '../../test-app.tsx'
import { FakeEventSource, fakeServer, json, narrow, type Call } from '../../test-server.ts'

afterEach(() => vi.unstubAllGlobals())

const hours = (h: number) => new Date(Date.now() - h * 60 * 60_000).toISOString()
const session = (last_backup: string | null = null): Session => ({ user: { name: 'ada' }, modules: [], version: '1.2.3', timezone: 'UTC', last_backup })
const token = (id: number, name: string): Token => ({ id, name, access: 'read', created_at: hours(48), last_used_at: null })
const secret = 'recall_tV3n9QkZp2Lw8xRb5HcM1yJf7sGd4aEu6oNqX0iTzKe'

// A server that keeps its tokens, makes one on POST and forgets one on DELETE; answer, when given, answers first.
function server(tokens: Token[], last_backup: string | null = null, answer: (c: Call) => Response | undefined = () => undefined) {
  return fakeServer((c: Call) => {
    const a = answer(c)
    if (a) return a
    if (c.path === '/api/session') return json(session(last_backup))
    if (c.path.startsWith('/api/areas')) return json({ areas: [] })
    if (c.path === '/api/auth/logout') return new Response(null, { status: 204 })
    if (c.path === '/api/tokens' && c.method === 'GET') return json({ tokens })
    if (c.path === '/api/tokens' && c.method === 'POST') {
      const t = { ...token(9, ''), ...(c.body as object) }
      tokens.push(t)
      return json({ ...t, token: secret }, 201)
    }
    const gone = tokens.findIndex((t) => c.method === 'DELETE' && c.path === `/api/tokens/${t.id}`)
    if (gone >= 0) {
      tokens.splice(gone, 1)
      return new Response(null, { status: 204 })
    }
  })
}

const main = () => within(screen.getByRole('main'))
// A section of Settings on desktop, by its anchor.
const section = (id: string) => within(document.getElementById(id)!)
const tokenReads = (calls: Call[]) => calls.filter((c) => c.method === 'GET' && c.path === '/api/tokens').length
const rows = () => main().getAllByRole('row').slice(1)

test('a token is made in place and shown once, with Copy, until Done', async () => {
  narrow(false)
  const writeText = vi.fn(() => Promise.resolve())
  vi.stubGlobal('navigator', { ...navigator, clipboard: { writeText } })
  const calls = server([token(1, 'portal')])
  await open('/settings')
  expect(rows()).toHaveLength(1)

  await userEvent.click(section('tokens').getAllByRole('button').at(-1)!)
  const form = within(main().getByRole('textbox').closest('form')!)
  const [create] = form.getAllByRole('button')
  expect(create).toBeDisabled()
  await userEvent.type(form.getByRole('textbox'), 'kitchen-tablet')
  await userEvent.click(form.getAllByRole('radio')[1])
  expect(form.getAllByRole('radio')[1]).toHaveAttribute('aria-checked', 'true')
  await userEvent.click(create)

  const post = calls.find((c) => c.method === 'POST')!
  expect([post.path, post.body]).toEqual(['/api/tokens', { name: 'kitchen-tablet', access: 'write' }])
  expect(tokenReads(calls)).toBe(2)
  expect(rows()).toHaveLength(2)
  expect(main().getByText(secret)).toBeInTheDocument()

  const [copy, done] = section('tokens').getAllByRole('button').slice(rows().length)
  await userEvent.click(copy)
  expect(writeText).toHaveBeenCalledWith(secret)
  await userEvent.click(done)
  expect(main().queryByText(secret)).toBeNull()
})

test('a token made is not shown again once the page is opened anew', async () => {
  narrow(false)
  server([])
  const first = await open('/settings')
  await userEvent.click(section('tokens').getByRole('button'))
  await userEvent.type(main().getByRole('textbox'), 'aim{Enter}')
  expect(main().getByText(secret)).toBeInTheDocument()
  first.unmount()

  await open('/settings')
  expect(rows()).toHaveLength(1)
  expect(screen.queryByText(secret)).toBeNull()
})

test('a token made on a phone stays shown when the phone is turned to the width of a desktop', async () => {
  const turn = narrow(true)
  server([])
  await open('/settings')
  await userEvent.click(within(main().getAllByRole('group')[0]).getByRole('button'))
  await userEvent.type(main().getByRole('textbox'), 'aim{Enter}')
  expect(main().getByText(secret)).toBeInTheDocument()
  act(() => turn(false))
  expect(main().getByText(secret)).toBeInTheDocument()
})

test('a refused token keeps the form and its name, says why, and shows no token', async () => {
  narrow(false)
  server([token(1, 'aim')], null, (c) => (c.method === 'POST' ? json({ error: 'taken' }, 409) : undefined))
  await open('/settings')
  await userEvent.click(section('tokens').getAllByRole('button').at(-1)!)
  await userEvent.type(main().getByRole('textbox'), 'aim{Enter}')
  expect(section('tokens').getByRole('alert')).toBeInTheDocument()
  expect(main().getByRole('textbox')).toHaveValue('aim')
  expect(main().queryByText(secret)).toBeNull()
})

test('the tokens are read again when the stream opens again', async () => {
  narrow(false)
  const calls = server([])
  await open('/settings')
  const es = FakeEventSource.all[0]
  await act(async () => es.open())
  await act(async () => es.fail())
  await act(async () => es.open())
  expect(tokenReads(calls)).toBe(2)
})

test('Revoke asks first, and only Revoke in the question sends it', async () => {
  narrow(false)
  const calls = server([token(1, 'portal'), token(2, 'aim')])
  await open('/settings')
  await userEvent.click(within(rows()[1]).getByRole('button'))
  const [keep] = within(screen.getByRole('dialog')).getAllByRole('button')
  await userEvent.click(keep)
  expect(screen.queryByRole('dialog')).toBeNull()
  expect(calls.filter((c) => c.method === 'DELETE')).toEqual([])

  await userEvent.click(within(rows()[1]).getByRole('button'))
  await userEvent.click(within(screen.getByRole('dialog')).getAllByRole('button')[1])
  const del = calls.find((c) => c.method === 'DELETE')!
  expect(del.path).toBe('/api/tokens/2')
  expect(del.headers.get('X-Recallsynapsis-CSRF')).toBeTruthy()
  expect(rows()).toHaveLength(1)
})

test.each([
  [null, false],
  [hours(6), false],
  [hours(35), false],
  [hours(37), true],
])('a last backup made at %s is marked as late: %s', async (at, late) => {
  narrow(false)
  server([], at)
  await open('/settings')
  const backups = document.getElementById('backups')!
  expect(backups.querySelector('.mk.wait') !== null).toBe(late)
})

test('Backups shows the last backup as it is when the page opens, not as it was at sign-in', async () => {
  narrow(false)
  let reads = 0
  fakeServer((c) => {
    if (c.path === '/api/session') return json(session(reads++ === 0 ? hours(50) : hours(6)))
    if (c.path.startsWith('/api/areas')) return json({ areas: [] })
    if (c.path === '/api/tokens') return json({ tokens: [] })
  })
  await open('/settings')
  expect(document.getElementById('backups')!.querySelector('.mk.wait')).toBeNull()
})

test('the palette and the theme apply at once', async () => {
  narrow(false)
  server([])
  await open('/settings')
  const [palette, theme] = main().getAllByRole('radiogroup')
  await userEvent.click(within(palette).getAllByRole('radio')[1])
  await userEvent.click(within(theme).getAllByRole('radio')[2])
  expect(document.documentElement.dataset).toMatchObject({ palette: 'catppuccin', theme: 'light' })
  expect(within(theme).getAllByRole('radio')[2]).toHaveAttribute('aria-checked', 'true')
})

test('Sign out ends the session and opens sign-in', async () => {
  narrow(false)
  const calls = server([])
  const { where } = await open('/settings')
  await userEvent.click(section('session').getByRole('button'))
  expect(calls.some((c) => c.method === 'POST' && c.path === '/api/auth/logout')).toBe(true)
  expect(where()).toBe('/login')
})

test('on desktop the section names link to the sections, and the version ends the page', async () => {
  narrow(false)
  server([])
  await open('/settings')
  const anchors = within(main().getByRole('navigation')).getAllByRole('link')
  expect(anchors.map((a) => document.getElementById(a.getAttribute('href')!.slice(1))?.tagName)).toEqual(['SECTION', 'SECTION', 'SECTION', 'SECTION'])
  expect(screen.getByRole('main')).toHaveTextContent('1.2.3')
})

test('on a phone each section folds, and the first is open', async () => {
  narrow(true)
  server([])
  await open('/settings')
  const sections = main().getAllByRole('group')
  expect(sections.map((d) => (d as HTMLDetailsElement).open)).toEqual([true, false, false, false])
})
