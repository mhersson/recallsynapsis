import { useRef, useState, type FormEvent } from 'react'
import { api, message } from '../../api/client.ts'
import { useStream } from '../../api/stream.ts'
import type { Token } from '../../api/types.ts'
import Dialog from '../../components/Dialog.tsx'
import { ago, day } from '../../components/time.ts'
import { useNarrow } from '../../components/useNarrow.ts'
import Choice from './Choice.tsx'

type Form = { name: string; access: Token['access'] }

const accesses: [Token['access'], string][] = [
  ['read', 'Read'],
  ['write', 'Write'],
]

export type Made = Token & { token: string }

// made holds a token's text from the answer that made it until Done; it is never stored.
export default function Tokens({ made, setMade }: { made: Made | null; setMade: (m: Made | null) => void }) {
  const narrow = useNarrow()
  const [tokens, setTokens] = useState<Token[] | null>(null)
  const [form, setForm] = useState<Form | null>(null)
  const [copied, setCopied] = useState(false)
  const [revoking, setRevoking] = useState<Token | null>(null)
  const [refused, setRefused] = useState('')
  const [sending, setSending] = useState(false)
  const secret = useRef<HTMLElement>(null)

  // No change to a token is streamed, so the list is read when the page opens and when the stream opens again.
  const read = () => api<{ tokens: Token[] }>('GET', '/tokens').then((r) => setTokens(r.tokens), (e: unknown) => setRefused(message(e)))
  useStream((changes) => {
    if (!changes) void read()
  })

  async function create(e: FormEvent, f: Form) {
    e.preventDefault()
    setRefused('')
    setSending(true)
    try {
      setMade(await api<Made>('POST', '/tokens', f))
      setCopied(false)
      setForm(null)
      void read()
    } catch (err) {
      setRefused(message(err))
    }
    setSending(false)
  }

  async function revoke(t: Token) {
    setRevoking(null)
    setRefused('')
    try {
      await api('DELETE', `/tokens/${t.id}`)
      void read()
    } catch (e) {
      setRefused(message(e))
    }
  }

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
    } catch {
      // Without a secure context there is no clipboard; the text is selected to copy by hand.
      if (secret.current) getSelection()?.selectAllChildren(secret.current)
    }
  }

  const used = (t: Token) => (t.last_used_at ? ago(t.last_used_at) : 'never')
  const revokeButton = (t: Token) => (
    <button className="btn sm" onClick={() => setRevoking(t)}>
      Revoke
    </button>
  )
  let list
  if (!tokens) list = null
  else if (tokens.length === 0) list = <div className="empty tok-empty">No tokens yet. Make one for each client, such as Portal or a coding agent.</div>
  else if (narrow)
    list = (
      <div className="trows">
        {tokens.map((t) => (
          <div key={t.id} className={t.id === made?.id ? 'trow new' : 'trow'}>
            <div className="bd">
              <span className="mono">{t.name}</span>
              <span className="t2">
                {t.access} · created {day(t.created_at)} · {t.last_used_at ? 'used ' + ago(t.last_used_at) : 'never used'}
              </span>
            </div>
            {revokeButton(t)}
          </div>
        ))}
      </div>
    )
  else
    list = (
      <table className="tok">
        <thead>
          <tr>
            <th>Name</th>
            <th>Access</th>
            <th>Created</th>
            <th>Last used</th>
            <th />
          </tr>
        </thead>
        <tbody>
          {tokens.map((t) => (
            <tr key={t.id} className={t.id === made?.id ? 'new' : undefined}>
              <td className="mono">{t.name}</td>
              <td>{t.access}</td>
              <td>{day(t.created_at)}</td>
              <td>{used(t)}</td>
              <td>{revokeButton(t)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    )

  let tail
  if (form)
    tail = (
      <form className="tok-form" onSubmit={(e) => void create(e, form)}>
        <div className="frow">
          <label htmlFor="token-name">Name</label>
          <input
            id="token-name"
            className="in"
            autoFocus
            autoComplete="off"
            spellCheck={false}
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
          />
          <p className="note">Name it after the client. Every change it makes carries this name.</p>
        </div>
        <Choice
          label="Access"
          options={accesses}
          value={form.access}
          onChange={(access) => setForm({ ...form, access })}
          note="Read can search and read. Write can also capture and change."
        />
        <div className="acts stack">
          <button className="btn primary" disabled={sending || !form.name.trim()}>
            Create
          </button>
          <button type="button" className="btn" onClick={() => setForm(null)}>
            Cancel
          </button>
        </div>
      </form>
    )
  else if (made)
    tail = (
      <div className="once">
        <p className="oh">
          <i className="mk wait" />
          <span>
            The token <span className="mono">{made.name}</span> is made, with {made.access} access.
          </span>
        </p>
        <div className="secret">
          <code ref={secret}>{made.token}</code>
          <button className="btn primary" onClick={() => void copy(made.token)}>
            {copied ? 'Copied' : 'Copy'}
          </button>
        </div>
        <p className="note">Copy it now. Recall Synapsis keeps only a hash of it, so it cannot be shown again.</p>
        <button className="btn" onClick={() => setMade(null)}>
          Done
        </button>
      </div>
    )
  else
    tail = (
      <button className="btn" onClick={() => setForm({ name: '', access: 'read' })}>
        New token
      </button>
    )

  return (
    <>
      <p className="note">A token lets an AI client or a script reach Recall Synapsis, through Portal or directly.</p>
      {list}
      {refused && (
        <p role="alert" className="err">
          {refused}
        </p>
      )}
      {tail}
      {revoking && (
        <Dialog
          title={
            <>
              Revoke the token <span className="mono">{revoking.name}</span>?
            </>
          }
          onClose={() => setRevoking(null)}
        >
          <p className="note">A client that uses it is refused from its next request. This cannot be undone.</p>
          <div className="acts">
            <button className="btn" onClick={() => setRevoking(null)}>
              Keep it
            </button>
            <button className="btn danger" onClick={() => void revoke(revoking)}>
              Revoke
            </button>
          </div>
        </Dialog>
      )}
    </>
  )
}
