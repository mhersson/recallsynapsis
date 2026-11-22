import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router'
import { fakeServer } from '../test-server.ts'
import { useSignOut } from './useSignOut.ts'

afterEach(() => vi.unstubAllGlobals())

function SignOut() {
  const signOut = useSignOut()
  return <button onClick={() => void signOut()} />
}

test('signing out ends the session and opens sign-in, also when the server cannot be reached', async () => {
  for (const reachable of [true, false]) {
    const calls = fakeServer((c) => (reachable && c.path === '/api/auth/logout' ? new Response(null, { status: 204 }) : undefined))
    if (!reachable) vi.stubGlobal('fetch', vi.fn(() => Promise.reject(new TypeError('Failed to fetch'))))
    const view = render(
      <MemoryRouter initialEntries={['/settings']}>
        <Routes>
          <Route path="/settings" element={<SignOut />} />
          <Route path="/login" element={<main />} />
        </Routes>
      </MemoryRouter>,
    )
    await userEvent.click(screen.getByRole('button'))
    expect(await screen.findByRole('main')).toBeInTheDocument()
    if (reachable) {
      expect(calls.map((c) => [c.method, c.path])).toEqual([['POST', '/api/auth/logout']])
      expect(calls[0].headers.get('X-Recallsynapsis-CSRF')).toBeTruthy()
    }
    view.unmount()
  }
})
