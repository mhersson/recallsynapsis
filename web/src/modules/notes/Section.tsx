import type { ReactNode } from 'react'
import { useNarrow } from '../../components/useNarrow.ts'
import './Sections.css'

type Props = { title: string; count: ReactNode; add?: ReactNode; children: ReactNode }

// A section of a note under its head on desktop, with what adds to it at the end of the head. On a phone it is folded,
// and what adds to it comes first.
export default function Section({ title, count, add, children }: Props) {
  const narrow = useNarrow()
  const head = (
    <>
      {title} <span className="dim">{count}</span>
    </>
  )
  return narrow ? (
    <details className="sect">
      <summary>{head}</summary>
      {add && <div className="sect-add">{add}</div>}
      {children}
    </details>
  ) : (
    <section className="sect">
      <div className="sect-head">
        <h2>{head}</h2>
        {add}
      </div>
      {children}
    </section>
  )
}
