import { useId, useState } from 'react'
import { ArrowRight, ChevronDown } from 'lucide-react'
import { Link, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { $api, type Schemas } from '@/api/client'
import { Row, RowList } from '@/components/blocks'
import { ConfirmButton } from '@/components/ConfirmButton'
import { QueryContent, EmptyState, ErrorNotice, Notice } from '@/components/common'
import { Page } from '@/components/layout'
import { When, whenText } from '@/components/When'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { useDocumentTitle } from '@/lib/useDocumentTitle'
import { cn } from '@/lib/utils'
import { newId } from '@/lib/uuid'
import { useCustomer } from './CustomerLayout'

type Appointment = Schemas['Appointment']
type Slot = Schemas['Slot']
type EmptyReason = Schemas['AvailabilityPage']['empty_reason']

/** Path of the confirmation page for a slot; k is the idempotency key. */
export function confirmPath(slug: string, s: Slot, date?: string) {
  const q = new URLSearchParams({ start: s.starts_at, end: s.ends_at, k: newId() })
  if (date !== undefined) q.set('date', date)
  return `/i/${slug}/booking/confirm?${q}`
}

/** Explains why there is nothing to book (see booking.EmptyReason on the server). */
export function NoSlots({ reason }: { reason: EmptyReason }) {
  const { t } = useTranslation()
  switch (reason) {
    case 'paused':
      return (
        <Notice kind="warning">
          <p>{t('customer.home.paused')}</p>
        </Notice>
      )
    case 'no_slots_on_date':
      return <EmptyState variant="plain">{t('availability.emptyDate')}</EmptyState>
    case 'full':
      return <EmptyState variant="plain">{t('customer.home.noSlotsFull')}</EmptyState>
    case 'customer_limit':
      return <EmptyState variant="plain">{t('customer.home.noSlotsCustomerLimit')}</EmptyState>
    case 'period_limit':
      return <EmptyState variant="plain">{t('customer.home.noSlotsPeriodLimit')}</EmptyState>
    default:
      return <EmptyState variant="plain">{t('customer.home.noSlots')}</EmptyState>
  }
}

function canCancel(a: Appointment, noticeMinutes: number) {
  return new Date(a.starts_at).getTime() - noticeMinutes * 60_000 >= Date.now()
}

export function BookingHome() {
  const { t } = useTranslation()
  const { me } = useCustomer()
  const tz = me.instance.timezone
  const slug = me.instance.slug
  const navigate = useNavigate()
  const [expanded, setExpanded] = useState(false)
  const otherAppointmentsId = useId()
  const appts = $api.useQuery('get', '/api/v1/customer/{slug}/appointments', { params: { path: { slug } } })
  const slots = $api.useQuery('get', '/api/v1/customer/{slug}/availability', {
    params: { path: { slug }, query: { limit: 3 } },
  })
  useDocumentTitle(me.instance.name)
  const cancel = $api.useMutation('post', '/api/v1/customer/{slug}/appointments/{appointment_id}/cancel')
  const cancellingId = cancel.isPending ? cancel.variables?.params.path.appointment_id : null

  const paused = me.instance.status !== 'active'

  const appointmentCard = (a: Appointment, next: boolean) => (
    <Card key={a.id} className="bg-muted/40 py-4 sm:py-5">
      <CardContent className="space-y-4">
        <div className="space-y-2">
          {next && <Badge size="md">{t('customer.home.next')}</Badge>}
          <When start={a.starts_at} end={a.ends_at} tz={tz} icons />
        </div>
        {canCancel(a, me.min_cancellation_notice_minutes) ? (
          <ConfirmButton
            variant="outline"
            confirmVariant="destructive"
            title={t('customer.cancel.title')}
            description={whenText(a.starts_at, a.ends_at, tz)}
            confirmLabel={t('customer.cancel.confirm')}
            onConfirm={() => cancel.mutateAsync({ params: { path: { slug, appointment_id: a.id } } })}
            error={cancel.error}
            onOpen={cancel.reset}
            pending={cancellingId === a.id}
            disabled={cancel.isPending && cancellingId !== a.id}
          >
            {cancellingId === a.id ? t('common.processing') : t('customer.cancel.button')}
          </ConfirmButton>
        ) : (
          <p className="text-muted-foreground">{t('customer.cancel.tooLate')}</p>
        )}
      </CardContent>
    </Card>
  )

  return (
    <Page title={t('customer.home.title')} description={t('customer.home.greeting', { name: me.name })}>
      <ErrorNotice error={cancel.error} />
      {cancel.isSuccess && (
        <Notice kind="success" title={t('customer.cancel.done')}>
          <p>{t('customer.cancel.doneBody')}</p>
        </Notice>
      )}

      <section aria-labelledby="next-heading" className="space-y-3">
        <h2 id="next-heading">{t('customer.home.yourAppointments')}</h2>
        <QueryContent query={appts}>
          {(data) => {
            const [next, ...others] = data.upcoming
            if (!next) return <EmptyState variant="plain">{t('customer.home.noAppointments')}</EmptyState>
            return (
              <>
                {appointmentCard(next, true)}
                {others.length > 0 && (
                  <>
                    <Button
                      variant="ghost"
                      className="w-full justify-between text-start"
                      aria-expanded={expanded}
                      aria-controls={otherAppointmentsId}
                      onClick={() => setExpanded((value) => !value)}
                    >
                      {expanded ? t('customer.home.hideOthers') : t('customer.home.otherAppointments', { count: others.length })}
                      <ChevronDown aria-hidden className={cn('transition-transform duration-150', expanded && 'rotate-180')} />
                    </Button>
                    <div id={otherAppointmentsId} hidden={!expanded} className="space-y-3">
                      {others.map((a) => appointmentCard(a, false))}
                    </div>
                  </>
                )}
              </>
            )
          }}
        </QueryContent>
      </section>

      <section aria-labelledby="slots-heading" className="space-y-3">
        <h2 id="slots-heading">{t('customer.home.earliest')}</h2>
        {paused ? (
          <Notice kind="warning">
            <p>{t('customer.home.paused')}</p>
          </Notice>
        ) : (
          <QueryContent query={slots}>
            {(data) => (
              <>
                {data.items.length === 0 ? (
                  <NoSlots reason={data.empty_reason} />
                ) : (
                  <div className="grid gap-3">
                    {data.items.map((s, i) => (
                      <Button
                        key={s.starts_at}
                        size="lg"
                        variant="outline"
                        className="h-auto flex-col items-stretch gap-3 px-4 py-4 text-start sm:flex-row sm:items-center sm:justify-between sm:px-6"
                        onClick={() => navigate(confirmPath(slug, s))}
                      >
                        <span className="flex flex-col items-start gap-1">
                          {i === 0 && <Badge size="md">{t('customer.home.earliestTag')}</Badge>}
                          <When start={s.starts_at} end={s.ends_at} tz={tz} />
                        </span>
                        <span className="text-primary inline-flex items-center gap-2">{t('customer.home.chooseTime')}<ArrowRight aria-hidden className="shrink-0" /></span>
                      </Button>
                    ))}
                  </div>
                )}
                <Button asChild size="lg" variant="secondary" className="w-full">
                  <Link to={`/i/${slug}/booking/times`}>{t('customer.home.moreTimes')}</Link>
                </Button>
              </>
            )}
          </QueryContent>
        )}
      </section>

      {appts.status === 'success' && appts.data.recent.length > 0 && (
        <section aria-labelledby="recent-heading" className="space-y-3">
          <h2 id="recent-heading">{t('customer.home.recent')}</h2>
          <RowList>
            {appts.data.recent.slice(0, 5).map((a) => (
              <Row key={a.id} end={<span className="font-semibold">{t(`status.appointment.${a.status}`)}</span>}>
                {whenText(a.starts_at, a.ends_at, tz)}
              </Row>
            ))}
          </RowList>
        </section>
      )}
    </Page>
  )
}
