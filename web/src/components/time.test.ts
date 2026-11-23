import { ago, clock, moment, when } from './time.ts'

const min = 60_000
const now = Date.parse('2026-11-22T12:00:00Z')

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(now)
})
afterEach(() => vi.useRealTimers())

test.each([
  [0, '0 min ago'],
  [59 * min, '59 min ago'],
  [60 * min, '1 h ago'],
  [90 * min, '1 h 30 min ago'],
  [47 * 60 * min, '47 h ago'],
  [48 * 60 * min, '2 days ago'],
])('ago(%i ms back) is %s', (ms, want) => {
  expect(ago(new Date(now - ms).toISOString())).toBe(want)
})

test('clock gives the time alone today and the day before it otherwise', () => {
  const today = new Date(now)
  today.setHours(9, 5)
  expect(clock(today.toISOString())).toBe('09:05')
  const before = new Date(today.getTime() - 3 * 24 * 60 * min)
  expect(clock(before.toISOString())).toMatch(/.+ 09:05$/)
})

test('when names the day only when it is not today', () => {
  const today = new Date(now)
  today.setHours(3, 0)
  const before = new Date(today.getTime() - 3 * 24 * 60 * min)
  expect(when(today.toISOString())).toMatch(/ 03:00$/)
  expect(when(before.toISOString())).toMatch(/ 03:00$/)
  expect(when(before.toISOString())).not.toBe(when(today.toISOString()))
})

test('moment says today before the time, and names another day instead', () => {
  const today = new Date(now)
  today.setHours(14, 0)
  const before = new Date(today.getTime() - 3 * 24 * 60 * min)
  expect(moment(today.toISOString())).toBe('today 14:00')
  expect(moment(before.toISOString())).toBe(clock(before.toISOString()))
})
