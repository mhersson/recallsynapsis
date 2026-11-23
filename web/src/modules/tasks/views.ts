import type { Now, Row } from './api.ts'
import { week } from './dates.ts'

export type View = 'today' | 'week' | 'next' | 'someday'

export type Group = 'sort' | 'left' | 'late' | 'today' | 'week' | 'later' | 'next' | 'someday'

// The views in the order of their row, each with its name and its groups in order.
export const views: [View, string, Group[]][] = [
  ['today', 'Today', ['sort', 'left', 'late', 'today']],
  ['week', 'Week', ['week', 'later']],
  ['next', 'Next', ['next']],
  ['someday', 'Someday', ['someday']],
]

// Whether a task belongs in a group on the server's today, end being the last day of its week.
function holds(g: Group, t: Row, today: string, end: string) {
  const from = t.planned_from
  const to = t.planned_to || from
  const due = t.deadline
  switch (g) {
    case 'sort':
      return t.state === 'unsorted'
    case 'left':
      return !!from && to < today
    case 'late':
      return !!due && due < today
    case 'today':
      return (!!from && from <= today && today <= to) || due === today
    case 'week':
      return (from > today && from <= end) || (due > today && due <= end)
    case 'later':
      return from > end || due > end
    default:
      return t.list === g
  }
}

// The groups of a view, each with its tasks in the order given; a task is in the first group it belongs to, or in none.
export function sortInto(view: View, tasks: Row[], now: Now): [Group, Row[]][] {
  const end = week(now)[1]
  const placed = new Set<number>()
  return views
    .find(([v]) => v === view)![2]
    .map((g) => {
      const held = tasks.filter((t) => !placed.has(t.id) && holds(g, t, now.date, end))
      held.forEach((t) => placed.add(t.id))
      return [g, held]
    })
}

// The order of the server's lists of open tasks: by the first day that matters, the planned from or else the deadline,
// those with a time first on it, those with no date last, and the oldest first among equals, which the id tells, as
// tasks made in one call share their time of making.
const key = (t: Row) => `${t.planned_from || t.deadline || '~'} ${t.planned_time || '~'}`
export const order = (a: Row, b: Row) => (key(a) < key(b) ? -1 : key(a) > key(b) ? 1 : a.id - b.id)
