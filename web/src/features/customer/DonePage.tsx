import { CheckCircle2 } from 'lucide-react'
import { Link, useParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { $api } from '@/api/client'
import { QueryContent } from '@/components/common'
import { Page } from '@/components/layout'
import { When } from '@/components/When'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { joinTitle, useDocumentTitle } from '@/lib/useDocumentTitle'
import { StepIndicator, useCustomer } from './CustomerLayout'

/** Result page. Refreshing only reads the booking; it never books again. */
export function DonePage() {
  const { t } = useTranslation()
  const { me } = useCustomer()
  const tz = me.instance.timezone
  const { appointmentId } = useParams()
  if (!appointmentId) throw new Error('Missing route parameter: appointmentId')
  const query = $api.useQuery('get', '/api/v1/customer/{slug}/appointments/{appointment_id}', {
    params: { path: { slug: me.instance.slug, appointment_id: appointmentId } },
  })
  const booked = query.status === 'success' && query.data.status === 'booked'
  useDocumentTitle(joinTitle(booked ? t('customer.done.title') : null, me.instance.name))
  return (
    <QueryContent query={query}>
      {(a) => (
        <Page steps={booked && <StepIndicator current="done" />}>
          <div
            role="status"
            className={cn(
              'space-y-6 rounded-xl border-2 p-6 sm:p-8',
              booked ? 'border-success bg-success/10' : 'border-input',
            )}
          >
            <h1 className="flex items-center gap-4 text-3xl">
              {booked && <CheckCircle2 aria-hidden className="text-success size-12 shrink-0" />}
              {booked ? t('customer.done.title') : t(`status.appointment.${a.status}`)}
            </h1>
            <When start={a.starts_at} end={a.ends_at} tz={tz} size="lg" />
          </div>
          {booked && <p className="text-lg">{t('customer.done.arrival')}</p>}
          <Button asChild size="lg" className="w-full">
            <Link to={`/i/${me.instance.slug}/booking`}>{t('common.backHome')}</Link>
          </Button>
        </Page>
      )}
    </QueryContent>
  )
}
