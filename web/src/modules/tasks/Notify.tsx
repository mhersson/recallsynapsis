import { useId, useRef, useState } from 'react'
import { api, message } from '../../api/client.ts'
import { useStream } from '../../api/stream.ts'
import Switch from '../../components/Switch.tsx'
import { useNarrow } from '../../components/useNarrow.ts'
import { useUnsaved } from '../../components/useUnsaved.tsx'
import type { Hour, Kind, Kinds } from './api.ts'
import './Tasks.css'

// Each kind, with its hour when it has one, its words and its quiet line.
const kinds: [Kind, Hour | null, string, string][] = [
  ['start', null, "At a task's start time", 'A task with a time reminds at that time.'],
  ['morning', 'morning_at', 'In the morning, at', "What the day holds: today's tasks, what is left from earlier and what is overdue. Not sent on a day with nothing."],
  ['deadline', 'deadline_at', 'A deadline, the day before, at', 'The tasks due tomorrow.'],
  ['untimed', 'untimed_at', 'Planned today with no time, at', 'The tasks planned for today with no start time today.'],
]

// The kinds of notification, which hold for every browser of the caller. A switch is sent as it is flipped and then shows
// what the server answers; an hour waits for Save, and stays as typed while the rest follows the server.
export default function Notify() {
  const narrow = useNarrow()
  const id = useId()
  const [held, setHeld] = useState<Kinds>()
  // Only the hours that differ from what the server held as they were typed.
  const [typed, setTyped] = useState<Partial<Record<Hour, string>>>({})
  const [refused, setRefused] = useState('')
  const [saving, setSaving] = useState(false)
  const asked = useRef(0)

  useStream((changes) => {
    if (changes && !changes.some((c) => c.module === 'tasks' && c.kind === 'notify')) return
    const n = ++asked.current
    api<Kinds>('GET', '/tasks/notify').then(
      (k) => {
        if (n !== asked.current) return
        setHeld(k)
        setRefused('')
      },
      (e: unknown) => n === asked.current && setRefused(message(e)),
    )
  })

  const differ = held ? (Object.keys(typed) as Hour[]).filter((h) => typed[h] !== held[h]) : []
  const leaving = useUnsaved(differ.length > 0, 'Settings')

  // A read asked before a change is answered is dropped, so it never shows the server as it was before.
  async function change(f: Partial<Kinds>) {
    setRefused('')
    try {
      const k = await api<Kinds>('PATCH', '/tasks/notify', f)
      asked.current++
      setHeld(k)
      return true
    } catch (e) {
      setRefused(message(e))
      return false
    }
  }
  async function save() {
    setSaving(true)
    if (await change(Object.fromEntries(differ.map((h) => [h, typed[h]])))) setTyped({})
    setSaving(false)
  }

  const alert = refused && (
    <p role="alert" className="err">
      {refused}
    </p>
  )
  if (!held) return alert
  const saveButton = (
    // A time field gives no value while it is empty or half typed.
    <button className="btn primary" disabled={saving || differ.some((h) => !typed[h])} onClick={() => void save()}>
      Save
    </button>
  )
  return (
    <div className="notify">
      <p className="note">These hold for every browser of yours. A switch is applied at once.</p>
      {kinds.map(([k, h, words, note]) => (
        <div key={k} className="notify-kind">
          <Switch id={id + k} aria-labelledby={id + k + 'w'} on={held[k]} onChange={(on) => void change({ [k]: on })} />
          <div>
            <div className="notify-line">
              {/* The words name the switch, and lead to the hour where there is one. */}
              <label id={id + k + 'w'} htmlFor={id + (h ?? k)}>
                {words}
              </label>
              {h && (
                <input
                  id={id + h}
                  type="time"
                  className={held[k] ? undefined : 'off'}
                  value={typed[h] ?? held[h]}
                  onChange={(e) => {
                    const { [h]: _, ...rest } = typed
                    setTyped(e.target.value === held[h] ? rest : { ...rest, [h]: e.target.value })
                  }}
                />
              )}
            </div>
            <p className="note">{note}</p>
          </div>
        </div>
      ))}
      {alert}
      {differ.length > 0 && (
        // Save is last on desktop and first on a phone in the DOM too, so the focus moves in the order shown.
        <div className={narrow ? 'acts stack' : 'acts end'}>
          {narrow && saveButton}
          <button className="btn" onClick={() => setTyped({})}>
            Cancel
          </button>
          {!narrow && saveButton}
        </div>
      )}
      {leaving}
    </div>
  )
}
