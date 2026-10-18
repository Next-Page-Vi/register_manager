import { useForm } from 'react-hook-form'
import { CalendarCheck } from 'lucide-react'
import { useNavigate, useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { $api } from '@/api/client'
import { ActionButton, ErrorNotice, Field } from '@/components/common'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { applyLocale } from '@/i18n'
import { joinTitle, useDocumentTitle } from '@/lib/useDocumentTitle'
import { Surface } from '@/components/Surface'

export function LoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const form = useForm({ defaultValues: { login: '', password: '' } })
  useDocumentTitle(joinTitle(t('login.title'), t('app.name')))

  const submit = $api.useMutation('post', '/api/v1/account/login', {
    onSuccess: (me) => {
      applyLocale({ profile: me.locale })
      const next = params.get('next')
      navigate(next && next.startsWith('/') && !next.startsWith('//') ? next : '/account', { replace: true })
    },
  })
  const onSubmit = form.handleSubmit(({ login, password }) =>
    submit.mutate({ body: { login: login.trim(), password } }),
  )

  return (
    <Surface kind="staff" className="relative flex min-h-dvh flex-col items-center justify-center gap-6 bg-canvas px-4 pt-20 pb-8">
      <LanguageSwitcher className="absolute end-4 top-4" />
      <p className="text-primary inline-flex items-center gap-2 text-2xl font-bold">
        <CalendarCheck aria-hidden className="size-8" />
        {t('app.name')}
      </p>
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle className="text-2xl">{t('login.title')}</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={onSubmit} className="space-y-(--form-gap)">
            <Field label={t('login.login')} htmlFor="login">
              <Input id="login" autoComplete="username" {...form.register('login')} required />
            </Field>
            <Field label={t('login.password')} htmlFor="password">
              <Input
                id="password"
                {...form.register('password')}
                type="password"
                autoComplete="current-password"
                required
              />
            </Field>
            <ErrorNotice error={submit.error} />
            <ActionButton type="submit" size="lg" className="w-full" pending={submit.isPending}>
              {t('login.submit')}
            </ActionButton>
          </form>
        </CardContent>
      </Card>
    </Surface>
  )
}
