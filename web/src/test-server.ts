// Fakes of what a page talks to, fetch and EventSource, and of the window's width.
export class FakeEventSource {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSED = 2
  static all: FakeEventSource[] = []
  readyState = FakeEventSource.CONNECTING
  onopen: (() => void) | null = null
  onmessage: ((m: MessageEvent<string>) => void) | null = null
  onerror: (() => void) | null = null
  url: string

  constructor(url: string) {
    this.url = url
    FakeEventSource.all.push(this)
  }

  close() {
    this.readyState = FakeEventSource.CLOSED
  }

  open() {
    this.readyState = FakeEventSource.OPEN
    this.onopen?.()
  }

  send(change: object) {
    this.onmessage?.({ data: JSON.stringify(change) } as MessageEvent<string>)
  }

  // A closed source is one the browser gave up on, as after a 502 or a 401.
  fail(closed = false) {
    this.readyState = closed ? FakeEventSource.CLOSED : FakeEventSource.CONNECTING
    this.onerror?.()
  }
}

export const json = (body: unknown, status = 200, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } })

// What fetch resolves with for a redirect it was told not to follow; no Response can be made with this type and status.
export const redirected = () =>
  Object.defineProperties(new Response(null), { type: { value: 'opaqueredirect' }, status: { value: 0 } })

// A body is what was sent: JSON parsed, a file as it is.
export type Call = { method: string; path: string; body: unknown; headers: Headers; signal?: AbortSignal }

// answer maps a request to its response, or to a promise of one that answers late; every request is recorded.
export function fakeServer(answer: (c: Call) => Response | Promise<Response> | undefined) {
  const calls: Call[] = []
  vi.stubGlobal('EventSource', FakeEventSource)
  FakeEventSource.all = []
  vi.stubGlobal(
    'fetch',
    vi.fn((path: string, init: RequestInit = {}) => {
      const call: Call = {
        method: init.method ?? 'GET',
        path,
        body: typeof init.body === 'string' ? JSON.parse(init.body) : init.body,
        headers: new Headers(init.headers),
        signal: init.signal ?? undefined,
      }
      calls.push(call)
      return Promise.resolve(answer(call) ?? json({ error: 'not found' }, 404))
    }),
  )
  return calls
}

// A phone's width while matches is true; the function it returns changes the width, as turning a phone does.
export function narrow(matches: boolean) {
  const listeners = new Set<() => void>()
  vi.stubGlobal('matchMedia', (query: string) => ({
    get matches() {
      return matches
    },
    media: query,
    addEventListener: (_: string, l: () => void) => listeners.add(l),
    removeEventListener: (_: string, l: () => void) => listeners.delete(l),
  }))
  return (next: boolean) => {
    matches = next
    listeners.forEach((l) => l())
  }
}
