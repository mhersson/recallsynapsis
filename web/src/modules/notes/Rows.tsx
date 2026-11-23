import { Link, useLocation } from 'react-router'
import { nameOf } from '../../api/areas.ts'
import type { Area } from '../../api/types.ts'
import { ago } from '../../components/time.ts'
import { back } from '../../components/useClose.ts'
import { mark, type Row } from './api.ts'

type RowsProps = { notes: Row[]; areas: Area[]; to: (r: Row) => string; open?: number; trashed?: boolean }

// Notes as a list gives them, each a link to its page; open is the one open beside the list. The trash's say when each
// was trashed in place of their tags and when they changed.
export function Rows({ notes, areas, to, open, trashed }: RowsProps) {
  // A link to the page shown replaces it; the open row keeps the page's state, so closing the note goes where it would.
  const { state } = useLocation()
  return (
    <div className="list">
      {notes.map((r) => (
        <Link key={r.id} className="row" to={to(r)} state={r.id === open ? state : back} aria-current={r.id === open || undefined}>
          <i className={`mk ${mark(r.kind, r.inbox)}`} />
          <span className="item">
            <span className="t1">{r.title || r.excerpt}</span>
            <span className="t2">
              <span>{nameOf(areas, r.area)}</span>
              {trashed ? (
                <>
                  {r.inbox && <span>in the inbox</span>}
                  <span>trashed {ago(r.trashed_at!)}</span>
                </>
              ) : (
                <>
                  {r.tags.map((t) => (
                    <span key={t}>#{t}</span>
                  ))}
                  <span>{ago(r.changed_at)}</span>
                </>
              )}
            </span>
          </span>
        </Link>
      ))}
    </div>
  )
}
