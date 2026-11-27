import type { ComponentType, ReactNode } from 'react'
import type { Area, Linked } from '../api/types.ts'
import { knowledge } from './knowledge/index.tsx'
import { notes } from './notes/index.tsx'
import { push } from './push/index.tsx'
import { tasks } from './tasks/index.tsx'

// A place the shell links to: an entry of the side bar, a phone tab or a row of More. count renders the number the
// shell shows beside an entry or a tab, or nothing.
export type Entry = { to: string; label: string; icon: ReactNode; count?: ComponentType }

// A module's one action, such as Capture. The shell opens form in a dialog over the page on desktop, and as a page at
// to on a phone. Opened from an area's page, it is given that area's id, which the page takes as ?area=.
export type Action = Pick<Entry, 'to' | 'label' | 'icon'> & { form: ComponentType<{ area?: number; dialog?: boolean; onClose: () => void }> }

// A kind a module adds to Capture beside Note and Idea. Chosen, Capture goes to an area, never to the Inbox, and shows more
// under its text, which keeps what is chosen there; save sends the text to the area with it, and throws a refusal.
export type CaptureKind = {
  name: string
  label: string
  more: ComponentType<{ value: Record<string, string>; onChange: (v: Record<string, string>) => void }>
  save: (text: string, area: string, more: Record<string, string>) => Promise<unknown>
}

// An item, as a link names it.
export type Named = Pick<Linked, 'ref' | 'kind' | 'state' | 'title'>

// What a module makes of another module's item, such as a task of a note: its label; its form, given the item, its text,
// its area and the areas, which tells whether anything is entered and closes with what it made, if anything; and atOnce,
// which makes one of a text in an area, with the owner's words, or throws a refusal.
export type Maker = {
  label: string
  form: ComponentType<{ of: Named; text: string; area: string; areas: Area[]; onEntered: (entered: boolean) => void; onClose: (made?: Named) => void }>
  atOnce: (text: string, area: string, said: string) => Promise<Named>
}

// Where an item leads, the class of the small mark that shows what it is, its state in words when it has one to show,
// and the tag a Links row shows after it, if any.
export type Item = { to: string; mark: string; state?: string; tag?: string }

// A tab of a project's page, after Activity, which holds the modules' sections: its name in the address, its label, the
// quiet number after the label, which renders nothing when there is none, what it shows, and the path the module's own
// pages are under, which keep Projects current on a phone, or More when Projects has no tab.
export type Tab = { name: string; label: string; count?: ComponentType<{ area: Area }>; body: ComponentType<{ area: Area }>; at: string }

// A part of the Settings page: its anchor, its title and what it holds.
export type Part = { id: string; title: string; body: ReactNode }

// What a module adds to the web app; the shell shows it only while GET /api/session names it.
export type Module = {
  name: string
  // The page a person lands on, when this is the first enabled module that names one; else they land on the Projects list.
  home?: string
  // The green button of the side bar and the centre tab on a phone, when this is the first enabled module that has one.
  action?: Action
  // What it adds to Capture.
  capture?: CaptureKind[]
  // In the side bar, above Search.
  nav: Entry[]
  // On a phone, before Projects and More.
  tabs: Entry[]
  // Rows of More, with a second line each. Their pages keep the More tab current.
  more: (Entry & { note: string })[]
  routes: { path: string; element: ReactNode }[]
  // What it holds in an area, shown on that area's page beside the other modules' on desktop; first when it comes first
  // there, so on a phone it opens.
  section?: ComponentType<{ area: Area; first: boolean }>
  // A tab of a project's page, after Activity.
  tab?: Tab
  // What it makes from another module's item, offered where that item is shown.
  make?: Maker
  // An item of the module, given its reference, kind and state, as a hit or a link names it.
  item?: (ref: string, kind: string, state: string) => Item
  // A part of Settings, after Backups.
  settings?: Part
  // Its kinds of notification, which the part Notifications shows under this browser's lines.
  notify?: ComponentType
  // What it does before the session ends, as a person signs out; the session ends whether it worked or not.
  signOut?: () => Promise<unknown>
}

// Every module, one line each, in the order the navigation shows them.
export const modules: Module[] = [tasks, notes, knowledge, push]
