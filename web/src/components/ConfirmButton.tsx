import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ErrorNotice } from '@/components/common'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'

/**
 * A button that asks for explicit confirmation before acting.
 * confirmVariant styles the dialog's confirm button (default: variant), so
 * an outlined trigger can still confirm with a red "Yes, cancel" button;
 * a destructive-outline trigger confirms with a solid red button.
 *
 * The dialog stays open while onConfirm runs and closes when it succeeds;
 * if it throws, the error shows in the dialog. body adds content under the
 * description (such as a field to type a name into), confirmDisabled keeps
 * the confirm button off until it is filled in, and onOpen resets it.
 */
export function ConfirmButton({
  children,
  title,
  description,
  confirmLabel,
  onConfirm,
  variant = 'default',
  confirmVariant = variant === 'destructive-outline' ? 'destructive' : variant,
  size,
  disabled,
  pending,
  error,
  className,
  body,
  confirmDisabled,
  onOpen,
}: {
  children: ReactNode
  title: ReactNode
  description?: ReactNode
  confirmLabel: ReactNode
  onConfirm: () => void | Promise<unknown>
  variant?: 'default' | 'destructive' | 'destructive-outline' | 'outline' | 'secondary' | 'ghost'
  confirmVariant?: 'default' | 'destructive' | 'outline' | 'secondary' | 'ghost'
  size?: 'default' | 'sm' | 'lg'
  disabled?: boolean
  pending: boolean
  error?: unknown
  className?: string
  body?: ReactNode
  confirmDisabled?: boolean
  onOpen?: () => void
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const onOpenChange = (o: boolean) => {
    if (pending) return
    if (o) {
      onOpen?.()
    }
    setOpen(o)
  }
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogTrigger asChild>
        <Button variant={variant} size={size} disabled={disabled} pending={pending} className={className}>
          {children}
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          {description && <AlertDialogDescription>{description}</AlertDialogDescription>}
        </AlertDialogHeader>
        {body}
        <ErrorNotice error={error} />
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>{t('common.goBack')}</AlertDialogCancel>
          <AlertDialogAction
            variant={confirmVariant}
            disabled={confirmDisabled || pending}
            onClick={async (e) => {
              // Stay open until the action is done, so a failure can be shown.
              e.preventDefault()
              try {
                await onConfirm()
                setOpen(false)
              } catch {
                // The mutation supplies the error; keep the confirmation open.
              }
            }}
          >
            {pending ? t('common.processing') : confirmLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
