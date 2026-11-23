import type { Module } from '../registry.ts'
import Notifications from './Notifications.tsx'
import { turnOff } from './push.ts'

// Push has no page of its own: it is a part of Settings, and turns this browser off before the session ends.
export const push: Module = {
  name: 'push',
  nav: [],
  tabs: [],
  more: [],
  routes: [],
  settings: { id: 'notifications', title: 'Notifications', body: <Notifications /> },
  signOut: turnOff,
}
