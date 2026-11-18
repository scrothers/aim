import { Activity, Ban, Clock, TriangleAlert, type LucideIcon } from 'lucide-react'
import type { TaskHealth, TaskPriority, TaskStatus, TaskView } from '@/api/tasks'
import { ago } from '@/lib/identity'
import { cn } from '@/lib/utils'
import { Badge, type BadgeTone } from '@/components/ui/badge'
import { HEALTH_LABEL, STATUS_LABEL } from './model'

const PRIORITY_TONE: Record<TaskPriority, BadgeTone> = {
  low: 'faint',
  normal: 'neutral',
  high: 'brand',
  urgent: 'attention',
}

export function PriorityBadge({ priority, className }: { priority: TaskPriority; className?: string }) {
  return (
    <Badge variant="priority" tone={PRIORITY_TONE[priority]} data-priority={priority} className={className}>
      {priority}
    </Badge>
  )
}

// Each health state differs in icon and colour, so colour is never the only cue.
const HEALTH: Record<TaskHealth, { icon: LucideIcon; tone: BadgeTone }> = {
  active: { icon: Activity, tone: 'online' },
  idle: { icon: Clock, tone: 'idle' },
  stale: { icon: TriangleAlert, tone: 'attention' },
  abandoned: { icon: Ban, tone: 'faint' },
}

/** Lease health of a claimed or started task, with last activity in the tooltip. */
export function HealthIndicator({ task, now, showLabel }: { task: Pick<TaskView, 'health' | 'last_activity_at'>; now?: number; showLabel?: boolean }) {
  if (!task.health) return null
  const { icon: Icon, tone } = HEALTH[task.health]
  const when = task.last_activity_at ? `last activity ${ago(task.last_activity_at, now)} ago` : 'no activity recorded'
  const label = `Health: ${HEALTH_LABEL[task.health].toLowerCase()}, ${when}`
  return (
    <Badge variant="health" tone={tone} role="img" aria-label={label} title={label} data-health={task.health}>
      <Icon aria-hidden="true" />
      {showLabel && HEALTH_LABEL[task.health]}
    </Badge>
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
    <Badge variant="status" data-status={status}>
      <StatusDot status={status} />
      {STATUS_LABEL[status]}
    </Badge>
  )
}
