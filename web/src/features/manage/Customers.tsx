import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useParams, useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { $api, type Schemas } from '@/api/client'
import { ConfirmButton } from '@/components/ConfirmButton'
import { DescriptionList, ItemCard, Row, RowList, SaveBar, TextLink } from '@/components/blocks'
import { QueryContent, EmptyState, ErrorNotice, Field, Notice } from '@/components/common'
import { Page } from '@/components/layout'
import { whenText } from '@/components/When'
import { QrLink } from '@/components/QrLink'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { formatDateTime } from '@/lib/datetime'
import { useInstance } from './ManageLayout'
import { appointmentStatusBadge, customerStatusBadge } from '@/lib/status'

type Customer = Schemas['Customer']
type IssuedLink = { path: string; name: string }
type Action = 'approve' | 'reject' | 'disable' | 'restore' | 'regenerate_link'
type StatusFilter = 'all' | 'pending' | 'approved' | 'rejected' | 'disabled'

const FILTERS: StatusFilter[] = ['pending', 'approved', 'rejected', 'disabled', 'all']

const ACTIONS: Record<Customer['status'], Action[]> = {
  pending: ['approve', 'reject'],
  approved: ['regenerate_link', 'disable'],
  rejected: ['approve'],
  disabled: ['restore'],
}

/** Shows a freshly issued personal link once, as text and QR code. */
function LinkDialog({ value, onClose }: { value: IssuedLink | null; onClose: () => void }) {
  const { t } = useTranslation()
  return (
    <Dialog open={value !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('manage.customers.linkTitle', { name: value?.name })}</DialogTitle>
          <DialogDescription>{t('manage.customers.linkOnce')}</DialogDescription>
        </DialogHeader>
        {value && (
          <QrLink
            url={window.location.origin + value.path}
            fileName={`link-${value.name}.png`}
            label={t('manage.customers.link')}
          />
        )}
        <Button onClick={onClose}>{t('common.done')}</Button>
      </DialogContent>
    </Dialog>
  )
}

/** Status action buttons for a customer; destructive ones ask first. */
function CustomerActions({ customer, onLink }: { customer: Customer; onLink: (value: IssuedLink) => void }) {
  const { t } = useTranslation()
  const { id } = useInstance()
  const act = $api.useMutation('post', '/api/v1/instances/{instance_id}/customers/{customer_id}/actions/{action}', {
    onSuccess: (r) => {
      if (r.access_path) onLink({ path: r.access_path, name: r.customer.name })
    },
  })
  const variables = (action: Action) => ({ params: { path: { instance_id: id, customer_id: customer.id, action } } })
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap gap-2">
        {ACTIONS[customer.status].map((a) =>
          a === 'approve' || a === 'restore' ? (
            // Approving is the main action only for a customer who is waiting.
            <Button
              key={a}
              size="sm"
              variant={customer.status === 'pending' ? 'default' : 'outline'}
              onClick={() => act.mutate(variables(a))}
              disabled={act.isPending}
            >
              {t(`manage.customers.actions.${a}`)}
            </Button>
          ) : (
            <ConfirmButton
              key={a}
              size="sm"
              variant={a === 'disable' || a === 'reject' ? 'destructive-outline' : 'outline'}
              title={t(`manage.customers.actions.${a}`)}
              description={t(`manage.customers.confirm.${a}`, { name: customer.name })}
              confirmLabel={t(`manage.customers.actions.${a}`)}
              onConfirm={() => act.mutateAsync(variables(a))}
              pending={act.isPending}
              error={act.error}
              onOpen={act.reset}
              disabled={act.isPending}
            >
              {t(`manage.customers.actions.${a}`)}
            </ConfirmButton>
          ),
        )}
      </div>
      <ErrorNotice error={act.error} />
    </div>
  )
}

export function Customers() {
  const { t } = useTranslation()
  const { tz, access, path } = useInstance()
  const [params, setParams] = useSearchParams()
  const [link, setLink] = useState<IssuedLink | null>(null)
  const requestedStatus = params.get('status')
  const status = FILTERS.find((value) => value === requestedStatus) ?? 'pending'
  const query = $api.useQuery('get', '/api/v1/instances/{instance_id}/customers', {
    params: { path, query: { status: status === 'all' ? undefined : status } },
  })
  return (
    <Page title={t('manage.customers.title')}>
      <LinkDialog value={link} onClose={() => setLink(null)} />
      <Tabs value={status} onValueChange={(v) => setParams({ status: v })}>
        <TabsList>
          {FILTERS.map((s) => (
            <TabsTrigger key={s} value={s}>
              {s === 'all' ? t('manage.customers.all') : t(`status.customer.${s}`)}
            </TabsTrigger>
          ))}
        </TabsList>
        {/* The list is the selected tab's panel, so the tab announces what it controls. */}
        <TabsContent value={status} className="space-y-3">
          <QueryContent query={query}>
            {(data) =>
              data.customers.length === 0 ? (
                <EmptyState>{t('manage.customers.empty')}</EmptyState>
              ) : (
                data.customers.map((c) => (
                  <ItemCard key={c.id} actions={<CustomerActions customer={c} onLink={setLink} />}>
                    <TextLink
                      variant="title"
                      className="inline-flex min-h-11 items-center text-lg"
                      to={`/manage/${access.instance.slug}/customers/${c.id}`}
                    >
                      {c.name}
                    </TextLink>
                    {c.contact && <p>{c.contact}</p>}
                    <p className="text-muted-foreground">
                      {t('manage.customers.registered', { when: formatDateTime(c.created_at, tz) })}
                    </p>
                    <Badge variant={customerStatusBadge[c.status]}>{t(`status.customer.${c.status}`)}</Badge>
                  </ItemCard>
                ))
              )
            }
          </QueryContent>
        </TabsContent>
      </Tabs>
    </Page>
  )
}

export function CustomerDetail() {
  const { t } = useTranslation()
  const { id, tz, access } = useInstance()
  const [link, setLink] = useState<IssuedLink | null>(null)
  const { customerId } = useParams()
  if (!customerId) throw new Error('Missing route parameter: customerId')
  const query = $api.useQuery('get', '/api/v1/instances/{instance_id}/customers/{customer_id}', {
    params: { path: { instance_id: id, customer_id: customerId } },
  })
  const back = {
    to: `/manage/${access.instance.slug}/customers${query.status === 'success' ? `?status=${query.data.status}` : ''}`,
    label: t('manage.nav.customers'),
  }
  if (query.data === undefined)
    return (
      <Page back={back}>
        <QueryContent query={query}>{() => null}</QueryContent>
      </Page>
    )
  const c = query.data
  return (
    <Page back={back} title={c.name}>
      <LinkDialog value={link} onClose={() => setLink(null)} />
      <ErrorNotice error={query.error} />
      <Card>
        <CardContent className="space-y-4">
          <DescriptionList
            items={[
              [t('manage.customers.contact'), c.contact ?? t('common.none')],
              [
                t('manage.customers.status'),
                <Badge variant={customerStatusBadge[c.status]}>{t(`status.customer.${c.status}`)}</Badge>,
              ],
              [t('manage.customers.registeredAt'), formatDateTime(c.created_at, tz)],
            ]}
          />
          <CustomerActions customer={c} onLink={setLink} />
        </CardContent>
      </Card>
      <CustomerNote key={c.id} customer={c} />
      <section className="space-y-3">
        <h2>{t('manage.customers.history')}</h2>
        {c.appointments.length === 0 ? (
          <EmptyState>{t('manage.appointments.empty')}</EmptyState>
        ) : (
          <RowList>
            {c.appointments.map((a) => (
              <Row
                key={a.id}
                end={<Badge variant={appointmentStatusBadge[a.status]}>{t(`status.appointment.${a.status}`)}</Badge>}
              >
                {whenText(a.starts_at, a.ends_at, tz)}
              </Row>
            ))}
          </RowList>
        )}
      </section>
    </Page>
  )
}

/** The loaded note is the initial value; refetches never replace an edit. */
function CustomerNote({ customer }: { customer: Customer }) {
  const { t } = useTranslation()
  const { id } = useInstance()
  const form = useForm({ defaultValues: { note: customer.internal_note ?? '' } })
  const save = $api.useMutation('put', '/api/v1/instances/{instance_id}/customers/{customer_id}/note', {
    onSuccess: (value) => form.reset({ note: value.internal_note ?? '' }),
  })
  const onSubmit = form.handleSubmit(({ note }) =>
    save.mutate({
      params: { path: { instance_id: id, customer_id: customer.id } },
      body: { note: note.trim() || null },
    }),
  )
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('manage.customers.note')}</CardTitle>
      </CardHeader>
      <CardContent>
        <form onSubmit={onSubmit} className="space-y-4">
          <Notice kind="info">
            <p>{t('manage.customers.noteHint')}</p>
          </Notice>
          <Field label={t('manage.customers.note')} htmlFor="note" hideLabel>
            <Textarea id="note" rows={4} maxLength={2000} {...form.register('note')} />
          </Field>
          <SaveBar error={save.error} saved={save.isSuccess && !form.formState.isDirty} pending={save.isPending} />
        </form>
      </CardContent>
    </Card>
  )
}
