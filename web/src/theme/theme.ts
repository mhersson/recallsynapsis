import { useSyncExternalStore } from 'react'

export type Look = { theme: 'system' | 'dark' | 'light'; palette: 'copper' | 'catppuccin' }

const dark = window.matchMedia('(prefers-color-scheme: dark)')
const listeners = new Set<() => void>()

// Browsers that block site data throw on access; the look then is not kept.
function read(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}

function write(key: string, value: string | null) {
  try {
    if (value === null) localStorage.removeItem(key)
    else localStorage.setItem(key, value)
  } catch {
    // not kept
  }
}

function load(): Look {
  const theme = read('theme')
  return {
    theme: theme === 'dark' || theme === 'light' ? theme : 'system',
    palette: read('palette') === 'catppuccin' ? 'catppuccin' : 'copper',
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
  write('theme', look.theme === 'system' ? null : look.theme)
  write('palette', look.palette)
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
