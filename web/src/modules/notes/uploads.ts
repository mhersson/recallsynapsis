import { useEffect, useRef, useState } from 'react'
import { api, message } from '../../api/client.ts'

// A size as a person reads it.
export function size(n: number) {
  if (n < 1024) return `${n} B`
  if (n < 1 << 20) return `${Math.round(n / 1024)} KiB`
  return `${+(n / (1 << 20)).toFixed(1)} MiB`
}

type Upload = { key: number; name: string; size: number }

// refused is the line of the last upload or removal refused, which refuse sets.
export type Uploads = { cap: number; going: Upload[]; refused: string; refuse: (line: string) => void; send: (files: File[]) => void }

// Attaches files to the note with the id, each sent as the body with its type and under way until done after it ends. A
// file over the server's cap is refused before it is sent, and an upload still under way ends when the note is left.
export function useUploads(id: number, done: () => Promise<unknown>): Uploads {
  const [cap, setCap] = useState(0)
  const [going, setGoing] = useState<Upload[]>([])
  const [refused, setRefused] = useState('')
  const ends = useRef<AbortController>(null)
  const count = useRef(0)
  // Made in the effect, so the second run of a development build has its own.
  useEffect(() => {
    const c = new AbortController()
    ends.current = c
    api<{ file_max_bytes: number }>('GET', '/notes/limits', undefined, c.signal).then((l) => setCap(l.file_max_bytes), () => {})
    return () => c.abort()
  }, [])

  const send = (files: File[]) => {
    const { signal } = ends.current!
    setRefused('')
    for (const f of files) {
      if (cap && f.size > cap) {
        setRefused(`${f.name} is ${size(f.size)}. An attachment can be at most ${size(cap)}, so it was not sent.`)
        continue
      }
      const u = { key: ++count.current, name: f.name, size: f.size }
      setGoing((g) => [...g, u])
      api('POST', `/notes/${id}/files?${new URLSearchParams({ name: f.name })}`, f, signal)
        .then(done, (e: unknown) => !signal.aborted && setRefused(message(e)))
        .finally(() => setGoing((g) => g.filter((x) => x !== u)))
    }
  }
  return { cap, going, refused, refuse: setRefused, send }
}
