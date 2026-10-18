import { useUnsavedChanges } from '@/components/UnsavedChanges'
import { useLayoutEffect, useRef, useState } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { $api, type Schemas } from '@/api/client'
import { SaveBar } from '@/components/blocks'
import { QueryContent, Field, FieldError, Notice } from '@/components/common'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { formatCivilShort, todayIn, weekdayOfDate } from '@/lib/datetime'
import { useInstance } from '../ManageLayout'
import { PreviewView } from '../Preview'
import { DayRow, TimeAxis } from './DayEditor'
import { copyDay, dayProblem, fromDraft, mondayOf, nextDateFor, toDraft, type DraftDay, type Period } from './draft'

type CycleState = Schemas['CycleState']

/** The repeating schedule, edited once it has loaded. */
export function CycleEditor() {
  const { id, path } = useInstance()
  const loaded = $api.useQuery('get', '/api/v1/instances/{instance_id}/schedule', { params: { path } })
  return <QueryContent query={loaded}>{(c) => <CycleForm key={id} initial={c} />}</QueryContent>
}

function CycleForm({ initial }: { initial: CycleState }) {
  const { t } = useTranslation()
  const { tz, path } = useInstance()
  const defaults = (value: CycleState) => ({
    length: String(value.cycle_length_days),
    anchor: value.anchor_date,
    days: value.days.map(toDraft),
  })
  const form = useForm({ defaultValues: defaults(initial) })
  const [anchor, days] = useWatch({ control: form.control, name: ['anchor', 'days'] })
  const [expandedDay, setExpandedDay] = useState<number | null>(null)
  const dayList = useRef<HTMLUListElement>(null)
  const focusProblem = useRef<number | null>(null)
  const { submitCount } = form.formState
  useLayoutEffect(() => {
    if (focusProblem.current === null) return
    const inputs = dayList.current?.querySelectorAll<HTMLInputElement>(`#day${focusProblem.current}-panel input[aria-invalid=true]`)
    const input = inputs && (Array.from(inputs).find((input) => !input.value) ?? inputs[0])
    if (input) {
      input.focus()
      focusProblem.current = null
    }
  }, [expandedDay, submitCount])
  const n = days.length
  const preview = $api.useMutation('post', '/api/v1/instances/{instance_id}/schedule/preview')
  const save = $api.useMutation('put', '/api/v1/instances/{instance_id}/schedule', {
    onSuccess: (result) => {
      form.reset(defaults(result))
      preview.reset()
    },
  })
  const maxDays = save.data?.max_cycle_days ?? initial.max_cycle_days
  const lengthError = t('errors.cycle_length_invalid', { max: maxDays })
  const overLimit = n > maxDays
  useUnsavedChanges(form.formState.isDirty, save.isPending)
  const pending = save.isPending || preview.isPending
  form.register('days', {
    validate: (value) => value.every((day) => dayProblem(day) === null) || t('errors.invalid_time'),
  })
  const updateDays = (updated: DraftDay[]) => {
    form.setValue('days', updated, { shouldDirty: true, shouldValidate: true })
    preview.reset()
  }
  const resize = (value: string) => {
    form.setValue('length', value, { shouldDirty: true })
    const len = value.trim() === '' ? null : Number(value)
    if (len === null || !Number.isInteger(len) || len < 1 || len > 366) return
    if (expandedDay !== null && expandedDay >= len) setExpandedDay(null)
    updateDays(
      len <= days.length
        ? days.slice(0, len)
        : [...days, ...Array.from({ length: len - days.length }, () => toDraft([]))],
    )
  }
  const run = (kind: 'preview' | 'save') =>
    form.handleSubmit((value) => {
      const body = {
        cycle_length_days: Number(value.length),
        anchor_date: value.anchor,
        days: value.days.map(fromDraft) as Period[][],
      }
      if (kind === 'preview') {
        save.reset()
        preview.mutate({ params: { path }, body })
      } else {
        preview.reset()
        save.mutate({ params: { path }, body })
      }
    }, (errors) => {
      // Let RHF focus the cycle settings first when they also need attention.
      if (errors.length || errors.anchor || !errors.days) return
      const index = form.getValues('days').findIndex((day) => dayProblem(day) !== null)
      if (index < 0) return
      focusProblem.current = index
      setExpandedDay(index)
    })()

  const today = todayIn(tz)
  const weekly = n === 7
  const nextDates = days.map((_, i) => (anchor ? nextDateFor(i, anchor, n, today) : ''))
  const names = days.map((_, i) =>
    weekly && anchor ? weekdayOfDate(nextDates[i]) : t('manage.schedule.dayN', { n: i + 1 }),
  )
  const dirty = form.formState.isDirty
  const error = save.error ?? preview.error

  return (
    <Card>
      <CardHeader className="px-[min(1rem,4vw)] sm:px-5">
        <CardTitle>{t('manage.schedule.cycleTitle')}</CardTitle>
      </CardHeader>
      <CardContent className="@container/form space-y-4 px-[min(1rem,4vw)] sm:px-5">
        <SaveBar
          sticky
          dirty={form.formState.isDirty}
          error={error}
          saved={save.isSuccess && !dirty}
          pending={pending}
          disabled={overLimit}
          onSave={() => void run('save')}
          extra={
            <Button type="button" variant="outline" onClick={() => void run('preview')} disabled={pending || overLimit}>
              {t('manage.preview.button')}
            </Button>
          }
        />
        <p>{t('manage.schedule.cycleIntro')}</p>
        <fieldset disabled={pending} className="min-w-0 space-y-4">
          <div className="control-row max-w-[40rem]">
            <Field
              label={t('manage.schedule.lengthMax', { max: maxDays })}
              htmlFor="cycle-n"
              error={form.formState.errors.length?.message}
            >
              <Input
                id="cycle-n"
                type="number"
                min={1}
                max={maxDays}
                className="w-full max-w-28"
                {...form.register('length', {
                  required: lengthError,
                  min: { value: 1, message: lengthError },
                  max: { value: maxDays, message: lengthError },
                  validate: (value) => Number.isInteger(Number(value)) || lengthError,
                  onChange: (e) => resize(e.target.value),
                })}
              />
            </Field>
            <Field
              label={t('manage.schedule.anchor')}
              htmlFor="cycle-anchor"
              error={form.formState.errors.anchor?.message}
            >
              <Input
                id="cycle-anchor"
                type="date"
                className="w-full max-w-[14rem]"
                {...form.register('anchor', { required: t('errors.invalid_date') })}
              />
            </Field>
            <Button
              type="button"
              variant="secondary"
              onClick={() => {
                resize('7')
                form.setValue('anchor', mondayOf(today), { shouldDirty: true })
              }}
            >
              {t('manage.schedule.weeklyPreset')}
            </Button>
          </div>
          {overLimit && (
            <Notice kind="warning">
              <p>{t('manage.schedule.overLimit', { n, max: maxDays })}</p>
            </Notice>
          )}
          <p className="text-muted-foreground">{t('manage.schedule.editHint')}</p>
          <div className="rounded-lg border">
            <TimeAxis />
            <ul ref={dayList} className="divide-y border-t">
              {days.map((d, i) => (
                <DayRow
                  key={i}
                  index={i}
                  name={names[i]}
                  next={anchor ? t('manage.schedule.nextDate', { date: formatCivilShort(nextDates[i], !weekly) }) : null}
                  value={d}
                  names={names}
                  expanded={expandedDay === i}
                  onToggle={() => setExpandedDay(expandedDay === i ? null : i)}
                  onChange={(day) => updateDays(days.map((x, j) => j === i ? day : x))}
                  onCopy={(targets) => updateDays(days.map((x, j) => targets.includes(j) ? copyDay(d) : x))}
                />
              ))}
            </ul>
          </div>
        </fieldset>
        <p className="text-muted-foreground">{t('manage.schedule.midnightHint')}</p>
        {form.formState.errors.days && <FieldError>{form.formState.errors.days.message}</FieldError>}
        {preview.data && <PreviewView preview={preview.data} tz={tz} />}
      </CardContent>
    </Card>
  )
}
