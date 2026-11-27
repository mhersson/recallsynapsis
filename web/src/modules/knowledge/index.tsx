import type { Module } from '../registry.ts'
import { mark, pathOf } from './api.ts'
import Decision from './Decision.tsx'
import Decisions, { Count } from './Decisions.tsx'
import New from './New.tsx'

export const knowledge: Module = {
  name: 'knowledge',
  nav: [],
  tabs: [],
  more: [],
  routes: [
    { path: '/decisions/new', element: <New /> },
    { path: '/decisions/:id', element: <Decision /> },
  ],
  tab: { name: 'decisions', label: 'Decisions', count: Count, body: Decisions, at: '/decisions' },
  // A reference is knowledge:ID; a replaced decision is tagged so in a Links row, where its grey mark alone would not say it.
  item: (ref, _kind, state) => ({ to: pathOf(ref), mark: mark(state), state, tag: state === 'replaced' ? 'replaced' : undefined }),
}
