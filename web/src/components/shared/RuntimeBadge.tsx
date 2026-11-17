import { runtimeFor, runtimeIconUrl } from '@/lib/identity'
import { cn } from '@/lib/utils'

/** Runtime icon on a white chip, or a text badge with the raw client name. */
export function RuntimeBadge({ client, className, size = 18 }: { client?: string; className?: string; size?: number }) {
  const rt = runtimeFor(client)
  if (rt.console) return null
  const url = rt.icon ? runtimeIconUrl(rt.icon) : undefined
  if (url) {
    return (
      <span
        title={rt.label}
        className={cn('box-border flex shrink-0 rounded-[5px] border border-border bg-white p-0.5', className)}
        style={{ width: size, height: size }}
      >
        <img src={url} alt={rt.label} className="size-full object-contain" />
      </span>
    )
  }
  return (
    <span
      title={rt.label}
      className={cn('shrink-0 rounded border border-border bg-code px-1 font-mono text-[10px] text-muted-foreground', className)}
    >
      {rt.label}
    </span>
  )
}
