import { useNarrow } from '../../components/useNarrow.ts'
import { useNow } from './api.ts'
import { period } from './dates.ts'
import { Shortcuts } from './fields.tsx'

type Props = { value: Record<string, string>; onChange: (v: Record<string, string>) => void }

// What Capture shows under its text once Task is chosen: the shortcuts for when, by the server's today, which it reads
// itself. The one chosen is chosen no more when picked again, and with none the task waits to be sorted.
export default function When({ value, onChange }: Props) {
  const narrow = useNarrow()
  const now = useNow()
  if (!now) return null
  const { planned_from: from = '', planned_to: to = '' } = value
  return (
    <div className="field when">
      {narrow && <span className="label">When</span>}
      <Shortcuts now={now} from={from} to={to} onPick={(a, b) => onChange(a === from && b === to ? {} : { planned_from: a, planned_to: b })} />
      <p className="note">{from ? `Planned for ${period(from, to)}.` : 'Choose none, and it waits on Tasks to be sorted.'}</p>
    </div>
  )
}
