import { keepPreviousData, queryOptions, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import type { components } from './schema'
import { ApiError, api, unwrap } from './client'
import { FAST_POLL_MS } from './queries'

export type TaskView = components['schemas']['TaskView']
export type TaskDetail = components['schemas']['TaskDetailView']
export type TaskEvent = components['schemas']['TaskEventView']
export type TaskActionRequest = components['schemas']['TaskActionRequest']
export type TaskCreateRequest = components['schemas']['TaskCreateRequest']
export type ThreadTask = components['schemas']['ThreadTaskView']
export type TaskStatus = TaskView['status']
export type TaskPriority = TaskView['priority']
export type TaskHealth = NonNullable<TaskView['health']>

export interface TaskFilters {
  scope?: string
  assignee?: string
  /** Only tasks this identity created, is assigned or holds. */
  involves?: string
  showFinished?: boolean
}

const PAGE = 200
const MAX_PAGES = 5

export interface TaskList {
  items: TaskView[]
  /** More tasks exist than the board loaded. */
  truncated: boolean
}

export const tasksQuery = (f: TaskFilters = {}) =>
  queryOptions({
    queryKey: ['tasks', f],
    queryFn: async (): Promise<TaskList> => {
      const items: TaskView[] = []
      let cursor: string | undefined
      for (let page = 0; page < MAX_PAGES; page++) {
        const q = {
          limit: PAGE,
          ...(f.showFinished ? {} : { open_only: true }),
          ...(f.scope ? { scope: f.scope } : {}),
          ...(f.assignee ? { assignee: f.assignee } : {}),
          ...(f.involves ? { involves: f.involves } : {}),
          ...(cursor ? { cursor } : {}),
        }
        const body = unwrap(await api.GET('/api/tasks', { params: { query: q } }))
        items.push(...(body.items ?? []))
        cursor = body.next_cursor || undefined
        if (!cursor) return { items, truncated: false }
      }
      return { items, truncated: true }
    },
    refetchInterval: FAST_POLL_MS,
    placeholderData: keepPreviousData,
  })

export const taskQuery = (id: string) =>
  queryOptions({
    queryKey: ['task', id],
    queryFn: async () => unwrap(await api.GET('/api/tasks/{id}', { params: { path: { id } } })),
    refetchInterval: FAST_POLL_MS,
    placeholderData: keepPreviousData,
  })

export const threadTasksQuery = (threadIds: string[]) => {
  const ids = [...new Set(threadIds)].sort()
  return queryOptions({
    queryKey: ['thread-tasks', ids],
    queryFn: async () => unwrap(await api.GET('/api/thread-tasks', { params: { query: { thread_id: ids } } })),
    refetchInterval: FAST_POLL_MS,
    placeholderData: keepPreviousData,
  })
}

/** Open tasks (everything but finished), for the nav badge; shares the board's default query. */
export const useOpenTaskCount = () => useQuery({ ...tasksQuery(), select: (d) => d.items.length }).data ?? 0

export const useTasks = (f: TaskFilters) => useQuery(tasksQuery(f))
export const useTask = (id: string | undefined) => useQuery({ ...taskQuery(id ?? ''), enabled: !!id })
export const useThreadTasks = (threadId: string | undefined) =>
  useQuery({ ...threadTasksQuery(threadId ? [threadId] : []), enabled: !!threadId, select: (d) => d.items ?? [] })

function describe(e: unknown, fallback: string): string {
  return e instanceof ApiError ? e.message : fallback
}

interface ActionVars {
  id: string
  body: TaskActionRequest
  /** Status to show on the board while the request is in flight. */
  optimistic?: TaskStatus
}

type Snapshot = Array<[readonly unknown[], TaskList | undefined]>

function invalidate(qc: ReturnType<typeof useQueryClient>) {
  void qc.invalidateQueries({ queryKey: ['tasks'] })
  void qc.invalidateQueries({ queryKey: ['task'] })
  void qc.invalidateQueries({ queryKey: ['thread-tasks'] })
}

/**
 * Applies a task action as the operator. With `optimistic` the board moves the
 * card at once and rolls back on failure. Errors are toasted unless the caller
 * passes its own `onError` to `mutate`.
 */
export function useTaskAction(opts: { silent?: boolean } = {}) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (v: ActionVars) => unwrap(await api.POST('/api/tasks/{id}/actions', { params: { path: { id: v.id } }, body: v.body })),
    onMutate: async (v): Promise<{ snap: Snapshot }> => {
      if (!v.optimistic) return { snap: [] }
      await qc.cancelQueries({ queryKey: ['tasks'] })
      const snap = qc.getQueriesData<TaskList>({ queryKey: ['tasks'] })
      const status = v.optimistic
      qc.setQueriesData<TaskList>({ queryKey: ['tasks'] }, (old) =>
        old
          ? {
              ...old,
              items: old.items.map((t) => {
                if (t.id !== v.id) return t
                const next: TaskView = { ...t, status }
                if (status === 'not_started' || status === 'finished') delete next.claimant
                else next.claimant = t.claimant ?? 'operator'
                if (status !== 'claimed' && status !== 'started') delete next.health
                return next
              }),
            }
          : old,
      )
      return { snap }
    },
    onSuccess: (detail) => {
      qc.setQueryData(['task', detail.id], detail)
    },
    onError: (e, _v, ctx) => {
      for (const [key, data] of ctx?.snap ?? []) qc.setQueryData(key, data)
      if (!opts.silent) toast.error(describe(e, 'Could not update the task'))
    },
    onSettled: () => invalidate(qc),
  })
}

export function useCreateTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (body: TaskCreateRequest) => unwrap(await api.POST('/api/tasks', { body })),
    onSuccess: (t) => {
      qc.setQueryData(['task', t.id], t)
      toast.success('Task created')
    },
    onSettled: () => invalidate(qc),
  })
}
