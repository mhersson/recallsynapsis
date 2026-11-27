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
import type { Wording } from '../pages/Projects/Brief.tsx'
import { fieldsOf, type Decision, type Fields, type Line } from '../modules/knowledge/api.ts'
import type { Note, Version } from '../modules/notes/api.ts'
import { AREAS, DECIDED_TODAY, DECISIONS, DONE, HEALTH, KINDS, NOTES, NOW, OLDER, OLDER_DONE, SECRET, SESSION, TASKS, TOKENS, TRASHED, answerTasks, decisionOf, full, history, listNotes, listed, note, onToday, refs, searchDecisions, searchNotes, searchTasks, tagsOf, version, wordings } from './fixtures.ts'

const query = new URLSearchParams(window.location.search)

const json = (body: unknown, status = 200, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } })

// What the page changes is kept until it reloads. empty=1 starts with the personal area alone, no token, no note and no
// task, brief=none starts every area with no brief, many=1 adds older notes and done tasks, so a list holds more than it
// shows, today=none leaves out the tasks Today would show, modules=a,b switches those modules on, and backup=old or
// backup=none ages or drops the last backup.
const empty = query.get('empty') === '1'
const areas = (empty ? AREAS.filter((a) => a.kind === 'personal') : AREAS).map((a) =>
  query.get('brief') === 'none' ? { ...a, brief: '', brief_revision: 0, brief_changed_at: null, brief_changed_by: '' } : a,
)
const tokens = empty ? [] : TOKENS
const many = query.get('many') === '1'
const notes = empty ? [] : many ? [...NOTES, ...OLDER] : NOTES
const today = query.get('today') === 'none' ? onToday(TASKS, NOW) : []
const tasks = empty ? [] : [...TASKS.filter((t) => !today.includes(t.id)), ...DONE, ...(many ? OLDER_DONE : []), ...TRASHED]
const backups: Record<string, string | null> = { old: new Date(Date.now() - 3 * 24 * 60 * 60_000).toISOString(), none: null }
const session = {
  ...SESSION,
  modules: query.get('modules')?.split(',') ?? SESSION.modules,
  last_backup: (query.get('backup') ?? '') in backups ? backups[query.get('backup')!] : SESSION.last_backup,
}
// A link to a decision leads nowhere while knowledge is off, as the server resolves it.
const decisions = session.modules.includes('knowledge') && !empty ? DECISIONS : []

// push=off, on, failed, home, refused or none makes this browser one with Web Push that is off, on, or on with its last
// notification failed, an iPhone's Safari tab, one that refuses notifications, or one with no Web Push. Two other
// browsers are on, but for push=off, which has none. The state is set on every load, as one profile serves many pages.
const push = query.get('push')
const browsers = !push || push === 'off' ? [] : ['on', 'failed'].includes(push) ? [1, 2, 3] : [2, 3]
if (push) {
  let subscribed = browsers.includes(1)
  const subscription = {
    endpoint: 'https://push.example/harness',
    toJSON: () => ({ endpoint: 'https://push.example/harness', keys: { p256dh: 'P', auth: 'A' } }),
    unsubscribe: async () => {
      subscribed = false
      return true
    },
  }
  const subscribe = async () => {
    subscribed = true
    return subscription
  }
  const registration = { pushManager: { getSubscription: async () => (subscribed ? subscription : null), subscribe } }
  const worker = { register: async () => registration, getRegistration: async () => registration, ready: Promise.resolve(registration) }
  Object.defineProperty(navigator, 'serviceWorker', { value: worker, configurable: true })
  const notification = { permission: push === 'refused' ? 'denied' : subscribed ? 'granted' : 'default', requestPermission: async () => (notification.permission = 'granted') }
  Object.defineProperty(window, 'Notification', { value: notification, configurable: true })
  if (push === 'home' || push === 'none') Reflect.deleteProperty(window, 'PushManager')
  if (push === 'home') Object.defineProperty(navigator, 'standalone', { value: false, configurable: true })
  if (subscribed) localStorage.setItem('push_browser', '1')
  else localStorage.removeItem('push_browser')
}

// The push routes, telling the stream of every change of a browser. A browser turned on is 4, then 5 and so on.
function answerPush(method: string, url: URL): Response {
  const told = () => {
    tell({ module: 'push', kind: 'browser', id: 0, area: 0 })
    return new Response(null, { status: 204 })
  }
  if (url.pathname === '/api/push' && method === 'GET') {
    const id = Number(url.searchParams.get('browser'))
    const failed = push === 'failed' && id === 1 ? { failed_at: new Date(new Date().setHours(14, 0, 0, 0)).toISOString(), failure: 'web.push.apple.com was not reached' } : { failed_at: null, failure: '' }
    return json({ key: 'BHarness-key_00', on: browsers.length, browser: browsers.includes(id) ? { id, ...failed } : null })
  }
  if (url.pathname === '/api/push' && method === 'PUT') {
    const id = Math.max(3, ...browsers) + 1
    browsers.push(id)
    told()
    return json({ id })
  }
  if (url.pathname === '/api/push' && method === 'DELETE') {
    browsers.length = 0
    return told()
  }
  const at = browsers.indexOf(Number(/^\/api\/push\/(\d+)$/.exec(url.pathname)?.[1]))
  if (method !== 'DELETE' || at < 0) return json({ error: 'not found' }, 404)
  browsers.splice(at, 1)
  return told()
}

// The kinds of notification, telling the stream of every change; notify=refused refuses every change, as a server that
// cannot keep them.
const kinds = { ...KINDS }
function answerNotify(method: string, body: Record<string, unknown>): Response {
  if (method === 'PATCH') {
    if (query.get('notify') === 'refused') return json({ error: 'internal error' }, 500)
    Object.assign(kinds, body)
    tell({ module: 'tasks', kind: 'notify', id: 0, area: 0 })
  }
  return json(kinds)
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
    const brief = String(body.brief ?? '')
    const a: Area = {
      id: areas.length + 10,
      kind: 'project',
      name,
      slug: name.toLowerCase(),
      brief,
      ...(brief ? { brief_revision: 1, brief_changed_at: new Date().toISOString(), brief_changed_by: 'web' } : { brief_revision: 0, brief_changed_at: null, brief_changed_by: '' }),
      archived: false,
    }
    areas.push(a)
    tell({ module: 'core', kind: 'area', id: a.id, area: a.id })
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
  if (path.startsWith('/api/areas/')) return answerArea(method, path, body)
  if (path.startsWith('/api/notes')) return answerNotes(method, url, body, file)
  if (path.startsWith('/api/push')) return answerPush(method, url)
  if (path.startsWith('/api/knowledge') && session.modules.includes('knowledge')) return answerKnowledge(method, url, body)
  if (path === '/api/tasks/notify') return answerNotify(method, body)
  if (path.startsWith('/api/tasks')) {
    const [status, answered] = answerTasks(
      tasks,
      method,
      url,
      body,
      NOW,
      (t) => tell(t ? { module: 'tasks', kind: 'task', id: t.id, area: areas.find((a) => a.slug === t.area)?.id ?? 0 } : { module: 'tasks', kind: 'trash', id: 0, area: 0 }),
      notes,
      decisions,
    )
    return status === 204 ? new Response(null, { status }) : json(answered, status)
  }
  if (method !== 'GET') return json({ error: 'not found' }, 404)
  if (path === '/api/health') return json(HEALTH)
  if (path === '/api/session') return json(session)
  if (path === '/api/tokens') return json({ tokens })
  if (path === '/api/search') {
    // The server groups the hits in its own order of the modules, the notes first.
    const q = url.searchParams.get('q')!
    const groups = [
      { module: 'notes', hits: searchNotes(notes, q, url.searchParams.get('area')) },
      { module: 'tasks', hits: searchTasks(tasks, q, url.searchParams.get('area')) },
      { module: 'knowledge', hits: searchDecisions(decisions, q, url.searchParams.get('area')) },
    ]
    return json({ groups: groups.filter((g) => session.modules.includes(g.module) && g.hits.length) })
  }
  if (path === '/api/areas') return json({ areas: areas.filter((a) => url.searchParams.get('archived') === 'true' || !a.archived) })
  return json({ error: 'not found' }, 404)
}

// The knowledge routes, as the server answers them: a project's decisions, one in full, a package, and a delete, telling
// the stream of each decision written or deleted.
function answerKnowledge(method: string, url: URL, body: Record<string, unknown>): Response {
  const d = decisions.find((x) => url.pathname === `/api/knowledge/${x.id}`)
  if (method === 'GET' && d) return json(decisionOf(d, notes, tasks))
  if (method === 'DELETE' && d) {
    decisions.splice(decisions.indexOf(d), 1)
    relink()
    told(d)
    return new Response(null, { status: 204 })
  }
  const named = (method === 'GET' ? url.searchParams.get('area') : String(body.area))?.toLowerCase() ?? ''
  const area = areas.find((a) => [a.slug, a.name.toLowerCase()].includes(named))
  if (method === 'GET' && url.pathname === '/api/knowledge') {
    if (!area) return json({ error: 'not found' }, 404)
    return json({ decisions: decisions.filter((x) => x.area === area.slug), today: DECIDED_TODAY })
  }
  if (method !== 'POST' || url.pathname !== '/api/knowledge') return json({ error: 'not found' }, 404)
  if (!area) return json({ error: `area ${named}: not found` }, 404)
  // An entry of add without a day is decided today.
  const add = ((body.add ?? []) as Fields[]).map((f) => ({ ...f, decided: f.decided || DECIDED_TODAY }))
  return answerPackage(area, add, (body.mend ?? []) as Mend[])
}

type Mend = Partial<Fields> & { ref: string; revision: number }

// The decisions save=moved has mended as a client would have.
const movedDecisions = new Set<number>()

// A package that mends decisions of the project and adds to it, refused whole on a decision that moved on or a text the
// server refuses. save=moved refuses the first mend of each decision, as if a client had mended it three minutes before.
function answerPackage(area: Area, add: Fields[], mend: Mend[]): Response {
  const of = (ref?: string) => decisions.find((x) => x.ref === ref)
  for (const [i, m] of mend.entries()) {
    const d = of(m.ref)
    if (!d) return json({ error: `entry ${i + 1} of mend: decision ${m.ref}: not found` }, 404)
    if (query.get('save') === 'moved' && !movedDecisions.has(d.id)) {
      movedDecisions.add(d.id)
      Object.assign(d, { why: `${d.why} Confirmed in review.`, revision: d.revision + 1, changed_at: new Date(Date.now() - 3 * 60_000).toISOString(), changed_by: 'aim' })
      told(d)
    }
    if (m.revision !== d.revision) return json({ error: `entry ${i + 1} of mend: ${m.ref} is at revision ${d.revision}, and the mend was made from ${m.revision}: conflict` }, 409)
    if (refusal({ ...fieldsOf(d), ...m })) return json({ error: `entry ${i + 1} of mend: ${refusal({ ...fieldsOf(d), ...m })}: invalid` }, 400)
  }
  const wrong = add.map(refusal).findIndex(Boolean)
  if (wrong >= 0) return json({ error: `entry ${wrong + 1} of add: ${refusal(add[wrong])}: invalid` }, 400)
  const at = new Date().toISOString()
  const mended = mend.map((m) => {
    const d = of(m.ref)!
    const f = { ...fieldsOf(d), ...m }
    return Object.assign(d, texts(f), { decided: f.decided, replaces: of(f.replaces) ?? null, revision: d.revision + 1, changed_at: at, changed_by: 'web' })
  })
  const added = add.map((f): Decision => {
    const id = nextDecision++
    return { id, ref: `knowledge:${id}`, area: area.slug, ...texts(f), decided: f.decided, state: 'stands', replaces: of(f.replaces) ?? null, replaced_by: null, revision: 1, created_at: at, changed_at: at, changed_by: 'web' }
  })
  decisions.push(...added)
  relink()
  const saved = [...mended, ...added]
  saved.forEach(told)
  return json({ decisions: saved })
}

// Tells the stream of a decision written or deleted.
const told = (d: Decision) => tell({ module: 'knowledge', kind: 'decision', id: d.id, area: areas.find((a) => a.slug === d.area)?.id ?? 0 })

// A decision's id is never used again, so a link to a deleted one leads to no other.
let nextDecision = Math.max(0, ...DECISIONS.map((d) => d.id)) + 1

// A decision's texts as the server keeps them, without the space around them.
const texts = (f: Fields) => ({ decision: f.decision.trim(), why: f.why.trim(), source: (f.source ?? '').trim() })

// What the server refuses of a decision's texts, as its line says it, or ''.
function refusal(f: Fields): string {
  const { decision, why, source } = texts(f)
  const n = (s: string) => [...s].length
  if (!decision || n(decision) > 200 || /\p{Cc}/u.test(decision)) return 'a decision is one line of 1 to 200 characters'
  if (!why || n(why) > 2000 || /[^\P{Cc}\n\t]/u.test(why)) return 'a why is 1 to 2000 characters, with no control characters but newlines and tabs'
  if (n(source) > 500 || /\p{Cc}/u.test(source)) return 'a source is one line of at most 500 characters'
  if (!/^\d{4}-\d{2}-\d{2}$/.test(f.decided)) return `decided "${f.decided}" is not a date, YYYY-MM-DD`
  return ''
}

// Gives each decision the line of the one it replaces and of the one that replaced it, which it then stands or is
// replaced by, and lists them as the server does: the newest day first, then the most recently made.
function relink() {
  const line = (x: Decision): Line => ({ ref: x.ref, decision: x.decision, decided: x.decided })
  for (const x of decisions) Object.assign(x, { state: 'stands', replaced_by: null })
  for (const x of decisions) {
    const older = x.replaces && decisions.find((y) => y.ref === x.replaces!.ref)
    x.replaces = older ? line(older) : null
    if (older) Object.assign(older, { state: 'replaced', replaced_by: line(x) })
  }
  decisions.sort((a, b) => b.decided.localeCompare(a.decided) || Date.parse(b.created_at) - Date.parse(a.created_at) || b.id - a.id)
}

// Each area's wordings of its brief, oldest first, from when the page first asks of the area, and the areas whose brief
// save=moved has moved.
const briefs = new Map<number, Wording[]>()
const movedBriefs = new Set<number>()
const wordingsOf = (a: Area) => briefs.get(a.id) ?? briefs.set(a.id, wordings(a)).get(a.id)!

// An area's routes, as the server answers them, telling the stream of every change. A brief, changed or restored to
// another text, is the next revision, by the web app. save=moved refuses the first save of each brief, as a client had
// saved it three minutes before.
function answerArea(method: string, path: string, body: Record<string, unknown>): Response {
  const [, id, revisions, sub, restore] = /^\/api\/areas\/(\d+)(\/revisions(?:\/(\d+)(\/restore)?)?)?$/.exec(path) ?? []
  const a = areas.find((x) => x.id === Number(id))
  if (!a) return json({ error: 'not found' }, 404)
  const kept = wordingsOf(a)
  const told = (brief?: string, restored_from: number | null = null) => {
    if (brief !== undefined && brief !== a.brief) {
      Object.assign(a, { brief, brief_revision: a.brief_revision + 1, brief_changed_at: new Date().toISOString(), brief_changed_by: 'web' })
      kept.push({ revision: a.brief_revision, at: a.brief_changed_at!, by: 'web', said: '', restored_from, brief })
    }
    tell({ module: 'core', kind: 'area', id: a.id, area: a.id })
    return json(a)
  }
  if (method === 'PATCH' && !revisions) {
    const { brief, brief_revision: _, ...change } = body
    if (typeof brief !== 'string') {
      Object.assign(a, change)
      return told()
    }
    if ([...brief.trim()].length > 4000) return json({ error: 'a brief is at most 4000 characters, with no control characters but newlines and tabs: invalid' }, 400)
    if (query.get('save') === 'moved' && !movedBriefs.has(a.id)) {
      movedBriefs.add(a.id)
      Object.assign(a, { brief: `${a.brief}\n\nKept up to date by the coding sessions.`.trim(), brief_revision: a.brief_revision + 1, brief_changed_at: new Date(Date.now() - 3 * 60_000).toISOString(), brief_changed_by: 'aim' })
      kept.push({ revision: a.brief_revision, at: a.brief_changed_at!, by: 'aim', said: '', restored_from: null, brief: a.brief })
      tell({ module: 'core', kind: 'area', id: a.id, area: a.id })
    }
    if (body.brief_revision !== a.brief_revision) return json({ error: `the brief is at revision ${a.brief_revision}, and the change was made from ${body.brief_revision}: conflict` }, 409)
    Object.assign(a, change)
    return told(brief.trim())
  }
  const w = kept.find((x) => x.revision === Number(sub))
  if (method === 'GET' && revisions && !sub) return json({ revisions: kept.map(({ brief: _, ...e }) => e).reverse() })
  if (!w) return json({ error: 'not found' }, 404)
  if (method === 'GET' && !restore) return json(w)
  if (method === 'POST' && restore) return told(w.brief, w.revision)
  return json({ error: 'not found' }, 404)
}

// save=moved refuses the first save of each note's text, as a client had saved the note three minutes before.
const moved = new Set<number>()

// Each note's revisions, oldest first, from when the page first asks of the note.
const versions = new Map<number, Version[]>()
const historyOf = (n: Note) => versions.get(n.id) ?? versions.set(n.id, history(n)).get(n.id)!

// The notes routes, as the server answers them, telling the stream of every write.
// upload=wait holds every upload, so it stays under way.
function answerNotes(method: string, url: URL, body: Record<string, unknown>, file?: Blob): Response | Promise<Response> {
  const told = (n: Note) => {
    tell({ module: 'notes', kind: 'note', id: n.id, area: areas.find((a) => a.slug === n.area)?.id ?? 0 })
    return full(notes, n, tasks, decisions)
  }
  const changed = (n: Note) => {
    Object.assign(n, { revision: n.revision + 1, source: 'web', changed_at: new Date().toISOString() })
    versions.get(n.id)?.push(version(n, n.revision))
    return told(n)
  }
  if (url.pathname === '/api/notes' && method === 'GET') return json(listNotes(notes, url.searchParams))
  if (url.pathname === '/api/notes/tags' && method === 'GET') return json(tagsOf(notes))
  if (url.pathname === '/api/notes/limits' && method === 'GET') return json({ file_max_bytes: 15 << 20 })
  if (url.pathname === '/api/notes/trash' && method === 'DELETE') {
    for (const n of notes.filter((x) => x.trashed_at)) notes.splice(notes.indexOf(n), 1)
    tell({ module: 'notes', kind: 'trash', id: 0, area: 0 })
    return new Response(null, { status: 204 })
  }
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
  const [, id, part, sub, restore] = /^\/api\/notes\/(\d+)(?:\/(trash|restore|files|revisions)(?:\/(\d+)(\/restore)?)?)?$/.exec(url.pathname) ?? []
  const n = notes.find((x) => x.id === Number(id))
  if (!n) return json({ error: 'not found' }, 404)
  const revisions = historyOf(n)
  if (part === 'revisions') {
    const v = revisions.find((r) => r.revision === Number(sub))
    if (!sub && method === 'GET') return json(listed(revisions))
    if (!v) return json({ error: 'not found' }, 404)
    if (method === 'GET' && !restore) return json(v)
    if (n.trashed_at) return json({ error: 'the note is in the trash, and is restored before it is changed: conflict' }, 409)
    const answer = changed(Object.assign(n, { title: v.title, body: v.body, tags: v.tags, links: refs(...v.links) }))
    revisions[revisions.length - 1].restored_from = v.revision
    return json(answer)
  }
  if (part === 'files' && method === 'POST' && file) {
    if (query.get('upload') === 'wait') return new Promise(() => {})
    const f = { id: Math.max(0, ...notes.flatMap((x) => x.files.map((y) => y.id))) + 1, name: url.searchParams.get('name')!, type: file.type || 'application/octet-stream', size: file.size }
    n.files.push(f)
    n.file_count++
    told(n)
    return json(f, 201)
  }
  if (part === 'files' && method === 'DELETE') {
    n.files = n.files.filter((f) => f.id !== Number(sub))
    n.file_count = n.files.length
    told(n)
    return new Response(null, { status: 204 })
  }
  if (part) {
    if (method === 'POST' && part === 'trash') return json(told(Object.assign(n, { trashed_at: new Date().toISOString() })))
    if (method === 'POST' && part === 'restore') return json(told(Object.assign(n, { trashed_at: null })))
    return json({ error: 'not found' }, 404)
  }
  if (method === 'GET') return json(full(notes, n, tasks, decisions))
  if (method === 'DELETE') {
    if (!n.trashed_at) return json({ error: `note ${n.id} is not in the trash, and only a note in the trash is deleted: conflict` }, 409)
    notes.splice(notes.indexOf(n), 1)
    told(n)
    return new Response(null, { status: 204 })
  }
  if (method !== 'PATCH') return json({ error: 'not found' }, 404)
  if (n.trashed_at) return json({ error: 'the note is in the trash, and is restored before it is changed: conflict' }, 409)
  if (query.get('save') === 'moved' && 'body' in body && !moved.has(n.id)) {
    moved.add(n.id)
    const at = new Date(Date.now() - 3 * 60_000).toISOString()
    Object.assign(n, { revision: n.revision + 1, source: 'portal', changed_at: at, body: n.body + '\n\nPortal reads the marks from its own configuration.' })
    revisions.push(version(n, n.revision))
    told(n)
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

// click=TEXT clicks the first button, link, summary or label whose text, or label, starts with TEXT, in the dialog open
// last when one is, as a person can reach nothing else then, type=TEXT types TEXT into the field that has the focus,
// and choose=N chooses an image of N bytes in the file picker of the note being read.
// Each waits until the page shows what it needs, and given more than once they run in order. end=1 then scrolls the
// page, and what is open beside a list, to their end.
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
  const scope = [...document.querySelectorAll('dialog[open]')].pop() ?? document
  const target = [...scope.querySelectorAll<HTMLElement>('button:enabled, a, summary, label')].find((e) => (e.textContent || e.getAttribute('aria-label'))?.startsWith(text))
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
