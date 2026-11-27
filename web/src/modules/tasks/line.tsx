import type { ReactNode } from 'react'
import { nameOf } from '../../api/areas.ts'
import type { Area } from '../../api/types.ts'
import { from } from '../../components/history.ts'
import { ago } from '../../components/time.ts'
import type { Now, Row } from './api.ts'
import { day, period } from './dates.ts'
import type { Group } from './views.ts'

// The name of a task's project, or nothing for a task of the personal area.
export const project = (t: Row, areas: Area[]) => areas.find((a) => a.slug === t.area)?.kind === 'project' && nameOf(areas, t.area)

// The quiet line of an open task in its group: its project, unless the list keeps to one area, who made it and when while
// it is to sort, its plan, or its time alone on Today, its deadline, and its list where the group is not that list.
export function line(t: Row, g: Group, now: Now, areas: Area[], oneArea: boolean): ReactNode[] {
  return [
    !oneArea && project(t, areas),
    g === 'sort' && from(t.source),
    g === 'sort' && ago(t.created_at),
    t.planned_from && (g === 'today' && t.planned_to === t.planned_from ? t.planned_time : `${period(t.planned_from, t.planned_to)}${t.planned_time && ' ' + t.planned_time}`),
    t.deadline && <span className={t.deadline < now.date ? 'late' : undefined}>due {day(t.deadline)}</span>,
    t.list && t.list !== g && t.list,
  ].filter(Boolean)
}
