import type { Area, Hit, Linked, Session, Token } from '../api/types.ts'
import type { Kind, List, Note, Row, Version } from '../modules/notes/api.ts'
import type { Entry, Now, List as TaskList, Row as TaskRow, Task } from '../modules/tasks/api.ts'
import { order, sortInto } from '../modules/tasks/views.ts'

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
  { id: 7, kind: 'project', name: 'Hytta', slug: 'hytta', description: 'Hytta i Valdres: vedlikehold, annekset og veden.', archived: false },
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

// GET /api/notes/{id} from notes: n with its links as the core resolves them, unavailable once their note is in the
// trash or gone, and the notes outside the trash that link to it.
export function full(notes: Note[], n: Note): Note {
  const item = (ref: string): Linked => {
    const t = notes.find((x) => x.ref === ref && !x.trashed_at)
    return t ? { ref, kind: t.kind, state: t.inbox ? 'inbox' : 'filed', title: label(t), available: true } : refs(ref)[0]
  }
  const from = notes.filter((x) => !x.trashed_at && x.links.some((l) => l.ref === n.ref))
  return { ...n, links: n.links.map((l) => item(l.ref)), linked_from: from.map((x) => item(x.ref)) }
}

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
  note(22, 26 * 60, { tags: ['garage'], title: 'Garage shelving layout', body: 'Left wall 240 cm, right wall 185 cm up to the window.' }),
  note(25, 27 * 60, {
    area: 'aim',
    kind: 'idea',
    tags: ['agents'],
    title: 'A context pack when an agent session starts',
    body: 'What the project is, what was done last, and what waits.',
    links: refs('notes:12'),
  }),
  note(18, 2 * 24 * 60, { area: 'recallsynapsis', tags: ['push'], title: 'Push is switched per module', body: 'Each module says whether it may push, and the owner switches it.' }),
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

// A task of the personal area made minutes ago, a from alone planning that one day; its log is its making unless t gives one.
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
    notes: '',
    said: '',
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
  return settled({ ...made, log: t.log ?? [making] })
}

// A task as a list gives it.
export const taskRow = ({ notes: _, said: __, log: ___, now: ____, ...r }: Task): TaskRow => r

// GET /api/tasks from tasks, as the server answers it: the open tasks, or the done with done=true, of the area and the
// list asked, or those to sort, in the server's order, as many as its limit, how many it asked for in all, and now.
export function listTasks(tasks: Task[], q: URLSearchParams, now: Now): TaskList {
  const done = q.get('done') === 'true'
  const list = q.get('list') === 'none' ? '' : q.get('list')
  const found = tasks
    .filter((t) => !!t.done_at === done && (!q.has('area') || t.area === q.get('area')) && (list === null || t.list === list) && (q.get('unsorted') !== 'true' || t.state === 'unsorted'))
    .sort(done ? (a, b) => b.done_at!.localeCompare(a.done_at!) : order)
  return { tasks: found.slice(0, Number(q.get('limit')) || 50).map(taskRow), total: found.length, now }
}

// POST /api/tasks for one task, as the server makes it, or its refusal.
export function makeTask(id: number, body: Record<string, unknown>, said: string): Task | [number, string] {
  const title = String(body.title ?? '').trim()
  if (!title || [...title].length > 200) return [400, 'title: give 1 to 200 characters']
  const given = Object.fromEntries(Object.entries(body).map(([k, v]) => [k, String(v)]))
  return task(id, 0, { ...given, title, said, list: (given.list ?? '') as Task['list'] })
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
  if (!next.title || [...next.title].length > 200) return [400, 'title: give 1 to 200 characters']
  if (!next.planned_from && next.planned_to) return [400, 'planned_to: give planned_from too']
  if (!next.planned_from && next.planned_time) return [400, 'planned_time: give planned_from too']
  if (next.planned_to < next.planned_from) return [400, `planned_to: ${next.planned_to} is before planned_from ${next.planned_from}`]
  // In the server's order: the fields as the wire lists them, then done, then the notes.
  const changes: Entry['changes'] = FIELDS.filter((f) => f !== 'notes' && next[f] !== t[f]).map((f) => ({ field: f, from: t[f], to: next[f] }))
  if (!!next.done_at !== !!t.done_at) changes.push({ field: 'done', from: String(!!t.done_at), to: String(!!next.done_at) })
  if (next.notes !== t.notes) changes.push({ field: 'notes' })
  if (changes.length === 0) return t
  const at = new Date().toISOString()
  return settled({ ...next, source: 'web', changed_at: at, log: [{ at, source: 'web', said: '', changes }, ...t.log] })
}

// The open tasks of the harness, as the tasks screens are drawn: dates from the Monday of NOW, made in the order listed.
export const TASKS: Task[] = (
  [
    [{ title: 'Fikse lyset i garasjen', source: 'portal', said: 'Lyset i garasjen blinker igjen. Legg inn at det må fikses.' }, 12],
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
    .filter((t) => has(t) && (!area || t.area === area))
    .map((t) => ({ ref: t.ref, kind: 'task', title: t.title, snippet: t.notes, area: t.area, state: t.state, changed_at: t.changed_at }))
}

// What Today would show of tasks, which today=none leaves out.
export const onToday = (tasks: Task[], now: Now) => sortInto('today', tasks.map(taskRow), now).flatMap(([, ts]) => ts.map((t) => t.id))

// The tasks routes from tasks, as the server answers them, as a status and a body; tell hears of each task made or changed.
export function answerTasks(tasks: Task[], method: string, url: URL, body: Record<string, unknown>, now: Now, tell: (t: Task) => void): [number, unknown] {
  if (url.pathname === '/api/tasks' && method === 'GET') return [200, listTasks(tasks, url.searchParams, now)]
  if (url.pathname === '/api/tasks' && method === 'POST') {
    const made: Task[] = []
    for (const b of body.tasks as Record<string, unknown>[]) {
      const t = makeTask(Math.max(0, ...tasks.map((x) => x.id), ...made.map((x) => x.id)) + 1, b, String(body.said ?? ''))
      if (Array.isArray(t)) return [t[0], { error: t[1] }]
      made.push(t)
    }
    tasks.push(...made)
    made.forEach(tell)
    return [201, { tasks: made.map(taskRow), now }]
  }
  const t = tasks.find((x) => url.pathname === `/api/tasks/${x.id}`)
  if (!t || !['GET', 'PATCH'].includes(method)) return [404, { error: 'not found' }]
  if (method === 'GET') return [200, { ...t, now }]
  const changed = changeTask(t, body)
  if (Array.isArray(changed)) return [changed[0], { error: changed[1] }]
  if (changed !== t) tell(Object.assign(t, changed))
  return [200, { ...t, now }]
}
