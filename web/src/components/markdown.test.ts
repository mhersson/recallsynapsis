import { blocks, inlines, type Align, type Block, type Inline } from './markdown.ts'

const t = (text: string): Inline => ({ kind: 'text', text })
const table = (head: string[], rows: string[][], align: Align[] = head.map(() => undefined)): Block => ({ kind: 'table', align, head, rows })
const code = (text: string): Inline => ({ kind: 'code', text })
const link = (href: string, text = href): Inline => ({ kind: 'link', href, children: [t(text)] })
const p = (text: string): Block => ({ kind: 'paragraph', text })

test('#, ## and ### give headings of level 1 to 3, and #### is a paragraph as typed', () => {
  expect(blocks('# One\n## Two\n### Three\n#### Four')).toEqual([
    { kind: 'heading', level: 1, text: 'One' },
    { kind: 'heading', level: 2, text: 'Two' },
    { kind: 'heading', level: 3, text: 'Three' },
    p('#### Four'),
  ])
})

test('a single line break inside a paragraph is kept, and a blank line starts the next', () => {
  expect(blocks('one\ntwo\r\n\nthree')).toEqual([p('one\ntwo'), p('three')])
  expect(inlines('one\ntwo')).toEqual([t('one'), { kind: 'break' }, t('two')])
})

test('-, * and 1. lists nest one level by indent, and a second level is text of its item', () => {
  expect(blocks('Changes:\n- a\n  - a1\n    - a2\n  - a3\n* b\n\n1. c\n2. d')).toEqual([
    p('Changes:'),
    {
      kind: 'list',
      items: [{ text: 'a', sub: { kind: 'list', items: [{ text: 'a1\n- a2' }, { text: 'a3' }] } }, { text: 'b' }],
    },
    { kind: 'list', start: 1, items: [{ text: 'c' }, { text: 'd' }] },
  ])
})

test('a fence inside a numbered step leaves the next step its number', () => {
  expect(blocks('1. Run:\n   ```sh\n   make check\n   ```\n2. Push')).toEqual([
    { kind: 'list', start: 1, items: [{ text: 'Run:' }] },
    { kind: 'code', text: 'make check' },
    { kind: 'list', start: 2, items: [{ text: 'Push' }] },
  ])
})

test('inline code', () => {
  expect(inlines('run `make check` and `a*b*c`')).toEqual([t('run '), code('make check'), t(' and '), code('a*b*c')])
})

test('a fenced block drops its language word', () => {
  expect(blocks('```go\nfunc main() {}\n\n// done\n```\nafter')).toEqual([{ kind: 'code', text: 'func main() {}\n\n// done' }, p('after')])
})

test('an unclosed fence runs to the end', () => {
  expect(blocks('before\n```\nline one\n\nline two')).toEqual([p('before'), { kind: 'code', text: 'line one\n\nline two' }])
})

test('a heading or a list marker inside a fence stays code', () => {
  expect(blocks('```\n# not a heading\n- not an item\n1. nor this\n```')).toEqual([{ kind: 'code', text: '# not a heading\n- not an item\n1. nor this' }])
})

test('**bold**, *italic* and _italic_', () => {
  expect(inlines('**b** *i* _j_ **a `*` *d* e**')).toEqual([
    { kind: 'strong', children: [t('b')] },
    t(' '),
    { kind: 'em', children: [t('i')] },
    t(' '),
    { kind: 'em', children: [t('j')] },
    t(' '),
    { kind: 'strong', children: [t('a '), code('*'), t(' '), { kind: 'em', children: [t('d')] }, t(' e')] },
  ])
})

test('_ inside a word is never emphasis, and a * or _ before a space opens nothing', () => {
  for (const s of ['push_notified and max_progress_text', '2 * 3 * 4', 'a _ b _ c', 'src/**/*.ts']) {
    expect(inlines(s), s).toEqual([t(s)])
  }
})

test('a * after punctuation and before a word closes nothing', () => {
  for (const s of ['(*Loop).List and (*Server).Serve', '[]*Task and map[int]*Store']) expect(inlines(s), s).toEqual([t(s)])
})

test('an unclosed backtick or ** is shown as typed', () => {
  for (const s of ['a `b c', 'a **b c', '``a`']) expect(inlines(s), s).toEqual([t(s)])
})

test('[text](address) and a bare address give links', () => {
  expect(inlines('[the **docs**](https://example.test/docs) or http://example.test/a')).toEqual([
    { kind: 'link', href: 'https://example.test/docs', children: [t('the '), { kind: 'strong', children: [t('docs')] }] },
    t(' or '),
    link('http://example.test/a'),
  ])
})

test("a bare address leaves a sentence's closing punctuation outside", () => {
  expect(inlines('(see https://example.test/a), https://example.test/b. https://example.test/c_(d)')).toEqual([
    t('(see '),
    link('https://example.test/a'),
    t('), '),
    link('https://example.test/b'),
    t('. '),
    link('https://example.test/c_(d)'),
  ])
})

test('a javascript link is shown as typed, as are mailto:, data: and relative addresses', () => {
  for (const s of ['[x](javascript:alert(1))', '[x](mailto:a@example.test)', '[x](data:text/html,<b>x</b>)', '[x](docs/api.md)', '[x](/tasks/1)']) {
    expect(inlines(s), s).toEqual([t(s)])
  }
})

test('raw HTML is shown as typed', () => {
  expect(blocks('<div onclick="x">\n<b>hi</b>\n</div>\n\nafter')).toEqual([{ kind: 'typed', text: '<div onclick="x">\n<b>hi</b>\n</div>' }, p('after')])
  expect(inlines('a <b>b</b> <img src=x onerror=alert(1)>')).toEqual([t('a <b>b</b> <img src=x onerror=alert(1)>')])
})

test('an image and a block quote are shown as typed', () => {
  expect(blocks('| a | b |\n|---|---|\n| 1 | 2 |\n\n> quoted\n> on')).toEqual([table(['a', 'b'], [['1', '2']]), { kind: 'typed', text: '> quoted\n> on' }])
  expect(inlines('![a](https://example.test/a.png)')).toEqual([t('![a](https://example.test/a.png)')])
})

test('a row over a rule of dashes is a table, with or without the outer pipes, and a heading ends it', () => {
  const want: Block[] = [table(['Item', 'State'], [['Milk', 'Ready']]), { kind: 'heading', level: 2, text: 'Next' }]
  expect(blocks('| Item | State |\n| --- | --- |\n| Milk | Ready |\n## Next')).toEqual(want)
  expect(blocks('Item | State\n--- | ---\nMilk | Ready\n## Next')).toEqual(want)
})

test("a rule's colons align its columns", () => {
  expect(blocks('a | b | c | d\n:-- | :-: | --: | -')).toEqual([table(['a', 'b', 'c', 'd'], [], ['left', 'center', 'right', undefined])])
})

test('an escaped pipe stays in its cell, and a row is cut or filled to the columns', () => {
  expect(blocks('| a | b |\n|---|---|\n| x \\| y | `p\\|q` |\n| 1 |\n| 1 | 2 | 3 |\nlast')).toEqual([
    table(['a', 'b'], [['x | y', '`p|q`'], ['1', ''], ['1', '2'], ['last', '']]),
  ])
})

test('a table ends a paragraph and a list, and a list item or a fence ends a table', () => {
  expect(blocks('Shopping:\n| a | b |\n|---|---|\n| 1 | 2 |\n- after\n| c | d |\n|-|-|\n```\ncode\n```')).toEqual([
    p('Shopping:'),
    table(['a', 'b'], [['1', '2']]),
    { kind: 'list', items: [{ text: 'after' }] },
    table(['c', 'd'], []),
    { kind: 'code', text: 'code' },
  ])
})

test('text with a pipe stays text, and so does a table in a fence', () => {
  for (const s of ['a | b', '| a | b |\n| c | d |', 'a | b | c\n--- | ---', 'a\n---']) expect(blocks(s), s).toEqual([p(s)])
  expect(blocks('```\n| a | b |\n|---|---|\n```')).toEqual([{ kind: 'code', text: '| a | b |\n|---|---|' }])
})
