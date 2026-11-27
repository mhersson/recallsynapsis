// One change on the event stream: which item of which module changed, and in which area.
export type Change = { module: string; kind: string; id: number; area: number }

// GET /api/session: who is signed in and what the server has switched on. last_backup is null until there is one.
export type Session = { user: { name: string }; modules: string[]; version: string; timezone: string; last_backup: string | null }

// An area as GET /api/areas lists it: the personal area or a project, with its brief. brief_revision is 0, brief_changed_at
// null and brief_changed_by '' while no brief was written.
export type Area = {
  id: number
  kind: 'personal' | 'project'
  name: string
  slug: string
  brief: string
  brief_revision: number
  brief_changed_at: string | null
  brief_changed_by: string
  archived: boolean
}

// A hit of GET /api/search: a module's item, the slug of the area it lives in, as the MCP tool search gives it to clients
// that know areas by slug, and its state, such as an unsorted capture's.
export type Hit = { ref: string; kind: string; title: string; snippet: string; area: string; state: string; changed_at: string }

// What a link names, as the core resolves it: an item of a module switched on, unavailable once it is gone.
export type Linked = { ref: string; kind: string; state: string; title: string; available: boolean }

// The hits of one module, ranked.
export type Group = { module: string; hits: Hit[] }

// An API token as GET /api/tokens lists it. last_used_at is null until it is used.
export type Token = { id: number; name: string; access: 'read' | 'write'; created_at: string; last_used_at: string | null }
