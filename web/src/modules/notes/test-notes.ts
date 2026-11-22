import type { Area, Session } from '../../api/types.ts'
import { listNotes, note, tagsOf } from '../../dev/fixtures.ts'
import { fakeServer, json, type Call } from '../../test-server.ts'
import type { Note } from './api.ts'

export const areas: Area[] = [
  { id: 1, kind: 'personal', name: 'Personal', slug: 'personal', description: '', archived: false },
  { id: 2, kind: 'project', name: 'garden', slug: 'garden', description: '', archived: false },
]

// A capture that waits in the inbox, made minutes ago.
export const capture = (id: number, minutes: number, n: Partial<Note> = {}) => note(id, minutes, { inbox: true, body: `capture ${id}`, ...n })

// A server with the given modules on that keeps the notes it is given, and lists them and their tags as the server
// does; answer, when given, answers first, or late with a promise.
export function notesServer(notes: Note[], answer: (c: Call) => Response | Promise<Response> | undefined = () => undefined, modules = ['notes']) {
  const session: Session = { user: { name: 'ada' }, modules, version: '1.2.3', timezone: 'UTC', last_backup: null }
  return fakeServer((c) => {
    const a = answer(c)
    if (a) return a
    if (c.path === '/api/session') return json(session)
    if (c.path === '/api/areas' || c.path === '/api/areas?archived=true') return json({ areas })
    if (c.method !== 'GET') return
    const url = new URL(c.path, 'http://here')
    if (url.pathname === '/api/notes') return json(listNotes(notes, url.searchParams))
    if (url.pathname === '/api/notes/tags') return json(tagsOf(notes))
    const n = notes.find((x) => url.pathname === `/api/notes/${x.id}`)
    if (n) return json(n)
  })
}
