import { useEffect, useRef, useState } from 'react'
import type { Change } from './types.ts'

type Subscriber = { change: (c: Change) => void; reconnect: () => void }

// The tab's one stream is the shell's; pages hear it through useStream.
const subscribers = new Set<Subscriber>()

const readGap = 2000
const firstDelay = 1000
const maxDelay = 30000

export function useEvents(): { live: boolean } {
  const [live, setLive] = useState(false)

  useEffect(() => {
    let source: EventSource | null = null
    let timer: ReturnType<typeof setTimeout> | undefined
    let delay = firstDelay
    let opened = false

    const open = () => {
      const s = new EventSource('/api/events')
      source = s
      s.onopen = () => {
        setLive(true)
        delay = firstDelay
        // The server keeps no change, so what was missed while closed is read afresh.
        if (opened) subscribers.forEach((p) => p.reconnect())
        opened = true
      }
      s.onmessage = (m: MessageEvent<string>) => {
        const c = JSON.parse(m.data) as Change
        subscribers.forEach((p) => p.change(c))
      }
      s.onerror = () => {
        setLive(false)
        // The browser retries a source it keeps; one it closed, as after a 401 or a 502, is opened again here.
        if (s.readyState !== EventSource.CLOSED) return
        s.close()
        timer = setTimeout(open, delay)
        delay = Math.min(delay * 2, maxDelay)
      }
    }
    open()

    return () => {
      clearTimeout(timer)
      source?.close()
    }
  }, [])

  return { live }
}

// read gets null to read everything, as when the page mounts and when the stream opens again, or the changes of a burst.
export function useStream(read: (changes: Change[] | null) => void): void {
  // A new read between renders is used without resubscribing.
  const current = useRef(read)
  useEffect(() => {
    current.current = read
  })

  useEffect(() => {
    let lastRead = 0
    let timer: ReturnType<typeof setTimeout> | undefined
    let pending: Change[] = []
    const run = (changes: Change[] | null) => {
      lastRead = Date.now()
      current.current(changes)
    }
    const sub: Subscriber = {
      change: (c) => {
        pending.push(c)
        if (timer !== undefined) return
        timer = setTimeout(() => {
          timer = undefined
          const changes = pending
          pending = []
          run(changes)
        }, Math.max(0, lastRead + readGap - Date.now()))
      },
      reconnect: () => {
        clearTimeout(timer)
        timer = undefined
        pending = []
        run(null)
      },
    }
    subscribers.add(sub)
    run(null)
    return () => {
      subscribers.delete(sub)
      clearTimeout(timer)
    }
  }, [])
}
