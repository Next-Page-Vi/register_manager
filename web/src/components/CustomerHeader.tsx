import type { ReactNode } from 'react'
import { isHexColor, readableOn } from '@/lib/color'
import { cn } from '@/lib/utils'

/**
 * Top bar of customer pages. With a page colour, the bar uses it and the
 * text is black or white, whichever is easier to read on it.
 */
export function CustomerHeader({ title, brand, children }: {
  title?: string
  brand?: string | null
  children?: ReactNode
}) {
  const style = isHexColor(brand) ? { backgroundColor: brand, color: readableOn(brand) } : undefined
  return (
    // On a page colour the edge is a faint shade of it, not a grey line,
    // and still shows when the colour is close to white.
    <header className={cn('border-b', style ? 'border-black/10' : 'bg-secondary/40')} style={style}>
      <div className="mx-auto flex max-w-[47.25rem] flex-wrap items-center justify-between gap-x-4 gap-y-2 px-4 py-3">
        <p className="min-w-0 flex-[1_1_14rem] font-semibold leading-snug">{title}</p>
        {children}
      </div>
    </header>
  )
}
