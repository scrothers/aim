import { Activity, Ban, Clock, TriangleAlert, type LucideIcon } from 'lucide-react'
import type { TaskHealth, TaskPriority, TaskStatus, TaskView } from '@/api/tasks'
import { ago } from '@/lib/identity'
import { cn } from '@/lib/utils'
import { HEALTH_LABEL, STATUS_LABEL } from './model'

const PRIORITY_CLS: Record<TaskPriority, string> = {
  low: 'bg-code text-faint',
  normal: 'bg-code text-muted-foreground',
  high: 'bg-accent-soft text-accent-text',
  urgent: 'bg-attention-soft text-attention-text',
}

export function PriorityBadge({ priority, className }: { priority: TaskPriority; className?: string }) {
  return (
    <span
      data-priority={priority}
      className={cn('inline-flex h-[18px] items-center rounded px-1.5 text-[11px] leading-none font-semibold uppercase tracking-wide', PRIORITY_CLS[priority], className)}
    >
      {priority}
    </span>
  )
}

// Each health state differs in icon and colour, so colour is never the only cue.
const HEALTH: Record<TaskHealth, { icon: LucideIcon; cls: string }> = {
  active: { icon: Activity, cls: 'text-online-text bg-online-soft' },
  idle: { icon: Clock, cls: 'text-idle bg-code' },
  stale: { icon: TriangleAlert, cls: 'text-attention-text bg-attention-soft' },
  abandoned: { icon: Ban, cls: 'text-faint bg-code' },
}

/** Lease health of a claimed or started task, with last activity in the tooltip. */
export function HealthIndicator({ task, now, showLabel }: { task: Pick<TaskView, 'health' | 'last_activity_at'>; now?: number; showLabel?: boolean }) {
  if (!task.health) return null
  const { icon: Icon, cls } = HEALTH[task.health]
  const when = task.last_activity_at ? `last activity ${ago(task.last_activity_at, now)} ago` : 'no activity recorded'
  const label = `Health: ${HEALTH_LABEL[task.health].toLowerCase()}, ${when}`
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      data-health={task.health}
      className={cn('inline-flex h-[18px] items-center gap-1 rounded px-1 text-[11px] font-medium', cls)}
    >
      <Icon aria-hidden="true" className="size-3" />
      {showLabel && HEALTH_LABEL[task.health]}
    </span>
  )
}

const STATUS_DOT: Record<TaskStatus, string> = {
  not_started: 'border-2 border-faint',
  claimed: 'bg-idle',
  started: 'bg-online',
  finished: 'bg-faint',
  abandoned: 'border-2 border-attention',
}

export function StatusDot({ status, className }: { status: TaskStatus; className?: string }) {
  return <span aria-hidden="true" className={cn('inline-block size-2.5 shrink-0 rounded-full', STATUS_DOT[status], className)} />
}

export function StatusLabel({ status }: { status: TaskStatus }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs font-medium">
      <StatusDot status={status} />
      {STATUS_LABEL[status]}
    </span>
  )
}
