import type { ReactNode } from 'react'
import { Link } from 'react-router'
import { back } from '../../components/useClose.ts'
import { useItem } from '../../session.ts'
import type { Named } from '../registry.ts'

// Tells of what another module made from a note, with its mark and the way to it, after what became of the note.
export default function MadeNotice({ made, children }: { made: Named; children: ReactNode }) {
  const it = useItem()(made.ref, made.kind, made.state)
  return (
    <div className="notice made">
      <div className="nt">
        <i className={it ? `mk ${it.mark}` : 'mk'} />
        Made the {made.kind} “{made.title}”
      </div>
      <p className="nd">
        {children}{' '}
        {it && (
          <Link to={it.to} state={back}>
            Open the {made.kind}
          </Link>
        )}
      </p>
    </div>
  )
}
