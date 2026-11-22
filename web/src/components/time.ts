export function ago(iso: string): string {
  const m = Math.round((Date.now() - Date.parse(iso)) / 60000)
  if (m < 60) return `${m} min ago`
  const h = Math.floor(m / 60)
  if (h < 48) return `${h} h${m % 60 ? ` ${m % 60} min` : ''} ago`
  return `${Math.floor(h / 24)} days ago`
}

// The local time as hours and minutes, on a 24 hour clock in every locale, after the day when it is not today.
export function clock(iso: string): string {
  const d = new Date(iso)
  const time = d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })
  if (d.toDateString() === new Date().toDateString()) return time
  return d.toLocaleDateString([], { weekday: 'short', day: 'numeric', month: 'short' }) + ' ' + time
}
