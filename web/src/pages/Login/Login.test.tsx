import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { fakeServer, json } from '../../test-server.ts'
import Login from './Login.tsx'

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

function show(answer: Parameters<typeof fakeServer>[0]) {
  const calls = fakeServer((c) => (c.path === '/api/health' ? json({ status: 'ok', version: '1.2.3' }) : answer(c)))
  render(
    <MemoryRouter>
      <Login />
    </MemoryRouter>,
  )
  const name = screen.getByRole('textbox')
  const password = document.querySelector<HTMLInputElement>('input[type=password]')!
  return { calls, name, password, button: screen.getByRole('button') }
}

test('the brand bar shows the version', async () => {
  show(() => undefined)
  expect(await screen.findByText('1.2.3')).toBeInTheDocument()
})

test('a wrong name or password clears the password and keeps the name', async () => {
  const { calls, name, password, button } = show((c) => (c.path === '/api/auth/login' ? json({ error: 'no' }, 401) : undefined))
  const user = userEvent.setup()
  expect(name).toHaveFocus()
  await user.type(name, 'ada')
  await user.type(password, 'secret')
  await user.click(button)

  const login = calls.find((c) => c.path === '/api/auth/login')!
  expect(login.method).toBe('POST')
  expect(login.body).toEqual({ name: 'ada', password: 'secret' })
  expect(login.headers.get('X-Recallsynapsis-CSRF')).toBeTruthy()
  expect(await screen.findByRole('alert')).toBeInTheDocument()
  expect(name).toHaveValue('ada')
  expect(password).toHaveValue('')
  expect(password).toHaveFocus()
})

test('while sign-ins must wait, the button is off until the wait has passed', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true })
  let waits = true
  const { calls, password, button } = show((c) => {
    if (c.path !== '/api/auth/login') return
    return waits ? json({ error: 'wait' }, 429, { 'Retry-After': '3' }) : new Response(null, { status: 204 })
  })
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
  await user.type(password, 'secret')
  await user.click(button)

  const alert = await screen.findByRole('alert')
  expect(button).toBeDisabled()
  expect(password).toHaveValue('secret')
  // The page counts down a second at a time.
  for (let s = 0; s < 2; s++) await act(() => vi.advanceTimersByTimeAsync(1000))
  expect(button).toBeDisabled()
  expect(alert).toBeInTheDocument()
  await act(() => vi.advanceTimersByTimeAsync(1000))
  expect(button).toBeEnabled()
  expect(screen.queryByRole('alert')).toBeNull()

  waits = false
  await user.click(button)
  expect(calls.filter((c) => c.path === '/api/auth/login')).toHaveLength(2)
})

test('any other refusal is shown as it is', async () => {
  const { button } = show((c) => (c.path === '/api/auth/login' ? json({ error: 'database is locked' }, 500) : undefined))
  await userEvent.click(button)
  expect(await screen.findByRole('alert')).toHaveTextContent('database is locked')
  expect(button).toBeEnabled()
})
