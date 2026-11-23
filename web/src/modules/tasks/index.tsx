import type { Entry, Module } from '../registry.ts'
import { ring, type State } from './api.ts'
import AreaTasks from './AreaTasks.tsx'
import { TasksIcon, Unsorted } from './nav.tsx'
import Tasks from './Tasks.tsx'
import Trashed from './Trashed.tsx'

const entry: Entry = { to: '/tasks', label: 'Tasks', icon: <TasksIcon />, count: Unsorted }

export const tasks: Module = {
  name: 'tasks',
  home: '/tasks',
  nav: [entry],
  tabs: [entry],
  more: [],
  routes: [
    { path: '/tasks/:id?', element: <Tasks /> },
    { path: '/tasks/trashed/:id?', element: <Trashed /> },
  ],
  section: AreaTasks,
  // A reference is tasks:ID. The ring tells an open task, so only done and to sort are said.
  item: (ref, _kind, state) => ({
    to: `/tasks/${ref.slice('tasks:'.length)}`,
    mark: ring(state as State),
    state: state === 'done' ? 'done' : state === 'unsorted' ? 'to sort' : undefined,
  }),
}
