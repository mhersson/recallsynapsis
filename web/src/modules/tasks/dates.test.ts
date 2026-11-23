import type { Now } from './api.ts'
import { add, day, period, shortcuts, stamp, week } from './dates.ts'

const on = (date: string, week_start = 'monday'): Now => ({ date, weekday: '', time: '10:00', timezone: 'Europe/Oslo', week_start })

test('a period is written as one day, within a month and across two', () => {
  expect(period('2026-11-28', '2026-11-28')).toBe('Sat 28 Nov')
  expect(period('2026-11-28', '2026-11-29')).toBe('Sat-Sun 28-29 Nov')
  expect(period('2026-11-30', '2026-12-06')).toBe('Mon-Sun 30 Nov-6 Dec')
  expect(period('2026-12-28', '2027-01-03')).toBe('Mon-Sun 28 Dec-3 Jan')
})

test('dates are counted as plain days, across a month, a year and a change of the clock', () => {
  expect(add('2026-11-30', 1)).toBe('2026-12-01')
  expect(add('2026-12-31', 1)).toBe('2027-01-01')
  expect(add('2026-03-29', -1)).toBe('2026-03-28')
  expect(add('2026-10-25', 1)).toBe('2026-10-26')
})

test.each([
  ['a Monday', on('2026-11-23'), ['2026-11-23', '2026-11-29'], [['2026-11-28', '2026-11-29'], ['2026-11-30', '2026-12-06']]],
  ['a Saturday', on('2026-11-28'), ['2026-11-23', '2026-11-29'], [['2026-11-28', '2026-11-29'], ['2026-11-30', '2026-12-06']]],
  ['a Sunday', on('2026-11-29'), ['2026-11-23', '2026-11-29'], [['2026-11-29', '2026-11-29'], ['2026-11-30', '2026-12-06']]],
  ['a Sunday, weeks from Sunday', on('2026-11-29', 'sunday'), ['2026-11-29', '2026-12-05'], [['2026-11-29', '2026-11-29'], ['2026-12-06', '2026-12-12']]],
  ['a Saturday, weeks from Sunday', on('2026-11-28', 'sunday'), ['2026-11-22', '2026-11-28'], [['2026-11-28', '2026-11-29'], ['2026-11-29', '2026-12-05']]],
])('on %s, the week and the shortcuts Weekend and Next week', (_, now, days, [weekend, next]) => {
  expect(week(now)).toEqual(days)
  const [today, tomorrow, w, n] = shortcuts(now)
  expect(today.slice(1)).toEqual([now.date, now.date])
  expect(tomorrow.slice(1)).toEqual([add(now.date, 1), add(now.date, 1)])
  expect(w.slice(1)).toEqual(weekend)
  expect(n.slice(1)).toEqual(next)
})

test('an instant is told on the server clock, by its day unless it is today, on a 24-hour clock', () => {
  const now = on('2026-11-23')
  const today = [stamp('2026-11-23T13:05:00Z', now), stamp('2026-11-22T23:30:00Z', now)]
  expect(today.map((s) => s.slice(-5))).toEqual(['14:05', '00:30'])
  expect(today.some((s) => s.includes(day(now.date)))).toBe(false)
  expect(stamp('2026-11-22T19:15:00Z', now)).toBe(`${day('2026-11-22')} 20:15`)
})
