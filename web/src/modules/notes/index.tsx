import { AddIcon } from '../../components/icons.tsx'
import type { Entry, Module } from '../registry.ts'
import { mark } from './api.ts'
import Capture from './Capture.tsx'
import Inbox from './Inbox.tsx'
import { IdeasIcon, InboxIcon, NotesIcon, Waiting } from './nav.tsx'
import Notes from './Notes.tsx'

const inbox: Entry = { to: '/inbox', label: 'Inbox', icon: <InboxIcon />, count: Waiting }
const notesEntry: Entry = { to: '/notes', label: 'Notes', icon: <NotesIcon /> }
const ideas: Entry = { to: '/ideas', label: 'Ideas', icon: <IdeasIcon /> }

export const notes: Module = {
  name: 'notes',
  home: '/inbox',
  action: { to: '/capture', label: 'Capture', icon: <AddIcon />, form: Capture },
  nav: [inbox, notesEntry, ideas],
  tabs: [inbox, notesEntry],
  more: [{ ...ideas, label: 'Idea bank', note: 'Every idea, across the areas' }],
  routes: [
    { path: '/inbox', element: <Inbox /> },
    // Keyed, so going between the two starts the page afresh rather than showing one list under the other's head.
    { path: '/notes/:id?', element: <Notes key="notes" /> },
    { path: '/ideas/:id?', element: <Notes key="ideas" bank /> },
  ],
  // A reference is notes:ID, and a capture not yet filed is in the state inbox.
  item: (ref, kind, state) => ({
    to: `/notes/${ref.slice('notes:'.length)}`,
    mark: mark(kind, state === 'inbox'),
    state: state === 'inbox' ? 'in the inbox' : undefined,
  }),
}
