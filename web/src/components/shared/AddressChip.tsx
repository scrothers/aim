import { parseAddress } from '@/lib/identity'
import { cn } from '@/lib/utils'

/** Renders `identity@` in the accent colour and the path in muted mono. */
export function AddressChip({ address, className }: { address: string; className?: string }) {
  const { subagent, identity, path } = parseAddress(address)
  return (
    <span className={cn('inline-flex items-baseline', className)} data-testid="address-chip">
      {subagent && <span className="font-semibold text-accent-text">{subagent}<span className="text-faint">@</span></span>}
      {identity && (
        <span className="font-semibold text-accent-text">
          {identity}
          {path && <span className="text-faint">@</span>}
        </span>
      )}
      {path && <span className="font-mono text-[13px] text-muted-foreground">{path}</span>}
    </span>
  )
}
