import { useStream } from '../../api/stream.ts'

export type Kind = 'note' | 'idea'

export const kinds: [Kind, string][] = [
  ['note', 'Note'],
  ['idea', 'Idea'],
]

// A note as GET /api/notes lists it: area is the slug, excerpt the first 200 characters of the body, and source that
// of the last change, web or the name of a token.
export type Row = {
  id: number
  ref: string
  area: string
  kind: Kind
  title: string
  excerpt: string
  inbox: boolean
  tags: string[]
  file_count: number
  revision: number
  source: string
  created_at: string
  changed_at: string
  trashed_at: string | null
}

// A note as GET /api/notes/{id} gives it; said is the owner's own words on its first revision, when a client gave them.
export type Note = Row & { body: string; said: string }

export type List = { notes: Row[]; total: number }

// Reads when the page mounts, when the stream opens again and on any change of a note. A note moved between areas
// tells of its new area alone, so no change is passed over for its area.
export function useNotes(read: () => void) {
  useStream((changes) => {
    if (!changes || changes.some((c) => c.module === 'notes')) read()
  })
}

// Where a change came from, as a list shows it.
export const from = (source: string) => (source === 'web' ? 'web' : `via ${source}`)
