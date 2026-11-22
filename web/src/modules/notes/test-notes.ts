import type { Area, Session } from '../../api/types.ts'
import { note, row } from '../../dev/fixtures.ts'
import { fakeServer, json, type Call } from '../../test-server.ts'
import type { Note } from './api.ts'

export const areas: Area[] = [
  { id: 1, kind: 'personal', name: 'Personal', slug: 'personal', description: '', archived: false },
  { id: 2, kind: 'project', name: 'garden', slug: 'garden', description: '', archived: false },
]

// A capture that waits in the inbox, made minutes ago.
export const capture = (id: number, minutes: number, n: Partial<Note> = {}) => note(id, minutes, { inbox: true, body: `capture ${id}`, ...n })

// A server with the given modules on that keeps the notes it is given and lists the inbox from them, the most recently
// changed first; answer, when given, answers first.
export function notesServer(notes: Note[], answer: (c: Call) => Response | undefined = () => undefined, modules = ['notes']) {
  const session: Session = { user: { name: 'ada' }, modules, version: '1.2.3', timezone: 'UTC', last_backup: null }
  return fakeServer((c) => {
    const a = answer(c)
    if (a) return a
    if (c.path === '/api/session') return json(session)
    if (c.path === '/api/areas') return json({ areas })
    const url = new URL(c.path, 'http://here')
    if (c.method === 'GET' && url.pathname === '/api/notes' && url.searchParams.get('inbox') === 'true') {
      const inbox = notes.filter((n) => n.inbox && !n.trashed_at).sort((a, b) => b.changed_at.localeCompare(a.changed_at))
      return json({ notes: inbox.slice(0, Number(url.searchParams.get('limit')) || 50).map(row), total: inbox.length })
    }
    const n = notes.find((x) => url.pathname === `/api/notes/${x.id}`)
    if (c.method === 'GET' && n) return json(n)
  })
}
