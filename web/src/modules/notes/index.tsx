import { AddIcon } from '../../components/icons.tsx'
import type { Entry, Module } from '../registry.ts'
import Capture from './Capture.tsx'
import Inbox from './Inbox.tsx'
import { InboxIcon, Waiting } from './nav.tsx'

const inbox: Entry = { to: '/inbox', label: 'Inbox', icon: <InboxIcon />, count: Waiting }

export const notes: Module = {
  name: 'notes',
  home: '/inbox',
  action: { to: '/capture', label: 'Capture', icon: <AddIcon />, form: Capture },
  nav: [inbox],
  tabs: [inbox],
  more: [],
  routes: [{ path: '/inbox', element: <Inbox /> }],
}
