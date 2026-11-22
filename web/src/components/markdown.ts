// The safe set of Markdown: headings to ###, paragraphs, lists one level deep, inline and fenced code,
// bold, italic and http or https links. Anything else is text as typed.

// A numbered list has its first number; a bullet list has none.
export type List = { kind: 'list'; start?: number; items: { text: string; sub?: List }[] }
// A typed block is a table, a block quote or raw HTML, kept as written.
export type Block = { kind: 'heading'; level: 1 | 2 | 3; text: string } | { kind: 'paragraph' | 'code' | 'typed'; text: string } | List
export type Inline = { kind: 'text' | 'code'; text: string } | { kind: 'strong' | 'em'; children: Inline[] } | { kind: 'link'; href: string; children: Inline[] } | { kind: 'break' }

const FENCE = /^( {0,3})(`{3,}|~{3,})[^`]*$/
const HEADING = /^ {0,3}(#{1,3})[ \t]+(.*?)[ \t]*$/
const ITEM = /^( *)([-*]|\d{1,9}\.)[ \t]+(.*)$/
const TYPED = /^ {0,3}(>|\||<[A-Za-z!/?])/
const blank = (l: string) => l.trim() === ''
const starts = (l: string) => FENCE.test(l) || HEADING.test(l) || ITEM.test(l) || TYPED.test(l)
const numbered = (marker: string) => /\d/.test(marker)
const list = (marker: string): List => (numbered(marker) ? { kind: 'list', start: parseInt(marker), items: [] } : { kind: 'list', items: [] })

export function blocks(text: string): Block[] {
  const lines = text.split(/\r?\n/)
  const out: Block[] = []
  let i = 0
  while (i < lines.length) {
    const l = lines[i]
    let m: RegExpExecArray | null
    if (blank(l)) {
      i++
    } else if ((m = FENCE.exec(l))) {
      const [, indent, fence] = m
      const shut = new RegExp(`^ {0,3}${fence[0]}{${fence.length},}[ \\t]*$`)
      const code: string[] = []
      // Unclosed, it runs to the end, as text cut at a limit often is. A fence indented under
      // an item takes that indent off each line.
      for (i++; i < lines.length && !shut.test(lines[i]); i++) {
        code.push(lines[i].slice(Math.min(indent.length, lines[i].search(/\S|$/))))
      }
      i++
      out.push({ kind: 'code', text: code.join('\n') })
    } else if ((m = HEADING.exec(l))) {
      out.push({ kind: 'heading', level: m[1].length as 1 | 2 | 3, text: m[2] })
      i++
    } else if ((m = ITEM.exec(l))) {
      const top = list(m[2])
      const base = m[1].length
      let item = { text: '' }
      let sub: List | undefined
      let deep = 0
      out.push(top)
      for (; i < lines.length; i++) {
        const l = lines[i]
        if (blank(l)) {
          // A blank line between two items keeps the list.
          let j = i + 1
          while (j < lines.length && blank(lines[j])) j++
          if (!ITEM.test(lines[j] ?? '')) break
          i = j - 1
          continue
        }
        const m = ITEM.exec(l)
        if (!m) {
          if (starts(l)) break
          item.text += '\n' + l.trim()
        } else if (m[1].length < base + 2) {
          if (numbered(m[2]) !== (top.start !== undefined)) break
          item = { text: m[3] }
          top.items.push(item)
          sub = undefined
        } else if (!sub || m[1].length < deep + 2) {
          if (!sub) {
            sub = top.items[top.items.length - 1].sub = list(m[2])
            deep = m[1].length
          }
          item = { text: m[3] }
          sub.items.push(item)
        } else {
          // A second level of nesting stays text of its item.
          item.text += '\n' + l.trim()
        }
      }
    } else if (TYPED.test(l)) {
      const from = i
      while (i < lines.length && !blank(lines[i])) i++
      out.push({ kind: 'typed', text: lines.slice(from, i).join('\n') })
    } else {
      const from = i++
      while (i < lines.length && !blank(lines[i]) && !starts(lines[i])) i++
      out.push({ kind: 'paragraph', text: lines.slice(from, i).map((x) => x.trim()).join('\n') })
    }
  }
  return out
}

// A code span: a run of backticks, then text that neither starts nor ends with one, then the same run.
const CODE = /^(`+)([^`]|[^`][\s\S]*?[^`])\1(?!`)/
const LINK = /^!?\[([^[\]]*)\]\(((?:[^\s()]|\([^\s()]*\))*)\)/
const BARE = /^https?:\/\/[^\s<>"'`]+/
const WORD = /[\p{L}\p{N}]/u
const run = (s: string, i: number) => {
  let n = 1
  while (s[i + n] === s[i]) n++
  return n
}

// Only an absolute http or https address becomes a link: without a base, new URL throws on a relative one.
function web(address: string): boolean {
  try {
    return ['http:', 'https:'].includes(new URL(address).protocol)
  } catch {
    return false
  }
}

// A bare address leaves the sentence's punctuation, and a parenthesis it never opened, outside.
function trim(url: string): string {
  for (;;) {
    const t = url.replace(/[.,;:!?*_]+$/, '')
    url = t.endsWith(')') && t.split(')').length > t.split('(').length ? t.slice(0, -1) : t
    if (url === t) return url
  }
}

// Where a run of exactly n c's that closes an emphasis starts, past code spans, or -1.
function closer(s: string, from: number, c: string, n: number): number {
  for (let j = from; j < s.length; ) {
    if (s[j] === '`') j += CODE.exec(s.slice(j))?.[0].length ?? run(s, j)
    else if (s[j] !== c) j++
    else {
      const m = run(s, j)
      if (m === n && j > from && /\S/.test(s[j - 1]) && !(WORD.test(s[j + m] ?? '') && (c === '_' || /[\p{P}\p{S}]/u.test(s[j - 1])))) return j
      j += m
    }
  }
  return -1
}

// The inline that starts at i and its end, or null and the end of the characters that stay text.
function token(s: string, i: number, links: boolean): [Inline | null, number] {
  const c = s[i]
  if (c === '\n') return [{ kind: 'break' }, i + 1]
  if (c === '`') {
    const m = CODE.exec(s.slice(i))
    return m ? [{ kind: 'code', text: m[2] }, i + m[0].length] : [null, i + run(s, i)]
  }
  if (c === '!' || (c === '[' && links)) {
    const m = LINK.exec(s.slice(i))
    // An image, and a link to any other scheme, stay as typed.
    if (m) return [c === '[' && web(m[2]) ? { kind: 'link', href: m[2], children: parse(m[1], false) } : null, i + m[0].length]
  }
  if (c === '*' || c === '_') {
    const n = run(s, i)
    const opens = n <= 2 && /\S/.test(s[i + n] ?? '') && (c === '*' || !WORD.test(s[i - 1] ?? ''))
    const j = opens ? closer(s, i + n, c, n) : -1
    return j < 0 ? [null, i + n] : [{ kind: n === 2 ? 'strong' : 'em', children: parse(s.slice(i + n, j), links) }, j + n]
  }
  if (c === 'h' && links && !WORD.test(s[i - 1] ?? '')) {
    const url = trim(BARE.exec(s.slice(i))?.[0] ?? '')
    if (web(url)) return [{ kind: 'link', href: url, children: [{ kind: 'text', text: url }] }, i + url.length]
  }
  return [null, i + 1]
}

// links is false inside a link's text, which holds no other link.
function parse(s: string, links: boolean): Inline[] {
  const out: Inline[] = []
  let text = ''
  for (let i = 0; i < s.length; ) {
    const [node, end] = token(s, i, links)
    if (node) {
      if (text) out.push({ kind: 'text', text })
      out.push(node)
      text = ''
    } else text += s.slice(i, end)
    i = end
  }
  if (text) out.push({ kind: 'text', text })
  return out
}

export function inlines(text: string): Inline[] {
  return parse(text, true)
}
