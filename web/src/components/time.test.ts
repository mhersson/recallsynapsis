import { ago, clock } from './time.ts'

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
