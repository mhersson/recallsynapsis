// One choice of a few, as a row of buttons; type=button keeps it from sending a form it sits in.
export default function Choice<T extends string>({ id, label, options, value, onChange, note }: {
  id: string
  label: string
  options: [T, string][]
  value: T
  onChange: (v: T) => void
  note?: string
}) {
  return (
    <div className="frow">
      <span className="label" id={id}>
        {label}
      </span>
      <div className="seg" role="radiogroup" aria-labelledby={id}>
        {options.map(([v, text]) => (
          <button key={v} type="button" role="radio" aria-checked={v === value} className={v === value ? 'on' : ''} onClick={() => onChange(v)}>
            {text}
          </button>
        ))}
      </div>
      {note && <p className="note">{note}</p>}
    </div>
  )
}
