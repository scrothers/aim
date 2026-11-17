import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import type { components } from './schema'
import { ApiError, api, unwrap } from './client'

export type Settings = components['schemas']['SettingsBody']
export type WebhookView = components['schemas']['WebhookView']
export type WebhookInput = components['schemas']['WebhookInput']
export type PruneResult = components['schemas']['PruneResult']
export type TestResult = components['schemas']['Result']

export const settingsQuery = () =>
  queryOptions({
    queryKey: ['settings'],
    queryFn: async () => unwrap(await api.GET('/api/settings')),
    refetchInterval: 15_000,
  })

const fail = (e: unknown) => toast.error(e instanceof ApiError ? e.message : 'Could not save settings')

function useSave<V>(fn: (v: V) => Promise<unknown>, done: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      toast.success(done)
      void qc.invalidateQueries({ queryKey: ['settings'] })
    },
    onError: fail,
  })
}

export const useSaveRetention = () =>
  useSave(
    async (days: number) => unwrap(await api.PUT('/api/settings/retention', { body: { prune_archived_after_days: days } })),
    'Retention saved',
  )

export const useSaveBackups = () =>
  useSave(
    async (b: { every_hours: number; keep: number; dir: string }) => unwrap(await api.PUT('/api/settings/backups', { body: b })),
    'Backup settings saved',
  )

export const useSaveNotify = () =>
  useSave(
    async (b: { public_url?: string; webhooks: WebhookInput[] }) => unwrap(await api.PUT('/api/settings/notify', { body: b })),
    'Notification settings saved',
  )

export function usePrune() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (dryRun: boolean) =>
      unwrap(await api.POST('/api/settings/prune', { params: { query: { dry_run: dryRun } } })),
    onSuccess: (r) => {
      if (!r.dry_run) toast.success(`Pruned ${r.messages} ${r.messages === 1 ? 'message' : 'messages'}`)
      void qc.invalidateQueries({ queryKey: ['settings'] })
    },
    onError: fail,
  })
}

export function useBackupNow() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => unwrap(await api.POST('/api/settings/backup')),
    onSuccess: () => toast.success('Backup written'),
    onError: fail,
    onSettled: () => void qc.invalidateQueries({ queryKey: ['settings'] }),
  })
}

export function useTestWebhook() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api.POST('/api/settings/webhooks/test', { params: { query: { id } } })),
    onError: fail,
    onSettled: () => void qc.invalidateQueries({ queryKey: ['settings'] }),
  })
}
