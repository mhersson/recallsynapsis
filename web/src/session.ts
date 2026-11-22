import { createContext, useContext } from 'react'
import type { Session } from './api/types.ts'
import type { Item, Module } from './modules/registry.ts'

// The shell provides it once read, so every page below the shell has one.
export const SessionContext = createContext<Session | null>(null)

// The action of the modules switched on, which the shell opens for an area: in a dialog over the page on desktop, as its
// page on a phone. Null when no module switched on has one.
export const ActionContext = createContext<{ label: string; open: (area: number) => void } | null>(null)

export const useAction = () => useContext(ActionContext)

export function useSession(): Session {
  return useContext(SessionContext)!
}

// The modules switched on, which the shell provides. It is not read from the registry here, since a module that asks
// for it would import the registry that imports the module.
export const ModulesContext = createContext<Module[]>([])

export const useModules = () => useContext(ModulesContext)

// The item a reference names, from the module switched on that its reference names; undefined when that module gives none.
export function useItem(): (ref: string, kind: string, state: string) => Item | undefined {
  const on = useModules()
  return (ref, kind, state) => on.find((m) => ref.startsWith(m.name + ':'))?.item?.(ref, kind, state)
}
