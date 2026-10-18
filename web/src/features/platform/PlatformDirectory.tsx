import type { ReactNode } from 'react'
import { $api, type Schemas } from '@/api/client'
import { QueryContent } from '@/components/common'

/** Fetch both independent directories in parallel and share their query cache. */
export function PlatformDirectory({
  children,
}: {
  children: (data: { accounts: Schemas['Account'][]; instances: Schemas['PlatformInstance'][] }) => ReactNode
}) {
  const accounts = $api.useQuery('get', '/api/v1/platform/accounts')
  const instances = $api.useQuery('get', '/api/v1/platform/instances')
  return (
    <QueryContent query={accounts}>
      {(a) => <QueryContent query={instances}>{(i) => children({ ...a, ...i })}</QueryContent>}
    </QueryContent>
  )
}
