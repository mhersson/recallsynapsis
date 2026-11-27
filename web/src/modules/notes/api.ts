import { useState } from 'react'
import { step, useGet, useLimit } from '../../api/client.ts'
import { useStream } from '../../api/stream.ts'
import type { Linked } from '../../api/types.ts'

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

// A file attached to a note, of the type it was declared as.
export type Attached = { id: number; name: string; type: string; size: number }

// A note as GET /api/notes/{id} gives it: in full, with the owner's own words on its first revision when a client gave
// them, its links, the items that link to it and its files in the order they were attached.
export type Note = Row & { body: string; said: string; links: Linked[]; linked_from: Linked[]; files: Attached[] }

export type List = { notes: Row[]; total: number }

// A revision as the history lists it, the newest first: restored_from is the revision a restore brought back.
export type Revision = { revision: number; at: string; source: string; said: string; restored_from: number | null }

// A revision in full, its links as references.
export type Version = Revision & { title: string; body: string; kind: Kind; area: string; inbox: boolean; tags: string[]; links: string[] }

// Reads when the page mounts, when the stream opens again, on any change of a note and on any change of an area, whose
// slug names the notes in it. A note moved between areas tells of its new area alone, so no change is passed over.
export function useNotes(read: () => void) {
  useStream((changes) => {
    if (!changes || changes.some((c) => c.module === 'notes' || (c.module === 'core' && c.kind === 'area'))) read()
  })
}

// What GET path gives, read as useNotes reads and when path changes; '' reads nothing. An answer to a question asked
// since is dropped, and a refusal is kept until a read succeeds.
export function useRead<T>(path: string): { got?: T; failed: string; again: () => void } {
  const [tick, setTick] = useState(0)
  const again = () => setTick((n) => n + 1)
  useNotes(again)
  return { ...useGet<T>(tick ? path : '', tick), again }
}

// The notes a list asks for with the query asked, as many as useLimit gives.
export function useList(asked: string) {
  const { limit, more } = useLimit(asked)
  const { got: list, failed, again: reread } = useRead<List>(asked && `/notes?${asked}${limit > step ? `&limit=${limit}` : ''}`)
  return { list, failed, more, reread }
}

// A count of things, as a sentence says it.
export const count = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`

// The class of a note's mark: blue for a note, purple for an idea, and the yellow of what waits for a capture not yet filed.
export const mark = (kind: string, inbox: boolean) => (inbox ? 'wait' : kind === 'idea' ? 'm-idea' : 'm-note')
