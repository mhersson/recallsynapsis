import { act, renderHook } from '@testing-library/react'
import { FakeEventSource } from '../test-server.ts'
import { useEvents, useStream } from './stream.ts'
import type { Change } from './types.ts'

beforeEach(() => {
  vi.useFakeTimers()
  FakeEventSource.all = []
  vi.stubGlobal('EventSource', FakeEventSource)
  vi.setSystemTime(10_000)
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

const last = () => FakeEventSource.all[FakeEventSource.all.length - 1]
const change = (id: number): Change => ({ module: 'notes', kind: 'note', id, area: 1 })

// The shell's stream, and a page that subscribes to it.
function setup() {
  const read = vi.fn<(changes: Change[] | null) => void>()
  const shell = renderHook(() => useEvents())
  const page = renderHook(() => useStream(read))
  return { read, shell, page }
}

test('a closed stream opens again after a delay that doubles to a cap', () => {
  setup()
  expect(last().url).toBe('/api/events')
  act(() => last().open())

  act(() => last().fail(true))
  expect(FakeEventSource.all).toHaveLength(1)
  act(() => vi.advanceTimersByTime(1000))
  expect(FakeEventSource.all).toHaveLength(2)
  expect(last().url).toBe('/api/events')

  act(() => last().fail(true))
  act(() => vi.advanceTimersByTime(1999))
  expect(FakeEventSource.all).toHaveLength(2)
  act(() => vi.advanceTimersByTime(1))
  expect(FakeEventSource.all).toHaveLength(3)
  for (let i = 0; i < 8; i++) {
    act(() => last().fail(true))
    act(() => vi.advanceTimersByTime(30000))
  }
  const n = FakeEventSource.all.length
  act(() => last().fail(true))
  act(() => vi.advanceTimersByTime(30000))
  expect(FakeEventSource.all).toHaveLength(n + 1)

  // An open resets the delay.
  act(() => last().open())
  act(() => last().fail(true))
  act(() => vi.advanceTimersByTime(1000))
  expect(FakeEventSource.all).toHaveLength(n + 2)
})

test("a reconnect reads the page's data again", () => {
  const { read } = setup()
  act(() => last().open())
  read.mockClear()

  // The browser reconnects a source that is still connecting by itself.
  act(() => last().fail(false))
  act(() => last().open())
  expect(read).toHaveBeenCalledTimes(1)
  expect(read).toHaveBeenLastCalledWith(null)
  expect(FakeEventSource.all).toHaveLength(1)

  act(() => last().fail(true))
  act(() => vi.advanceTimersByTime(1000))
  act(() => last().open())
  expect(read).toHaveBeenCalledTimes(2)
})

test('live is false after an error and true at the next open', () => {
  const { shell } = setup()
  expect(shell.result.current.live).toBe(false)
  act(() => last().open())
  expect(shell.result.current.live).toBe(true)
  act(() => last().fail(false))
  expect(shell.result.current.live).toBe(false)
  act(() => last().open())
  expect(shell.result.current.live).toBe(true)

  const source = last()
  shell.unmount()
  expect(source.readyState).toBe(FakeEventSource.CLOSED)
})

test('a new read does not reopen the stream', () => {
  const first = vi.fn()
  const second = vi.fn()
  renderHook(() => useEvents())
  const { rerender } = renderHook(({ f }) => useStream(f), { initialProps: { f: first } })
  rerender({ f: second })
  expect(FakeEventSource.all).toHaveLength(1)
  first.mockClear()
  act(() => last().send(change(3)))
  act(() => vi.advanceTimersByTime(2000))
  expect(first).not.toHaveBeenCalled()
  expect(second).toHaveBeenCalledTimes(1)
})

test('a page reads once when it mounts', () => {
  const { read } = setup()
  expect(read).toHaveBeenCalledTimes(1)
  expect(read).toHaveBeenCalledWith(null)
})

test('a burst of changes leads to one read with all of them', () => {
  const { read } = setup()
  act(() => last().open())
  read.mockClear()

  // The mount read was just now, so the burst waits out the gap.
  act(() => last().send(change(7)))
  act(() => last().send(change(9)))
  expect(read).not.toHaveBeenCalled()
  act(() => vi.advanceTimersByTime(2000))
  expect(read).toHaveBeenCalledTimes(1)
  expect(read).toHaveBeenCalledWith([change(7), change(9)])

  // The last read is old enough, so the next burst reads at once.
  act(() => vi.advanceTimersByTime(5000))
  act(() => last().send(change(8)))
  act(() => vi.advanceTimersByTime(0))
  expect(read).toHaveBeenCalledTimes(2)
  expect(read).toHaveBeenLastCalledWith([change(8)])
})

test('a reconnect reads everything at once', () => {
  const { read } = setup()
  act(() => last().open())
  read.mockClear()
  act(() => last().send(change(7)))
  act(() => last().fail(false))
  act(() => last().open())
  expect(read).toHaveBeenCalledTimes(1)
  expect(read).toHaveBeenCalledWith(null)

  // The pending burst was folded into the reconnect's read.
  act(() => vi.advanceTimersByTime(2000))
  expect(read).toHaveBeenCalledTimes(1)
})

test('a page that unmounts hears nothing more', () => {
  const { read, page } = setup()
  act(() => last().open())
  act(() => last().send(change(7)))
  read.mockClear()
  page.unmount()
  act(() => vi.advanceTimersByTime(5000))
  act(() => last().send(change(7)))
  act(() => last().fail(false))
  act(() => last().open())
  act(() => vi.advanceTimersByTime(5000))
  expect(read).not.toHaveBeenCalled()
})
