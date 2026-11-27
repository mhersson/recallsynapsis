import { render } from '@testing-library/react'
import css from './Markdown.css?raw'
import { Markdown, MarkdownInline } from './Markdown.tsx'

test('a link has its address and opens in a new tab without a referrer', () => {
  const { container } = render(<Markdown text="[the docs](https://example.test/docs) and https://example.test/a." />)
  const links = [...container.querySelectorAll('a')].map((a) => [a.getAttribute('href'), a.getAttribute('target'), a.getAttribute('rel')])
  expect(links).toEqual([
    ['https://example.test/docs', '_blank', 'noreferrer'],
    ['https://example.test/a', '_blank', 'noreferrer'],
  ])
})

test('markup in the text appears as text, and no element is made from it', () => {
  const typed = ['<script>alert(1)</script>', '<b>b</b>', '<img src=x onerror=alert(1)>', '[x](javascript:alert(1))', '![i](https://example.test/i.png)', '<i>i</i>']
  const { container } = render(
    <>
      <Markdown text={`${typed[0]}\n\nA ${typed[1]}, ${typed[2]}, ${typed[3]} and ${typed[4]}`} />
      <MarkdownInline text={`${typed[5]} ${typed[3]}`} />
    </>,
  )
  expect(container.querySelector('script, b, i, img, a')).toBeNull()
  for (const s of typed) expect(container.textContent).toContain(s)
})

test('# is a heading under the page\'s own, and ## and ### step below it', () => {
  const { container } = render(<Markdown text={'# a\n## b\n### c'} />)
  expect([...container.querySelectorAll('h1, h2, h3, h4, h5')].map((h) => h.tagName)).toEqual(['H2', 'H3', 'H4'])
})

test('a table gives column headers and cells with their inline Markdown, and the heading after it is no row', () => {
  const text = ['| Item | State |', '| :-- | --: |', '| **Milk** | `a\\|b` and [list](https://example.test/l) |', '| <b>b</b> | [x](javascript:alert(1)) |', '## Next'].join('\n')
  const { container, getAllByRole, getByRole } = render(<Markdown text={text} />)
  expect(getAllByRole('columnheader').map((c) => c.textContent)).toEqual(['Item', 'State'])
  const cells = getAllByRole('cell')
  expect(cells.map((c) => c.textContent)).toEqual(['Milk', 'a|b and list', '<b>b</b>', '[x](javascript:alert(1))'])
  expect(cells.map((c) => c.style.textAlign)).toEqual(['left', 'right', 'left', 'right'])
  expect(cells[0].querySelector('strong')).toHaveTextContent('Milk')
  expect(cells[1].querySelector('code')).toHaveTextContent('a|b')
  expect([...container.querySelectorAll('a')].map((a) => a.getAttribute('href'))).toEqual(['https://example.test/l'])
  expect(container.querySelector('b')).toBeNull()
  expect(getAllByRole('row')).toHaveLength(3)
  expect(getByRole('heading', { level: 3, name: 'Next' }).closest('table')).toBeNull()
})

test('a long code line and a wide table scroll sideways in their own boxes', () => {
  const style = document.createElement('style')
  style.textContent = css
  document.head.append(style)
  const line = Array(40).fill('go test ./...').join(' && ')
  const row = Array(12).fill('column').join(' | ')
  const { container } = render(<Markdown text={['```sh', line, '```', row, Array(12).fill('---').join(' | '), row].join('\n')} />)
  const box = container.querySelector('pre')!
  expect(box.textContent).toBe(line)
  expect(box.childElementCount).toBe(0)
  const look = getComputedStyle(box)
  expect([look.overflowX, look.whiteSpace]).toEqual(['auto', 'pre'])
  expect(getComputedStyle(container.querySelector('table')!.parentElement!).overflowX).toBe('auto')
})

test('what ends the text sits in its last paragraph, or after any other last block in a paragraph of its own', () => {
  const end = <button />
  const into = render(<Markdown text={'a\n\nb *c*'} end={end} />).container
  expect([...into.querySelectorAll('.md > *')].map((b) => [b.tagName, !!b.querySelector('button')])).toEqual([
    ['P', false],
    ['P', true],
  ])
  expect(into.querySelector('p:last-child em')).toHaveTextContent('c')
  const after = render(<Markdown text={'- a\n- b'} end={end} />).container
  expect([...after.querySelectorAll('.md > *')].map((b) => [b.tagName, !!b.querySelector('button')])).toEqual([
    ['UL', false],
    ['P', true],
  ])
})

test('a line shows code, bold and italic, and makes no link, since it is often inside one', () => {
  const { container } = render(<MarkdownInline text="`create_note`, **bold**, _italic_, [docs](https://example.test/d) and https://example.test/a" />)
  expect([...container.querySelectorAll('code, strong, em')].map((e) => e.tagName)).toEqual(['CODE', 'STRONG', 'EM'])
  expect(container.querySelector('a')).toBeNull()
})
