import type { ComponentProps, ReactNode } from 'react'
import type { UseQueryResult } from '@tanstack/react-query'
import { Controller, useForm, useWatch } from 'react-hook-form'
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { $api } from '@/api/client'
import { ConfirmButton } from '@/components/ConfirmButton'
import { Row, RowList } from '@/components/blocks'
import { QueryContent, EmptyState, Field, FieldError } from '@/components/common'
import { FormDialog } from '@/components/FormDialog'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  formatCivilDate,
  formatDateTimeRange,
  formatMinutesRange,
  instantToWall,
  todayIn,
  wallToInstant,
} from '@/lib/datetime'
import { useInstance } from '../ManageLayout'
import { DayEditor } from './DayEditor'
import { dayProblem, fromDraft, type DraftDay } from './draft'

function ListEditorCard<T>({
  title,
  intro,
  add,
  query,
  empty,
  row,
}: {
  title: ReactNode
  intro: ReactNode
  add: ReactNode
  query: UseQueryResult<T[], unknown>
  empty: ReactNode
  row: (item: T) => ReactNode
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p>{intro}</p>
        {add}
        <QueryContent query={query}>
          {(items) => (items.length === 0 ? <EmptyState>{empty}</EmptyState> : <RowList borderless>{items.map(row)}</RowList>)}
        </QueryContent>
      </CardContent>
    </Card>
  )
}

function DeleteEntryButton(props: Omit<ComponentProps<typeof ConfirmButton>, 'children' | 'confirmLabel'>) {
  const { t } = useTranslation()
  return (
    <ConfirmButton size="sm" variant="destructive-outline" {...props} confirmLabel={t('common.delete')}>
      {t('common.delete')}
    </ConfirmButton>
  )
}

const newOverride = (tz: string) => ({
  date: todayIn(tz),
  draft: { off: true, periods: [{ start: '09:00', end: '12:00' }] } as DraftDay,
})

export function OverridesEditor() {
  const { t } = useTranslation()
  const { id, tz, path } = useInstance()
  const query = $api.useQuery(
    'get',
    '/api/v1/instances/{instance_id}/overrides',
    { params: { path, query: { from: todayIn(tz) } } },
    { select: (value) => value.overrides },
  )
  const form = useForm({ defaultValues: newOverride(tz) })
  const date = useWatch({ control: form.control, name: 'date' })
  const save = $api.useMutation('put', '/api/v1/instances/{instance_id}/overrides/{date}')
  const del = $api.useMutation('delete', '/api/v1/instances/{instance_id}/overrides/{date}')
  return (
    <ListEditorCard
      title={t('manage.schedule.overridesTitle')}
      intro={t('manage.schedule.overridesIntro')}
      query={query}
      empty={t('manage.schedule.noOverrides')}
      add={
        <FormDialog
          trigger={
            <Button variant="outline">
              <Plus aria-hidden />
              {t('manage.schedule.addOverride')}
            </Button>
          }
          title={t('manage.schedule.addOverride')}
          submitLabel={t('common.save')}
          pending={save.isPending}
          error={save.error}
          onOpen={() => {
            form.reset(newOverride(tz))
            save.reset()
          }}
          onSubmit={(close) =>
            form.handleSubmit(async (value) => {
              const periods = fromDraft(value.draft)
              if (periods === null) return
              await save.mutateAsync({ params: { path: { instance_id: id, date: value.date } }, body: { periods } })
              close()
            })()
          }
        >
          <Field label={t('manage.schedule.date')} htmlFor="ov-date">
            <Input id="ov-date" type="date" {...form.register('date', { required: true })} required />
          </Field>
          <Controller
            name="draft"
            control={form.control}
            rules={{ validate: (value) => dayProblem(value) === null || t('errors.invalid_time') }}
            render={({ field, fieldState }) => (
              <>
                <DayEditor
                  idPrefix="ov"
                  name={date ? formatCivilDate(date) : t('manage.schedule.date')}
                  value={field.value}
                  onChange={field.onChange}
                />
                {fieldState.error && <FieldError>{fieldState.error.message}</FieldError>}
              </>
            )}
          />
          <p className="text-muted-foreground">{t('manage.schedule.midnightHint')}</p>
        </FormDialog>
      }
      row={(o) => (
        <Row
          key={o.date}
          label={formatCivilDate(o.date)}
          end={
            <DeleteEntryButton
              title={t('manage.schedule.removeOverride')}
              description={t('manage.schedule.removeOverrideHint')}
              onConfirm={() => del.mutateAsync({ params: { path: { instance_id: id, date: o.date } } })}
              pending={del.isPending}
              error={del.error}
              onOpen={del.reset}
            />
          }
        >
          {o.periods.length === 0
            ? t('manage.schedule.dayOff')
            : o.periods.map((p) => formatMinutesRange(p.start, p.end)).join(', ')}
        </Row>
      )}
    />
  )
}

function newBlock(tz: string) {
  const now = instantToWall(new Date().toISOString(), tz)
  return { start: now, end: now, reason: '' }
}

export function BlocksEditor() {
  const { t } = useTranslation()
  const { id, tz, path } = useInstance()
  const query = $api.useQuery(
    'get',
    '/api/v1/instances/{instance_id}/blocks',
    { params: { path } },
    { select: (value) => value.blocks },
  )
  const form = useForm({ defaultValues: newBlock(tz) })
  const save = $api.useMutation('post', '/api/v1/instances/{instance_id}/blocks')
  const del = $api.useMutation('delete', '/api/v1/instances/{instance_id}/blocks/{block_id}')
  const validTime = (value: string) => {
    try {
      wallToInstant(value, tz)
      return true
    } catch {
      return t('errors.invalid_time')
    }
  }
  return (
    <ListEditorCard
      title={t('manage.schedule.blocksTitle')}
      intro={t('manage.schedule.blocksIntro', { tz })}
      query={query}
      empty={t('manage.schedule.noBlocks')}
      add={
        <FormDialog
          trigger={
            <Button variant="outline">
              <Plus aria-hidden />
              {t('manage.schedule.addBlock')}
            </Button>
          }
          title={t('manage.schedule.addBlock')}
          submitLabel={t('common.save')}
          pending={save.isPending}
          error={save.error}
          onOpen={() => {
            form.reset(newBlock(tz))
            save.reset()
          }}
          onSubmit={(close) =>
            form.handleSubmit(async (value) => {
              await save.mutateAsync({
                params: { path },
                body: {
                  starts_at: wallToInstant(value.start, tz),
                  ends_at: wallToInstant(value.end, tz),
                  reason: value.reason.trim() || null,
                },
              })
              close()
            })()
          }
        >
          <Field label={t('manage.schedule.blockStart')} htmlFor="b-start" error={form.formState.errors.start?.message}>
            <Input id="b-start" type="datetime-local" required {...form.register('start', { validate: validTime })} />
          </Field>
          <Field label={t('manage.schedule.blockEnd')} htmlFor="b-end" error={form.formState.errors.end?.message}>
            <Input
              id="b-end"
              type="datetime-local"
              required
              {...form.register('end', {
                validate: (value) =>
                  (validTime(value) === true && value > form.getValues('start')) || t('errors.invalid_time'),
              })}
            />
          </Field>
          <Field label={t('manage.schedule.reason')} htmlFor="b-reason">
            <Input id="b-reason" maxLength={200} {...form.register('reason')} />
          </Field>
        </FormDialog>
      }
      row={(b) => (
        <Row
          key={b.id}
          end={
            <DeleteEntryButton
              title={t('manage.schedule.removeBlock')}
              pending={del.isPending}
              error={del.error}
              onOpen={del.reset}
              onConfirm={() => del.mutateAsync({ params: { path: { instance_id: id, block_id: b.id } } })}
            />
          }
        >
          {formatDateTimeRange(b.starts_at, b.ends_at, tz)}
          {b.reason && <span className="text-muted-foreground"> · {b.reason}</span>}
        </Row>
      )}
    />
  )
}
