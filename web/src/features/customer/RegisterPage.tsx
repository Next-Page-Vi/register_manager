import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useLocation, useNavigate, useParams, useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { $api } from '@/api/client'
import { TextLink } from '@/components/blocks'
import { ActionButton, ErrorNotice, Field, Loading, Notice } from '@/components/common'
import { Page } from '@/components/layout'
import { Input } from '@/components/ui/input'
import { applyLocale, currentLocale } from '@/i18n'
import { joinTitle, useDocumentTitle } from '@/lib/useDocumentTitle'
import { stickyId } from '@/lib/uuid'
import { CustomerFrame } from './CustomerLayout'

/** Public registration from the instance's QR code. */
export function RegisterPage() {
  const { t } = useTranslation()
  const { slug } = useParams()
  if (!slug) throw new Error('Missing route parameter: slug')
  const [params] = useSearchParams()
  const invite = params.get('invite')
  const navigate = useNavigate()
  const inst = $api.useQuery('get', '/api/v1/public/instances/{slug}', { params: { path: { slug } } })
  const form = useForm({ defaultValues: { name: '', contact: '' } })

  const data = inst.status === 'success' ? inst.data : null
  useDocumentTitle(joinTitle(t('register.title'), data?.name))
  useEffect(() => {
    if (data) applyLocale({ instanceDefault: data.default_locale })
  }, [data])

  const submit = $api.useMutation('post', '/api/v1/public/instances/{slug}/registrations', {
    onSuccess: (r) => navigate(`/i/${slug}/register/done`, { replace: true, state: { status: r.status } }),
  })
  const onSubmit = form.handleSubmit(({ name, contact }) => {
    if (!invite) return
    const id = stickyId(`register:${slug}:${name.trim()}:${contact.trim()}`)
    submit.mutate({
      params: { path: { slug } },
      body: { id, invite, name: name.trim(), contact: contact.trim() || null, locale: currentLocale() },
    })
  })

  let body
  if (inst.status === 'pending') body = <Loading />
  else if (inst.status === 'error') body = <ErrorNotice error={inst.error} />
  else if (!invite)
    body = (
      <Notice kind="error">
        <p>{t('errors.invite_invalid')}</p>
      </Notice>
    )
  else if (data && data.status !== 'active')
    body = (
      <Notice kind="warning">
        <p>{t('register.paused')}</p>
      </Notice>
    )
  else if (data) {
    const kind = data.contact_kind
    body = (
      <Page title={t('register.title')}>
        <p>{t('register.intro')}</p>
        <form onSubmit={onSubmit} className="space-y-5" noValidate>
          <Field label={t('register.name')} htmlFor="name" error={form.formState.errors.name?.message}>
            <Input
              id="name"
              autoComplete="name"
              {...form.register('name', { validate: (value) => !!value.trim() || t('register.nameRequired') })}
              maxLength={100}
            />
          </Field>
          <Field
            label={
              data.contact_required
                ? t(`register.contact.${kind}`)
                : t('register.optional', { label: t(`register.contact.${kind}`) })
            }
            htmlFor="contact"
            hint={t(`register.contactHint.${kind}`)}
            error={form.formState.errors.contact?.message}
          >
            <Input
              id="contact"
              type={kind === 'email' ? 'email' : kind === 'phone' ? 'tel' : 'text'}
              autoComplete={kind === 'email' ? 'email' : kind === 'phone' ? 'tel' : 'off'}
              inputMode={kind === 'phone' ? 'tel' : undefined}
              {...form.register('contact', {
                validate: (value) => !data.contact_required || !!value.trim() || t('errors.contact_required'),
              })}
              maxLength={200}
            />
          </Field>
          <ErrorNotice error={submit.error} />
          <ActionButton type="submit" size="lg" className="w-full" pending={submit.isPending}>
            {t('register.submit')}
          </ActionButton>
        </form>
      </Page>
    )
  }
  return (
    <CustomerFrame title={data?.name} brand={data?.brand_color}>
      {body}
    </CustomerFrame>
  )
}

export function RegisterDonePage() {
  const { t } = useTranslation()
  const location = useLocation()
  const status = (location.state as { status?: string } | null)?.status
  useDocumentTitle(t('register.done.title'))
  return (
    <CustomerFrame>
      <Page>
        <Notice kind="success" title={t('register.done.title')}>
          <p>{status === 'approved' ? t('register.done.alreadyApproved') : t('register.done.body')}</p>
        </Notice>
        <p>{t('register.done.next')}</p>
      </Page>
    </CustomerFrame>
  )
}

export function LinkInvalidPage() {
  const { t } = useTranslation()
  const [params] = useSearchParams()
  const reason = params.get('reason')
  useDocumentTitle(t('customer.linkInvalid.title'))
  return (
    <CustomerFrame>
      <Notice kind="error" title={t('customer.linkInvalid.title')}>
        <p>{reason === 'rate_limited' ? t('errors.rate_limited') : t('customer.linkInvalid.body')}</p>
      </Notice>
    </CustomerFrame>
  )
}

export function NotFoundPage() {
  const { t } = useTranslation()
  useDocumentTitle(t('common.notFound'))
  return (
    <CustomerFrame>
      <Notice kind="info" title={t('common.notFound')}>
        <TextLink to="/">{t('common.backHome')}</TextLink>
      </Notice>
    </CustomerFrame>
  )
}
