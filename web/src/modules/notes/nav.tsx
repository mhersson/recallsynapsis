import { useState } from 'react'
import { api } from '../../api/client.ts'
import { Icon } from '../../components/icons.tsx'
import { useNotes, type List } from './api.ts'

export const InboxIcon = () => (
  <Icon>
    <path d="M2 9.5 4 3h8l2 6.5V13H2z" />
    <path d="M2 9.5h3.5l1 1.5h3l1-1.5H14" />
  </Icon>
)

export const NotesIcon = () => (
  <Icon>
    <path d="M4 2h5.5L12 4.5V14H4z" />
    <path d="M6 7.5h4M6 10h4" />
  </Icon>
)

export const IdeasIcon = () => (
  <Icon>
    <path d="M5.6 10.6a4 4 0 1 1 4.8 0V12H5.6z" />
    <path d="M6.6 14h2.8" />
  </Icon>
)

export const TrashIcon = () => (
  <Icon>
    <path d="M2.5 4.5h11M6.5 4.5V2.8h3v1.7M4 4.5l.7 9h6.6l.7-9" />
  </Icon>
)

// How many captures wait: the total of a list of one.
export function Waiting() {
  const [n, setN] = useState(0)
  useNotes(() => void api<List>('GET', '/notes?inbox=true&limit=1').then((r) => setN(r.total), () => {}))
  return n || null
}
