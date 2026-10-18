import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

/** Wrap whole actions in DOM order. Dialog actions grow to fill a narrow row. */
export function ActionGroup({ className, layout = 'inline', ...props }: ComponentProps<'div'> & {
  layout?: 'inline' | 'responsive'
}) {
  return <div className={cn('flex min-w-0 max-w-full flex-wrap items-center gap-2', layout === 'responsive' && 'justify-end [&>button]:flex-[1_1_auto] @min-[24rem]/dialog:[&>button]:flex-none', className)} {...props} />
}
