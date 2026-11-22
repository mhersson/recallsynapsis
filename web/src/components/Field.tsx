import { cloneElement, useId, type ReactElement, type ReactNode } from 'react'
import './Field.css'

type Props = { label: ReactNode; children: ReactElement<{ id?: string }> }

export default function Field({ label, children }: Props) {
  const id = useId()
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {cloneElement(children, { id })}
    </div>
  )
}
