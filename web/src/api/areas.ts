import { useState } from 'react'
import { api } from './client.ts'
import { touchesAreas, useStream } from './stream.ts'
import type { Area } from './types.ts'

// The name of the area a slug names, or the slug while the areas are not read.
export const nameOf = (areas: Area[], slug: string) => areas.find((a) => a.slug === slug)?.name ?? slug

// The areas that are not archived, or every area when archived is true, the personal area first, read again whenever
// one changes.
export function useAreas(archived = false): Area[] {
  const [areas, setAreas] = useState<Area[]>([])
  useStream((changes) => {
    if (touchesAreas(changes)) api<{ areas: Area[] }>('GET', archived ? '/areas?archived=true' : '/areas').then((r) => setAreas(r.areas), () => {})
  })
  return areas
}
