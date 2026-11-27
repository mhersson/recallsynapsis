import { cameIn } from './history.ts'

const now = Date.parse('2026-11-22T12:00:00Z')
const back = (s: number) => new Date(now - s * 1000).toISOString()

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(now)
})
afterEach(() => vi.useRealTimers())

test('a change under a minute old came in just now, an older one as long ago as it was, each by the web app or via its token', () => {
  expect(cameIn(back(30), 'web')).toBe('just now, by the web app')
  expect(cameIn(back(59), 'aim')).toBe('just now, via aim')
  expect(cameIn(back(180), 'aim')).toBe('3 min ago, via aim')
  expect(cameIn(back(180), 'web')).toBe('3 min ago, by the web app')
})
