import type { ReactNode } from 'react'
import { Link, useParams } from 'react-router'
import { useAreas } from '../../api/areas.ts'
import { day } from '../../components/dates.ts'
import { from } from '../../components/history.ts'
import { LinkRow } from '../../components/Links.tsx'
import { web } from '../../components/markdown.ts'
import { Markdown, MarkdownInline } from '../../components/Markdown.tsx'
import Section from '../../components/Section.tsx'
import { moment } from '../../components/time.ts'
import { back } from '../../components/useClose.ts'
import { useAreaShown } from '../../session.ts'
import { mark, pathOf, useDecision, useDecisions, type Line } from './api.ts'
import './Knowledge.css'

// Keyed by the id, so going to another decision starts afresh.
export default function Decision() {
  const { id } = useParams()
  return <View key={id} id={Number(id)} />
}

// A source is a link only when the whole of it is one http or https address, opened as a link in a note's text is.
export function Source({ text }: { text: string }) {
  return !/\s/.test(text) && web(text) ? (
    <a href={text} target="_blank" rel="noreferrer">
      {text}
    </a>
  ) : (
    text
  )
}

// A decision as a row that leads to it, its line whole.
function Leads({ line, state }: { line: Line; state: string }) {
  return (
    <div className="lrow whole">
      <i className={`mk ${mark(state)}`} />
      <Link to={pathOf(line.ref)} state={back}>
        <MarkdownInline text={line.decision} />
      </Link>
    </div>
  )
}

const Part = ({ label, children }: { label: string; children: ReactNode }) => (
  <div className="kpart">
    <span className="label">{label}</span>
    {children}
  </div>
)

// A decision, read, with the way back to its project's decisions. What replaced a replaced one comes first, with the mark
// of its own state, which only the project's list tells.
function View({ id }: { id: number }) {
  const { decision: d, failed } = useDecision(id)
  const project = useAreas(true).find((a) => a.slug === d?.area)
  useAreaShown(project?.id)
  const { got } = useDecisions(d?.replaced_by ? d.area : '')
  if (!d)
    return (
      failed && (
        <div className="page narrow">
          <p role="alert" className="err">
            {failed}
          </p>
        </div>
      )
    )

  const newer = d.replaced_by && got?.decisions.find((x) => x.ref === d.replaced_by!.ref)
  return (
    <div className="page narrow decision">
      {project && (
        <div className="crumb">
          <Link to={`/areas/${project.id}`}>{project.name}</Link> / <Link to={`/areas/${project.id}?tab=decisions`}>Decisions</Link> /
        </div>
      )}
      <div className="page-head">
        <h1>
          <MarkdownInline text={d.decision} />
        </h1>
      </div>
      <span className="t2 decision-meta">
        <i className={`mk ${mark(d.state)}`} />
        <span>decision</span>
        <span>{d.state}</span>
        <span>decided {day(d.decided)}</span>
        <span>changed {moment(d.changed_at)}</span>
        <span>{from(d.changed_by)}</span>
      </span>
      {d.replaced_by && (
        <Part label="Replaced by">
          <Leads line={d.replaced_by} state={newer ? newer.state : 'stands'} />
        </Part>
      )}
      <Part label="Why">
        <Markdown text={d.why} />
      </Part>
      {d.source && (
        <Part label="Source">
          <p className="ksource">
            <Source text={d.source} />
          </p>
        </Part>
      )}
      {d.replaces && (
        <Part label="Replaces">
          <Leads line={d.replaces} state="replaced" />
        </Part>
      )}
      {d.linked_from.length > 0 && (
        <div className="sects">
          <Section title="Linked from" count={d.linked_from.length} open>
            {d.linked_from.map((l) => (
              <LinkRow key={l.ref} l={l} />
            ))}
          </Section>
        </div>
      )}
    </div>
  )
}
