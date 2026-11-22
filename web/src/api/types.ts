// One change on the event stream: which item of which module changed, and in which area.
export type Change = { module: string; kind: string; id: number; area: number }

// GET /api/session: who is signed in and what the server has switched on. last_backup is null until there is one.
export type Session = { user: { name: string }; modules: string[]; version: string; timezone: string; last_backup: string | null }
