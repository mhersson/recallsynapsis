import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import TagField from './TagField.tsx'

// The field, and the tags it holds as its owner keeps them.
function Tags() {
  const [tags, setTags] = useState(['a'])
  return (
    <>
      <TagField tags={tags} onChange={setTags} />
      <output>{tags.join(' ')}</output>
    </>
  )
}

const held = () => screen.getByRole('status').textContent

test('a space, a comma or Enter ends a tag, kept as the server keeps it and once', async () => {
  render(<Tags />)
  await userEvent.type(screen.getByRole('textbox'), '#Nas, home A{Enter}')
  expect(held()).toBe('a nas home')
  expect(screen.getByRole('textbox')).toHaveValue('')
})

test('Backspace in the empty field removes the last tag, and pressing a tag removes it', async () => {
  render(<Tags />)
  await userEvent.type(screen.getByRole('textbox'), 'b c ')
  await userEvent.type(screen.getByRole('textbox'), '{Backspace}')
  expect(held()).toBe('a b')
  await userEvent.click(screen.getAllByRole('button')[0])
  expect(held()).toBe('b')
})
