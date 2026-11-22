import type { Area, Hit, Session, Token } from '../api/types.ts'
import type { Note, Row } from '../modules/notes/api.ts'

// What the API serves, in its shapes; the harness answers from it. Times are counted back from when the page loads.
const before = (minutes: number) => new Date(Date.now() - minutes * 60_000).toISOString()

export const HEALTH = { status: 'ok', version: '0.1.0' }

export const SESSION: Session = { user: { name: 'ada' }, modules: [], version: '0.1.0', timezone: 'Europe/Oslo', last_backup: before(6 * 60) }

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
    changed_at: before(2 * 60),
  },
  {
    ref: 'notes:40',
    kind: 'note',
    title: 'Restore drill: prove a backup restores every month',
    snippet: 'Start a server on last night’s backup and open the inbox.',
    area: 'personal',
    state: 'inbox',
    changed_at: before(13 * 60),
  },
  {
    ref: 'notes:31',
    kind: 'idea',
    title: 'Trigram search for Norwegian compound words',
    snippet: 'So backup finds backupjobben and garasjeport finds port.',
    area: 'recallsynapsis',
    state: 'filed',
    changed_at: before(2 * 24 * 60),
  },
  {
    ref: 'notes:7',
    kind: 'note',
    title: 'The backup job retries once',
    snippet: 'A failed backup is logged and the next night runs as usual.',
    area: 'cavu',
    state: 'filed',
    changed_at: before(5 * 24 * 60),
  },
]

// GET /api/tokens, oldest first.
export const TOKENS: Token[] = [
  { id: 4, name: 'old-shortcut', access: 'read', created_at: '2026-10-12T18:00:00Z', last_used_at: null },
  { id: 1, name: 'portal', access: 'write', created_at: '2026-11-02T10:00:00Z', last_used_at: before(4) },
  { id: 2, name: 'aim', access: 'write', created_at: '2026-11-05T12:00:00Z', last_used_at: before(2 * 60) },
  { id: 3, name: 'home-assistant', access: 'read', created_at: '2026-11-09T09:00:00Z', last_used_at: before(20 * 60) },
]

// What POST /api/tokens answers the one time it shows a token.
export const SECRET = 'recall_tV3n9QkZp2Lw8xRb5HcM1yJf7sGd4aEu6oNqX0iTzKe'

// A note of the personal area, changed minutes ago at its first revision, unless n says otherwise.
export const note = (id: number, minutes: number, n: Partial<Note>): Note => ({
  excerpt: (n.body ?? '').slice(0, 200),
  id,
  ref: `notes:${id}`,
  area: 'personal',
  kind: 'note',
  title: '',
  inbox: false,
  tags: [],
  file_count: 0,
  revision: 1,
  source: 'web',
  created_at: before(minutes),
  changed_at: before(minutes),
  trashed_at: null,
  body: '',
  said: '',
  ...n,
})

// A note as a list gives it.
export const row = ({ body: _, said: __, ...r }: Note): Row => r

// GET /api/notes/{id} for each note: the captures that wait in the inbox, then filed notes and ideas. A list gives the
// first 200 characters of the body as the excerpt.
export const NOTES: Note[] = [
  note(118, 12, {
    inbox: true,
    tags: ['homelab', 'backup'],
    source: 'portal',
    body: 'Check whether Longhorn can back up to the NAS over NFS.',
    said: 'Remind me to check if Longhorn can back up to the NAS.',
  }),
  note(117, 60, {
    inbox: true,
    source: 'portal',
    body: 'Husk å bestille time til EU-kontroll for bilen før desember.',
    said: 'Legg inn at jeg må bestille EU-kontroll før desember.',
  }),
  note(116, 3 * 60, { inbox: true, body: 'Let the idea bank bring up ideas nobody has touched for 90 days.' }),
  note(40, 13 * 60, {
    inbox: true,
    source: 'aim',
    title: 'Restore drill: prove a backup restores every month',
    body: 'Start a server on last night’s backup and open the inbox.',
  }),
  note(115, 26 * 60, { inbox: true, source: 'aim', body: 'The capture cap should count characters after trimming.' }),
  note(12, 2 * 60, {
    area: 'recallsynapsis',
    tags: ['approval', 'portal'],
    revision: 3,
    title: 'Approval belongs to the client',
    body: 'Portal asks before a tool writes; the server never waits for a person.',
  }),
  note(31, 2 * 24 * 60, {
    area: 'recallsynapsis',
    kind: 'idea',
    tags: ['search'],
    title: 'Trigram search for Norwegian compound words',
    body: 'So backup finds backupjobben and garasjeport finds port.',
  }),
  note(7, 5 * 24 * 60, { area: 'cavu', title: 'The backup job retries once', body: 'A failed backup is logged and the next night runs as usual.' }),
]
