import type { ComponentType, ReactNode } from 'react'
import type { Area } from '../api/types.ts'
import { notes } from './notes/index.tsx'
import { tasks } from './tasks/index.tsx'

// A place the shell links to: an entry of the side bar, a phone tab or a row of More. count renders the number the
// shell shows beside an entry or a tab, or nothing.
export type Entry = { to: string; label: string; icon: ReactNode; count?: ComponentType }

// A module's one action, such as Capture. The shell opens form in a dialog over the page on desktop, and as a page at
// to on a phone. Opened from an area's page, it is given that area's id, which the page takes as ?area=.
export type Action = Pick<Entry, 'to' | 'label' | 'icon'> & { form: ComponentType<{ area?: number; dialog?: boolean; onClose: () => void }> }

// Where an item leads, the class of the small mark that shows what it is, and its state in words when it has one
// to show.
export type Item = { to: string; mark: string; state?: string }

// What a module adds to the web app; the shell shows it only while GET /api/session names it.
export type Module = {
  name: string
  // The page a person lands on, when this is the first enabled module that names one; else they land on the Projects list.
  home?: string
  // The green button of the side bar and the centre tab on a phone, when this is the first enabled module that has one.
  action?: Action
  // In the side bar, above Search.
  nav: Entry[]
  // On a phone, before Projects and More.
  tabs: Entry[]
  // Rows of More, with a second line each. Their pages keep the More tab current.
  more: (Entry & { note: string })[]
  routes: { path: string; element: ReactNode }[]
  // What it holds in an area, shown on that area's page.
  section?: ComponentType<{ area: Area }>
  // An item of the module, given its reference, kind and state, as a hit or a link names it.
  item?: (ref: string, kind: string, state: string) => Item
}

// Every module, one line each, in the order the navigation shows them.
export const modules: Module[] = [tasks, notes]
