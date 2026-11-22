import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import Dialog from './Dialog.tsx'

afterEach(() => vi.restoreAllMocks())

it('opens as a modal dialog of the browser, named by its title', () => {
  const showModal = vi.spyOn(HTMLDialogElement.prototype, 'showModal')
  render(<Dialog title="New project" onClose={() => {}}>body</Dialog>)
  expect(screen.getByRole('dialog', { name: 'New project' })).toHaveAttribute('open')
  expect(showModal).toHaveBeenCalledTimes(1)
})

it('the cancel that Escape sends asks to close the dialog, and does not close it', () => {
  const onClose = vi.fn()
  render(<Dialog title="New project" onClose={onClose}>body</Dialog>)
  expect(fireEvent(screen.getByRole('dialog'), new Event('cancel', { cancelable: true }))).toBe(false)
  expect(onClose).toHaveBeenCalledTimes(1)
})

it('is closed while it is still in the page', () => {
  const connected: boolean[] = []
  vi.spyOn(HTMLDialogElement.prototype, 'close').mockImplementation(function (this: HTMLDialogElement) {
    connected.push(this.isConnected)
  })
  const { unmount } = render(<Dialog title="New project" onClose={() => {}}>body</Dialog>)
  unmount()
  expect(connected).toEqual([true])
})
