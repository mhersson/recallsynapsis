import { screen, within } from '@testing-library/react'
import type { Session } from '../../api/types.ts'
import { open } from '../../test-app.tsx'
import { fakeServer, json, narrow } from '../../test-server.ts'

// A stand-in module with two rows of More, and one with four tabs, which leave Projects no room among the tabs.
vi.mock('../../modules/registry.ts', () => ({
  modules: [
    {
      name: 'things',
      nav: [],
      tabs: [],
      more: [
        { to: '/ideas', label: 'Ideas', icon: null, note: 'Every idea' },
        { to: '/trash', label: 'Trash', icon: null, note: 'What was trashed' },
      ],
      routes: [],
    },
    {
      name: 'crowd',
      nav: [],
      tabs: ['/one', '/two', '/three', '/four'].map((to) => ({ to, label: to, icon: null })),
      more: [],
      routes: [],
    },
  ],
}))

afterEach(() => vi.unstubAllGlobals())

test.each([
  [[], ['/search', '/settings']],
  [['things'], ['/ideas', '/trash', '/search', '/settings']],
  [['things', 'crowd'], ['/ideas', '/trash', '/projects', '/search', '/settings']],
])('with the modules %j on, More lists %j, each with a second line', async (modules, rows) => {
  narrow(true)
  const session: Session = { user: { name: 'ada' }, modules, version: '1.2.3', timezone: 'UTC', last_backup: null }
  fakeServer((c) => (c.path === '/api/session' ? json(session) : c.path.startsWith('/api/areas') ? json({ areas: [] }) : undefined))
  await open('/more')
  const links = within(screen.getByRole('main')).getAllByRole('link')
  expect(links.map((a) => a.getAttribute('href'))).toEqual(rows)
  expect(links.every((a) => a.lastElementChild!.children.length === 2)).toBe(true)
})
