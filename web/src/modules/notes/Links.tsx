import { useEffect, useState } from 'react'
import { Link } from 'react-router'
import { api, message } from '../../api/client.ts'
import type { Area, Group, Hit } from '../../api/types.ts'
import { SearchIcon } from '../../components/icons.tsx'
import { back } from '../../components/useClose.ts'
import { useItem } from '../../session.ts'
import { nameOf, type Linked, type Note } from './api.ts'
import { LinkIcon, XIcon } from './icons.tsx'
import Section from './Section.tsx'

type Props = { note: Note; areas: Area[]; trashed: boolean; sending: boolean; onLinks: (refs: string[]) => Promise<string> }

// What the note links to and what links to it, each with the mark of what it names. What it links to is changed here,
// each removal and addition a change of the note, so its history can bring a link back; what links to it is what other
// items say. A note in the trash changes nothing.
export default function Links({ note, areas, trashed, sending, onLinks }: Props) {
  const item = useItem()
  const [finding, setFinding] = useState(false)
  const [refused, setRefused] = useState('')
  const refs = note.links.map((l) => l.ref)
  const change = (next: string[]) =>
    onLinks(next).then((line) => {
      setRefused(line)
      if (!line) setFinding(false)
    })

  const row = (l: Linked, removable: boolean) => {
    const it = l.available ? item(l.ref, l.kind, l.state) : undefined
    return (
      <div key={l.ref} className="lrow">
        <i className={it ? `mk ${it.mark}` : 'mk'} />
        {!l.available ? (
          <>
            <span className="mono">{l.ref}</span>
            <span className="tag">unavailable</span>
          </>
        ) : it ? (
          <Link to={it.to} state={back}>
            {l.title}
          </Link>
        ) : (
          <span>{l.title}</span>
        )}
        {removable && !trashed && (
          <button className="xb" aria-label={`Remove the link to ${l.title || l.ref}`} disabled={sending} onClick={() => void change(refs.filter((r) => r !== l.ref))}>
            <XIcon />
          </button>
        )}
      </div>
    )
  }

  const add = !trashed && !finding && (
    <button className="btn link" onClick={() => setFinding(true)}>
      <LinkIcon />
      Add a link
    </button>
  )
  return (
    <Section title="Links" count={note.links.length + note.linked_from.length} add={add}>
      {refused && (
        <p role="alert" className="err">
          {refused}
        </p>
      )}
      {note.links.length > 0 && <div className="lgrp">links to</div>}
      {note.links.map((l) => row(l, true))}
      {finding && !trashed && <Finder note={note} refs={refs} areas={areas} sending={sending} onPick={(ref) => void change([...refs, ref])} onClose={() => setFinding(false)} />}
      {note.linked_from.length > 0 && <div className="lgrp">linked from</div>}
      {note.linked_from.map((l) => row(l, false))}
    </Section>
  )
}

type FinderProps = { note: Note; refs: string[]; areas: Area[]; sending: boolean; onPick: (ref: string) => void; onClose: () => void }

// Finds an item to link to through search, every module and every area, leaving out the note itself; what the note
// links to already is shown and cannot be picked again.
function Finder({ note, refs, areas, sending, onPick, onClose }: FinderProps) {
  const item = useItem()
  const [q, setQ] = useState('')
  const query = q.trim()
  const [found, setFound] = useState<{ q: string; hits: Hit[] }>()
  const [failed, setFailed] = useState('')
  useEffect(() => {
    if (!query) return
    let stale = false
    // Typing asks once it pauses, and an answer to a question asked before is dropped.
    const timer = setTimeout(() => {
      api<{ groups: Group[] }>('GET', '/search?' + new URLSearchParams({ q: query })).then(
        (r) => {
          if (stale) return
          setFound({ q: query, hits: r.groups.flatMap((g) => g.hits).filter((h) => h.ref !== note.ref) })
          setFailed('')
        },
        (e: unknown) => !stale && setFailed(message(e)),
      )
    }, 250)
    return () => {
      stale = true
      clearTimeout(timer)
    }
  }, [query, note.ref])

  const hits = (query && found?.hits) || []
  let said = 'Pick one to link to it.'
  if (failed) said = failed
  else if (query && found?.q === query && hits.length === 0) said = `Nothing matches “${query}”.`
  return (
    <div className="finder">
      <label className="search-field">
        <SearchIcon />
        <input type="search" aria-label="Find what to link to" autoFocus value={q} onChange={(e) => setQ(e.target.value)} />
      </label>
      {hits.length > 0 && (
        <div className="fhits">
          {hits.map((h) => {
            const it = item(h.ref, h.kind, h.state)
            const linked = refs.includes(h.ref)
            const state = it ? it.state : h.state
            return (
              <button key={h.ref} className="fhit" disabled={linked || sending} onClick={() => onPick(h.ref)}>
                <i className={it ? `mk ${it.mark}` : 'mk'} />
                <span className="item">
                  <span className="t1">{h.title}</span>
                  {/* A short note with no title is its own snippet. */}
                  {h.snippet && h.snippet !== h.title && <span className="t3">{h.snippet}</span>}
                  <span className="t2">
                    <span>{nameOf(areas, h.area)}</span>
                    <span>{h.kind}</span>
                    {state && <span>{state}</span>}
                    {linked && <span>linked already</span>}
                  </span>
                </span>
              </button>
            )
          })}
        </div>
      )}
      <div className="acts">
        <span className={failed ? 'err' : 'note'} role={failed ? 'alert' : undefined}>
          {said}
        </span>
        <span className="spacer" />
        <button className="btn sm" onClick={onClose}>
          Cancel
        </button>
      </div>
    </div>
  )
}
