import { useState } from 'react'
import { first } from '../../api/areas.ts'
import { api, message, useGet } from '../../api/client.ts'
import type { Area } from '../../api/types.ts'
import { from, type Entry } from '../../components/history.ts'
import { RevisionFrame, Revisions } from '../../components/History.tsx'
import { Markdown } from '../../components/Markdown.tsx'
import { moment } from '../../components/time.ts'

// A wording of a brief in full.
export type Wording = Entry & { brief: string }

type Props = { area: Area; onRestored: (a: Area) => void }

// The brief at the head of an area's page: its first paragraph, which unfolds in place, then its revision, when it
// changed and by what, and History, whose Read shows an earlier wording in the brief's place.
export default function Brief({ area, onRestored }: Props) {
  const [whole, setWhole] = useState(false)
  const [open, setOpen] = useState(false)
  const [reading, setReading] = useState<number>()

  if (reading)
    return (
      <Reading
        area={area}
        revision={reading}
        onBack={() => setReading(undefined)}
        onRestored={(a) => {
          setReading(undefined)
          setOpen(false)
          onRestored(a)
        }}
      />
    )

  const head = first(area.brief)
  const folds = head !== area.brief
  return (
    <div className="brief">
      {area.brief ? (
        <Markdown
          text={whole ? area.brief : head}
          end={
            folds && (
              <button className="btn link" onClick={() => setWhole(!whole)}>
                {whole ? 'Show less' : 'Show all'}
              </button>
            )
          }
        />
      ) : (
        <p className="sub">
          No brief yet.
          {!area.archived && ` Write ${area.kind === 'personal' ? 'what belongs here' : 'what the project is and why'} with Edit, or ask an AI client to draft it.`}
        </p>
      )}
      {area.brief_revision > 0 && (
        <span className="t2">
          <span>revision {area.brief_revision}</span>
          <span>changed {moment(area.brief_changed_at!)}</span>
          <span>{from(area.brief_changed_by)}</span>
          <button className="btn link" aria-expanded={open} onClick={() => setOpen(!open)}>
            History
          </button>
        </span>
      )}
      {open && <History area={area} onRead={setReading} />}
    </div>
  )
}

// The brief's wordings, the newest first, read again whenever it moves on.
function History({ area, onRead }: { area: Area; onRead: (revision: number) => void }) {
  const { got, failed } = useGet<{ revisions: Entry[] }>(`/areas/${area.id}/revisions`, area.brief_revision)
  return (
    <div className="brief-history">
      {failed && (
        <p role="alert" className="err">
          {failed}
        </p>
      )}
      {got && <Revisions list={got.revisions} current={area.brief_revision} onRead={onRead} />}
    </div>
  )
}

type ReadingProps = { area: Area; revision: number; onRestored: (a: Area) => void; onBack: () => void }

// An earlier wording. An archived project, which has no Edit, has no Restore either, and a wording the brief has now
// would restore nothing, since the server then writes no revision.
function Reading({ area, revision, onRestored, onBack }: ReadingProps) {
  const { got, failed } = useGet<Wording>(`/areas/${area.id}/revisions/${revision}`)
  const same = got?.brief === area.brief
  const restore = () =>
    api<Area>('POST', `/areas/${area.id}/revisions/${revision}/restore`).then((a) => {
      onRestored(a)
      return ''
    }, message)
  return (
    <div className="brief">
      <RevisionFrame
        revision={revision}
        of={area.brief_revision}
        head
        got={got}
        says={same ? 'This is how the brief read then, as it reads now.' : `This is how the brief read then, not its current text. Restoring brings it back as revision ${area.brief_revision + 1}.`}
        failed={failed}
        onRestore={area.archived || same ? undefined : restore}
        onBack={onBack}
      >
        {got && <Markdown text={got.brief} />}
      </RevisionFrame>
    </div>
  )
}
