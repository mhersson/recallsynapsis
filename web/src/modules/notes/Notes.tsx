import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { nameOf, useAreas } from '../../api/areas.ts'
import PageHead from '../../components/PageHead.tsx'
import { useClose } from '../../components/useClose.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { useList, useRead, type Row } from './api.ts'
import NoteView from './NoteView.tsx'
import { ListEnd, Rows } from './Rows.tsx'
import './Notes.css'

type Filter = { area: string; kind: string; tag: string }

// The Notes list, or the idea bank, with the note open beside it on desktop and as a page of its own on a phone. The
// dropdowns' choice and the note open live in the address, so a reload keeps them.
export default function Notes({ bank = false }: { bank?: boolean }) {
  const narrow = useNarrow()
  const navigate = useNavigate()
  const id = Number(useParams().id) || undefined
  const [params, setParams] = useSearchParams()
  const filter: Filter = { area: params.get('area') ?? '', kind: bank ? '' : (params.get('kind') ?? ''), tag: params.get('tag') ?? '' }
  const search = params.toString() ? `?${params}` : ''
  const base = bank ? '/ideas' : '/notes'
  const close = useClose(base + search)
  const areas = useAreas(true)
  // A note open as a page of its own needs no list.
  const alone = narrow && !!id
  const asked = new URLSearchParams(Object.entries({ inbox: 'false', area: filter.area, kind: bank ? 'idea' : filter.kind, tag: filter.tag }).filter(([, v]) => v)).toString()
  const { list, failed, more, reread } = useList(alone ? '' : asked)
  const tags = useRead<{ tags: { tag: string }[] }>(alone ? '' : '/notes/tags').got?.tags ?? []
  const title = bank ? 'Idea bank' : 'Notes'
  // Notes on desktop opens the newest by naming it in the address, so it stays open whatever the list does meanwhile.
  // The note trashed here is passed over, as the list may still hold it until it is read again.
  const [trashed, setTrashed] = useState<number>()
  const newest = list?.notes.find((r) => r.id !== trashed)?.id
  useEffect(() => {
    if (!narrow && !bank && !id && newest) void navigate(`/notes/${newest}${search}`, { replace: true })
  }, [narrow, bank, id, newest, search, navigate])

  const set = (f: Partial<Filter>) => setParams(Object.entries({ ...filter, ...f }).filter(([, v]) => v), { replace: true })
  const select = (label: string, key: keyof Filter, all: string, options: string[][]) => (
    <select aria-label={label} value={filter[key]} onChange={(e) => set({ [key]: e.target.value })}>
      <option value="">{all}</option>
      {options.map(([v, text]) => (
        <option key={v} value={v}>
          {text}
        </option>
      ))}
    </select>
  )
  const rows = (notes: Row[]) => <Rows notes={notes} areas={areas} to={(r) => `${base}/${r.id}${search}`} open={id} />
  const filtered = !!(filter.area || filter.kind || filter.tag)
  const clear = (
    <button className="btn link" onClick={() => setParams({}, { replace: true })}>
      Clear the filters
    </button>
  )

  let listed
  if (failed)
    listed = (
      <div className="empty" role="alert">
        <b>{failed}</b>
        {filtered && clear}
      </div>
    )
  else if (!list) listed = null
  else if (list.notes.length === 0)
    listed = bank ? (
      <div className="empty">
        <b>{filtered ? 'No idea matches these filters.' : 'No ideas yet.'}</b>
        {filtered ? clear : 'Capture one and choose Idea, or ask an AI client to put a thought in the idea bank.'}
      </div>
    ) : (
      <div className="empty">
        <b>{filtered ? 'No note or idea matches these filters.' : 'Nothing is filed yet.'}</b>
        {filtered ? clear : 'A capture shows here once it is filed from the Inbox.'}
      </div>
    )
  else if (bank)
    listed = [...new Set(list.notes.map((r) => r.area))].map((a) => (
      <section key={a}>
        <h2 className="ghd">{nameOf(areas, a)}</h2>
        {rows(list.notes.filter((r) => r.area === a))}
      </section>
    ))
  else listed = rows(list.notes)

  // The note keeps its place in the page on a phone and on desktop, so turning a phone never ends an edit.
  return (
    <div className={alone ? 'page narrow' : 'page notes'}>
      {alone ? (
        <div className="crumb">
          <Link to={base + search}>{title}</Link> /
        </div>
      ) : (
        <>
          {narrow && bank && (
            <div className="crumb">
              <Link to="/more">More</Link> /
            </div>
          )}
          <PageHead title={title} />
          <p className="sub">{bank ? 'Every idea, across the personal area and the projects.' : 'The most recently changed first.'}</p>
        </>
      )}
      <div className={narrow ? undefined : 'panes'}>
        {!alone && (
          <div>
            <div className="filters">
              {select(
                'Area',
                'area',
                'All areas',
                areas.filter((a) => !a.archived || a.slug === filter.area).map((a) => [a.slug, a.name]),
              )}
              {!bank &&
                select('Kind', 'kind', 'All kinds', [
                  ['note', 'Notes'],
                  ['idea', 'Ideas'],
                ])}
              {select(
                'Tag',
                'tag',
                'Any tag',
                tags.map((t) => [t.tag, `#${t.tag}`]),
              )}
            </div>
            {listed}
            {list && !failed && <ListEnd list={list} more={more} />}
          </div>
        )}
        {id ? (
          <div className={narrow ? undefined : 'box'}>
            <NoteView
              key={id}
              id={id}
              areas={areas}
              onGone={() => {
                setTrashed(id)
                close()
              }}
              onChanged={reread}
            />
          </div>
        ) : (
          bank && !narrow && !!list?.notes.length && <div className="empty">Pick an idea to read it here.</div>
        )}
      </div>
    </div>
  )
}
