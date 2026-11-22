// Development only: shows any route on fixed data, with no server. Vite builds index.html alone.
import { useEffect } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider, createMemoryRouter } from 'react-router'
import '../theme/tokens.css'
import '../theme/base.css'
import App from '../App.tsx'
import { api } from '../api/client.ts'
import { useLook } from '../theme/theme.ts'
import type { Look } from '../theme/theme.ts'
import type { Area, Token } from '../api/types.ts'
import type { Note } from '../modules/notes/api.ts'
import { AREAS, HEALTH, NOTES, OLDER, SECRET, SESSION, TOKENS, full, listNotes, note, refs, searchNotes, tagsOf } from './fixtures.ts'

const query = new URLSearchParams(window.location.search)

const json = (body: unknown, status = 200, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } })

// What the page changes is kept until it reloads. empty=1 starts with the personal area alone, no token and no note,
// many=1 adds older notes, so a list holds more than it shows, modules=a,b switches those modules on, and backup=old or
// backup=none ages or drops the last backup.
const empty = query.get('empty') === '1'
const areas = empty ? AREAS.filter((a) => a.kind === 'personal') : AREAS
const tokens = empty ? [] : TOKENS
const notes = empty ? [] : query.get('many') === '1' ? [...NOTES, ...OLDER] : NOTES
const backups: Record<string, string | null> = { old: new Date(Date.now() - 3 * 24 * 60 * 60_000).toISOString(), none: null }
const session = {
  ...SESSION,
  modules: query.get('modules')?.split(',') ?? SESSION.modules,
  last_backup: (query.get('backup') ?? '') in backups ? backups[query.get('backup')!] : SESSION.last_backup,
}

// login=wrong refuses every sign-in, and login=wait makes it wait 28 seconds.
function answer(method: string, url: URL, body: Record<string, unknown>, file?: Blob): Response | Promise<Response> {
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
  if (path.startsWith('/api/notes')) return answerNotes(method, url, body, file)
  if (method !== 'GET') return json({ error: 'not found' }, 404)
  if (path === '/api/health') return json(HEALTH)
  if (path === '/api/session') return json(session)
  if (path === '/api/tokens') return json({ tokens })
  if (path === '/api/search') {
    const hits = searchNotes(notes, url.searchParams.get('q')!, url.searchParams.get('area'))
    return json({ groups: hits.length ? [{ module: 'notes', hits }] : [] })
  }
  if (path === '/api/areas') return json({ areas: areas.filter((a) => url.searchParams.get('archived') === 'true' || !a.archived) })
  return json({ error: 'not found' }, 404)
}

// save=moved refuses the first save of each note's text, as a client had saved the note three minutes before.
const moved = new Set<number>()

// The notes routes, as the server answers them, telling the stream of every write.
// upload=wait holds every upload, so it stays under way.
function answerNotes(method: string, url: URL, body: Record<string, unknown>, file?: Blob): Response | Promise<Response> {
  const told = (n: Note) => {
    tell({ module: 'notes', kind: 'note', id: n.id, area: areas.find((a) => a.slug === n.area)?.id ?? 0 })
    return full(notes, n)
  }
  const changed = (n: Note) => told(Object.assign(n, { revision: n.revision + 1, source: 'web', changed_at: new Date().toISOString() }))
  if (url.pathname === '/api/notes' && method === 'GET') return json(listNotes(notes, url.searchParams))
  if (url.pathname === '/api/notes/tags' && method === 'GET') return json(tagsOf(notes))
  if (url.pathname === '/api/notes/limits' && method === 'GET') return json({ file_max_bytes: 15 << 20 })
  if (url.pathname === '/api/notes' && method === 'POST') {
    const n = note(Math.max(0, ...notes.map((x) => x.id)) + 1, 0, {
      revision: 0,
      body: String(body.body),
      kind: body.kind === 'idea' ? 'idea' : 'note',
      area: body.inbox || !body.area ? 'personal' : String(body.area),
      inbox: !!body.inbox,
    })
    notes.push(n)
    return json(changed(n), 201)
  }
  const [, id, part, fileID] = /^\/api\/notes\/(\d+)(?:\/(trash|files)(?:\/(\d+))?)?$/.exec(url.pathname) ?? []
  const n = notes.find((x) => x.id === Number(id))
  if (!n) return json({ error: 'not found' }, 404)
  if (part === 'files' && method === 'POST' && file) {
    if (query.get('upload') === 'wait') return new Promise(() => {})
    const f = { id: Math.max(0, ...notes.flatMap((x) => x.files.map((y) => y.id))) + 1, name: url.searchParams.get('name')!, type: file.type || 'application/octet-stream', size: file.size }
    n.files.push(f)
    n.file_count++
    told(n)
    return json(f, 201)
  }
  if (part === 'files' && method === 'DELETE') {
    n.files = n.files.filter((f) => f.id !== Number(fileID))
    n.file_count = n.files.length
    told(n)
    return new Response(null, { status: 204 })
  }
  if (part) {
    if (method === 'POST' && part === 'trash') return json(told(Object.assign(n, { trashed_at: new Date().toISOString() })))
    return json({ error: 'not found' }, 404)
  }
  if (method === 'GET') return json(full(notes, n))
  if (method !== 'PATCH') return json({ error: 'not found' }, 404)
  if (n.trashed_at) return json({ error: 'the note is in the trash, and is restored before it is changed: conflict' }, 409)
  if (query.get('save') === 'moved' && 'body' in body && !moved.has(n.id)) {
    moved.add(n.id)
    const at = new Date(Date.now() - 3 * 60_000).toISOString()
    told(Object.assign(n, { revision: n.revision + 1, source: 'portal', changed_at: at, body: n.body + '\n\nPortal reads the marks from its own configuration.' }))
  }
  if (body.revision !== n.revision) return json({ error: `note ${n.id} is at revision ${n.revision}, and the change was made from ${body.revision}: conflict` }, 409)
  const { revision: _, ...change } = body
  if (Array.isArray(change.links)) change.links = refs(...(change.links as string[]))
  // A change of area, kind or tags files a capture, unless it puts it in the inbox.
  if (['area', 'kind', 'tags'].some((k) => k in change) && !('inbox' in change)) n.inbox = false
  return json(changed(Object.assign(n, change)))
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
  const file = init?.body instanceof Blob ? init.body : undefined
  return Promise.resolve(answer(init?.method ?? 'GET', new URL(String(input), window.location.origin), body, file))
}

// A stream that opens and stays live, and tells of each change the harness makes.
const sources = new Set<LiveEventSource>()
class LiveEventSource {
  static CLOSED = 2
  readyState = 1
  onopen: (() => void) | null = null
  onmessage: ((m: { data: string }) => void) | null = null
  constructor() {
    sources.add(this)
    setTimeout(() => this.onopen?.(), 0)
  }
  close() {
    this.readyState = LiveEventSource.CLOSED
    sources.delete(this)
  }
}
function tell(change: object) {
  setTimeout(() => sources.forEach((s) => s.onmessage?.({ data: JSON.stringify(change) })), 0)
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

// click=TEXT clicks the first button, link or summary whose text, or label, starts with TEXT, type=TEXT types TEXT into
// the field that has the focus, and choose=N chooses an image of N bytes in the file picker of the note being read.
// Each waits until the page shows what it needs, and given more than once they run in order. end=1 then scrolls the
// page, and the note open beside a list, to their end.
function step(kind: string, text: string): boolean {
  if (kind === 'choose') {
    const picker = document.querySelector<HTMLInputElement>('.note-read input[type=file]')
    if (!picker) return false
    const chosen = new DataTransfer()
    chosen.items.add(new File([new Uint8Array(Number(text))], 'image.png', { type: 'image/png' }))
    picker.files = chosen.files
    picker.dispatchEvent(new Event('change', { bubbles: true }))
    return true
  }
  if (kind === 'type') {
    const field = document.activeElement
    if (!(field instanceof HTMLInputElement || field instanceof HTMLTextAreaElement)) return false
    // React hears a value set through the prototype's setter and then announced.
    Object.getOwnPropertyDescriptor(Object.getPrototypeOf(field), 'value')!.set!.call(field, text)
    field.dispatchEvent(new Event('input', { bubbles: true }))
    return true
  }
  const target = [...document.querySelectorAll<HTMLElement>('button:enabled, a, summary')].find((e) => (e.textContent || e.getAttribute('aria-label'))?.startsWith(text))
  target?.click()
  target?.scrollIntoView()
  return !!target
}
void (async () => {
  for (const [kind, text] of [...query].filter(([k]) => ['click', 'type', 'choose'].includes(k))) {
    for (let tries = 0; tries < 20; tries++) {
      await new Promise((r) => setTimeout(r, 100))
      if (step(kind, text)) break
    }
  }
  if (query.get('end') !== '1') return
  await new Promise((r) => setTimeout(r, 300))
  for (const scrolled of document.querySelectorAll('main, .panes > .box')) scrolled.scrollTo(0, scrolled.scrollHeight)
})()

createRoot(document.getElementById('root')!).render(
  <RouterProvider
    router={createMemoryRouter(
      [
        {
          path: '*',
          element: (
            <>
              <Apply />
              <App />
            </>
          ),
        },
      ],
      { initialEntries: [query.get('path') ?? '/'] },
    )}
  />,
)
