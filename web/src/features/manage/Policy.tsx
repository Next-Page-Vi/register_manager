import { useUnsavedChanges } from '@/components/UnsavedChanges'
import type { CSSProperties } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { $api, type Schemas } from '@/api/client'
import { policyConstraints } from '@/api/constraints'
import { CheckboxField, SaveBar } from '@/components/blocks'
import { QueryContent, Field, Loading, Notice } from '@/components/common'
import { Page } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { useInstance } from './ManageLayout'
import { PreviewView } from './Preview'
import './policy.css'

type Policy = Schemas['Policy']
type PolicyDraft = { [K in keyof Policy]: null extends Policy[K] ? string | null : string }
const fields = Object.keys(policyConstraints) as (keyof Policy)[]
const sections = [
  { key: 'duration', fields: ['service_duration_minutes', 'slot_step_minutes', 'buffer_minutes'] },
  { key: 'timing', fields: ['booking_horizon_days', 'min_booking_notice_minutes', 'min_cancellation_notice_minutes'] },
  { key: 'limits', fields: ['daily_customer_limit', 'per_customer_daily_limit', 'per_customer_weekly_limit', 'max_future_bookings'] },
] as const satisfies readonly { key: string; fields: readonly (keyof Policy)[] }[]
const draft = (policy: Policy): PolicyDraft =>
  Object.fromEntries(fields.map((key) => [key, policy[key] === null ? null : String(policy[key])])) as PolicyDraft
// Called only after every field has passed integer and range validation.
const policy = (values: PolicyDraft): Policy =>
  Object.fromEntries(fields.map((key) => [key, values[key] === null ? null : Number(values[key])])) as Policy

/** Shared label/control/help rows without subgrid or measuring translated text. */
function fieldPosition(index: number, limits: boolean): CSSProperties {
  const tracks = limits ? 4 : 3
  return {
    '--column-two': index % 2 + 1,
    '--row-two': Math.floor(index / 2) * tracks + 1,
    '--column-three': index % 3 + 1,
    '--row-three': Math.floor(index / 3) * tracks + 1,
    '--group-gap-two': index < 2 ? '0rem' : '0.75rem',
    '--group-gap-three': index < 3 ? '0rem' : '0.75rem',
  } as CSSProperties
}

export function PolicyPage() {
  const { t } = useTranslation()
  const { id, path } = useInstance()
  const query = $api.useQuery('get', '/api/v1/instances/{instance_id}/policy', { params: { path } })
  return (
    <Page title={t('manage.policy.title')} description={t('manage.policy.intro')}>
      <QueryContent query={query} pendingFallback={<Loading layout="form" />}>{(value) => <PolicyForm key={id} initial={value} />}</QueryContent>
    </Page>
  )
}

function PolicyForm({ initial }: { initial: Policy }) {
  const { t } = useTranslation()
  const { path, tz } = useInstance()
  const form = useForm<PolicyDraft>({ defaultValues: draft(initial) })
  const preview = $api.useMutation('post', '/api/v1/instances/{instance_id}/policy/preview')
  const save = $api.useMutation('put', '/api/v1/instances/{instance_id}/policy', {
    onSuccess: (result) => {
      form.reset(draft(result.policy))
      preview.reset()
    },
  })
  useUnsavedChanges(form.formState.isDirty, save.isPending)
  const pending = save.isPending || preview.isPending
  return (
    <form
      noValidate
      className="@container/form space-y-(--form-gap)"
      onSubmit={form.handleSubmit((values) => {
        preview.reset()
        save.mutate({ params: { path }, body: policy(values) })
      })}
    >
      <SaveBar
        sticky
        dirty={form.formState.isDirty}
        error={save.error ?? preview.error}
        pending={pending}
        saved={
          save.isSuccess &&
          !form.formState.isDirty && (
            <Notice kind="success" title={t('common.saved')} />
          )
        }
        extra={
          <Button
            type="button"
            variant="outline"
            disabled={pending}
            onClick={form.handleSubmit((values) => {
              save.reset()
              preview.mutate({ params: { path }, body: policy(values) })
            })}
          >
            {t('manage.preview.button')}
          </Button>
        }
      />
      <fieldset disabled={save.isPending} className="min-w-0 space-y-(--form-gap)">
        {sections.map((section) => (
          <Card key={section.key} role="group" aria-labelledby={`policy-${section.key}`}>
            <CardHeader>
              <CardTitle id={`policy-${section.key}`}>{t(`manage.policy.groups.${section.key}`)}</CardTitle>
            </CardHeader>
            <CardContent className="policy-fields" data-columns={section.key === 'limits' ? 2 : 3}>
              {section.fields.map((key, index) => {
                const { min, max, nullable } = policyConstraints[key]
                return (
                  <Controller
                    key={key}
                    name={key}
                    control={form.control}
                    rules={{
                      validate: (value) =>
                        value === null
                          ? nullable
                          : (value.trim() !== '' &&
                              Number.isInteger(Number(value)) &&
                              Number(value) >= min &&
                              Number(value) <= max) ||
                            t('errors.invalid_policy', { field: t(`manage.policy.fields.${key}`) }),
                    }}
                    render={({ field, fieldState }) => (
                      <div className="policy-field space-y-2" style={fieldPosition(index, section.key === 'limits')}>
                        <Field
                          className="policy-field-body"
                          label={t(`manage.policy.fields.${key}`)}
                          htmlFor={key}
                          hint={t(`manage.policy.hints.${key}`)}
                          error={fieldState.error?.message}
                        >
                          <Input
                            {...field}
                            id={key}
                            type="number"
                            inputMode="numeric"
                            min={min}
                            max={max}
                            required
                            disabled={field.value === null}
                            value={field.value ?? ''}
                          />
                        </Field>
                        {nullable && (
                          <CheckboxField
                            className="policy-unlimited"
                            checked={field.value === null}
                            onCheckedChange={(unlimited) => field.onChange(unlimited ? null : '1')}
                          >
                            {t('manage.policy.unlimited')}
                          </CheckboxField>
                        )}
                      </div>
                    )}
                  />
                )
              })}
            </CardContent>
          </Card>
        ))}
      </fieldset>
      {preview.data && <PreviewView preview={preview.data} tz={tz} />}
    </form>
  )
}
