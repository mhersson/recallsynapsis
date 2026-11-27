import { useState } from 'react'
import { api } from './client.ts'
import { touchesAreas, useStream } from './stream.ts'
import type { Area } from './types.ts'

// The name of the area a slug names, or the slug while the areas are not read.
export const nameOf = (areas: Area[], slug: string) => areas.find((a) => a.slug === slug)?.name ?? slug

// The first paragraph of a brief: the text before the first blank line.
export const first = (brief: string) => brief.split(/\n\s*\n/, 1)[0]

// The user's area with the id, archived or not, or undefined when there is none.
export const areaOf = (id: number) => api<{ areas: Area[] }>('GET', '/areas?archived=true').then((r) => r.areas.find((a) => a.id === id))

// The areas that are not archived, or every area when archived is true, the personal area first, read again whenever
// one changes.
export function useAreas(archived = false): Area[] {
  const [areas, setAreas] = useState<Area[]>([])
  useStream((changes) => {
    if (touchesAreas(changes)) api<{ areas: Area[] }>('GET', archived ? '/areas?archived=true' : '/areas').then((r) => setAreas(r.areas), () => {})
  })
  return areas
}
