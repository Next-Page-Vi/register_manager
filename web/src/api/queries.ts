import { MutationCache, QueryClient } from '@tanstack/react-query'

type RequestOptions = { params?: { path?: { instance_id?: string } } }

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: false, staleTime: 0, refetchOnWindowFocus: true },
    mutations: { retry: false },
  },
  mutationCache: new MutationCache({
    onSuccess: async (_data, variables, _context, mutation) => {
      const path = mutation.options.mutationKey?.[1]
      if (typeof path === 'string' && path.endsWith('/preview')) return
      await invalidate(variables)
    },
    onError: async (error, variables) => {
      if ('status' in error && error.status === 409) await invalidate(variables)
    },
  }),
})

function invalidate(variables: unknown) {
  const instanceID = (variables as RequestOptions | undefined)?.params?.path?.instance_id
  return queryClient.invalidateQueries({
    predicate: (query) => {
      const queriedInstance = (query.queryKey[2] as RequestOptions | undefined)?.params?.path?.instance_id
      // Shared directories and customer/public views also reflect instance
      // changes. Other instances' manager queries can keep their cache.
      return !instanceID || !queriedInstance || instanceID === queriedInstance
    },
  })
}

export async function clearSessionCache() {
  await queryClient.cancelQueries()
  queryClient.clear()
}

export const cursorOptions = {
  initialPageParam: null,
  getNextPageParam: (page: { next_cursor: string | null }) => page.next_cursor,
}

/** Changing the date starts at page one, even when revisiting a cached date. */
export function resetAvailabilityPages(scope: string) {
  queryClient.setQueriesData<{ pages: unknown[]; pageParams: unknown[] }>({
    predicate: (query) => {
      const path = query.queryKey[1]
      const params = (query.queryKey[2] as { params?: { path?: { slug?: string; instance_id?: string } } } | undefined)?.params
      return typeof path === 'string' && path.endsWith('/availability') &&
        (params?.path?.slug === scope || params?.path?.instance_id === scope)
    },
  }, (data) => data && 'pages' in data ? { ...data, pages: data.pages.slice(0, 1), pageParams: data.pageParams.slice(0, 1) } : data)
}
