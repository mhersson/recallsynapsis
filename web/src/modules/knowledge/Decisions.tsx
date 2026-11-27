import type { MouseEvent, ReactNode } from 'react'
import { Link, useNavigate } from 'react-router'
import type { Area } from '../../api/types.ts'
import { day } from '../../components/dates.ts'
import { Markdown, MarkdownInline } from '../../components/Markdown.tsx'
import { back } from '../../components/useClose.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { mark, pathOf, useDecisions, type Decision } from './api.ts'
import { Source } from './Decision.tsx'
import './Knowledge.css'

// The number of the project's decisions that stand, or nothing when none does.
export function Count({ area }: { area: Area }) {
  const { got } = useDecisions(area.slug)
  return got?.decisions.filter((d) => d.state === 'stands').length || null
}

// What replaced a decision, and when.
const replacedBy = (d: Decision) =>
  d.replaced_by && (
    <>
      {day(d.replaced_by.decided)}, by <MarkdownInline text={d.replaced_by.decision} />
    </>
  )

// A project's decisions: those that stand, the newest first, each why whole, and beneath them the replaced, folded under
// their count. On desktop each is a row of a table, on a phone a block; a click anywhere on one but on a link opens it.
export default function Decisions({ area }: { area: Area }) {
  const narrow = useNarrow()
  const navigate = useNavigate()
  const { got, failed } = useDecisions(area.slug)
  if (!got)
    return (
      failed && (
        <p role="alert" className="err">
          {failed}
        </p>
      )
    )
  if (got.decisions.length === 0)
    return (
      <div className="empty knone">
        <b>No decisions yet.</b>
      </div>
    )

  const stand = got.decisions.filter((d) => d.state === 'stands')
  const replaced = got.decisions.filter((d) => d.state === 'replaced')
  // A click that ends a selection of text, as of a why to copy, opens nothing.
  const opens = (d: Decision) => (e: MouseEvent) => {
    if (!(e.target as Element).closest('a') && !getSelection()?.toString()) void navigate(pathOf(d.ref), { state: back })
  }
  const line = (d: Decision) => (
    <Link to={pathOf(d.ref)} state={back}>
      <MarkdownInline text={d.decision} />
    </Link>
  )

  // A block: the line, the day, and, for one that stands, the why, then its other parts under their names.
  const block = (d: Decision, parts: [string, ReactNode][]) => (
    <div key={d.id} className={`kb ${d.state}`} onClick={opens(d)}>
      <i className={`mk ${mark(d.state)}`} />
      <div className="bd">
        <div className="kline">{line(d)}</div>
        <span className="t2">{day(d.decided)}</span>
        {d.state === 'stands' && <Markdown text={d.why} />}
        <div className="kv">
          {parts
            .filter(([, v]) => v)
            .map(([k, v]) => (
              <div key={k}>
                <span>{k}</span>
                <span>{v}</span>
              </div>
            ))}
        </div>
      </div>
    </div>
  )
  const head = (d: Decision) => (
    <>
      <td className="w">{day(d.decided)}</td>
      <td className="l">
        <div>
          <i className={`mk ${mark(d.state)}`} />
          {line(d)}
        </div>
      </td>
    </>
  )

  const standing = narrow ? (
    <div className="kblocks">
      {stand.map((d) =>
        block(d, [
          ['Source', d.source && <Source text={d.source} />],
          ['Replaces', d.replaces && <MarkdownInline text={d.replaces.decision} />],
        ]),
      )}
    </div>
  ) : (
    <table className="kt">
      <colgroup>
        <col className="kw" />
        <col className="kl" />
        <col />
        <col className="ks" />
        <col className="kr" />
      </colgroup>
      <thead>
        <tr>
          <th>When</th>
          <th>The decision</th>
          <th>Why</th>
          <th>Source</th>
          <th>Replaces</th>
        </tr>
      </thead>
      <tbody>
        {stand.map((d) => (
          <tr key={d.id} onClick={opens(d)}>
            {head(d)}
            <td>
              <Markdown text={d.why} />
            </td>
            <td className="s">
              <Source text={d.source} />
            </td>
            <td className="s">{d.replaces && <MarkdownInline text={d.replaces.decision} />}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
  return (
    <div className="decisions">
      {standing}
      {replaced.length > 0 && (
        <details className="kfold">
          <summary>
            Replaced <span className="dim">{replaced.length}</span>
          </summary>
          {narrow ? (
            <div className="kblocks">{replaced.map((d) => block(d, [['Replaced', replacedBy(d)]]))}</div>
          ) : (
            <table className="kt">
              <colgroup>
                <col className="kw" />
                <col className="kh" />
                <col />
              </colgroup>
              <thead>
                <tr>
                  <th>When</th>
                  <th>The decision</th>
                  <th>Replaced</th>
                </tr>
              </thead>
              <tbody>
                {replaced.map((d) => (
                  <tr key={d.id} className="replaced" onClick={opens(d)}>
                    {head(d)}
                    <td className="s">{replacedBy(d)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </details>
      )}
    </div>
  )
}
