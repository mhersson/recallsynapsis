import { useState } from 'react'
import { api } from './client.ts'
import { touchesAreas, useStream } from './stream.ts'
import type { Area } from './types.ts'

// The areas that are not archived, the personal area first, read again whenever one changes.
export function useAreas(): Area[] {
  const [areas, setAreas] = useState<Area[]>([])
  useStream((changes) => {
    if (touchesAreas(changes)) api<{ areas: Area[] }>('GET', '/areas').then((r) => setAreas(r.areas), () => {})
  })
  return areas
}
