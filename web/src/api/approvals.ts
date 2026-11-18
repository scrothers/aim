import { keepPreviousData, queryOptions, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import type { components } from './schema'
import { ApiError, api, unwrap } from './client'
import { FAST_POLL_MS } from './queries'

export type Approval = components['schemas']['Approval']
export type ApprovalSettings = components['schemas']['ApprovalSettingsBody']
export type Decision = 'allow' | 'deny'

/** `background` keeps polling while the tab is hidden (needed for desktop notifications). */
export const approvalsQuery = (background = false) =>
  queryOptions({
    queryKey: ['approvals'],
    queryFn: async () => unwrap(await api.GET('/api/approvals')),
    refetchInterval: FAST_POLL_MS,
    refetchIntervalInBackground: background,
    placeholderData: keepPreviousData,
  })

export const approvalSettingsQuery = () =>
  queryOptions({
    queryKey: ['approval-settings'],
    queryFn: async () => unwrap(await api.GET('/api/settings/approvals')),
    refetchInterval: 15_000,
  })

export const useApprovals = () => useQuery(approvalsQuery())
export const useApprovalSettings = () => useQuery(approvalSettingsQuery())

export function useDecideApproval() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (v: { id: string; decision: Decision; reason?: string }) =>
      unwrap(await api.POST('/api/approvals/{id}', { params: { path: { id: v.id } }, body: { decision: v.decision, ...(v.reason ? { reason: v.reason } : {}) } })),
    onSuccess: (r) => toast.success(r.decision === 'allow' ? 'Allowed' : 'Denied'),
    onError: (e) => {
      if (e instanceof ApiError && e.status === 404) toast.error('No longer pending: already decided or the agent stopped waiting')
      else toast.error(e instanceof ApiError ? e.message : 'Could not send decision')
    },
    onSettled: () => void qc.invalidateQueries({ queryKey: ['approvals'] }),
  })
}

export function useSaveApprovalSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (b: { enabled: boolean; timeout_seconds?: number }) => unwrap(await api.PUT('/api/settings/approvals', { body: b })),
    onSuccess: (r) => toast.success(r.enabled ? 'Remote approvals on' : 'Remote approvals off'),
    onError: (e) => toast.error(e instanceof ApiError ? e.message : 'Could not save settings'),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ['approval-settings'] })
      void qc.invalidateQueries({ queryKey: ['approvals'] })
    },
  })
}
