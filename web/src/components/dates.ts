// Dates are the wire's YYYY-MM-DD and are worked on at midnight UTC, where no time zone moves one onto the day before.
export const at = (date: string) => new Date(date + 'T00:00:00Z')

export const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
export const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

// A day as a row writes it: Sat 28 Nov.
export function day(date: string) {
  const d = at(date)
  return `${DAYS[d.getUTCDay()]} ${d.getUTCDate()} ${MONTHS[d.getUTCMonth()]}`
}
