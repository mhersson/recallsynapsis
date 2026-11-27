import type { Area, Linked, Session } from '../../api/types.ts'
import { fakeServer, json, type Call } from '../../test-server.ts'
import type { Decision, Fields, Full, Line } from './api.ts'

// The day the server gives as today, which no clock the tests run on shows.
export const TODAY = '2026-11-24'

export const session = (modules = ['notes', 'tasks', 'knowledge']): Session => ({ user: { name: 'ada' }, modules, version: '1.2.3', timezone: 'UTC', last_backup: null })

const area = (id: number, kind: Area['kind'], name: string): Area => ({
  id,
  kind,
  name,
  slug: name,
  brief: 'what it is',
  brief_revision: 1,
  brief_changed_at: '2026-11-20T10:00:00Z',
  brief_changed_by: 'web',
  archived: false,
})

export const areas: Area[] = [area(1, 'personal', 'Personal'), area(2, 'project', 'garden'), area(3, 'project', 'shed')]

// A decision of the garden that stands, decided on the day given, unless d says otherwise.
export const decision = (id: number, decided: string, d: Partial<Full> = {}): Full => ({
  id,
  ref: `knowledge:${id}`,
  area: 'garden',
  decision: `decision ${id}`,
  why: `why ${id}`,
  source: '',
  decided,
  state: 'stands',
  replaces: null,
  replaced_by: null,
  revision: 1,
  created_at: `${decided}T10:00:00Z`,
  changed_at: `${decided}T10:00:00Z`,
  changed_by: 'web',
  linked_from: [],
  ...d,
})

export const line = (d: Decision): Line => ({ ref: d.ref, decision: d.decision, decided: d.decided })

// A decision as a list gives it.
export const row = ({ linked_from: _, ...d }: Full): Decision => d

// Makes newer replace older in decisions, as the server tells both.
export function replace(newer: Full, older: Full) {
  newer.replaces = line(older)
  older.replaced_by = line(newer)
  older.state = 'replaced'
}

export const linked = (ref: string, kind: string, state: string): Linked => ({ ref, kind, state, title: `${kind} ${ref}`, available: true })

// Makes d replace the decision ref names, or nothing for '', as the server tells both.
function relate(decisions: Full[], d: Full, ref: string) {
  const was = decisions.find((x) => x.ref === d.replaces?.ref)
  if (was) Object.assign(was, { state: 'stands', replaced_by: null })
  d.replaces = null
  const target = decisions.find((x) => x.ref === ref)
  if (target) replace(d, target)
}

type Entry = Partial<Fields> & { ref?: string; revision?: number }

// A package as the server applies it: mends from the decision's revision, then adds, the newest first, and none of them
// with an empty why.
function apply(decisions: Full[], { add = [], mend = [] }: { add?: Entry[]; mend?: Entry[] }) {
  if ([...add, ...mend].some((e) => e.why?.trim() === '')) return json({ error: 'entry 1: a why is 1 to 2000 characters: invalid' }, 400)
  const out: Full[] = []
  for (const { ref, revision, replaces, ...f } of mend) {
    const d = decisions.find((x) => x.ref === ref)
    if (!d) return json({ error: 'entry 1 of mend: not found' }, 404)
    if (d.revision !== revision) return json({ error: 'entry 1 of mend: moved on: conflict' }, 409)
    Object.assign(d, f, { revision: d.revision + 1, changed_by: 'web' })
    if (replaces !== undefined) relate(decisions, d, replaces)
    out.push(d)
  }
  for (const { replaces, ...f } of add) {
    const d = decision(Math.max(...decisions.map((x) => x.id)) + 1, f.decided || TODAY, f)
    decisions.unshift(d)
    if (replaces) relate(decisions, d, replaces)
    out.push(d)
  }
  return json({ decisions: out.map(row) })
}

// A server that keeps the decisions given, as the knowledge routes answer and change them; answer, when given, answers
// first.
export function keeper(decisions: Full[], answer: (c: Call) => Response | undefined = () => undefined) {
  return fakeServer((c) => {
    const a = answer(c)
    if (a) return a
    if (c.path === '/api/session') return json(session())
    if (c.path.startsWith('/api/areas')) return json({ areas })
    const area = /^\/api\/knowledge\?area=(.+)$/.exec(c.path)?.[1]
    if (area) return json({ decisions: decisions.filter((d) => d.area === area).map(row), today: TODAY })
    if (c.method === 'POST' && c.path === '/api/knowledge') return apply(decisions, c.body as { add?: Entry[]; mend?: Entry[] })
    const d = decisions.find((x) => c.path === `/api/knowledge/${x.id}`)
    if (d) return json(d)
    if (c.path.startsWith('/api/notes')) return json({ notes: [], total: 0 })
    if (c.path.startsWith('/api/tasks')) return json({ tasks: [], total: 0, now: { date: '2026-11-23', weekday: 'Monday', time: '10:00', timezone: 'UTC', week_start: 'monday' } })
  })
}
