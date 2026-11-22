import { createContext, useContext } from 'react'
import type { Session } from './api/types.ts'
import { modules, type Item, type Module } from './modules/registry.ts'

// The shell provides it once read, so every page below the shell has one.
export const SessionContext = createContext<Session | null>(null)

export function useSession(): Session {
  return useContext(SessionContext)!
}

// The modules of the registry that the server names as switched on, in the registry's order.
export function enabled(names: string[]): Module[] {
  return modules.filter((m) => names.includes(m.name))
}

export function useModules(): Module[] {
  return enabled(useSession().modules)
}

// The item a reference names, from the module switched on that its reference names; undefined when that module gives none.
export function useItem(): (ref: string, kind: string, state: string) => Item | undefined {
  const on = useModules()
  return (ref, kind, state) => on.find((m) => ref.startsWith(m.name + ':'))?.item?.(ref, kind, state)
}
