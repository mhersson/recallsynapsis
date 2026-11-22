import type { ReactNode } from 'react'
import { blocks, inlines, type Block, type Inline, type List } from './markdown.ts'
import './Markdown.css'

// React elements and text children only; a link's address is http or https, as inlines checked.
function spans(list: Inline[]): ReactNode[] {
  return list.map((n, i) => {
    switch (n.kind) {
      case 'text':
        return n.text
      case 'break':
        return <br key={i} />
      case 'code':
        return <code key={i}>{n.text}</code>
      case 'strong':
        return <strong key={i}>{spans(n.children)}</strong>
      case 'em':
        return <em key={i}>{spans(n.children)}</em>
      case 'link':
        return (
          <a key={i} href={n.href} target="_blank" rel="noreferrer">
            {spans(n.children)}
          </a>
        )
    }
  })
}

function ListOf({ list }: { list: List }) {
  const items = list.items.map((it, i) => (
    <li key={i}>
      {spans(inlines(it.text))}
      {it.sub && <ListOf list={it.sub} />}
    </li>
  ))
  return list.start === undefined ? <ul>{items}</ul> : <ol start={list.start}>{items}</ol>
}

// A heading sits under the page's own h1, so # is an h2.
function block(b: Block, key: number): ReactNode {
  switch (b.kind) {
    case 'heading': {
      const H = `h${b.level + 1}` as 'h2' | 'h3' | 'h4'
      return <H key={key}>{spans(inlines(b.text))}</H>
    }
    case 'paragraph':
      return <p key={key}>{spans(inlines(b.text))}</p>
    case 'list':
      return <ListOf key={key} list={b} />
    case 'code':
      return <pre key={key}>{b.text}</pre>
    case 'typed':
      return (
        <p key={key} className="typed">
          {b.text}
        </p>
      )
  }
}

export function Markdown({ text }: { text: string }) {
  return <div className="md">{blocks(text).map(block)}</div>
}

export function MarkdownInline({ text }: { text: string }) {
  return <span className="md">{spans(inlines(text))}</span>
}
