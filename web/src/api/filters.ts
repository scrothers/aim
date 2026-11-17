import { queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import type { components } from './schema'
import type { MailSearch } from './mail'
import { ApiError, api, unwrap, unwrapVoid } from './client'

export type SavedFilter = components['schemas']['SavedFilter']
export type FilterParams = Record<string, string>

export const filtersQuery = () =>
  queryOptions({
    queryKey: ['filters'],
    queryFn: async () => unwrap(await api.GET('/api/filters')),
    refetchInterval: 15_000,
  })

/** Console search params that reproduce a /mail view (the open thread is not part of it). */
export function mailFilterParams(s: MailSearch): FilterParams {
  const p: FilterParams = {}
  if (s.view) p.view = s.view
  if (s.mailbox) p.mailbox = s.mailbox
  if (s.state) p.state = s.state
  if (s.muted) p.muted = 'true'
  return p
}

const VIEWS = ['operator', 'unread', 'today', 'sent', 'archived'] as const
const STATES = ['inbox', 'unread', 'all', 'archived'] as const

/** Inverse of mailFilterParams; unknown values are dropped. */
export function mailSearchFromParams(p: FilterParams): MailSearch {
  const s: MailSearch = {}
  const view = VIEWS.find((v) => v === p.view)
  const state = STATES.find((v) => v === p.state)
  if (view) s.view = view
  if (p.mailbox) s.mailbox = p.mailbox
  if (state) s.state = state
  if (p.muted === 'true') s.muted = true
  return s
}

function useInvalidateFilters() {
  const qc = useQueryClient()
  return () => qc.invalidateQueries({ queryKey: ['filters'] })
}

const fail = (e: unknown) => toast.error(e instanceof ApiError ? e.message : 'Could not update saved views')

export function useCreateFilter() {
  const invalidate = useInvalidateFilters()
  return useMutation({
    mutationFn: async (b: { name: string; params: FilterParams }) => unwrap(await api.POST('/api/filters', { body: b })),
    onSuccess: (f) => {
      toast.success(`Saved view “${f.name}”`)
      void invalidate()
    },
    onError: fail,
  })
}

export function useRenameFilter() {
  const invalidate = useInvalidateFilters()
  return useMutation({
    mutationFn: async ({ id, name }: { id: string; name: string }) =>
      unwrap(await api.PATCH('/api/filters', { params: { query: { id } }, body: { name } })),
    onSuccess: () => void invalidate(),
    onError: fail,
  })
}

export function useDeleteFilter() {
  const invalidate = useInvalidateFilters()
  return useMutation({
    mutationFn: async (id: string) => unwrapVoid(await api.DELETE('/api/filters', { params: { query: { id } } })),
    onSuccess: () => void invalidate(),
    onError: fail,
  })
}
