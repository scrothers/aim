import { keepPreviousData, queryOptions, useQuery } from '@tanstack/react-query'
import { api, unwrap } from './client'

export const FAST_POLL_MS = 5_000
export const DEFAULT_POLL_MS = 15_000

/** Start of the operator's local day as RFC 3339. */
export function localMidnight(now = new Date()): string {
  const d = new Date(now)
  d.setHours(0, 0, 0, 0)
  return d.toISOString()
}

/** `background` keeps polling while the tab is hidden (needed for desktop notifications). */
export const viewsQuery = (background = false) =>
  queryOptions({
    queryKey: ['views'],
    queryFn: async () => unwrap(await api.GET('/api/views', { params: { query: { today_since: localMidnight() } } })),
    refetchInterval: FAST_POLL_MS,
    refetchIntervalInBackground: background,
    placeholderData: keepPreviousData,
  })

export const treeQuery = () =>
  queryOptions({
    queryKey: ['tree'],
    queryFn: async () => unwrap(await api.GET('/api/tree')),
    refetchInterval: FAST_POLL_MS,
    placeholderData: keepPreviousData,
  })

export const presenceQuery = () =>
  queryOptions({
    queryKey: ['presence'],
    queryFn: async () => unwrap(await api.GET('/api/presence')),
    refetchInterval: FAST_POLL_MS,
    placeholderData: keepPreviousData,
  })

export const statsQuery = () =>
  queryOptions({
    queryKey: ['stats'],
    queryFn: async () => unwrap(await api.GET('/api/stats')),
    refetchInterval: DEFAULT_POLL_MS,
    placeholderData: keepPreviousData,
  })

export const healthQuery = () =>
  queryOptions({
    queryKey: ['healthz'],
    queryFn: async () => unwrap(await api.GET('/healthz')),
    refetchInterval: DEFAULT_POLL_MS,
    retry: false,
  })

export const useViews = () => useQuery(viewsQuery())
export const useTree = () => useQuery(treeQuery())
export const usePresence = () => useQuery(presenceQuery())
export const useStats = () => useQuery(statsQuery())
export const useHealth = () => useQuery(healthQuery())

/** Every mailbox address in the tree (orgs, repos and identities), for autocomplete. */
export function useMailboxAddresses(): string[] {
  const tree = useTree().data
  return (tree?.orgs ?? []).flatMap((o) => [
    o.address,
    ...(o.identities ?? []).map((i) => i.address),
    ...(o.repos ?? []).flatMap((r) => [r.address, ...(r.identities ?? []).map((i) => i.address)]),
  ])
}
