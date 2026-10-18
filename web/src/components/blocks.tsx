import type { ReactNode } from 'react'
import { ArrowRight } from 'lucide-react'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ActionButton, ErrorNotice, Notice } from '@/components/common'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { cn } from '@/lib/utils'
import { ActionGroup } from '@/components/ActionGroup'

/**
 * One record in a list of cards: what it is on the left, its actions on the
 * right (below on phones). footer holds what belongs to the whole card,
 * such as an error from one of its actions.
 */
export function ItemCard({
  children,
  actions,
  footer,
}: {
  children: ReactNode
  actions?: ReactNode
  footer?: ReactNode
}) {
  return (
    <Card className="@container/item gap-3 py-4">
      <CardContent className="grid items-start gap-4 @min-[45rem]/item:grid-cols-[minmax(0,1fr)_fit-content(24rem)]">
        <div className="min-w-0 space-y-1">{children}</div>
        {actions && <ActionGroup className="@min-[45rem]/item:justify-end">{actions}</ActionGroup>}
      </CardContent>
      {footer && <CardContent>{footer}</CardContent>}
    </Card>
  )
}

/** Compact rows; outer cards can provide the border instead. */
export function RowList({ children, borderless = false }: { children: ReactNode; borderless?: boolean }) {
  return <ul className={cn('divide-y bg-card', !borderless && 'rounded-lg border')}>{children}</ul>
}

/**
 * A row of RowList: an optional label column (the same width in every
 * list), the content, and actions or a status at the end.
 */
export function Row({ label, children, end }: { label?: ReactNode; children?: ReactNode; end?: ReactNode }) {
  return (
    <li className="flex flex-wrap items-center gap-x-4 gap-y-2 p-3">
      {label && <span className="w-full shrink-0 font-semibold sm:w-48">{label}</span>}
      <span className="min-w-0 max-w-full flex-[1_1_12rem]">{children}</span>
      {end && <div className="flex max-w-full flex-wrap items-center gap-2">{end}</div>}
    </li>
  )
}

/** Label and value pairs, side by side from small tablets up. */
export function DescriptionList({ items }: { items: [ReactNode, ReactNode][] }) {
  return (
    <dl className="grid gap-x-6 gap-y-3 sm:grid-cols-[auto_minmax(0,1fr)] sm:items-center sm:gap-y-1">
      {items.map(([k, v], i) => (
        <div key={i} className="grid min-w-0 gap-1 sm:contents">
          <dt className="text-muted-foreground">{k}</dt>
          <dd className="min-w-0">{v}</dd>
        </div>
      ))}
    </dl>
  )
}

/** A checkbox with its label; the whole row is the tap target. */
export function CheckboxField({
  checked,
  onCheckedChange,
  className,
  children,
}: {
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  className?: string
  children: ReactNode
}) {
  return (
    <label className={cn('flex min-h-11 items-center gap-3', className)}>
      <Checkbox checked={checked} onCheckedChange={(v) => onCheckedChange(v === true)} />
      {children}
    </label>
  )
}

/**
 * Links that are not buttons. inline: inside running text or a notice.
 * action: a next step ("Review →"). title: a record's name in a list,
 * underlined on hover only.
 */
export function TextLink({
  to,
  variant = 'inline',
  className,
  children,
}: {
  to: string
  variant?: 'inline' | 'action' | 'title'
  className?: string
  children: ReactNode
}) {
  return (
    <Link
      to={to}
      className={cn(
        'font-semibold underline-offset-4',
        variant === 'inline' && 'underline',
        variant === 'action' && 'text-primary inline-flex min-h-11 items-center gap-1 hover:underline',
        variant === 'title' && 'hover:underline',
        className,
      )}
    >
      {children}
      {variant === 'action' && <ArrowRight aria-hidden className="size-5" />}
    </Link>
  )
}

/** A name with a remove button, such as an operator of a calendar; long names wrap inside it. */
export function Chip({ children, remove }: { children: ReactNode; remove?: ReactNode }) {
  return (
    <span className={cn('bg-secondary inline-flex max-w-full items-center gap-1 rounded-lg ps-3', remove ? 'min-h-10 pe-1' : 'py-1 pe-3')}>
      <span className="min-w-0">{children}</span>
      {remove}
    </span>
  )
}

/**
 * A form's save area: its error, the "saved" message, and the save
 * button with optional other buttons (extra) and a note (such as "unsaved
 * changes"). Without onSave the button submits the surrounding form.
 * saved may be a custom notice instead of true. Sticky bars belong at the
 * top of an @container/form; very narrow forms keep them in normal flow.
 */
export function SaveBar({
  error,
  saved,
  pending,
  onSave,
  label,
  extra,
  note,
  disabled,
  sticky = false,
  dirty = false,
}: {
  error?: unknown
  saved?: boolean | ReactNode
  pending: boolean
  onSave?: () => void
  label?: ReactNode
  extra?: ReactNode
  note?: ReactNode
  disabled?: boolean
  sticky?: boolean
  dirty?: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className={cn('space-y-3', sticky && 'sticky top-0 z-20 border-b bg-background py-2 @max-[20rem]/form:static')} data-save-bar>
      <ErrorNotice error={error} />
      {saved === true ? (
        <Notice kind="success">
          <p>{t('common.saved')}</p>
        </Notice>
      ) : (
        saved || null
      )}
      <ActionGroup className={cn(!sticky && 'border-t pt-4')}>
        {extra}
        <ActionButton type={onSave ? 'button' : 'submit'} onClick={onSave} pending={pending} disabled={disabled}>
          {label ?? t('common.save')}
        </ActionButton>
        {dirty && <span role="status" className="text-warning font-semibold">{t('common.unsaved.label')}</span>}
        {note}
      </ActionGroup>
    </div>
  )
}

/**
 * "Show more" under a list: its error, and the button while there is more.
 * Customer pages use the large, full-width size.
 */
export function LoadMore({
  query,
  label,
  large,
}: {
  query: { hasNextPage: boolean; fetchNextPage: () => Promise<unknown>; isFetching: boolean; error: unknown }
  label?: ReactNode
  large?: boolean
}) {
  const { t } = useTranslation()
  return (
    <>
      <ErrorNotice error={query.error} />
      {query.hasNextPage && (
        <ActionButton
          type="button"
          variant="secondary"
          size={large ? 'lg' : 'default'}
          className={cn(large && 'w-full')}
          onClick={() => void query.fetchNextPage()}
          pending={query.isFetching}
          pendingLabel={t('common.loading')}
        >
          {label ?? t('common.loadMore')}
        </ActionButton>
      )}
    </>
  )
}
