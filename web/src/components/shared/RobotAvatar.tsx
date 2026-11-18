import { useId } from 'react'
import { OPERATOR } from '@/lib/address'
import { robotParts, type RobotPart } from '@/lib/robot'
import { cn } from '@/lib/utils'
import { RuntimeBadge } from './RuntimeBadge'

/** At or below this size the simplified head-crop variant renders. */
export const COMPACT_MAX_SIZE = 24

/**
 * The operator's mark, drawn like the AIM logo: a sunflower rounded square
 * with a thin near-black outline and a near-black terminal prompt. Fixed
 * colours, so it looks the same in both themes.
 */
export function ConsoleMark({ size, className }: { size: number; className?: string }) {
  return (
    <span
      aria-hidden="true"
      data-testid="console-mark"
      className={cn(
        'flex shrink-0 items-center justify-center rounded-[22%] border-[#18181b] bg-[#f5c518] font-mono font-bold tracking-tighter text-[#18181b]',
        size >= 28 ? 'border-[1.5px]' : 'border',
        className,
      )}
      style={{ width: size, height: size, fontSize: Math.round(size * 0.42) }}
    >
      {'>_'}
    </span>
  )
}

function Part({ part, clipId }: { part: RobotPart; clipId: string }) {
  const Tag = part.tag
  return <Tag {...part.attrs} clipPath={part.clip ? `url(#${clipId})` : undefined} />
}

/**
 * Procedural robot for an address (the operator gets the console mark).
 * Decorative by default, since every call site sits next to text naming the
 * identity; pass `decorative={false}` to expose it as a labelled image.
 */
export function RobotAvatar({
  address,
  client,
  size = 32,
  decorative = true,
  className,
}: {
  address: string
  client?: string
  size?: number
  decorative?: boolean
  className?: string
}) {
  const uid = useId().replace(/[^a-zA-Z0-9_-]/g, '')
  const clipId = `robot-clip-${uid}`
  const canonical = address.trim().toLowerCase()
  const isOperator = canonical === OPERATOR
  const compact = size <= COMPACT_MAX_SIZE
  const shape = isOperator ? null : robotParts(canonical, { compact })
  const a11y = decorative
    ? ({ 'aria-hidden': true } as const)
    : ({ role: 'img', 'aria-label': `Robot avatar for ${address}` } as const)

  return (
    <span className={cn('relative inline-flex shrink-0', className)} style={{ width: size, height: size }} data-testid="robot-avatar" data-compact={compact || undefined}>
      {shape ? (
        <svg {...a11y} xmlns="http://www.w3.org/2000/svg" viewBox={shape.viewBox} width={size} height={size}>
          {!compact && (
            <defs>
              <clipPath id={clipId}>
                <rect width="64" height="64" rx="14" />
              </clipPath>
            </defs>
          )}
          {shape.parts.map((p, i) => (
            <Part key={i} part={p} clipId={clipId} />
          ))}
        </svg>
      ) : (
        <ConsoleMark size={size} />
      )}
      {!isOperator && client !== undefined && <RuntimeBadge client={client} size={Math.round(size * 0.56)} className="absolute -right-1 -bottom-1" />}
    </span>
  )
}
