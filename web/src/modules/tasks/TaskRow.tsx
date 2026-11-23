import type { ReactNode } from 'react'
import { Link, useLocation } from 'react-router'
import { back } from '../../components/useClose.ts'
import { ring, type Row } from './api.ts'
import Check from './Check.tsx'

type Props = { t: Row; to: string; bits: ReactNode[]; on: boolean; done?: boolean; added?: boolean; onRing?: () => void }

// A task's row, a link to the task: its ring, or its mark where it cannot be changed, and its title over a quiet line of
// bits. on is the row of the task open beside the list, which keeps the page's state, so closing the task goes where it
// would.
export default function TaskRow({ t, to, bits, on, done = false, added, onRing }: Props) {
  const { state } = useLocation()
  return (
    <div className={'row task-row' + (on ? ' on' : '') + (done ? ' done' : '') + (added ? ' new' : '')}>
      {onRing ? <Check done={done} sort={t.state === 'unsorted'} onClick={onRing} /> : <i className={`mk ${ring(t.state)}`} />}
      <Link to={to} state={on ? state : back} aria-current={on || undefined}>
        <span className="t1">{t.title}</span>
        {bits.length > 0 && (
          <span className="t2">
            {bits.map((b, i) => (
              <span key={i}>{b}</span>
            ))}
          </span>
        )}
      </Link>
    </div>
  )
}
