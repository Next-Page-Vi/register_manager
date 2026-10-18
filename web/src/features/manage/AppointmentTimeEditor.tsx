import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Schemas } from '@/api/client'
import { Field, FieldError } from '@/components/common'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { instantToWall, wallToInstant } from '@/lib/datetime'
import { SlotPicker } from './SlotPicker'

type Slot = Schemas['Slot']
// Prefilled instants retain their offset and precision until that field is edited.
// Keeping them distinct from wall times avoids losing the later DST occurrence.
type TimeValue = { kind: 'instant'; value: string } | { kind: 'wall'; value: string }
export type AppointmentTimeDraft = { start: TimeValue | null; end: TimeValue | null }
export const emptyTime = (): AppointmentTimeDraft => ({ start: null, end: null })

export function timeDraft(slot: Slot): AppointmentTimeDraft {
  return { start: { kind: 'instant', value: slot.starts_at }, end: { kind: 'instant', value: slot.ends_at } }
}

function wallValue(time: TimeValue | null, tz: string): string | null {
  if (time === null) return null
  return time.kind === 'instant' ? instantToWall(time.value, tz) : time.value
}

function editTime(time: TimeValue | null, wall: string, tz: string): TimeValue | null {
  if (wall === '') return null
  return wall === wallValue(time, tz) ? time : { kind: 'wall', value: wall }
}

function resolveTime(time: TimeValue, tz: string): string | null {
  if (time.kind === 'instant') return time.value
  const instant = wallToInstant(time.value, tz)
  return instantToWall(instant, tz) === time.value ? instant : null
}

/** Only complete, real local times with a positive elapsed duration can submit. */
export function appointmentTime(draft: AppointmentTimeDraft, tz: string): Slot | null {
  if (draft.start === null || draft.end === null) return null
  try {
    const start = resolveTime(draft.start, tz)
    const end = resolveTime(draft.end, tz)
    if (start === null || end === null || new Date(start) >= new Date(end)) return null
    return { starts_at: start, ends_at: end }
  } catch {
    return null
  }
}

export function AppointmentTimeEditor({ instanceId, tz, context, value, onChange, error }: {
  instanceId: string
  tz: string
  context: { kind: 'new'; customerId: string } | { kind: 'reschedule'; appointmentId: string }
  value: AppointmentTimeDraft
  onChange: (value: AppointmentTimeDraft) => void
  error?: string
}) {
  const { t } = useTranslation()
  const id = useId()
  const [recommendations, setRecommendations] = useState(false)
  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground" id={`${id}-hint`}>{t('manage.appointments.timeHint', { tz })}</p>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label={t('manage.appointments.startsAt')} htmlFor={`${id}-start`}>
          <Input
            id={`${id}-start`} type="datetime-local" step={60} required
            aria-invalid={error ? true : undefined} aria-describedby={error ? `${id}-hint ${id}-error` : `${id}-hint`} value={wallValue(value.start, tz) ?? ''}
            onChange={(event) => onChange({ ...value, start: editTime(value.start, event.target.value, tz) })}
          />
        </Field>
        <Field label={t('manage.appointments.endsAt')} htmlFor={`${id}-end`}>
          <Input
            id={`${id}-end`} type="datetime-local" step={60} required
            aria-invalid={error ? true : undefined} aria-describedby={error ? `${id}-hint ${id}-error` : `${id}-hint`} value={wallValue(value.end, tz) ?? ''}
            onChange={(event) => onChange({ ...value, end: editTime(value.end, event.target.value, tz) })}
          />
        </Field>
      </div>
      {error && <FieldError id={`${id}-error`}>{error}</FieldError>}
      <Button type="button" variant="outline" aria-expanded={recommendations} aria-controls={`${id}-recommendations`} onClick={() => setRecommendations(!recommendations)}>
        {t('manage.appointments.recommendations')}
      </Button>
      <div id={`${id}-recommendations`} hidden={!recommendations}>
        {recommendations && (
          <div className="space-y-3">
            <p className="text-sm text-muted-foreground">{t('manage.appointments.recommendationsHint')}</p>
            <SlotPicker instanceId={instanceId} tz={tz} context={context} value={appointmentTime(value, tz)} onChange={(slot) => {
              if (slot !== null) onChange(timeDraft(slot))
            }} />
          </div>
        )}
      </div>
    </div>
  )
}
