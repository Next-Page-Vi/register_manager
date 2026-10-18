// Date and time helpers. Business times are always shown in the instance's
// time zone, never the device's, formatted for the current interface
// language with Intl. Times of day always use the 24-hour clock, in every
// language (hourCycle h23), so a time reads the same for everyone.

import { currentLocale } from '@/i18n'

// Building an Intl.DateTimeFormat is expensive and a list renders hundreds
// of times, so formatters are kept per locale, zone and options.
const formatters = new Map<string, Intl.DateTimeFormat>()

function formatter(locale: string, opts: Intl.DateTimeFormatOptions) {
  const key = `${locale}|${JSON.stringify(opts)}`
  let f = formatters.get(key)
  if (!f) {
    f = new Intl.DateTimeFormat(locale, opts)
    formatters.set(key, f)
  }
  return f
}

function fmt(tz: string, opts: Intl.DateTimeFormatOptions, locale: string = currentLocale()) {
  return formatter(locale, { timeZone: tz, ...opts })
}

/** e.g. "Tuesday, June 2, 2026" / "2026年6月2日火曜日". */
export function formatDate(iso: string | Date, tz: string) {
  return fmt(tz, { year: 'numeric', month: 'long', day: 'numeric', weekday: 'long' }).format(new Date(iso))
}

/** e.g. "Tue, Jun 2". */
export function formatShortDate(iso: string | Date, tz: string) {
  return fmt(tz, { month: 'short', day: 'numeric', weekday: 'short' }).format(new Date(iso))
}

const wallKey = (d: Date, tz: string) =>
  fmt(tz, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit' }, 'en-GB').format(d)

/**
 * Whether the wall-clock time of this instant occurs twice on that day
 * (a daylight-saving fall-back). Such times are shown with their offset.
 */
export function isAmbiguous(iso: string | Date, tz: string) {
  const d = new Date(iso)
  const k = wallKey(d, tz)
  return [30, 60, 120].some((m) => {
    const delta = m * 60_000
    return wallKey(new Date(d.getTime() + delta), tz) === k || wallKey(new Date(d.getTime() - delta), tz) === k
  })
}

/**
 * A 24-hour time from Intl parts, with the hour never zero-padded: Intl
 * pads it in some languages ("09:05" in English) but not others ("9:05" in
 * Japanese), and every language should read the same. hour replaces the
 * hour, for "24:00".
 */
function h23(parts: Intl.DateTimeFormatPart[], hour?: string) {
  return parts.map((p) => (p.type === 'hour' ? (hour ?? String(Number(p.value))) : p.value)).join('')
}

/** e.g. "9:00" / "16:30", with the UTC offset when ambiguous. */
export function formatTime(iso: string | Date, tz: string) {
  const ambiguous = isAmbiguous(iso, tz)
  return h23(
    fmt(tz, {
      hour: 'numeric',
      minute: '2-digit',
      hourCycle: 'h23',
      ...(ambiguous ? { timeZoneName: 'shortOffset' } : {}),
    }).formatToParts(new Date(iso)),
  )
}

export function formatTimeRange(start: string, end: string, tz: string) {
  const endText = localDateKey(start, tz) === localDateKey(end, tz) ? formatTime(end, tz) : `${formatDate(end, tz)} ${formatTime(end, tz)}`
  return `${formatTime(start, tz)} – ${endText}`
}

export function formatDateTime(iso: string | Date, tz: string) {
  return `${formatShortDate(iso, tz)} ${formatTime(iso, tz)}`
}

/**
 * A span of instants, e.g. "Tue, Nov 10 9:00 – 12:00"; the end's date is
 * left out when it is the same day as the start.
 */
export function formatDateTimeRange(start: string, end: string, tz: string) {
  const endText = localDateKey(start, tz) === localDateKey(end, tz) ? formatTime(end, tz) : formatDateTime(end, tz)
  return `${formatDateTime(start, tz)} – ${endText}`
}

/**
 * Minutes since midnight as a time of day, styled like formatTime (e.g.
 * "9:00"). 1440, the end of a period that runs to midnight, shows as
 * "24:00" rather than "0:00".
 */
export function formatMinutes(m: number) {
  const parts = fmt('UTC', { hour: 'numeric', minute: '2-digit', hourCycle: 'h23' }).formatToParts(
    new Date(Date.UTC(2000, 0, 1, 0, m % 1440)),
  )
  return h23(parts, m >= 1440 ? '24' : undefined)
}

/** A range of minutes since midnight, styled like formatTimeRange. */
export function formatMinutesRange(start: number, end: number) {
  return `${formatMinutes(start)} – ${formatMinutes(end)}`
}

/** Instance-local calendar date "YYYY-MM-DD" of an instant. */
export function localDateKey(iso: string | Date, tz: string) {
  return fmt(tz, { year: 'numeric', month: '2-digit', day: '2-digit' }, 'en-CA').format(new Date(iso))
}

/** Groups times (in order) by their calendar day in the zone, keeping order. */
export function groupByDay<T extends { starts_at: string }>(items: T[], tz: string) {
  const groups: { day: string; items: T[] }[] = []
  for (const s of items) {
    const day = localDateKey(s.starts_at, tz)
    const g = groups[groups.length - 1]
    if (g && g.day === day) g.items.push(s)
    else groups.push({ day, items: [s] })
  }
  return groups
}

/** Weekday name of a civil date "YYYY-MM-DD" (time zone independent). */
export function weekdayOfDate(date: string, style: 'long' | 'short' = 'long') {
  return fmt('UTC', { weekday: style }).format(new Date(`${date}T12:00:00Z`))
}

/** Formats a civil date "YYYY-MM-DD" with its weekday. */
export function formatCivilDate(date: string) {
  return fmt('UTC', { year: 'numeric', month: 'long', day: 'numeric', weekday: 'long' }).format(new Date(`${date}T12:00:00Z`))
}

/** A civil date without the year, e.g. "Oct 19" / "10月19日", optionally with the weekday. */
export function formatCivilShort(date: string, weekday = false) {
  return fmt('UTC', { month: 'short', day: 'numeric', ...(weekday ? { weekday: 'short' as const } : {}) }).format(
    new Date(`${date}T12:00:00Z`),
  )
}

/** Adds days to a civil date. */
export function addDays(date: string, n: number) {
  const d = new Date(`${date}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + n)
  return d.toISOString().slice(0, 10)
}

/** Today's civil date in tz. */
export function todayIn(tz: string) {
  return localDateKey(new Date(), tz)
}

/** Minutes since midnight to "HH:MM" (1440 → "24:00"). */
export function minutesToHHMM(m: number) {
  return `${String(Math.floor(m / 60)).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`
}

/** "HH:MM" to minutes since midnight; "24:00" → 1440. Returns NaN if invalid. */
export function hhmmToMinutes(s: string) {
  const m = /^(\d{1,2}):(\d{2})$/.exec(s.trim())
  if (!m) return NaN
  const h = Number(m[1])
  const min = Number(m[2])
  if (min > 59 || h > 24 || (h === 24 && min !== 0)) return NaN
  return h * 60 + min
}

function offsetMs(utcMs: number, tz: string) {
  const parts = fmt(tz, {
    hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  }, 'en-US').formatToParts(new Date(utcMs))
  const get = (t: string) => Number(parts.find((p) => p.type === t)?.value)
  const asUTC = Date.UTC(get('year'), get('month') - 1, get('day'), get('hour'), get('minute'), get('second'))
  return asUTC - utcMs
}

/**
 * Converts an instance-local wall time "YYYY-MM-DDTHH:MM" (as produced by
 * <input type="datetime-local">) to an ISO instant. Repeated wall times
 * resolve to the earlier instant, like the server.
 */
export function wallToInstant(wall: string, tz: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(wall)
  if (!m) throw new Error('invalid wall time')
  const guess = Date.UTC(+m[1], +m[2] - 1, +m[3], +m[4], +m[5])
  const candidates = new Set<number>()
  for (const probe of [guess - 86_400_000, guess, guess + 86_400_000]) {
    const t = guess - offsetMs(probe, tz)
    if (t + offsetMs(t, tz) === guess) candidates.add(t)
  }
  const sorted = [...candidates].sort((a, b) => a - b)
  const t = sorted[0] ?? guess - offsetMs(guess, tz)
  return new Date(t).toISOString()
}

/** Inverse of wallToInstant, for prefilling datetime-local inputs. */
export function instantToWall(iso: string, tz: string) {
  const d = new Date(iso)
  const p = fmt(tz, {
    hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
  }, 'en-CA').formatToParts(d)
  const g = (t: string) => p.find((x) => x.type === t)?.value ?? '00'
  return `${g('year')}-${g('month')}-${g('day')}T${g('hour')}:${g('minute')}`
}

/**
 * Readable name of a time zone in the current language, e.g. "Central
 * European Time" / "中欧时间"; falls back to the IANA name.
 */
export function formatZoneName(tz: string, at: Date = new Date()) {
  for (const timeZoneName of ['longGeneric', 'long'] as const) {
    try {
      const part = fmt(tz, { timeZoneName }).formatToParts(at).find((p) => p.type === 'timeZoneName')
      if (part) return part.value
    } catch {
      // Older engines reject 'longGeneric'; try the next style.
    }
  }
  return tz
}

/** Whether this device's clock currently uses the same UTC offset as tz. */
export function deviceMatchesZone(tz: string, at: Date = new Date()) {
  const device = -at.getTimezoneOffset() * 60_000
  return Math.round(offsetMs(at.getTime(), tz) / 60_000) === Math.round(device / 60_000)
}
