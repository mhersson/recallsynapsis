import type { ReactNode } from 'react'

// A place the shell links to: an entry of the side bar, a phone tab or a row of More.
export type Entry = { to: string; label: string; icon: ReactNode }

// What a module adds to the web app; the shell shows it only while GET /api/session names it.
export type Module = {
  name: string
  // The page a person lands on, when this is the first enabled module that names one; else they land on the Projects list.
  home?: string
  // In the side bar, above Search.
  nav: Entry[]
  // On a phone, before Projects and More.
  tabs: Entry[]
  // Rows of More, with a second line each. Their pages keep the More tab current.
  more: (Entry & { note: string })[]
  routes: { path: string; element: ReactNode }[]
}

// Every module, one line each, in the order the navigation shows them.
export const modules: Module[] = []
