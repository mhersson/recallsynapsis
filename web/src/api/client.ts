import { useEffect, useState } from 'react'

export class ApiError extends Error {
  status: number
  // The seconds a Retry-After header asks the client to wait, else 0.
  retryAfter: number

  constructor(status: number, message: string, retryAfter = 0) {
    super(message)
    this.status = status
    this.retryAfter = retryAfter
  }
}

// The one line a page shows for a refusal or a failure.
export const message = (e: unknown) => (e instanceof Error ? e.message : String(e))

let unauthorized: () => void = () => {}
let redirected: () => void = () => {}

// One handler, replaced by a later call.
export function onUnauthorized(handler: () => void) {
  unauthorized = handler
}

// One handler, replaced by a later call.
export function onRedirect(handler: () => void) {
  redirected = handler
}

// A file or a blob is sent as it is, as its own type; any other body as JSON. signal ends the request.
export async function api<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const headers: Record<string, string> = {}
  const raw = body instanceof Blob
  if (body !== undefined) headers['Content-Type'] = raw ? body.type || 'application/octet-stream' : 'application/json'
  if (method !== 'GET' && method !== 'HEAD') headers['X-Recallsynapsis-CSRF'] = '1'
  const res = await fetch('/api' + path, {
    method,
    headers,
    credentials: 'same-origin',
    // With the default follow, the page's policy refuses a redirect to another origin and fetch fails as if offline.
    redirect: 'manual',
    body: body === undefined || raw ? body : JSON.stringify(body),
    signal,
  })
  if (res.type === 'opaqueredirect') {
    redirected()
    throw new ApiError(0, 'Access to this page has ended. Reload to sign in again.')
  }
  if (res.status === 204) return undefined as T
  if (res.status >= 400) {
    // A refused sign-in is the sign-in page's to show.
    if (res.status === 401 && !(method === 'POST' && path === '/auth/login')) unauthorized()
    let message = res.statusText || `HTTP ${res.status}`
    try {
      const e = (await res.json()) as { error?: unknown }
      if (typeof e.error === 'string') message = e.error
    } catch {
      // the body is not the server's JSON
    }
    throw new ApiError(res.status, message, Number(res.headers.get('Retry-After')) || 0)
  }
  return (await res.json()) as T
}

// What GET path gives, read again when path or again changes; '' reads nothing. An answer to a question asked since is
// dropped, and a refusal is kept until a read succeeds.
export function useGet<T>(path: string, again?: unknown): { got?: T; failed: string } {
  const [got, setGot] = useState<T>()
  const [failed, setFailed] = useState('')
  useEffect(() => {
    if (!path) return
    let stale = false
    api<T>('GET', path).then(
      (v) => {
        if (stale) return
        setGot(v)
        setFailed('')
      },
      (e: unknown) => !stale && setFailed(message(e)),
    )
    return () => {
      stale = true
    }
  }, [path, again])
  return { got, failed }
}

export const step = 50

// The limit of a list for the query asked: 50, and 50 more each time more is called, up to the 500 a list gives. A new
// query starts again at 50.
export function useLimit(asked: string) {
  const [shown, setShown] = useState({ asked, limit: step })
  const limit = shown.asked === asked ? shown.limit : step
  return { limit, more: limit < 500 ? () => setShown({ asked, limit: limit + step }) : undefined }
}
