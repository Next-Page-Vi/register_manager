// Working periods as edited in the schedule forms, and the checks run on
// them as the user types. Pure functions, no React.

import type { Schemas } from '@/api/client'
import { addDays, hhmmToMinutes, minutesToHHMM } from '@/lib/datetime'

export type Period = Schemas['Period']

/** A period being edited as "HH:MM" strings. End "00:00" means midnight. */
export type DraftPeriod = { start: string; end: string }
export type DraftDay = { off: boolean; periods: DraftPeriod[] }

export const toDraft = (ps: Period[]): DraftDay =>
  ps.length === 0
    ? { off: true, periods: [{ start: '09:00', end: '17:00' }] }
    : { off: false, periods: ps.map((p) => ({ start: minutesToHHMM(p.start), end: minutesToHHMM(p.end % 1440) })) }

/** A draft period in minutes; an end of 00:00 is midnight (1440). NaN marks an invalid time. */
function parsePeriod(p: DraftPeriod): Period {
  const end = hhmmToMinutes(p.end)
  return { start: hhmmToMinutes(p.start), end: end === 0 ? 1440 : end }
}

/** Converts a draft day; returns null when a time is invalid. */
export function fromDraft(d: DraftDay): Period[] | null {
  if (d.off) return []
  const out = d.periods.map(parsePeriod)
  return out.some((p) => Number.isNaN(p.start) || Number.isNaN(p.end)) ? null : out
}

export type Problem = 'format' | 'order' | 'overlap'

/**
 * What is wrong with a day's periods and which periods are affected,
 * checked as the user types so mistakes show next to the day instead of
 * only after saving.
 */
export function dayProblems(d: DraftDay): { problem: Problem | null; bad: Set<number> } {
  const bad = new Set<number>()
  if (d.off) return { problem: null, bad }
  const mins = d.periods.map(parsePeriod)
  mins.forEach((m, i) => (Number.isNaN(m.start) || Number.isNaN(m.end)) && bad.add(i))
  if (bad.size) return { problem: 'format', bad }
  mins.forEach((m, i) => m.start >= m.end && bad.add(i))
  if (bad.size) return { problem: 'order', bad }
  mins.forEach((a, i) => mins.forEach((b, j) => i !== j && a.start < b.end && b.start < a.end && bad.add(i)))
  return { problem: bad.size ? 'overlap' : null, bad }
}

export const dayProblem = (d: DraftDay) => dayProblems(d).problem

/**
 * The next period starts where the last one ends and lasts an hour (up to
 * midnight); with no room left after the last one it starts at 09:00.
 */
export function nextPeriod(periods: DraftPeriod[]): DraftPeriod {
  const end = periods.length ? hhmmToMinutes(periods[periods.length - 1].end) : NaN
  if (Number.isNaN(end) || end === 0 || end >= 1440) return { start: '09:00', end: '10:00' }
  return { start: minutesToHHMM(end), end: minutesToHHMM(Math.min(end + 60, 1440) % 1440) }
}

export const copyDay = (d: DraftDay): DraftDay => ({ off: d.off, periods: d.periods.map((p) => ({ ...p })) })

/** Monday of the week containing a civil date. */
export function mondayOf(date: string) {
  const wd = new Date(`${date}T12:00:00Z`).getUTCDay()
  return addDays(date, -((wd + 6) % 7))
}

/** Next date on or after `from` that maps to cycle day `index`. */
export function nextDateFor(index: number, anchor: string, n: number, from: string) {
  const diff = Math.round((Date.parse(`${from}T00:00:00Z`) - Date.parse(`${anchor}T00:00:00Z`)) / 86_400_000)
  const cur = ((diff % n) + n) % n
  return addDays(from, (index - cur + n) % n)
}
