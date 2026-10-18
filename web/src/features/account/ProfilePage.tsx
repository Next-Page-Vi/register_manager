import { useForm } from 'react-hook-form'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { $api } from '@/api/client'
import { SaveBar } from '@/components/blocks'
import { Field, Notice } from '@/components/common'
import { Page } from '@/components/layout'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { useAccount } from './AccountLayout'

/**
 * My account: every account edits its own login name, name and password.
 * The super admin is sent here first after signing in with the generated
 * initial password, and sees only the password form until it is changed.
 */
export function ProfilePage() {
  const { t } = useTranslation()
  const { me } = useAccount()
  return (
    <Page title={t('profile.title')}>
      {me.must_change_password && (
        <Notice kind="warning">
          <p>{t('profile.mustChange')}</p>
        </Notice>
      )}
      <div className="grid items-start gap-6 lg:grid-cols-2">
        {!me.must_change_password && <DetailsCard />}
        <PasswordCard />
      </div>
    </Page>
  )
}

function DetailsCard() {
  const { t } = useTranslation()
  const { me } = useAccount()
  const form = useForm({ defaultValues: { login: me.login, name: me.name } })
  const save = $api.useMutation('put', '/api/v1/account/profile', {
    onSuccess: (_value, variables) => form.reset(variables.body),
  })
  const onSubmit = form.handleSubmit(({ login, name }) =>
    save.mutate({ body: { login: login.trim(), name: name.trim() } }),
  )
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('profile.detailsTitle')}</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={onSubmit} className="space-y-(--form-gap)">
          <Field label={t('platform.accounts.login')} htmlFor="me-login">
            <Input id="me-login" {...form.register('login')} autoComplete="username" required />
          </Field>
          <Field label={t('platform.accounts.name')} htmlFor="me-name">
            <Input id="me-name" {...form.register('name')} autoComplete="name" required />
          </Field>
          <SaveBar error={save.error} saved={save.isSuccess && !form.formState.isDirty} pending={save.isPending} />
        </form>
      </CardContent>
    </Card>
  )
}

function PasswordCard() {
  const { t } = useTranslation()
  const { me } = useAccount()
  const navigate = useNavigate()
  const form = useForm({ defaultValues: { current: '', next: '', repeat: '' } })
  const firstChange = me.must_change_password
  const change = $api.useMutation('post', '/api/v1/account/password', {
    onSuccess: () => {
      form.reset()
      if (firstChange) navigate('/account', { replace: true })
    },
  })
  const onSubmit = form.handleSubmit(({ current, next }) =>
    change.mutate({ body: { current_password: current, new_password: next } }),
  )
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('profile.passwordTitle')}</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={onSubmit} className="space-y-(--form-gap)">
          {/* A hidden user name lets password managers file the new password correctly. */}
          <input type="text" autoComplete="username" value={me.login} readOnly hidden />
          <Field label={t('profile.currentPassword')} htmlFor="me-current">
            <Input
              id="me-current"
              type="password"
              {...form.register('current')}
              autoComplete="current-password"
              required
            />
          </Field>
          <Field label={t('profile.newPassword')} htmlFor="me-new" hint={t('profile.passwordHint')}>
            <Input id="me-new" type="password" {...form.register('next')} autoComplete="new-password" required />
          </Field>
          <Field label={t('profile.repeatPassword')} htmlFor="me-repeat" error={form.formState.errors.repeat?.message}>
            <Input
              id="me-repeat"
              type="password"
              {...form.register('repeat', {
                validate: (value) => value === form.getValues('next') || t('profile.mismatch'),
              })}
              autoComplete="new-password"
              required
            />
          </Field>
          <SaveBar
            error={change.error}
            saved={
              change.isSuccess &&
              !form.formState.isDirty && (
                <Notice kind="success">
                  <p>{t('profile.passwordChanged')}</p>
                </Notice>
              )
            }
            pending={change.isPending}
            label={t('profile.passwordTitle')}
          />
        </form>
      </CardContent>
    </Card>
  )
}
