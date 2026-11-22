// Development only: shows any route on fixed data, with no server. Vite builds index.html alone.
import { useEffect } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router'
import '../theme/tokens.css'
import '../theme/base.css'
import App from '../App.tsx'
import { api } from '../api/client.ts'
import { useLook } from '../theme/theme.ts'
import type { Look } from '../theme/theme.ts'
import { AREAS, HEALTH, SESSION } from './fixtures.ts'

const query = new URLSearchParams(window.location.search)

const json = (body: unknown, status = 200, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } })

// login=wrong refuses every sign-in, and login=wait makes it wait 28 seconds.
function answer(method: string, url: URL): Response {
  const path = url.pathname
  if (method === 'POST' && path === '/api/auth/login') {
    if (query.get('login') === 'wrong') return json({ error: 'wrong name or password' }, 401)
    if (query.get('login') === 'wait') return json({ error: 'too many failed sign-ins' }, 429, { 'Retry-After': '28' })
    return new Response(null, { status: 204 })
  }
  if (method === 'POST' && path === '/api/auth/logout') return new Response(null, { status: 204 })
  if (method !== 'GET') return json({ error: 'not found' }, 404)
  if (path === '/api/health') return json(HEALTH)
  if (path === '/api/session') return json(SESSION)
  if (path === '/api/areas') return json({ areas: AREAS.filter((a) => url.searchParams.get('archived') === 'true' || !a.archived) })
  return json({ error: 'not found' }, 404)
}

// lapsed=1 ends the access a moment after the page loads: every later request is redirected, and one is made, as the next page's would be.
let lapsed = false
if (query.get('lapsed') === '1') {
  setTimeout(() => {
    lapsed = true
    api('GET', '/health').catch(() => {})
  }, 500)
}

window.fetch = (input, init) => {
  if (lapsed) return Promise.resolve(Object.defineProperties(new Response(null), { type: { value: 'opaqueredirect' }, status: { value: 0 } }))
  return Promise.resolve(answer(init?.method ?? 'GET', new URL(String(input), window.location.origin)))
}

// A stream that opens and stays live.
class LiveEventSource {
  static CLOSED = 2
  readyState = 1
  onopen: (() => void) | null = null
  constructor() {
    setTimeout(() => this.onopen?.(), 0)
  }
  close() {
    this.readyState = LiveEventSource.CLOSED
  }
}
window.EventSource = LiveEventSource as unknown as typeof EventSource

const look: Look = {
  theme: ['dark', 'light'].includes(query.get('theme') ?? '') ? (query.get('theme') as Look['theme']) : 'system',
  palette: query.get('palette') === 'catppuccin' ? 'catppuccin' : 'copper',
}

export function Apply() {
  const [, setLook] = useLook()
  useEffect(() => setLook(look), [setLook])
  return null
}

// click=TEXT clicks the first button, link or summary whose text starts with TEXT, once the page shows it.
const click = query.get('click')
if (click) {
  let tries = 0
  const timer = setInterval(() => {
    const target = [...document.querySelectorAll<HTMLElement>('button, a, summary')].find((e) => e.textContent?.startsWith(click))
    target?.click()
    target?.scrollIntoView()
    if (target || ++tries === 20) clearInterval(timer)
  }, 100)
}

createRoot(document.getElementById('root')!).render(
  <MemoryRouter initialEntries={[query.get('path') ?? '/']}>
    <Apply />
    <App />
  </MemoryRouter>,
)
