import * as React from "react"
import { cn } from "@/lib/utils"

/**
 * A native select styled like Input; native controls work best on phones
 * (the system picker, large touch targets, screen reader support).
 */
function NativeSelect({ className, ...props }: React.ComponentProps<"select">) {
  return (
    <select
      data-slot="native-select"
      className={cn(
        "h-(--control-height) w-full min-w-0 rounded-md border border-input bg-background px-3 text-base transition-[color,border-color,box-shadow] duration-150 outline-none disabled:cursor-not-allowed disabled:opacity-50",
        "focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50",
        "aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40",
        className
      )}
      {...props}
    />
  )
}

export { NativeSelect }
