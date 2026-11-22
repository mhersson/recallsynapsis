import { render, screen } from '@testing-library/react'
import { expect, it } from 'vitest'
import Field from './Field.tsx'

it("a field's label names its control", () => {
  render(
    <Field label="Title">
      <input />
    </Field>,
  )
  expect(screen.getByLabelText('Title')).toBeInstanceOf(HTMLInputElement)
})
