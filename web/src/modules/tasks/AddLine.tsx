import { useState, type FormEvent } from 'react'
import { message } from '../../api/client.ts'
import { AddIcon } from '../../components/icons.tsx'
import { make, type Fields, type Row } from './api.ts'

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
      const t = await make({ title: title.trim(), ...fields })
      setTitle('')
      onAdded(t)
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
