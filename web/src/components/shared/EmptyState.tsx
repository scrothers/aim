import type { ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

/**
 * The one empty state: an icon, one line of text and an optional next action.
 * `compact` fits narrow slots such as a board column or the mailbox list.
 */
export function EmptyState({
  icon: Icon,
  art,
  children,
  action,
  compact = false,
  className,
}: {
  icon?: LucideIcon
  /** Replaces the icon bubble; reserved for the few celebratory states (inbox zero). */
  art?: ReactNode
  /** One line saying what is empty, or what to do. */
  children: ReactNode
  action?: ReactNode
  compact?: boolean
  className?: string
}) {
  return (
    <div
      data-slot="empty-state"
      className={cn(
        'flex flex-col items-center justify-center text-center',
        compact ? 'gap-1.5 rounded-lg border border-dashed px-3 py-6' : 'gap-3 px-4 py-10',
        className,
      )}
    >
      {art ??
        (Icon && (
          <span
            aria-hidden="true"
            className={cn('flex items-center justify-center rounded-full bg-code text-faint', compact ? 'size-7' : 'size-10')}
          >
            <Icon className={compact ? 'size-3.5' : 'size-5'} />
          </span>
        ))}
      <p className={cn('m-0 text-muted-foreground', compact ? 'text-xs' : 'text-[13px]')}>{children}</p>
      {action}
    </div>
  )
}
