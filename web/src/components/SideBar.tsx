import { Link, NavLink, useLocation } from 'react-router'
import { useAreas } from '../api/areas.ts'
import type { Action, Module } from '../modules/registry.ts'
import { AddIcon, ProjectsIcon, SearchIcon, SettingsIcon } from './icons.tsx'
import './SideBar.css'

type Props = { modules: Module[]; action?: Action; onAction: () => void; live: boolean }

const item = ({ isActive }: { isActive: boolean }) => 'side-item' + (isActive ? ' on' : '')

export default function SideBar({ modules, action, onAction, live }: Props) {
  // The dialog that makes a project opens over the Projects list, which stays current.
  const { pathname } = useLocation()
  const projects = pathname === '/projects' || pathname === '/areas/new'
  const areas = useAreas()

  return (
    <nav className="side">
      <span className="brand">Recall Synapsis</span>
      {action && (
        <button className="btn primary side-act" onClick={onAction}>
          {action.icon}
          {action.label}
        </button>
      )}
      {modules.flatMap((m) => m.nav).map((e) => (
        <NavLink key={e.to} to={e.to} className={item}>
          {e.icon}
          {e.label}
          {e.count && (
            <span className="cnt">
              <e.count />
            </span>
          )}
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
