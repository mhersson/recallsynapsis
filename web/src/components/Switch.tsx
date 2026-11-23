import type { ComponentProps } from 'react'
import './Switch.css'

type Props = Omit<ComponentProps<'button'>, 'onChange'> & { on: boolean; onChange: (on: boolean) => void }

// On or off, as a button with the role switch; a press asks for the other state, and on shows what holds.
export default function Switch({ on, onChange, ...rest }: Props) {
  return <button {...rest} type="button" role="switch" className="switch" aria-checked={on} onClick={() => onChange(!on)} />
}
