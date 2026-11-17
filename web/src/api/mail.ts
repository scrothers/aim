import { infiniteQueryOptions, keepPreviousData, queryOptions, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import type { components } from './schema'
import { ApiError, api, unwrap } from './client'
import { DEFAULT_POLL_MS, FAST_POLL_MS, localMidnight } from './queries'

export type Header = components['schemas']['Header']
export type MessageView = components['schemas']['MessageView']
export type SendRequest = components['schemas']['SendRequest']
export type MailState = 'unread' | 'all' | 'archived'

export interface MailSearch {
  view?: 'operator' | 'unread' | 'today' | 'sent' | 'archived'
  mailbox?: string
  state?: MailState
  thread?: string
}

export interface ListParams {
  address?: string
  from?: string
  state: 'unread' | 'read' | 'archived' | 'all'
  since?: string
  until?: string
}

export const ARCHIVE_DAYS = 30

/** Resolves URL search params to API list parameters. */
export function listParamsFor(search: MailSearch, now = new Date()): ListParams {
  const view = search.mailbox ? undefined : (search.view ?? 'operator')
  let state: MailState = search.state ?? (view === 'unread' ? 'unread' : 'all')
  if (view === 'archived') state = 'archived'
  if (view === 'unread') state = 'unread'
  const p: ListParams = { state }
  if (search.mailbox) p.address = search.mailbox
  else if (view === 'operator' || view === 'archived') p.address = 'operator'
  else if (view === 'sent') p.from = 'operator'
  else if (view === 'today') p.since = localMidnight(now)
  if (state === 'archived') {
    const until = new Date(now)
    until.setSeconds(0, 0)
    until.setMinutes(until.getMinutes() + 1)
    const since = new Date(until.getTime() - ARCHIVE_DAYS * 86_400_000)
    p.since = since.toISOString()
    p.until = until.toISOString()
  }
  return p
}

export const messagesQuery = (p: ListParams) =>
  infiniteQueryOptions({
    queryKey: ['messages', p],
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) =>
      unwrap(await api.GET('/api/messages', { params: { query: { ...p, ...(pageParam ? { cursor: pageParam } : {}) } } })),
    getNextPageParam: (last) => last.next_cursor || undefined,
    refetchInterval: p.address === 'operator' ? FAST_POLL_MS : DEFAULT_POLL_MS,
    placeholderData: keepPreviousData,
  })

export const threadQuery = (id: string) =>
  queryOptions({
    queryKey: ['thread', id],
    queryFn: async () => unwrap(await api.GET('/api/threads/{id}', { params: { path: { id } } })),
    refetchInterval: DEFAULT_POLL_MS,
    placeholderData: keepPreviousData,
  })

export const rosterQuery = () =>
  queryOptions({
    queryKey: ['roster'],
    queryFn: async () => unwrap(await api.GET('/api/roster')),
    staleTime: 30_000,
    refetchInterval: false,
  })

export const renderQuery = (markdown: string) =>
  queryOptions({
    queryKey: ['render', markdown],
    queryFn: async () => unwrap(await api.POST('/api/render', { body: { markdown } })),
    staleTime: Infinity,
    refetchInterval: false,
    retry: false,
  })

export function useInvalidateMail() {
  const qc = useQueryClient()
  return () =>
    Promise.all(['messages', 'thread', 'tree', 'views'].map((k) => qc.invalidateQueries({ queryKey: [k] })))
}

export type MarkState = 'read' | 'unread' | 'archived' | 'unarchived'

export function useMark() {
  const invalidate = useInvalidateMail()
  return useMutation({
    mutationFn: async ({ ids, state }: { ids: string[]; state: MarkState }) =>
      unwrap(await api.POST('/api/marks', { body: { message_ids: ids, state } })),
    onSuccess: (_d, v) => {
      if (v.state !== 'read') toast.success(v.state === 'archived' ? 'Archived' : v.state === 'unread' ? 'Marked unread' : 'Updated')
      void invalidate()
    },
    onError: (e) => toast.error(e instanceof ApiError ? e.message : 'Could not update messages'),
  })
}

export function useReadMessage() {
  const invalidate = useInvalidateMail()
  return useMutation({
    mutationFn: async (id: string) => unwrap(await api.POST('/api/messages/{id}/read', { params: { path: { id } } })),
    onSuccess: () => void invalidate(),
  })
}

export function useSend() {
  const invalidate = useInvalidateMail()
  return useMutation({
    mutationFn: async (req: SendRequest) => unwrap(await api.POST('/api/messages', { body: req })),
    onSuccess: () => {
      toast.success('Message sent')
      void invalidate()
    },
  })
}
