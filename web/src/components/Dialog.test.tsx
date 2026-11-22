import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it, vi } from 'vitest'
import Dialog from './Dialog.tsx'

it('the dialog is named by its title', () => {
  render(<Dialog title="New project" onClose={() => {}}>body</Dialog>)
  expect(screen.getByRole('dialog', { name: 'New project' })).toHaveAttribute('aria-modal', 'true')
})

it('Escape closes the dialog', async () => {
  const onClose = vi.fn()
  render(<Dialog title="New project" onClose={onClose}>body</Dialog>)
  await userEvent.keyboard('{Escape}')
  expect(onClose).toHaveBeenCalledTimes(1)
})
