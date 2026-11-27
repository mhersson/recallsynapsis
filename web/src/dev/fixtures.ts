import type { Area, Hit, Linked, Session, Token } from '../api/types.ts'
import type { Wording } from '../pages/Projects/Brief.tsx'
import type { Decision, Full } from '../modules/knowledge/api.ts'
import type { Kind, List, Note, Row, Version } from '../modules/notes/api.ts'
import type { Entry, Kinds, Now, List as TaskList, Row as TaskRow, Task } from '../modules/tasks/api.ts'
import { add } from '../modules/tasks/dates.ts'
import { order, sortInto } from '../modules/tasks/views.ts'

// What the API serves, in its shapes; the harness answers from it. Times are counted back from when the page loads.
const before = (minutes: number) => new Date(Date.now() - minutes * 60_000).toISOString()

export const HEALTH = { status: 'ok', version: '0.1.0' }

export const SESSION: Session = { user: { name: 'ada' }, modules: [], version: '0.1.0', timezone: 'Europe/Oslo', last_backup: before(6 * 60) }

// The brief of each fixed area but recallsynapsis is its first wording, written three days before.
const briefed = { brief_revision: 1, brief_changed_at: before(3 * 24 * 60), brief_changed_by: 'web' }
const BRIEF = [
  "Recall Synapsis is a self-hosted place for the owner's notes, ideas, tasks and project knowledge. It is reached from a web app, on desktop and phone, and from AI clients over MCP: Portal relays chat and voice clients to it, and coding sessions in AIM reach it directly on the home network.",
  'It exists so that what the owner means to do, and what was decided and why, is kept in one place that every client can read, and is not spread over conversations that end.',
  "It is one Go binary with the web app embedded, on SQLite. Every feature is a module, built on its own and switched on in the configuration. Notes, tasks and push are built and run in the owner's cluster. Knowledge is next. It has one user.",
  "What shapes it: the code stays tight and simple, and security is not taken lightly. Whether an AI's change needs a person's yes is the client's to ask, not Recall's. The look is CAVU's.",
]

// GET /api/areas?archived=true: the personal area first, then the projects by name.
export const AREAS: Area[] = [
  { id: 1, kind: 'personal', name: 'Personal', slug: 'personal', brief: 'Everything that belongs to no project.', ...briefed, archived: false },
  { id: 5, kind: 'project', name: 'aim', slug: 'aim', brief: 'The terminal multiplexer the coding agents work in.', ...briefed, archived: false },
  { id: 3, kind: 'project', name: 'cavu', slug: 'cavu', brief: 'Runs coding tasks in containers and asks when it needs me.', ...briefed, archived: false },
  { id: 6, kind: 'project', name: 'homelander', slug: 'homelander', brief: 'The first home cluster, retired in October.', ...briefed, archived: true },
  { id: 7, kind: 'project', name: 'Hytta', slug: 'hytta', brief: 'Hytta i Valdres: vedlikehold, annekset og veden.', ...briefed, archived: false },
  { id: 4, kind: 'project', name: 'portal', slug: 'portal', brief: 'Relays AI clients to the services at home.', ...briefed, archived: false },
  {
    id: 2,
    kind: 'project',
    name: 'recallsynapsis',
    slug: 'recallsynapsis',
    brief: BRIEF.join('\n\n'),
    brief_revision: 3,
    brief_changed_at: before(3 * 60),
    brief_changed_by: 'web',
    archived: false,
  },
]

// What an area's earlier wordings held where it was not what the area holds now, oldest first: recallsynapsis had one line,
// then a client drafted its brief, whose third paragraph the owner mended.
const EARLIER_BRIEFS: Record<number, Partial<Wording>[]> = {
  2: [
    { at: before(5 * 24 * 60), brief: 'Notes, ideas, tasks and project knowledge, reachable from a web app and from AI clients.' },
    {
      at: before(15 * 60),
      by: 'aim',
      said: 'Draft a brief for Recall Synapsis from the spec and the hand-off note, for me to approve.',
      brief: BRIEF.map((p) => p.replace(" and run in the owner's cluster. Knowledge is next.", '.')).join('\n\n'),
    },
  ],
}

// The wordings an area's brief has, oldest first: each holds the brief now, but where EARLIER_BRIEFS says otherwise.
export const wordings = (a: Area): Wording[] =>
  Array.from({ length: a.brief_revision }, (_, i) => ({
    revision: i + 1,
    at: a.brief_changed_at!,
    by: a.brief_changed_by,
    said: '',
    restored_from: null,
    brief: a.brief,
    ...EARLIER_BRIEFS[a.id]?.[i],
  }))

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
  file_count: n.files?.length ?? 0,
  revision: 1,
  source: 'web',
  created_at: before(minutes),
  changed_at: before(minutes),
  trashed_at: null,
  body: '',
  said: '',
  links: [],
  linked_from: [],
  files: [],
  ...n,
})

// A note as a list gives it.
export const row = ({ body: _, said: __, links: ___, linked_from: ____, files: _____, ...r }: Note): Row => r

// Links as a note keeps them, by reference alone; full resolves them.
export const refs = (...to: string[]): Linked[] => to.map((ref) => ({ ref, kind: '', state: '', title: '', available: false }))

// What a note goes by in a hit and in a link: its title, or the first line of its body.
const label = (n: Note) => n.title || n.body.split('\n')[0].slice(0, 80)

// A reference as the core resolves it among notes, tasks and decisions: unavailable once its item is in the trash or gone.
function linked(ref: string, notes: Note[], tasks: Task[], decisions: Decision[] = []): Linked {
  const n = notes.find((x) => x.ref === ref && !x.trashed_at)
  if (n) return { ref, kind: n.kind, state: n.inbox ? 'inbox' : 'filed', title: label(n), available: true }
  const t = tasks.find((x) => x.ref === ref && !x.trashed_at)
  if (t) return { ref, kind: 'task', state: t.state, title: t.title, available: true }
  const d = decisions.find((x) => x.ref === ref)
  return d ? { ref, kind: 'decision', state: d.state, title: d.decision, available: true } : refs(ref)[0]
}

// The notes and tasks outside the trash that link to ref, the notes first, as the core resolves them.
const linking = (ref: string, notes: Note[], tasks: Task[]) =>
  [...notes, ...tasks].filter((x) => !x.trashed_at && x.links.some((l) => l.ref === ref)).map((x) => linked(x.ref, notes, tasks))

// What an item links to as the core resolves it, among the decisions too when given, and what links to it.
export function resolved<T extends Note | Task>(item: T, notes: Note[], tasks: Task[], decisions: Decision[] = []): T {
  return { ...item, links: item.links.map((l) => linked(l.ref, notes, tasks, decisions)), linked_from: linking(item.ref, notes, tasks) }
}

// GET /api/notes/{id} from notes, and from tasks and decisions when given: n with its links and what links to it, resolved.
export const full = (notes: Note[], n: Note, tasks: Task[] = [], decisions: Decision[] = []): Note => resolved(n, notes, tasks, decisions)

// A note trashed hours ago, changed ten hours before that.
const trashed = (id: number, hours: number, n: Partial<Note>) => note(id, (hours + 10) * 60, { ...n, trashed_at: before(hours * 60) })

// GET /api/notes/{id} for each note: the captures that wait in the inbox, then filed notes and ideas, then those in the
// trash. A list gives the first 200 characters of the body as the excerpt.
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
    title: 'Sjekk backupjobben på NAS-en i helgen',
    body: 'Den feilet to netter på rad forrige uke.',
    links: refs('notes:12'),
  }),
  note(115, 26 * 60, { inbox: true, source: 'aim', body: 'The capture cap should count characters after trimming.' }),
  note(12, 2 * 60, {
    area: 'recallsynapsis',
    tags: ['approval', 'portal'],
    revision: 3,
    title: 'Approval belongs to the client',
    said: 'Note that approving what an AI writes is the client’s job, not ours.',
    // notes:37 was deleted for good, so its link leads nowhere.
    links: refs('notes:18', 'notes:44', 'notes:37'),
    files: [
      { id: 1, name: 'allow-card.png', type: 'image/png', size: 186_000 },
      { id: 2, name: 'whiteboard.jpg', type: 'image/jpeg', size: 2_516_582 },
      { id: 3, name: 'portal-marks.pdf', type: 'application/pdf', size: 86_016 },
    ],
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
    body: 'So a search for port finds garasjeport too.',
  }),
  note(22, 26 * 60, {
    tags: ['garage'],
    title: 'Garage shelving layout',
    body: `Left wall 240 cm, right wall 185 cm up to the window.

| Shelf | Wall | From the floor | Depth | Width | Holds |
| :---: | --- | ---: | ---: | ---: | --- |
| 1 | Left | 30 cm | 40 cm | 240 cm | **Tyres** under it, paint on it |
| 2 | Left | 75 cm | 40 cm | 240 cm | Tools, \`M8\` and \`M10\` bolts |
| 3 | Right | 75 cm | 30 cm | 185 cm | Camping gear \\| ski wax |
## Still to buy
Brackets for the right wall, see https://example.test/brackets.`,
    links: refs('tasks:106'),
  }),
  note(25, 27 * 60, {
    area: 'aim',
    kind: 'idea',
    tags: ['agents'],
    title: 'A context pack when an agent session starts',
    body: 'What the project is, what was done last, and what waits.',
    links: refs('notes:12'),
  }),
  note(18, 2 * 24 * 60, { area: 'recallsynapsis', tags: ['push'], title: 'Push is switched per module', body: 'Each module says whether it may push, and the owner switches it.' }),
  note(19, 3 * 24 * 60, {
    area: 'recallsynapsis',
    tags: ['web'],
    title: 'Save and Cancel on item pages',
    body: "Changes on an item's page go through Save and Cancel. Only a one-tap toggle saves at once, and the rows of a list carry no quick buttons.\n\nA form's buttons sit at its right, Cancel then Save, so Save is furthest right.",
    links: refs('knowledge:10', 'knowledge:6'),
  }),
  note(27, 3 * 24 * 60, { area: 'cavu', kind: 'idea', title: 'A read-only status page for the kitchen tablet', body: 'What runs, what waits, and nothing to press.' }),
  note(21, 4 * 24 * 60, { tags: ['hytte'], title: 'Pakkeliste til hytteturen', body: 'Sovepose, hodelykt, ved og fyrstikker.' }),
  note(9, 2 * 24 * 60 + 30, {
    tags: ['homelab'],
    title: 'Backup directory on the NAS',
    body: 'The nightly snapshot goes to a share on the NAS, never to the data volume itself.',
  }),
  note(7, 5 * 24 * 60, { area: 'cavu', title: 'The backup job retries once', body: 'A failed backup is logged and the next night runs as usual.' }),
  note(44, 7 * 24 * 60, {
    area: 'recallsynapsis',
    kind: 'idea',
    tags: ['backup'],
    title: 'Restore drill: prove a backup restores every month',
    body: 'Start a server on last night’s backup and open the inbox.',
  }),
  note(15, 8 * 24 * 60, { area: 'portal', tags: ['approval'], title: 'Marks are set per tool, never by the service', body: 'The service cannot vouch for itself.' }),
  trashed(50, 2, {
    tags: ['garage'],
    revision: 3,
    title: 'Old shelf measurements',
    body: 'Left wall 240 cm, right wall 185 cm up to the window.\n\nFive shelves, 40 cm deep, the lowest 30 cm off the floor so the tyres fit under it.',
    files: [{ id: 4, name: 'shelf.jpg', type: 'image/jpeg', size: 1_258_291 }],
  }),
  trashed(51, 26, { inbox: true, source: 'portal', body: 'Ring the garage door company about the spring.' }),
  trashed(52, 50, { area: 'recallsynapsis', kind: 'idea', title: 'A weekly digest of new captures by mail', body: 'Sent on Sunday evening.' }),
  trashed(53, 74, { area: 'cavu', title: 'Meeting moved to Thursday, bring the drawings', body: 'At ten, in the small room.' }),
  trashed(54, 5 * 24, { area: 'recallsynapsis', title: 'Token names, first draft', body: 'portal, aim, home-assistant' }),
  trashed(55, 7 * 24, { kind: 'idea', title: 'Capture by voice from the watch', body: 'A shortcut that sends what was said.' }),
  trashed(56, 10 * 24, { body: 'test' }),
]

// Revision n of note as it is now, as GET /api/notes/{id}/revisions/{n} gives it; the first carries what was said.
export const version = (n: Note, revision: number): Version => ({
  revision,
  at: revision === n.revision ? n.changed_at : n.created_at,
  source: n.source,
  said: revision === 1 ? n.said : '',
  restored_from: null,
  title: n.title,
  body: n.body,
  kind: n.kind,
  area: n.area,
  inbox: n.inbox,
  tags: n.tags,
  links: n.links.map((l) => l.ref),
})

// What a note's earlier revisions held where it was not what the note holds now, oldest first.
const EARLIER: Record<number, Partial<Version>[]> = {
  12: [
    { at: before(3 * 24 * 60 + 200), source: 'portal', body: 'Portal marks each tool as free or as needing a tap.', tags: ['approval'], links: [] },
    {
      at: before(2 * 24 * 60 + 300),
      source: 'aim',
      body: 'Portal marks each tool as free or as needing a tap. Recall Synapsis only has to split its tools by risk.\n\n- capture, list and read tools can be free\n- knowledge changes always meet the Allow card',
      tags: ['approval'],
      links: ['notes:18'],
    },
  ],
}

// The revisions a note has, oldest first: each holds what the note holds now, but where EARLIER says otherwise.
export const history = (n: Note): Version[] => Array.from({ length: n.revision }, (_, i) => ({ ...version(n, i + 1), ...EARLIER[n.id]?.[i] }))

// GET /api/notes/{id}/revisions from a note's revisions, oldest first: each as the history lists it, the newest first.
export const listed = (versions: Version[]) => ({
  revisions: versions.map(({ revision, at, source, said, restored_from }) => ({ revision, at, source, said, restored_from })).reverse(),
})

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

// GET /api/notes from notes, as the server answers it: those the query asks for, outside the trash or with trashed=true
// in it, the most recently changed or trashed first, as many as its limit, and how many it asked for in all.
export function listNotes(notes: Note[], q: URLSearchParams): List {
  const asks = (key: string, value: string) => !q.has(key) || q.get(key) === value
  const trash = q.get('trashed') === 'true'
  const when = (n: Note) => (trash ? n.trashed_at! : n.changed_at)
  const found = notes
    .filter((n) => !!n.trashed_at === trash && asks('inbox', String(n.inbox)) && asks('area', n.area) && asks('kind', n.kind) && (!q.has('tag') || n.tags.includes(q.get('tag')!)))
    .sort((a, b) => when(b).localeCompare(when(a)))
  return { notes: found.slice(0, Number(q.get('limit')) || 50).map(row), total: found.length }
}

// GET /api/notes/tags from notes: each tag of a note outside the trash, by tag, with how many notes have it.
export function tagsOf(notes: Note[]) {
  const count = new Map<string, number>()
  for (const t of notes.filter((n) => !n.trashed_at).flatMap((n) => n.tags)) count.set(t, (count.get(t) ?? 0) + 1)
  return { tags: [...count].sort(([a], [b]) => a.localeCompare(b)).map(([tag, count]) => ({ tag, count })) }
}

// GET /api/search from notes, as the notes module answers it: every note outside the trash whose title or body holds q,
// in any case, and in the area asked for, if one is.
export function searchNotes(notes: Note[], q: string, area: string | null): Hit[] {
  const has = (n: Note) => (n.title + '\n' + n.body).toLowerCase().includes(q.toLowerCase())
  return notes
    .filter((n) => !n.trashed_at && has(n) && (!area || n.area === area))
    .map((n) => ({
      ref: n.ref,
      kind: n.kind,
      title: label(n),
      snippet: n.body,
      area: n.area,
      state: n.inbox ? 'inbox' : 'filed',
      changed_at: n.changed_at,
    }))
}

// The day GET /api/tasks gives the harness: Monday 23 November 2026 in Oslo, with weeks that begin on a Monday.
export const NOW: Now = { date: '2026-11-23', weekday: 'Monday', time: '10:00', timezone: 'Europe/Oslo', week_start: 'monday' }

// GET /api/tasks/notify for a user who has changed none of the kinds.
export const KINDS: Kinds = { start: true, morning: true, morning_at: '07:00', deadline: false, deadline_at: '18:00', untimed: false, untimed_at: '09:00' }

const WEEKDAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
const weekday = (date: string) => (date ? WEEKDAYS[new Date(date + 'T00:00:00Z').getUTCDay()] : '')

// The fields of a task its log tells of, as the wire names them; done is told of apart.
const FIELDS = ['title', 'area', 'planned_from', 'planned_to', 'planned_time', 'deadline', 'list', 'notes'] as const

// A task with what follows from what it holds: the weekday of each date, its state and whether it has notes.
const settled = (t: Task): Task => ({
  ...t,
  planned_from_day: weekday(t.planned_from),
  planned_to_day: weekday(t.planned_to),
  deadline_day: weekday(t.deadline),
  has_notes: t.notes !== '',
  state: t.done_at ? 'done' : t.planned_from || t.deadline || t.list ? 'open' : 'unsorted',
})

// A task of the personal area made minutes ago, a from alone planning that one day; its log is its making, and its marking
// done once it is, unless t gives one.
export function task(id: number, minutes: number, t: Partial<Task>): Task {
  const made: Task = {
    id,
    ref: `tasks:${id}`,
    area: 'personal',
    title: '',
    planned_from: '',
    planned_from_day: '',
    planned_to: '',
    planned_to_day: '',
    planned_time: '',
    deadline: '',
    deadline_day: '',
    list: '',
    state: 'open',
    done_at: null,
    has_notes: false,
    source: 'web',
    created_at: before(minutes),
    changed_at: before(minutes),
    trashed_at: null,
    notes: '',
    said: '',
    links: [],
    linked_from: [],
    log: [],
    now: NOW,
    ...t,
  }
  made.planned_to ||= made.planned_from
  const making: Entry = {
    at: made.created_at,
    source: made.source,
    said: made.said,
    changes: FIELDS.filter((f) => made[f]).map((f) => (f === 'notes' ? { field: f } : { field: f, from: '', to: made[f] })),
  }
  const marked: Entry[] = made.done_at ? [{ at: made.done_at, source: made.source, said: '', changes: [{ field: 'done', from: 'false', to: 'true' }] }] : []
  return settled({ ...made, log: t.log ?? [...marked, making] })
}

// A task as a list gives it.
export const taskRow = ({ notes: _, said: __, links: ___, linked_from: ____, log: _____, now: ______, ...r }: Task): TaskRow => r

// GET /api/tasks from tasks, as the server answers it: the open tasks outside the trash, or the done with done=true, or
// with trashed=true the whole trash, done and open alike, of the area and the list asked, or those to sort, in the
// server's order, as many as its limit, how many it asked for in all, and now.
export function listTasks(tasks: Task[], q: URLSearchParams, now: Now): TaskList {
  const done = q.get('done') === 'true'
  const trash = q.get('trashed') === 'true'
  const list = q.get('list') === 'none' ? '' : q.get('list')
  const found = tasks
    .filter(
      (t) =>
        !!t.trashed_at === trash &&
        ((trash && !done) || !!t.done_at === done) &&
        (!q.has('area') || t.area === q.get('area')) &&
        (list === null || t.list === list) &&
        (q.get('unsorted') !== 'true' || t.state === 'unsorted'),
    )
    .sort(trash ? (a, b) => b.trashed_at!.localeCompare(a.trashed_at!) : done ? (a, b) => b.done_at!.localeCompare(a.done_at!) : order)
  const offset = Number(q.get('offset'))
  return { tasks: found.slice(offset, offset + (Number(q.get('limit')) || 50)).map(taskRow), total: found.length, now }
}

// POST /api/tasks for one task, as the server makes it, or its refusal.
export function makeTask(id: number, body: Record<string, unknown>, said: string): Task | [number, string] {
  const title = String(body.title ?? '').trim()
  if (!title || [...title].length > 200) return [400, 'title: give 1 to 200 characters']
  const { links, ...rest } = body
  const given = Object.fromEntries(Object.entries(rest).map(([k, v]) => [k, String(v)]))
  return task(id, 0, { ...given, title, said, list: (given.list ?? '') as Task['list'], links: refs(...((links as string[]) ?? [])) })
}

// PATCH /api/tasks/{id}, as the server applies it: a from sets the plan, one day unless a to comes with it, and a from of
// "" clears the plan whole; a to of "" makes the plan one day; then the time. A change of what the task holds writes an
// entry of its log.
export function changeTask(t: Task, body: Record<string, unknown>): Task | [number, string] {
  if (Object.keys(body).length === 0) return [400, 'nothing to change']
  const next: Task = { ...t }
  for (const k of ['title', 'notes', 'area', 'deadline', 'list'] as const) if (k in body) Object.assign(next, { [k]: String(body[k]) })
  next.title = next.title.trim()
  if ('planned_from' in body) Object.assign(next, { planned_from: String(body.planned_from), planned_to: String(body.planned_to ?? '') }, !body.planned_from && { planned_time: '' })
  else if ('planned_to' in body) next.planned_to = String(body.planned_to)
  next.planned_to ||= next.planned_from
  if ('planned_time' in body) next.planned_time = String(body.planned_time)
  if ('done' in body) next.done_at = body.done ? (t.done_at ?? new Date().toISOString()) : null
  if (Array.isArray(body.links)) next.links = refs(...(body.links as string[]))
  if (!next.title || [...next.title].length > 200) return [400, 'title: give 1 to 200 characters']
  if (!next.planned_from && next.planned_to) return [400, 'planned_to: give planned_from too']
  if (!next.planned_from && next.planned_time) return [400, 'planned_time: give planned_from too']
  if (next.planned_to < next.planned_from) return [400, `planned_to: ${next.planned_to} is before planned_from ${next.planned_from}`]
  // In the server's order: the fields as the wire lists them, then done, then the notes and the links.
  const changes: Entry['changes'] = FIELDS.filter((f) => f !== 'notes' && next[f] !== t[f]).map((f) => ({ field: f, from: t[f], to: next[f] }))
  if (!!next.done_at !== !!t.done_at) changes.push({ field: 'done', from: String(!!t.done_at), to: String(!!next.done_at) })
  if (next.notes !== t.notes) changes.push({ field: 'notes' })
  if (next.links.map((l) => l.ref).join() !== t.links.map((l) => l.ref).join()) changes.push({ field: 'links' })
  if (changes.length === 0) return t
  const at = new Date().toISOString()
  return settled({ ...next, source: 'web', changed_at: at, log: [{ at, source: 'web', said: '', changes }, ...t.log] })
}

// The open tasks of the harness, as the tasks screens are drawn: dates from the Monday of NOW, made in the order listed.
export const TASKS: Task[] = (
  [
    [{ title: 'Fikse lyset i garasjen', source: 'portal', said: 'Lyset i garasjen blinker igjen. Legg inn at det må fikses.', links: refs('tasks:9') }, 12],
    [{ title: 'Skifte til vinterdekk', planned_from: '2026-11-21' }],
    [{ title: 'Stenge vannet på hytta', area: 'hytta', planned_from: '2026-11-21', planned_to: '2026-11-22' }],
    [{ title: 'Betale fakturaen fra elektrikeren', deadline: '2026-11-20' }],
    [{ title: 'Ringe rørleggeren', planned_from: '2026-11-23', planned_time: '14:00' }],
    [{ title: 'Hente pakke på posten', planned_from: '2026-11-23' }],
    [{ title: 'Ringe snekkeren om annekset', area: 'hytta', planned_from: '2026-11-23' }],
    [{ title: 'Søke om byggetillatelse for annekset', area: 'hytta', deadline: '2026-11-27' }],
    [
      {
        title: 'Rydde garasjen',
        changed_at: before(2 * 60),
        links: refs('notes:22'),
        planned_from: '2026-11-28',
        planned_to: '2026-11-29',
        notes: '- kaste gammel maling\n- sortere skruene\n- få plass til bilen før snøen',
        log: [
          { at: before(2 * 60), source: 'web', said: '', changes: [{ field: 'notes' }] },
          {
            at: before(14 * 60),
            source: 'web',
            said: '',
            changes: [
              { field: 'planned_from', from: '2026-11-21', to: '2026-11-28' },
              { field: 'planned_to', from: '2026-11-21', to: '2026-11-29' },
            ],
          },
          {
            at: before(4 * 24 * 60),
            source: 'portal',
            said: 'Legg inn at jeg må rydde garasjen på lørdag.',
            changes: [
              { field: 'title', from: '', to: 'Rydde garasjen' },
              { field: 'area', from: '', to: 'personal' },
              { field: 'planned_from', from: '', to: '2026-11-21' },
              { field: 'planned_to', from: '', to: '2026-11-21' },
            ],
          },
        ],
      },
    ],
    [{ title: 'Vaske bilen', planned_from: '2026-11-28', planned_to: '2026-11-29' }],
    [{ title: 'Handle materialer', area: 'hytta', planned_from: '2026-11-28', planned_to: '2026-11-29', list: 'soon' }],
    [{ title: 'Kjøre ved til hytta', area: 'hytta', planned_from: '2026-12-05', planned_to: '2026-12-06' }],
    [{ title: 'Bytte olje på bilen', planned_from: '2026-12-09', planned_time: '08:30' }],
    [{ title: 'Bestille EU-kontroll', deadline: '2026-12-14', list: 'soon' }],
    [{ title: 'Måle opp kjøkkenet på hytta', area: 'hytta', list: 'soon' }],
    [{ title: 'Bytte pakning i kjøkkenkrana', list: 'soon' }],
    [{ title: 'Bygge vedskjul', area: 'hytta', list: 'someday' }],
    [{ title: 'Male gjerdet', list: 'someday' }],
    [{ title: 'Legge inn fiber på hytta', area: 'hytta', list: 'someday', notes: 'Spør kommunen om graving langs veien.' }],
  ] as [Partial<Task>, number?][]
).map(([t, minutes], i) => task(i + 1, minutes ?? (40 - i) * 24 * 60, t))

// GET /api/search from tasks, as the tasks module answers it: every task outside the trash, done or open, whose title or
// notes hold q, in any case, and in the area asked for, if one is.
export function searchTasks(tasks: Task[], q: string, area: string | null): Hit[] {
  const has = (t: Task) => (t.title + '\n' + t.notes).toLowerCase().includes(q.toLowerCase())
  return tasks
    .filter((t) => !t.trashed_at && has(t) && (!area || t.area === area))
    .map((t) => ({ ref: t.ref, kind: 'task', title: t.title, snippet: t.notes, area: t.area, state: t.state, changed_at: t.changed_at }))
}

// The done tasks of the harness, the most recently done first, each done at the time of Oslo given on a day of the week
// of NOW: Done lists them by day.
const oslo = (date: string, time: string) => new Date(`${date}T${time}:00+01:00`).toISOString()
export const DONE: Task[] = (
  [
    [{ title: 'Kjøpe frostvæske', list: 'soon', notes: 'Den blå, til minus 25.' }, '2026-11-23', '09:10'],
    [{ title: 'Bestille brøyting til hytta', area: 'hytta' }, '2026-11-22', '19:30'],
    [{ title: 'Rense sluket på badet' }, '2026-11-22', '11:05'],
    [{ title: 'Skifte lyspærer i gangen' }, '2026-11-21', '15:40'],
    [{ title: 'Bære inn utemøblene' }, '2026-11-21', '12:20'],
    [{ title: 'Kjøpe hyllekonsoller', list: 'soon' }, '2026-11-20', '17:05'],
    [{ title: 'Smøre garasjeporten' }, '2026-11-19', '18:40'],
    [{ title: 'Hente vinterhjulene på dekkhotellet' }, '2026-11-19', '10:15'],
    [{ title: 'Kappe ved til peisen', area: 'hytta' }, '2026-11-18', '20:05'],
    [{ title: "Put a task's dates, its list and its area behind Save", area: 'recallsynapsis', links: refs('knowledge:10') }, '2026-11-18', '16:30'],
  ] as [Partial<Task>, string, string][]
).map(([t, date, time], i) => task(101 + i, (12 + i) * 24 * 60, { ...t, done_at: oslo(date, time), changed_at: oslo(date, time) }))

// What many=1 adds to the done: sixty more, two a day from the Tuesday before NOW back, so Done holds more than it shows.
const CHORES = ['Spyle terrassen', 'Bytte lås på boden', 'Sjekke oljen på bilen', 'Rydde kjøleskapet', 'Kjøpe ny dørmatte', 'Skifte batteri i dørklokka']
export const OLDER_DONE: Task[] = Array.from({ length: 60 }, (_, i) => {
  const at = oslo(add('2026-11-17', -Math.floor(i / 2)), i % 2 ? '10:15' : '18:40')
  return task(201 + i, (20 + i) * 24 * 60, { title: CHORES[i % CHORES.length], area: i % 3 ? 'personal' : 'hytta', done_at: at, changed_at: at })
})

// The trashed tasks of the harness, the most recently trashed first: one planned, one on Someday and one to sort.
export const TRASHED: Task[] = (
  [
    [{ title: 'Ringe banken om lånet', planned_from: '2026-11-24', notes: 'Spør om fastrente på resten av lånet.' }, 2 * 60],
    [{ title: 'Kjøpe ny snøfreser', list: 'someday' }, 26 * 60],
    [{ title: 'Rydde i bilen', area: 'hytta' }, 5 * 24 * 60],
  ] as [Partial<Task>, number][]
).map(([t, minutes], i) => task(301 + i, minutes + 3 * 24 * 60, { ...t, trashed_at: before(minutes) }))

// What Today would show of tasks, which today=none leaves out.
export const onToday = (tasks: Task[], now: Now) => sortInto('today', tasks.map(taskRow), now).flatMap(([, ts]) => ts.map((t) => t.id))

// The tasks routes from tasks, as the server answers them, as a status and a body, none with 204, a task in full with its
// links resolved among notes and tasks, and the decisions when given; tell hears of each task made, changed or deleted,
// and of none when the trash is emptied.
export function answerTasks(
  tasks: Task[],
  method: string,
  url: URL,
  body: Record<string, unknown>,
  now: Now,
  tell: (t?: Task) => void,
  notes: Note[] = [],
  decisions: Decision[] = [],
): [number, unknown] {
  const whole = (x: Task) => ({ ...resolved(x, notes, tasks, decisions), now })
  if (url.pathname === '/api/tasks' && method === 'GET') return [200, listTasks(tasks, url.searchParams, now)]
  if (url.pathname === '/api/tasks' && method === 'POST') {
    const made: Task[] = []
    for (const b of body.tasks as Record<string, unknown>[]) {
      const t = makeTask(Math.max(0, ...tasks.map((x) => x.id), ...made.map((x) => x.id)) + 1, b, String(body.said ?? ''))
      if (Array.isArray(t)) return [t[0], { error: t[1] }]
      made.push(t)
    }
    tasks.push(...made)
    made.forEach((t) => tell(t))
    return [201, { tasks: made.map(taskRow), now }]
  }
  if (url.pathname === '/api/tasks/trash' && method === 'DELETE') {
    for (const t of tasks.filter((x) => x.trashed_at)) tasks.splice(tasks.indexOf(t), 1)
    tell()
    return [204, undefined]
  }
  const [, id, part] = /^\/api\/tasks\/(\d+)(?:\/(trash|restore))?$/.exec(url.pathname) ?? []
  const t = tasks.find((x) => x.id === Number(id))
  if (!t) return [404, { error: 'not found' }]
  const told = (x: Task): [number, unknown] => {
    tell(x)
    return [200, whole(x)]
  }
  // Trashing a task in the trash, or restoring one outside it, changes nothing and tells nothing.
  if (part && method === 'POST') return !!t.trashed_at === (part === 'trash') ? [200, whole(t)] : told(Object.assign(t, { trashed_at: part === 'trash' ? new Date().toISOString() : null }))
  if (method === 'GET' && !part) return [200, whole(t)]
  if (method === 'DELETE' && !part) {
    if (!t.trashed_at) return [409, { error: `task ${t.id} is not in the trash, and only a task in the trash is deleted: conflict` }]
    tasks.splice(tasks.indexOf(t), 1)
    tell(t)
    return [204, undefined]
  }
  if (method !== 'PATCH' || part) return [404, { error: 'not found' }]
  if (t.trashed_at) return [409, { error: 'the task is in the trash, and is restored before it is changed: conflict' }]
  const changed = changeTask(t, body)
  if (Array.isArray(changed)) return [changed[0], { error: changed[1] }]
  if (changed === t) return [200, whole(t)]
  return told(Object.assign(t, changed))
}

// The decisions of recallsynapsis, as the knowledge screens are drawn, the newest day first and, within a day, the most
// recently made first, as GET /api/knowledge lists them. Most were recorded in one retrospective; one was mended in the
// web app since. None of another area has any.
const recorded = { created_at: oslo('2026-11-23', '21:40'), changed_at: oslo('2026-11-23', '21:40'), changed_by: 'aim', revision: 1 }
type Drawn = Pick<Decision, 'id' | 'decision' | 'why' | 'source' | 'decided'> & Partial<Decision> & { replacing?: number }
const DRAWN: Drawn[] = [
  {
    id: 11,
    decision: 'Tasks opens on Agenda, where each open task is once, under Scheduled or Unscheduled',
    why: 'Not recorded: the owner built it directly.',
    source: 'https://github.com/mhersson/recallsynapsis/pull/41',
    decided: '2026-11-26',
    replacing: 7,
    created_at: oslo('2026-11-26', '22:14'),
    changed_at: oslo('2026-11-26', '22:14'),
  },
  {
    id: 10,
    decision: 'A switch saves the moment it is flipped, and a field waits for an explicit Save',
    why: 'A date or a time has in-between values while it is typed or stepped through, and none of those should be saved. A switch has none.',
    source: "The owner's words on the built task page; the push playground's verdict",
    decided: '2026-11-23',
    replacing: 6,
    revision: 2,
    changed_at: oslo('2026-11-24', '09:12'),
    changed_by: 'web',
  },
  {
    id: 9,
    decision: "What a person chooses about behaviour is set in the web app's Settings, not in the configuration file",
    why: '“The most user friendly”. The file is for whoever runs the server.',
    source: "Push's design pass",
    decided: '2026-11-23',
  },
  { id: 8, decision: 'The list is Soon, not Next', why: 'Next read as next week, beside the shortcut of that name.', source: 'The spec, section 7', decided: '2026-11-23' },
  {
    id: 7,
    decision: 'With tasks on, a person lands on Today',
    why: 'Chosen in the tasks playground, where Today was the first of the views and held what waits to be sorted, what is left from earlier, what is overdue and what the day holds.',
    source: "The tasks playground's verdict",
    decided: '2026-11-23',
  },
  {
    id: 6,
    decision: "On a task's page the title and the notes are behind Save, and the other controls save as they change",
    why: "The owner's first choice in the tasks playground. Left the same day, once the owner saw it built.",
    source: "The tasks playground's verdict",
    decided: '2026-11-23',
    created_at: oslo('2026-11-23', '11:32'),
    changed_at: oslo('2026-11-23', '11:32'),
  },
  {
    id: 5,
    decision: "A tool is named verb and the module's noun: `create_note`, not `capture`",
    why: 'So one sees which module a tool belongs to, and `capture` could as well have meant `update_note`.',
    source: 'Review of the notes plan',
    decided: '2026-11-22',
  },
  {
    id: 4,
    decision: 'SQLite, not PostgreSQL',
    why: "No PostgreSQL runs in the cluster, and Portal, CAVU and ContextMatrix all use pure Go SQLite. Turned down: the requirements note's PostgreSQL.",
    source: 'The spec, section 2',
    decided: '2026-11-22',
  },
  {
    id: 3,
    decision: 'Recall has no approval mechanism. Approval belongs to the client, and only a change to curated knowledge must meet its card',
    why: "The spec gives the limit the owner accepted, that this guards against an overeager agent and not a hostile client, and no reason beyond it. Turned down: the requirements note's change sets.",
    source: 'The spec, section 2',
    decided: '2026-11-22',
  },
  {
    id: 2,
    decision: 'An idea is a kind of note, and a project is an area',
    why: "Not recorded. Turned down: the requirements note's Idea and Workspace as things of their own.",
    source: 'The spec, section 2',
    decided: '2026-11-22',
  },
  { id: 1, decision: 'Tasks are lean: no recurrence, no priority, no subtasks', why: 'Not recorded.', source: 'The spec, section 2', decided: '2026-11-22' },
]
export const DECISIONS: Decision[] = DRAWN.map(({ replacing: _, ...d }) => ({
  ref: `knowledge:${d.id}`,
  area: 'recallsynapsis',
  state: DRAWN.some((x) => x.replacing === d.id) ? 'replaced' : 'stands',
  replaces: null,
  replaced_by: null,
  ...recorded,
  ...d,
}))
for (const d of DRAWN.filter((x) => x.replacing)) {
  const [newer, older] = [DECISIONS.find((x) => x.id === d.id)!, DECISIONS.find((x) => x.id === d.replacing)!]
  newer.replaces = { ref: older.ref, decision: older.decision, decided: older.decided }
  older.replaced_by = { ref: newer.ref, decision: newer.decision, decided: newer.decided }
}

// The day GET /api/knowledge gives the harness: the decisions are of the days before it.
export const DECIDED_TODAY = '2026-11-27'

// GET /api/knowledge/{id} from decisions: d with the notes and tasks outside the trash that link to it.
export const decisionOf = (d: Decision, notes: Note[], tasks: Task[]): Full => ({ ...d, linked_from: linking(d.ref, notes, tasks) })

// GET /api/search from decisions, as the knowledge module answers it: every decision, standing or replaced, whose line,
// why or source holds q, in any case, and in the area asked for, if one is.
export function searchDecisions(decisions: Decision[], q: string, area: string | null): Hit[] {
  const has = (d: Decision) => [d.decision, d.why, d.source].join('\n').toLowerCase().includes(q.toLowerCase())
  return decisions
    .filter((d) => has(d) && (!area || d.area === area))
    .map((d) => ({ ref: d.ref, kind: 'decision', title: d.decision, snippet: d.why, area: d.area, state: d.state, changed_at: d.changed_at }))
}
