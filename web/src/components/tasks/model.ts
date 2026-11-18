import type { TaskEvent, TaskHealth, TaskPriority, TaskStatus, TaskView } from '@/api/tasks'

/** Native `<select>` styling shared by the board filters and the drawer fields. */
export const selectCls =
  'h-8 min-w-0 rounded-md border bg-panel px-2 text-[13px] text-text outline-none focus-visible:ring-2 focus-visible:ring-ring/40 max-md:h-11'

/** Board order. */
export const STATUSES: TaskStatus[] = ['not_started', 'claimed', 'started', 'abandoned', 'finished']

export const STATUS_LABEL: Record<TaskStatus, string> = {
  not_started: 'Not started',
  claimed: 'Claimed',
  started: 'Started',
  abandoned: 'Abandoned',
  finished: 'Finished',
}

export const PRIORITIES: TaskPriority[] = ['low', 'normal', 'high', 'urgent']
const PRIORITY_RANK: Record<TaskPriority, number> = { urgent: 0, high: 1, normal: 2, low: 3 }

export const HEALTH_LABEL: Record<TaskHealth, string> = {
  active: 'Active',
  idle: 'Idle',
  stale: 'Stale',
  abandoned: 'Abandoned',
}

/** Urgent first, then most recently updated. */
export function byPriority(a: TaskView, b: TaskView): number {
  return PRIORITY_RANK[a.priority] - PRIORITY_RANK[b.priority] || Date.parse(b.updated_at) - Date.parse(a.updated_at)
}

export function groupByStatus(items: TaskView[]): Record<TaskStatus, TaskView[]> {
  const out: Record<TaskStatus, TaskView[]> = { not_started: [], claimed: [], started: [], abandoned: [], finished: [] }
  for (const t of items) out[t.status].push(t)
  for (const s of STATUSES) out[s].sort(byPriority)
  return out
}

export function isOverdue(t: Pick<TaskView, 'due_at' | 'status'>, now = Date.now()): boolean {
  if (!t.due_at || t.status === 'finished') return false
  const due = Date.parse(t.due_at)
  return !Number.isNaN(due) && due < now
}

export type ActionName = 'claim' | 'start' | 'finish' | 'release' | 'reopen'

/** Lifecycle actions the operator may take on a task in `status`, in button order. */
export function actionsFor(status: TaskStatus): ActionName[] {
  switch (status) {
    case 'not_started':
      return ['start', 'claim']
    case 'claimed':
      return ['start', 'release']
    case 'started':
      return ['finish', 'release']
    case 'finished':
      return ['reopen']
    case 'abandoned':
      return ['start', 'claim', 'reopen']
  }
}

export interface MovePlan {
  /** Actions to run in order. */
  actions: ActionName[]
  /** Ask before running. */
  confirm?: string
}

export type Move = MovePlan | { reject: string } | null

/**
 * Maps a card dragged from `from` to column `to` onto lifecycle actions.
 * null means the drop changes nothing; `reject` explains a refused move.
 */
export function planMove(from: TaskStatus, to: TaskStatus): Move {
  if (from === to) return null
  switch (to) {
    case 'abandoned':
      return { reject: 'Tasks become abandoned on their own when a claim lapses.' }
    case 'started':
      return { actions: ['start'] }
    case 'claimed':
      if (from === 'not_started' || from === 'abandoned') return { actions: ['claim'], confirm: 'Claim this task as the operator?' }
      return { reject: from === 'started' ? 'A started task cannot go back to claimed. Release it instead.' : 'Reopen the task first.' }
    case 'finished':
      if (from === 'started') return { actions: ['finish'] }
      if (from === 'claimed') return { actions: ['start', 'finish'], confirm: 'Mark this task finished? It was claimed but never started.' }
      return { actions: ['start', 'finish'], confirm: 'Mark this task finished? Nobody has started it.' }
    case 'not_started':
      if (from === 'claimed' || from === 'started') return { actions: ['release'], confirm: 'Release this task? The current claimant loses it.' }
      return { actions: ['reopen'] }
  }
}

/** The status a card shows after `actions` ran. */
export function resultStatus(actions: ActionName[]): TaskStatus {
  const last = actions[actions.length - 1]
  switch (last) {
    case 'claim':
      return 'claimed'
    case 'start':
      return 'started'
    case 'finish':
      return 'finished'
    default:
      return 'not_started'
  }
}

const EVENT_TEXT: Record<TaskEvent['kind'], string> = {
  created: 'created the task',
  stale: 'lease went stale',
  decayed: 'claim lapsed, task abandoned',
  claim: 'claimed it',
  start: 'started it',
  finish: 'finished it',
  release: 'released it',
  assign: 'assigned it',
  unassign: 'removed the assignee',
  reopen: 'reopened it',
  comment: 'commented',
  link: 'linked a message',
  edit: 'edited the task',
}

/** One-line description of a history event, without the actor. */
export function eventText(e: TaskEvent): string {
  const base = EVENT_TEXT[e.kind]
  if (e.kind === 'assign' && e.note) return `${base} to ${e.note}`
  if (e.from_status && e.to_status && e.from_status !== e.to_status) return `${base} (${STATUS_LABEL[e.from_status]} to ${STATUS_LABEL[e.to_status]})`
  return base
}

/** Date part for `<input type="date">` in local time. */
export function toDateInput(iso: string | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

/** A date input value as an RFC 3339 deadline at the end of that local day. */
export function fromDateInput(v: string): string | undefined {
  if (!v) return undefined
  const [y, m, d] = v.split('-').map(Number)
  if (!y || !m || !d) return undefined
  return new Date(y, m - 1, d, 23, 59, 0).toISOString()
}

export function dueLabel(iso: string): string {
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleDateString([], { month: 'short', day: 'numeric' })
}
