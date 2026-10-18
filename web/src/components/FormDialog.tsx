import { useState, type FormEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ActionButton, ErrorNotice } from '@/components/common'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'

/**
 * A form shell. The caller validates and submits, then calls close on success.
 * Mutation state is supplied by the caller; failed submits retain the form.
 */
export function FormDialog({
  trigger,
  title,
  description,
  submitLabel,
  onSubmit,
  pending,
  error,
  onOpen,
  children,
  wide,
}: {
  trigger: ReactNode
  title: ReactNode
  description?: ReactNode
  submitLabel: ReactNode
  onSubmit: (close: () => void) => Promise<void>
  pending: boolean
  error?: unknown
  onOpen?: () => void
  children: ReactNode
  wide?: boolean
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (pending) return
    // Mutation/form state owns the error; this is the DOM promise boundary.
    void onSubmit(() => setOpen(false)).catch(() => {})
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (pending) return
        setOpen(o)
        if (o) {
          onOpen?.()
        }
      }}
    >
      <DialogTrigger asChild>{trigger}</DialogTrigger>
      <DialogContent className={wide ? 'sm:max-w-[45rem]' : undefined}>
        <form onSubmit={submit} className="space-y-(--form-gap)">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            {description && <DialogDescription>{description}</DialogDescription>}
          </DialogHeader>
          {children}
          <ErrorNotice error={error} />
          <DialogFooter sticky>
            <Button type="button" variant="outline" disabled={pending} onClick={() => setOpen(false)}>
              {t('common.cancel')}
            </Button>
            <ActionButton type="submit" pending={pending}>
              {submitLabel}
            </ActionButton>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
