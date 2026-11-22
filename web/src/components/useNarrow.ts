import { useSyncExternalStore } from 'react'

const query = '(max-width: 720px)'

function subscribe(onChange: () => void) {
  const mql = window.matchMedia(query)
  mql.addEventListener('change', onChange)
  return () => mql.removeEventListener('change', onChange)
}

// True on a phone: the shell then has tabs at the bottom and no side bar.
export function useNarrow(): boolean {
  return useSyncExternalStore(subscribe, () => window.matchMedia(query).matches)
}
