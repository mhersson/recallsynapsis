import type { Area, Session } from '../../api/types.ts'
import { full, history, listNotes, listed, note, tagsOf } from '../../dev/fixtures.ts'
import { fakeServer, json, type Call } from '../../test-server.ts'
import type { Note } from './api.ts'

export const areas: Area[] = [
  { id: 1, kind: 'personal', name: 'Personal', slug: 'personal', brief: '', brief_revision: 0, brief_changed_at: null, brief_changed_by: '', archived: false },
  { id: 2, kind: 'project', name: 'garden', slug: 'garden', brief: '', brief_revision: 0, brief_changed_at: null, brief_changed_by: '', archived: false },
]

// A capture that waits in the inbox, made minutes ago.
export const capture = (id: number, minutes: number, n: Partial<Note> = {}) => note(id, minutes, { inbox: true, body: `capture ${id}`, ...n })

// A server with the given modules on that keeps the notes it is given, and lists them, their tags and each with its
// links and its history as the server does, with a cap of 15 MiB on a file; answer, when given, answers first, or late with a promise.
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
    if (url.pathname === '/api/notes/limits') return json({ file_max_bytes: 15 << 20 })
    const n = notes.find((x) => url.pathname === `/api/notes/${x.id}`)
    if (n) return json(full(notes, n))
    const h = notes.find((x) => url.pathname === `/api/notes/${x.id}/revisions`)
    if (h) return json(listed(history(h)))
  })
}
