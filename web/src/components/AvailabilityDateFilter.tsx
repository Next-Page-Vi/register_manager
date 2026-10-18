import { useId, useState } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { Schemas } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { addDays } from '@/lib/datetime'
import { ActionGroup } from '@/components/ActionGroup'

export type AvailabilitySelection = { kind: 'nearest' } | { kind: 'date'; date: string }

/** Dates are civil dates in the instance zone; bounds come from the server clock. */
export function AvailabilityDateFilter({ value, range, onChange }: {
  value: AvailabilitySelection
  range: Schemas['DateRange'] | undefined
  onChange: (value: AvailabilitySelection) => void
}) {
  const { t } = useTranslation()
  const id = useId()
  const [knownRange, setKnownRange] = useState(range)
  if (range && (range.from !== knownRange?.from || range.to !== knownRange?.to)) setKnownRange(range)
  const bounds = range ?? knownRange
  const valid = value.kind === 'date' && /^\d{4}-\d{2}-\d{2}$/.test(value.date) && !Number.isNaN(Date.parse(`${value.date}T00:00:00Z`))
  return (
    <div className="@container/date-filter space-y-3" data-date-filter>
      <ActionGroup role="group" aria-label={t('availability.browse')}>
        <Button type="button" size="sm" variant="outline" selected={value.kind === 'nearest'} onClick={() => { if (value.kind !== 'nearest') onChange({ kind: 'nearest' }) }}>
          {t('availability.nearest')}
        </Button>
        <Button type="button" size="sm" variant="outline" selected={value.kind === 'date'} disabled={!bounds} onClick={() => { if (bounds && value.kind !== 'date') onChange({ kind: 'date', date: bounds.from }) }}>
          {t('availability.byDate')}
        </Button>
      </ActionGroup>
      {value.kind === 'date' && (
        <div className="space-y-2">
          <label htmlFor={id} className="block font-semibold">{t('availability.date')}</label>
          <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] items-center gap-2 @min-[20rem]/date-filter:grid-cols-[minmax(0,1fr)_auto]">
            <Input id={id} type="date" className="px-[min(0.75rem,3vw)]" value={valid ? value.date : ''} min={bounds?.from} max={bounds?.to} onChange={(event) => {
              if (event.target.value && event.target.validity.valid) onChange({ kind: 'date', date: event.target.value })
            }} />
            <div className="flex items-center gap-2" data-date-navigation>
              <Button type="button" size="icon" variant="outline" aria-label={t('availability.previous')} disabled={!valid || !bounds || value.date <= bounds.from} onClick={() => onChange({ kind: 'date', date: addDays(value.date, -1) })}><ChevronLeft aria-hidden /></Button>
              <Button type="button" size="icon" variant="outline" aria-label={t('availability.next')} disabled={!valid || !bounds || value.date >= bounds.to} onClick={() => onChange({ kind: 'date', date: addDays(value.date, 1) })}><ChevronRight aria-hidden /></Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
