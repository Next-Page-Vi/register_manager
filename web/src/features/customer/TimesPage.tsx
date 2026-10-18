import { AvailabilityDateFilter, type AvailabilitySelection } from '@/components/AvailabilityDateFilter'
import { useNavigate, useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { $api } from '@/api/client'
import { LoadMore } from '@/components/blocks'
import { QueryContent, Notice } from '@/components/common'
import { Page } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { cursorOptions, resetAvailabilityPages } from '@/api/queries'
import { formatDate, formatTime, groupByDay } from '@/lib/datetime'
import { joinTitle, useDocumentTitle } from '@/lib/useDocumentTitle'
import { confirmPath, NoSlots } from './BookingHome'
import { StepIndicator, useCustomer } from './CustomerLayout'

const PAGE = 30

/** All bookable times grouped by day, nearest first, with "show more". */
export function TimesPage() {
  const { t } = useTranslation()
  const { me } = useCustomer()
  const tz = me.instance.timezone
  const slug = me.instance.slug
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const date = params.get('date') ?? undefined
  const selection: AvailabilitySelection = date === undefined ? { kind: 'nearest' } : { kind: 'date', date }
  useDocumentTitle(joinTitle(t('customer.times.title'), me.instance.name))

  const slots = $api.useInfiniteQuery(
    'get',
    '/api/v1/customer/{slug}/availability',
    {
      params: { path: { slug }, query: { limit: PAGE, date } },
    },
    cursorOptions,
  )
  const items = slots.data?.pages.flatMap((page) => page.items) ?? []

  const back = { to: `/i/${slug}/booking`, label: t('common.back') }

  if (me.instance.status !== 'active') {
    return (
      <Page back={back}>
        <Notice kind="warning">
          <p>{t('customer.home.paused')}</p>
        </Notice>
      </Page>
    )
  }

  return (
    <Page back={back} steps={<StepIndicator current="choose" />} title={t('customer.times.title')}>
      <AvailabilityDateFilter value={selection} range={slots.data?.pages[0].date_range} onChange={(next) => { resetAvailabilityPages(slug); setParams(next.kind === 'date' ? { date: next.date } : {}) }} />
      <QueryContent query={slots}>
        {(first) =>
          items.length === 0 ? (
            <NoSlots reason={first.pages[0].empty_reason} />
          ) : (
            groupByDay(items, tz).map((g) => (
              <section key={g.day} aria-label={formatDate(g.items[0].starts_at, tz)} className="space-y-3">
                <h2 className="sticky top-0 z-10 border-b bg-background py-3">{formatDate(g.items[0].starts_at, tz)}</h2>
                <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
                  {g.items.map((s) => (
                    <Button
                      key={s.starts_at}
                      variant="outline"
                      size="lg"
                      className="h-auto min-h-14 py-3 text-xl tabular-nums"
                      onClick={() => navigate(confirmPath(slug, s, date))}
                    >
                      {formatTime(s.starts_at, tz)}
                    </Button>
                  ))}
                </div>
              </section>
            ))
          )
        }
      </QueryContent>
      {slots.data && <LoadMore query={slots} label={t(date === undefined ? 'customer.times.more' : 'availability.moreOnDate')} large />}
      {slots.isError && <Button variant="outline" onClick={() => void slots.refetch()} pending={slots.isFetching}>{t('common.retry')}</Button>}
    </Page>
  )
}
