import type { Now } from './api.ts'

// Dates are the wire's YYYY-MM-DD and are worked on at midnight UTC, where no time zone moves one onto the day before.
const at = (date: string) => new Date(date + 'T00:00:00Z')

const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
const NAMES = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday']

// The date days after date, or before it when days is negative.
export function add(date: string, days: number) {
  const d = at(date)
  d.setUTCDate(d.getUTCDate() + days)
  return d.toISOString().slice(0, 10)
}

// A day as a row writes it: Sat 28 Nov.
export function day(date: string) {
  const d = at(date)
  return `${DAYS[d.getUTCDay()]} ${d.getUTCDate()} ${MONTHS[d.getUTCMonth()]}`
}

// A period: Sat 28 Nov for one day, Sat-Sun 28-29 Nov within a month and Mon-Sun 30 Nov-6 Dec across two.
export function period(from: string, to: string) {
  if (!to || to === from) return day(from)
  const a = at(from)
  const b = at(to)
  const days = `${DAYS[a.getUTCDay()]}-${DAYS[b.getUTCDay()]}`
  if (a.getUTCMonth() === b.getUTCMonth() && a.getUTCFullYear() === b.getUTCFullYear()) return `${days} ${a.getUTCDate()}-${b.getUTCDate()} ${MONTHS[a.getUTCMonth()]}`
  return `${days} ${a.getUTCDate()} ${MONTHS[a.getUTCMonth()]}-${b.getUTCDate()} ${MONTHS[b.getUTCMonth()]}`
}

// The first and the last day of the week today is in.
export function week(now: Now): [string, string] {
  const start = add(now.date, -((at(now.date).getUTCDay() - NAMES.indexOf(now.week_start) + 7) % 7))
  return [start, add(start, 6)]
}

// The shortcuts for when, each with the first and the last day it plans. Weekend is the Saturday and the Sunday next to
// come, today among them, so on a Sunday it is that day alone; Next week the seven days from the next first day of a week.
export function shortcuts(now: Now): [string, string, string][] {
  const today = now.date
  const weekday = at(today).getUTCDay()
  const saturday = add(today, 6 - weekday)
  const next = add(week(now)[0], 7)
  return [
    ['Today', today, today],
    ['Tomorrow', add(today, 1), add(today, 1)],
    ['Weekend', weekday === 0 ? today : saturday, weekday === 0 ? today : add(saturday, 1)],
    ['Next week', next, add(next, 6)],
  ]
}

// An instant on the server's clock, as its date and its time.
export function clock(iso: string, now: Now): [string, string] {
  const parts = new Intl.DateTimeFormat('en-CA', { timeZone: now.timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).formatToParts(new Date(iso))
  const p = Object.fromEntries(parts.map((x) => [x.type, x.value]))
  return [`${p.year}-${p.month}-${p.day}`, `${p.hour}:${p.minute}`]
}

// An instant on the server's clock: today and the time, or the day and the time.
export function stamp(iso: string, now: Now) {
  const [date, time] = clock(iso, now)
  return `${date === now.date ? 'today' : day(date)} ${time}`
}
