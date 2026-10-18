import { Controller, useForm } from 'react-hook-form'
import { Navigate, useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from 'recharts'
import { $api, type Schemas } from '@/api/client'
import { EmptyState, Field, QueryContent } from '@/components/common'
import { Page } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardTitle } from '@/components/ui/card'
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from '@/components/ui/chart'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { addDays, formatCivilShort, todayIn } from '@/lib/datetime'
import { useInstance } from './ManageLayout'

type Statistics = Schemas['Statistics']
type Basis = Statistics['basis']
type Filter = { from: string; to: string; basis: Basis }
const STATUSES = ['booked', 'completed', 'cancelled', 'no_show'] as const
const DATE_FIELDS = ['from', 'to'] as const
const CHART_MARGIN = { top: 16, right: 16, left: 0, bottom: 4 }
const AXIS_STYLE = {
  axisLine: { stroke: 'var(--muted-foreground)' },
  tick: { fill: 'var(--muted-foreground)' },
  tickLine: false,
  tickSize: 0,
  tickMargin: 8,
}
const colors = {
  booked: 'var(--chart-booked)', completed: 'var(--chart-completed)',
  cancelled: 'var(--chart-cancelled)', no_show: 'var(--chart-no-show)',
}

function isDate(value: string | null): value is string {
  if (value === null || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return false
  const date = new Date(`${value}T00:00:00Z`)
  return !Number.isNaN(date.getTime()) && date.toISOString().slice(0, 10) === value
}

function DateRangeForm({ filter, onChange }: { filter: Filter; onChange: (filter: Filter) => void }) {
  const { t } = useTranslation()
  const form = useForm<{ from: string | null; to: string | null }>({
    defaultValues: { from: isDate(filter.from) ? filter.from : null, to: isDate(filter.to) ? filter.to : null },
  })
  return (
    <form className="control-row" onSubmit={form.handleSubmit(({ from, to }) => {
      if (!isDate(from) || !isDate(to)) return
      if (to < from || (Date.parse(to) - Date.parse(from)) / 86400_000 >= 366) {
        form.setError('to', { message: t('errors.invalid_statistics_range') })
        return
      }
      onChange({ ...filter, from, to })
    })}>
      {DATE_FIELDS.map((name) => (
        <Controller key={name} control={form.control} name={name} rules={{ validate: (value) => isDate(value) || t('errors.invalid_date') }} render={({ field, fieldState }) => (
          <Field htmlFor={`statistics-${name}`} label={t(`manage.statistics.${name}`)} error={fieldState.error?.message}>
            <Input id={`statistics-${name}`} type="date" required value={field.value ?? ''} onBlur={field.onBlur} ref={field.ref} name={field.name} onChange={(e) => field.onChange(e.target.value || null)} />
          </Field>
        )} />
      ))}
      <Button type="submit" variant="secondary">{t('manage.statistics.apply')}</Button>
    </form>
  )
}

function bucketLabel(bucket: Pick<Schemas['StatisticsBucket'], 'from' | 'to'>) {
  return bucket.from === bucket.to ? formatCivilShort(bucket.from) : `${formatCivilShort(bucket.from)} – ${formatCivilShort(bucket.to)}`
}

function StatisticsData({ data }: { data: Statistics }) {
  const { t, i18n } = useTranslation()
  const number = new Intl.NumberFormat(i18n.language)
  const percent = new Intl.NumberFormat(i18n.language, { style: 'percent', maximumFractionDigits: 1 })
  const config = Object.fromEntries(STATUSES.map((status) => [status, { label: t(`status.appointment.${status}`), color: colors[status] }])) satisfies ChartConfig
  const series = data.series.map((bucket) => ({ ...bucket.counts, label: bucketLabel(bucket), from: bucket.from }))
  const distribution = STATUSES.map((status) => ({
    status, label: t(`status.appointment.${status}`), count: data.summary[status], fill: colors[status],
  }))
  const summary = [
    { key: 'total', label: t('manage.statistics.total'), value: data.summary.total },
    { key: 'customers', label: t('manage.statistics.customers'), value: data.summary.unique_customers },
    { key: 'completed', label: t('status.appointment.completed'), value: data.summary.completed },
    { key: 'cancelled', label: t('status.appointment.cancelled'), value: data.summary.cancelled },
  ]
  return (
    <>
      <Card className="grid gap-0 py-0 sm:py-0 xl:grid-cols-[minmax(0,3fr)_minmax(0,2fr)]">
        <section className="grid min-w-0 gap-3 p-4 xl:row-span-2 xl:grid-rows-subgrid">
          <div className="space-y-1">
            <CardTitle>{t('manage.statistics.trend')}</CardTitle>
            <p className="text-sm text-muted-foreground">{t(`manage.statistics.grain.${data.granularity}`)}</p>
          </div>
          <div className="space-y-2">
            {data.summary.total === 0 ? <EmptyState>{t('manage.statistics.empty')}</EmptyState> : (
              <ChartContainer config={config} className="h-52 w-full aspect-auto text-sm tabular-nums sm:h-60" role="group" aria-label={t('manage.statistics.trend')}>
                <BarChart accessibilityLayer data={series} margin={CHART_MARGIN}>
                  <CartesianGrid vertical={false} />
                  <XAxis {...AXIS_STYLE} dataKey="from" height="auto" tickFormatter={(value: string) => formatCivilShort(value)} minTickGap={28} />
                  <YAxis {...AXIS_STYLE} width="auto" allowDecimals={false} tickFormatter={(value: number) => number.format(value)} />
                  <ChartTooltip isAnimationActive={false} content={<ChartTooltipContent locale={i18n.language} labelFormatter={(value) => series.find((bucket) => bucket.from === value)?.label} />} />
                  {STATUSES.map((status) => <Bar key={status} dataKey={status} stackId="appointments" fill={colors[status]} isAnimationActive={false} maxBarSize={40} />)}
                </BarChart>
              </ChartContainer>
            )}
            <ul className="flex flex-wrap gap-x-3 gap-y-1 text-sm" aria-label={t('manage.statistics.legend')}>
              {STATUSES.map((status) => <li key={status} className="flex items-center gap-1.5"><span aria-hidden className="size-3 shrink-0 rounded-sm" style={{ backgroundColor: colors[status] }} />{t(`status.appointment.${status}`)}</li>)}
            </ul>
          </div>
        </section>
        <section className="grid min-w-0 gap-3 border-t p-4 xl:row-span-2 xl:grid-rows-subgrid xl:border-t-0 xl:border-l">
          <div className="space-y-1">
            <CardTitle>{t('manage.statistics.distribution')}</CardTitle>
            <p className="text-sm text-muted-foreground">{t('manage.statistics.shareHint')}</p>
          </div>
          <div>
            {data.summary.total === 0 ? <EmptyState>{t('manage.statistics.noShare')}</EmptyState> : (
              <ChartContainer config={config} className="h-52 w-full aspect-auto text-sm tabular-nums sm:h-60" role="group" aria-label={t('manage.statistics.distribution')}>
                <BarChart accessibilityLayer data={distribution} layout="vertical" margin={CHART_MARGIN}>
                  <CartesianGrid horizontal={false} />
                  <XAxis {...AXIS_STYLE} type="number" height="auto" allowDecimals={false} tickCount={3} tickFormatter={(value: number) => number.format(value)} />
                  <YAxis {...AXIS_STYLE} type="category" dataKey="label" width="auto" interval={0} />
                  <ChartTooltip isAnimationActive={false} content={<ChartTooltipContent locale={i18n.language} labelKey="status" formatter={(value) => (
                    typeof value === 'number' ? t('manage.statistics.countShare', {
                      value: number.format(value), share: percent.format(value / data.summary.total),
                    }) : value
                  )} />} />
                  <Bar dataKey="count" radius={4} isAnimationActive={false} maxBarSize={16} />
                </BarChart>
              </ChartContainer>
            )}
          </div>
        </section>
      </Card>
      <dl className="grid grid-cols-2 gap-px overflow-hidden rounded-xl border bg-border lg:grid-cols-4" aria-label={t('manage.statistics.summary')}>
        {summary.map((item) => (
          <div key={item.key} className="min-w-0 space-y-2 bg-card p-4 sm:p-5" data-statistic={item.key}>
            <dt className="text-sm text-muted-foreground">{item.label}</dt>
            <dd className="break-words text-2xl font-semibold tabular-nums">{number.format(item.value)}</dd>
          </div>
        ))}
      </dl>
      <p className="text-sm text-muted-foreground">{t('manage.statistics.currentStatus')}</p>
    </>
  )
}

function StatisticsTable({ data }: { data: Statistics }) {
  const { t, i18n } = useTranslation()
  const number = new Intl.NumberFormat(i18n.language)
  return (
    <details className="rounded-xl border bg-card p-4 sm:p-6">
      <summary className="cursor-pointer py-2 font-semibold">{t('manage.statistics.dataTable')}</summary>
      <div className="mt-4 sm:hidden">
        {data.series.map((bucket) => <section key={bucket.from} className="space-y-2 border-t py-4"><h3 className="font-semibold">{bucketLabel(bucket)}</h3><dl className="space-y-1 text-sm">{STATUSES.map((status) => <div key={status} className="flex justify-between gap-3"><dt>{t(`status.appointment.${status}`)}</dt><dd className="tabular-nums">{number.format(bucket.counts[status])}</dd></div>)}</dl></section>)}
      </div>
      <table className="mt-4 hidden w-full table-fixed text-sm sm:table">
        <caption className="sr-only">{t('manage.statistics.dataTable')}</caption>
        <thead><tr className="border-b"><th scope="col" className="w-1/3 py-3 pe-3 text-start">{t('manage.statistics.period')}</th>{STATUSES.map((status) => <th key={status} scope="col" className="px-1 py-3 text-end">{t(`status.appointment.${status}`)}</th>)}</tr></thead>
        <tbody>{data.series.map((bucket) => <tr key={bucket.from} className="border-b last:border-0"><th scope="row" className="py-3 pe-3 text-start font-normal">{bucketLabel(bucket)}</th>{STATUSES.map((status) => <td key={status} className="px-1 py-3 text-end tabular-nums">{number.format(bucket.counts[status])}</td>)}</tr>)}</tbody>
      </table>
    </details>
  )
}

function StatisticsReport({ filter, onChange }: { filter: Filter; onChange: (filter: Filter) => void }) {
  const { t } = useTranslation()
  const { path, tz } = useInstance()
  const today = todayIn(tz)
  const monthStart = `${today.slice(0, 7)}-01`
  const nextMonth = new Date(`${monthStart}T00:00:00Z`)
  nextMonth.setUTCMonth(nextMonth.getUTCMonth() + 1)
  const presets = [
    { key: 'last7', from: addDays(today, -6), to: today },
    { key: 'last30', from: addDays(today, -29), to: today },
    { key: 'month', from: monthStart, to: addDays(nextMonth.toISOString().slice(0, 10), -1) },
  ] as const
  const query = $api.useQuery('get', '/api/v1/instances/{instance_id}/statistics', { params: { path, query: filter } })
  return (
    <Page title={t('manage.statistics.title')} description={t('manage.statistics.description')}>
      <QueryContent query={query}>{(data) => <StatisticsData data={data} />}</QueryContent>
      {query.isError && <Button variant="outline" onClick={() => void query.refetch()} disabled={query.isFetching}>{t('common.retry')}</Button>}
      <div className="@container/form space-y-4 border-t pt-5">
        <Field htmlFor="statistics-basis" label={t('manage.statistics.basis')}>
          <NativeSelect id="statistics-basis" className="sm:max-w-sm" value={filter.basis} onChange={(e) => onChange({ ...filter, basis: e.target.value === 'created_at' ? 'created_at' : 'starts_at' })}>
            <option value="starts_at">{t('manage.statistics.startsAt')}</option>
            <option value="created_at">{t('manage.statistics.createdAt')}</option>
          </NativeSelect>
        </Field>
        <div className="flex flex-wrap gap-2" role="group" aria-label={t('manage.statistics.presets')}>
          {presets.map(({ key, from, to }) => <Button key={key} type="button" variant="outline" size="sm" selected={filter.from === from && filter.to === to} onClick={() => onChange({ ...filter, from, to })}>{t(`manage.statistics.${key}`)}</Button>)}
        </div>
        <DateRangeForm key={`${filter.from}/${filter.to}`} filter={filter} onChange={onChange} />
        <p className="text-sm text-muted-foreground">{t('manage.statistics.rangeHint', { timezone: tz })}</p>
        <p className="text-sm">{t(`manage.statistics.basisHint.${filter.basis}`)}</p>
      </div>
      {query.data !== undefined && <StatisticsTable data={query.data} />}
    </Page>
  )
}

export function StatisticsPage() {
  const { tz } = useInstance()
  const [params, setParams] = useSearchParams()
  const today = todayIn(tz)
  const filter: Filter = {
    from: params.get('from') ?? addDays(today, -29), to: params.get('to') ?? today,
    basis: params.get('basis') === 'created_at' ? 'created_at' : 'starts_at',
  }
  if (params.get('from') === null || params.get('to') === null || params.get('basis') !== filter.basis) {
    return <Navigate replace to={{ search: new URLSearchParams(filter).toString() }} />
  }
  return <StatisticsReport filter={filter} onChange={(value) => setParams(new URLSearchParams(value))} />
}
