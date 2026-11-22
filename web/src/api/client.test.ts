import { redirected } from '../test-server.ts'
import { ApiError, api, onRedirect, onUnauthorized } from './client.ts'

function fakeServer(status: number, body?: unknown) {
  const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) =>
    status === 204
      ? new Response(null, { status })
      : new Response(JSON.stringify(body), { status, statusText: 'Status text' }),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function headersOf(fetchMock: ReturnType<typeof fakeServer>) {
  return new Headers(fetchMock.mock.calls[0][1]?.headers)
}

afterEach(() => {
  vi.unstubAllGlobals()
  onUnauthorized(() => {})
  onRedirect(() => {})
})

test('client follows no redirect, on a read or a write', async () => {
  const read = fakeServer(200, {})
  await api('GET', '/areas')
  expect(read.mock.calls[0][1]?.redirect).toBe('manual')

  const write = fakeServer(200, {})
  await api('POST', '/areas', { name: 'x' })
  expect(write.mock.calls[0][1]?.redirect).toBe('manual')
})

test('a redirect calls its own handler and the caller gets an error', async () => {
  const redirect = vi.fn()
  const unauthorized = vi.fn()
  onRedirect(redirect)
  onUnauthorized(unauthorized)
  vi.stubGlobal('fetch', vi.fn(async () => redirected()))
  const err = await api('GET', '/areas').catch((e: unknown) => e)
  expect(err).toBeInstanceOf(ApiError)
  expect(err).toMatchObject({ status: 0 })
  expect(redirect).toHaveBeenCalledTimes(1)
  expect(unauthorized).not.toHaveBeenCalled()

  fakeServer(401, { error: 'sign in' })
  await expect(api('GET', '/areas')).rejects.toMatchObject({ status: 401 })
  expect(unauthorized).toHaveBeenCalledTimes(1)
  expect(redirect).toHaveBeenCalledTimes(1)
})

test('client sends the CSRF header on a write and not on a read', async () => {
  const read = fakeServer(200, {})
  await api('GET', '/areas')
  expect(read.mock.calls[0][0]).toBe('/api/areas')
  expect(headersOf(read).has('X-Recallsynapsis-CSRF')).toBe(false)

  const write = fakeServer(200, {})
  await api('POST', '/areas', { name: 'x' })
  expect(headersOf(write).get('X-Recallsynapsis-CSRF')).toBeTruthy()
  expect(headersOf(write).get('Content-Type')).toBe('application/json')
  const init = write.mock.calls[0][1]
  expect(init?.body).toBe('{"name":"x"}')
  expect(init?.credentials).toBe('same-origin')

  const del = fakeServer(204)
  await expect(api('DELETE', '/tokens/1')).resolves.toBeUndefined()
  expect(headersOf(del).get('X-Recallsynapsis-CSRF')).toBeTruthy()
})

test("client throws the server's line", async () => {
  fakeServer(400, { error: 'name is required' })
  const err = await api('POST', '/areas', {}).catch((e: unknown) => e)
  expect(err).toBeInstanceOf(ApiError)
  expect(err).toMatchObject({ status: 400, message: 'name is required' })

  fakeServer(502, undefined)
  const plain = await api('GET', '/areas').catch((e: unknown) => e)
  expect(plain).toMatchObject({ status: 502, message: 'Status text' })
})

test('client names the status when a non-JSON body comes with no status text', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response('<html>Bad Gateway</html>', { status: 502 })))
  const err = await api('GET', '/areas').catch((e: unknown) => e)
  expect(err).toMatchObject({ status: 502 })
  expect((err as Error).message).toContain('502')
  expect((err as Error).message).not.toContain('<html>')
})

test('a 401 calls the registered handler', async () => {
  const handler = vi.fn()
  onUnauthorized(handler)
  fakeServer(401, { error: 'sign in' })
  await expect(api('GET', '/areas')).rejects.toMatchObject({ status: 401 })
  expect(handler).toHaveBeenCalledTimes(1)

  fakeServer(401, { error: 'wrong name or password' })
  await expect(api('POST', '/auth/login', {})).rejects.toMatchObject({ status: 401 })
  expect(handler).toHaveBeenCalledTimes(1)
})

test('a refusal carries the wait the server asks for', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response('{}', { status: 429, headers: { 'Retry-After': '28' } })))
  await expect(api('POST', '/auth/login', {})).rejects.toMatchObject({ status: 429, retryAfter: 28 })

  fakeServer(429, {})
  await expect(api('POST', '/auth/login', {})).rejects.toMatchObject({ status: 429, retryAfter: 0 })
})
