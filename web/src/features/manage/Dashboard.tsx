import { useTranslation } from 'react-i18next'
import { $api } from '@/api/client'
import { TextLink } from '@/components/blocks'
import { QueryContent, EmptyState, Loading, Notice } from '@/components/common'
import { Page } from '@/components/layout'
import { When } from '@/components/When'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { AppointmentRow } from './Appointments'
import { useInstance } from './ManageLayout'

export function Dashboard() {
  const { t, i18n } = useTranslation()
  const { access, tz, path } = useInstance()
  const slug = access.instance.slug
  const query = $api.useQuery('get', '/api/v1/instances/{instance_id}/dashboard', { params: { path } })
  return (
    <Page title={t('manage.dashboard.title')}>
      {access.instance.status !== 'active' && (
        <Notice kind="warning" title={t('manage.dashboard.pausedTitle')}>
          <p>{t('manage.dashboard.pausedBody')}</p>
          <TextLink to={`/manage/${slug}/settings`}>{t('manage.nav.settings')}</TextLink>
        </Notice>
      )}
      <QueryContent query={query} pendingFallback={<Loading layout="summary" />}>
        {(data) => (
          <>
            <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
              <Card className="gap-4">
                <CardHeader>
                  <CardTitle className="text-base text-muted-foreground">{t('manage.dashboard.next')}</CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                  {data.next ? (
                    <>
                      <p className="text-xl font-semibold leading-snug">{data.next.customer_name}</p>
                      <When start={data.next.starts_at} end={data.next.ends_at} tz={tz} size="sm" />
                    </>
                  ) : (
                    <p className="text-muted-foreground">{t('manage.dashboard.noNext')}</p>
                  )}
                </CardContent>
              </Card>
              <dl className="grid grid-cols-2 divide-x rounded-xl border bg-card xl:grid-cols-1 xl:divide-x-0 xl:divide-y">
                <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 p-4 sm:p-5">
                  <dt className="min-w-0 text-muted-foreground">{t('manage.dashboard.remaining')}</dt>
                  <dd className="text-2xl font-semibold tabular-nums">{new Intl.NumberFormat(i18n.language).format(data.remaining_today)}</dd>
                </div>
                <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 p-4 sm:p-5">
                  <dt className="min-w-0 text-muted-foreground">{t('manage.dashboard.pending')}</dt>
                  <dd className="text-2xl font-semibold tabular-nums">{new Intl.NumberFormat(i18n.language).format(data.pending_customers)}</dd>
                  {data.pending_customers > 0 && (
                    <dd className="w-full">
                      <TextLink variant="action" to={`/manage/${slug}/customers?status=pending`}>
                        {t('manage.dashboard.review')}
                      </TextLink>
                    </dd>
                  )}
                </div>
              </dl>
            </div>
            <section className="space-y-3">
              <h2>{t('manage.dashboard.today')}</h2>
              {data.today.length === 0 ? (
                <EmptyState>{t('manage.appointments.empty')}</EmptyState>
              ) : (
                data.today.map((a) => <AppointmentRow key={a.id} appt={a} showDate={false} />)
              )}
            </section>
          </>
        )}
      </QueryContent>
    </Page>
  )
}
