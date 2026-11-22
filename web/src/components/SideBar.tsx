import { useState } from 'react'
import { Link, NavLink, useLocation } from 'react-router'
import { api } from '../api/client.ts'
import { touchesAreas, useStream } from '../api/stream.ts'
import type { Area } from '../api/types.ts'
import type { Module } from '../modules/registry.ts'
import { AddIcon, ProjectsIcon, SearchIcon, SettingsIcon } from './icons.tsx'
import './SideBar.css'

type Props = { modules: Module[]; live: boolean }

const item = ({ isActive }: { isActive: boolean }) => 'side-item' + (isActive ? ' on' : '')

export default function SideBar({ modules, live }: Props) {
  // The dialog that makes a project opens over the Projects list, which stays current.
  const { pathname } = useLocation()
  const projects = pathname === '/projects' || pathname === '/areas/new'
  const [areas, setAreas] = useState<Area[]>([])
  useStream((changes) => {
    if (!touchesAreas(changes)) return
    api<{ areas: Area[] }>('GET', '/areas').then((r) => setAreas(r.areas), () => {})
  })

  return (
    <nav className="side">
      <span className="brand">Recall Synapsis</span>
      {modules.flatMap((m) => m.nav).map((e) => (
        <NavLink key={e.to} to={e.to} className={item}>
          {e.icon}
          {e.label}
        </NavLink>
      ))}
      <NavLink to="/search" className={item}>
        <SearchIcon />
        Search
      </NavLink>
      <div className="side-head">
        <Link to="/projects" className={projects ? 'on' : undefined} aria-current={projects ? 'page' : undefined}>
          Areas
        </Link>
        <Link to="/areas/new" className="side-add" aria-label="New project" title="New project">
          <AddIcon />
        </Link>
      </div>
      {areas.map((a) => (
        <NavLink key={a.id} to={`/areas/${a.id}`} className={item}>
          <ProjectsIcon />
          <span className={a.kind === 'project' ? 'mono' : undefined}>{a.name}</span>
        </NavLink>
      ))}
      <div className="side-foot">
        <NavLink to="/settings" className={item}>
          <SettingsIcon />
          Settings
        </NavLink>
        <span className="live" role="status" data-live={live}>
          {live ? 'live' : 'offline'}
        </span>
      </div>
    </nav>
  )
}
