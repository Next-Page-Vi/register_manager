import * as React from "react"
import { cva, type VariantProps } from "class-variance-authority"
import { Loader2 } from "lucide-react"
import { cn } from "@/lib/utils"
import { Slot } from "radix-ui"

const buttonVariants = cva(
  "inline-flex max-w-full shrink-0 items-center justify-center gap-2 rounded-md text-base font-semibold whitespace-normal text-center leading-tight transition-[color,background-color,border-color,box-shadow] duration-150 outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default: "bg-primary text-primary-foreground hover:bg-primary/90",
        destructive:
          "bg-destructive text-destructive-foreground hover:bg-destructive/90 focus-visible:ring-destructive/20 dark:focus-visible:ring-destructive/40",
        // A destructive action that is not the main one: red text and border,
        // so a list of rows is not covered in red blocks.
        "destructive-outline":
          "border border-destructive bg-background text-destructive hover:bg-destructive/10",
        outline:
          "border border-input bg-background hover:bg-accent hover:text-accent-foreground",
        secondary:
          "bg-secondary text-secondary-foreground hover:bg-secondary/80",
        ghost:
          "hover:bg-accent hover:text-accent-foreground dark:hover:bg-accent/50",
        link: "text-primary underline-offset-4 hover:underline",
      },
      size: {
        // Icons follow the button size: 20px on default and larger buttons.
        default: "min-h-(--control-height) px-4 py-2 has-[>svg]:px-4 [&_svg:not([class*='size-'])]:size-5",
        xs: "h-6 gap-1 rounded-md px-2 text-xs has-[>svg]:px-1.5 [&_svg:not([class*='size-'])]:size-3",
        sm: "min-h-(--control-small) gap-1.5 rounded-md px-3 has-[>svg]:px-2.5",
        lg: "min-h-(--control-large) rounded-lg px-6 text-lg has-[>svg]:px-5 [&_svg:not([class*='size-'])]:size-5",
        icon: "size-(--control-height) [&_svg:not([class*='size-'])]:size-5",
        "icon-xs": "size-6 rounded-md [&_svg:not([class*='size-'])]:size-3",
        "icon-sm": "size-(--control-small) [&_svg:not([class*='size-'])]:size-5",
        "icon-lg": "size-(--control-large) [&_svg:not([class*='size-'])]:size-6",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

/**
 * selected: an option chosen from a set (a time, a language). It gets a
 * thick primary border and a light primary wash, and aria-pressed.
 *
 * pending: the action is running. The button shows a spinner and ignores
 * presses (including form submission) but keeps full contrast, unlike
 * disabled, so "Processing…" stays readable.
 */
function Button({
  className,
  variant = "default",
  size = "default",
  asChild = false,
  pending = false,
  selected,
  onClick,
  children,
  ...props
}: React.ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean
    pending?: boolean
    selected?: boolean
  }) {
  const Comp = asChild ? Slot.Root : "button"

  return (
    <Comp
      data-slot="button"
      data-variant={variant}
      data-size={size}
      data-pending={pending || undefined}
      aria-busy={pending || undefined}
      aria-pressed={selected}
      aria-disabled={pending || props["aria-disabled"]}
      className={cn(
        buttonVariants({ variant, size }),
        selected !== undefined && "border-2",
        selected && "border-primary bg-primary/10 hover:bg-primary/15",
        pending && "cursor-progress",
        className
      )}
      onClick={pending ? (e: React.MouseEvent<HTMLButtonElement>) => e.preventDefault() : onClick}
      {...props}
    >
      {/* Slot needs its single child as is, so only plain buttons get the spinner. */}
      {asChild ? (
        children
      ) : (
        <>
          {pending && <Loader2 aria-hidden className="animate-spin" />}
          {children}
        </>
      )}
    </Comp>
  )
}

export { Button, buttonVariants }
