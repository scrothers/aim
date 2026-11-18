import { CalendarClock, Hand, UserRound } from 'lucide-react'
import type { TaskView } from '@/api/tasks'
import { OPERATOR } from '@/lib/address'
import { parseAddress } from '@/lib/identity'
import { cn } from '@/lib/utils'
import { dueLabel, isOverdue } from './model'
import { HealthIndicator, PriorityBadge } from './TaskBadges'

/** Short display name for an identity address: `reviewer@host/org` becomes `reviewer`. */
export function shortName(address: string): string {
  if (address === OPERATOR) return OPERATOR
  const p = parseAddress(address)
  return p.subagent ?? p.identity ?? address
}

function PersonChip({ kind, address }: { kind: 'Assigned to' | 'Claimed by'; address: string }) {
  const Icon = kind === 'Claimed by' ? Hand : UserRound
  return (
    <span
      title={`${kind} ${address}`}
      aria-label={`${kind} ${address}`}
      data-chip={kind === 'Claimed by' ? 'claimant' : 'assignee'}
      className="inline-flex max-w-full items-center gap-1 rounded-full bg-accent-soft px-2 py-px text-xs font-medium text-accent-text"
    >
      <Icon aria-hidden="true" className="size-3 shrink-0" />
      <span className="truncate">{shortName(address)}</span>
    </span>
  )
}

export interface TaskCardProps {
  task: TaskView
  now: number
  dragging: boolean
  onOpen: (id: string) => void
  onDragStart: (id: string) => void
  onDragEnd: () => void
}

export function TaskCard({ task: t, now, dragging, onOpen, onDragStart, onDragEnd }: TaskCardProps) {
  const overdue = isOverdue(t, now)
  return (
    <li
      draggable
      data-testid="task-card"
      data-task-id={t.id}
      data-status={t.status}
      data-dragging={dragging || undefined}
      onDragStart={(e) => {
        e.dataTransfer.effectAllowed = 'move'
        e.dataTransfer.setData('text/plain', t.id)
        onDragStart(t.id)
      }}
      onDragEnd={onDragEnd}
      className={cn(
        'flex min-w-0 cursor-grab flex-col gap-2 rounded-lg border bg-panel p-3 shadow-xs active:cursor-grabbing',
        t.priority === 'urgent' && 'border-l-[3px] border-l-attention',
        dragging && 'opacity-40',
      )}
    >
      <button
        type="button"
        onClick={() => onOpen(t.id)}
        className="min-w-0 rounded text-left text-[13px] leading-[18px] font-semibold break-words text-text hover:underline max-md:min-h-11"
      >
        {t.title}
      </button>
      <div className="flex flex-wrap items-center gap-1.5">
        <PriorityBadge priority={t.priority} />
        <HealthIndicator task={t} now={now} />
        {t.due_at && (
          <span
            data-overdue={overdue || undefined}
            title={overdue ? 'Overdue' : 'Due date'}
            className={cn(
              'inline-flex items-center gap-1 text-xs',
              overdue ? 'rounded bg-attention-soft px-1 font-semibold text-attention-text' : 'text-muted-foreground',
            )}
          >
            <CalendarClock aria-hidden="true" className="size-3" />
            <span className="sr-only">{overdue ? 'Overdue, due ' : 'Due '}</span>
            {dueLabel(t.due_at)}
          </span>
        )}
      </div>
      {t.scope && (
        <span className="truncate font-mono text-xs text-faint" title={t.scope}>
          {t.scope}
        </span>
      )}
      {(t.assignee || t.claimant) && (
        <div className="flex flex-wrap items-center gap-1.5">
          {t.assignee && <PersonChip kind="Assigned to" address={t.assignee} />}
          {t.claimant && <PersonChip kind="Claimed by" address={t.claimant} />}
        </div>
      )}
    </li>
  )
}
