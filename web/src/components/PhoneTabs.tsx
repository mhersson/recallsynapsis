import { Link, useLocation } from 'react-router'
import type { Entry, Module } from '../modules/registry.ts'
import { MoreIcon, ProjectsIcon } from './icons.tsx'
import './PhoneTabs.css'

const core: Entry[] = [
  { to: '/', label: 'Projects', icon: <ProjectsIcon /> },
  { to: '/more', label: 'More', icon: <MoreIcon /> },
]

export default function PhoneTabs({ modules }: { modules: Module[] }) {
  const { pathname } = useLocation()
  // A project keeps Projects current, and a page reached through More keeps More current.
  const also: Record<string, string[]> = {
    '/': ['/areas'],
    '/more': ['/search', '/settings', ...modules.flatMap((m) => m.more.map((r) => r.to))],
  }
  const current = (to: string) => [to, ...(also[to] ?? [])].some((p) => pathname === p || pathname.startsWith(p + '/'))

  return (
    <nav className="tabs">
      {[...modules.flatMap((m) => m.tabs), ...core].map((t) => (
        <Link key={t.to} to={t.to} className={current(t.to) ? 'on' : undefined} aria-current={current(t.to) ? 'page' : undefined}>
          {t.icon}
          {t.label}
        </Link>
      ))}
    </nav>
  )
}
