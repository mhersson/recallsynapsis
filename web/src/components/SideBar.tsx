import { useState } from 'react'
import { Link, NavLink } from 'react-router'
import { api } from '../api/client.ts'
import { useStream } from '../api/stream.ts'
import type { Area } from '../api/types.ts'
import type { Module } from '../modules/registry.ts'
import { AddIcon, ProjectsIcon, SearchIcon, SettingsIcon } from './icons.tsx'
import './SideBar.css'

type Props = { modules: Module[]; live: boolean }

const item = ({ isActive }: { isActive: boolean }) => 'side-item' + (isActive ? ' on' : '')

export default function SideBar({ modules, live }: Props) {
  const [areas, setAreas] = useState<Area[]>([])
  useStream((changes) => {
    if (changes && !changes.some((c) => c.module === 'core' && c.kind === 'area')) return
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
        <NavLink to="/" end className={({ isActive }) => (isActive ? 'on' : '')}>
          Areas
        </NavLink>
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
