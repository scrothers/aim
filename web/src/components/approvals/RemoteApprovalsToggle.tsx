import { Link } from '@tanstack/react-router'
import { ShieldCheck } from 'lucide-react'
import { useApprovals, useApprovalSettings, useSaveApprovalSettings } from '@/api/approvals'
import { cn } from '@/lib/utils'

export function timeoutLabel(seconds: number): string {
  if (seconds % 60 === 0) return `${seconds / 60} min`
  return `${seconds} s`
}

/** Header switch for remote approvals plus a badge linking to the pending list. */
export function RemoteApprovalsToggle() {
  const settings = useApprovalSettings().data
  const pending = useApprovals().data?.items?.length ?? 0
  const save = useSaveApprovalSettings()
  const enabled = settings?.enabled ?? false
  const timeout = settings?.timeout_seconds ?? 300
  const hint = enabled
    ? `On: agent prompts wait here up to ${timeoutLabel(timeout)}, then fall back to the terminal.`
    : 'Off: agents prompt in their terminal. Turn on to approve their prompts here.'
  return (
    <span className="flex items-center gap-1.5">
      <button
        type="button"
        role="switch"
        aria-checked={enabled}
        aria-label="Remote approvals"
        title={hint}
        disabled={!settings || save.isPending}
        onClick={() => save.mutate({ enabled: !enabled, timeout_seconds: timeout })}
        className="flex h-8 items-center gap-2 rounded-lg border px-2 text-[13px] text-muted-foreground hover:bg-hover disabled:opacity-50"
      >
        <ShieldCheck aria-hidden="true" className="size-4" />
        <span className="max-lg:hidden">Remote approvals</span>
        <span
          aria-hidden="true"
          className={cn('relative h-4 w-7 rounded-full transition-colors', enabled ? 'bg-brand' : 'bg-border')}
        >
          <span className={cn('absolute top-0.5 size-3 rounded-full bg-white transition-all', enabled ? 'left-3.5' : 'left-0.5')} />
        </span>
      </button>
      {pending > 0 && (
        <Link
          to="/approvals"
          aria-label={`${pending} pending ${pending === 1 ? 'approval' : 'approvals'}`}
          className="h-[18px] min-w-5 rounded-[9px] bg-brand text-center text-xs leading-[18px] text-on-accent"
        >
          {pending}
        </Link>
      )}
    </span>
  )
}
