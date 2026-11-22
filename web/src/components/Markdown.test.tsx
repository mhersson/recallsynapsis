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

test('a long code line scrolls sideways in its own box', () => {
  const style = document.createElement('style')
  style.textContent = css
  document.head.append(style)
  const line = Array(40).fill('go test ./...').join(' && ')
  const { container } = render(<Markdown text={'```sh\n' + line + '\n```'} />)
  const box = container.querySelector('pre')!
  expect(box.textContent).toBe(line)
  expect(box.childElementCount).toBe(0)
  const look = getComputedStyle(box)
  expect([look.overflowX, look.whiteSpace]).toEqual(['auto', 'pre'])
})
