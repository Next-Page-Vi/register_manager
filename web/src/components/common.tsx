import { cloneElement, isValidElement, type ComponentProps, type ReactNode } from 'react'
import { AlertCircle, CheckCircle2, Info, Loader2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { errorInfo } from '@/api/client'
import { Button } from '@/components/ui/button'
import type { UseQueryResult } from '@tanstack/react-query'
import { cn } from '@/lib/utils'

/** Translates an API error by its stable code, with parameters. */
export function errorText(t: TFunction, err: unknown): string {
  if (!err) return ''
  const info = errorInfo(err)
  const key = `errors.${info.code}`
  const params = { ...info.params } as Record<string, unknown>
  return t(key, { ...params, defaultValue: t('errors.unknown_error') })
}

type NoticeKind = 'error' | 'success' | 'info' | 'warning'

// Text and an icon carry the state; a quiet tinted surface separates it
// from the content without competing with the page's main action.
const noticeStyles: Record<NoticeKind, string> = {
  error: 'border-destructive/30 bg-destructive/10 text-destructive',
  success: 'border-success/30 bg-success/10 text-success',
  info: 'border-primary/25 bg-primary/5 text-primary',
  warning: 'border-warning/30 bg-warning/15 text-warning',
}

const noticeIcons = { error: AlertCircle, success: CheckCircle2, info: Info, warning: AlertCircle }

/** A message box. Meaning is carried by text and an icon, not colour alone. */
export function Notice({
  kind = 'info',
  title,
  children,
  className,
}: {
  kind?: NoticeKind
  title?: ReactNode
  children?: ReactNode
  className?: string
}) {
  const Icon = noticeIcons[kind]
  return (
    <div
      role={kind === 'error' ? 'alert' : 'status'}
      className={cn('flex gap-3 rounded-lg border p-4', noticeStyles[kind], className)}
    >
      <Icon aria-hidden className="mt-1 size-6 shrink-0" />
      <div className="min-w-0 space-y-1 text-foreground">
        {title && <p className="font-semibold">{title}</p>}
        {children}
      </div>
    </div>
  )
}

export function ErrorNotice({ error, className }: { error: unknown; className?: string }) {
  const { t } = useTranslation()
  if (!error) return null
  return (
    <Notice kind="error" className={className}>
      <p>{errorText(t, error)}</p>
    </Notice>
  )
}

export function Loading({ className, layout = 'list' }: { className?: string; layout?: 'list' | 'form' | 'summary' }) {
  const { t } = useTranslation()
  return (
    <div role="status" className={cn('space-y-4 text-muted-foreground', className)}>
      <span className="sr-only">{t('common.loading')}</span>
      <div aria-hidden className={cn('grid gap-4 motion-safe:animate-pulse', layout === 'summary' && 'sm:grid-cols-2')}>
        {Array.from({ length: layout === 'summary' ? 2 : 3 }, (_, i) => (
          <div key={i} className="space-y-3 rounded-xl border bg-card p-5">
            <div className="h-4 w-1/3 rounded bg-muted" />
            <div className={cn('rounded bg-muted', layout === 'form' ? 'h-12 w-full' : 'h-6 w-2/3')} />
          </div>
        ))}
      </div>
    </div>
  )
}

/** Spinner shown before translations load (no text available yet). */
export function BootSpinner() {
  return (
    <div className="flex min-h-dvh items-center justify-center">
      {/* Shown before any translation has loaded, so no translated text exists yet. */}
      {/* eslint-disable-next-line i18next/no-literal-string */}
      <Loader2 aria-label="Loading" className="size-8 animate-spin text-muted-foreground" />
    </div>
  )
}

/**
 * A navigation link, shared by the top bar, the calendar sidebar and the
 * phone menu. The selected page has a tinted face, a border and primary text.
 */
export function navItemClass(isActive: boolean, orientation: 'horizontal' | 'vertical') {
  return cn(
    'relative flex min-h-11 items-center gap-3 rounded-lg border border-transparent px-3 font-semibold transition-colors duration-150 hover:bg-accent',
    orientation === 'vertical' ? 'py-2' : 'py-2 whitespace-nowrap',
    isActive && 'border-primary/20 bg-accent text-primary',
  )
}

type ControlProps = { 'aria-describedby'?: string; 'aria-invalid'?: boolean | 'true' | 'false' }

/**
 * A labelled control. The hint and error are linked to the control with
 * aria-describedby, and an error marks it aria-invalid, so screen readers
 * announce them when the control gets focus.
 */
export function Field({
  label,
  htmlFor,
  hint,
  error,
  hideLabel,
  className,
  children,
}: {
  label: ReactNode
  htmlFor: string
  /** Keeps the label for screen readers only, when a heading already says it. */
  hideLabel?: boolean
  hint?: ReactNode
  error?: ReactNode
  className?: string
  children: ReactNode
}) {
  const hintId = `${htmlFor}-hint`
  const errorId = `${htmlFor}-error`
  let control = children
  if (isValidElement<ControlProps>(children)) {
    const describedBy = [children.props['aria-describedby'], hint ? hintId : null, error ? errorId : null]
      .filter(Boolean)
      .join(' ')
    control = cloneElement(children, {
      'aria-describedby': describedBy || undefined,
      'aria-invalid': error ? true : children.props['aria-invalid'],
    })
  }
  return (
    <div data-slot="field" className={cn('min-w-0 max-w-full space-y-2', className)}>
      <label data-slot="field-label" htmlFor={htmlFor} className={hideLabel ? 'sr-only' : 'block font-semibold'}>
        {label}
      </label>
      <div data-slot="field-control" className="min-w-0">{control}</div>
      {(hint || error) && (
        <div data-slot="field-support" className="space-y-2">
          {hint && <p id={hintId} className="text-muted-foreground text-sm">{hint}</p>}
          {error && <FieldError id={errorId}>{error}</FieldError>}
        </div>
      )}
    </div>
  )
}

/**
 * An error next to the input it is about: icon and red text, announced.
 * inline renders a quiet span for use inside other controls (a summary in
 * a button), where it is read as part of that control instead.
 */
export function FieldError({ id, inline, children }: { id?: string; inline?: boolean; children: ReactNode }) {
  const Tag = inline ? 'span' : 'p'
  return (
    <Tag id={id} role={inline ? undefined : 'alert'} className="text-destructive flex items-center gap-2 text-base">
      <AlertCircle aria-hidden className="size-5 shrink-0" />
      {children}
    </Tag>
  )
}

/**
 * "Nothing here": a dashed box in the back office (lists of records), plain
 * muted text on customer pages, which stay as quiet as possible.
 */
export function EmptyState({ variant = 'box', children }: { variant?: 'box' | 'plain'; children: ReactNode }) {
  return (
    <p className={cn('text-muted-foreground', variant === 'box' && 'rounded-xl border border-dashed bg-card px-4 py-8 text-center')}>
      {children}
    </p>
  )
}

/**
 * What a request shows while loading or after failing; children render the
 * data once it has arrived.
 */
export function QueryContent<T>({
  query,
  pendingFallback,
  children,
}: {
  query: UseQueryResult<T, unknown>
  pendingFallback?: ReactNode
  children: (data: T) => ReactNode
}) {
  if (query.data !== undefined)
    return (
      <>
        <ErrorNotice error={query.error} />
        {children(query.data)}
      </>
    )
  if (query.isPending) return pendingFallback ?? <Loading />
  return <ErrorNotice error={query.error} />
}

/** A button for an action that takes a moment: says "Processing…" meanwhile. */
export function ActionButton({
  pending,
  pendingLabel,
  children,
  ...props
}: ComponentProps<typeof Button> & {
  pendingLabel?: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <Button pending={pending} {...props}>
      {pending ? (pendingLabel ?? t('common.processing')) : children}
    </Button>
  )
}
