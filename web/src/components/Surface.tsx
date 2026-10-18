import { createContext, useContext, type ComponentProps } from 'react'

export type SurfaceKind = 'staff' | 'customer'
const SurfaceContext = createContext<SurfaceKind>('staff')

/** React context also crosses Radix portals; CSS inheritance alone does not. */
export function useSurface() {
  return useContext(SurfaceContext)
}

export function Surface({ kind, ...props }: ComponentProps<'div'> & { kind: SurfaceKind }) {
  return (
    <SurfaceContext value={kind}>
      <div data-ui-surface={kind} {...props} />
    </SurfaceContext>
  )
}
