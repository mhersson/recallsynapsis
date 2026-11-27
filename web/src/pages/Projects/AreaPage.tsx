import { useState } from 'react'
import { Link, Navigate, useParams } from 'react-router'
import { api, message } from '../../api/client.ts'
import { touchesAreas, useStream } from '../../api/stream.ts'
import type { Area } from '../../api/types.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { useAction, useModules, useSession } from '../../session.ts'
import AreaForm from './AreaForm.tsx'
import Brief from './Brief.tsx'
import './Projects.css'

// Keyed by the id, so going to another area ends an edit.
export default function AreaPage() {
  const { id } = useParams()
  return <View key={id} id={Number(id)} />
}

function View({ id }: { id: number }) {
  const narrow = useNarrow()
  const { modules: names } = useSession()
  const modules = useModules()
  const action = useAction()
  // undefined until read, null when there is no such area.
  const [area, setArea] = useState<Area | null>()
  const [editing, setEditing] = useState(false)
  const [refused, setRefused] = useState('')

  useStream((changes) => {
    if (!touchesAreas(changes)) return
    api<{ areas: Area[] }>('GET', '/areas?archived=true').then((r) => setArea(r.areas.find((a) => a.id === id) ?? null), () => {})
  })

  if (area === undefined) return null
  if (area === null) return <Navigate to="/projects" replace />

  async function archive(archived: boolean) {
    setRefused('')
    try {
      setArea(await api<Area>('PATCH', `/areas/${id}`, { archived }))
    } catch (e) {
      setRefused(message(e))
    }
  }

  // Only a project is chosen in advance: what is captured without an area waits in the inbox.
  const here = action && area.kind === 'project' && !area.archived && (
    <button className="btn" onClick={() => action.open(area.id)}>
      {action.label} here
    </button>
  )
  const buttons = area.archived ? (
    <button className="btn" onClick={() => void archive(false)}>
      Restore
    </button>
  ) : (
    <>
      <button className="btn" onClick={() => setEditing(true)}>
        Edit
      </button>
      {area.kind === 'project' && (
        <button className="btn" onClick={() => void archive(true)}>
          Archive
        </button>
      )}
    </>
  )
  const parts = modules.filter((m) => m.section)
  const holds =
    names.length === 0 ? (
      <div className="empty">No module is switched on, so {area.kind === 'personal' ? 'the personal area' : 'this project'} holds nothing yet.</div>
    ) : (
      <div className="area-parts">
        {parts.map((m, i) => m.section && <m.section key={m.name} area={area} first={i === 0} />)}
      </div>
    )

  // The sections of more than one module take the page's whole width.
  return (
    <div className={parts.length > 1 ? 'page' : 'page narrow'}>
      {narrow && (
        <div className="crumb">
          <Link to="/projects">Projects</Link> /
        </div>
      )}
      <div className="page-head">
        <h1>{area.name}</h1>
        {area.archived && <span className="tag">archived</span>}
        {!narrow && !editing && (
          <>
            <span className="spacer" />
            {here}
            {buttons}
          </>
        )}
      </div>
      {editing ? (
        <AreaForm
          area={area}
          onSaved={(a) => {
            setArea(a)
            setEditing(false)
          }}
          onCancel={() => setEditing(false)}
        />
      ) : (
        <>
          <Brief area={area} onRestored={setArea} />
          {refused && (
            <p role="alert" className="err">
              {refused}
            </p>
          )}
          {area.archived && (
            <p className="banner">
              This project is archived. It is left out of the side bar and of the areas AI clients are offered, and everything it holds is
              kept.
            </p>
          )}
        </>
      )}
      {!(narrow && editing) && holds}
      {narrow && !editing && (
        <div className="acts stack area-foot">
          {here}
          {buttons}
        </div>
      )}
    </div>
  )
}
