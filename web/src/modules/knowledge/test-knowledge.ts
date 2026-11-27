import type { Area, Linked, Session } from '../../api/types.ts'
import type { Decision, Full, Line } from './api.ts'

export const session = (modules = ['notes', 'tasks', 'knowledge']): Session => ({ user: { name: 'ada' }, modules, version: '1.2.3', timezone: 'UTC', last_backup: null })

const area = (id: number, kind: Area['kind'], name: string): Area => ({
  id,
  kind,
  name,
  slug: name,
  brief: 'what it is',
  brief_revision: 1,
  brief_changed_at: '2026-11-20T10:00:00Z',
  brief_changed_by: 'web',
  archived: false,
})

export const areas: Area[] = [area(1, 'personal', 'Personal'), area(2, 'project', 'garden'), area(3, 'project', 'shed')]

// A decision of the garden that stands, decided on the day given, unless d says otherwise.
export const decision = (id: number, decided: string, d: Partial<Full> = {}): Full => ({
  id,
  ref: `knowledge:${id}`,
  area: 'garden',
  decision: `decision ${id}`,
  why: `why ${id}`,
  source: '',
  decided,
  state: 'stands',
  replaces: null,
  replaced_by: null,
  revision: 1,
  created_at: `${decided}T10:00:00Z`,
  changed_at: `${decided}T10:00:00Z`,
  changed_by: 'web',
  linked_from: [],
  ...d,
})

export const line = (d: Decision): Line => ({ ref: d.ref, decision: d.decision, decided: d.decided })

// A decision as a list gives it.
export const row = ({ linked_from: _, ...d }: Full): Decision => d

// Makes newer replace older in decisions, as the server tells both.
export function replace(newer: Full, older: Full) {
  newer.replaces = line(older)
  older.replaced_by = line(newer)
  older.state = 'replaced'
}

export const linked = (ref: string, kind: string, state: string): Linked => ({ ref, kind, state, title: `${kind} ${ref}`, available: true })
