import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { $api } from '@/api/client'
import { SaveBar, TextLink } from '@/components/blocks'
import { QueryContent, Field, Notice } from '@/components/common'
import { Page } from '@/components/layout'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'

/**
 * System-wide settings for the super admin: the longest repeating schedule a
 * calendar may save, and the calendars that are already longer.
 */
export function SettingsPage() {
  const { t } = useTranslation()
  const query = $api.useQuery('get', '/api/v1/platform/settings')

  return (
    <Page title={t('platform.settings.title')}>
      <QueryContent query={query}>
        {(data) => (
          <>
            <SettingsForm initial={data.max_cycle_days} />
            {data.over_limit.length > 0 && (
              <Notice kind="warning" title={t('platform.settings.overLimitTitle')}>
                <p>{t('platform.settings.overLimitBody')}</p>
                <ul className="space-y-1">
                  {data.over_limit.map((o) => (
                    <li key={o.instance_id} className="flex flex-wrap items-center gap-x-3">
                      <span>{t('platform.settings.overLimitItem', { name: o.name, days: o.cycle_length_days })}</span>
                      <TextLink variant="action" to={`/manage/${o.slug}/schedule`}>
                        {t('platform.settings.openSchedule')}
                      </TextLink>
                    </li>
                  ))}
                </ul>
              </Notice>
            )}
          </>
        )}
      </QueryContent>
    </Page>
  )
}

function SettingsForm({ initial }: { initial: number }) {
  const { t } = useTranslation()
  const form = useForm({ defaultValues: { max: String(initial) } })
  const save = $api.useMutation('put', '/api/v1/platform/settings', {
    onSuccess: (result) => form.reset({ max: String(result.max_cycle_days) }),
  })
  return (
    <Card>
      <CardContent>
        <form
          className="space-y-(--form-gap)"
          onSubmit={form.handleSubmit(({ max }) => save.mutate({ body: { max_cycle_days: Number(max) } }))}
        >
          <Field
            label={t('platform.settings.maxCycleDays')}
            htmlFor="max-cycle"
            hint={t('platform.settings.maxCycleDaysHint')}
          >
            <Input
              id="max-cycle"
              type="number"
              min={1}
              max={366}
              className="w-32"
              required
              {...form.register('max', {
                required: true,
                min: 1,
                max: 366,
                validate: (value) => Number.isInteger(Number(value)),
              })}
            />
          </Field>
          <SaveBar error={save.error} saved={save.isSuccess && !form.formState.isDirty} pending={save.isPending} />
        </form>
      </CardContent>
    </Card>
  )
}
