import { useNavigate } from 'react-router'
import { api } from '../api/client.ts'
import { useModules } from '../session.ts'

export function useSignOut(): () => Promise<void> {
  const navigate = useNavigate()
  const on = useModules()
  return async () => {
    // What a module does first, such as turning notifications off in this browser, never keeps the session open.
    await Promise.all(on.map((m) => m.signOut?.().catch(() => {})))
    // The session is gone or cannot be reached: sign-in is the next step either way.
    await api('POST', '/auth/logout').catch(() => {})
    navigate('/login')
  }
}
