import { useState } from 'react'
import { Link } from 'react-router'
import { api } from '../../api/client.ts'
import { useStream } from '../../api/stream.ts'
import type { Session } from '../../api/types.ts'
import PageHead from '../../components/PageHead.tsx'
import { ago, since, when } from '../../components/time.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import { useSignOut } from '../../components/useSignOut.ts'
import { useSession } from '../../session.ts'
import { useLook, type Look } from '../../theme/theme.ts'
import Choice from './Choice.tsx'
import Tokens, { type Made } from './Tokens.tsx'
import './Settings.css'

const palettes: [Look['palette'], string][] = [
  ['copper', 'Copper'],
  ['catppuccin', 'Catppuccin'],
]

const themes: [Look['theme'], string][] = [
  ['system', 'As the system'],
  ['dark', 'Dark'],
  ['light', 'Light'],
]

// A backup is made every night, so one older than a day and a half means at least one night failed.
const late = 36 * 60 * 60 * 1000

function Backup({ at }: { at: string | null }) {
  if (!at)
    return (
      <p>
        There is no backup yet. With <code>backup.dir</code> set in <code>recallsynapsis.toml</code>, the server makes one every night.
      </p>
    )
  const made = `The last backup was made ${when(at)}, ${ago(at)}.`
  if (since(at) <= late) return <p>{made}</p>
  return (
    <p className="late">
      <i className="mk wait" />
      <span>{made} None has succeeded since, and the server's log says why.</span>
    </p>
  )
}

export default function Settings() {
  const narrow = useNarrow()
  const session = useSession()
  const [look, setLook] = useLook()
  const signOut = useSignOut()
  // Held above the two layouts, so a phone turned across the width of a desktop keeps the one showing of a token.
  const [made, setMade] = useState<Made | null>(null)
  // The shell reads the session only as the stream opens, so a tab left open would show an old last backup.
  const [backup, setBackup] = useState(session.last_backup)
  useStream((changes) => {
    if (!changes) api<Session>('GET', '/session').then((s) => setBackup(s.last_backup), () => {})
  })

  const parts = [
    { id: 'tokens', title: 'Tokens', body: <Tokens made={made} setMade={setMade} /> },
    { id: 'backups', title: 'Backups', body: <Backup at={backup} /> },
    {
      id: 'browser',
      title: 'This browser',
      body: (
        <>
          <Choice id="look-palette" label="Palette" options={palettes} value={look.palette} onChange={(palette) => setLook({ ...look, palette })} />
          <Choice id="look-theme" label="Theme" options={themes} value={look.theme} onChange={(theme) => setLook({ ...look, theme })} />
          <p className="note">Applied at once. It holds for this browser only.</p>
        </>
      ),
    },
    {
      id: 'session',
      title: 'Session',
      body: (
        <div className="acts stack signout">
          <p>Signed in as {session.user.name}. Signing out ends the session in this browser only.</p>
          <button className="btn" onClick={() => void signOut()}>
            Sign out
          </button>
        </div>
      ),
    },
  ]
  const version = <p className="ver">Recall Synapsis {session.version}</p>

  if (narrow)
    return (
      <div className="page set-page">
        <div className="crumb">
          <Link to="/more">More</Link> /
        </div>
        <PageHead title="Settings" />
        <div className="set">
          {parts.map((p, i) => (
            <details key={p.id} className="acc" open={i === 0}>
              <summary>{p.title}</summary>
              <section>{p.body}</section>
            </details>
          ))}
        </div>
        {version}
      </div>
    )

  return (
    <div className="page set-page">
      <div className="col">
        <PageHead title="Settings" />
        <div className="set-grid">
          <nav className="set-nav" aria-label="Sections">
            {parts.map((p) => (
              <a key={p.id} href={'#' + p.id}>
                {p.title}
              </a>
            ))}
          </nav>
          <div>
            <div className="set">
              {parts.map((p) => (
                <section key={p.id} id={p.id}>
                  <h2>{p.title}</h2>
                  {p.body}
                </section>
              ))}
            </div>
            {version}
          </div>
        </div>
      </div>
    </div>
  )
}
