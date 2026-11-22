import { Link } from 'react-router'
import PageHead from '../../components/PageHead.tsx'
import { SearchIcon, SettingsIcon } from '../../components/icons.tsx'
import { useModules, useSession } from '../../session.ts'
import './More.css'

// The phone's list of what has no tab: the modules' rows first, as their entries come first in the side bar.
export default function More() {
  const modules = useModules()
  const none = useSession().modules.length === 0
  const rows = [
    ...modules.flatMap((m) => m.more),
    { to: '/search', label: 'Search', icon: <SearchIcon />, note: none ? 'Every area' : 'Every area, every module' },
    { to: '/settings', label: 'Settings', icon: <SettingsIcon />, note: 'Tokens, backups, the look and signing out' },
  ]
  return (
    <div className="page">
      <PageHead title="More" />
      <div className="list more">
        {rows.map((r) => (
          <Link key={r.to} className="mrow" to={r.to}>
            {r.icon}
            <span>
              <span className="t1">{r.label}</span>
              <span className="mrow-note">{r.note}</span>
            </span>
          </Link>
        ))}
      </div>
    </div>
  )
}
