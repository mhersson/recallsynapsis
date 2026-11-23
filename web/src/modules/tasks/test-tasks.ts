import type { Area, Session } from '../../api/types.ts'
import { answerTasks } from '../../dev/fixtures.ts'
import { fakeServer, json, type Call } from '../../test-server.ts'
import type { Now, Task } from './api.ts'

export const areas: Area[] = [
  { id: 1, kind: 'personal', name: 'Personal', slug: 'personal', description: '', archived: false },
  { id: 2, kind: 'project', name: 'garden', slug: 'garden', description: '', archived: false },
]

// The server's today: a Monday, in a week that begins on a Monday.
export const monday: Now = { date: '2026-11-23', weekday: 'Monday', time: '10:00', timezone: 'UTC', week_start: 'monday' }

// A server with the given modules on that keeps the tasks it is given and answers the tasks routes as the server does,
// on the day now gives; answer, when given, answers first, or late with a promise.
export function tasksServer(tasks: Task[], now: Now = monday, answer: (c: Call) => Response | Promise<Response> | undefined = () => undefined, modules = ['tasks']) {
  const session: Session = { user: { name: 'ada' }, modules, version: '1.2.3', timezone: 'UTC', last_backup: null }
  return fakeServer((c) => {
    const a = answer(c)
    if (a) return a
    if (c.path === '/api/session') return json(session)
    if (c.path.startsWith('/api/areas')) return json({ areas })
    if (!c.path.startsWith('/api/tasks')) return
    const [status, body] = answerTasks(tasks, c.method, new URL(c.path, 'http://here'), (c.body ?? {}) as Record<string, unknown>, now, () => {})
    return json(body, status)
  })
}
