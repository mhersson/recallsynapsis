import { Link, useLocation } from 'react-router'
import type { Action, Entry, Module } from '../modules/registry.ts'
import { projectsTabbed } from '../session.ts'
import { MoreIcon, ProjectsIcon } from './icons.tsx'
import { back } from './useClose.ts'
import './PhoneTabs.css'

const projects: Entry = { to: '/projects', label: 'Projects', icon: <ProjectsIcon /> }
const more: Entry = { to: '/more', label: 'More', icon: <MoreIcon /> }

type Props = { modules: Module[]; action?: Action; area?: number }

export default function PhoneTabs({ modules, action, area }: Props) {
  const { pathname, search, state } = useLocation()
  // A project, and a page of a module's tab of a project, keep Projects current, and a page reached through More keeps More
  // current, Projects too when it has no tab.
  const tabbed = projectsTabbed(modules)
  const project = ['/areas', ...modules.flatMap((m) => (m.tab ? [m.tab.at] : []))]
  const also: Record<string, string[]> = {
    '/projects': project,
    '/more': ['/search', '/settings', ...(tabbed ? [] : ['/projects', ...project]), ...modules.flatMap((m) => m.more.map((r) => r.to))],
  }
  const current = (to: string) => [to, ...(also[to] ?? [])].some((p) => pathname === p || pathname.startsWith(p + '/'))
  // The action's tab is green whether its page is open or not.
  const tab = (t: Entry, to = t.to, carried?: unknown) => (
    <Link key={t.to} to={to} state={carried} className={t === action ? 'act' : current(t.to) ? 'on' : undefined} aria-current={current(t.to) ? 'page' : undefined}>
      {t.icon}
      <span className="lbl">
        {t.label}
        {t.count && (
          <span className="cnt">
            <t.count />
          </span>
        )}
      </span>
    </Link>
  )

  const tabs = [...modules.flatMap((m) => m.tabs), ...(tabbed ? [projects] : []), more].map((t) => tab(t))
  if (action) {
    // Pressed again on its own page, the action's tab leaves that page as it is; from elsewhere it tells the page to close back.
    const here = current(action.to)
    tabs.splice(Math.floor(tabs.length / 2), 0, tab(action, here ? pathname + search : area ? `${action.to}?area=${area}` : action.to, here ? state : back))
  }
  return <nav className="tabs">{tabs}</nav>
}
