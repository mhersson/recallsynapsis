import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { api, message } from '../../api/client.ts'
import { touchesAreas, useStream } from '../../api/stream.ts'
import type { Area, Group } from '../../api/types.ts'
import PageHead from '../../components/PageHead.tsx'
import { SearchIcon } from '../../components/icons.tsx'
import { ago } from '../../components/time.ts'
import { back } from '../../components/useClose.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { useItem, useSession } from '../../session.ts'
import './Search.css'

// What a search answered, with the question it answered, which the page names while a newer one is typed.
type Result = { q: string; area: string; groups: Group[] }

// The question lives in the address, so a reload or a step back asks it again. The area is a slug.
export default function Search() {
  const narrow = useNarrow()
  const none = useSession().modules.length === 0
  const item = useItem()
  const [params, setParams] = useSearchParams()
  const [q, setQ] = useState(params.get('q') ?? '')
  // Spaces around the words would be searched for as part of them.
  const query = q.trim()
  const [area, setArea] = useState(params.get('area') ?? '')
  const [areas, setAreas] = useState<Area[]>([])
  const [result, setResult] = useState<Result | null>(null)
  const [failed, setFailed] = useState('')
  // Bumped to ask again when a module changed something.
  const [asked, setAsked] = useState(0)

  useStream((changes) => {
    if (touchesAreas(changes)) api<{ areas: Area[] }>('GET', '/areas?archived=true').then((r) => setAreas(r.areas), () => {})
    if (!changes || changes.some((c) => c.module !== 'core')) setAsked((n) => n + 1)
  })

  useEffect(() => {
    if (none || !query) return
    let stale = false
    // Typing asks once it pauses.
    const timer = setTimeout(() => {
      api<{ groups: Group[] }>('GET', '/search?' + new URLSearchParams(area ? { q: query, area } : { q: query })).then(
        (r) => {
          if (stale) return
          setResult({ q: query, area, groups: r.groups })
          setFailed('')
        },
        (e: unknown) => !stale && setFailed(message(e)),
      )
    }, 250)
    return () => {
      stale = true
      clearTimeout(timer)
    }
  }, [none, query, area, asked])

  function ask(next: { q?: string; area?: string }) {
    const n = { q, area, ...next }
    setQ(n.q)
    setArea(n.area)
    setParams(Object.fromEntries(Object.entries(n).filter(([, v]) => v)), { replace: true })
  }

  const name = (by: (a: Area) => boolean) => areas.find(by)?.name ?? ''
  let body
  if (none) body = <div className="empty">No module is switched on, so there is nothing to search yet.</div>
  else if (!query) body = <p className="note">Finds parts of words too, so backup finds backupjobben.</p>
  else if (failed)
    body = (
      <p role="alert" className="err search-failed">
        {failed}
      </p>
    )
  else if (result?.groups.length === 0)
    body = result.area ? (
      <div className="empty">
        Nothing in <span className="mono">{name((a) => a.slug === result.area) || result.area}</span> matches “{result.q}”.{' '}
        <button className="btn link" onClick={() => ask({ area: '' })}>
          Search every area
        </button>
      </div>
    ) : (
      <div className="empty">Nothing matches “{result.q}”.</div>
    )
  else
    body = result?.groups.map((g) => (
      <section key={g.module}>
        <h2 className="ghd hits-head">{g.module}</h2>
        <div className="list">
          {g.hits.map((h) => {
            const it = item(h.ref, h.kind, h.state)
            // Once a module gives the item, its words replace the raw state, and none hides it.
            const state = it ? it.state : h.state
            const inner = (
              <>
                <i className={it ? `mk ${it.mark}` : 'mk'} />
                <span className="hit">
                  <span className="t1">{h.title}</span>
                  {h.snippet && <span className="t3">{h.snippet}</span>}
                  <span className="t2">
                    <span>{name((a) => a.slug === h.area)}</span>
                    <span>{h.kind}</span>
                    {state && <span>{state}</span>}
                    <span>{ago(h.changed_at)}</span>
                  </span>
                </span>
              </>
            )
            return it ? (
              <Link key={h.ref} className="row" to={it.to} state={back}>
                {inner}
              </Link>
            ) : (
              <div key={h.ref} className="row">
                {inner}
              </div>
            )
          })}
        </div>
      </section>
    ))

  return (
    <div className="page narrow">
      {narrow && (
        <div className="crumb">
          <Link to="/more">More</Link> /
        </div>
      )}
      <PageHead title="Search" />
      <div className="search-top">
        <label className="search-field">
          <SearchIcon />
          <input type="search" aria-label="Search" placeholder="Search every area" autoFocus value={q} onChange={(e) => ask({ q: e.target.value })} />
        </label>
        <select aria-label="Area" value={area} onChange={(e) => ask({ area: e.target.value })}>
          <option value="">All areas</option>
          {areas
            .filter((a) => !a.archived)
            .map((a) => (
              <option key={a.id} value={a.slug}>
                {a.name}
              </option>
            ))}
        </select>
      </div>
      {body}
    </div>
  )
}
