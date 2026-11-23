import { useState } from 'react'
import { api } from '../../api/client.ts'
import { Icon } from '../../components/icons.tsx'
import { useTasks, type List } from './api.ts'

export const TasksIcon = () => (
  <Icon>
    <circle cx="8" cy="8" r="5.8" />
    <path d="m5.6 8.1 1.7 1.7 3.2-3.4" />
  </Icon>
)

// How many tasks wait to be sorted: the total of a list of one.
export function Unsorted() {
  const [n, setN] = useState(0)
  useTasks(() => void api<List>('GET', '/tasks?unsorted=true&limit=1').then((r) => setN(r.total), () => {}))
  return n || null
}
