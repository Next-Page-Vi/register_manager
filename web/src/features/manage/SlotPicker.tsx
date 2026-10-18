import { useState } from 'react'
import { AvailabilityDateFilter, type AvailabilitySelection } from '@/components/AvailabilityDateFilter'
import { Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { $api, type Schemas } from '@/api/client'
import { LoadMore } from '@/components/blocks'
import { QueryContent, Notice } from '@/components/common'
import { Button } from '@/components/ui/button'
import { cursorOptions, resetAvailabilityPages } from '@/api/queries'
import { formatShortDate, formatTime, groupByDay } from '@/lib/datetime'
import { cn } from '@/lib/utils'

type Slot = Schemas['Slot']

/**
 * Lists bookable slots computed by the server for a customer (quotas
 * apply) or for rescheduling an appointment (which is excluded).
 */
export function SlotPicker({
  instanceId,
  tz,
  context,
  value,
  onChange,
}: {
  instanceId: string
  tz: string
  context: { kind: 'new'; customerId: string } | { kind: 'reschedule'; appointmentId: string }
  value: Slot | null
  onChange: (s: Slot | null) => void
}) {
  const { t } = useTranslation()
  const [selection, setSelection] = useState<AvailabilitySelection>({ kind: 'nearest' })
  const date = selection.kind === 'date' ? selection.date : undefined
  const slots = $api.useInfiniteQuery(
    'get',
    '/api/v1/instances/{instance_id}/availability',
    {
      params: {
        path: { instance_id: instanceId },
        query: { ...(context.kind === 'new' ? { customer_id: context.customerId } : { exclude_appointment_id: context.appointmentId }), date, limit: 40 },
      },
    },
    cursorOptions,
  )
  const items = slots.data?.pages.flatMap((page) => page.items) ?? []

  return (
    <div className="space-y-3">
      <AvailabilityDateFilter value={selection} range={slots.data?.pages[0].date_range} onChange={(next) => {
        resetAvailabilityPages(instanceId)
        setSelection(next)
        onChange(null)
      }} />
      <QueryContent query={slots}>
        {() =>
          items.length === 0 ? (
            <Notice kind="info"><p>{t('manage.appointments.noRecommendations')}</p></Notice>
          ) : (
            <div className="max-h-80 space-y-3 overflow-y-auto pe-1">
              {groupByDay(items, tz).map((g) => (
                <fieldset key={g.day}>
                  <legend className="mb-2 font-semibold">{formatShortDate(g.items[0].starts_at, tz)}</legend>
                  <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,7.5rem),1fr))] gap-2" data-slot-grid>
                    {g.items.map((s) => {
                      const selected = value !== null && new Date(value.starts_at).getTime() === new Date(s.starts_at).getTime() && new Date(value.ends_at).getTime() === new Date(s.ends_at).getTime()
                      return (
                        <Button
                          key={s.starts_at}
                          type="button"
                          size="sm"
                          variant="outline"
                          selected={selected}
                          className="grid h-auto min-h-(--control-small) grid-cols-[1.25rem_minmax(0,1fr)_1.25rem] gap-1 px-2 py-2 tabular-nums"
                          onClick={() => onChange(s)}
                        >
                          <Check aria-hidden className={cn('size-5', !selected && 'invisible')} />
                          <span>{formatTime(s.starts_at, tz)}</span>
                          <span aria-hidden />
                        </Button>
                      )
                    })}
                  </div>
                </fieldset>
              ))}
            </div>
          )
        }
      </QueryContent>
      {slots.data && <LoadMore query={slots} label={t(date === undefined ? 'customer.times.more' : 'availability.moreOnDate')} />}
      {slots.isError && <Button type="button" variant="outline" onClick={() => void slots.refetch()} pending={slots.isFetching}>{t('common.retry')}</Button>}
    </div>
  )
}
