import { useSyncExternalStore } from 'react'
import { store, stored } from '../components/stored.ts'

export type Look = { theme: 'system' | 'dark' | 'light'; palette: 'copper' | 'catppuccin' }

const dark = window.matchMedia('(prefers-color-scheme: dark)')
const listeners = new Set<() => void>()

function load(): Look {
  const theme = stored('theme')
  return {
    theme: theme === 'dark' || theme === 'light' ? theme : 'system',
    palette: stored('palette') === 'catppuccin' ? 'catppuccin' : 'copper',
  }
}

let look = load()

function apply() {
  const root = document.documentElement
  root.dataset.theme = look.theme === 'system' ? (dark.matches ? 'dark' : 'light') : look.theme
  root.dataset.palette = look.palette
}

function setLook(next: Look) {
  look = { ...next }
  store('theme', look.theme === 'system' ? null : look.theme)
  store('palette', look.palette)
  apply()
  listeners.forEach((l) => l())
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => {
    listeners.delete(l)
  }
}

apply()
dark.addEventListener('change', apply)

export function useLook(): [Look, (l: Look) => void] {
  return [useSyncExternalStore(subscribe, () => look), setLook]
}
