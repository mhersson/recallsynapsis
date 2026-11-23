import { nameOf } from '../../api/areas.ts'
import type { Area } from '../../api/types.ts'
import type { Entry } from './api.ts'
import { day, period } from './dates.ts'

const lists: Record<string, string> = { soon: 'Soon', someday: 'Someday' }

// What an entry of a task's log did, in words; the making says what the task was made with.
export function words(e: Entry, making: boolean, areas: Area[]): string {
  const c = Object.fromEntries(e.changes.map((x) => [x.field, x]))
  const to = (f: string) => c[f]?.to ?? ''
  const from = (f: string) => c[f]?.from ?? ''
  const was = (said: string, old: string) => (old ? `${said}, was ${old}` : said)
  if (making) {
    const held = [
      to('planned_from') && `planned for ${period(to('planned_from'), to('planned_to'))}${to('planned_time') && ` at ${to('planned_time')}`}`,
      to('deadline') && `due ${day(to('deadline'))}`,
      to('list') && `on ${lists[to('list')]}`,
    ].filter(Boolean)
    return ['made it', ...(held.length ? held : ['to sort'])].join(', ')
  }
  const cleared = c.planned_from && !to('planned_from')
  return [
    c.done && (to('done') === 'true' ? 'marked it done' : 'opened it again'),
    c.title && `renamed it “${to('title')}”`,
    cleared && 'cleared the plan',
    !cleared && c.planned_from && c.planned_to && was(`planned it for ${period(to('planned_from'), to('planned_to'))}`, from('planned_from') && period(from('planned_from'), from('planned_to'))),
    !cleared && c.planned_from && !c.planned_to && was(`moved its start to ${day(to('planned_from'))}`, from('planned_from') && day(from('planned_from'))),
    !c.planned_from && c.planned_to && was(`moved its end to ${day(to('planned_to'))}`, day(from('planned_to'))),
    !cleared && c.planned_time && (to('planned_time') ? was(`set it to start at ${to('planned_time')}`, from('planned_time')) : 'cleared its time'),
    c.deadline && (to('deadline') ? was(`set its deadline to ${day(to('deadline'))}`, from('deadline') && day(from('deadline'))) : 'cleared its deadline'),
    c.list && (to('list') ? `put it on ${lists[to('list')]}` : `took it off ${lists[from('list')]}`),
    c.area && was(`moved it to ${nameOf(areas, to('area'))}`, nameOf(areas, from('area'))),
    c.notes && 'changed the notes',
    c.links && 'changed the links',
  ]
    .filter(Boolean)
    .join(', ')
}
