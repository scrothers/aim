import { hueFor, initialsFor } from '@/lib/identity'
import { cn } from '@/lib/utils'
import { RuntimeBadge } from './RuntimeBadge'

export function avatarColor(name: string): string {
  return `hsl(${hueFor(name)} 55% 38%)`
}

export function AgentAvatar({
  name,
  client,
  size = 32,
  className,
}: {
  name: string
  client?: string
  size?: number
  className?: string
}) {
  return (
    <span
      aria-hidden="true"
      className={cn('relative flex shrink-0 items-center justify-center rounded-full font-bold text-white', className)}
      style={{ width: size, height: size, background: avatarColor(name), fontSize: Math.round(size * 0.37) }}
    >
      {initialsFor(name)}
      {client !== undefined && <RuntimeBadge client={client} size={Math.round(size * 0.56)} className="absolute -right-1 -bottom-1" />}
    </span>
  )
}
