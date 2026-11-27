import { useState, type ReactNode } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { useAreas } from '../../api/areas.ts'
import { api, message, useGet } from '../../api/client.ts'
import type { Area } from '../../api/types.ts'
import { day } from '../../components/dates.ts'
import Dialog from '../../components/Dialog.tsx'
import { from, type Entry } from '../../components/history.ts'
import { RevisionFrame, Revisions } from '../../components/History.tsx'
import { LinkRow } from '../../components/Links.tsx'
import { plain, web } from '../../components/markdown.ts'
import { Markdown, MarkdownInline } from '../../components/Markdown.tsx'
import Section from '../../components/Section.tsx'
import { moment } from '../../components/time.ts'
import { back } from '../../components/useClose.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { useAreaShown } from '../../session.ts'
import { fieldsOf, mark, pathOf, useDecision, useDecisions, type Full, type Line, type Wording } from './api.ts'
import DecisionForm from './Form.tsx'
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

// The server's line, on a page of its own, for what could not be read.
export const Failed = ({ line }: { line: string }) =>
  line && (
    <div className="page narrow">
      <p role="alert" className="err">
        {line}
      </p>
    </div>
  )

// The way back to the project's page, on Decisions.
export const Crumb = ({ project }: { project: Area }) => (
  <div className="crumb">
    <Link to={`/areas/${project.id}`}>{project.name}</Link> / <Link to={`/areas/${project.id}?tab=decisions`}>Decisions</Link> /
  </div>
)

const Part = ({ label, children }: { label: string; children: ReactNode }) => (
  <div className="kpart">
    <span className="label">{label}</span>
    {children}
  </div>
)

// A decision, read, or edited in place in a form no read replaces. The project's list gives the state of what replaced
// it and what the form's Replaces offers.
function View({ id }: { id: number }) {
  const narrow = useNarrow()
  const navigate = useNavigate()
  const { decision: d, failed, show } = useDecision(id)
  // The decision as the editing began.
  const [editing, setEditing] = useState<Full>()
  const [deleting, setDeleting] = useState(false)
  const [refused, setRefused] = useState('')
  // History stays unfolded while an earlier wording is read, and as the way back returns from it.
  const [history, setHistory] = useState(false)
  const [reading, setReading] = useState<number>()
  const shown = editing ?? d
  const project = useAreas(true).find((a) => a.slug === shown?.area)
  useAreaShown(project?.id)
  const { got } = useDecisions(shown && (shown.replaced_by || editing) ? shown.area : '')
  const crumb = project && <Crumb project={project} />
  if (editing)
    return (
      <div className="page narrow decision">
        {crumb}
        <DecisionForm
          area={editing.area}
          of={editing}
          start={fieldsOf(editing)}
          decisions={got?.decisions ?? []}
          onSaved={(r) => {
            show(r)
            setEditing(undefined)
          }}
          onCancel={() => setEditing(undefined)}
        />
      </div>
    )
  if (!d) return <Failed line={failed} />

  if (reading)
    return (
      <div className="page narrow decision">
        {crumb}
        <Reading d={d} revision={reading} onBack={() => setReading(undefined)} />
      </div>
    )

  async function remove() {
    setDeleting(false)
    setRefused('')
    try {
      await api('DELETE', `/knowledge/${id}`)
      void navigate(project ? `/areas/${project.id}?tab=decisions` : '/projects', { replace: true })
    } catch (e) {
      setRefused(message(e))
    }
  }

  const newer = d.replaced_by && got?.decisions.find((x) => x.ref === d.replaced_by!.ref)
  const acts = (
    <>
      <button
        className="btn"
        onClick={() => {
          setRefused('')
          setEditing(d)
        }}
      >
        Edit
      </button>
      {d.state === 'stands' && (
        <Link className="btn" to={'/decisions/new?' + new URLSearchParams({ area: d.area, replaces: String(d.id) })} state={back}>
          Replace
        </Link>
      )}
      <button className="btn danger" onClick={() => setDeleting(true)}>
        Delete
      </button>
    </>
  )
  return (
    <div className="page narrow decision">
      {crumb}
      <div className="page-head">
        <h1>
          <MarkdownInline text={d.decision} />
        </h1>
        {!narrow && (
          <>
            <span className="spacer" />
            {acts}
          </>
        )}
      </div>
      <span className="t2 decision-meta">
        <i className={`mk ${mark(d.state)}`} />
        <span>decision</span>
        <span>{d.state}</span>
        <span>decided {day(d.decided)}</span>
        <span>changed {moment(d.changed_at)}</span>
        <span>{from(d.changed_by)}</span>
      </span>
      {refused && (
        <p role="alert" className="err">
          {refused}
        </p>
      )}
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
      <div className="sects">
        {d.linked_from.length > 0 && (
          <Section title="Linked from" count={d.linked_from.length} open>
            {d.linked_from.map((l) => (
              <LinkRow key={l.ref} l={l} />
            ))}
          </Section>
        )}
        <details className="sect" open={history} onToggle={(e) => setHistory(e.currentTarget.open)}>
          <summary>
            History <span className="dim">{count(d.revision)}</span>
          </summary>
          {history && <History d={d} onRead={setReading} />}
        </details>
      </div>
      {narrow && <div className="acts stack kfoot">{acts}</div>}
      {deleting && (
        <Dialog title="Delete this decision for good?" onClose={() => setDeleting(false)}>
          <p className="note">
            “{plain(d.decision)}” is deleted with its history.
            {d.replaces
              ? ` The decision it replaced stands again: “${plain(d.replaces.decision)}”.`
              : d.replaced_by && ' The decision that replaced it then replaces nothing.'}{' '}
            This cannot be undone.
          </p>
          <div className="acts">
            <button className="btn" onClick={() => setDeleting(false)}>
              Keep it
            </button>
            <button className="btn danger" onClick={() => void remove()}>
              Delete for good
            </button>
          </div>
        </Dialog>
      )}
    </div>
  )
}

const count = (n: number) => `${n} ${n === 1 ? 'revision' : 'revisions'}`

// The decision's wordings, the newest first, read again whenever it moves on.
function History({ d, onRead }: { d: Full; onRead: (revision: number) => void }) {
  const { got, failed } = useGet<{ revisions: Entry[] }>(`/knowledge/${d.id}/revisions`, d.revision)
  return (
    <>
      {failed && (
        <p role="alert" className="err">
          {failed}
        </p>
      )}
      {got && <Revisions list={got.revisions} current={d.revision} onRead={onRead} />}
    </>
  )
}

type ReadingProps = { d: Full; revision: number; onBack: () => void }

// An earlier wording of d, which is read and never restored: mending it back is Edit's. What it replaced then may since
// be deleted, and then leads nowhere.
function Reading({ d, revision, onBack }: ReadingProps) {
  const { got, failed } = useGet<Wording>(`/knowledge/${d.id}/revisions/${revision}`)
  const decisions = useDecisions(got?.replaces ? d.area : '').got?.decisions
  const was = got?.replaces && decisions?.find((x) => x.ref === got.replaces)
  return (
    <RevisionFrame revision={revision} of={d.revision} got={got} says="This is how the decision read then, not its current text." failed={failed} onBack={onBack}>
      {got && (
        <>
          <h2 className="rt">
            <MarkdownInline text={got.decision} />
          </h2>
          <Part label="Why">
            <Markdown text={got.why} />
          </Part>
          {got.source && (
            <Part label="Source">
              <p className="ksource">
                <Source text={got.source} />
              </p>
            </Part>
          )}
          <Part label="Decided">{day(got.decided)}</Part>
          {got.replaces && decisions && (
            <Part label="Replaces">
              {was ? <Leads line={was} state={was.state} /> : <LinkRow l={{ ref: got.replaces, kind: 'decision', state: '', title: '', available: false }} />}
            </Part>
          )}
        </>
      )}
    </RevisionFrame>
  )
}
