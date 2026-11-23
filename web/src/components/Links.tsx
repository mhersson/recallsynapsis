import { useEffect, useState } from 'react'
import { Link } from 'react-router'
import { nameOf } from '../api/areas.ts'
import { api, message } from '../api/client.ts'
import type { Area, Group, Hit, Linked } from '../api/types.ts'
import { useItem, useRanked } from '../session.ts'
import { LinkIcon, SearchIcon, XIcon } from './icons.tsx'
import Section from './Section.tsx'
import { back } from './useClose.ts'
import './Links.css'

// The item whose links are shown, of any module.
type Of = { ref: string; links: Linked[]; linked_from: Linked[] }

type Props = { of: Of; areas: Area[]; trashed: boolean; sending: boolean; open?: boolean; onLinks: (refs: string[]) => Promise<string> }

// What an item links to, each removed or added at once as a change of the item, and what links to it. An item in the
// trash changes nothing, and shows the section only when it has a link. On a phone it is folded unless open.
export default function Links({ of, areas, trashed, sending, open, onLinks }: Props) {
  const item = useItem()
  const [finding, setFinding] = useState(false)
  const [refused, setRefused] = useState('')
  const refs = of.links.map((l) => l.ref)
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

  if (trashed && of.links.length + of.linked_from.length === 0) return null
  const add = !trashed && !finding && (
    <button className="btn link" onClick={() => setFinding(true)}>
      <LinkIcon />
      Add a link
    </button>
  )
  return (
    <Section title="Links" count={of.links.length + of.linked_from.length} add={add} open={open}>
      {refused && (
        <p role="alert" className="err">
          {refused}
        </p>
      )}
      {of.links.length > 0 && <div className="lgrp">links to</div>}
      {of.links.map((l) => row(l, true))}
      {finding && !trashed && <Finder self={of.ref} refs={refs} areas={areas} sending={sending} onPick={(ref) => void change([...refs, ref])} onClose={() => setFinding(false)} />}
      {of.linked_from.length > 0 && <div className="lgrp">linked from</div>}
      {of.linked_from.map((l) => row(l, false))}
    </Section>
  )
}

type FinderProps = { self: string; refs: string[]; areas: Area[]; sending: boolean; onPick: (ref: string) => void; onClose: () => void }

// Finds an item to link to through search, every module and every area, leaving out the item itself, self; what it
// links to already is shown and cannot be picked again.
function Finder({ self, refs, areas, sending, onPick, onClose }: FinderProps) {
  const item = useItem()
  const ranked = useRanked()
  const [q, setQ] = useState('')
  const query = q.trim()
  const [found, setFound] = useState<{ q: string; groups: Group[] }>()
  const [failed, setFailed] = useState('')
  useEffect(() => {
    if (!query) return
    let stale = false
    // Typing asks once it pauses, and an answer to a question asked before is dropped.
    const timer = setTimeout(() => {
      api<{ groups: Group[] }>('GET', '/search?' + new URLSearchParams({ q: query })).then(
        (r) => {
          if (stale) return
          setFound({ q: query, groups: r.groups })
          setFailed('')
        },
        (e: unknown) => !stale && setFailed(message(e)),
      )
    }, 250)
    return () => {
      stale = true
      clearTimeout(timer)
    }
  }, [query])

  const hits: Hit[] = (query && found && ranked(found.groups).flatMap((g) => g.hits).filter((h) => h.ref !== self)) || []
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
