import { task } from '../../dev/fixtures.ts'
import type { Now, Task } from './api.ts'
import { sortInto, type View } from './views.ts'

const on = (date: string, week_start = 'monday'): Now => ({ date, weekday: '', time: '10:00', timezone: 'UTC', week_start })

// Tasks around the week of Monday 23 November 2026, each named for what it holds.
const tasks: Task[] = [
  task(1, 9, { title: 'to sort' }),
  task(2, 8, { title: 'last weekend', planned_from: '2026-11-21', planned_to: '2026-11-22' }),
  task(3, 7, { title: 'due Friday 20', deadline: '2026-11-20' }),
  task(4, 6, { title: 'Monday', planned_from: '2026-11-23' }),
  task(5, 5, { title: 'due Monday', deadline: '2026-11-23' }),
  task(6, 4, { title: 'this weekend', planned_from: '2026-11-28', planned_to: '2026-11-29', list: 'soon' }),
  task(7, 3, { title: 'Monday 30', planned_from: '2026-11-30' }),
  task(8, 2, { title: 'due Wednesday 25, planned Monday 30', planned_from: '2026-11-30', deadline: '2026-11-25' }),
  task(9, 1, { title: 'last weekend, due Sunday 29', planned_from: '2026-11-21', deadline: '2026-11-29', list: 'someday' }),
  task(10, 0, { title: 'Monday to Friday', planned_from: '2026-11-23', planned_to: '2026-11-27' }),
]

const sorted = (view: View, now: Now) => Object.fromEntries(sortInto(view, tasks, now).map(([g, ts]) => [g, ts.map((t) => t.title)]))

test('on a Monday, Today holds what waits, what was left, what is overdue and the day, and Week the rest of it', () => {
  const now = on('2026-11-23')
  expect(sorted('today', now)).toEqual({
    sort: ['to sort'],
    left: ['last weekend', 'last weekend, due Sunday 29'],
    late: ['due Friday 20'],
    today: ['Monday', 'due Monday', 'Monday to Friday'],
  })
  expect(sorted('week', now)).toEqual({
    week: ['this weekend', 'due Wednesday 25, planned Monday 30', 'last weekend, due Sunday 29'],
    later: ['Monday 30'],
  })
  expect(sorted('soon', now)).toEqual({ soon: ['this weekend'] })
  expect(sorted('someday', now)).toEqual({ someday: ['last weekend, due Sunday 29'] })
})

test('on the Saturday, the weekend is today, a deadline passed is overdue, and Monday 30 is later', () => {
  const now = on('2026-11-28')
  expect(sorted('today', now)).toEqual({
    sort: ['to sort'],
    left: ['last weekend', 'Monday', 'last weekend, due Sunday 29', 'Monday to Friday'],
    late: ['due Friday 20', 'due Monday', 'due Wednesday 25, planned Monday 30'],
    today: ['this weekend'],
  })
  expect(sorted('week', now)).toEqual({ week: ['last weekend, due Sunday 29'], later: ['Monday 30', 'due Wednesday 25, planned Monday 30'] })
})

test('on the Sunday, a week from Monday has nothing left of it, and a week from Sunday holds Monday 30', () => {
  const later = ['Monday 30', 'due Wednesday 25, planned Monday 30']
  expect(sorted('today', on('2026-11-29')).today).toEqual(['this weekend'])
  expect(sorted('week', on('2026-11-29'))).toEqual({ week: [], later })
  expect(sorted('week', on('2026-11-29', 'sunday'))).toEqual({ week: later, later: [] })
})

test('with weeks from Sunday, the week ends on the Saturday, so a deadline on its Sunday is later', () => {
  expect(sorted('week', on('2026-11-27', 'sunday'))).toEqual({ week: ['this weekend'], later: ['Monday 30', 'due Wednesday 25, planned Monday 30', 'last weekend, due Sunday 29'] })
  expect(sorted('week', on('2026-11-27'))).toEqual({ week: ['this weekend', 'last weekend, due Sunday 29'], later: ['Monday 30', 'due Wednesday 25, planned Monday 30'] })
})
