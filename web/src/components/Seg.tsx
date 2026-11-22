import type { ReactNode } from 'react'

// One of a few, as a row of buttons; type=button keeps it from sending a form it sits in.
export default function Seg<T extends string>({ label, options, value, onChange }: { label: string; options: [T, ReactNode][]; value: T; onChange: (v: T) => void }) {
  return (
    <div className="seg" role="radiogroup" aria-label={label}>
      {options.map(([v, text]) => (
        <button key={v} type="button" role="radio" aria-checked={v === value} className={v === value ? 'on' : undefined} onClick={() => onChange(v)}>
          {text}
        </button>
      ))}
    </div>
  )
}
