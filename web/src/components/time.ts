// How many milliseconds ago iso was.
export const since = (iso: string) => Date.now() - Date.parse(iso)

export function ago(iso: string): string {
  const m = Math.round(since(iso) / 60000)
  if (m < 60) return `${m} min ago`
  const h = Math.floor(m / 60)
  if (h < 48) return `${h} h${m % 60 ? ` ${m % 60} min` : ''} ago`
  return `${Math.floor(h / 24)} days ago`
}

const hm = (d: Date) => d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })
const today = (d: Date) => d.toDateString() === new Date().toDateString()

// The local time as hours and minutes, on a 24 hour clock in every locale, after the day when it is not today.
export function clock(iso: string): string {
  const d = new Date(iso)
  if (today(d)) return hm(d)
  return d.toLocaleDateString([], { weekday: 'short', day: 'numeric', month: 'short' }) + ' ' + hm(d)
}

// The day of the month and the month, as a list of things made shows when each was made.
export const day = (iso: string) => new Date(iso).toLocaleDateString([], { day: 'numeric', month: 'short' })

// When something happened, as a sentence says it: today at a time, or on a day at a time.
export function when(iso: string): string {
  const d = new Date(iso)
  return `${today(d) ? 'today' : 'on ' + day(iso)} at ${hm(d)}`
}
