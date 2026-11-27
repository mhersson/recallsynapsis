import { useNavigate, useSearchParams } from 'react-router'
import { useAreas } from '../../api/areas.ts'
import PageHead from '../../components/PageHead.tsx'
import { useClose } from '../../components/useClose.ts'
import { useAreaShown } from '../../session.ts'
import { pathOf, useDecisions } from './api.ts'
import { Crumb, Failed } from './Decision.tsx'
import DecisionForm from './Form.tsx'

// The form of a new decision of the project ?area= names, on the day the server gives as today; from Replace, ?replaces=
// names the decision it replaces, while that one stands. Saved, the new decision opens in the form's place.
export default function New() {
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const slug = params.get('area') ?? ''
  const project = useAreas(true).find((a) => a.slug === slug)
  useAreaShown(project?.id)
  const { got, failed } = useDecisions(slug)
  const replacing = got?.decisions.find((d) => d.id === Number(params.get('replaces')) && d.state === 'stands')
  const close = useClose(replacing ? pathOf(replacing.ref) : project ? `/areas/${project.id}?tab=decisions` : '/projects')
  if (!got) return <Failed line={failed} />

  return (
    <div className="page narrow">
      {project && <Crumb project={project} />}
      <PageHead title={replacing ? 'Replace a decision' : 'Add a decision'} />
      <p className="sub">{replacing ? 'What is saved here stands, and the decision under Replaces is kept as replaced.' : 'One thing that was decided, and why.'}</p>
      <DecisionForm
        area={slug}
        start={{ decision: '', why: '', source: '', decided: got.today, replaces: replacing?.ref ?? '' }}
        decisions={got.decisions}
        onSaved={(d) => void navigate(pathOf(d.ref), { replace: true })}
        onCancel={close}
      />
    </div>
  )
}
