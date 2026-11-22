import { cloneElement, useId, type ReactElement } from 'react'
import './Field.css'

type Props = { label: string; children: ReactElement<{ id?: string }> }

export default function Field({ label, children }: Props) {
  const id = useId()
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {cloneElement(children, { id })}
    </div>
  )
}
