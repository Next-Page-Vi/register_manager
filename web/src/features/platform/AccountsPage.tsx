import { useState } from 'react'
import { Controller, useForm } from 'react-hook-form'
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { $api, type Schemas } from '@/api/client'
import { ItemCard } from '@/components/blocks'
import { ConfirmButton } from '@/components/ConfirmButton'
import { Field, Notice } from '@/components/common'
import { FormDialog } from '@/components/FormDialog'
import { Page } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { LOCALE_NAMES, SUPPORTED_LOCALES } from '@/i18n'
import { accountStatusBadge } from '@/lib/status'
import { PlatformDirectory } from './PlatformDirectory'

type Account = Schemas['Account']

export function AccountsPage() {
  const { t } = useTranslation()
  const action = $api.useMutation('put', '/api/v1/platform/accounts/{account_id}/disabled')
  // Shown at the top after a password reset, which closes its dialog.
  const [resetDone, setResetDone] = useState(false)

  return (
    <Page title={t('platform.accounts.title')} actions={<CreateAccountDialog />}>
      {resetDone && (
        <Notice kind="success">
          <p>{t('platform.accounts.resetDone')}</p>
        </Notice>
      )}
      <PlatformDirectory>
        {(data) => (
          <div className="space-y-3">
            {data.accounts.map((a) => {
              const bound = data.instances.filter((i) => a.instance_ids.includes(i.id))
              return (
                <ItemCard
                  key={a.id}
                  actions={
                    // The super admin edits itself on "My account".
                    !a.super_admin && (
                      <>
                        <EditAccountDialog account={a} />
                        <ResetPasswordDialog account={a} onDone={() => setResetDone(true)} />
                        <ConfirmButton
                          variant={a.disabled ? 'outline' : 'destructive-outline'}
                          size="sm"
                          title={a.disabled ? t('platform.accounts.enable') : t('platform.accounts.disable')}
                          description={a.disabled ? a.name : t('platform.accounts.disableHint')}
                          confirmLabel={t('common.confirm')}
                          pending={action.isPending}
                          error={action.error}
                          onOpen={action.reset}
                          onConfirm={() =>
                            action.mutateAsync({
                              params: { path: { account_id: a.id } },
                              body: { disabled: !a.disabled },
                            })
                          }
                        >
                          {a.disabled ? t('platform.accounts.enable') : t('platform.accounts.disable')}
                        </ConfirmButton>
                      </>
                    )
                  }
                >
                  <p className="text-lg font-semibold">
                    {a.name} <span className="text-muted-foreground font-normal">({a.login})</span>
                  </p>
                  <div className="flex flex-wrap gap-2">
                    <Badge variant={a.super_admin ? 'default' : 'secondary'}>
                      {t(a.super_admin ? 'role.super' : 'role.operator')}
                    </Badge>
                    {bound.map((i) => (
                      <Badge key={i.id} variant="outline">
                        {i.name}
                      </Badge>
                    ))}
                    {!a.super_admin && bound.length === 0 && (
                      <Badge variant="outline">{t('platform.accounts.unassigned')}</Badge>
                    )}
                    {a.disabled && (
                      <Badge variant={accountStatusBadge.disabled}>{t('platform.accounts.disabled')}</Badge>
                    )}
                  </div>
                  {a.super_admin && <p className="text-muted-foreground">{t('platform.accounts.superAdminHint')}</p>}
                </ItemCard>
              )
            })}
          </div>
        )}
      </PlatformDirectory>
    </Page>
  )
}

function CreateAccountDialog() {
  const { t } = useTranslation()
  const form = useForm({ defaultValues: { login: '', name: '', password: '', locale: null as string | null } })
  const create = $api.useMutation('post', '/api/v1/platform/accounts')
  return (
    <FormDialog
      trigger={
        <Button>
          <Plus aria-hidden />
          {t('platform.accounts.create')}
        </Button>
      }
      title={t('platform.accounts.create')}
      description={t('platform.accounts.createHint')}
      submitLabel={t('common.create')}
      pending={create.isPending}
      error={create.error}
      onOpen={() => {
        form.reset()
        create.reset()
      }}
      onSubmit={(close) =>
        form.handleSubmit(async ({ login, name, password, locale }) => {
          await create.mutateAsync({ body: { login: login.trim(), name: name.trim(), password, locale } })
          close()
        })()
      }
    >
      <Field label={t('platform.accounts.login')} htmlFor="new-login">
        <Input id="new-login" {...form.register('login')} autoComplete="off" required />
      </Field>
      <Field label={t('platform.accounts.name')} htmlFor="new-name">
        <Input id="new-name" {...form.register('name')} required />
      </Field>
      <Field
        label={t('platform.accounts.initialPassword')}
        htmlFor="new-password"
        hint={t('platform.accounts.passwordHint')}
      >
        <Input id="new-password" type="text" {...form.register('password')} autoComplete="new-password" required />
      </Field>
      <Field label={t('common.language')} htmlFor="new-locale">
        <Controller
          name="locale"
          control={form.control}
          render={({ field }) => (
            <NativeSelect
              id="new-locale"
              {...field}
              value={field.value ?? ''}
              onChange={(event) => field.onChange(event.target.value || null)}
            >
              <option value="">{t('platform.accounts.localeAuto')}</option>
              {SUPPORTED_LOCALES.map((locale) => (
                <option key={locale} value={locale}>
                  {LOCALE_NAMES[locale]}
                </option>
              ))}
            </NativeSelect>
          )}
        />
      </Field>
    </FormDialog>
  )
}

function EditAccountDialog({ account }: { account: Account }) {
  const { t } = useTranslation()
  const defaults = { login: account.login, name: account.name }
  const form = useForm({ defaultValues: defaults })
  const save = $api.useMutation('put', '/api/v1/platform/accounts/{account_id}')
  return (
    <FormDialog
      trigger={
        <Button variant="outline" size="sm">
          {t('common.edit')}
        </Button>
      }
      title={t('platform.accounts.edit')}
      submitLabel={t('common.save')}
      pending={save.isPending}
      error={save.error}
      onOpen={() => {
        form.reset(defaults)
        save.reset()
      }}
      onSubmit={(close) =>
        form.handleSubmit(async ({ login, name }) => {
          await save.mutateAsync({
            params: { path: { account_id: account.id } },
            body: { login: login.trim(), name: name.trim() },
          })
          close()
        })()
      }
    >
      <Field label={t('platform.accounts.login')} htmlFor={`login-${account.id}`}>
        <Input id={`login-${account.id}`} {...form.register('login')} required />
      </Field>
      <Field label={t('platform.accounts.name')} htmlFor={`name-${account.id}`}>
        <Input id={`name-${account.id}`} {...form.register('name')} required />
      </Field>
    </FormDialog>
  )
}

function ResetPasswordDialog({ account, onDone }: { account: Account; onDone: () => void }) {
  const { t } = useTranslation()
  const form = useForm({ defaultValues: { password: '' } })
  const save = $api.useMutation('post', '/api/v1/platform/accounts/{account_id}/password')
  return (
    <FormDialog
      trigger={
        <Button variant="outline" size="sm">
          {t('platform.accounts.resetPassword')}
        </Button>
      }
      title={t('platform.accounts.resetPassword')}
      description={t('platform.accounts.resetHint', { name: account.name })}
      submitLabel={t('platform.accounts.resetPassword')}
      pending={save.isPending}
      error={save.error}
      onOpen={() => {
        form.reset()
        save.reset()
      }}
      onSubmit={(close) =>
        form.handleSubmit(async ({ password }) => {
          await save.mutateAsync({ params: { path: { account_id: account.id } }, body: { password } })
          onDone()
          close()
        })()
      }
    >
      <Field
        label={t('platform.accounts.newPassword')}
        htmlFor={`pw-${account.id}`}
        hint={t('platform.accounts.passwordHint')}
      >
        <Input
          id={`pw-${account.id}`}
          type="text"
          autoComplete="new-password"
          {...form.register('password')}
          required
        />
      </Field>
    </FormDialog>
  )
}
