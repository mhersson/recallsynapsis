import { createContext, useContext, useEffect } from 'react'
import type { Group, Session } from './api/types.ts'
import type { Item, Module } from './modules/registry.ts'

// The shell provides it once read, so every page below the shell has one.
export const SessionContext = createContext<Session | null>(null)

// The action of the modules switched on, which the shell opens for an area: in a dialog over the page on desktop, as its
// page on a phone. Null when no module switched on has one.
export const ActionContext = createContext<{ label: string; open: (area: number) => void } | null>(null)

export const useAction = () => useContext(ActionContext)

// What tells the shell the area a page belongs to when its address names none, so the side bar marks it as current.
export const AreaShownContext = createContext<(id?: number) => void>(() => {})

// Marks the area id as the page's while the page is open.
export function useAreaShown(id?: number) {
  const show = useContext(AreaShownContext)
  useEffect(() => {
    show(id)
    return () => show(undefined)
  }, [show, id])
}

export function useSession(): Session {
  return useContext(SessionContext)!
}

// The modules switched on, which the shell provides. It is not read from the registry here, since a module that asks
// for it would import the registry that imports the module.
export const ModulesContext = createContext<Module[]>([])

export const useModules = () => useContext(ModulesContext)

// Whether Projects is a tab on a phone: while the modules' tabs, their action's and More leave it room among five.
export const projectsTabbed = (on: Module[]) => on.flatMap((m) => m.tabs).length + (on.some((m) => m.action) ? 1 : 0) + 2 <= 5

// The groups of a search in the order of the registry, which the server does not keep; a module switched off comes last.
export function useRanked(): (groups: Group[]) => Group[] {
  const names = useModules().map((m) => m.name)
  const rank = (g: Group) => (names.includes(g.module) ? names.indexOf(g.module) : names.length)
  return (groups) => [...groups].sort((a, b) => rank(a) - rank(b))
}

// The item a reference names, from the module switched on that its reference names; undefined when that module gives none.
export function useItem(): (ref: string, kind: string, state: string) => Item | undefined {
  const on = useModules()
  return (ref, kind, state) => on.find((m) => ref.startsWith(m.name + ':'))?.item?.(ref, kind, state)
}
