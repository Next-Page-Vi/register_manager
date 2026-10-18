import { useSurface } from '@/components/Surface'
import { ActionGroup } from '@/components/ActionGroup'

import { useTranslation } from "react-i18next"

import * as React from "react"
import { cn } from "@/lib/utils"
import { XIcon } from "lucide-react"
import { Dialog as DialogPrimitive } from "radix-ui"

import { Button } from "@/components/ui/button"

function Dialog({
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Root>) {
  return <DialogPrimitive.Root data-slot="dialog" {...props} />
}

function DialogTrigger({
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Trigger>) {
  return <DialogPrimitive.Trigger data-slot="dialog-trigger" {...props} />
}

function DialogPortal({
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Portal>) {
  return <DialogPrimitive.Portal data-slot="dialog-portal" {...props} />
}

function DialogClose({
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Close>) {
  return <DialogPrimitive.Close data-slot="dialog-close" {...props} />
}

function DialogOverlay({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Overlay>) {
  return (
    <DialogPrimitive.Overlay
      data-slot="dialog-overlay"
      className={cn(
        "fixed inset-0 z-50 bg-black/50 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0",
        className
      )}
      {...props}
    />
  )
}

function DialogContent({
  className,
  children,
  showCloseButton = true,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Content> & {
  showCloseButton?: boolean
}) {
  const { t } = useTranslation()
  const surface = useSurface()
  return (
    <DialogPortal data-slot="dialog-portal">
      <DialogOverlay />
      <DialogPrimitive.Content
        data-slot="dialog-content"
        data-ui-surface={surface}
        className={cn(
          "@container/dialog [--dialog-padding:min(1.5rem,5vw)] fixed top-[50%] left-[50%] z-51 grid grid-cols-[minmax(0,1fr)] w-[calc(100%-2*var(--dialog-inline-inset))] max-w-(--dialog-width) translate-x-[-50%] translate-y-[-50%] max-h-[calc(100dvh-2*var(--dialog-block-inset))] gap-4 overflow-y-auto overscroll-contain rounded-lg border bg-background p-[var(--dialog-padding)] shadow-lg duration-200 outline-none data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95",
          className
        )}
        {...props}
      >
        {children}
        {showCloseButton && (
          <DialogPrimitive.Close
            data-slot="dialog-close"
            className="absolute end-2 top-2 inline-flex size-11 items-center justify-center rounded-md opacity-70 ring-offset-background transition-opacity hover:opacity-100 focus:ring-2 focus:ring-ring focus:ring-offset-2 focus:outline-hidden disabled:pointer-events-none data-[state=open]:bg-accent data-[state=open]:text-muted-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-5"
          >
            <XIcon />
            <span className="sr-only">{t("common.close")}</span>
          </DialogPrimitive.Close>
        )}
      </DialogPrimitive.Content>
    </DialogPortal>
  )
}

function DialogHeader({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      data-slot="dialog-header"
      className={cn("flex min-w-0 flex-col gap-2 pe-12 text-start @max-[12rem]/dialog:pe-0 @max-[12rem]/dialog:pt-11", className)}
      {...props}
    />
  )
}

/**
 * sticky keeps the buttons in view at the bottom while long dialog content
 * scrolls; it shares DialogContent's fluid padding so the bar spans the dialog.
 * Narrow containers wrap whole actions in the same order as keyboard focus.
 */
function DialogFooter({
  className,
  showCloseButton = false,
  sticky = false,
  children,
  ...props
}: React.ComponentProps<"div"> & {
  showCloseButton?: boolean
  sticky?: boolean
}) {
  const { t } = useTranslation()
  return (
    <ActionGroup
      layout="responsive"
      data-slot="dialog-footer"
      className={cn(
        sticky && "max-w-none sticky -bottom-[var(--dialog-padding)] -mx-[var(--dialog-padding)] -mb-[var(--dialog-padding)] border-t bg-background px-[var(--dialog-padding)] pt-3 pb-[var(--dialog-padding)] @max-[12rem]/dialog:static",
        className
      )}
      {...props}
    >
      {children}
      {showCloseButton && (
        <DialogPrimitive.Close asChild>
          <Button variant="outline">{t("common.close")}</Button>
        </DialogPrimitive.Close>
      )}
    </ActionGroup>
  )
}

function DialogTitle({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Title>) {
  return (
    <DialogPrimitive.Title
      data-slot="dialog-title"
      className={cn("text-xl leading-snug font-semibold", className)}
      {...props}
    />
  )
}

function DialogDescription({
  className,
  ...props
}: React.ComponentProps<typeof DialogPrimitive.Description>) {
  return (
    <DialogPrimitive.Description
      data-slot="dialog-description"
      className={cn("text-base text-foreground", className)}
      {...props}
    />
  )
}

export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
}
