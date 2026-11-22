import { act, renderHook } from '@testing-library/react'
import type { Look } from './theme.ts'

function fakeSystem(startDark: boolean) {
  const listeners = new Set<() => void>()
  const mql = {
    matches: startDark,
    addEventListener: (_: string, l: () => void) => listeners.add(l),
    removeEventListener: (_: string, l: () => void) => listeners.delete(l),
  }
  window.matchMedia = (() => mql) as unknown as typeof window.matchMedia
  return (dark: boolean) => {
    mql.matches = dark
    listeners.forEach((l) => l())
  }
}

async function load() {
  vi.resetModules()
  return import('./theme.ts')
}

const root = document.documentElement

test('the look follows the system until the switch is used', async () => {
  const setSystem = fakeSystem(true)
  const { useLook } = await load()
  expect(root.dataset.theme).toBe('dark')
  expect(root.dataset.palette).toBe('copper')

  act(() => setSystem(false))
  expect(root.dataset.theme).toBe('light')

  const { result } = renderHook(() => useLook())
  act(() => result.current[1]({ theme: 'light', palette: 'copper' }))
  act(() => setSystem(true))
  expect(root.dataset.theme).toBe('light')

  act(() => result.current[1]({ theme: 'system', palette: 'copper' }))
  expect(root.dataset.theme).toBe('dark')
})

test('a chosen look is kept and wins', async () => {
  fakeSystem(false)
  localStorage.setItem('theme', 'dark')
  localStorage.setItem('palette', 'catppuccin')
  const { useLook } = await load()
  expect(root.dataset.theme).toBe('dark')
  expect(root.dataset.palette).toBe('catppuccin')

  const a = renderHook(() => useLook())
  const b = renderHook(() => useLook())
  expect(a.result.current[0]).toEqual({ theme: 'dark', palette: 'catppuccin' })

  const next: Look = { theme: 'light', palette: 'copper' }
  act(() => a.result.current[1](next))
  expect(b.result.current[0]).toEqual(next)
  expect(localStorage.getItem('theme')).toBe('light')
  expect(localStorage.getItem('palette')).toBe('copper')

  act(() => a.result.current[1]({ theme: 'system', palette: 'copper' }))
  expect(localStorage.getItem('theme')).toBeNull()
})

test('a stored value that is not a look counts as absent', async () => {
  fakeSystem(true)
  localStorage.setItem('theme', 'purple')
  localStorage.setItem('palette', 'everforest')
  const { useLook } = await load()
  expect(renderHook(() => useLook()).result.current[0]).toEqual({ theme: 'system', palette: 'copper' })
  expect(root.dataset.theme).toBe('dark')
})
