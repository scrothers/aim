import { cn } from '@/lib/utils'

/** Online is a solid green dot, idle a hollow amber ring, offline a grey dot. */
export function PresenceDot({ online, idle = false, className }: { online: boolean; idle?: boolean; className?: string }) {
  const isIdle = online && idle
  return (
    <span
      role="img"
      aria-label={isIdle ? 'idle' : online ? 'online' : 'offline'}
      className={cn(
        'inline-block size-2 shrink-0 rounded-full',
        isIdle ? 'border-[1.5px] border-idle bg-transparent' : online ? 'bg-online' : 'bg-border',
        className,
      )}
    />
  )
}
