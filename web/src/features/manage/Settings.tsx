import { useUnsavedChanges } from '@/components/UnsavedChanges'
import { useMemo } from 'react'
import { Controller, useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { $api, errorInfo, type Schemas } from '@/api/client'
import { ConfirmButton } from '@/components/ConfirmButton'
import { CheckboxField, SaveBar } from '@/components/blocks'
import { ActionButton, QueryContent, ErrorNotice, Field, Notice } from '@/components/common'
import { Page } from '@/components/layout'
import { NativeSelect } from '@/components/ui/native-select'
import { QrLink } from '@/components/QrLink'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { LOCALE_NAMES, SUPPORTED_LOCALES } from '@/i18n'
import { timeZones } from '@/lib/timezones'
import { CustomerHeader } from '@/components/CustomerHeader'
import { isHexColor } from '@/lib/color'
import { useInstance } from './ManageLayout'
import { Surface } from '@/components/Surface'

function StatusCard() {
  const { t } = useTranslation()
  const { access, path } = useInstance()
  const active = access.instance.status === 'active'
  const act = $api.useMutation('put', '/api/v1/instances/{instance_id}/status')
  const variables = { params: { path }, body: { status: active ? ('paused' as const) : ('active' as const) } }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('settings.statusTitle')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-lg font-semibold">{t(`status.instance.${access.instance.status}`)}</p>
        <p>{active ? t('settings.activeHint') : t('settings.pausedHint')}</p>
        <ErrorNotice error={act.error} />
        {active ? (
          <ConfirmButton
            variant="destructive-outline"
            title={t('settings.pause')}
            description={t('settings.pauseConfirm')}
            confirmLabel={t('settings.pause')}
            onConfirm={() => act.mutateAsync(variables)}
            pending={act.isPending}
            error={act.error}
            onOpen={act.reset}
          >
            {t('settings.pause')}
          </ConfirmButton>
        ) : (
          <ActionButton onClick={() => act.mutate(variables)} pending={act.isPending}>
            {t('settings.activate')}
          </ActionButton>
        )}
      </CardContent>
    </Card>
  )
}

function InviteCard() {
  const { t } = useTranslation()
  const { access, path } = useInstance()
  const invite = $api.useQuery('get', '/api/v1/instances/{instance_id}/invite', { params: { path } })
  const rotate = $api.useMutation('post', '/api/v1/instances/{instance_id}/invite/rotate')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('settings.inviteTitle')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p>{t('settings.inviteIntro')}</p>
        <QueryContent query={invite}>
          {(data) => (
            <QrLink
              url={window.location.origin + data.register_path}
              fileName={`register-${access.instance.slug}.png`}
              label={t('settings.registerLink')}
            />
          )}
        </QueryContent>
        <ErrorNotice error={rotate.error} />
        <ConfirmButton
          variant="outline"
          title={t('settings.rotate')}
          description={t('settings.rotateConfirm')}
          confirmLabel={t('settings.rotate')}
          onConfirm={() => rotate.mutateAsync({ params: { path } })}
          pending={rotate.isPending}
          error={rotate.error}
          onOpen={rotate.reset}
        >
          {t('settings.rotate')}
        </ConfirmButton>
      </CardContent>
    </Card>
  )
}

function GeneralCard() {
  const { t } = useTranslation()
  const { access, path } = useInstance()
  const inst = access.instance
  const zones = useMemo(timeZones, [])
  const values = (value: Schemas['Instance']) => ({
    name: value.name,
    timezone: value.timezone,
    default_locale: value.default_locale,
    contact_kind: value.contact_kind,
    contact_required: value.contact_required,
    color: value.brand_color,
  })
  const form = useForm({ defaultValues: values(inst) })
  const color = useWatch({ control: form.control, name: 'color' })
  const name = useWatch({ control: form.control, name: 'name' })
  const mutation = $api.useMutation('put', '/api/v1/instances/{instance_id}', {
    onSuccess: (result) => form.reset(values(result)),
  })
  useUnsavedChanges(form.formState.isDirty, mutation.isPending)
  const info = errorInfo(mutation.error)
  const needConfirm =
    info.code === 'timezone_change_needs_confirmation' && typeof info.params.count === 'number'
      ? info.params.count
      : null
  const save = (confirm: boolean) =>
    form.handleSubmit(({ color, ...value }) =>
      mutation.mutate({
        params: { path },
        body: {
          ...value,
          name: value.name.trim(),
          brand_color: color === null ? null : color.trim(),
          confirm_timezone_change: confirm,
        },
      }),
    )()

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('settings.generalTitle')}</CardTitle>
      </CardHeader>
      <CardContent className="@container/form space-y-(--form-gap)">
        <SaveBar
          sticky
          dirty={form.formState.isDirty}
          error={needConfirm === null ? mutation.error : null}
          saved={mutation.isSuccess && !form.formState.isDirty}
          pending={mutation.isPending}
          onSave={() => void save(false)}
        />
        {needConfirm !== null && (
          <Notice kind="warning" title={t('settings.tzConfirmTitle')}>
            <p>{t('settings.tzConfirmBody', { count: needConfirm })}</p>
            <Button variant="destructive" className="mt-2" onClick={() => save(true)} pending={mutation.isPending}>
              {t('settings.tzConfirmButton')}
            </Button>
          </Notice>
        )}
        <fieldset disabled={mutation.isPending} className="min-w-0 space-y-(--form-gap)">
        <Field label={t('platform.instances.name')} htmlFor="s-name" error={form.formState.errors.name?.message}>
          <Input id="s-name" {...form.register('name', { required: t('errors.invalid_name', { max: 100 }) })} />
        </Field>
        <Field label={t('platform.instances.timezone')} htmlFor="s-tz" hint={t('settings.timezoneHint')}>
          <NativeSelect id="s-tz" {...form.register('timezone')}>
            {zones.map((z) => (
              <option key={z} value={z}>
                {z}
              </option>
            ))}
          </NativeSelect>
        </Field>
        <Field label={t('settings.defaultLocale')} htmlFor="s-locale" hint={t('settings.defaultLocaleHint')}>
          <NativeSelect id="s-locale" {...form.register('default_locale')}>
            {SUPPORTED_LOCALES.map((l) => (
              <option key={l} value={l}>
                {LOCALE_NAMES[l]}
              </option>
            ))}
          </NativeSelect>
        </Field>
        <div className="space-y-(--form-gap) border-t pt-5">
          <Field label={t('settings.contactKind')} htmlFor="s-kind">
            <NativeSelect id="s-kind" {...form.register('contact_kind')}>
              <option value="phone">{t('register.contact.phone')}</option>
              <option value="email">{t('register.contact.email')}</option>
              <option value="text">{t('register.contact.text')}</option>
            </NativeSelect>
          </Field>
          <Controller
            name="contact_required"
            control={form.control}
            render={({ field }) => (
              <CheckboxField checked={field.value} onCheckedChange={field.onChange}>
                {t('settings.contactRequired')}
              </CheckboxField>
            )}
          />
        </div>
        <div className="space-y-3 border-t pt-5">
          <Controller
            name="color"
            control={form.control}
            rules={{ validate: (value) => value === null || isHexColor(value.trim()) || t('errors.invalid_color') }}
            render={({ field, fieldState }) => (
              <Field label={t('settings.brandColor')} htmlFor="s-color" hint={t('settings.brandColorHint')} error={fieldState.error?.message}>
                <Input
                  {...field}
                  id="s-color"
                  className="w-40 max-w-full font-mono"
                  maxLength={7}
                  spellCheck={false}
                  autoComplete="off"
                  value={field.value ?? ''}
                  onChange={(e) => field.onChange(e.target.value.trim() === '' ? null : e.target.value)}
                />
              </Field>
            )}
          />
          <div className="flex flex-wrap items-center gap-3">
            <input
              type="color"
              aria-label={t('settings.brandColorCode')}
              className="border-input h-(--control-height) w-16 cursor-pointer rounded-md border bg-background p-1"
              value={isHexColor(color) ? color.toLowerCase() : '#1d4ed8'}
              onChange={(e) => form.setValue('color', e.target.value, { shouldDirty: true, shouldValidate: true })}
            />
            <Button
              type="button"
              variant="outline"
              disabled={color === null}
              onClick={() => form.setValue('color', null, { shouldDirty: true, shouldValidate: true })}
            >
              {t('settings.brandColorClear')}
            </Button>
          </div>
          <figure className="space-y-1">
            <figcaption className="text-muted-foreground">{t('settings.brandColorPreview')}</figcaption>
            <Surface kind="customer" className="overflow-hidden rounded-lg border" aria-hidden>
              <CustomerHeader title={name || inst.name} brand={isHexColor(color) ? color : null} />
            </Surface>
          </figure>
        </div>
        </fieldset>
      </CardContent>
    </Card>
  )
}

export function Settings() {
  const { t } = useTranslation()
  const { id } = useInstance()
  return (
    <Page title={t('manage.nav.settings')}>
      <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)] xl:grid-rows-[auto_1fr]">
        <div className="min-w-0 xl:col-start-2"><StatusCard /></div>
        <div className="min-w-0 xl:col-start-1 xl:row-span-2 xl:row-start-1"><GeneralCard key={id} /></div>
        <div className="min-w-0 xl:col-start-2"><InviteCard /></div>
      </div>
    </Page>
  )
}
