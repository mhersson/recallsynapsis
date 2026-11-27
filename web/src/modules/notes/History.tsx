import { useGet } from '../../api/client.ts'
import { Revisions, RevisionFrame } from '../../components/History.tsx'
import { Markdown } from '../../components/Markdown.tsx'
import Section from '../../components/Section.tsx'
import { count, useRead, type Note, type Revision, type Version } from './api.ts'

// The note's revisions, the newest first, read again whenever the note moves on. fresh is the one a restore just wrote.
export default function History({ note, fresh, onRead }: { note: Note; fresh?: number; onRead: (revision: number) => void }) {
  const { got, failed } = useGet<{ revisions: Revision[] }>(`/notes/${note.id}/revisions`, note.revision)
  return (
    <Section title="History" count={count(note.revision, 'revision')}>
      {failed && (
        <p role="alert" className="err">
          {failed}
        </p>
      )}
      {got && <Revisions list={got.revisions.map(({ source, ...r }) => ({ ...r, by: source }))} current={note.revision} fresh={fresh} onRead={onRead} />}
    </Section>
  )
}

type ReadProps = { note: Note; revision: number; onRestore: (v: Version) => Promise<string>; onBack: () => void }

// An earlier revision of the note. Restoring it is a change of the note, which a note in the trash takes none of.
export function RevisionView({ note, revision, onRestore, onBack }: ReadProps) {
  const { got: v, failed } = useRead<Version>(`/notes/${note.id}/revisions/${revision}`)
  return (
    <RevisionFrame
      revision={revision}
      of={note.revision}
      got={v && { ...v, by: v.source }}
      says={`This is how the note read then, not its current text. Restoring brings back its title, text, tags and links as revision ${note.revision + 1}. The area and the kind stay.`}
      failed={failed}
      onRestore={v && !note.trashed_at ? () => onRestore(v) : undefined}
      onBack={onBack}
    >
      {v && (
        <>
          {v.title && <h2 className="rt">{v.title}</h2>}
          <Markdown text={v.body} />
          <div className="note-tags">
            {v.tags.map((t) => (
              <span key={t} className="tag">
                #{t}
              </span>
            ))}
          </div>
        </>
      )}
    </RevisionFrame>
  )
}
