import { cloneElement, useId, type ReactElement, type ReactNode } from 'react'
import './Field.css'

type Props = { label: ReactNode; hint?: ReactNode; children: ReactElement<{ id?: string }> }

export default function Field({ label, hint, children }: Props) {
  const id = useId()
  return (
    <div className="field">
      <div className="field-head">
        <label htmlFor={id}>{label}</label>
        {hint && <span className="t2">{hint}</span>}
      </div>
      {cloneElement(children, { id })}
    </div>
  )
}
