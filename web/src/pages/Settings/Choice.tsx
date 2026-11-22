import Seg from '../../components/Seg.tsx'

// One choice of a few under its label, with a note when given.
export default function Choice<T extends string>({ label, options, value, onChange, note }: {
  label: string
  options: [T, string][]
  value: T
  onChange: (v: T) => void
  note?: string
}) {
  return (
    <div className="frow">
      <span className="label">{label}</span>
      <Seg label={label} options={options} value={value} onChange={onChange} />
      {note && <p className="note">{note}</p>}
    </div>
  )
}
