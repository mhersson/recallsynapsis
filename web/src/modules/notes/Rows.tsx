import { Link, useLocation } from 'react-router'
import type { Area } from '../../api/types.ts'
import { ago } from '../../components/time.ts'
import { back } from '../../components/useClose.ts'
import { mark, type List, type Row } from './api.ts'

type RowsProps = { notes: Row[]; areas: Area[]; to: (r: Row) => string; open?: number }

// Notes as a list gives them, each a link to its page; open is the one open beside the list.
export function Rows({ notes, areas, to, open }: RowsProps) {
  const name = (slug: string) => areas.find((a) => a.slug === slug)?.name ?? slug
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
              <span>{name(r.area)}</span>
              {r.tags.map((t) => (
                <span key={t}>#{t}</span>
              ))}
              <span>{ago(r.changed_at)}</span>
            </span>
          </span>
        </Link>
      ))}
    </div>
  )
}

// The end of a list that holds more than it shows, with a way to show more while the list may give more.
export function ListEnd({ list, more }: { list: List; more?: () => void }) {
  if (list.total <= list.notes.length) return null
  return (
    <div className="list-end">
      <span>These are the {list.notes.length} most recently changed. There are more.</span>
      {more && (
        <button className="btn" onClick={more}>
          Show 50 more
        </button>
      )}
    </div>
  )
}
