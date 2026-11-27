import type { ReactNode } from 'react'
import Count from './Count.tsx'
import { useNarrow } from './useNarrow.ts'
import './Section.css'

type Props = { title: string; count: number | string; add?: ReactNode; open?: boolean; children: ReactNode }

// A section of an item's page under its head on desktop, with what adds to it at the end of the head. On a phone it is folded,
// open when asked, and what adds to it comes first.
export default function Section({ title, count, add, open, children }: Props) {
  const narrow = useNarrow()
  const head = (
    <>
      {title} <Count>{count}</Count>
    </>
  )
  return narrow ? (
    <details className="sect" open={open}>
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
