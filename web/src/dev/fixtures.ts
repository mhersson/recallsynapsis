import type { Area, Hit, Session, Token } from '../api/types.ts'
import type { Kind, List, Note, Row } from '../modules/notes/api.ts'

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
  links: [],
  linked_from: [],
  ...n,
})

// A note as a list gives it.
export const row = ({ body: _, said: __, links: ___, linked_from: ____, ...r }: Note): Row => r

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
    body: `Portal marks each tool as free or as needing a tap. Recall Synapsis only has to split its tools by risk.

## What follows

- capture, list and read tools can be free
- knowledge changes always meet the Allow card
- one call carries the whole change, all or nothing

> "Always allow" removes the tap. Accepted.

\`update_knowledge\` is never listed under \`free\`.`,
  }),
  note(31, 5 * 60, {
    area: 'recallsynapsis',
    kind: 'idea',
    tags: ['search'],
    title: 'Trigram search for Norwegian compound words',
    body: 'So backup finds backupjobben and garasjeport finds port.',
  }),
  note(22, 26 * 60, { tags: ['garage'], title: 'Garage shelving layout', body: 'Left wall 240 cm, right wall 185 cm up to the window.' }),
  note(25, 27 * 60, { area: 'aim', kind: 'idea', tags: ['agents'], title: 'A context pack when an agent session starts', body: 'What the project is, what was done last, and what waits.' }),
  note(18, 2 * 24 * 60, { area: 'recallsynapsis', tags: ['push'], title: 'Push is switched per module', body: 'Each module says whether it may push, and the owner switches it.' }),
  note(27, 3 * 24 * 60, { area: 'cavu', kind: 'idea', title: 'A read-only status page for the kitchen tablet', body: 'What runs, what waits, and nothing to press.' }),
  note(21, 4 * 24 * 60, { tags: ['hytte'], title: 'Pakkeliste til hytteturen', body: 'Sovepose, hodelykt, ved og fyrstikker.' }),
  note(7, 5 * 24 * 60, { area: 'cavu', title: 'The backup job retries once', body: 'A failed backup is logged and the next night runs as usual.' }),
  note(15, 8 * 24 * 60, { area: 'portal', tags: ['approval'], title: 'Marks are set per tool, never by the service', body: 'The service cannot vouch for itself.' }),
]

// What many=1 adds: older notes, two days apart from ten days back, so a list holds more than it shows.
const PAST: [Kind, string, string, string[]][] = [
  ['note', 'Longhorn snapshots before every upgrade', 'personal', ['homelab']],
  ['idea', 'Suggest tags from earlier notes', 'recallsynapsis', ['tags']],
  ['note', 'Cloudflare Access covers the whole hostname', 'portal', ['access']],
  ['note', 'Vinterdekk på før første snøfall', 'personal', ['bil']],
  ['idea', 'Show what an agent did overnight', 'cavu', ['agents']],
  ['note', 'The data directory is made with mode 700', 'recallsynapsis', ['security']],
  ['note', 'Pane titles follow the working directory', 'aim', ['tmux']],
  ['idea', 'A dry run for every migration', 'recallsynapsis', ['migrations']],
  ['note', 'Nye pakninger til kjøkkenkrana', 'personal', ['hus']],
  ['note', 'Retry once, then wait for the next night', 'recallsynapsis', ['backup']],
]
export const OLDER: Note[] = Array.from({ length: 60 }, (_, i) => {
  const [kind, title, area, tags] = PAST[i % PAST.length]
  return note(300 + i, (10 + 2 * i) * 24 * 60, { kind, title, area, tags, body: title })
})

// GET /api/notes from notes, as the server answers it: those outside the trash that the query asks for, the most
// recently changed first, as many as its limit, and how many it asked for in all.
export function listNotes(notes: Note[], q: URLSearchParams): List {
  const asks = (key: string, value: string) => !q.has(key) || q.get(key) === value
  const found = notes
    .filter((n) => !n.trashed_at && asks('inbox', String(n.inbox)) && asks('area', n.area) && asks('kind', n.kind) && (!q.has('tag') || n.tags.includes(q.get('tag')!)))
    .sort((a, b) => b.changed_at.localeCompare(a.changed_at))
  return { notes: found.slice(0, Number(q.get('limit')) || 50).map(row), total: found.length }
}

// GET /api/notes/tags from notes: each tag of a note outside the trash, by tag, with how many notes have it.
export function tagsOf(notes: Note[]) {
  const count = new Map<string, number>()
  for (const t of notes.filter((n) => !n.trashed_at).flatMap((n) => n.tags)) count.set(t, (count.get(t) ?? 0) + 1)
  return { tags: [...count].sort(([a], [b]) => a.localeCompare(b)).map(([tag, count]) => ({ tag, count })) }
}
