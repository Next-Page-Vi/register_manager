import * as React from "react"
import { cn } from "@/lib/utils"

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        "h-(--control-height) w-full min-w-0 max-w-full rounded-md border border-input bg-background px-3 py-1 text-base transition-[color,border-color,box-shadow] duration-150 outline-none selection:bg-primary selection:text-primary-foreground file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground placeholder:text-muted-foreground disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50",
        "focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50",
        "aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40",
        // iOS's native date/time theme forces content-box sizing and a
        // minimum width, so 100% + padding spills out of the field. Removing
        // that appearance preserves the native picker and our border-box.
        // https://bugs.webkit.org/show_bug.cgi?id=301648
        (type === "date" || type === "time" || type === "datetime-local") && "appearance-none",
        className
      )}
      {...props}
    />
  )
}

export { Input }
