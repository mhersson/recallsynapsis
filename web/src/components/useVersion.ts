import { useEffect, useState } from 'react'
import { api } from '../api/client.ts'

export function useVersion(): string {
  const [version, setVersion] = useState('')
  useEffect(() => {
    api<{ version: string }>('GET', '/health')
      .then((h) => setVersion(h.version))
      .catch(() => {})
  }, [])
  return version
}
