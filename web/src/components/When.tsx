import { CalendarCheck, Clock } from 'lucide-react'
import { formatDate, formatTimeRange, localDateKey } from '@/lib/datetime'
import { cn } from '@/lib/utils'

/** An appointment's date and time range on one line, for lists and descriptions. */
export function whenText(start: string, end: string, tz: string) {
  return `${formatDate(start, tz)} ${formatTimeRange(start, end, tz)}`
}

// Date line and time line per size. The time is the larger line: it is what
// people look for first.
const SIZES = {
  sm: { date: 'text-muted-foreground', time: 'text-lg font-semibold' },
  md: { date: 'text-lg', time: 'text-2xl font-bold' },
  lg: { date: 'text-xl', time: 'text-3xl font-bold' },
} as const

/**
 * An appointment's date above its time range, in three sizes: sm for back
 * office lists, md for cards and buttons, lg for the booking confirmation.
 * icons adds a calendar and a clock, for customer pages.
 */
export function When({ start, end, tz, size = 'md', showDate = true, icons, className }: {
  start: string
  end: string
  tz: string
  size?: keyof typeof SIZES
  showDate?: boolean
  icons?: boolean
  className?: string
}) {
  const s = SIZES[size]
  return (
    <span className={cn('flex flex-col', className)}>
      {(showDate || localDateKey(start, tz) !== localDateKey(end, tz)) && (
        <span className={cn('flex items-start gap-2', s.date)}>
          {icons && <CalendarCheck aria-hidden className="mt-1 size-5 shrink-0" />}
          {formatDate(start, tz)}
        </span>
      )}
      <span className={cn('flex items-start gap-2 [overflow-wrap:normal]', s.time)}>
        {icons && <Clock aria-hidden className="mt-1 size-5 shrink-0" />}
        <span className="min-w-0">{formatTimeRange(start, end, tz)}</span>
      </span>
    </span>
  )
}
