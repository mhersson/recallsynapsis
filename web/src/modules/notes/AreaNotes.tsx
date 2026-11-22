import type { Area } from '../../api/types.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { useList, type List } from './api.ts'
import { ListEnd, Rows } from './Rows.tsx'
import './Notes.css'

// An area's filed notes and ideas, side by side on desktop and folded on a phone, the notes open. Each opens beside
// the list in Notes or the idea bank that holds the same, filtered to the area.
export default function AreaNotes({ area }: { area: Area }) {
  const narrow = useNarrow()
  const ask = (kind: string) => new URLSearchParams({ inbox: 'false', area: area.slug, kind }).toString()
  const notes = useList(ask('note'))
  const ideas = useList(ask('idea'))
  const failed = notes.failed || ideas.failed
  if (failed)
    return (
      <p role="alert" className="err">
        {failed}
      </p>
    )
  if (!notes.list || !ideas.list) return null

  const part = (title: string, to: (id: number) => string, list: List, more?: () => void) => {
    const head = (
      <>
        {title} <span className="dim">{list.total}</span>
      </>
    )
    const body = list.notes.length > 0 && (
      <>
        <Rows notes={list.notes} areas={[area]} to={(r) => to(r.id)} />
        <ListEnd list={list} more={more} />
      </>
    )
    return narrow ? (
      <details open={title === 'Notes'}>
        <summary>{head}</summary>
        {body}
      </details>
    ) : (
      <section>
        <h2 className="part">{head}</h2>
        {body}
      </section>
    )
  }
  return (
    <div className="area-notes">
      {part('Notes', (id) => `/notes/${id}?area=${area.slug}&kind=note`, notes.list, notes.more)}
      {part('Ideas', (id) => `/ideas/${id}?area=${area.slug}`, ideas.list, ideas.more)}
    </div>
  )
}
