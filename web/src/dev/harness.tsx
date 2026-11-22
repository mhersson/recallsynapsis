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
import type { Area, Token } from '../api/types.ts'
import { AREAS, HEALTH, HITS, SECRET, SESSION, TOKENS } from './fixtures.ts'

const query = new URLSearchParams(window.location.search)

const json = (body: unknown, status = 200, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } })

// What the page changes is kept until it reloads. empty=1 starts with the personal area alone and no token,
// modules=a,b switches those modules on, and backup=old or backup=none ages or drops the last backup.
const empty = query.get('empty') === '1'
const areas = empty ? AREAS.filter((a) => a.kind === 'personal') : AREAS
const tokens = empty ? [] : TOKENS
const backups: Record<string, string | null> = { old: new Date(Date.now() - 3 * 24 * 60 * 60_000).toISOString(), none: null }
const session = {
  ...SESSION,
  modules: query.get('modules')?.split(',') ?? SESSION.modules,
  last_backup: (query.get('backup') ?? '') in backups ? backups[query.get('backup')!] : SESSION.last_backup,
}

// login=wrong refuses every sign-in, and login=wait makes it wait 28 seconds.
function answer(method: string, url: URL, body: Record<string, unknown>): Response {
  const path = url.pathname
  if (method === 'POST' && path === '/api/auth/login') {
    if (query.get('login') === 'wrong') return json({ error: 'wrong name or password' }, 401)
    if (query.get('login') === 'wait') return json({ error: 'too many failed sign-ins' }, 429, { 'Retry-After': '28' })
    return new Response(null, { status: 204 })
  }
  if (method === 'POST' && path === '/api/auth/logout') return new Response(null, { status: 204 })
  if (method === 'POST' && path === '/api/areas') {
    const name = String(body.name)
    if (areas.some((a) => a.name.toLowerCase() === name.toLowerCase())) return json({ error: `a project named ${name} exists` }, 409)
    const a: Area = { id: areas.length + 10, kind: 'project', name, slug: name.toLowerCase(), description: String(body.description), archived: false }
    areas.push(a)
    return json(a, 201)
  }
  if (method === 'POST' && path === '/api/tokens') {
    const t: Token = { id: Math.max(0, ...tokens.map((x) => x.id)) + 1, name: String(body.name), access: body.access === 'write' ? 'write' : 'read', created_at: new Date().toISOString(), last_used_at: null }
    tokens.push(t)
    return json({ ...t, token: SECRET }, 201)
  }
  const token = tokens.findIndex((t) => path === `/api/tokens/${t.id}`)
  if (method === 'DELETE' && token >= 0) {
    tokens.splice(token, 1)
    return new Response(null, { status: 204 })
  }
  const area = areas.find((a) => path === `/api/areas/${a.id}`)
  if (method === 'PATCH' && area) return json(Object.assign(area, body))
  if (method !== 'GET') return json({ error: 'not found' }, 404)
  if (path === '/api/health') return json(HEALTH)
  if (path === '/api/session') return json(session)
  if (path === '/api/tokens') return json({ tokens })
  if (path === '/api/search') {
    const q = url.searchParams.get('q')!.toLowerCase()
    const area = url.searchParams.get('area')
    const hits = HITS.filter((h) => (h.title + h.snippet).toLowerCase().includes(q) && (!area || h.area === area))
    return json({ groups: hits.length ? [{ module: 'notes', hits }] : [] })
  }
  if (path === '/api/areas') return json({ areas: areas.filter((a) => url.searchParams.get('archived') === 'true' || !a.archived) })
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
  const body = typeof init?.body === 'string' ? (JSON.parse(init.body) as Record<string, unknown>) : {}
  return Promise.resolve(answer(init?.method ?? 'GET', new URL(String(input), window.location.origin), body))
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

// click=TEXT clicks the first button, link or summary whose text starts with TEXT, and type=TEXT types TEXT into the
// field that has the focus. Each waits until the page shows what it needs, and given more than once they run in order.
function step(kind: string, text: string): boolean {
  if (kind === 'type') {
    const field = document.activeElement
    if (!(field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement)) return false
    // React hears a value set through the prototype's setter and then announced.
    Object.getOwnPropertyDescriptor(Object.getPrototypeOf(field), 'value')!.set!.call(field, text)
    field.dispatchEvent(new Event('input', { bubbles: true }))
    return true
  }
  const target = [...document.querySelectorAll<HTMLElement>('button:enabled, a, summary')].find((e) => e.textContent?.startsWith(text))
  target?.click()
  target?.scrollIntoView()
  return !!target
}
void (async () => {
  for (const [kind, text] of [...query].filter(([k]) => k === 'click' || k === 'type')) {
    for (let tries = 0; tries < 20; tries++) {
      await new Promise((r) => setTimeout(r, 100))
      if (step(kind, text)) break
    }
  }
})()

createRoot(document.getElementById('root')!).render(
  <MemoryRouter initialEntries={[query.get('path') ?? '/']}>
    <Apply />
    <App />
  </MemoryRouter>,
)
