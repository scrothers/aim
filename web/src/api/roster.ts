import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import type { components } from './schema'
import { ApiError, api, unwrap } from './client'
import { DEFAULT_POLL_MS } from './queries'

export type RosterEntry = components['schemas']['RosterEntry']
export type OrgChartData = components['schemas']['OrgChart']
export type OrgChartNode = components['schemas']['OrgChartNode']
export type UpdateRequest = components['schemas']['UpdateRequest']

export const orgChartQuery = () =>
  queryOptions({
    queryKey: ['orgchart'],
    queryFn: async () => unwrap(await api.GET('/api/orgchart')),
    refetchInterval: DEFAULT_POLL_MS,
  })

/** Refreshes everything that shows identity metadata or retirement. */
function useInvalidateIdentities() {
  const qc = useQueryClient()
  return () =>
    Promise.all(['roster', 'orgchart', 'tree', 'presence'].map((k) => qc.invalidateQueries({ queryKey: [k] })))
}

export function useUpdateIdentity() {
  const invalidate = useInvalidateIdentities()
  return useMutation({
    mutationFn: async ({ address, body }: { address: string; body: UpdateRequest }) =>
      unwrap(await api.PATCH('/api/identities', { params: { query: { address } }, body })),
    onSuccess: () => {
      toast.success('Identity updated')
      void invalidate()
    },
  })
}

export function useRetire() {
  const invalidate = useInvalidateIdentities()
  return useMutation({
    mutationFn: async ({ address, retire }: { address: string; retire: boolean }) =>
      unwrap(
        await api.POST(retire ? '/api/identities/retire' : '/api/identities/unretire', {
          params: { query: { address } },
        }),
      ),
    onSuccess: (_d, v) => {
      toast.success(v.retire ? 'Identity retired' : 'Identity restored')
      void invalidate()
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : 'Could not update identity'),
  })
}
