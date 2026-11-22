import { createContext, useContext } from 'react'
import type { Session } from './api/types.ts'

// The shell provides it once read, so every page below the shell has one.
export const SessionContext = createContext<Session | null>(null)

export function useSession(): Session {
  return useContext(SessionContext)!
}
