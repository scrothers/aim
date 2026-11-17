import { cn } from '@/lib/utils'

export function PresenceDot({ online, className }: { online: boolean; className?: string }) {
  return (
    <span
      role="img"
      aria-label={online ? 'online' : 'offline'}
      className={cn('inline-block size-2 shrink-0 rounded-full', online ? 'bg-online' : 'bg-border', className)}
    />
  )
}
