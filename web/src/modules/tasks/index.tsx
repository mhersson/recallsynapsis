import type { Entry, Module } from '../registry.ts'
import { TasksIcon, Unsorted } from './nav.tsx'
import Tasks from './Tasks.tsx'

const entry: Entry = { to: '/tasks', label: 'Tasks', icon: <TasksIcon />, count: Unsorted }

export const tasks: Module = {
  name: 'tasks',
  home: '/tasks',
  nav: [entry],
  tabs: [entry],
  more: [],
  routes: [{ path: '/tasks/:id?', element: <Tasks /> }],
}
