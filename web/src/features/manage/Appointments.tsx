import { useState } from 'react'
import { Controller, useForm, useWatch } from 'react-hook-form'
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { $api, type Schemas } from '@/api/client'
import { cursorOptions } from '@/api/queries'
import { ItemCard, LoadMore, TextLink } from '@/components/blocks'
import { ConfirmButton } from '@/components/ConfirmButton'
import { QueryContent, EmptyState, ErrorNotice, Field, Notice } from '@/components/common'
import { FormDialog } from '@/components/FormDialog'
import { Page } from '@/components/layout'
import { When, whenText } from '@/components/When'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { NativeSelect } from '@/components/ui/native-select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { appointmentStatusBadge } from '@/lib/status'
import { newId } from '@/lib/uuid'
import { useInstance } from './ManageLayout'
import { AppointmentTimeEditor, appointmentTime, timeDraft, emptyTime, type AppointmentTimeDraft } from './AppointmentTimeEditor'

type Appointment = Schemas['ManagedAppointment']
type View = 'today' | 'upcoming' | 'history'

export function AppointmentRow({ appt, showDate = true }: { appt: Appointment; showDate?: boolean }) {
  const { t } = useTranslation()
  const { id, tz, access } = useInstance()
  const action = $api.useMutation('post', '/api/v1/instances/{instance_id}/appointments/{appointment_id}/status')
  const change = (status: 'cancelled' | 'completed' | 'no_show') => ({
    params: { path: { instance_id: id, appointment_id: appt.id } },
    body: { status },
  })
  const started = new Date(appt.starts_at).getTime() <= Date.now()
  const description = `${appt.customer_name} · ${whenText(appt.starts_at, appt.ends_at, tz)}`
  const actions = [
    ...(appt.status === 'booked' ? [
      <RescheduleDialog key="reschedule" appt={appt} />,
      <ConfirmButton
        key="cancel"
        variant="destructive-outline"
        size="sm"
        title={t('manage.appointments.cancelTitle')}
        description={description}
        confirmLabel={t('manage.appointments.cancel')}
        onConfirm={() => action.mutateAsync(change('cancelled'))}
        pending={action.isPending}
        error={action.error}
        onOpen={action.reset}
      >
        {t('manage.appointments.cancel')}
      </ConfirmButton>,
    ] : []),
    ...(started && appt.status !== 'cancelled' && appt.status !== 'completed' ? [
      <Button
        key="complete"
        size="sm"
        variant="secondary"
        onClick={() => action.mutate(change('completed'))}
        disabled={action.isPending}
      >
        {t('manage.appointments.markCompleted')}
      </Button>,
    ] : []),
    ...(started && appt.status !== 'cancelled' && appt.status !== 'no_show' ? [
      <ConfirmButton
        key="no-show"
        variant="outline"
        size="sm"
        title={t('manage.appointments.noShowTitle')}
        description={description}
        confirmLabel={t('manage.appointments.markNoShow')}
        onConfirm={() => action.mutateAsync(change('no_show'))}
        pending={action.isPending}
        error={action.error}
        onOpen={action.reset}
      >
        {t('manage.appointments.markNoShow')}
      </ConfirmButton>,
    ] : []),
  ]
  return (
    <ItemCard
      footer={action.error ? <ErrorNotice error={action.error} /> : null}
      actions={actions.length > 0 ? actions : null}
    >
      <When start={appt.starts_at} end={appt.ends_at} tz={tz} size="sm" showDate={showDate} />
      <p>
        <TextLink variant="title" to={`/manage/${access.instance.slug}/customers/${appt.customer_id}`}>
          {appt.customer_name}
        </TextLink>
      </p>
      <Badge variant={appointmentStatusBadge[appt.status]}>{t(`status.appointment.${appt.status}`)}</Badge>
    </ItemCard>
  )
}

function RescheduleDialog({ appt }: { appt: Appointment }) {
  const { t } = useTranslation()
  const { id, tz } = useInstance()
  const form = useForm<{ original: Appointment; time: AppointmentTimeDraft }>({
    defaultValues: { original: appt, time: timeDraft(appt) },
  })
  const mutation = $api.useMutation('post', '/api/v1/instances/{instance_id}/appointments/{appointment_id}/reschedule')
  return (
    <FormDialog
      wide
      trigger={
        <Button size="sm" variant="outline">
          {t('manage.appointments.reschedule')}
        </Button>
      }
      title={t('manage.appointments.reschedule')}
      description={`${appt.customer_name} · ${whenText(appt.starts_at, appt.ends_at, tz)}`}
      submitLabel={t('manage.appointments.moveHere')}
      pending={mutation.isPending}
      error={mutation.error}
      onOpen={() => {
        // The precondition belongs to this draft, even if a refetch updates appt.
        form.reset({ original: appt, time: timeDraft(appt) })
        mutation.reset()
      }}
      onSubmit={(close) =>
        form.handleSubmit(async ({ original, time }) => {
          const slot = appointmentTime(time, tz)
          if (!slot) return
          await mutation.mutateAsync({
            params: { path: { instance_id: id, appointment_id: appt.id } },
            body: {
              expected_starts_at: original.starts_at, expected_ends_at: original.ends_at,
              starts_at: slot.starts_at, ends_at: slot.ends_at,
            },
          })
          close()
        })()
      }
    >
      <Controller
        control={form.control}
        name="time"
        rules={{ validate: (time) => appointmentTime(time, tz) !== null || t('errors.invalid_appointment_time') }}
        render={({ field, fieldState }) => (
          <AppointmentTimeEditor instanceId={id} tz={tz} context={{ kind: 'reschedule', appointmentId: appt.id }} value={field.value} onChange={field.onChange} error={fieldState.error?.message} />
        )}
      />
    </FormDialog>
  )
}

function CustomerSelect({ value, onChange }: { value: string | null; onChange: (id: string | null) => void }) {
  const { t } = useTranslation()
  const { path } = useInstance()
  const customers = $api.useQuery('get', '/api/v1/instances/{instance_id}/customers', {
    params: { path, query: { status: 'approved' } },
  })
  return (
    <QueryContent query={customers}>
      {({ customers: list }) =>
        list.length === 0 ? (
          <Notice kind="info">
            <p>{t('manage.appointments.noApprovedCustomers')}</p>
          </Notice>
        ) : (
          <Field label={t('manage.appointments.customer')} htmlFor="new-booking-customer">
            <NativeSelect
              id="new-booking-customer"
              value={value ?? ''}
              onChange={(e) => onChange(e.target.value || null)}
              required
            >
              <option value="">{t('manage.appointments.chooseCustomer')}</option>
              {list.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                  {c.contact ? ` (${c.contact})` : ''}
                </option>
              ))}
            </NativeSelect>
          </Field>
        )
      }
    </QueryContent>
  )
}

function NewBookingDialog() {
  const { t } = useTranslation()
  const { id, tz, path } = useInstance()
  const form = useForm<{ customer: string | null; time: AppointmentTimeDraft; id: string }>({
    defaultValues: { customer: null, time: emptyTime(), id: newId() },
  })
  const customer = useWatch({ control: form.control, name: 'customer' })
  const mutation = $api.useMutation('post', '/api/v1/instances/{instance_id}/appointments')
  return (
    <FormDialog
      wide
      trigger={
        <Button>
          <Plus aria-hidden />
          {t('manage.appointments.new')}
        </Button>
      }
      title={t('manage.appointments.new')}
      submitLabel={t('manage.appointments.book')}
      pending={mutation.isPending}
      error={mutation.error}
      onOpen={() => {
        form.reset({ customer: null, time: emptyTime(), id: newId() })
        mutation.reset()
      }}
      onSubmit={(close) =>
        form.handleSubmit(async ({ customer, time, id }) => {
          const slot = appointmentTime(time, tz)
          if (!slot || !customer) return
          await mutation.mutateAsync({
            params: { path },
            body: { id, customer_id: customer, starts_at: slot.starts_at, ends_at: slot.ends_at },
          })
          close()
        })()
      }
    >
      <Controller
        control={form.control}
        name="customer"
        rules={{ required: true }}
        render={({ field }) => (
          <CustomerSelect
            value={field.value}
            onChange={field.onChange}
          />
        )}
      />
      <Controller
        control={form.control}
        name="time"
        rules={{ validate: (time) => appointmentTime(time, tz) !== null || t('errors.invalid_appointment_time') }}
        render={({ field, fieldState }) => (
          <>
            {customer && (
              <AppointmentTimeEditor instanceId={id} tz={tz} context={{ kind: 'new', customerId: customer }} value={field.value} onChange={field.onChange} error={fieldState.error?.message} />
            )}
          </>
        )}
      />
    </FormDialog>
  )
}

export function Appointments() {
  const { t } = useTranslation()
  const { path } = useInstance()
  const [view, setView] = useState<View>('today')
  const query = $api.useInfiniteQuery(
    'get',
    '/api/v1/instances/{instance_id}/appointments',
    { params: { path, query: { view } } },
    cursorOptions,
  )
  const items = query.data?.pages.flatMap((page) => page.items) ?? []
  return (
    <Page title={t('manage.appointments.title')} actions={<NewBookingDialog />}>
      <Tabs value={view} onValueChange={(value) => setView(value as View)}>
        <TabsList>
          <TabsTrigger value="today">{t('manage.appointments.today')}</TabsTrigger>
          <TabsTrigger value="upcoming">{t('manage.appointments.upcoming')}</TabsTrigger>
          <TabsTrigger value="history">{t('manage.appointments.history')}</TabsTrigger>
        </TabsList>
        <TabsContent value={view} className="space-y-3">
          <QueryContent query={query}>
            {() =>
              items.length === 0 ? (
                <EmptyState>{t('manage.appointments.empty')}</EmptyState>
              ) : (
                items.map((appointment) => <AppointmentRow key={appointment.id} appt={appointment} />)
              )
            }
          </QueryContent>
          <LoadMore query={query} />
        </TabsContent>
      </Tabs>
    </Page>
  )
}
