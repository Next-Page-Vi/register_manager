import { createContext, useContext, useEffect } from 'react'
import { Outlet, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { $api, errorInfo, UNAUTHENTICATED_EVENT, type SessionScope, type Schemas } from '@/api/client'
import { ErrorNotice, Loading, Notice } from '@/components/common'
import { LanguageSwitcher } from '@/components/LanguageSwitcher'
import { applyLocale } from '@/i18n'
import { isHexColor } from '@/lib/color'
import { CustomerHeader } from '@/components/CustomerHeader'
import { cn } from '@/lib/utils'
import { deviceMatchesZone, formatZoneName } from '@/lib/datetime'
import { Surface } from '@/components/Surface'

type CustomerMe = Schemas['CustomerMe']

const Ctx = createContext<{ me: CustomerMe } | null>(null)

export function useCustomer() {
  const v = useContext(Ctx)
  if (!v) throw new Error('useCustomer outside CustomerLayout')
  return v
}

/** Uses the page colour for the browser's toolbar while mounted. */
function useThemeColor(brand: string | null | undefined) {
  useEffect(() => {
    if (!isHexColor(brand)) return
    const metas = [...document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')]
    const saved = metas.map((m) => m.content)
    metas.forEach((m) => (m.content = brand))
    return () => metas.forEach((m, i) => (m.content = saved[i]))
  }, [brand])
}

/**
 * Frame for public customer pages: instance name and language button. With
 * timeZone the language dialog also says which zone the times are in.
 */
export function CustomerFrame({
  title,
  brand,
  timeZone,
  children,
  onSaveLocale,
}: {
  title?: string
  brand?: string | null
  timeZone?: string
  children: React.ReactNode
  onSaveLocale?: (l: string) => void
}) {
  useThemeColor(brand)
  return (
    <Surface kind="customer" className="min-h-dvh">
      <CustomerHeader title={title} brand={brand}>
        <LanguageSwitcher onSave={onSaveLocale} timeZone={timeZone} />
      </CustomerHeader>
      <main className="mx-auto max-w-[47.25rem] px-4 py-6">{children}</main>
    </Surface>
  )
}

const STEPS = ['choose', 'confirm', 'done'] as const

/**
 * Where the customer is in the three booking steps, as text (current step
 * in bold, marked for screen readers).
 */
export function StepIndicator({ current }: { current: (typeof STEPS)[number] }) {
  const { t } = useTranslation()
  const currentIndex = STEPS.indexOf(current)
  return (
    <ol aria-label={t('customer.steps.label')} className="grid grid-cols-3 gap-3">
      {STEPS.map((s, i) => (
        <li
          key={s}
          aria-current={s === current ? 'step' : undefined}
          className={cn('min-w-0 space-y-2 text-base leading-snug', s === current ? 'text-primary font-semibold' : 'text-muted-foreground')}
        >
          <span aria-hidden className={cn('block h-1 rounded-full', i <= currentIndex ? 'bg-primary' : 'bg-border')} />
          <span className="block">{t('customer.steps.item', { n: i + 1, name: t(`customer.steps.${s}`) })}</span>
        </li>
      ))}
    </ol>
  )
}

/**
 * Loads the customer of the instance in the URL. A browser may be signed in
 * to several instances; without an identity for this one the customer is
 * asked to open their personal link again. An instance that no longer exists
 * (deleted, or a wrong address) shows the same message as an invalid link.
 */
export function CustomerLayout() {
  const { slug } = useParams()
  if (!slug) throw new Error('Missing route parameter: slug')
  const { t } = useTranslation()
  const query = $api.useQuery('get', '/api/v1/customer/{slug}/me', { params: { path: { slug } } })

  const me = query.data?.customer ?? null
  useEffect(() => {
    if (me) applyLocale({ profile: me.locale, instanceDefault: me.instance.default_locale })
  }, [me])

  const locale = $api.useMutation('put', '/api/v1/customer/{slug}/locale')
  const saveLocale = (value: string) => locale.mutate({ params: { path: { slug } }, body: { locale: value } })
  const refetch = query.refetch
  useEffect(() => {
    const onUnauth = (event: Event) => {
      const scope = (event as CustomEvent<SessionScope>).detail
      if (scope.kind === 'customer' && scope.slug === slug) void refetch()
    }
    window.addEventListener(UNAUTHENTICATED_EVENT, onUnauth)
    return () => window.removeEventListener(UNAUTHENTICATED_EVENT, onUnauth)
  }, [slug, refetch])

  if (query.status === 'pending') {
    return (
      <CustomerFrame>
        <Loading />
      </CustomerFrame>
    )
  }
  if (query.status === 'error' && query.data === undefined) {
    return (
      <CustomerFrame>
        {errorInfo(query.error).code === 'instance_not_found' ? (
          <Notice kind="error" title={t('customer.linkInvalid.title')}>
            <p>{t('customer.linkInvalid.body')}</p>
          </Notice>
        ) : (
          <ErrorNotice error={query.error} />
        )}
      </CustomerFrame>
    )
  }
  if (!me || me.instance.slug !== slug) {
    return (
      <CustomerFrame>
        <Notice kind="info" title={t('customer.needLink.title')}>
          <p>{t('customer.needLink.body')}</p>
        </Notice>
      </CustomerFrame>
    )
  }
  return (
    <Ctx.Provider value={{ me }}>
      <CustomerFrame
        title={me.instance.name}
        brand={me.instance.brand_color}
        timeZone={me.instance.timezone}
        onSaveLocale={saveLocale}
      >
        {/* Times are always in the instance's zone; say so when this device differs. */}
        {!deviceMatchesZone(me.instance.timezone) && (
          <Notice kind="info" className="mb-6">
            <p>{t('customer.zoneNotice', { zone: formatZoneName(me.instance.timezone) })}</p>
          </Notice>
        )}
        <ErrorNotice error={locale.error} />
        <Outlet />
      </CustomerFrame>
    </Ctx.Provider>
  )
}
