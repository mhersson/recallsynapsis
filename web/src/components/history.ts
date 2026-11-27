// A revision as a history lists it: by is where the change came from, web or the name of a token, and restored_from
// the revision a restore brought back.
export type Entry = { revision: number; at: string; by: string; said: string; restored_from: number | null }

// Where a change came from, as a list shows it.
export const from = (by: string) => (by === 'web' ? 'web' : `via ${by}`)
