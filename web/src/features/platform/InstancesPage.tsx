import { useMemo, useState } from 'react'
import { Controller, useForm, useWatch } from 'react-hook-form'
import { Plus, Trash2, X } from 'lucide-react'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { $api, type Schemas } from '@/api/client'
import { CheckboxField, Chip, DescriptionList, ItemCard } from '@/components/blocks'
import { ConfirmButton } from '@/components/ConfirmButton'
import { QueryContent, EmptyState, Field, Notice } from '@/components/common'
import { FormDialog } from '@/components/FormDialog'
import { Page } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { LOCALE_NAMES, SUPPORTED_LOCALES, currentLocale } from '@/i18n'
import { accountStatusBadge, instanceStatusBadge } from '@/lib/status'
import { timeZones } from '@/lib/timezones'
import { PlatformDirectory } from './PlatformDirectory'

type Account = Schemas['Account']
type PlatformInstance = Schemas['PlatformInstance']

/**
 * All calendars, for the super admin: create, add and remove operators,
 * and delete.
 */
export function InstancesPage() {
  const { t } = useTranslation()
  const action = $api.useMutation('delete', '/api/v1/platform/instances/{instance_id}/operators/{account_id}')
  const [deleted, setDeleted] = useState<string | null>(null)

  return (
    <Page title={t('platform.instances.title')} actions={<CreateInstanceDialog />}>
      {deleted && (
        <Notice kind="success">
          <p>{t('platform.instances.deleted', { name: deleted })}</p>
        </Notice>
      )}
      <PlatformDirectory>
        {(data) => {
          // Operators that can be bound: enabled, and never the super admin.
          const operators = data.accounts.filter((a) => !a.super_admin && !a.disabled)
          if (data.instances.length === 0) return <EmptyState>{t('platform.instances.empty')}</EmptyState>
          return (
            <div className="space-y-3">
              {data.instances.map((i) => (
                <ItemCard
                  key={i.id}
                  actions={
                    <>
                      <Button asChild size="sm">
                        <Link to={`/manage/${i.slug}`}>{t('platform.instances.open')}</Link>
                      </Button>
                      <BindOperatorDialog instance={i} operators={operators} />
                      <DeleteInstanceDialog
                        instance={i}
                        onDeleted={() => {
                          setDeleted(i.name)
                        }}
                      />
                    </>
                  }
                >
                  <h3 className="flex flex-wrap items-center gap-2">
                    {i.name}
                    <Badge variant={instanceStatusBadge[i.status]}>{t(`status.instance.${i.status}`)}</Badge>
                  </h3>
                  <DescriptionList
                    items={[
                      [t('platform.instances.slug'), i.slug],
                      [t('platform.instances.timezone'), i.timezone],
                      [
                        t('platform.instances.operators'),
                        <span className="flex flex-wrap items-start gap-2">
                          {i.operators.length === 0 && (
                            <span className="text-muted-foreground">{t('platform.instances.noOperators')}</span>
                          )}
                          {i.operators.map((o) => (
                            <Chip
                              key={o.id}
                              remove={
                                <ConfirmButton
                                  variant="ghost"
                                  size="sm"
                                  className="hover:bg-background min-w-(--control-small) rounded-full px-2"
                                  title={t('platform.instances.unbind')}
                                  description={t('platform.instances.unbindHint', { name: o.name, instance: i.name })}
                                  confirmLabel={t('platform.instances.unbind')}
                                  confirmVariant="destructive"
                                  pending={action.isPending}
                                  error={action.error}
                                  onOpen={action.reset}
                                  onConfirm={() =>
                                    action.mutateAsync({ params: { path: { instance_id: i.id, account_id: o.id } } })
                                  }
                                >
                                  <X aria-hidden />
                                  <span className="sr-only">{`${t('platform.instances.unbind')} ${o.name}`}</span>
                                </ConfirmButton>
                              }
                            >
                              {o.name}
                              {o.disabled && (
                                <Badge variant={accountStatusBadge.disabled}>{t('platform.accounts.disabled')}</Badge>
                              )}
                            </Chip>
                          ))}
                        </span>,
                      ],
                    ]}
                  />
                </ItemCard>
              ))}
            </div>
          )
        }}
      </PlatformDirectory>
    </Page>
  )
}

function CreateInstanceDialog() {
  const { t } = useTranslation()
  const zones = useMemo(timeZones, [])
  const defaults = () => ({
    name: '',
    slug: '',
    copyFrom: null as string | null,
    tz: Intl.DateTimeFormat().resolvedOptions().timeZone,
    locale: currentLocale() as string,
    kind: 'phone' as 'phone' | 'email' | 'text',
    required: true,
  })
  const form = useForm({ defaultValues: defaults() })
  const query = $api.useQuery('get', '/api/v1/platform/instances')
  const instances = query.data?.instances ?? []
  const create = $api.useMutation('post', '/api/v1/platform/instances')

  // Copying starts from the source's time zone, language and contact
  // settings; all can still be changed before creating.
  const chooseSource = (id: string | null) => {
    const src = instances.find((instance) => instance.id === id)
    const values = form.getValues()
    form.reset(
      src
        ? {
            ...values,
            copyFrom: id,
            tz: src.timezone,
            locale: src.default_locale,
            kind: src.contact_kind,
            required: src.contact_required,
          }
        : { ...values, copyFrom: id },
    )
  }

  return (
    <FormDialog
      wide
      trigger={
        <Button>
          <Plus aria-hidden />
          {t('platform.instances.create')}
        </Button>
      }
      title={t('platform.instances.create')}
      description={t('platform.instances.createHint')}
      submitLabel={t('common.create')}
      pending={create.isPending}
      error={create.error}
      onOpen={() => {
        form.reset(defaults())
        create.reset()
      }}
      onSubmit={(close) =>
        form.handleSubmit(async ({ name, slug, tz, locale, kind, required, copyFrom }) => {
          await create.mutateAsync({
            body: {
              name: name.trim(),
              slug: slug.trim().toLowerCase(),
              timezone: tz,
              default_locale: locale,
              contact_kind: kind,
              contact_required: required,
              copy_from: copyFrom,
            },
          })
          close()
        })()
      }
    >
      <Field label={t('platform.instances.name')} htmlFor="inst-name">
        <Input id="inst-name" {...form.register('name')} required />
      </Field>
      <Field label={t('platform.instances.slug')} htmlFor="inst-slug" hint={t('platform.instances.slugHint')}>
        <Input id="inst-slug" {...form.register('slug')} pattern="[a-z0-9]+(-[a-z0-9]+)*" required />
      </Field>
      {instances.length > 0 && (
        <Field label={t('platform.instances.copyFrom')} htmlFor="inst-copy" hint={t('platform.instances.copyHint')}>
          <Controller
            name="copyFrom"
            control={form.control}
            render={({ field }) => (
              <NativeSelect
                id="inst-copy"
                {...field}
                value={field.value ?? ''}
                onChange={(event) => chooseSource(event.target.value || null)}
              >
                <option value="">{t('platform.instances.copyNone')}</option>
                {instances.map((instance) => (
                  <option key={instance.id} value={instance.id}>
                    {instance.name}
                  </option>
                ))}
              </NativeSelect>
            )}
          />
        </Field>
      )}
      <Field label={t('platform.instances.timezone')} htmlFor="inst-tz">
        <NativeSelect id="inst-tz" {...form.register('tz')}>
          {zones.map((z) => (
            <option key={z} value={z}>
              {z}
            </option>
          ))}
        </NativeSelect>
      </Field>
      <Field label={t('settings.defaultLocale')} htmlFor="inst-locale">
        <NativeSelect id="inst-locale" {...form.register('locale')}>
          {SUPPORTED_LOCALES.map((l) => (
            <option key={l} value={l}>
              {LOCALE_NAMES[l]}
            </option>
          ))}
        </NativeSelect>
      </Field>
      <Field label={t('settings.contactKind')} htmlFor="inst-kind">
        <NativeSelect id="inst-kind" {...form.register('kind')}>
          <option value="phone">{t('register.contact.phone')}</option>
          <option value="email">{t('register.contact.email')}</option>
          <option value="text">{t('register.contact.text')}</option>
        </NativeSelect>
      </Field>
      <Controller
        name="required"
        control={form.control}
        render={({ field }) => (
          <CheckboxField checked={field.value} onCheckedChange={field.onChange}>
            {t('settings.contactRequired')}
          </CheckboxField>
        )}
      />
    </FormDialog>
  )
}

function BindOperatorDialog({ instance, operators }: { instance: PlatformInstance; operators: Account[] }) {
  const { t } = useTranslation()
  const options = operators.filter((a) => !instance.operators.some((o) => o.id === a.id))
  const form = useForm({ defaultValues: { account: options[0]?.id ?? null } })
  const bind = $api.useMutation('put', '/api/v1/platform/instances/{instance_id}/operators/{account_id}')
  return (
    <FormDialog
      trigger={
        <Button variant="outline" size="sm">
          {t('platform.instances.bindOperator')}
        </Button>
      }
      title={t('platform.instances.bindOperator')}
      description={`${instance.name} · ${t('platform.instances.bindHint')}`}
      submitLabel={t('common.add')}
      pending={bind.isPending}
      error={bind.error}
      onOpen={() => {
        form.reset({ account: options[0]?.id ?? null })
        bind.reset()
      }}
      onSubmit={(close) =>
        form.handleSubmit(async ({ account }) => {
          if (account === null) return
          await bind.mutateAsync({ params: { path: { instance_id: instance.id, account_id: account } } })
          close()
        })()
      }
    >
      {options.length === 0 ? (
        <Notice kind="warning">
          <p>{t('platform.instances.noOperatorCandidates')}</p>
        </Notice>
      ) : (
        <Field label={t('platform.instances.operator')} htmlFor={`op-${instance.id}`}>
          <Controller
            name="account"
            control={form.control}
            rules={{ required: true }}
            render={({ field }) => (
              <NativeSelect
                id={`op-${instance.id}`}
                {...field}
                value={field.value ?? ''}
                onChange={(event) => field.onChange(event.target.value || null)}
                required
              >
                {options.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name} ({a.login})
                  </option>
                ))}
              </NativeSelect>
            )}
          />
        </Field>
      )}
    </FormDialog>
  )
}

/**
 * Deleting shows what will be removed (upcoming appointments stand out) and
 * is enabled only after the calendar's name is typed exactly.
 */
function DeleteInstanceDialog({ instance, onDeleted }: { instance: PlatformInstance; onDeleted: () => void }) {
  const { t } = useTranslation()
  const form = useForm({ defaultValues: { typed: '' } })
  const typed = useWatch({ control: form.control, name: 'typed' })
  const inputId = `del-${instance.id}`
  const impact = $api.useQuery(
    'get',
    '/api/v1/platform/instances/{instance_id}/deletion-preview',
    { params: { path: { instance_id: instance.id } } },
    { enabled: false },
  )
  const remove = $api.useMutation('delete', '/api/v1/platform/instances/{instance_id}', { onSuccess: onDeleted })
  const load = () => {
    form.reset()
    remove.reset()
    void impact.refetch()
  }

  return (
    <ConfirmButton
      variant="destructive-outline"
      size="sm"
      title={t('platform.instances.deleteTitle', { name: instance.name })}
      description={t('platform.instances.deleteWarning')}
      confirmLabel={t('platform.instances.deleteButton')}
      confirmDisabled={typed.trim() !== instance.name || !impact.isSuccess || impact.isFetching}
      pending={remove.isPending}
      error={remove.error}
      onOpen={load}
      onConfirm={() =>
        remove.mutateAsync({ params: { path: { instance_id: instance.id } }, body: { confirm_name: typed.trim() } })
      }
      body={
        <>
          <QueryContent query={impact}>
            {(impact) => (
              <ul className="space-y-1">
                <li>{t('platform.instances.deleteCustomers', { count: impact.customers })}</li>
                <li className={impact.future_appointments > 0 ? 'text-destructive font-bold' : ''}>
                  {t('platform.instances.deleteFuture', { count: impact.future_appointments })}
                </li>
                <li>{t('platform.instances.deleteAll', { count: impact.appointments })}</li>
              </ul>
            )}
          </QueryContent>
          <Field label={t('platform.instances.deleteConfirm', { name: instance.name })} htmlFor={inputId}>
            <Input id={inputId} {...form.register('typed')} autoComplete="off" spellCheck={false} />
          </Field>
        </>
      }
    >
      <Trash2 aria-hidden />
      {t('platform.instances.delete')}
    </ConfirmButton>
  )
}
