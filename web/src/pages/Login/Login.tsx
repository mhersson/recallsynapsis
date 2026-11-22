import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { useLocation, useNavigate } from 'react-router'
import { ApiError, api } from '../../api/client.ts'
import Field from '../../components/Field.tsx'
import { useVersion } from '../../components/useVersion.ts'
import './Login.css'

export default function Login() {
  const navigate = useNavigate()
  const from = (useLocation().state as { from?: string } | null)?.from ?? '/'
  const version = useVersion()
  const [name, setName] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [wait, setWait] = useState(0)
  const passwordField = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (wait === 0) return
    const tick = setTimeout(() => setWait(wait - 1), 1000)
    return () => clearTimeout(tick)
  }, [wait])

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError('')
    try {
      await api('POST', '/auth/login', { name, password })
      navigate(from, { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.status === 429 && err.retryAfter > 0) {
        setWait(err.retryAfter)
      } else if (err instanceof ApiError && err.status === 401) {
        setPassword('')
        passwordField.current?.focus()
        setError('Wrong name or password.')
      } else {
        setError(err instanceof Error ? err.message : String(err))
      }
    }
  }

  const alert = wait > 0 ? `Too many failed sign-ins. Try again in ${wait} ${wait === 1 ? 'second' : 'seconds'}.` : error

  return (
    <>
      <div className="brand-bar">
        <span className="brand">Recall Synapsis</span>
        <span>{version}</span>
      </div>
      <form className="login" onSubmit={(e) => void submit(e)}>
        <h1>Sign in</h1>
        <Field label="Name">
          <input name="name" autoComplete="username" autoFocus value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Password">
          <input
            ref={passwordField}
            name="password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </Field>
        {alert && (
          <p role="alert" className="login-error">
            {alert}
          </p>
        )}
        <button className="btn primary login-submit" disabled={wait > 0}>
          Sign in
        </button>
        <p className="sub">
          Recall Synapsis has one owner. The name is <code>owner</code> in <code>recallsynapsis.toml</code>, and the password is set
          with <code>recallsynapsis password</code>.
        </p>
      </form>
    </>
  )
}
