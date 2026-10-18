import { Fragment, useLayoutEffect, useRef, useState } from 'react'
import { ChevronDown, Plus, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { CheckboxField } from '@/components/blocks'
import { FieldError } from '@/components/common'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { formatMinutesRange, hhmmToMinutes } from '@/lib/datetime'
import { cn } from '@/lib/utils'
import { dayProblem, dayProblems, fromDraft, nextPeriod, type DraftDay, type DraftPeriod } from './draft'

/** A 0–24 h bar with the working periods filled in; decorative. */
export function DayTimeline({ value }: { value: DraftDay }) {
  const ps = value.off ? [] : (fromDraft(value) ?? []).filter((p) => p.start < p.end)
  return (
    <div aria-hidden className="bg-muted relative h-3 overflow-hidden rounded-full">
      {/* Faint marks at 6, 12 and 18 o'clock. */}
      {[25, 50, 75].map((x) => (
        <span key={x} className="bg-border absolute inset-y-0 w-px" style={{ left: `${x}%` }} />
      ))}
      {ps.map((p, i) => (
        <span
          key={i}
          className="bg-primary absolute inset-y-0 rounded-full"
          style={{ left: `${(p.start / 1440) * 100}%`, width: `${((p.end - p.start) / 1440) * 100}%` }}
        />
      ))}
    </div>
  )
}

/** A draft period in the interface language; raw text while it is invalid. */
function periodText(p: DraftPeriod) {
  const start = hhmmToMinutes(p.start)
  const end = hhmmToMinutes(p.end)
  return Number.isNaN(start) || Number.isNaN(end) ? `${p.start} – ${p.end}` : formatMinutesRange(start, end)
}

/**
 * The day's periods as text, e.g. "9:00 – 12:00, 13:00 – 17:00", or "Day off".
 * Keep a period together when it fits; allow wrapping with enlarged text.
 */
function DaySummary({ value }: { value: DraftDay }) {
  const { t } = useTranslation()
  if (value.off) return <p className="text-muted-foreground">{t('manage.schedule.dayOff')}</p>
  return (
    <p>
      {value.periods.map((p, i) => (
        <Fragment key={i}>
          {i > 0 && ', '}
          <span className="inline-block max-w-full">{periodText(p)}</span>
        </Fragment>
      ))}
    </p>
  )
}

// On phones the picker icon is hidden so the whole time (with AM/PM where
// the device uses it) fits; tapping the field still opens the picker.
const timeInputCls = 'px-2 max-sm:[&::-webkit-calendar-picker-indicator]:hidden'

/**
 * Editor for one day's plan: day off, or a list of working periods with one
 * responsive group per period. `name` names the day for screen readers.
 */
export function DayEditor({
  value,
  onChange,
  idPrefix,
  name,
}: {
  value: DraftDay
  onChange: (d: DraftDay) => void
  idPrefix: string
  name: string
}) {
  const { t } = useTranslation()
  const { problem, bad } = dayProblems(value)
  const setPeriod = (i: number, p: Partial<DraftPeriod>) =>
    onChange({ ...value, periods: value.periods.map((x, j) => (j === i ? { ...x, ...p } : x)) })
  return (
    <div className="space-y-3">
      <CheckboxField
        className="w-fit font-semibold"
        checked={value.off}
        onCheckedChange={(off) => onChange({ ...value, off })}
      >
        {t('manage.schedule.dayOff')}
      </CheckboxField>
      {!value.off && (
        <div className="@container/periods space-y-3">
          <div className="space-y-4">
            {value.periods.map((p, i) => (
              <div key={i} className="grid min-w-0 grid-cols-1 gap-3 @min-[20rem]/periods:grid-cols-2">
                <div className="flex min-h-10 items-center justify-between gap-2 @min-[20rem]/periods:col-span-2">
                  <span className="min-w-0 font-semibold">{t('manage.schedule.periodN', { n: i + 1 })}</span>
                  {value.periods.length > 1 && (
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      className="w-[min(var(--control-small),3rem)] [&_svg]:max-w-full"
                      aria-label={t('manage.schedule.removePeriodN', { day: name, n: i + 1 })}
                      onClick={() => onChange({ ...value, periods: value.periods.filter((_, j) => j !== i) })}
                    >
                      <Trash2 aria-hidden />
                    </Button>
                  )}
                </div>
                <label className="min-w-0 space-y-2">
                  <span className="block text-muted-foreground">{t('manage.schedule.from')}</span>
                  <Input
                    id={`${idPrefix}-s-${i}`}
                    type="time"
                    className={timeInputCls}
                    aria-label={t('manage.schedule.periodStart', { day: name, n: i + 1 })}
                    aria-invalid={bad.has(i) || undefined}
                    aria-describedby={bad.has(i) ? `${idPrefix}-error` : undefined}
                    value={p.start}
                    onChange={(e) => setPeriod(i, { start: e.target.value })}
                  />
                </label>
                <label className="min-w-0 space-y-2">
                  <span className="block text-muted-foreground">{t('manage.schedule.to')}</span>
                  <Input
                    id={`${idPrefix}-e-${i}`}
                    type="time"
                    className={timeInputCls}
                    aria-label={t('manage.schedule.periodEnd', { day: name, n: i + 1 })}
                    aria-invalid={bad.has(i) || undefined}
                    aria-describedby={bad.has(i) ? `${idPrefix}-error` : undefined}
                    value={p.end}
                    onChange={(e) => setPeriod(i, { end: e.target.value })}
                  />
                </label>
              </div>
            ))}
          </div>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => onChange({ ...value, periods: [...value.periods, nextPeriod(value.periods)] })}
          >
            <Plus aria-hidden />
            {t('manage.schedule.addPeriod')}
          </Button>
        </div>
      )}
      {problem && <FieldError id={`${idPrefix}-error`}>{t(`manage.schedule.problem.${problem}`)}</FieldError>}
    </div>
  )
}

// Inline padding of each day row; the hour labels above use the same, so
// they line up with every row's 0–24 h bar.
const ROW_PAD = 'px-[min(0.75rem,3vw)]'

/** Hour labels above the list, lined up with every row's 0–24 h bar. */
export function TimeAxis() {
  return (
    <div aria-hidden className={cn('text-muted-foreground @container/axis mt-2', ROW_PAD)}>
      <div className="relative h-6 text-sm">
        {[0, 6, 12, 18, 24].map((h) => (
          <span
            key={h}
            className={cn("absolute top-0 whitespace-nowrap", (h === 6 || h === 18) && "hidden @min-[20rem]/axis:block")}
            style={{
              left: `${(h / 24) * 100}%`,
              transform: h === 0 ? 'none' : h === 24 ? 'translateX(-100%)' : 'translateX(-50%)',
            }}
          >
            {`${h}:00`}
          </span>
        ))}
      </div>
    </div>
  )
}

/** One cycle day. Expanding only changes visibility; the parent owns its draft. */
export function DayRow({
  index,
  name,
  next,
  value,
  names,
  expanded,
  onToggle,
  onChange,
  onCopy,
}: {
  index: number
  name: string
  next: string | null
  value: DraftDay
  names: string[]
  expanded: boolean
  onToggle: () => void
  onChange: (day: DraftDay) => void
  onCopy: (targets: number[]) => void
}) {
  const { t } = useTranslation()
  const trigger = useRef<HTMLButtonElement>(null)
  const problem = dayProblem(value)
  const prefix = `day${index}`
  useLayoutEffect(() => {
    if (!expanded || !trigger.current) return
    // Collapsing the preceding day can move this heading above the viewport.
    // Keep it visible without focusing an input or opening the phone keyboard.
    const rect = trigger.current.getBoundingClientRect()
    if (rect.top < 0 || rect.bottom > window.innerHeight) {
      trigger.current.scrollIntoView({ block: 'nearest', behavior: 'instant' })
    }
  }, [expanded])
  return (
    <li className={expanded ? 'bg-accent/30' : undefined}>
      <button
        ref={trigger}
        id={`${prefix}-trigger`}
        type="button"
        aria-expanded={expanded}
        aria-controls={`${prefix}-panel`}
        className={cn(
          'hover:bg-accent/60 block w-full space-y-1.5 py-3 text-start focus-visible:-outline-offset-2',
          ROW_PAD,
        )}
        onClick={onToggle}
      >
        <span className="flex items-start gap-3">
          <span className="flex min-w-0 flex-1 flex-wrap items-baseline justify-between gap-x-3">
            <span id={`${prefix}-name`} className="font-semibold">{name}</span>
            {next && <span className="text-muted-foreground text-sm">{next}</span>}
          </span>
          <ChevronDown aria-hidden className={cn('mt-0.5 size-5 shrink-0 transition-transform motion-reduce:transition-none', expanded && 'rotate-180')} />
        </span>
        <DayTimeline value={value} />
        {problem ? (
          <FieldError inline>{t(`manage.schedule.problem.${problem}`)}</FieldError>
        ) : (
          <DaySummary value={value} />
        )}
      </button>
      <div id={`${prefix}-panel`} role="region" aria-labelledby={`${prefix}-name`} hidden={!expanded}>
        {expanded && <div className={cn('space-y-4 border-t py-3', ROW_PAD)}>
          <DayEditor
            idPrefix={prefix}
            name={name}
            value={value}
            onChange={onChange}
          />
          {names.length > 1 && (
            <CopyDay key={names.length} index={index} names={names} disabled={problem !== null} onCopy={onCopy} />
          )}
        </div>}
      </div>
    </li>
  )
}

/** Selecting targets never changes their drafts until the copy action is pressed. */
function CopyDay({ index, names, disabled, onCopy }: {
  index: number
  names: string[]
  disabled: boolean
  onCopy: (targets: number[]) => void
}) {
  const { t } = useTranslation()
  const [targets, setTargets] = useState<number[]>([])
  const [copied, setCopied] = useState(false)
  return (
    <details className="@container/copy border-t pt-2">
      <summary className="min-h-11 cursor-pointer content-center py-2 font-semibold">
        {t('manage.schedule.applyToOthers')}
      </summary>
      <div className="space-y-3 pt-2">
        <p className="text-muted-foreground">{t('manage.schedule.copyHint')}</p>
        <div className="grid grid-cols-1 gap-x-4 gap-y-1 @min-[24rem]/copy:grid-cols-2">
          {names.map((name, i) => i === index ? null : (
            <CheckboxField
              key={i}
              checked={targets.includes(i)}
              onCheckedChange={(checked) => {
                setTargets(checked ? [...targets, i] : targets.filter((target) => target !== i))
                setCopied(false)
              }}
            >
              {name}
            </CheckboxField>
          ))}
        </div>
        <Button
          type="button"
          variant="secondary"
          disabled={disabled || targets.length === 0}
          onClick={() => {
            onCopy(targets)
            setTargets([])
            setCopied(true)
          }}
        >
          {t('manage.schedule.applySelected')}
        </Button>
        <p role="status" className="text-muted-foreground empty:hidden">{copied && t('manage.schedule.copyDone')}</p>
      </div>
    </details>
  )
}
