import type { Area, Hit, Session } from '../api/types.ts'

// What the API serves, in its shapes; the harness answers from it.
export const HEALTH = { status: 'ok', version: '0.1.0' }

export const SESSION: Session = { user: { name: 'ada' }, modules: [], version: '0.1.0', timezone: 'Europe/Oslo', last_backup: '2026-11-22T02:00:00Z' }

// GET /api/areas?archived=true: the personal area first, then the projects by name.
export const AREAS: Area[] = [
  { id: 1, kind: 'personal', name: 'Personal', slug: 'personal', description: 'Everything that belongs to no project.', archived: false },
  { id: 5, kind: 'project', name: 'aim', slug: 'aim', description: 'The terminal multiplexer the coding agents work in.', archived: false },
  { id: 3, kind: 'project', name: 'cavu', slug: 'cavu', description: 'Runs coding tasks in containers and asks when it needs me.', archived: false },
  { id: 6, kind: 'project', name: 'homelander', slug: 'homelander', description: 'The first home cluster, retired in October.', archived: true },
  { id: 4, kind: 'project', name: 'portal', slug: 'portal', description: 'Relays AI clients to the services at home.', archived: false },
  {
    id: 2,
    kind: 'project',
    name: 'recallsynapsis',
    slug: 'recallsynapsis',
    description: 'Notes, ideas, tasks and project knowledge, reachable from a web app and from AI clients.',
    archived: false,
  },
]

// What GET /api/search finds, as a module named notes would answer.
export const HITS: Hit[] = [
  {
    ref: 'notes:12',
    kind: 'note',
    title: 'Approval belongs to the client',
    snippet: 'Portal asks before a tool writes; the server never waits for a person.',
    area: 'recallsynapsis',
    state: 'filed',
    changed_at: '2026-11-22T06:10:00Z',
  },
  {
    ref: 'notes:40',
    kind: 'note',
    title: 'Restore drill: prove a backup restores every month',
    snippet: 'Start a server on last night’s backup and open the inbox.',
    area: 'personal',
    state: 'inbox',
    changed_at: '2026-11-21T19:30:00Z',
  },
  {
    ref: 'notes:31',
    kind: 'idea',
    title: 'Trigram search for Norwegian compound words',
    snippet: 'So backup finds backupjobben and garasjeport finds port.',
    area: 'recallsynapsis',
    state: 'filed',
    changed_at: '2026-11-20T11:00:00Z',
  },
  {
    ref: 'notes:7',
    kind: 'note',
    title: 'The backup job retries once',
    snippet: 'A failed backup is logged and the next night runs as usual.',
    area: 'cavu',
    state: 'filed',
    changed_at: '2026-11-18T09:45:00Z',
  },
]
