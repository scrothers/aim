import { hueFor, initialsFor } from '@/lib/identity'
import { OPERATOR } from '@/lib/address'
import { cn } from '@/lib/utils'
import { RuntimeBadge } from './RuntimeBadge'

export function avatarColor(name: string): string {
  return `hsl(${hueFor(name)} 55% 38%)`
}

/** The operator's mark: a dark rounded square with a terminal prompt. */
function ConsoleMark({ size, className }: { size: number; className?: string }) {
  return (
    <span
      aria-hidden="true"
      data-testid="console-mark"
      className={cn('flex shrink-0 items-center justify-center rounded-lg border border-white/15 bg-[#18181b] font-mono font-bold text-white', className)}
      style={{ width: size, height: size, fontSize: Math.round(size * 0.4) }}
    >
      {'>_'}
    </span>
  )
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
  if (name === OPERATOR) return <ConsoleMark size={size} className={className} />
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
