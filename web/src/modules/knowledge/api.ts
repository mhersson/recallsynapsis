import { useEffect, useState } from 'react'
import { ApiError, api, message, useGet } from '../../api/client.ts'
import { useStream } from '../../api/stream.ts'
import type { Linked } from '../../api/types.ts'
import type { Entry } from '../../components/history.ts'

// A decision as a line, as another decision names it in replaces or replaced_by.
export type Line = { ref: string; decision: string; decided: string }

export type State = 'stands' | 'replaced'

// A decision as GET /api/knowledge lists it: area is the slug, decided is YYYY-MM-DD, it is replaced while another
// decision names it in replaces, and changed_by is web or the name of a token. source is where it was said, or ''.
export type Decision = {
  id: number
  ref: string
  area: string
  decision: string
  why: string
  source: string
  decided: string
  state: State
  replaces: Line | null
  replaced_by: Line | null
  revision: number
  created_at: string
  changed_at: string
  changed_by: string
}

// A decision as GET /api/knowledge/{id} gives it, with the items that link to it as the core resolves them.
export type Full = Decision & { linked_from: Linked[] }

// Every decision of a project, the newest day first, and the server's today.
export type List = { decisions: Decision[]; today: string }

// What a decision's form holds: replaces is a reference, or '' for none.
export type Fields = Pick<Decision, 'decision' | 'why' | 'source' | 'decided'> & { replaces: string }

// A wording of a decision in full, with the reference it replaced then, or ''.
export type Wording = Entry & Fields

// The fields of a decision, as its form opens with them.
export const fieldsOf = (d: Decision): Fields => ({ decision: d.decision, why: d.why, source: d.source, decided: d.decided, replaces: d.replaces?.ref ?? '' })

// The page of the decision a reference names.
export const pathOf = (ref: string) => `/decisions/${ref.slice('knowledge:'.length)}`

// The class of a decision's mark after mk, a diamond: green while it stands and grey once replaced.
export const mark = (state: string) => 'm-decision' + (state === 'replaced' ? ' d-replaced' : '')

// The decisions of the project slug names, '' for none, read when asked, when the stream opens again and on every change
// of a decision.
export function useDecisions(area: string) {
  const [tick, setTick] = useState(0)
  useStream((changes) => {
    if (!changes || changes.some((c) => c.module === 'knowledge')) setTick((n) => n + 1)
  })
  return useGet<List>(area && tick ? '/knowledge?' + new URLSearchParams({ area }) : '', tick)
}

// The decision id names, in full, read on every change the stream tells of, since what links to it may be any module's
// item. One deleted meanwhile is undefined, with the server's line.
export function useDecision(id: number) {
  const [decision, setDecision] = useState<Full>()
  const [failed, setFailed] = useState('')
  const [tick, setTick] = useState(0)
  useStream(() => setTick((n) => n + 1))
  useEffect(() => {
    if (!tick) return
    let stale = false
    api<Full>('GET', `/knowledge/${id}`).then(
      (d) => {
        if (stale) return
        setDecision(d)
        setFailed('')
      },
      (e: unknown) => {
        if (stale) return
        if (e instanceof ApiError && e.status === 404) setDecision(undefined)
        setFailed(message(e))
      },
    )
    return () => {
      stale = true
    }
  }, [id, tick])
  // A decision as a change answers it, which keeps what links to it.
  const show = (d: Decision) => setDecision((x) => x && { ...x, ...d })
  return { decision, failed, show }
}
