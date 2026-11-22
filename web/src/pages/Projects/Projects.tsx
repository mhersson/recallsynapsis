import { useState } from 'react'
import { Link } from 'react-router'
import { api, message } from '../../api/client.ts'
import { touchesAreas, useStream } from '../../api/stream.ts'
import type { Area } from '../../api/types.ts'
import Dialog from '../../components/Dialog.tsx'
import PageHead from '../../components/PageHead.tsx'
import { ProjectsIcon } from '../../components/icons.tsx'
import { back, useClose } from '../../components/useClose.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { useSession } from '../../session.ts'
import AreaForm from './AreaForm.tsx'
import './Projects.css'

function count(n: number) {
  if (n === 0) return 'Your personal area, and no projects yet.'
  return `Your personal area and ${n} ${n === 1 ? 'project' : 'projects'}.`
}

// making opens the form that makes a project: over the list on desktop, as its own page on a phone.
export default function Projects({ making = false }: { making?: boolean }) {
  const narrow = useNarrow()
  const { modules } = useSession()
  const [areas, setAreas] = useState<Area[] | null>(null)
  const [refused, setRefused] = useState('')

  // The list and the form share this page, so it stays mounted while the form opens and closes, and reads after each change.
  const read = () => api<{ areas: Area[] }>('GET', '/areas?archived=true').then((r) => setAreas(r.areas), () => {})
  useStream((changes) => {
    if (touchesAreas(changes)) void read()
  })

  async function restore(a: Area) {
    setRefused('')
    try {
      await api('PATCH', `/areas/${a.id}`, { archived: false })
      void read()
    } catch (e) {
      setRefused(message(e))
    }
  }

  const close = useClose('/projects')
  const made = () => {
    close()
    void read()
  }

  if (narrow && making)
    return (
      <div className="page">
        <div className="crumb">
          <Link to="/projects">Projects</Link> /
        </div>
        <PageHead title="New project" />
        <AreaForm onSaved={made} onCancel={close} />
      </div>
    )

  const row = (a: Area) => (
    <div key={a.id} className="row">
      <ProjectsIcon />
      <Link to={`/areas/${a.id}`}>
        <span className={a.kind === 'project' ? 't1 mono' : 't1'}>{a.name}</span>
        {a.description && <span className="t3">{a.description}</span>}
      </Link>
      {a.archived && (
        <button className="btn sm" onClick={() => void restore(a)}>
          Restore
        </button>
      )}
    </div>
  )
  const live = areas?.filter((a) => !a.archived) ?? []
  const projects = live.filter((a) => a.kind === 'project')
  const archived = areas?.filter((a) => a.archived) ?? []

  return (
    <div className="page narrow">
      <PageHead title="Projects">
        <Link className="btn" to="/areas/new" state={back}>
          New project
        </Link>
      </PageHead>
      {areas && (
        <>
          <p className="sub">{count(projects.length)}</p>
          {refused && (
            <p role="alert" className="err">
              {refused}
            </p>
          )}
          <div className="list">{live.map(row)}</div>
          {projects.length === 0 && <div className="empty">A project gathers what belongs to one piece of work. AI clients see it and can file into it by name.</div>}
          {archived.length > 0 && (
            <>
              <h2 className="ghd">Archived</h2>
              <div className="list">{archived.map(row)}</div>
            </>
          )}
          {modules.length === 0 && <p className="note projects-none">No module is switched on, so the areas hold nothing yet.</p>}
        </>
      )}
      {making && (
        <Dialog title="New project" onClose={close}>
          <AreaForm dialog onSaved={made} onCancel={close} />
        </Dialog>
      )}
    </div>
  )
}
