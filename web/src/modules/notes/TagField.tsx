import { useState } from 'react'

// As the server stores a tag: lower-cased, without the # it starts with.
const clean = (w: string) => w.replace(/^#+/, '').toLowerCase()

// Tags as chips. A space, a comma, Enter or leaving the field ends the one being typed, Backspace in the empty field
// removes the last, and pressing a chip removes it.
export default function TagField({ tags, onChange }: { tags: string[]; onChange: (tags: string[]) => void }) {
  const [draft, setDraft] = useState('')
  const add = (words: string[]) => {
    const next = [...new Set([...tags, ...words.map(clean).filter(Boolean)])]
    if (next.length > tags.length) onChange(next)
  }
  const end = () => {
    add([draft])
    setDraft('')
  }
  return (
    <div className="tag-field">
      {tags.map((t) => (
        <button key={t} type="button" className="tag" aria-label={`Remove ${t}`} onClick={() => onChange(tags.filter((x) => x !== t))}>
          #{t} ×
        </button>
      ))}
      <input
        aria-label="Tags"
        placeholder="Add a tag"
        autoCapitalize="none"
        autoCorrect="off"
        spellCheck={false}
        value={draft}
        onChange={(e) => {
          const words = e.target.value.split(/[\s,]+/)
          setDraft(words.pop()!)
          add(words)
        }}
        onBlur={end}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            end()
          } else if (e.key === 'Backspace' && !draft) onChange(tags.slice(0, -1))
        }}
      />
    </div>
  )
}
