import { useState, type FormEvent } from 'react'
import { api, message } from '../../api/client.ts'
import { AddIcon } from '../../components/icons.tsx'
import type { Fields, Row } from './api.ts'

type Props = { placeholder: string; fields: Fields; onAdded: (t: Row) => void }

// The line that adds a task where one stands, given what that puts on it: planned for today, on the list, in the area.
export default function AddLine({ placeholder, fields, onAdded }: Props) {
  const [title, setTitle] = useState('')
  const [sending, setSending] = useState(false)
  const [refused, setRefused] = useState('')
  async function add(e: FormEvent) {
    e.preventDefault()
    setRefused('')
    setSending(true)
    try {
      const r = await api<{ tasks: Row[] }>('POST', '/tasks', { tasks: [{ title: title.trim(), ...fields }] })
      setTitle('')
      onAdded(r.tasks[0])
    } catch (err) {
      setRefused(message(err))
    }
    setSending(false)
  }
  return (
    <form className="addl" onSubmit={(e) => void add(e)}>
      <label className="addl-field">
        <AddIcon />
        <input aria-label="New task" placeholder={placeholder} autoComplete="off" value={title} onChange={(e) => setTitle(e.target.value)} />
      </label>
      <button className="btn" disabled={sending || !title.trim()}>
        Add
      </button>
      {refused && (
        <p role="alert" className="err">
          {refused}
        </p>
      )}
    </form>
  )
}
