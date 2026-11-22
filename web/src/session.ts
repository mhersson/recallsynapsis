import { createContext, useContext } from 'react'
import type { Session } from './api/types.ts'
import { modules, type Module } from './modules/registry.ts'

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
