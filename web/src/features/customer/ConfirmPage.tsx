import { Link, useNavigate, useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { $api, errorInfo } from '@/api/client'
import { TextLink } from '@/components/blocks'
import { ActionButton, ErrorNotice, Notice } from '@/components/common'
import { Page } from '@/components/layout'
import { When } from '@/components/When'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { joinTitle, useDocumentTitle } from '@/lib/useDocumentTitle'
import { StepIndicator, useCustomer } from './CustomerLayout'

// Errors after which the customer should pick another time.
const PICK_AGAIN = new Set([
  'slot_taken',
  'slot_unavailable',
  'slot_blocked',
  'too_soon',
  'beyond_horizon',
  'daily_capacity_full',
  'duration_changed',
])

/**
 * Step 3: confirm. The idempotency key `k` comes from the URL, so refreshing
 * or going back and pressing confirm again returns the same booking.
 */
export function ConfirmPage() {
  const { t } = useTranslation()
  const { me } = useCustomer()
  const tz = me.instance.timezone
  const slug = me.instance.slug
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const start = params.get('start')
  const end = params.get('end')
  const key = params.get('k')
  const date = params.get('date')
  const timesPath = `/i/${slug}/booking/times${date === null ? '' : `?${new URLSearchParams({ date })}`}`
  useDocumentTitle(joinTitle(t('customer.confirm.title'), me.instance.name))

  const book = $api.useMutation('post', '/api/v1/customer/{slug}/appointments', {
    onSuccess: (a) => navigate(`/i/${slug}/booking/done/${a.id}`, { replace: true }),
  })

  if (!start || !end || !key || Number.isNaN(Date.parse(start))) {
    return (
      <Notice kind="error">
        <p>{t('customer.confirm.invalid')}</p>
        <TextLink to={`/i/${slug}/booking`}>{t('common.backHome')}</TextLink>
      </Notice>
    )
  }

  const minutes = Math.round((Date.parse(end) - Date.parse(start)) / 60000)
  const pickAgain = book.error && PICK_AGAIN.has(errorInfo(book.error).code)

  return (
    <Page
      back={{ to: timesPath, label: t('customer.confirm.changeTime') }}
      steps={<StepIndicator current="confirm" />}
      title={t('customer.confirm.title')}
    >
      <Card className="border-primary border-2">
        <CardContent className="space-y-2">
          <When start={start} end={end} tz={tz} size="lg" />
          <p className="text-muted-foreground">{t('customer.confirm.duration', { count: minutes })}</p>
        </CardContent>
      </Card>

      <ErrorNotice error={book.error} />
      {pickAgain ? (
        <Button asChild size="lg" className="w-full">
          <Link to={timesPath}>{t('customer.confirm.pickAnother')}</Link>
        </Button>
      ) : (
        <ActionButton
          size="lg"
          className="w-full text-xl"
          onClick={() => book.mutate({ params: { path: { slug } }, body: { id: key, starts_at: start, ends_at: end } })}
          pending={book.isPending}
        >
          {t('customer.confirm.button')}
        </ActionButton>
      )}
    </Page>
  )
}
