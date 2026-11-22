import { AddIcon } from '../../components/icons.tsx'
import type { Entry, Module } from '../registry.ts'
import Capture from './Capture.tsx'
import Inbox from './Inbox.tsx'
import { InboxIcon, NotesIcon, Waiting } from './nav.tsx'
import Notes from './Notes.tsx'

const inbox: Entry = { to: '/inbox', label: 'Inbox', icon: <InboxIcon />, count: Waiting }
const notesEntry: Entry = { to: '/notes', label: 'Notes', icon: <NotesIcon /> }

export const notes: Module = {
  name: 'notes',
  home: '/inbox',
  action: { to: '/capture', label: 'Capture', icon: <AddIcon />, form: Capture },
  nav: [inbox, notesEntry],
  tabs: [inbox, notesEntry],
  more: [],
  routes: [
    { path: '/inbox', element: <Inbox /> },
    { path: '/notes/:id?', element: <Notes /> },
  ],
}
